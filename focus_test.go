// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestFocusRingTraversal(t *testing.T) {
	c0, c1, c2 := 0, 0, 0
	b0 := NewButton("a", func() { c0++ })
	b1 := NewButton("b", func() { c1++ })
	b2 := NewButton("c", func() { c2++ })
	b0.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	b1.SetBounds(toolkit.Rect{X: 0, Y: 1, W: 10, H: 1})
	b2.SetBounds(toolkit.Rect{X: 0, Y: 2, W: 10, H: 1})
	r := NewFocusRing(b0, b1, b2)

	if r.Focused() != 0 {
		t.Fatalf("initial focus = %d, want 0", r.Focused())
	}
	// Tab advances, wrapping; Shift+Tab retreats, wrapping.
	r.OnEvent(vkey("Tab"))
	r.OnEvent(vkey("Tab"))
	if r.Focused() != 2 {
		t.Fatalf("after 2 Tab: %d, want 2", r.Focused())
	}
	r.OnEvent(vkey("Tab")) // wrap to 0
	if r.Focused() != 0 {
		t.Fatalf("Tab wrap: %d, want 0", r.Focused())
	}
	r.OnEvent(vkey("Shift+Tab")) // wrap back to 2
	if r.Focused() != 2 {
		t.Fatalf("Shift+Tab wrap: %d, want 2", r.Focused())
	}
	// Traversal keys do not activate any button.
	if c0+c1+c2 != 0 {
		t.Fatalf("traversal activated a button: %d/%d/%d", c0, c1, c2)
	}

	// Focus clamps out-of-range indices.
	r.Focus(99)
	if r.Focused() != 2 {
		t.Errorf("Focus(99) = %d, want 2 (clamped)", r.Focused())
	}
	r.Focus(-5)
	if r.Focused() != 0 {
		t.Errorf("Focus(-5) = %d, want 0 (clamped)", r.Focused())
	}

	// A forwarded Enter activates the focused button only.
	r.Focus(1)
	r.OnEvent(vkey("Enter"))
	if c1 != 1 || c0 != 0 || c2 != 0 {
		t.Fatalf("Enter forwarded wrong: %d/%d/%d", c0, c1, c2)
	}

	// A click focuses + forwards to the member it hits.
	r.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 2}) // inside b2
	if r.Focused() != 2 || c2 != 1 {
		t.Fatalf("click routing: focused=%d c2=%d", r.Focused(), c2)
	}
	// A click hitting no member is a no-op.
	before := c0 + c1 + c2
	r.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 99, Y: 99})
	if c0+c1+c2 != before {
		t.Errorf("out-of-bounds click activated something")
	}
}

func TestFocusRingEmpty(t *testing.T) {
	r := NewFocusRing()
	if r.Focused() != -1 {
		t.Errorf("empty Focused() = %d, want -1", r.Focused())
	}
	// All operations are safe no-ops on an empty ring.
	r.Next()
	r.Prev()
	r.Focus(0)
	r.OnEvent(vkey("Tab"))
	r.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	r.OnEvent(vkey("x"))
}

// TestFocusRingDrawsFocusCue focuses each member in turn and draws the ring, so
// every widget type's focused-render branch (Entry caret, Button face,
// CheckButton / RadioButton accent mark) is exercised.
func TestFocusRingDrawsFocusCue(t *testing.T) {
	mk := func() *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, 40*4*4), 40, 4) }
	theme := toolkit.DefaultLight()

	en := NewEntry("hi")
	bt := NewButton("Go", nil)
	cb := NewCheckButton("On", false)  // unchecked → focus cue is the accent box
	rb := NewRadioButton("Pick")       // unchecked → accent mark on focus
	for i, w := range []Focusable{en, bt, cb, rb} {
		w.SetBounds(toolkit.Rect{X: 0, Y: i, W: 10, H: 1})
	}
	r := NewFocusRing(en, bt, cb, rb)
	for i := 0; i < 4; i++ {
		r.Focus(i)
		r.Draw(mk(), theme)
	}
}
