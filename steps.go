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
}

// stepSep is the connector between steps.
const stepSep = " ─ "

// NewSteps builds a Steps indicator over labels with the given active step.
func NewSteps(labels []string, current int) *Steps {
	return &Steps{Labels: labels, Current: current}
}

// Draw paints "1 Label ─ 2 Label ─ …" with per-state ink.
func (s *Steps) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
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
