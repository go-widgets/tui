// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// spyWidget records the events it receives; Bounds/SetBounds/HitTest come from
// toolkit.Base (bounds-based hit-testing).
type spyWidget struct {
	toolkit.Base
	last  toolkit.Event
	count int
}

func (s *spyWidget) Draw(painter.Painter, *toolkit.Theme) {}
func (s *spyWidget) OnEvent(ev toolkit.Event)             { s.last = ev; s.count++ }

func vclick(x, y int) toolkit.Event { return toolkit.Event{Kind: toolkit.EventClick, X: x, Y: y} }
func vdrag(x, y int) toolkit.Event  { return toolkit.Event{Kind: toolkit.EventMouseDrag, X: x, Y: y} }
func vup(x, y int) toolkit.Event    { return toolkit.Event{Kind: toolkit.EventMouseUp, X: x, Y: y} }

func TestVBoxLayoutAndDraw(t *testing.T) {
	h, b, f := &spyWidget{}, &spyWidget{}, &spyWidget{}
	o := &spyWidget{}
	v := &VBox{Header: h, Body: b, Footer: f, HeaderH: 1, FooterH: 1, Overlays: []toolkit.Widget{o}}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	if h.Bounds().H != 1 || f.Bounds().Y != 19 || b.Bounds().Y != 1 || b.Bounds().H != 18 {
		t.Fatalf("layout: header=%+v body=%+v footer=%+v", h.Bounds(), b.Bounds(), f.Bounds())
	}
	// Overlay inset: x=4, y=headerH+2=3, w=W-8=12, h=H-hH-fH-4=14.
	if ob := o.Bounds(); ob.X != 4 || ob.Y != 3 || ob.W != 12 || ob.H != 14 {
		t.Fatalf("overlay bounds = %+v, want {4,3,12,14}", ob)
	}
	// Tiny bounds clamp the overlay w/h to >= 1.
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 2, H: 2})
	if ob := o.Bounds(); ob.W < 1 || ob.H < 1 {
		t.Errorf("overlay w/h not clamped: %+v", ob)
	}
	v.Draw(painter.NewPixelPainter(make([]byte, 20*20*4), 20, 20), toolkit.DefaultLight())
}

func TestVBoxRouting(t *testing.T) {
	h, b, f := &spyWidget{}, &spyWidget{}, &spyWidget{}
	o := &spyWidget{}
	v := &VBox{Header: h, Body: b, Footer: f, HeaderH: 1, FooterH: 1, Overlays: []toolkit.Widget{o}}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20}) // overlay at x[4,16) y[3,17)

	// Click inside the overlay -> overlay (captured), coords translated.
	v.OnEvent(vclick(5, 5))
	if o.count != 1 || o.last.X != 1 || o.last.Y != 2 {
		t.Fatalf("overlay click: count=%d last=%+v", o.count, o.last)
	}
	// Drag continues to the captured overlay; MouseUp releases capture.
	v.OnEvent(vdrag(6, 6))
	if o.count != 2 {
		t.Fatalf("captured drag not forwarded: count=%d", o.count)
	}
	v.OnEvent(vup(6, 6))
	if v.dragTarget != nil {
		t.Fatal("MouseUp did not release the drag capture")
	}
	// Click header band (Y < headerH) -> header.
	v.OnEvent(vclick(2, 0))
	if h.count != 1 {
		t.Errorf("header click count=%d", h.count)
	}
	// Click footer band (Y >= H-footerH) -> footer (translated).
	v.OnEvent(vup(0, 0)) // clear any capture from the header click
	v.dragTarget = nil
	v.OnEvent(vclick(2, 19))
	if f.count != 1 || f.last.Y != 0 {
		t.Errorf("footer click count=%d last=%+v", f.count, f.last)
	}
	// Click body band, X left of the overlay (overlay HitTest false) -> body.
	v.dragTarget = nil
	v.OnEvent(vclick(2, 10))
	if b.count != 1 || b.last.Y != 9 {
		t.Errorf("body click count=%d last=%+v", b.count, b.last)
	}
	// A non-click event routes to the body.
	v.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if b.count != 2 {
		t.Errorf("non-click not routed to body: count=%d", b.count)
	}
	// A drag with no active capture falls through to the body (not dropped).
	v.dragTarget = nil
	v.OnEvent(vdrag(5, 5))
	if b.count != 3 {
		t.Errorf("uncaptured drag not routed to body: count=%d", b.count)
	}
}

func TestVBoxOverlayPriorityAndNilChildren(t *testing.T) {
	// Two overlays share the inset bounds; the later one (top) claims first.
	lo, hi := &spyWidget{}, &spyWidget{}
	v := &VBox{HeaderH: 1, FooterH: 1, Overlays: []toolkit.Widget{lo, hi}}
	v.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	v.OnEvent(vclick(5, 5))
	if hi.count != 1 || lo.count != 0 {
		t.Fatalf("overlay priority: hi=%d lo=%d, want 1/0", hi.count, lo.count)
	}

	// Nil header/body/footer: clicks in each band are silent no-ops, and
	// SetBounds / Draw skip the nil children.
	empty := &VBox{HeaderH: 1, FooterH: 1}
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 10})
	empty.Draw(painter.NewPixelPainter(make([]byte, 10*10*4), 10, 10), toolkit.DefaultLight())
	empty.OnEvent(vclick(0, 0))  // header band, nil header
	empty.OnEvent(vclick(0, 9))  // footer band, nil footer
	empty.OnEvent(vclick(0, 5))  // body band, nil body
	empty.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "X"}) // non-click, nil body
}
