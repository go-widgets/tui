// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func mkPainter(w, h int) *painter.PixelPainter {
	return painter.NewPixelPainter(make([]byte, w*h*4), w, h)
}

func key(code string) toolkit.Event  { return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code} }
func char(s string) toolkit.Event    { return toolkit.Event{Kind: toolkit.EventChar, Code: s} }
func click(x, y int) toolkit.Event   { return toolkit.Event{Kind: toolkit.EventClick, X: x, Y: y} }

func TestNewTextEditor(t *testing.T) {
	e := NewTextEditor()
	if len(e.Lines) != 1 || e.Lines[0] != "" {
		t.Fatalf("Lines = %v, want one empty line", e.Lines)
	}
	if !e.ShowGutter {
		t.Error("gutter should be enabled by default")
	}
	if e.spans == nil {
		t.Error("spans should be initialised")
	}
}

func TestTextEditorTextSetText(t *testing.T) {
	e := NewTextEditor()
	if e.Text() != "" {
		t.Errorf("empty Text() = %q", e.Text())
	}
	e.SetText("a\nbb\nccc")
	if got := e.Text(); got != "a\nbb\nccc" {
		t.Errorf("round-trip = %q", got)
	}
	if len(e.Lines) != 3 || e.CursorLine != 0 || e.CursorCol != 0 {
		t.Errorf("after SetText: lines=%d cursor=(%d,%d)", len(e.Lines), e.CursorLine, e.CursorCol)
	}
	e.SetText("") // empty -> one empty line
	if len(e.Lines) != 1 || e.Lines[0] != "" {
		t.Errorf("SetText(\"\") = %v", e.Lines)
	}
	// Text() with a nil buffer.
	e.Lines = nil
	if e.Text() != "" {
		t.Error("nil Lines Text() should be empty")
	}
}

func TestTextEditorLeftPad(t *testing.T) {
	e := NewTextEditor()
	e.SetText("x")
	if got := e.leftPad(); got != GutterWidth(1) {
		t.Errorf("gutter leftPad = %d, want %d", got, GutterWidth(1))
	}
	e.ShowGutter = false
	if got := e.leftPad(); got != 1 {
		t.Errorf("no-gutter leftPad = %d, want 1", got)
	}
}

func TestTextEditorInsertAndDelete(t *testing.T) {
	e := NewTextEditor()
	for _, c := range []string{"h", "i"} {
		e.OnEvent(char(c))
	}
	if e.Lines[0] != "hi" || e.CursorCol != 2 {
		t.Fatalf("after typing: %q col=%d", e.Lines[0], e.CursorCol)
	}
	// Backspace mid-line.
	e.OnEvent(key("Backspace"))
	if e.Lines[0] != "h" || e.CursorCol != 1 {
		t.Fatalf("after backspace: %q col=%d", e.Lines[0], e.CursorCol)
	}
	// Enter splits; Backspace at col0 joins.
	e.CursorCol = 1
	e.OnEvent(key("Enter"))
	if len(e.Lines) != 2 || e.CursorLine != 1 {
		t.Fatalf("after Enter: lines=%v cursor=%d", e.Lines, e.CursorLine)
	}
	e.OnEvent(key("Backspace")) // at col0, line1 -> join back
	if len(e.Lines) != 1 || e.Lines[0] != "h" {
		t.Fatalf("after join: %v", e.Lines)
	}
	// Backspace at 0,0 is a no-op.
	e.CursorCol = 0
	e.OnEvent(key("Backspace"))
	if e.Lines[0] != "h" {
		t.Errorf("backspace at 0,0 mutated: %q", e.Lines[0])
	}
	// EventChar with CursorCol past the line length clamps.
	e.CursorCol = 99
	e.OnEvent(char("!"))
	if e.Lines[0] != "h!" {
		t.Errorf("clamped insert = %q", e.Lines[0])
	}
	// Enter with CursorCol past the line length clamps in splitLine.
	e.CursorCol = 99
	e.OnEvent(key("Enter"))
	if len(e.Lines) != 2 || e.Lines[0] != "h!" || e.Lines[1] != "" {
		t.Errorf("clamped Enter split = %v", e.Lines)
	}
}

