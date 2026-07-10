// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Badge is a cell-native accent pill: the Text (a count or short label) on an
// Accent background in the theme Background ink — the little "(3)" marker next to
// a tab or list header. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Badge struct {
	toolkit.Base
	Text string
}

// NewBadge builds a Badge with the given text.
func NewBadge(text string) *Badge { return &Badge{Text: text} }

// Draw paints the accent pill (a 1-cell pad on each side) and the text.
func (b *Badge) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := b.Bounds()
	w := utf8.RuneCountInString(b.Text) + 2
	if w > r.W {
		w = r.W
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: w, H: 1}, theme.Accent)
	toolkit.DrawText(pnt, r.X+1, r.Y, b.Text, theme.Background)
}
