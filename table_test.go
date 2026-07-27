// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestTableColumnWidths(t *testing.T) {
	// No columns -> nil.
	if w := (&Table{}).columnWidths(20); w != nil {
		t.Errorf("no-column widths = %v, want nil", w)
	}
	// All fixed -> declared widths.
	allFixed := &Table{Columns: []TableColumn{{Width: 5}, {Width: 5}}}
	if w := allFixed.columnWidths(20); w[0] != 5 || w[1] != 5 {
		t.Errorf("all-fixed = %v, want [5 5]", w)
	}
	// Three auto columns: share 6 with the +2 leftover on the last.
	auto := &Table{Columns: []TableColumn{{}, {}, {}}}
	if w := auto.columnWidths(20); w[0] != 6 || w[1] != 6 || w[2] != 8 {
		t.Errorf("auto = %v, want [6 6 8]", w)
	}
	// Fixed wider than total: remaining clamps to 0 and the last auto clamps to 0.
	tight := &Table{Columns: []TableColumn{{Width: 30}, {}}}
	if w := tight.columnWidths(20); w[0] != 30 || w[1] != 0 {
		t.Errorf("tight = %v, want [30 0]", w)
	}
}

func TestTableSelectAndNav(t *testing.T) {
	selected := -1
	rows := [][]string{{"1", "a"}, {"2", "b"}, {"3", "c"}, {"4", "d"}, {"5", "e"}}
	tb := NewTable([]TableColumn{{Title: "N"}, {Title: "L"}}, rows)
	tb.OnSelect = func(r int) { selected = r }
	if tb.Selected != -1 {
		t.Fatalf("NewTable Selected = %d, want -1", tb.Selected)
	}
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3}) // bodyH = 2

	tb.OnEvent(vkey("Down")) // -1 -> 0
	if tb.Selected != 0 || selected != 0 {
		t.Fatalf("Down: sel=%d cb=%d", tb.Selected, selected)
	}
	tb.OnEvent(vkey("End")) // -> last
	if tb.Selected != 4 {
		t.Fatalf("End: %d", tb.Selected)
	}
	tb.OnEvent(vkey("Home")) // -> 0
	if tb.Selected != 0 {
		t.Fatalf("Home: %d", tb.Selected)
	}
	tb.OnEvent(vkey("PageDown")) // +2
	if tb.Selected != 2 {
		t.Fatalf("PageDown: %d", tb.Selected)
	}
	tb.OnEvent(vkey("PageUp")) // -2
	if tb.Selected != 0 {
		t.Fatalf("PageUp: %d", tb.Selected)
	}
	tb.OnEvent(vkey("Up")) // clamp at 0, no change (no callback re-fire)
	if tb.Selected != 0 {
		t.Fatalf("Up clamp: %d", tb.Selected)
	}
	// Overshoot: PageDown from row 3 lands past the end and clamps to the last.
	tb.Selected = 3
	tb.OnEvent(vkey("PageDown")) // 3 + 2 = 5 -> clamp 4
	if tb.Selected != 4 {
		t.Fatalf("PageDown overshoot: %d, want 4", tb.Selected)
	}
	tb.Selected = 0

	// Click a body row (Y=2 -> row scrollY+1). scrollY is 0 here.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 2})
	if tb.Selected != 1 {
		t.Fatalf("body click: %d, want 1", tb.Selected)
	}
	// Click the header row is inert.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	if tb.Selected != 1 {
		t.Errorf("header click changed selection: %d", tb.Selected)
	}
	// Click past the last body row is a no-op.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 99})
	if tb.Selected != 1 {
		t.Errorf("out-of-range click: %d", tb.Selected)
	}

	// Navigation on an empty table is safe (setSelected early-returns), and
	// page() floors at 1 when there is no body height.
	empty := NewTable([]TableColumn{{Title: "X"}}, nil)
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1}) // bodyH = 0
	empty.OnEvent(vkey("PageDown"))
	if empty.Selected != -1 {
		t.Errorf("empty nav changed selection: %d", empty.Selected)
	}
	// A selection change with no OnSelect handler must not panic.
	NewTable([]TableColumn{{Title: "X"}}, [][]string{{"a"}, {"b"}}).OnEvent(vkey("Down"))
}