func TestTextEditorNavigation(t *testing.T) {
	e := NewTextEditor()
	e.SetText("abc\nde\nf")
	e.OnEvent(key("Down")) // ->line1
	e.OnEvent(key("Down")) // ->line2
	e.OnEvent(key("Down")) // at bottom, no-op
	if e.CursorLine != 2 {
		t.Fatalf("Down: line=%d, want 2", e.CursorLine)
	}
	e.CursorCol = 0
	e.OnEvent(key("Right"))
	if e.CursorCol != 1 {
		t.Errorf("Right: col=%d", e.CursorCol)
	}
	e.OnEvent(key("Right")) // "f" len 1, at end -> no-op
	if e.CursorCol != 1 {
		t.Errorf("Right at end: col=%d", e.CursorCol)
	}
	e.OnEvent(key("Left"))
	if e.CursorCol != 0 {
		t.Errorf("Left: col=%d", e.CursorCol)
	}
	e.OnEvent(key("Left")) // at 0 -> no-op
	// Up from line2 (col clamps: line "abc" len 3 keeps col 0).
	e.CursorCol = 0
	e.OnEvent(key("Up"))
	if e.CursorLine != 1 {
		t.Errorf("Up: line=%d", e.CursorLine)
	}
	e.CursorLine = 0
	e.OnEvent(key("Up")) // at top -> no-op
	if e.CursorLine != 0 {
		t.Errorf("Up at top: line=%d", e.CursorLine)
	}
	// Up/Down that clamps a too-far column.
	e.SetText("longline\nx")
	e.CursorLine, e.CursorCol = 0, 8
	e.OnEvent(key("Down")) // to "x" (len 1) -> col clamps to 1
	if e.CursorCol != 1 {
		t.Errorf("Down clamp col=%d, want 1", e.CursorCol)
	}
	e.CursorLine, e.CursorCol = 1, 1
	e.SetText("y\nlongline")
	e.CursorLine, e.CursorCol = 1, 8
	e.OnEvent(key("Up")) // to "y" (len 1) -> col clamps to 1
	if e.CursorCol != 1 {
		t.Errorf("Up clamp col=%d, want 1", e.CursorCol)
	}
}

func TestTextEditorClick(t *testing.T) {
	e := NewTextEditor()
	e.SetText("hello\nworld!")
	pad := e.leftPad() // gutter for 2 lines
	e.OnEvent(click(pad+3, 1))
	if e.CursorLine != 1 || e.CursorCol != 3 {
		t.Fatalf("click: (%d,%d), want (1,3)", e.CursorLine, e.CursorCol)
	}
	// Clamps: X past end, Y past last, negatives.
	e.OnEvent(click(pad+100, 0))
	if e.CursorCol != 5 { // len("hello")
		t.Errorf("X clamp col=%d, want 5", e.CursorCol)
	}
	e.OnEvent(click(pad+1, 999))
	if e.CursorLine != 1 {
		t.Errorf("Y clamp line=%d, want 1", e.CursorLine)
	}
	e.OnEvent(click(-9, -9))
	if e.CursorLine != 0 || e.CursorCol != 0 {
		t.Errorf("neg clamp = (%d,%d)", e.CursorLine, e.CursorCol)
	}
	// Without a gutter the left pad is 1.
	e.ShowGutter = false
	e.OnEvent(click(1+2, 0))
	if e.CursorCol != 2 {
		t.Errorf("no-gutter click col=%d, want 2", e.CursorCol)
	}
}

func TestTextEditorUndoRedo(t *testing.T) {
	e := NewTextEditor()
	e.OnEvent(char("a"))
	e.OnEvent(char("b")) // "ab"
	e.OnEvent(key("Enter"))
	e.OnEvent(char("c")) // "ab\nc"
	if e.Text() != "ab\nc" {
		t.Fatalf("setup = %q", e.Text())
	}
	// Undo the 'c', then the Enter, then 'b', then 'a'.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "ab\n" {
		t.Fatalf("undo c = %q, want %q", e.Text(), "ab\n")
	}
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "ab" {
		t.Fatalf("undo Enter = %q, want %q", e.Text(), "ab")
	}
	// Redo the Enter back.
	e.OnEvent(key("Ctrl+Y"))
	if e.Text() != "ab\n" {
		t.Fatalf("redo Enter = %q, want %q", e.Text(), "ab\n")
	}
	// A fresh edit clears the redo branch.
	e.OnEvent(char("Z"))
	e.OnEvent(key("Ctrl+Y")) // nothing to redo now
	if e.Text() != "ab\nZ" {
		t.Fatalf("redo after new edit should be a no-op: %q", e.Text())
	}
	// Undo restores the caret too.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "ab\n" || e.CursorLine != 1 || e.CursorCol != 0 {
		t.Fatalf("undo cursor = (%d,%d) text %q", e.CursorLine, e.CursorCol, e.Text())
	}
}

