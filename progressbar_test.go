// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestProgressBarSetFraction(t *testing.T) {
	p := NewProgressBar()
	if p.Fraction != 0 {
		t.Fatalf("NewProgressBar Fraction = %v, want 0", p.Fraction)
	}
	p.SetFraction(0.5)
	if p.Fraction != 0.5 {
		t.Errorf("SetFraction(0.5) = %v", p.Fraction)
	}
	p.SetFraction(-1) // clamp low
	if p.Fraction != 0 {
		t.Errorf("clamp low = %v", p.Fraction)
	}
	p.SetFraction(2) // clamp high
	if p.Fraction != 1 {
		t.Errorf("clamp high = %v", p.Fraction)
	}
}

func TestProgressBarDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Partial fill + label.
	p := &ProgressBar{Fraction: 0.5, Label: "50%"}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	p.Draw(mk(20, 1), theme)

	// Empty (fillW == 0) + no label, multi-row bar.
	e := NewProgressBar()
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
	e.Draw(mk(20, 3), theme)

	// Out-of-range Fraction set directly on the field is clamped in Draw.
	over := &ProgressBar{Fraction: 5}
	over.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	over.Draw(mk(20, 1), theme)
	under := &ProgressBar{Fraction: -3, Label: "x"}
	under.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	under.Draw(mk(20, 1), theme)
}
