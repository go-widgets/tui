// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui/syntax"
)

// TextEditor is a cell-native, read-write multi-line text editor widget with
// syntax highlighting and an optional line-number gutter. It is a
// toolkit.Widget: give it a bounds, set Focused, and forward events to
// OnEvent.
//
//   - One cell per glyph, one row per line (no soft wrap).
//   - Filename's extension drives the syntax language (see the syntax package);
//     "" leaves the text plain. Highlighting is recomputed on every change so
//     it tracks live edits.
//   - Set ReadOnly to use it as a code viewer: insert / delete / newline events
//     are ignored, but the caret still navigates (arrows, click).
//   - ShowGutter draws a right-aligned line-number gutter (NewTextEditor
//     enables it).
type TextEditor struct {
	toolkit.Base
	Lines      []string
	CursorLine int
	CursorCol  int
	Focused    bool
	Filename   string
	ReadOnly   bool
	ShowGutter bool

	spans      [][]syntax.Span
	undo, redo []teSnapshot
	lastQuery  string // remembered by Find for FindNext

	// Selection: anchored at (anchorLine, anchorCol), with the caret as the
	// moving end; active only while selActive. clip is the widget-local
	// clipboard for Copy/Cut/Paste.
	anchorLine, anchorCol int
	selActive             bool
	clip                  string

	scrollY int // first visible line; the viewport follows the caret
}

// scrollToCaret adjusts the scroll offset so the caret's line stays within the
// visible rows (the pane height). Called at the top of Draw.
func (t *TextEditor) scrollToCaret() {
	h := t.Bounds().H
	if h <= 0 {
		return
	}
	if t.CursorLine < t.scrollY {
		t.scrollY = t.CursorLine
	} else if t.CursorLine >= t.scrollY+h {
		t.scrollY = t.CursorLine - h + 1
	}
}

// selRange returns the normalised selection as [start, end) plus whether a
// non-empty selection is active.
func (t *TextEditor) selRange() (sl, sc, el, ec int, ok bool) {
	if !t.selActive {
		return 0, 0, 0, 0, false
	}
	al, ac, cl, cc := t.anchorLine, t.anchorCol, t.CursorLine, t.CursorCol
	if al > cl || (al == cl && ac > cc) {
		al, ac, cl, cc = cl, cc, al, ac
	}
	return al, ac, cl, cc, al != cl || ac != cc
}

// SelectedText returns the text covered by the active selection, or "" if none.
func (t *TextEditor) SelectedText() string {
	sl, sc, el, ec, ok := t.selRange()
	if !ok {
		return ""
	}
	if sl == el {
		return t.Lines[sl][sc:ec]
	}
	var b strings.Builder
	b.WriteString(t.Lines[sl][sc:])
	for i := sl + 1; i < el; i++ {
		b.WriteByte('\n')
		b.WriteString(t.Lines[i])
	}
	b.WriteByte('\n')
	b.WriteString(t.Lines[el][:ec])
	return b.String()
}

// Copy stores the selection in the widget clipboard and returns it.
func (t *TextEditor) Copy() string {
	t.clip = t.SelectedText()
	return t.clip
}

// deleteRange removes [sl,sc)-(el,ec], leaving the caret at the start and no
// active selection. The caller records undo.
func (t *TextEditor) deleteRange(sl, sc, el, ec int) {
	if sl == el {
		t.Lines[sl] = t.Lines[sl][:sc] + t.Lines[sl][ec:]
	} else {
		t.Lines[sl] = t.Lines[sl][:sc] + t.Lines[el][ec:]
		t.Lines = append(t.Lines[:sl+1], t.Lines[el+1:]...)
	}
	t.CursorLine, t.CursorCol = sl, sc
	t.selActive = false
}