func TestTextEditorUndoRedoEmptyAndReadOnly(t *testing.T) {
	e := NewTextEditor()
	// Nothing to undo/redo yet -> no-ops.
	e.OnEvent(key("Ctrl+Z"))
	e.OnEvent(key("Ctrl+Y"))
	if e.Text() != "" {
		t.Fatalf("empty-history undo/redo mutated: %q", e.Text())
	}
	// ReadOnly ignores undo/redo (and never records history).
	e.SetText("abc")
	e.ReadOnly = true
	e.OnEvent(key("Ctrl+Z"))
	e.OnEvent(key("Ctrl+Y"))
	if e.Text() != "abc" {
		t.Fatalf("ReadOnly undo/redo mutated: %q", e.Text())
	}
}

func TestTextEditorUndoHistoryCap(t *testing.T) {
	e := NewTextEditor()
	for i := 0; i < maxUndo+50; i++ {
		e.OnEvent(char("x"))
	}
	if len(e.undo) != maxUndo {
		t.Fatalf("undo stack = %d, want capped at %d", len(e.undo), maxUndo)
	}
	// Undo still works after the cap.
	before := e.Text()
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() == before {
		t.Error("undo after cap did nothing")
	}
}

func TestTextEditorFind(t *testing.T) {
	e := NewTextEditor()
	e.SetText("alpha bravo\ncharlie\nalpha delta")
	// Empty query is a no-op.
	if e.Find("") {
		t.Error("Find(\"\") should return false")
	}
	// First match is at (0,0).
	if !e.Find("alpha") || e.CursorLine != 0 || e.CursorCol != 0 {
		t.Fatalf("Find alpha #1 = (%d,%d)", e.CursorLine, e.CursorCol)
	}
	// FindNext walks to the second "alpha" on line 2.
	if !e.FindNext() || e.CursorLine != 2 || e.CursorCol != 0 {
		t.Fatalf("FindNext = (%d,%d), want (2,0)", e.CursorLine, e.CursorCol)
	}
	// FindNext again wraps back to the first "alpha" (k==len re-check of the
	// start line from column 0).
	if !e.FindNext() || e.CursorLine != 0 || e.CursorCol != 0 {
		t.Fatalf("FindNext wrap = (%d,%d), want (0,0)", e.CursorLine, e.CursorCol)
	}
	// A match later on the current line (bravo at col 6).
	e.CursorLine, e.CursorCol = 0, 0
	if !e.Find("bravo") || e.CursorLine != 0 || e.CursorCol != 6 {
		t.Fatalf("Find bravo = (%d,%d), want (0,6)", e.CursorLine, e.CursorCol)
	}
	// No match leaves the caret where it was and returns false.
	e.CursorLine, e.CursorCol = 1, 0
	if e.Find("zzz") {
		t.Error("Find zzz should return false")
	}
	// FindNext with no prior successful query (fresh editor) is a no-op.
	fresh := NewTextEditor()
	if fresh.FindNext() {
		t.Error("FindNext with no query should return false")
	}
	// Find on an empty buffer returns false (searchForward n==0 guard via
	// a cleared buffer).
	empty := &TextEditor{}
	if empty.Find("x") {
		t.Error("Find on empty buffer should return false")
	}
	// FindNext when the caret sits past the end of its line: the k==0 line is
	// skipped and the search resumes on the next line.
	e.SetText("ab\nfindme")
	e.CursorLine, e.CursorCol = 0, 99 // past EOL of line 0
	if !e.Find("findme") || e.CursorLine != 1 || e.CursorCol != 0 {
		t.Fatalf("Find past-EOL = (%d,%d), want (1,0)", e.CursorLine, e.CursorCol)
	}
}

