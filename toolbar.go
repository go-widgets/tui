// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// ToolbarItem is one cell in a Toolbar: a labelled button with an OnClick and a
// Disabled flag, or — when Separator is true — a 1-cell vertical divider (the
// other fields are ignored).
type ToolbarItem struct {
	Label     string
	OnClick   func()
	Disabled  bool
	Separator bool
}

// Toolbar is a cell-native horizontal strip of labelled buttons and optional
// separators — the action strip that sits below a MenuBar and composes with
// Notebook + Statusbar into a stock window frame. Buttons size to their labels;
// a click runs an enabled item's OnClick.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Toolbar struct {
	toolkit.Base
	Items []ToolbarItem
}

// NewToolbar builds a Toolbar with the given items.
func NewToolbar(items []ToolbarItem) *Toolbar { return &Toolbar{Items: items} }

// itemRanges returns the cumulative start columns of the items (length
// len(Items)+1); item i spans [xs[i], xs[i+1]). A separator is 1 cell; a button
// is its label plus a 1-cell pad on each side.
func (t *Toolbar) itemRanges() []int {
	xs := make([]int, len(t.Items)+1)
	x := 0
	for i, it := range t.Items {
		xs[i] = x
		if it.Separator {
			x++
		} else {
			x += utf8.RuneCountInString(it.Label) + 2
		}
	}
	xs[len(t.Items)] = x
	return xs
}

// Draw paints the strip background, each button (muted when Disabled), and the
// separators.
func (t *Toolbar) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := t.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	xs := t.itemRanges()
	for i, it := range t.Items {
		x := r.X + xs[i]
		if it.Separator {
			toolkit.DrawText(pnt, x, r.Y, "│", theme.Border)
			continue
		}
		ink := theme.OnSurface
		if it.Disabled {
			ink = LineNumberInk(theme)
		}
		toolkit.DrawText(pnt, x+1, r.Y, it.Label, ink)
	}
}

// OnEvent runs the clicked item's OnClick when it is an enabled button.
func (t *Toolbar) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	xs := t.itemRanges()
	for i, it := range t.Items {
		if ev.X < xs[i] || ev.X >= xs[i+1] {
			continue
		}
		if !it.Separator && !it.Disabled && it.OnClick != nil {
			it.OnClick()
		}
		return
	}
}
