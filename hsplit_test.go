// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestHSplitLayout(t *testing.T) {
	l, r := &spyWidget{}, &spyWidget{}
	h := &HSplit{Left: l, Right: r, LeftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 20})
	// lw = 100*30/100 = 30; left [0,30), grip at 30, right [31,100).
	if lb := l.Bounds(); lb.X != 0 || lb.W != 30 {
		t.Errorf("left bounds = %+v, want X0 W30", lb)
	}
	if rb := r.Bounds(); rb.X != 31 || rb.W != 69 {
		t.Errorf("right bounds = %+v, want X31 W69", rb)
	}
	if h.gripLocalX() != 30 {
		t.Errorf("gripLocalX = %d, want 30", h.gripLocalX())
	}
	// Nil panes: SetBounds skips them.
	(&HSplit{LeftFrac: 30}).SetBounds(toolkit.Rect{W: 100, H: 20})
}

func TestGripZone(t *testing.T) {
	cases := []struct{ barH, wantLo, wantHi int }{
		{12, 4, 7},   // 12/6=2 -> floored to 3; y0=(12-3)/2=4 -> [4,7)
		{30, 12, 17}, // 30/6=5 in range, y0=(30-5)/2=12 -> [12,17)
		{60, 26, 33}, // 60/6=10 -> capped 7, y0=(60-7)/2=26 -> [26,33)
		{1, 0, 1},    // 1/6=0 -> 3 -> h>barH -> h=1, y0=0 -> [0,1)
	}
	for _, c := range cases {
		lo, hi := gripZone(c.barH)
		if lo != c.wantLo || hi != c.wantHi {
			t.Errorf("gripZone(%d) = [%d,%d), want [%d,%d)", c.barH, lo, hi, c.wantLo, c.wantHi)
		}
	}
}

func TestHSplitDraw(t *testing.T) {
	l, r := &spyWidget{}, &spyWidget{}
	h := &HSplit{Left: l, Right: r, LeftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 24})
	mk := func() *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, 40*24*4), 40, 24) }
	h.Draw(mk(), toolkit.DefaultLight()) // Border grip
	h.dragging = true
	h.Draw(mk(), toolkit.DefaultLight()) // Accent grip
}

func TestHSplitResizeDrag(t *testing.T) {
	h := &HSplit{Left: &spyWidget{}, Right: &spyWidget{}, LeftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 20})

	// Click the grip -> enter drag mode.
	h.OnEvent(vclick(30, 5))
	if !h.dragging {
		t.Fatal("grip click did not start a drag session")
	}
	// Drag within range updates LeftFrac.
	h.OnEvent(vdrag(50, 5))
	if h.LeftFrac != 50 {
		t.Errorf("drag to X50: LeftFrac=%d, want 50", h.LeftFrac)
	}
	// Drag past the low bound clamps to HSplitMinFrac.
	h.OnEvent(vdrag(5, 5))
	if h.LeftFrac != HSplitMinFrac {
		t.Errorf("drag low: LeftFrac=%d, want %d", h.LeftFrac, HSplitMinFrac)
	}
	// Drag past the high bound clamps to HSplitMaxFrac.
	h.OnEvent(vdrag(95, 5))
	if h.LeftFrac != HSplitMaxFrac {
		t.Errorf("drag high: LeftFrac=%d, want %d", h.LeftFrac, HSplitMaxFrac)
	}
	// MouseUp ends the session.
	h.OnEvent(vup(95, 5))
	if h.dragging {
		t.Error("MouseUp did not end the drag session")
	}

	// A drag with a zero-width split is ignored (no divide by zero).
	z := &HSplit{LeftFrac: 30, dragging: true}
	z.SetBounds(toolkit.Rect{W: 0, H: 10})
	z.OnEvent(vdrag(5, 5)) // w<=0 -> early return
}

func TestHSplitClickRouting(t *testing.T) {
	l, r := &spyWidget{}, &spyWidget{}
	h := &HSplit{Left: l, Right: r, LeftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 20}) // lw=30

	// Click left of the grip -> left pane, coords unchanged.
	h.OnEvent(vclick(10, 4))
	if l.count != 1 || l.last.X != 10 {
		t.Errorf("left click: count=%d X=%d", l.count, l.last.X)
	}
	// Click right of the grip -> right pane, translated by lw+1.
	h.OnEvent(vclick(50, 4))
	if r.count != 1 || r.last.X != 50-31 {
		t.Errorf("right click: count=%d X=%d, want X=19", r.count, r.last.X)
	}
	// A non-mouse event goes to the left pane.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if l.count != 2 {
		t.Errorf("keydown not routed to left: count=%d", l.count)
	}
	// A drag/up with no active session is ignored.
	h.OnEvent(vdrag(50, 4))
	h.OnEvent(vup(50, 4))
	if l.count != 2 || r.count != 1 {
		t.Errorf("stray drag/up leaked: left=%d right=%d", l.count, r.count)
	}

	// Nil-pane guards: clicks and keys on both sides are silent no-ops.
	empty := &HSplit{LeftFrac: 30}
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 20})
	empty.OnEvent(vclick(10, 4)) // nil left
	empty.OnEvent(vclick(50, 4)) // nil right
	empty.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "X"}) // nil left
}