func TestTextEditorReplace(t *testing.T) {
	e := NewTextEditor()
	e.SetText("foo bar foo\nbaz foo")
	// Replace the first "foo" -> "X"; caret lands just after it.
	if !e.Replace("foo", "X") || e.Text() != "X bar foo\nbaz foo" {
		t.Fatalf("Replace #1 = %q", e.Text())
	}
	if e.CursorLine != 0 || e.CursorCol != 1 {
		t.Errorf("caret after replace = (%d,%d), want (0,1)", e.CursorLine, e.CursorCol)
	}
	// A second Replace walks to the next "foo" (line 0 col 6).
	if !e.Replace("foo", "X") || e.Text() != "X bar X\nbaz foo" {
		t.Fatalf("Replace #2 = %q", e.Text())
	}
	// Undo restores the buffer one replacement at a time.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "X bar foo\nbaz foo" {
		t.Fatalf("undo replace = %q", e.Text())
	}
	// No match / empty query / ReadOnly are no-ops.
	if e.Replace("zzz", "!") {
		t.Error("Replace no-match should be false")
	}
	if e.Replace("", "!") {
		t.Error("Replace empty query should be false")
	}
	e.ReadOnly = true
	if e.Replace("foo", "!") {
		t.Error("ReadOnly Replace should be false")
	}
}

func TestTextEditorReplaceAll(t *testing.T) {
	e := NewTextEditor()
	e.SetText("aa a aa\naaa")
	e.CursorLine, e.CursorCol = 0, 99 // caret past EOL -> clamped after ReplaceAll
	if n := e.ReplaceAll("a", "bb"); n != 8 {
		t.Fatalf("ReplaceAll count = %d, want 8", n)
	}
	if e.Text() != "bbbb bb bbbb\nbbbbbb" {
		t.Fatalf("ReplaceAll text = %q", e.Text())
	}
	if e.CursorCol > len(e.Lines[e.CursorLine]) {
		t.Errorf("caret not clamped: col=%d line-len=%d", e.CursorCol, len(e.Lines[e.CursorLine]))
	}
	// One undo reverts the whole ReplaceAll.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "aa a aa\naaa" {
		t.Fatalf("undo ReplaceAll = %q", e.Text())
	}
	// No match / empty query / ReadOnly return 0.
	if e.ReplaceAll("zzz", "!") != 0 {
		t.Error("ReplaceAll no-match should be 0")
	}
	if e.ReplaceAll("", "!") != 0 {
		t.Error("ReplaceAll empty query should be 0")
	}
	e.ReadOnly = true
	if e.ReplaceAll("a", "!") != 0 {
		t.Error("ReadOnly ReplaceAll should be 0")
	}
}

// drag is a mouse-drag event helper.
func drag(x, y int) toolkit.Event { return toolkit.Event{Kind: toolkit.EventMouseDrag, X: x, Y: y} }

func TestTextEditorSelectionMouse(t *testing.T) {
	e := NewTextEditor()
	e.ShowGutter = false // leftPad = 1
	e.SetText("hello world\nsecond line")
	// Click at col 6 ("world"), then drag to col 11 -> selects "world".
	e.OnEvent(click(1+6, 0)) // leftPad(1)+col6
	if e.selActive {
		t.Fatal("a click should clear the selection")
	}
	e.OnEvent(drag(1+11, 0))
	if got := e.SelectedText(); got != "world" {
		t.Fatalf("SelectedText = %q, want %q", got, "world")
	}
	// A drag back onto the anchor makes the selection empty/inactive.
	e.OnEvent(drag(1+6, 0))
	if e.SelectedText() != "" {
		t.Errorf("collapsed selection should be empty, got %q", e.SelectedText())
	}
	// Multi-line selection: anchor at (0,6), drag to (1,6).
	e.OnEvent(click(1+6, 0))
	e.OnEvent(drag(1+6, 1))
	if got := e.SelectedText(); got != "world\nsecond" {
		t.Fatalf("multi-line SelectedText = %q, want %q", got, "world\nsecond")
	}
	// selRange normalises a backwards selection (anchor after caret).
	e.OnEvent(click(1+6, 1)) // caret on line 1
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	if got := e.SelectedText(); got != "hello world\nsecond" {
		t.Fatalf("backwards SelectedText = %q, want %q", got, "hello world\nsecond")
	}
}

