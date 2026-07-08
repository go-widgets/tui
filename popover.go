// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Popover is a cell-native modal overlay: a bordered box with a Title on the
// top row and Body lines below, shown only while Visible. It sizes its drawn
// height to the content (3 border/title rows + one per Body line, capped to its
// bounds). HitTest returns false while hidden, so an invisible Popover never
// claims a click from the widgets beneath it.
//
// A toolkit.Widget: renders through painter.Painter (cell grid for TUI, RGBA
// buffer for WUI/GUI).
type Popover struct {
	toolkit.Base
	Title   string
	Body    []string
	Visible bool
}

// HitTest — an invisible Popover must not claim clicks (its bounds still cover
// the area), else routing would swallow events meant for the content below.
func (p *Popover) HitTest(px, py int) bool {
	if !p.Visible {
		return false
	}
	return p.Base.HitTest(px, py)
}

// Draw paints the box (when Visible) tightened to the content height.
func (p *Popover) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !p.Visible {
		return
	}
	r := p.Bounds()
	need := 3 + len(p.Body) // border-top + title + gap + body ... capped below
	if need > r.H {
		need = r.H
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, theme.SurfaceAlt)
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, theme.Border, 1)
	toolkit.DrawText(pnt, r.X+2, r.Y+1, p.Title, theme.OnSurface)
	for i, line := range p.Body {
		y := r.Y + 2 + i
		if y >= r.Y+need-1 {
			break
		}
		toolkit.DrawText(pnt, r.X+2, y, line, theme.OnSurface)
	}
}
