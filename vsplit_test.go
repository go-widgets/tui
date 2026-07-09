// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestVSplitLayout(t *testing.T) {
	top, bot := &spyWidget{}, &spyWidget{}
	v := &VSplit{Top: top, Bottom: bot, TopFrac: 30}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	// th = 20*30/100 = 6; top [0,6), grip at 6, bottom [7,20).
	if tb := top.Bounds(); tb.Y != 0 || tb.H != 6 {
		t.Errorf("top bounds = %+v, want Y0 H6", tb)
	}
	if bb := bot.Bounds(); bb.Y != 7 || bb.H != 13 {
		t.Errorf("bottom bounds = %+v, want Y7 H13", bb)
	}
	if v.gripLocalY() != 6 {
		t.Errorf("gripLocalY = %d, want 6", v.gripLocalY())
	}
	// Nil panes: SetBounds skips them.
	(&VSplit{TopFrac: 30}).SetBounds(toolkit.Rect{W: 20, H: 20})
}

func TestVSplitDraw(t *testing.T) {
	top, bot := &spyWidget{}, &spyWidget{}
	v := &VSplit{Top: top, Bottom: bot, TopFrac: 30}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 24, H: 40})
	mk := func() *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, 24*40*4), 24, 40) }
	v.Draw(mk(), toolkit.DefaultLight()) // Border grip
	v.dragging = true
	v.Draw(mk(), toolkit.DefaultLight()) // Accent grip
}

func TestVSplitResizeDrag(t *testing.T) {
	v := &VSplit{Top: &spyWidget{}, Bottom: &spyWidget{}, TopFrac: 30}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 100})

	// Click the grip -> enter drag mode.
	v.OnEvent(vclick(5, 30))
	if !v.dragging {
		t.Fatal("grip click did not start a drag session")
	}
	// Drag within range sets TopFrac.
	v.OnEvent(vdrag(5, 50))
	if v.TopFrac != 50 {
		t.Errorf("drag to Y50: TopFrac=%d, want 50", v.TopFrac)
	}
	// Clamp low / high.
	v.OnEvent(vdrag(5, 5))
	if v.TopFrac != VSplitMinFrac {
		t.Errorf("drag low: TopFrac=%d, want %d", v.TopFrac, VSplitMinFrac)
	}
	v.OnEvent(vdrag(5, 95))
	if v.TopFrac != VSplitMaxFrac {
		t.Errorf("drag high: TopFrac=%d, want %d", v.TopFrac, VSplitMaxFrac)
	}
	// MouseUp ends the session.
	v.OnEvent(vup(5, 95))
	if v.dragging {
		t.Error("MouseUp did not end the drag session")
	}

	// Zero-height split ignores drags (no divide by zero).
	z := &VSplit{TopFrac: 30, dragging: true}
	z.SetBounds(toolkit.Rect{W: 10, H: 0})
	z.OnEvent(vdrag(5, 5))
}

func TestVSplitClickRouting(t *testing.T) {
	top, bot := &spyWidget{}, &spyWidget{}
	v := &VSplit{Top: top, Bottom: bot, TopFrac: 30}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 100}) // grip at Y=30

	// Click above the grip -> top pane, coords unchanged.
	v.OnEvent(vclick(4, 10))
	if top.count != 1 || top.last.Y != 10 {
		t.Errorf("top click: count=%d Y=%d", top.count, top.last.Y)
	}
	// Click below the grip -> bottom pane, translated by th+1.
	v.OnEvent(vclick(4, 50))
	if bot.count != 1 || bot.last.Y != 50-31 {
		t.Errorf("bottom click: count=%d Y=%d, want Y=19", bot.count, bot.last.Y)
	}
	// Non-mouse event -> top pane.
	v.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Tab"})
	if top.count != 2 {
		t.Errorf("keydown not routed to top: count=%d", top.count)
	}
	// Stray drag/up without a session are ignored.
	v.OnEvent(vdrag(4, 50))
	v.OnEvent(vup(4, 50))
	if top.count != 2 || bot.count != 1 {
		t.Errorf("stray drag/up leaked: top=%d bottom=%d", top.count, bot.count)
	}

	// Nil-pane guards: clicks and keys on both sides are silent no-ops.
	empty := &VSplit{TopFrac: 30}
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 100})
	empty.OnEvent(vclick(4, 10))                                        // nil top
	empty.OnEvent(vclick(4, 50))                                        // nil bottom
	empty.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "x"}) // nil top
}
