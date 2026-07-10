// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Sparkline is a cell-native mini-chart: each value in Values becomes one
// vertical bar column drawn with the eighth-block glyphs ▁▂▃▄▅▆▇█, scaled between
// the series' own min and max. It's the compact data-viz the cell set was
// missing — an inline trend line for a metric. When there are more values than
// columns, the most recent ones (the tail) are shown. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Sparkline struct {
	toolkit.Base
	Values []float64
}

// sparkBlocks are the eight partial-block glyphs, low to high.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// NewSparkline builds a Sparkline over the given series.
func NewSparkline(values []float64) *Sparkline { return &Sparkline{Values: values} }

// minMax returns the smallest and largest of a non-empty series.
func minMax(vs []float64) (float64, float64) {
	mn, mx := vs[0], vs[0]
	for _, v := range vs[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}

// Draw paints one block glyph per value (Accent) on the bottom row, scaled to
// the series range; a flat series renders as the lowest block.
func (s *Sparkline) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if len(s.Values) == 0 {
		return
	}
	r := s.Bounds()
	mn, mx := minMax(s.Values)
	rng := mx - mn
	if rng == 0 {
		rng = 1
	}
	start := 0
	if len(s.Values) > r.W {
		start = len(s.Values) - r.W // show the most recent columns
	}
	y := r.Y + r.H - 1
	for i := start; i < len(s.Values); i++ {
		level := int((s.Values[i] - mn) / rng * 7.999) // always in [0,7]
		toolkit.DrawText(pnt, r.X+(i-start), y, string(sparkBlocks[level]), theme.Accent)
	}
}