// DeleteSelection removes the selected text (a single undo step). Returns
// whether anything was deleted.
func (t *TextEditor) DeleteSelection() bool {
	sl, sc, el, ec, ok := t.selRange()
	if !ok || t.ReadOnly {
		return false
	}
	t.pushUndo()
	t.deleteRange(sl, sc, el, ec)
	t.rehighlight()
	return true
}

// Cut copies the selection to the clipboard and removes it, returning the text.
func (t *TextEditor) Cut() string {
	s := t.Copy()
	if s != "" {
		t.DeleteSelection()
	}
	return s
}

// insertText inserts s (which may contain newlines) at the caret, moving the
// caret to the end of the inserted text. The caller records undo.
func (t *TextEditor) insertText(s string) {
	parts := strings.Split(s, "\n")
	line := t.Lines[t.CursorLine]
	head, tail := line[:t.CursorCol], line[t.CursorCol:]
	if len(parts) == 1 {
		t.Lines[t.CursorLine] = head + parts[0] + tail
		t.CursorCol += len(parts[0])
		return
	}
	inserted := make([]string, 0, len(parts))
	inserted = append(inserted, head+parts[0])
	inserted = append(inserted, parts[1:len(parts)-1]...)
	last := parts[len(parts)-1]
	inserted = append(inserted, last+tail)
	rest := append([]string(nil), t.Lines[t.CursorLine+1:]...)
	t.Lines = append(t.Lines[:t.CursorLine], append(inserted, rest...)...)
	t.CursorLine += len(parts) - 1
	t.CursorCol = len(last)
}

// Paste inserts the clipboard at the caret, replacing an active selection. A
// single undo step. No-op when ReadOnly or the clipboard is empty.
func (t *TextEditor) Paste() {
	if t.ReadOnly || t.clip == "" {
		return
	}
	t.pushUndo()
	if sl, sc, el, ec, ok := t.selRange(); ok {
		t.deleteRange(sl, sc, el, ec)
	}
	t.insertText(t.clip)
	t.rehighlight()
}

// dropSelectionForEdit deletes the active selection without pushing undo -- used
// at the start of an insert/delete so typing replaces the selection within the
// caller's single undo step. Returns whether a selection was deleted.
func (t *TextEditor) dropSelectionForEdit() bool {
	if sl, sc, el, ec, ok := t.selRange(); ok {
		t.deleteRange(sl, sc, el, ec)
		return true
	}
	return false
}

// Find moves the caret to the next occurrence of query at or after the current
// caret, searching forward and wrapping to the top; it returns whether a match
// was found. An empty query is a no-op. The query is remembered for FindNext.
func (t *TextEditor) Find(query string) bool {
	t.lastQuery = query
	if query == "" {
		return false
	}
	return t.searchForward(t.CursorLine, t.CursorCol, query)
}

// FindNext repeats the last Find starting just past the caret, so successive
// calls walk consecutive matches (wrapping). No-op with no prior Find.
func (t *TextEditor) FindNext() bool {
	if t.lastQuery == "" {
		return false
	}
	return t.searchForward(t.CursorLine, t.CursorCol+1, t.lastQuery)
}

// Replace finds the next occurrence of query (forward from the caret, wrapping)
// and replaces it with repl, leaving the caret just after the replacement so a
// repeated call walks to the next one. Returns whether a replacement happened.
// No-op (false) when ReadOnly or query is empty. Undoable as one step.
func (t *TextEditor) Replace(query, repl string) bool {
	if t.ReadOnly || query == "" {
		return false
	}
	if !t.searchForward(t.CursorLine, t.CursorCol, query) {
		return false
	}
	t.pushUndo()
	line := t.Lines[t.CursorLine]
	t.Lines[t.CursorLine] = line[:t.CursorCol] + repl + line[t.CursorCol+len(query):]
	t.CursorCol += len(repl)
	t.rehighlight()
	return true
}