// TestTableMultiSelect mirrors ListBox's MultiSelect Ctrl/Shift click model
// (see toolkit.ListBox) and asserts the resulting highlight at cell
// precision.
func TestTableMultiSelect(t *testing.T) {
	rows := [][]string{{"1"}, {"2"}, {"3"}, {"4"}, {"5"}}
	tb := NewTable([]TableColumn{{Title: "N"}}, rows)
	tb.MultiSelect = true
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 6}) // bodyH = 5, no scroll
	var fired []int
	tb.OnSelect = func(r int) { fired = append(fired, r) }

	click := func(row int, ctrl, shift bool) toolkit.Event {
		return toolkit.Event{Kind: toolkit.EventClick, Y: row + 1, Ctrl: ctrl, Shift: shift}
	}

	// Plain click selects only that row + moves the anchor.
	tb.OnEvent(click(1, false, false))
	if tb.Selected != 1 || !tb.IsSelected(1) || tb.IsSelected(0) {
		t.Fatalf("plain click: Selected=%d IsSelected(1)=%v IsSelected(0)=%v", tb.Selected, tb.IsSelected(1), tb.IsSelected(0))
	}

	// Ctrl-click adds row 3 + moves the anchor without clearing row 1.
	tb.OnEvent(click(3, true, false))
	if tb.Selected != 3 || !tb.IsSelected(1) || !tb.IsSelected(3) {
		t.Fatalf("ctrl-click add: Selected=%d IsSelected(1)=%v IsSelected(3)=%v", tb.Selected, tb.IsSelected(1), tb.IsSelected(3))
	}
	// Second Ctrl-click on the same row removes it.
	tb.OnEvent(click(3, true, false))
	if tb.IsSelected(3) {
		t.Fatalf("ctrl-click remove: row 3 still selected")
	}

	// Shift-click selects the inclusive range from the anchor (3) to the
	// clicked row (1), replacing the set + leaving the anchor unchanged.
	tb.OnEvent(click(1, false, true))
	for i, want := range map[int]bool{0: false, 1: true, 2: true, 3: true, 4: false} {
		if got := tb.IsSelected(i); got != want {
			t.Errorf("after shift-click range(1,3): IsSelected(%d) = %v, want %v", i, got, want)
		}
	}
	if tb.Selected != 3 {
		t.Errorf("shift-click should not move the anchor: Selected = %d, want 3", tb.Selected)
	}
	if len(fired) != 4 {
		t.Errorf("OnSelect fired %d times, want 4 (%v)", len(fired), fired)
	}

	// Draw at cell precision: rows 1-3 are IsSelected -> Accent background;
	// row 0 and row 4 are not.
	theme := toolkit.DefaultLight()
	cp := painter.NewCellPainter(10, 6)
	tb.Draw(cp, theme)
	bg := func(y int) painter.RGBA { return cp.Cells[y*cp.W].Bg }
	for i, want := range map[int]bool{0: false, 1: true, 2: true, 3: true, 4: false} {
		got := bg(i+1) == theme.Accent
		if got != want {
			t.Errorf("row %d bg==Accent = %v, want %v", i, got, want)
		}
	}

	// A click outside the row range is a no-op even in MultiSelect mode.
	before := tb.SelectedIndices()
	tb.OnEvent(click(99, false, false))
	if got := tb.SelectedIndices(); len(got) != len(before) {
		t.Errorf("out-of-range click in MultiSelect mode changed selection: %v", got)
	}

	// With MultiSelect false (the default), Ctrl/Shift are ignored entirely:
	// a plain setSelected happens + the selection set is never touched.
	single := NewTable([]TableColumn{{Title: "N"}}, rows)
	single.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 6})
	single.OnEvent(click(2, true, false))
	if single.Selected != 2 || single.IsSelected(2) {
		t.Fatalf("ctrl-click with MultiSelect=false: Selected=%d IsSelected(2)=%v", single.Selected, single.IsSelected(2))
	}
}

