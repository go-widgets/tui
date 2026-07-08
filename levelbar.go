// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// LevelBar is the discrete cousin of ProgressBar: Max equal segments separated
// by a 1-cell gap, the first Value in Accent and the rest in SurfaceAlt. Useful
// for battery / signal-strength / step indicators at terminal scale.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type LevelBar struct {
	toolkit.Base
	Value, Max int
}

// NewLevelBar builds a LevelBar with the given Max (floored at 1) and Value 0.
func NewLevelBar(max int) *LevelBar {
	if max < 1 {
		max = 1
	}
	return &LevelBar{Max: max}
}

// Draw paints Max segments with a 1-cell gap; the first Value segments use
// Accent, the rest SurfaceAlt.
func (l *LevelBar) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if l.Max < 1 {
		return
	}
	r := l.Bounds()
	cellW := (r.W - (l.Max - 1)) / l.Max
	if cellW < 1 {
		cellW = 1
	}
	for i := 0; i < l.Max; i++ {
		fill := theme.SurfaceAlt
		if i < l.Value {
			fill = theme.Accent
		}
		x := r.X + i*(cellW+1)
		pnt.FillRect(painter.Rect{X: x, Y: r.Y, W: cellW, H: r.H}, fill)
	}
}
