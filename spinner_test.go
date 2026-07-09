// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func tick() toolkit.Event { return toolkit.Event{Kind: EventTick} }

func TestSpinnerAdvance(t *testing.T) {
	s := NewSpinner("Loading")
	if !s.Active || len(s.Frames) == 0 {
		t.Fatal("NewSpinner should be active with default frames")
	}
	// Each tick advances one frame, wrapping.
	for i := 1; i <= len(s.Frames); i++ {
		s.OnEvent(tick())
		want := i % len(s.Frames)
		if s.frame != want {
			t.Fatalf("tick %d: frame=%d, want %d", i, s.frame, want)
		}
	}
	// Inactive: ticks do not advance.
	s.Active = false
	before := s.frame
	s.OnEvent(tick())
	if s.frame != before {
		t.Errorf("inactive spinner advanced: %d -> %d", before, s.frame)
	}
	// Non-tick events are ignored.
	s.Active = true
	s.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "x"})
	if s.frame != before {
		t.Errorf("keydown advanced the spinner")
	}
	// Empty frames: tick is a safe no-op.
	empty := &Spinner{Active: true}
	empty.OnEvent(tick())
}

func TestSpinnerDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	s := NewSpinner("Working")
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	s.Draw(mk(20, 1), theme)

	// No label → only the frame glyph.
	nl := NewSpinner("")
	nl.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	nl.Draw(mk(4, 1), theme)

	// No frames → only the label (guarded).
	nf := &Spinner{Label: "text"}
	nf.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	nf.Draw(mk(10, 1), theme)
}