func TestTextEditorSelectionMultiLine(t *testing.T) {
	e := NewTextEditor()
	e.SetText("aaa\nbbb\nccc\nddd")
	// 4-line selection (0,1)-(3,2) exercises SelectedText's middle-line loop.
	e.anchorLine, e.anchorCol, e.selActive = 0, 1, true
	e.CursorLine, e.CursorCol = 3, 2
	if got := e.SelectedText(); got != "aa\nbbb\nccc\ndd" {
		t.Fatalf("multi-line SelectedText = %q", got)
	}
	// Cut exercises deleteRange's multi-line (line-join) branch.
	if e.Cut() != "aa\nbbb\nccc\ndd" || e.Text() != "ad" {
		t.Fatalf("multi-line Cut left %q", e.Text())
	}
	// Same-line backwards selection (anchor col > caret col) normalises.
	e.SetText("hello")
	e.CursorLine, e.CursorCol = 0, 1
	e.anchorLine, e.anchorCol, e.selActive = 0, 4, true
	if got := e.SelectedText(); got != "ell" {
		t.Fatalf("same-line backwards = %q, want ell", got)
	}
}

func TestTextEditorCopyCutPaste(t *testing.T) {
	e := NewTextEditor()
	e.SetText("abcdef")
	e.anchorLine, e.anchorCol, e.selActive = 0, 1, true
	e.CursorCol = 4 // select "bcd"
	if e.Copy() != "bcd" {
		t.Fatalf("Copy = %q", e.Copy())
	}
	// Paste at end (move caret, no selection) inserts the clipboard.
	e.selActive = false
	e.CursorCol = 6
	e.Paste()
	if e.Text() != "abcdefbcd" {
		t.Fatalf("Paste = %q", e.Text())
	}
	// Cut removes the selection + fills the clipboard.
	e.SetText("one two three")
	e.anchorLine, e.anchorCol, e.selActive = 0, 4, true
	e.CursorCol = 7 // "two"
	if e.Cut() != "two" || e.Text() != "one  three" {
		t.Fatalf("Cut left %q", e.Text())
	}
	// Paste replaces an active selection.
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorCol = 3 // select "one"
	e.Paste()       // clipboard is "two"
	if e.Text() != "two  three" {
		t.Fatalf("Paste-over-selection = %q", e.Text())
	}
	// Multi-line paste.
	e.SetText("XY")
	e.CursorCol = 1
	e.clip = "a\nb"
	e.selActive = false
	e.Paste()
	if e.Text() != "Xa\nbY" || e.CursorLine != 1 || e.CursorCol != 1 {
		t.Fatalf("multi-line paste = %q cursor (%d,%d)", e.Text(), e.CursorLine, e.CursorCol)
	}
	// Empty clipboard + ReadOnly are no-ops.
	e.clip = ""
	before := e.Text()
	e.Paste()
	e.ReadOnly = true
	e.clip = "z"
	e.Paste()
	if e.Text() != before {
		t.Errorf("no-op paste mutated: %q", e.Text())
	}
}

func TestTextEditorSelectionEditsAndKeys(t *testing.T) {
	// Typing over a selection replaces it (one undo step).
	e := NewTextEditor()
	e.SetText("hello")
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorCol = 5 // whole word
	e.OnEvent(char("X"))
	if e.Text() != "X" {
		t.Fatalf("type-over-selection = %q", e.Text())
	}
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "hello" {
		t.Fatalf("undo type-over = %q", e.Text())
	}
	// Backspace with a selection deletes the selection (not a char).
	e.anchorLine, e.anchorCol, e.selActive = 0, 1, true
	e.CursorCol = 4 // "ell"
	e.OnEvent(key("Backspace"))
	if e.Text() != "ho" {
		t.Fatalf("backspace-selection = %q", e.Text())
	}
	// Enter with a selection replaces it with a line break.
	e.SetText("abcdef")
	e.anchorLine, e.anchorCol, e.selActive = 0, 2, true
	e.CursorCol = 4 // "cd"
	e.OnEvent(key("Enter"))
	if e.Text() != "ab\nef" {
		t.Fatalf("enter-selection = %q", e.Text())
	}
	// An arrow key clears the selection without deleting.
	e.SetText("abc")
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorCol = 2
	e.OnEvent(key("Right"))
	if e.selActive || e.Text() != "abc" {
		t.Fatalf("arrow should clear selection without editing: active=%v text=%q", e.selActive, e.Text())
	}
	// Ctrl+X / Ctrl+V through OnEvent.
	e.SetText("cut me")
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorCol = 3
	e.OnEvent(key("Ctrl+X")) // cut "cut"
	if e.Text() != " me" {
		t.Fatalf("Ctrl+X = %q", e.Text())
	}
	e.CursorCol = len(e.Lines[0])
	e.OnEvent(key("Ctrl+V")) // paste "cut" at end
	if e.Text() != " mecut" {
		t.Fatalf("Ctrl+V = %q", e.Text())
	}
	// SelectedText with no active selection is empty.
	e.selActive = false
	if e.SelectedText() != "" || e.DeleteSelection() {
		t.Error("no-selection ops should be inert")
	}
}

