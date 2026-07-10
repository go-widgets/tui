// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestExpanderToggleAndForward(t *testing.T) {
	body := &spyWidget{}
	toggles := 0
	last := false
	e := NewExpander("Details", body)
	e.OnToggle = func(v bool) { toggles++; last = v }
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 5})
	// Body laid out below the header row.
	if b := body.Bounds(); b.Y != 1 || b.H != 4 {
		t.Fatalf("body bounds = %+v, want Y1 H4", b)
	}

	// Header click expands; another collapses.
	e.OnEvent(vclick(3, 0))
	if !e.Expanded || toggles != 1 || !last {
		t.Fatalf("expand click: expanded=%v toggles=%d", e.Expanded, toggles)
	}
	// While collapsed... first re-collapse.
	e.OnEvent(vclick(3, 0))
	if e.Expanded {
		t.Fatal("second header click did not collapse")
	}
	// A body-area click while collapsed is a no-op (body gets nothing).
	e.OnEvent(vclick(3, 2))
	if body.count != 0 {
		t.Errorf("collapsed body received a click: %d", body.count)
	}
	// Expand (via Enter), then a body click forwards (translated Y-1).
	e.OnEvent(vkey("Enter"))
	if !e.Expanded {
		t.Fatal("Enter did not expand")
	}
	e.OnEvent(vclick(3, 2))
	if body.count != 1 || body.last.Y != 1 {
		t.Errorf("body click: count=%d Y=%d, want 1/1", body.count, body.last.Y)
	}
	// An unrelated key is a no-op.
	before := e.Expanded
	e.OnEvent(vkey("x"))
	if e.Expanded != before {
		t.Error("unrelated key toggled")
	}
	// nil body + nil OnToggle are safe.
	bare := NewExpander("t", nil)
	bare.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	bare.OnEvent(vclick(0, 0)) // toggle, nil OnToggle
	bare.OnEvent(vclick(0, 2)) // expanded, nil body → no-op
}

func TestExpanderDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()
	body := &spyWidget{}
	e := NewExpander("Section", body)
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 5})
	e.Draw(mk(20, 5), theme) // collapsed (▸), body not drawn
	e.Expanded = true
	e.Draw(mk(20, 5), theme) // expanded (▾), body drawn
	e.SetFocused(true)
	e.Draw(mk(20, 5), theme) // accent chevron
	// Expanded with nil body → header only, no panic.
	nb := NewExpander("t", nil)
	nb.Expanded = true
	nb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	nb.Draw(mk(10, 3), theme)
}
