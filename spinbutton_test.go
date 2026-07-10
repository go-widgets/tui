// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestSpinButtonNewAndSetValue(t *testing.T) {
	s := NewSpinButton(0, 10, 5, 2)
	if s.Value != 5 || s.Step != 2 {
		t.Fatalf("New: value=%d step=%d", s.Value, s.Step)
	}
	// Step <= 0 floors to 1; initial clamps into range.
	if got := NewSpinButton(0, 10, 99, 0); got.Step != 1 || got.Value != 10 {
		t.Errorf("floor/clamp: step=%d value=%d", got.Step, got.Value)
	}
	s.SetValue(-5)
	if s.Value != 0 {
		t.Errorf("clamp low = %d", s.Value)
	}
	s.SetValue(99)
	if s.Value != 10 {
		t.Errorf("clamp high = %d", s.Value)
	}
}

func TestSpinButtonStep(t *testing.T) {
	got := -1
	s := NewSpinButton(0, 10, 4, 2)
	s.OnChange = func(v int) { got = v }

	// Keyboard: Up/'+'/Right increment; Down/'-'/Left decrement.
	s.OnEvent(vkey("Up"))
	if s.Value != 6 || got != 6 {
		t.Fatalf("Up: value=%d onchange=%d", s.Value, got)
	}
	s.OnEvent(vchar("+"))
	if s.Value != 8 {
		t.Fatalf("+: value=%d", s.Value)
	}
	s.OnEvent(vkey("Left"))
	if s.Value != 6 {
		t.Fatalf("Left: value=%d", s.Value)
	}
	s.OnEvent(vchar("-"))
	if s.Value != 4 {
		t.Fatalf("-: value=%d", s.Value)
	}
	// Clamp at Max fires no OnChange.
	s.SetValue(10)
	got = -1
	s.OnEvent(vkey("Up"))
	if s.Value != 10 || got != -1 {
		t.Errorf("clamp-at-max: value=%d onchange=%d", s.Value, got)
	}
	// Unrelated key / char are ignored.
	s.OnEvent(vkey("Enter"))
	s.OnEvent(vchar("x"))
	if s.Value != 10 {
		t.Errorf("unrelated input changed value: %d", s.Value)
	}
	// nil OnChange must not panic.
	NewSpinButton(0, 5, 0, 1).OnEvent(vkey("Up"))
}

func TestSpinButtonClick(t *testing.T) {
	s := NewSpinButton(0, 100, 50, 5)
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	// Left cap (x<=1) decrements; right cap (x>=W-2) increments; middle no-op.
	s.OnEvent(vclick(0, 0))
	if s.Value != 45 {
		t.Fatalf("left cap: value=%d, want 45", s.Value)
	}
	s.OnEvent(vclick(9, 0))
	if s.Value != 50 {
		t.Fatalf("right cap: value=%d, want 50", s.Value)
	}
	s.OnEvent(vclick(5, 0)) // middle — no change
	if s.Value != 50 {
		t.Errorf("middle click changed value: %d", s.Value)
	}
}

func TestSpinButtonDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()
	s := NewSpinButton(0, 100, 42, 1)
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	s.Draw(mk(10, 1), theme)
	s.SetFocused(true)
	s.Draw(mk(10, 1), theme) // accent caps
}