// ReplaceAll replaces every occurrence of query with repl across the whole
// buffer and returns the number replaced. The caret column is clamped to its
// (possibly shorter) line. No-op (0) when ReadOnly or query is empty; the whole
// operation is a single undo step.
func (t *TextEditor) ReplaceAll(query, repl string) int {
	if t.ReadOnly || query == "" {
		return 0
	}
	total := 0
	for _, line := range t.Lines {
		total += strings.Count(line, query)
	}
	if total == 0 {
		return 0
	}
	t.pushUndo()
	for i := range t.Lines {
		t.Lines[i] = strings.ReplaceAll(t.Lines[i], query, repl)
	}
	t.clampCol()
	t.rehighlight()
	return total
}

// searchForward scans lines from (startLine, startCol) forward, wrapping once
// through the whole buffer (the k==len pass re-checks the start line from its
// beginning so a match before startCol is still found). On a hit it moves the
// caret to the match start and returns true.
func (t *TextEditor) searchForward(startLine, startCol int, query string) bool {
	n := len(t.Lines)
	if n == 0 {
		return false
	}
	for k := 0; k <= n; k++ {
		li := (startLine + k) % n
		line := t.Lines[li]
		from := 0
		if k == 0 {
			// startCol comes from the caret (Find) or caret+1 (FindNext), so
			// it is never negative; a value past the line end skips this line's
			// first pass and the search resumes on the next line.
			from = startCol
			if from > len(line) {
				continue
			}
		}
		if idx := strings.Index(line[from:], query); idx >= 0 {
			t.CursorLine = li
			t.CursorCol = from + idx
			return true
		}
	}
	return false
}

// maxUndo caps the undo history so a long editing session can't grow the
// snapshot stack without bound.
const maxUndo = 200

// teSnapshot is a point-in-time copy of the buffer + caret for undo/redo.
type teSnapshot struct {
	lines []string
	line  int
	col   int
}

func (t *TextEditor) snapshot() teSnapshot {
	cp := make([]string, len(t.Lines))
	copy(cp, t.Lines)
	return teSnapshot{lines: cp, line: t.CursorLine, col: t.CursorCol}
}

func (t *TextEditor) restore(s teSnapshot) {
	t.Lines = s.lines
	t.CursorLine, t.CursorCol = s.line, s.col
}

// pushUndo records the current state before a mutation and drops any redo
// history (a fresh edit invalidates the redo branch).
func (t *TextEditor) pushUndo() {
	t.undo = append(t.undo, t.snapshot())
	if len(t.undo) > maxUndo {
		t.undo = t.undo[len(t.undo)-maxUndo:]
	}
	t.redo = nil
}

// NewTextEditor returns an empty editor with the line-number gutter enabled.
func NewTextEditor() *TextEditor {
	t := &TextEditor{Lines: []string{""}, ShowGutter: true}
	t.rehighlight()
	return t
}

// Text returns the buffer joined by newlines (no trailing newline).
func (t *TextEditor) Text() string {
	if len(t.Lines) == 0 {
		return ""
	}
	total := 0
	for _, l := range t.Lines {
		total += len(l) + 1
	}
	buf := make([]byte, 0, total)
	for i, l := range t.Lines {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, l...)
	}
	return string(buf)
}

// SetText replaces the buffer (split on "\n"), resets the caret to the top,
// and re-highlights.
func (t *TextEditor) SetText(s string) {
	t.Lines = nil
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			t.Lines = append(t.Lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		t.Lines = append(t.Lines, s[start:])
	}
	if len(t.Lines) == 0 {
		t.Lines = []string{""}
	}
	t.CursorLine = 0
	t.CursorCol = 0
	t.rehighlight()
}

func (t *TextEditor) rehighlight() {
	t.spans = syntax.Highlight(t.Text(), t.Filename)
}

// leftPad is the number of cells to the left of the code: the gutter width when
// ShowGutter, otherwise a single-cell margin.
func (t *TextEditor) leftPad() int {
	if t.ShowGutter {
		return GutterWidth(len(t.Lines))
	}
	return 1
}

