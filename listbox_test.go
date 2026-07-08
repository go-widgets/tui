// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func lbKey(code string) toolkit.Event  { return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code} }
func lbClick(y int) toolkit.Event       { return toolkit.Event{Kind: toolkit.EventClick, Y: y} }
func lbPainter(w, h int) *painter.PixelPainter {
	return painter.NewPixelPainter(make([]byte, w*h*4), w, h)
}

func TestListBoxNavigation(t *testing.T) {
	l := NewListBox([]string{"a", "b", "c", "d"})
	var fired []int
	l.OnSelect = func(i int) { fired = append(fired, i) }
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 4})

	l.OnEvent(lbKey("Down")) // 0 -> 1
	l.OnEvent(lbKey("Down")) // 1 -> 2
	if l.Selected != 2 {
		t.Fatalf("Down: selected = %d, want 2", l.Selected)
	}
	l.OnEvent(lbKey("Up")) // 2 -> 1
	if l.Selected != 1 {
		t.Fatalf("Up: selected = %d, want 1", l.Selected)
	}
	l.OnEvent(lbKey("End")) // -> last
	if l.Selected != 3 {
		t.Fatalf("End: selected = %d, want 3", l.Selected)
	}
	l.OnEvent(lbKey("Down")) // at the bottom -> no-op, no fire
	if l.Selected != 3 {
		t.Fatalf("Down at bottom: selected = %d, want 3", l.Selected)
	}
	l.OnEvent(lbKey("Home")) // -> 0
	if l.Selected != 0 {
		t.Fatalf("Home: selected = %d, want 0", l.Selected)
	}
	l.OnEvent(lbKey("Up")) // at the top -> no-op
	if l.Selected != 0 {
		t.Fatalf("Up at top: selected = %d, want 0", l.Selected)
	}
	// PageDown / PageUp move ~a viewport (page = H = 4).
	l.OnEvent(lbKey("PageDown")) // 0 -> clamp 3
	if l.Selected != 3 {
		t.Fatalf("PageDown: selected = %d, want 3", l.Selected)
	}
	l.OnEvent(lbKey("PageUp")) // 3 -> clamp 0
	if l.Selected != 0 {
		t.Fatalf("PageUp: selected = %d, want 0", l.Selected)
	}
	// OnSelect fired once per real change (Down,Down,Up,End,Home,PageDown,PageUp).
	if len(fired) != 7 {
		t.Errorf("OnSelect fired %d times, want 7 (%v)", len(fired), fired)
	}
}

func TestListBoxClick(t *testing.T) {
	l := NewListBox([]string{"a", "b", "c", "d", "e", "f"})
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3}) // 3 visible rows
	// Click row 2 selects item 2.
	l.OnEvent(lbClick(2))
	if l.Selected != 2 {
		t.Fatalf("click row 2: selected = %d, want 2", l.Selected)
	}
	// Scroll to the end, then a click maps through the scroll offset.
	l.OnEvent(lbKey("End")) // selected 5, scroll follows on next Draw
	l.Draw(lbPainter(10, 3), toolkit.DefaultLight())
	if l.scrollY != 3 { // 5 - 3 + 1
		t.Fatalf("scroll after End = %d, want 3", l.scrollY)
	}
	l.OnEvent(lbClick(0)) // screen row 0 -> item scrollY+0 = 3
	if l.Selected != 3 {
		t.Fatalf("click through scroll: selected = %d, want 3", l.Selected)
	}
	// Clicks that map outside the item range (accounting for scrollY=3) are
	// ignored: -10 -> item -7, 99 -> item 102.
	l.OnEvent(lbClick(-10))
	l.OnEvent(lbClick(99))
	if l.Selected != 3 {
		t.Errorf("out-of-range click changed selection to %d", l.Selected)
	}
}

func TestListBoxDrawAndScroll(t *testing.T) {
	items := make([]string, 20)
	for i := range items {
		items[i] = "row" + strconv.Itoa(i)
	}
	l := NewListBox(items)
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 4})
	// Draw at the top (selection 0) -> no scroll, selected-row highlight.
	l.Draw(lbPainter(10, 4), toolkit.DefaultLight())
	if l.scrollY != 0 {
		t.Fatalf("top scrollY = %d, want 0", l.scrollY)
	}
	// Select far down -> viewport scrolls; Draw exercises the scrolled loop.
	l.Selected = 15
	l.Draw(lbPainter(10, 4), toolkit.DefaultLight())
	if l.scrollY != 12 { // 15 - 4 + 1
		t.Fatalf("scrolled scrollY = %d, want 12", l.scrollY)
	}
	// Select back up -> scroll up.
	l.Selected = 5
	l.Draw(lbPainter(10, 4), toolkit.DefaultLight())
	if l.scrollY != 5 {
		t.Fatalf("scroll-up scrollY = %d, want 5", l.scrollY)
	}
	// Zero-height bounds: scrollToSel is a no-op; page() falls back to 1.
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 0})
	before := l.scrollY
	l.Draw(lbPainter(10, 1), toolkit.DefaultLight())
	if l.scrollY != before {
		t.Errorf("zero-height Draw changed scrollY")
	}
	l.Selected = 5
	l.OnEvent(lbKey("PageDown")) // page()==1 with H=0 -> selected 6
	if l.Selected != 6 {
		t.Errorf("PageDown with page fallback: selected = %d, want 6", l.Selected)
	}
}

func TestListBoxEmpty(t *testing.T) {
	l := NewListBox(nil)
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	// Every op is inert on an empty list.
	l.OnEvent(lbKey("Down"))
	l.OnEvent(lbKey("End"))
	l.OnEvent(lbClick(0))
	l.Draw(lbPainter(10, 3), toolkit.DefaultLight())
	if l.Selected != 0 {
		t.Errorf("empty list selection = %d, want 0", l.Selected)
	}
}
