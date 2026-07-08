// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestScaleSetValue(t *testing.T) {
	s := NewScale(0, 100, 50)
	if s.Value != 50 {
		t.Fatalf("NewScale value = %v, want 50", s.Value)
	}
	s.SetValue(-10)
	if s.Value != 0 {
		t.Errorf("clamp low = %v", s.Value)
	}
	s.SetValue(200)
	if s.Value != 100 {
		t.Errorf("clamp high = %v", s.Value)
	}
}

func TestScaleClickAndDrag(t *testing.T) {
	got := -1.0
	s := NewScale(0, 100, 0)
	s.OnChange = func(v float64) { got = v }
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 11, H: 1}) // W-1 = 10 columns

	// Click at column 5 -> half of [0,100].
	s.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5})
	if s.Value != 50 || got != 50 {
		t.Fatalf("click col 5: value=%v onchange=%v, want 50", s.Value, got)
	}
	// Drag maps the same way (captured-drag path).
	s.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 10})
	if s.Value != 100 {
		t.Errorf("drag col 10: value=%v, want 100", s.Value)
	}
	// Out-of-range columns clamp.
	s.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: -3})
	if s.Value != 0 {
		t.Errorf("click below: value=%v", s.Value)
	}
	s.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 999})
	if s.Value != 100 {
		t.Errorf("click above: value=%v", s.Value)
	}

	// Degenerate 1-cell width parks at Min.
	one := NewScale(0, 100, 40)
	one.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 1, H: 1})
	one.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0})
	if one.Value != 0 {
		t.Errorf("1-cell click: value=%v, want 0 (Min)", one.Value)
	}

	// Zero-width is ignored (guarded before setFromX).
	zw := NewScale(0, 100, 30)
	zw.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 0, H: 1})
	zw.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0})
	if zw.Value != 30 {
		t.Errorf("zero-width click changed value: %v", zw.Value)
	}

	// No OnChange handler must not panic.
	NewScale(0, 10, 0).OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0})
}

func TestScaleKeys(t *testing.T) {
	// Explicit Step, with an OnChange handler that must fire on key steps.
	fired := -1.0
	s := NewScale(0, 100, 50)
	s.Step = 5
	s.OnChange = func(v float64) { fired = v }
	s.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "ArrowRight"})
	if s.Value != 55 || fired != 55 {
		t.Errorf("right +Step: value=%v onchange=%v, want 55", s.Value, fired)
	}
	s.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "ArrowLeft"})
	if s.Value != 50 {
		t.Errorf("left -Step: %v, want 50", s.Value)
	}
	// Default step = range/10 when Step <= 0.
	d := NewScale(0, 100, 50)
	d.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "ArrowRight"})
	if d.Value != 60 {
		t.Errorf("right default step: %v, want 60", d.Value)
	}
	// An unrelated key is a no-op.
	d.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if d.Value != 60 {
		t.Errorf("Enter changed value: %v", d.Value)
	}

	// Non-interactive (Max == Min) ignores every event.
	flat := NewScale(5, 5, 5)
	flat.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	flat.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5})
	flat.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "ArrowRight"})
	if flat.Value != 5 {
		t.Errorf("flat scale changed: %v", flat.Value)
	}
}

func TestScaleDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	s := NewScale(0, 100, 75)
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
	s.Draw(mk(20, 3), theme)

	// Min == Max: fraction() takes the 0 branch.
	flat := NewScale(5, 5, 5)
	flat.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	flat.Draw(mk(20, 1), theme)
}
