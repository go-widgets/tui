// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// VBox is a header / body / footer vertical layout: Header keeps a fixed
// HeaderH rows at the top, Footer a fixed FooterH rows at the bottom, and Body
// fills the space between. Overlays float on top, inset from the body area, and
// are hit-tested (so an invisible Popover / anchored MenuDropdown claims clicks
// only when it wants them). It captures a drag once a click lands on a child so
// subsequent EventMouseDrag / EventMouseUp keep flowing to that child with the
// same coordinate translation.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type VBox struct {
	toolkit.Base
	Header   toolkit.Widget
	Body     toolkit.Widget
	Footer   toolkit.Widget
	HeaderH  int
	FooterH  int
	Overlays []toolkit.Widget

	dragTarget toolkit.Widget
	dragDx     int
	dragDy     int
}

// SetBounds lays out the header, body, footer and overlays within r.
func (p *VBox) SetBounds(r toolkit.Rect) {
	p.Base.SetBounds(r)
	if p.Header != nil {
		p.Header.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: p.HeaderH})
	}
	if p.Footer != nil {
		p.Footer.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + r.H - p.FooterH, W: r.W, H: p.FooterH})
	}
	if p.Body != nil {
		p.Body.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + p.HeaderH, W: r.W, H: r.H - p.HeaderH - p.FooterH})
	}
	for _, o := range p.Overlays {
		bw := r.W - 8
		bh := r.H - p.HeaderH - p.FooterH - 4
		if bw < 1 {
			bw = 1
		}
		if bh < 1 {
			bh = 1
		}
		o.SetBounds(toolkit.Rect{X: r.X + 4, Y: r.Y + p.HeaderH + 2, W: bw, H: bh})
	}
}

// Draw paints body first, then header + footer chrome, then overlays on top.
func (p *VBox) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if p.Body != nil {
		p.Body.Draw(pnt, theme)
	}
	if p.Header != nil {
		p.Header.Draw(pnt, theme)
	}
	if p.Footer != nil {
		p.Footer.Draw(pnt, theme)
	}
	for _, o := range p.Overlays {
		o.Draw(pnt, theme)
	}
}

// OnEvent routes an event: an active drag stays with its captured child; a
// click goes to the topmost overlay that hit-tests, else the header / footer /
// body band it lands in (translated to child-local coordinates); any other
// event goes to the body.
func (p *VBox) OnEvent(ev toolkit.Event) {
	if p.dragTarget != nil && (ev.Kind == toolkit.EventMouseDrag || ev.Kind == toolkit.EventMouseUp) {
		child := ev
		child.X -= p.dragDx
		child.Y -= p.dragDy
		p.dragTarget.OnEvent(child)
		if ev.Kind == toolkit.EventMouseUp {
			p.dragTarget, p.dragDx, p.dragDy = nil, 0, 0
		}
		return
	}
	if ev.Kind == toolkit.EventClick {
		r := p.Bounds()
		// Overlays paint last, so hit-test them first (reverse order).
		for i := len(p.Overlays) - 1; i >= 0; i-- {
			o := p.Overlays[i]
			if !o.HitTest(ev.X, ev.Y) {
				continue
			}
			ob := o.Bounds()
			child := ev
			child.X -= ob.X
			child.Y -= ob.Y
			o.OnEvent(child)
			p.dragTarget, p.dragDx, p.dragDy = o, ob.X, ob.Y
			return
		}
		switch {
		case ev.Y < p.HeaderH:
			if p.Header != nil {
				p.Header.OnEvent(ev)
				p.dragTarget, p.dragDx, p.dragDy = p.Header, 0, 0
			}
		case ev.Y >= r.H-p.FooterH:
			if p.Footer != nil {
				child := ev
				child.Y -= r.H - p.FooterH
				p.Footer.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.Footer, 0, r.H-p.FooterH
			}
		default:
			if p.Body != nil {
				child := ev
				child.Y -= p.HeaderH
				p.Body.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.Body, 0, p.HeaderH
			}
		}
		return
	}
	if p.Body != nil {
		p.Body.OnEvent(ev)
	}
}
