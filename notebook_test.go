// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestNotebookTabsAndRanges(t *testing.T) {
	n := NewNotebook()
	p1, p2 := &spyWidget{}, &spyWidget{}
	n.AddTab("One", p1)  // width 3+2 = 5 -> [0,5)
	n.AddTab("Two2", p2) // width 4+2 = 6 -> [5,11)
	xs := n.tabRanges()
	if len(xs) != 3 || xs[0] != 0 || xs[1] != 5 || xs[2] != 11 {
		t.Fatalf("tabRanges = %v, want [0 5 11]", xs)
	}
}

func TestNotebookTabSelect(t *testing.T) {
	changed := -1
	n := NewNotebook()
	n.AddTab("One", &spyWidget{})
	n.AddTab("Two", &spyWidget{})
	n.OnTabChanged = func(i int) { changed = i }
	n.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})

	// Click the second tab (starts at column 5) -> Active = 1.
	n.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 0})
	if n.Active != 1 || changed != 1 {
		t.Fatalf("tab click: active=%d changed=%d, want 1/1", n.Active, changed)
	}
	// Click the first tab.
	n.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 0})
	if n.Active != 0 || changed != 0 {
		t.Fatalf("tab0 click: active=%d changed=%d", n.Active, changed)
	}
	// A strip click past the last tab selects nothing.
	n.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 99, Y: 0})
	if n.Active != 0 {
		t.Errorf("out-of-strip click changed active: %d", n.Active)
	}
	// Tab select with no OnTabChanged handler must not panic.
	bare := NewNotebook()
	bare.AddTab("x", &spyWidget{})
	bare.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 0})
}

func TestNotebookBodyRouting(t *testing.T) {
	page := &spyWidget{}
	n := NewNotebook()
	n.AddTab("Body", page)
	n.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})

	// A body click routes to the active page, translated below the strip.
	n.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 4, Y: 3})
	if page.count != 1 || page.last.Y != 2 {
		t.Fatalf("body click: count=%d Y=%d, want 1/2", page.count, page.last.Y)
	}
	// A non-click event also routes to the page.
	n.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if page.count != 2 {
		t.Errorf("keydown not routed: count=%d", page.count)
	}

	// Active out of range -> no routing (no panic).
	n.Active = 5
	n.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "X"})

	// A nil page is skipped.
	nilp := NewNotebook()
	nilp.AddTab("Nil", nil)
	nilp.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "X"})
}

func TestNotebookDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Two tabs (active + inactive) + a real page that gets drawn.
	page := &spyWidget{}
	n := NewNotebook()
	n.AddTab("A", page)
	n.AddTab("B", &spyWidget{})
	n.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})
	n.Draw(mk(20, 6), theme)

	// Nil active page -> body draw skipped.
	nilp := NewNotebook()
	nilp.AddTab("Nil", nil)
	nilp.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})
	nilp.Draw(mk(20, 6), theme)

	// Empty notebook (Active 0 but no tabs) -> body branch skipped.
	empty := NewNotebook()
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 6})
	empty.Draw(mk(20, 6), theme)
}