func TestTextEditorDrawSelection(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	e := NewTextEditor()
	e.SetText("alpha\nbravo\ncharlie")
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 2}) // H < lines -> clipped row
	// Multi-line selection spanning all three lines (line 2 is clipped away).
	e.anchorLine, e.anchorCol, e.selActive = 0, 2, true
	e.CursorLine, e.CursorCol = 2, 3
	e.Draw(mk(40, 2), toolkit.DefaultLight())
	// Single-line selection.
	e.CursorLine, e.CursorCol = 0, 4
	e.Draw(mk(40, 2), toolkit.DefaultLight())
}

func TestTextEditorScroll(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	e := NewTextEditor()
	e.ShowGutter = false
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i)
	}
	e.SetText(strings.Join(lines, "\n"))
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 5}) // 5 visible rows
	e.Focused = true

	// Caret at top -> no scroll.
	e.Draw(mk(40, 5), toolkit.DefaultLight())
	if e.scrollY != 0 {
		t.Fatalf("top scrollY = %d, want 0", e.scrollY)
	}
	// Caret past the bottom -> scroll down so it's the last visible row.
	e.CursorLine = 12
	e.Draw(mk(40, 5), toolkit.DefaultLight())
	if e.scrollY != 8 { // 12 - 5 + 1
		t.Fatalf("scroll-down scrollY = %d, want 8", e.scrollY)
	}
	// Caret above the viewport -> scroll up to it.
	e.CursorLine = 3
	e.Draw(mk(40, 5), toolkit.DefaultLight())
	if e.scrollY != 3 {
		t.Fatalf("scroll-up scrollY = %d, want 3", e.scrollY)
	}
	// A click at screen row 2 maps to buffer line scrollY+2 = 5.
	e.OnEvent(click(1+0, 2))
	if e.CursorLine != 5 {
		t.Fatalf("click with scroll: line=%d, want 5", e.CursorLine)
	}
	// A selection within the scrolled viewport draws (scrolled highlight branch).
	e.anchorLine, e.anchorCol, e.selActive = 4, 0, true
	e.CursorLine, e.CursorCol = 6, 2
	e.Draw(mk(40, 5), toolkit.DefaultLight())
	// Zero-height bounds -> scrollToCaret's h<=0 guard is a no-op.
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 0})
	before := e.scrollY
	e.Draw(mk(40, 1), toolkit.DefaultLight())
	if e.scrollY != before {
		t.Errorf("zero-height Draw changed scrollY %d -> %d", before, e.scrollY)
	}
}

