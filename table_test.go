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
	// NewTable defaults Selected to -1 (no row highlighted); Draw must be
	// called with a valid (>= 0) Selected, matching every other Table Draw
	// test in this file — Selected=-1 with non-empty Rows hits a pre-existing
	// scrollToSel bug unrelated to Align (out of scope for this change).
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