// Draw paints the pane background, the (optional) line-number gutter, the
// syntax-highlighted text, and the caret.
func (t *TextEditor) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := t.Bounds()
	t.scrollToCaret() // keep the caret in view; scrollY stays 0 for short buffers
	// SurfaceAlt background so the pane edge is visible against the frame in
	// any theme.
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	pad := t.leftPad()
	numInk := LineNumberInk(theme)
	// Selection highlight, painted under the gutter numbers + code.
	if sl, sc, el, ec, ok := t.selRange(); ok {
		selBg := painter.RGBA{R: theme.Accent.R, G: theme.Accent.G, B: theme.Accent.B, A: theme.Accent.A}
		for li := sl; li <= el; li++ {
			y := r.Y + (li - t.scrollY)
			if li >= len(t.Lines) || li < t.scrollY || y >= r.Y+r.H {
				continue
			}
			startC, endC := 0, len(t.Lines[li])
			if li == sl {
				startC = sc
			}
			if li == el {
				endC = ec
			}
			w := endC - startC
			if li != el {
				w++ // include the line-break cell so full-line spans read solid
			}
			if w > 0 {
				pnt.FillRect(painter.Rect{X: r.X + pad + startC, Y: y, W: w, H: 1}, selBg)
			}
		}
	}
	for i := t.scrollY; i < len(t.spans); i++ {
		y := r.Y + (i - t.scrollY)
		if y >= r.Y+r.H {
			break
		}
		if t.ShowGutter {
			num := strconv.Itoa(i + 1)
			toolkit.DrawText(pnt, r.X+pad-1-len(num), y, num, numInk)
		}
		x := r.X + pad
		for _, sp := range t.spans[i] {
			toolkit.DrawText(pnt, x, y, sp.Text, SyntaxInk(sp.Kind, theme))
			x += utf8.RuneCountInString(sp.Text) // 1 cell per rune
		}
	}
	// Caret: a reverse-video block one cell wide at the cursor.
	if t.Focused && t.CursorLine >= t.scrollY {
		cx := r.X + pad + t.CursorCol
		cy := r.Y + (t.CursorLine - t.scrollY)
		if cx < r.X+r.W && cy < r.Y+r.H {
			pnt.FillRect(painter.Rect{X: cx, Y: cy, W: 1, H: 1}, painter.RGBA{
				R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A,
			})
		}
	}
}

