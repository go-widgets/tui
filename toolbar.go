// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strings"
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

// Toolbar is a cell-native strip of labelled buttons and optional separators —
// the action strip that sits below a MenuBar and composes with Notebook +
// Statusbar into a stock window frame. Buttons size to their labels; a click
// runs an enabled item's OnClick.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Toolbar struct {
	toolkit.Base
	Items []ToolbarItem
	// Orientation lays the buttons out left-to-right (toolkit.Horizontal, the
	// zero value) or top-to-bottom (toolkit.Vertical). A vertical toolbar
	// draws its separators as horizontal dividers spanning the full width, so
	// the same Items slice works unchanged as a side rail. Reuses
	// toolkit.Orientation so the pixel and cell Toolbars share one
	// vocabulary.
	Orientation toolkit.Orientation
}

// NewToolbar builds a Toolbar with the given items.
func NewToolbar(items []ToolbarItem) *Toolbar { return &Toolbar{Items: items} }

// vertical reports whether the strip lays its items out top-to-bottom.
func (t *Toolbar) vertical() bool { return t.Orientation == toolkit.Vertical }

// itemRanges returns the cumulative start offsets of the items along the
// layout axis (length len(Items)+1); item i spans [xs[i], xs[i+1]). For the
// default horizontal layout the axis is columns: a separator is 1 cell, a
// button is its label plus a 1-cell pad on each side. For a vertical layout
// the axis is rows: every item — button or separator — is exactly 1 row.
func (t *Toolbar) itemRanges() []int {
	xs := make([]int, len(t.Items)+1)
	if t.vertical() {
		for i := range xs {
			xs[i] = i
		}
		return xs
	}
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
// separators (a vertical bar when horizontal, a full-width horizontal rule
// when vertical).
func (t *Toolbar) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := t.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	xs := t.itemRanges()
	vertical := t.vertical()
	for i, it := range t.Items {
		x, y := r.X+xs[i], r.Y
		if vertical {
			x, y = r.X, r.Y+xs[i]
		}
		if it.Separator {
			if vertical {
				toolkit.DrawText(pnt, x, y, strings.Repeat("─", r.W), theme.Border)
			} else {
				toolkit.DrawText(pnt, x, y, "│", theme.Border)
			}
			continue
		}
		ink := theme.OnSurface
		if it.Disabled {
			ink = LineNumberInk(theme)
		}
		toolkit.DrawText(pnt, x+1, y, it.Label, ink)
	}
}

// OnEvent runs the clicked item's OnClick when it is an enabled button. The
// hit-tested coordinate follows the layout axis: columns when horizontal,
// rows when vertical.
func (t *Toolbar) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	xs := t.itemRanges()
	along := ev.X
	if t.vertical() {
		along = ev.Y
	}
	for i, it := range t.Items {
		if along < xs[i] || along >= xs[i+1] {
			continue
		}
		if !it.Separator && !it.Disabled && it.OnClick != nil {
			it.OnClick()
		}
		return
	}
}
