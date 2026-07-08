// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// MenuDropdown is an anchored, self-sizing dropdown menu -- the panel that
// opens under a MenuBar item. It positions itself at (AnchorX, AnchorY) and
// sizes to its content, ignoring any bounds a container hands it. A click on a
// Body row runs the matching ItemActions entry (nil / short slice = an
// informational row: the click still dismisses the menu but runs nothing) and
// hides the menu.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type MenuDropdown struct {
	toolkit.Base
	Title       string
	Body        []string
	ItemActions []func() // parallel to Body; nil / short = informational row
	Visible     bool
	AnchorX     int
	AnchorY     int
}

// size returns the natural (width, height) in cells: the widest of Title / Body
// plus padding, and a row per Body line (minimum height 3).
func (d *MenuDropdown) size() (int, int) {
	w := utf8.RuneCountInString(d.Title)
	for _, line := range d.Body {
		if l := utf8.RuneCountInString(line); l > w {
			w = l
		}
	}
	w += 4 // 1-cell border + 1-cell text pad on each side
	h := 2 + len(d.Body)
	if h < 3 {
		h = 3
	}
	return w, h
}

// SetBounds ignores the requested rect and self-positions at the anchor with
// the natural size (a container's layout pass calls this; the dropdown opts out
// of the normal flow).
func (d *MenuDropdown) SetBounds(_ toolkit.Rect) {
	w, h := d.size()
	d.Base.SetBounds(toolkit.Rect{X: d.AnchorX, Y: d.AnchorY, W: w, H: h})
}

// HitTest — hidden dropdowns claim no clicks.
func (d *MenuDropdown) HitTest(px, py int) bool {
	if !d.Visible {
		return false
	}
	return d.Base.HitTest(px, py)
}

// Draw paints the anchored box (when Visible): Title on the top row, Body rows
// below.
func (d *MenuDropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !d.Visible {
		return
	}
	d.SetBounds(toolkit.Rect{}) // refresh the anchored geometry
	r := d.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.Border, 1)
	if d.Title != "" {
		toolkit.DrawText(pnt, r.X+2, r.Y, d.Title, theme.OnSurface)
	}
	for i, line := range d.Body {
		toolkit.DrawText(pnt, r.X+2, r.Y+1+i, line, theme.OnSurface)
	}
}

// OnEvent runs the clicked Body row's action (if any) and dismisses the menu.
func (d *MenuDropdown) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	idx := ev.Y - 1 // Body rows start at local Y=1 (Title on 0)
	if idx >= 0 && idx < len(d.ItemActions) && d.ItemActions[idx] != nil {
		d.ItemActions[idx]()
	}
	d.Visible = false
}