func TestTableSelectionAPI(t *testing.T) {
	rows := [][]string{{"1"}, {"2"}, {"3"}, {"4"}}
	tb := NewTable([]TableColumn{{Title: "N"}}, rows)

	tb.SetSelection(-1, 1, 2, 99)
	if got := tb.SelectedIndices(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("SetSelection = %v, want [1 2]", got)
	}

	tb.ClearSelection()
	tb.ToggleSelect(0)
	if !tb.IsSelected(0) {
		t.Fatalf("ToggleSelect on nil set: row 0 not selected")
	}
	tb.ToggleSelect(0)
	if tb.IsSelected(0) {
		t.Fatalf("ToggleSelect twice: row 0 still selected")
	}
	tb.ToggleSelect(-1)
	tb.ToggleSelect(99)
	if len(tb.SelectedIndices()) != 0 {
		t.Fatalf("out-of-range ToggleSelect mutated selection: %v", tb.SelectedIndices())
	}

	tb.SelectRange(2, 0)
	if got := tb.SelectedIndices(); len(got) != 3 || got[0] != 0 || got[2] != 2 {
		t.Fatalf("SelectRange(2,0) = %v, want [0 1 2]", got)
	}
	tb.SelectRange(-5, 99)
	if got := tb.SelectedIndices(); len(got) != len(rows) {
		t.Fatalf("SelectRange clamped = %v, want all rows", got)
	}

	tb.Selected = 2
	tb.ClearSelection()
	if len(tb.SelectedIndices()) != 0 || tb.Selected != 2 {
		t.Fatalf("ClearSelection: indices=%v Selected=%d, want []/2", tb.SelectedIndices(), tb.Selected)
	}

	empty := NewTable([]TableColumn{{Title: "N"}}, nil)
	empty.SelectRange(0, 0)
	if len(empty.SelectedIndices()) != 0 {
		t.Fatalf("SelectRange on empty table = %v, want []", empty.SelectedIndices())
	}
}

// TestCellTextX exercises every Align branch of cellTextX directly, including
// the "text too wide for the cell" clamp shared by AlignRight/AlignCenter.
func TestCellTextX(t *testing.T) {
	if x := cellTextX(100, 20, "hi", AlignLeft); x != 101 {
		t.Errorf("AlignLeft = %d, want 101", x)
	}
	if x := cellTextX(100, 20, "hi", AlignRight); x != 117 { // 100+20-1-2
		t.Errorf("AlignRight = %d, want 117", x)
	}
	if x := cellTextX(100, 20, "hi", AlignCenter); x != 109 { // 100+(20-2)/2
		t.Errorf("AlignCenter = %d, want 109", x)
	}
	// Text wider than the cell clamps to the left pad (cellX+1) for both
	// AlignRight and AlignCenter.
	if x := cellTextX(100, 1, "wide text", AlignRight); x != 101 {
		t.Errorf("AlignRight clamp = %d, want 101", x)
	}
	if x := cellTextX(100, 1, "wide text", AlignCenter); x != 101 {
		t.Errorf("AlignCenter clamp = %d, want 101", x)
	}
}

// TestTableColumnAlign draws a 3-column table (left/right/center) at
// non-zero bounds and asserts, at cell precision, that both the header
// titles and the body cells honor each column's Align.
func TestTableColumnAlign(t *testing.T) {
	theme := toolkit.DefaultLight()
	cols := []TableColumn{
		{Title: "Name", Width: 8, Align: AlignLeft},
		{Title: "Qty", Width: 6, Align: AlignRight},
		{Title: "OK", Width: 5, Align: AlignCenter},
	}
	rows := [][]string{{"Alice", "42", "OK"}}
	tb := NewTable(cols, rows)
	// Select row 0 so this test also covers the Accent-highlight ink branch
	// alongside the alignment geometry. (The no-selection default is covered
	// separately by TestTableDrawNoSelectionDoesNotPanic.)
	tb.Selected = 0
	tb.SetBounds(toolkit.Rect{X: 1, Y: 2, W: 19, H: 3})

	cp := painter.NewCellPainter(21, 6)
	tb.Draw(cp, theme)
	at := func(x, y int) painter.Cell { return cp.Cells[y*cp.W+x] }

	// Header row (Y=2): col0 [1,9) left-pads to 2; col1 [9,15) right-aligns
	// "Qty" (n=3) to 9+6-1-3=11; col2 [15,20) centers "OK" (n=2) to
	// 15+(5-2)/2=16.
	if got := at(2, 2).Rune; got != 'N' {
		t.Errorf("header col0 (left) start = %q, want 'N'", got)
	}
	if got := at(11, 2).Rune; got != 'Q' {
		t.Errorf("header col1 (right) start = %q, want 'Q'", got)
	}
	if got := at(16, 2).Rune; got != 'O' {
		t.Errorf("header col2 (center) start = %q, want 'O'", got)
	}

	// Body row (Y=3): same per-column alignment applies to cell text. "42"
	// (n=2) right-aligns to 9+6-1-2=12; "OK" (n=2) centers to 16 (same as
	// the header, same width/text length).
	if got := at(2, 3).Rune; got != 'A' {
		t.Errorf("body col0 (left) start = %q, want 'A'", got)
	}
	if got := at(12, 3).Rune; got != '4' {
		t.Errorf("body col1 (right) start = %q, want '4'", got)
	}
	if got := at(16, 3).Rune; got != 'O' {
		t.Errorf("body col2 (center) start = %q, want 'O'", got)
	}

	// Row 0 is Selected → Accent-highlighted; the per-column alignment still
	// applies alongside that ink branch.
	if got := at(2, 3).Bg; got != theme.Accent {
		t.Errorf("selected row bg = %+v, want theme.Accent %+v", got, theme.Accent)
	}
}