// OnEvent applies one input event: character insert, Backspace, Delete, Enter,
// Ctrl+Z/Y (undo/redo), Ctrl+X/V (cut/paste) -- all no-ops when ReadOnly -- the
// arrow / Home / End / PageUp / PageDown navigation keys, or a click / drag
// (caret + selection). Re-highlights after any change.
func (t *TextEditor) OnEvent(ev toolkit.Event) {
	if len(t.Lines) == 0 {
		t.Lines = []string{""}
	}
	switch ev.Kind {
	case toolkit.EventChar:
		if !t.ReadOnly {
			t.pushUndo()
			t.dropSelectionForEdit()
			line := t.Lines[t.CursorLine]
			if t.CursorCol > len(line) {
				t.CursorCol = len(line)
			}
			t.Lines[t.CursorLine] = line[:t.CursorCol] + ev.Code + line[t.CursorCol:]
			t.CursorCol += len(ev.Code)
		}
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Backspace":
			if !t.ReadOnly {
				t.pushUndo()
				if !t.dropSelectionForEdit() {
					t.backspace()
				}
			}
		case "Enter":
			if !t.ReadOnly {
				t.pushUndo()
				t.dropSelectionForEdit()
				t.splitLine()
			}
		case "Ctrl+X":
			if !t.ReadOnly {
				t.Cut()
			}
		case "Ctrl+V":
			if !t.ReadOnly {
				t.Paste()
			}
		case "Ctrl+Z":
			if !t.ReadOnly && len(t.undo) > 0 {
				t.redo = append(t.redo, t.snapshot())
				t.restore(t.undo[len(t.undo)-1])
				t.undo = t.undo[:len(t.undo)-1]
			}
		case "Ctrl+Y":
			if !t.ReadOnly && len(t.redo) > 0 {
				t.undo = append(t.undo, t.snapshot())
				t.restore(t.redo[len(t.redo)-1])
				t.redo = t.redo[:len(t.redo)-1]
			}
		case "Up":
			if t.CursorLine > 0 {
				t.CursorLine--
				t.clampCol()
			}
			t.selActive = false
		case "Down":
			if t.CursorLine < len(t.Lines)-1 {
				t.CursorLine++
				t.clampCol()
			}
			t.selActive = false
		case "Left":
			if t.CursorCol > 0 {
				t.CursorCol--
			}
			t.selActive = false
		case "Right":
			if t.CursorCol < len(t.Lines[t.CursorLine]) {
				t.CursorCol++
			}
			t.selActive = false
		case "Home":
			t.CursorCol = 0
			t.selActive = false
		case "End":
			t.CursorCol = len(t.Lines[t.CursorLine])
			t.selActive = false
		case "PageUp":
			t.CursorLine -= t.page()
			if t.CursorLine < 0 {
				t.CursorLine = 0
			}
			t.clampCol()
			t.selActive = false
		case "PageDown":
			t.CursorLine += t.page()
			if t.CursorLine > len(t.Lines)-1 {
				t.CursorLine = len(t.Lines) - 1
			}
			t.clampCol()
			t.selActive = false
		case "Delete":
			if !t.ReadOnly {
				t.pushUndo()
				if !t.dropSelectionForEdit() {
					t.deleteForward()
				}
			}
		case "Tab":
			if !t.ReadOnly {
				t.pushUndo()
				if sl, _, el, _, ok := t.selRange(); ok && sl != el {
					t.indentLines(sl, el, +1)
				} else {
					t.dropSelectionForEdit()
					t.insertText(strings.Repeat(" ", indentWidth))
				}
			}
		case "Shift+Tab":
			if !t.ReadOnly {
				t.pushUndo()
				sl, _, el, _, ok := t.selRange()
				if !ok || sl == el {
					sl, el = t.CursorLine, t.CursorLine
				}
				t.indentLines(sl, el, -1)
			}
		case "Alt+Up":
			if !t.ReadOnly {
				t.pushUndo()
				t.moveLineUp()
				t.selActive = false
			}
		case "Alt+Down":
			if !t.ReadOnly {
				t.pushUndo()
				t.moveLineDown()
				t.selActive = false
			}
		}
	case toolkit.EventClick:
		// A click positions the caret and drops the anchor there; a subsequent
		// drag from here grows the selection.
		t.clickAt(ev.X, ev.Y)
		t.anchorLine, t.anchorCol = t.CursorLine, t.CursorCol
		t.selActive = false
	case toolkit.EventMouseDrag:
		// Extend the selection: the caret follows the drag, anchored at the
		// press position.
		t.clickAt(ev.X, ev.Y)
		t.selActive = t.anchorLine != t.CursorLine || t.anchorCol != t.CursorCol
	}
	t.rehighlight()
}

func (t *TextEditor) clampCol() {
	if t.CursorCol > len(t.Lines[t.CursorLine]) {
		t.CursorCol = len(t.Lines[t.CursorLine])
	}
}

// page is the number of lines a PageUp/PageDown moves the caret: one viewport
// minus a line of overlap, at least 1.
func (t *TextEditor) page() int {
	if h := t.Bounds().H - 1; h > 1 {
		return h
	}
	return 1
}

// indentWidth is the number of spaces one Tab / Shift+Tab step adds or removes.
const indentWidth = 4

