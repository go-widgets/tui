// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Breadcrumbs is a cell-native trail: the Items joined by a " › " separator, with
// the last (current) crumb in Accent and the rest in OnSurface. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Breadcrumbs struct {
	toolkit.Base
	Items []string
}

// breadcrumbSep is the trail separator (1 pad + chevron + 1 pad = 3 cells).
const breadcrumbSep = " › "

// NewBreadcrumbs builds a trail over items.
func NewBreadcrumbs(items []string) *Breadcrumbs { return &Breadcrumbs{Items: items} }

// Draw paints each crumb left to right, the last one in Accent, with a muted
// separator between them.
func (b *Breadcrumbs) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := b.Bounds()
	x := r.X
	for i, item := range b.Items {
		ink := theme.OnSurface
		if i == len(b.Items)-1 {
			ink = theme.Accent
		}
		toolkit.DrawText(pnt, x, r.Y, item, ink)
		x += utf8.RuneCountInString(item)
		if i < len(b.Items)-1 {
			toolkit.DrawText(pnt, x, r.Y, breadcrumbSep, theme.Border)
			x += 3
		}
	}
}