func TestTextEditorNavKeys(t *testing.T) {
	e := NewTextEditor()
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 5})
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line" + strconv.Itoa(i)
	}
	e.SetText(strings.Join(lines, "\n"))
	// Home / End on a line.
	e.CursorLine, e.CursorCol = 0, 3
	e.OnEvent(key("End"))
	if e.CursorCol != len("line0") {
		t.Fatalf("End col = %d, want %d", e.CursorCol, len("line0"))
	}
	e.OnEvent(key("Home"))
	if e.CursorCol != 0 {
		t.Fatalf("Home col = %d, want 0", e.CursorCol)
	}
	// PageDown moves ~a viewport down (page = H-1 = 4); PageUp back.
	e.CursorLine = 0
	e.OnEvent(key("PageDown"))
	if e.CursorLine != 4 {
		t.Fatalf("PageDown line = %d, want 4", e.CursorLine)
	}
	e.OnEvent(key("PageUp"))
	if e.CursorLine != 0 {
		t.Fatalf("PageUp line = %d, want 0", e.CursorLine)
	}
	// PageDown clamps at the last line; PageUp clamps at 0.
	e.CursorLine = 18
	e.OnEvent(key("PageDown"))
	if e.CursorLine != 19 {
		t.Fatalf("PageDown clamp = %d, want 19", e.CursorLine)
	}
	e.CursorLine = 2
	e.OnEvent(key("PageUp"))
	if e.CursorLine != 0 {
		t.Fatalf("PageUp clamp = %d, want 0", e.CursorLine)
	}
	// PageDown clamps the column to the (shorter) destination line.
	e.SetText("longcolumnhere\nx")
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 1}) // page() -> 1
	e.CursorLine, e.CursorCol = 0, 12
	e.OnEvent(key("PageDown"))
	if e.CursorLine != 1 || e.CursorCol != 1 {
		t.Fatalf("PageDown clampCol = (%d,%d), want (1,1)", e.CursorLine, e.CursorCol)
	}
	// A nav key clears an active selection.
	e.anchorLine, e.anchorCol, e.selActive = 1, 0, true
	e.OnEvent(key("Home"))
	if e.selActive {
		t.Error("Home should clear the selection")
	}
}

func TestTextEditorDeleteKey(t *testing.T) {
	e := NewTextEditor()
	e.SetText("abc\ndef")
	// Delete the char after the caret.
	e.CursorLine, e.CursorCol = 0, 1
	e.OnEvent(key("Delete"))
	if e.Text() != "ac\ndef" {
		t.Fatalf("Delete mid-line = %q", e.Text())
	}
	// Delete at end-of-line joins the next line.
	e.CursorCol = 2
	e.OnEvent(key("Delete"))
	if e.Text() != "acdef" {
		t.Fatalf("Delete at EOL = %q", e.Text())
	}
	// Delete at end-of-buffer is a no-op.
	e.CursorLine, e.CursorCol = 0, len(e.Lines[0])
	before := e.Text()
	e.OnEvent(key("Delete"))
	if e.Text() != before {
		t.Errorf("Delete at EOF mutated: %q", e.Text())
	}
	// Delete with a selection removes the selection.
	e.SetText("keep XXX end")
	e.anchorLine, e.anchorCol, e.selActive = 0, 5, true
	e.CursorCol = 8
	e.OnEvent(key("Delete"))
	if e.Text() != "keep  end" {
		t.Fatalf("Delete selection = %q", e.Text())
	}
	// Undo restores it; ReadOnly Delete is a no-op.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "keep XXX end" {
		t.Fatalf("undo Delete = %q", e.Text())
	}
	e.ReadOnly = true
	e.CursorCol = 0
	e.OnEvent(key("Delete"))
	if e.Text() != "keep XXX end" {
		t.Error("ReadOnly Delete mutated")
	}
}

