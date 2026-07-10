// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestSparklineMinMax(t *testing.T) {
	// Exercises both the "new min" and "new max" branches.
	mn, mx := minMax([]float64{3, 1, 5, 2})
	if mn != 1 || mx != 5 {
		t.Errorf("minMax = (%v,%v), want (1,5)", mn, mx)
	}
}

func TestSparklineDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	// Decode to a CellPainter so we can read the glyphs.
	draw := func(vals []float64, w int) []rune {
		s := NewSparkline(vals)
		s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: w, H: 1})
		cp := painter.NewCellPainter(w, 1)
		s.Draw(cp, theme)
		out := make([]rune, w)
		for x := 0; x < w; x++ {
			out[x] = cp.Cells[x].Rune
		}
		return out
	}

	// A rising series: first col is the lowest block, last is the highest.
	g := draw([]float64{1, 2, 3, 4}, 4)
	if g[0] != '▁' || g[3] != '█' {
		t.Errorf("rising series = %q, want ▁…█", string(g))
	}

	// A flat series renders as the lowest block everywhere (rng==0 branch).
	f := draw([]float64{5, 5, 5}, 3)
	for i, r := range f {
		if r != '▁' {
			t.Errorf("flat[%d] = %q, want ▁", i, string(r))
		}
	}

	// More values than columns: only the most recent W are drawn (tail), scaled
	// over the FULL series (min 1, max 9), so the tail's 1→▁ and 8→▇.
	tail := draw([]float64{9, 9, 9, 1, 8}, 2)
	if tail[0] != '▁' || tail[1] != '▇' {
		t.Errorf("tail = %q, want ▁▇", string(tail))
	}

	// Empty series draws nothing.
	empty := NewSparkline(nil)
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 5, H: 1})
	empty.Draw(painter.NewPixelPainter(make([]byte, 5*4), 5, 1), theme)
}
