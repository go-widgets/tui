// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestStatusbarSetSegment(t *testing.T) {
	s := NewStatusbar([]string{"a"})
	if s.SegmentMinW != StatusbarSegmentMinW {
		t.Errorf("NewStatusbar min = %d, want %d", s.SegmentMinW, StatusbarSegmentMinW)
	}
	// Grow lazily: index past the end fills intermediate slots with "".
	s.SetSegment(3, "d")
	if len(s.Segments) != 4 || s.Segments[3] != "d" || s.Segments[1] != "" {
		t.Errorf("grow: %#v", s.Segments)
	}
	// Replace in place.
	s.SetSegment(0, "z")
	if s.Segments[0] != "z" {
		t.Errorf("replace: %#v", s.Segments)
	}
	// Negative index ignored.
	s.SetSegment(-1, "nope")
	if len(s.Segments) != 4 {
		t.Errorf("negative index changed length: %#v", s.Segments)
	}
}

func TestStatusbarDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }

	// Multi-segment: non-last (short -> min width, and long -> text+2) plus a
	// last segment that fills the remainder + dividers between them.
	s := NewStatusbar([]string{"x", "a longer segment", "tail"})
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 60, H: 1})
	s.Draw(mk(60, 1), toolkit.DefaultLight())

	// Zero SegmentMinW falls back to the default.
	s.SegmentMinW = 0
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 60, H: 3}) // H>1 exercises the centred row
	s.Draw(mk(60, 3), toolkit.DefaultLight())

	// Empty bar: just the background, no segments.
	e := NewStatusbar(nil)
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	e.Draw(mk(20, 1), toolkit.DefaultLight())
}