func TestTextEditorIndent(t *testing.T) {
	e := NewTextEditor()
	// Tab with no selection inserts a 4-space indent at the caret.
	e.SetText("foo")
	e.CursorCol = 0
	e.OnEvent(key("Tab"))
	if e.Text() != "    foo" || e.CursorCol != 4 {
		t.Fatalf("Tab insert = %q col=%d", e.Text(), e.CursorCol)
	}
	// Shift+Tab (no selection) dedents the caret line, moving the caret back.
	e.OnEvent(key("Shift+Tab"))
	if e.Text() != "foo" || e.CursorCol != 0 {
		t.Fatalf("Shift+Tab dedent = %q col=%d", e.Text(), e.CursorCol)
	}
	// Shift+Tab on a line with fewer than 4 leading spaces removes only those.
	e.SetText("  bar") // 2 spaces
	e.CursorCol = 3
	e.OnEvent(key("Shift+Tab"))
	if e.Text() != "bar" || e.CursorCol != 1 {
		t.Fatalf("partial dedent = %q col=%d", e.Text(), e.CursorCol)
	}
	// Multi-line selection: Tab indents every selected line + shifts caret/anchor.
	e.SetText("a\nb\nc")
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorLine, e.CursorCol = 2, 1
	e.OnEvent(key("Tab"))
	if e.Text() != "    a\n    b\n    c" {
		t.Fatalf("multi-line indent = %q", e.Text())
	}
	if e.CursorCol != 5 || e.anchorCol != 4 {
		t.Fatalf("indent caret=%d anchor=%d, want 5/4", e.CursorCol, e.anchorCol)
	}
	// Shift+Tab dedents the whole selection back.
	e.OnEvent(key("Shift+Tab"))
	if e.Text() != "a\nb\nc" || e.CursorCol != 1 || e.anchorCol != 0 {
		t.Fatalf("multi-line dedent = %q caret=%d anchor=%d", e.Text(), e.CursorCol, e.anchorCol)
	}
	// A single-line selection + Tab replaces the selection with an indent.
	e.SetText("xyz")
	e.anchorLine, e.anchorCol, e.selActive = 0, 0, true
	e.CursorLine, e.CursorCol = 0, 3
	e.OnEvent(key("Tab"))
	if e.Text() != "    " {
		t.Fatalf("single-line-selection Tab = %q", e.Text())
	}
	// Undo reverts the indent.
	e.OnEvent(key("Ctrl+Z"))
	if e.Text() != "xyz" {
		t.Fatalf("undo indent = %q", e.Text())
	}
	// Dedent with the caret + anchor inside the removed whitespace clamps both
	// columns to 0.
	e.SetText("    a\n    b")
	e.anchorLine, e.anchorCol, e.selActive = 0, 2, true // anchor within line 0's spaces
	e.CursorLine, e.CursorCol = 1, 1                     // caret within line 1's spaces
	e.OnEvent(key("Shift+Tab"))
	if e.Text() != "a\nb" || e.CursorCol != 0 || e.anchorCol != 0 {
		t.Fatalf("dedent col-clamp = %q caret=%d anchor=%d", e.Text(), e.CursorCol, e.anchorCol)
	}
	// ReadOnly Tab/Shift+Tab are no-ops.
	e.SetText("z")
	e.ReadOnly = true
	e.OnEvent(key("Tab"))
	e.OnEvent(key("Shift+Tab"))
	if e.Text() != "z" {
		t.Errorf("ReadOnly indent mutated: %q", e.Text())
	}
}

func TestTextEditorReadOnly(t *testing.T) {
	e := NewTextEditor()
	e.SetText("abc")
	e.ReadOnly = true
	e.CursorCol = 1
	e.OnEvent(char("Z"))
	e.OnEvent(key("Backspace"))
	e.OnEvent(key("Enter"))
	if e.Text() != "abc" || len(e.Lines) != 1 {
		t.Fatalf("ReadOnly mutated the buffer: %q", e.Text())
	}
	// Navigation still works in ReadOnly.
	e.OnEvent(key("Right"))
	if e.CursorCol != 2 {
		t.Errorf("ReadOnly Right col=%d, want 2", e.CursorCol)
	}
	e.OnEvent(click(e.leftPad()+0, 0))
	if e.CursorCol != 0 {
		t.Errorf("ReadOnly click col=%d, want 0", e.CursorCol)
	}
}

func TestTextEditorOnEventEmptyBuffer(t *testing.T) {
	e := &TextEditor{} // no Lines
	e.OnEvent(key("Right"))
	if len(e.Lines) != 1 {
		t.Errorf("empty-buffer OnEvent should seed one line, got %v", e.Lines)
	}
}

func TestTextEditorDraw(t *testing.T) {
	e := NewTextEditor()
	e.Filename = "x.go"
	e.SetText("package main\nfunc f() {}\nvar x = 1")
	e.Focused = true
	e.CursorLine, e.CursorCol = 1, 2
	// Gutter on, H < line count -> exercises gutter + overflow break + caret.
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 2})
	e.Draw(mkPainter(40, 2), toolkit.DefaultLight())
	e.Draw(mkPainter(40, 2), toolkit.DefaultDark())
	// Gutter off.
	e.ShowGutter = false
	e.Draw(mkPainter(40, 2), toolkit.DefaultLight())
	// Caret guards: past visible width, row out of view, not focused.
	e.ShowGutter = true
	e.CursorCol = 100
	e.Draw(mkPainter(40, 2), toolkit.DefaultLight())
	e.CursorLine = 9
	e.Draw(mkPainter(40, 2), toolkit.DefaultLight())
	e.Focused = false
	e.CursorLine = 0
	e.Draw(mkPainter(40, 2), toolkit.DefaultLight())
}
