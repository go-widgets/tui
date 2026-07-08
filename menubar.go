// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// MenuItem is one clickable entry in a MenuBar. OnClick is optional -- a nil
// callback is a no-op (useful for a decorative or coming-soon slot).
type MenuItem struct {
	Label   string
	OnClick func()
}

const (
	menuBarSep = 3 // cells between items
	menuBarPad = 1 // left padding before the first item
)

// MenuBar is a cell-native horizontal menu: item labels laid out left to right
// on a single row, separated by menuBarSep cells, with a click on a label's
// cell range firing its OnClick. It is a toolkit.Widget, rendering through
// painter.Painter (cell grid for TUI, RGBA buffer for WUI/GUI).
type MenuBar struct {
	toolkit.Base
	Items []MenuItem
}

// NewMenuBar returns a MenuBar over items.
func NewMenuBar(items ...MenuItem) *MenuBar { return &MenuBar{Items: items} }

// Draw paints the bar background and the item labels.
func (m *MenuBar) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := m.Bounds()
	p.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.Background)
	x := r.X + menuBarPad
	for _, item := range m.Items {
		toolkit.DrawText(p, x, r.Y, item.Label, theme.OnSurface)
		x += utf8.RuneCountInString(item.Label) + menuBarSep
	}
}

// ItemXRange returns the [start, end) local-X cell range of the i-th item, or
// (-1, -1) if i is out of range. Callers use it both to hit-test and to anchor
// a dropdown directly under an item.
func (m *MenuBar) ItemXRange(i int) (int, int) {
	x := menuBarPad
	for k, item := range m.Items {
		if k == i {
			return x, x + utf8.RuneCountInString(item.Label)
		}
		x += utf8.RuneCountInString(item.Label) + menuBarSep
	}
	return -1, -1
}

// OnEvent fires the OnClick of whichever item's label range contains a click.
func (m *MenuBar) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	for i, item := range m.Items {
		x0, x1 := m.ItemXRange(i)
		if ev.X >= x0 && ev.X < x1 {
			if item.OnClick != nil {
				item.OnClick()
			}
			return
		}
	}
}
