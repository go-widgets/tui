// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// ProgressBar is a cell-native horizontal fill: a SurfaceAlt track with the
// first Fraction of its cells filled in Accent, and an optional Label centred
// over the bar. Fraction is clamped to [0,1]. Use it for download / load /
// completion indicators at terminal scale.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type ProgressBar struct {
	toolkit.Base
	Fraction float64
	Label    string
}

// NewProgressBar builds an empty (Fraction=0) ProgressBar.
func NewProgressBar() *ProgressBar { return &ProgressBar{} }

// SetFraction clamps f to [0,1] and assigns it (0 = empty, 1 = full).
func (p *ProgressBar) SetFraction(f float64) {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	p.Fraction = f
}

// Draw paints the track, the Accent fill proportional to Fraction, and the
// centred Label.
func (p *ProgressBar) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := p.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	f := p.Fraction
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	fillW := int(float64(r.W) * f)
	if fillW > 0 {
		pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: fillW, H: r.H}, theme.Accent)
	}
	if p.Label != "" {
		tw := utf8.RuneCountInString(p.Label)
		tx := r.X + (r.W-tw)/2
		ty := r.Y + (r.H-1)/2
		toolkit.DrawText(pnt, tx, ty, p.Label, theme.OnSurface)
	}
}
