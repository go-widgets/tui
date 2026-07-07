// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"
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

	spans [][]syntax.Span
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
	// SurfaceAlt background so the pane edge is visible against the frame in
	// any theme.
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	pad := t.leftPad()
	numInk := LineNumberInk(theme)
	for i, line := range t.spans {
		y := r.Y + i
		if y >= r.Y+r.H {
			break
		}
		if t.ShowGutter {
			num := strconv.Itoa(i + 1)
			toolkit.DrawText(pnt, r.X+pad-1-len(num), y, num, numInk)
		}
		x := r.X + pad
		for _, sp := range line {
			toolkit.DrawText(pnt, x, y, sp.Text, SyntaxInk(sp.Kind, theme))
			x += utf8.RuneCountInString(sp.Text) // 1 cell per rune
		}
	}
	// Caret: a reverse-video block one cell wide at the cursor.
	if t.Focused && t.CursorLine >= 0 && t.CursorLine < r.H {
		cx := r.X + pad + t.CursorCol
		cy := r.Y + t.CursorLine
		if cx < r.X+r.W && cy < r.Y+r.H {
			pnt.FillRect(painter.Rect{X: cx, Y: cy, W: 1, H: 1}, painter.RGBA{
				R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A,
			})
		}
	}
}

// OnEvent applies one input event: character insert, Backspace, Enter (unless
// ReadOnly), the arrow keys, or a click (which positions the caret past the
// gutter). Re-highlights after any change.
func (t *TextEditor) OnEvent(ev toolkit.Event) {
	if len(t.Lines) == 0 {
		t.Lines = []string{""}
	}
	switch ev.Kind {
	case toolkit.EventChar:
		if !t.ReadOnly {
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
				t.backspace()
			}
		case "Enter":
			if !t.ReadOnly {
				t.splitLine()
			}
		case "Up":
			if t.CursorLine > 0 {
				t.CursorLine--
				t.clampCol()
			}
		case "Down":
			if t.CursorLine < len(t.Lines)-1 {
				t.CursorLine++
				t.clampCol()
			}
		case "Left":
			if t.CursorCol > 0 {
				t.CursorCol--
			}
		case "Right":
			if t.CursorCol < len(t.Lines[t.CursorLine]) {
				t.CursorCol++
			}
		}
	case toolkit.EventClick:
		t.clickAt(ev.X, ev.Y)
	}
	t.rehighlight()
}

func (t *TextEditor) clampCol() {
	if t.CursorCol > len(t.Lines[t.CursorLine]) {
		t.CursorCol = len(t.Lines[t.CursorLine])
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
	y := evy
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