// indentLines indents (dir=+1) or dedents (dir=-1) every line in [sl, el],
// keeping the caret + selection anchor aligned with their text. The caller
// records undo.
func (t *TextEditor) indentLines(sl, el, dir int) {
	for li := sl; li <= el; li++ {
		before := len(t.Lines[li])
		if dir > 0 {
			t.Lines[li] = strings.Repeat(" ", indentWidth) + t.Lines[li]
		} else {
			n := 0
			for n < indentWidth && n < len(t.Lines[li]) && t.Lines[li][n] == ' ' {
				n++
			}
			t.Lines[li] = t.Lines[li][n:]
		}
		delta := len(t.Lines[li]) - before
		if li == t.CursorLine {
			t.CursorCol += delta
			if t.CursorCol < 0 {
				t.CursorCol = 0
			}
		}
		if li == t.anchorLine {
			t.anchorCol += delta
			if t.anchorCol < 0 {
				t.anchorCol = 0
			}
		}
	}
}

// moveLineUp swaps the caret's line with the one above (no-op at the top). The
// caller records undo.
func (t *TextEditor) moveLineUp() {
	if t.CursorLine > 0 {
		t.Lines[t.CursorLine-1], t.Lines[t.CursorLine] = t.Lines[t.CursorLine], t.Lines[t.CursorLine-1]
		t.CursorLine--
	}
}

// moveLineDown swaps the caret's line with the one below (no-op at the bottom).
// The caller records undo.
func (t *TextEditor) moveLineDown() {
	if t.CursorLine < len(t.Lines)-1 {
		t.Lines[t.CursorLine+1], t.Lines[t.CursorLine] = t.Lines[t.CursorLine], t.Lines[t.CursorLine+1]
		t.CursorLine++
	}
}

// deleteForward removes the character after the caret, or joins the next line
// when the caret is at end-of-line. The caller records undo.
func (t *TextEditor) deleteForward() {
	line := t.Lines[t.CursorLine]
	switch {
	case t.CursorCol < len(line):
		t.Lines[t.CursorLine] = line[:t.CursorCol] + line[t.CursorCol+1:]
	case t.CursorLine < len(t.Lines)-1:
		t.Lines[t.CursorLine] = line + t.Lines[t.CursorLine+1]
		t.Lines = append(t.Lines[:t.CursorLine+1], t.Lines[t.CursorLine+2:]...)
	}
}

func (t *TextEditor) backspace() {
	line := t.Lines[t.CursorLine]
	switch {
	case t.CursorCol > 0 && t.CursorCol <= len(line):
		t.Lines[t.CursorLine] = line[:t.CursorCol-1] + line[t.CursorCol:]
		t.CursorCol--
	case t.CursorCol == 0 && t.CursorLine > 0:
		prev := t.Lines[t.CursorLine-1]
		t.CursorCol = len(prev)
		t.Lines[t.CursorLine-1] = prev + line
		t.Lines = append(t.Lines[:t.CursorLine], t.Lines[t.CursorLine+1:]...)
		t.CursorLine--
	}
}

func (t *TextEditor) splitLine() {
	line := t.Lines[t.CursorLine]
	if t.CursorCol > len(line) {
		t.CursorCol = len(line)
	}
	head, tail := line[:t.CursorCol], line[t.CursorCol:]
	t.Lines[t.CursorLine] = head
	t.Lines = append(t.Lines[:t.CursorLine+1], append([]string{tail}, t.Lines[t.CursorLine+1:]...)...)
	t.CursorLine++
	t.CursorCol = 0
}

func (t *TextEditor) clickAt(evx, evy int) {
	y := evy + t.scrollY // screen row -> buffer line
	if y < 0 {
		y = 0
	}
	if y >= len(t.Lines) {
		y = len(t.Lines) - 1
	}
	t.CursorLine = y
	x := evx - t.leftPad() // the click X is past the gutter
	if x < 0 {
		x = 0
	}
	if x > len(t.Lines[t.CursorLine]) {
		x = len(t.Lines[t.CursorLine])
	}
	t.CursorCol = x
}
