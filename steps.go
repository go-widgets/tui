// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Steps is a cell-native step/wizard indicator: numbered Labels joined by " ─ ",
// with the Current step in Accent, completed steps (before Current) in the muted
// Border ink, and upcoming steps in OnSurface. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Steps struct {
	toolkit.Base
	Labels  []string
	Current int
	// Orientation lays the steps out left-to-right (toolkit.Horizontal, the
	// zero value) or top-to-bottom (toolkit.Vertical — a side checklist).
	// Vertical draws a "│" connector between rows instead of the horizontal
	// " ─ " separator. Reuses toolkit.Orientation so the pixel and cell Steps
	// share one vocabulary.
	Orientation toolkit.Orientation
}

// stepSep is the connector between steps.
const stepSep = " ─ "

// NewSteps builds a Steps indicator over labels with the given active step.
func NewSteps(labels []string, current int) *Steps {
	return &Steps{Labels: labels, Current: current}
}

// Draw paints "1 Label ─ 2 Label ─ …" with per-state ink when Horizontal
// (the default), or one "N Label" per row joined by a "│" connector when
// Orientation is Vertical.
func (s *Steps) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	if s.Orientation == toolkit.Vertical {
		s.drawVertical(pnt, theme, r)
		return
	}
	x := r.X
	for i, label := range s.Labels {
		ink := theme.OnSurface // upcoming
		switch {
		case i == s.Current:
			ink = theme.Accent
		case i < s.Current:
			ink = theme.Border // done (muted)
		}
		text := strconv.Itoa(i+1) + " " + label
		toolkit.DrawText(pnt, x, r.Y, text, ink)
		x += utf8.RuneCountInString(text)
		if i < len(s.Labels)-1 {
			toolkit.DrawText(pnt, x, r.Y, stepSep, theme.Border)
			x += 3
		}
	}
}

// drawVertical paints one "N Label" per row, with a "│" connector row between
// successive steps, clipped to r's height.
func (s *Steps) drawVertical(pnt painter.Painter, theme *toolkit.Theme, r toolkit.Rect) {
	y := r.Y
	for i, label := range s.Labels {
		if y >= r.Y+r.H {
			break
		}
		ink := theme.OnSurface // upcoming
		switch {
		case i == s.Current:
			ink = theme.Accent
		case i < s.Current:
			ink = theme.Border // done (muted)
		}
		text := strconv.Itoa(i+1) + " " + label
		toolkit.DrawText(pnt, r.X, y, text, ink)
		y++
		if i < len(s.Labels)-1 && y < r.Y+r.H {
			toolkit.DrawText(pnt, r.X, y, "│", theme.Border)
			y++
		}
	}
}
