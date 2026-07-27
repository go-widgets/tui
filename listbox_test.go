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

func TestListBoxMultiSelectDisabledIgnoresModifiers(t *testing.T) {
	l := NewListBox([]string{"a", "b", "c"})
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	// MultiSelect is false (the default): Ctrl/Shift on a click are ignored
	// entirely -- a plain setSelected happens exactly as before this
	// feature existed. IsSelected must stay false for every row: the
	// selection set is never touched.
	l.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1, Ctrl: true})
	if l.Selected != 1 {
		t.Fatalf("Ctrl-click with MultiSelect=false: selected = %d, want 1", l.Selected)
	}
	l.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 2, Shift: true})
	if l.Selected != 2 {
		t.Fatalf("Shift-click with MultiSelect=false: selected = %d, want 2", l.Selected)
	}
	for i := range l.Items {
		if l.IsSelected(i) {
			t.Errorf("row %d reported selected with MultiSelect=false", i)
		}
	}
	l.Draw(lbPainter(10, 3), toolkit.DefaultLight()) // exercise the non-multi highlight branch
}

func TestListBoxMultiSelectClick(t *testing.T) {
	l := NewListBox([]string{"a", "b", "c", "d", "e"})
	l.MultiSelect = true
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 5})
	var fired []int
	l.OnSelect = func(i int) { fired = append(fired, i) }

	// A plain click selects only that row + moves the anchor.
	l.OnEvent(lbClick(1))
	if l.Selected != 1 || !l.IsSelected(1) || l.IsSelected(0) {
		t.Fatalf("plain click: Selected=%d IsSelected(1)=%v IsSelected(0)=%v", l.Selected, l.IsSelected(1), l.IsSelected(0))
	}

	// A Ctrl-click toggles membership + moves the anchor without clearing
	// the existing selection.
	l.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 3, Ctrl: true})
	if l.Selected != 3 || !l.IsSelected(1) || !l.IsSelected(3) {
		t.Fatalf("ctrl-click add: Selected=%d IsSelected(1)=%v IsSelected(3)=%v", l.Selected, l.IsSelected(1), l.IsSelected(3))
	}
	// A second Ctrl-click on the same row removes it.
	l.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 3, Ctrl: true})
	if l.IsSelected(3) {
		t.Fatalf("ctrl-click remove: row 3 still selected")
	}
	if !l.IsSelected(1) {
		t.Fatalf("ctrl-click remove: row 1 should remain selected")
	}

	// A Shift-click selects the inclusive range from the anchor (still 3
	// from the last Ctrl-click) to the clicked row, replacing the set, and
	// leaves the anchor unchanged.
	l.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1, Shift: true})
	for i, want := range map[int]bool{0: false, 1: true, 2: true, 3: true, 4: false} {
		if got := l.IsSelected(i); got != want {
			t.Errorf("after shift-click range(1,3): IsSelected(%d) = %v, want %v", i, got, want)
		}
	}
	if l.Selected != 3 {
		t.Errorf("shift-click should not move the anchor: Selected = %d, want 3", l.Selected)
	}

	// OnSelect fires once per click regardless of modifier.
	if len(fired) != 4 {
		t.Errorf("OnSelect fired %d times, want 4 (%v)", len(fired), fired)
	}

	// Draw exercises the MultiSelect highlight branch (IsSelected instead
	// of Selected).
	l.Draw(lbPainter(10, 5), toolkit.DefaultLight())

	// A click outside the item range is a no-op, even in MultiSelect mode.
	before := l.SelectedIndices()
	l.OnEvent(lbClick(99))
	if got := l.SelectedIndices(); len(got) != len(before) {
		t.Errorf("out-of-range click in MultiSelect mode changed selection: %v", got)
	}
}

func TestListBoxSelectionAPI(t *testing.T) {
	l := NewListBox([]string{"a", "b", "c", "d"})

	// SetSelection drops out-of-range indices + replaces the set.
	l.SetSelection(-1, 1, 2, 99)
	if got := l.SelectedIndices(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("SetSelection = %v, want [1 2]", got)
	}

	// ToggleSelect flips membership, including on a fresh (nil) set.
	l.ClearSelection()
	l.ToggleSelect(0)
	if !l.IsSelected(0) {
		t.Fatalf("ToggleSelect on nil set: row 0 not selected")
	}
	l.ToggleSelect(0)
	if l.IsSelected(0) {
		t.Fatalf("ToggleSelect twice: row 0 still selected")
	}
	// Out-of-range ToggleSelect is a no-op.
	l.ToggleSelect(-1)
	l.ToggleSelect(99)
	if len(l.SelectedIndices()) != 0 {
		t.Fatalf("out-of-range ToggleSelect mutated selection: %v", l.SelectedIndices())
	}

	// SelectRange accepts either order + clamps to [0, len(Items)).
	l.SelectRange(2, 0)
	if got := l.SelectedIndices(); len(got) != 3 || got[0] != 0 || got[2] != 2 {
		t.Fatalf("SelectRange(2,0) = %v, want [0 1 2]", got)
	}
	l.SelectRange(-5, 99)
	if got := l.SelectedIndices(); len(got) != len(l.Items) {
		t.Fatalf("SelectRange clamped = %v, want all 4 rows", got)
	}

	// ClearSelection empties the set without touching Selected.
	l.Selected = 2
	l.ClearSelection()
	if len(l.SelectedIndices()) != 0 || l.Selected != 2 {
		t.Fatalf("ClearSelection: indices=%v Selected=%d, want []/2", l.SelectedIndices(), l.Selected)
	}

	// SelectRange on an empty list yields an empty selection.
	empty := NewListBox(nil)
	empty.SelectRange(0, 0)
	if len(empty.SelectedIndices()) != 0 {
		t.Fatalf("SelectRange on empty list = %v, want []", empty.SelectedIndices())
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
