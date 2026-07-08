// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Statusbar is a cell-native footer strip: N text segments laid out
// left-to-right (e.g. "Line 12, Col 4" + "UTF-8" + "Go" in an editor), each
// with a 1-cell pad, a thin vertical divider between them, and the LAST segment
// stretched to fill the remaining width so an empty bar still looks deliberate.
// It is the natural pairing for a MenuBar above and a document area between —
// together they assemble the stock window frame at terminal scale.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Statusbar struct {
	toolkit.Base
	Segments []string
	// SegmentMinW is the minimum width (in cells) any non-last segment takes;
	// the last segment always fills the rest of the bar. Defaults to
	// StatusbarSegmentMinW when <= 0.
	SegmentMinW int
}

// StatusbarSegmentMinW is the default minimum width of a non-last segment.
const StatusbarSegmentMinW = 10

// NewStatusbar builds a Statusbar with the given segments and the default
// minimum segment width.
func NewStatusbar(segs []string) *Statusbar {
	return &Statusbar{Segments: segs, SegmentMinW: StatusbarSegmentMinW}
}

// SetSegment replaces the i-th segment in place. An index past the end grows
// the bar (filling intermediate slots with "") so callers can add segments
// lazily; a negative index is ignored.
func (s *Statusbar) SetSegment(i int, text string) {
	if i < 0 {
		return
	}
	for len(s.Segments) <= i {
		s.Segments = append(s.Segments, "")
	}
	s.Segments[i] = text
}

// Draw paints the strip background plus every segment, with a divider column
// between adjacent segments.
func (s *Statusbar) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	min := s.SegmentMinW
	if min <= 0 {
		min = StatusbarSegmentMinW
	}
	ty := r.Y + (r.H-1)/2 // centre the text row within the bar height
	x := r.X
	n := len(s.Segments)
	for i, seg := range s.Segments {
		var w int
		if i == n-1 {
			w = r.X + r.W - x // last segment fills the remainder
		} else {
			w = utf8.RuneCountInString(seg) + 2
			if w < min {
				w = min
			}
		}
		toolkit.DrawText(pnt, x+1, ty, seg, theme.OnSurface)
		if i < n-1 {
			toolkit.DrawText(pnt, x+w-1, ty, "│", theme.Border)
		}
		x += w
	}
}
