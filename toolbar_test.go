// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestToolbarRangesAndClick(t *testing.T) {
	clicked := ""
	tb := NewToolbar([]ToolbarItem{
		{Label: "New", OnClick: func() { clicked = "New" }}, // [0,5)
		{Separator: true}, // [5,6)
		{Label: "Save", OnClick: func() { clicked = "Save" }},               // [6,12)
		{Label: "Del", Disabled: true, OnClick: func() { clicked = "Del" }}, // [12,17)
		{Label: "NoOp"}, // nil OnClick [17,23)
	})
	xs := tb.itemRanges()
	if len(xs) != 6 || xs[1] != 5 || xs[2] != 6 || xs[3] != 12 || xs[4] != 17 || xs[5] != 23 {
		t.Fatalf("itemRanges = %v", xs)
	}
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 1})

	// Click an enabled button runs its handler.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 0})
	if clicked != "New" {
		t.Fatalf("New click: %q", clicked)
	}
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 8, Y: 0})
	if clicked != "Save" {
		t.Fatalf("Save click: %q", clicked)
	}
	// Click the separator: no-op.
	clicked = ""
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 0})
	// Click the disabled button: no-op.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 13, Y: 0})
	// Click the nil-handler button: no-op.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 18, Y: 0})
	if clicked != "" {
		t.Errorf("inert click ran: %q", clicked)
	}
	// Click past the last item: no match.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 99, Y: 0})
	// Non-click ignored.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
}

// TestToolbarVerticalRangesAndClick exercises the Orientation=Vertical
// itemRanges (one row per item, including separators) and OnEvent's
// along-the-Y-axis hit-testing.
func TestToolbarVerticalRangesAndClick(t *testing.T) {
	clicked := ""
	tb := NewToolbar([]ToolbarItem{
		{Label: "New", OnClick: func() { clicked = "New" }}, // row 0
		{Separator: true}, // row 1
		{Label: "Save", OnClick: func() { clicked = "Save" }},               // row 2
		{Label: "Del", Disabled: true, OnClick: func() { clicked = "Del" }}, // row 3
		{Label: "NoOp"}, // nil OnClick, row 4
	})
	tb.Orientation = toolkit.Vertical

	xs := tb.itemRanges()
	if len(xs) != 6 {
		t.Fatalf("vertical itemRanges len = %d, want 6", len(xs))
	}
	for i, want := range []int{0, 1, 2, 3, 4, 5} {
		if xs[i] != want {
			t.Errorf("vertical itemRanges[%d] = %d, want %d", i, xs[i], want)
		}
	}

	tb.SetBounds(toolkit.Rect{X: 2, Y: 3, W: 8, H: 5})

	// Click item 0 ("New", local Y=0) runs its handler.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 0})
	if clicked != "New" {
		t.Fatalf("vertical New click: %q", clicked)
	}
	// Click item 2 ("Save", local Y=2) runs its handler.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 2})
	if clicked != "Save" {
		t.Fatalf("vertical Save click: %q", clicked)
	}
	// Click the separator row (local Y=1): no-op.
	clicked = ""
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 1})
	// Click the disabled row (local Y=3): no-op.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 3})
	// Click the nil-handler row (local Y=4): no-op.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 4})
	if clicked != "" {
		t.Errorf("vertical inert click ran: %q", clicked)
	}
	// Click past the last row: no match.
	tb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 99})
}

// TestToolbarVerticalDraw asserts, at cell precision and non-zero bounds,
// that a vertical Toolbar stacks its buttons one per row and draws a
// separator as a full-width horizontal rule instead of a "│" bar.
func TestToolbarVerticalDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	tb := NewToolbar([]ToolbarItem{
		{Label: "New"},                 // row 0
		{Separator: true},              // row 1
		{Label: "Save"},                // row 2
		{Label: "Del", Disabled: true}, // row 3
	})
	tb.Orientation = toolkit.Vertical
	tb.SetBounds(toolkit.Rect{X: 2, Y: 3, W: 8, H: 4})

	cp := painter.NewCellPainter(12, 10)
	tb.Draw(cp, theme)
	at := func(x, y int) painter.Cell { return cp.Cells[y*cp.W+x] }

	// Row 0 ("New"): label starts 1 cell in from the left edge of Bounds.
	if got := at(3, 3).Rune; got != 'N' {
		t.Errorf("row0 label start = %q, want 'N'", got)
	}
	// Row 1 (separator): a FULL-WIDTH horizontal rule, not a single "│".
	if got := at(2, 4).Rune; got != '─' {
		t.Errorf("separator row start = %q, want '─'", got)
	}
	if got := at(9, 4).Rune; got != '─' { // r.X + r.W - 1 = 2+8-1
		t.Errorf("separator row end = %q, want '─'", got)
	}
	// Row 2 ("Save").
	if got := at(3, 5).Rune; got != 'S' {
		t.Errorf("row2 label start = %q, want 'S'", got)
	}
	// Row 3 ("Del", disabled) uses the muted LineNumberInk, not OnSurface.
	delCell := at(3, 6)
	if delCell.Rune != 'D' {
		t.Errorf("row3 label start = %q, want 'D'", delCell.Rune)
	}
	if delCell.Fg != LineNumberInk(theme) {
		t.Errorf("disabled row ink = %+v, want LineNumberInk %+v", delCell.Fg, LineNumberInk(theme))
	}
}

func TestToolbarDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	tb := NewToolbar([]ToolbarItem{
		{Label: "Cut"},
		{Separator: true},
		{Label: "Paste", Disabled: true},
	})
	tb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	tb.Draw(mk(20, 1), toolkit.DefaultLight())
}