// A Table built by NewTable (Selected == -1) with non-empty Rows must Draw
// without panicking: scrollToSel used to follow Selected to -1, and the body
// loop then indexed Rows[-1]. This is the default configuration of any freshly
// constructed Table, so it must render the rows top-aligned with no highlight.
func TestTableDrawNoSelectionDoesNotPanic(t *testing.T) {
	theme := toolkit.DefaultLight()
	cols := []TableColumn{{Title: "A", Width: 6}}
	rows := [][]string{{"r0"}, {"r1"}, {"r2"}}
	tb := NewTable(cols, rows) // Selected stays -1
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 6})

	cp := painter.NewCellPainter(10, 6)
	tb.Draw(cp, theme) // must not panic
	at := func(x, y int) painter.Cell { return cp.Cells[y*cp.W+x] }

	// scrollY stayed at 0, so row 0 renders on the first body line (Y=1).
	if tb.scrollY != 0 {
		t.Fatalf("scrollY = %d, want 0 (no selection must not move the viewport)", tb.scrollY)
	}
	// Left-aligned column pads by 1 cell, so "r0" starts at x=1 on the first
	// body line.
	if got := at(1, 1).Rune; got != 'r' {
		t.Errorf("body row 0 text start = %q, want 'r'", got)
	}
	// No row is highlighted: the first body row keeps the plain SurfaceAlt fill
	// (row 0 is even, so not even the zebra Surface stripe), never Accent.
	if got := at(0, 1).Bg; got == theme.Accent {
		t.Errorf("body row 0 bg = %+v, must not be Accent with no selection", got)
	}
}

func TestTableDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Full grid: header + selected row + zebra + a short row (missing cell) +
	// two columns (one separator).
	rows := [][]string{{"1", "a"}, {"2", "b"}, {"3"}, {"4", "d"}}
	tb := NewTable([]TableColumn{{Title: "N"}, {Title: "L"}}, rows)
	tb.Selected = 0
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})
	tb.Draw(mk(20, 6), theme)

	// Five rows in a tiny body: the row loop breaks past the viewport, and the
	// selection scrolls both down (else-if) and back up (if).
	big := NewTable([]TableColumn{{Title: "N"}, {Title: "L"}},
		[][]string{{"1", "a"}, {"2", "b"}, {"3", "c"}, {"4", "d"}, {"5", "e"}})
	big.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3}) // bodyH = 2
	big.Selected = 0
	big.Draw(mk(20, 3), theme) // no scroll, loop breaks at the 3rd visible row
	big.Selected = 4
	big.Draw(mk(20, 3), theme) // scroll down
	if big.scrollY != 3 {
		t.Errorf("scroll down: scrollY=%d, want 3", big.scrollY)
	}
	big.Selected = 0
	big.Draw(mk(20, 3), theme) // scroll back up
	if big.scrollY != 0 {
		t.Errorf("scroll up: scrollY=%d, want 0", big.scrollY)
	}

	// Rows present but no body height (H=1): scrollToSel returns early.
	noBody := NewTable([]TableColumn{{Title: "N"}}, [][]string{{"a"}, {"b"}})
	noBody.Selected = 1
	noBody.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	noBody.Draw(mk(10, 1), theme)

	// Empty table -> "(no data)" placeholder.
	empty := NewTable([]TableColumn{{Title: "X"}}, nil)
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 4})
	empty.Draw(mk(20, 4), theme)

	// Zero-size bounds -> Draw returns early.
	z := NewTable([]TableColumn{{Title: "X"}}, [][]string{{"a"}})
	z.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 0, H: 0})
	z.Draw(mk(1, 1), theme)
}
