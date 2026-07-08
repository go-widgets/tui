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
		{Separator: true},                                   // [5,6)
		{Label: "Save", OnClick: func() { clicked = "Save" }}, // [6,12)
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
