// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Expander is a cell-native collapsible section: a header row with a ▾/▸ chevron
// and a Title, and a Body widget shown in the rows below only while Expanded.
// Clicking the header (or Enter while focused) toggles it; clicks in the open
// body forward to Body. It is Focusable (chevron renders Accent when focused),
// so it fits a FocusRing.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Expander struct {
	toolkit.Base
	Title    string
	Body     toolkit.Widget
	Expanded bool
	OnToggle func(expanded bool)

	focused bool
}

// NewExpander builds a collapsed Expander with the given title and body.
func NewExpander(title string, body toolkit.Widget) *Expander {
	return &Expander{Title: title, Body: body}
}

// SetFocused implements Focusable.
func (e *Expander) SetFocused(v bool) { e.focused = v }

// SetBounds reserves the header row and lays the Body out below it.
func (e *Expander) SetBounds(r toolkit.Rect) {
	e.Base.SetBounds(r)
	if e.Body != nil {
		e.Body.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + 1, W: r.W, H: r.H - 1})
	}
}

// toggle flips Expanded and fires OnToggle.
func (e *Expander) toggle() {
	e.Expanded = !e.Expanded
	if e.OnToggle != nil {
		e.OnToggle(e.Expanded)
	}
}

// Draw paints the header (chevron + title) and, when Expanded, the Body.
func (e *Expander) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := e.Bounds()
	chev := "▸"
	if e.Expanded {
		chev = "▾"
	}
	chevInk := theme.OnSurface
	if e.focused {
		chevInk = theme.Accent
	}
	toolkit.DrawText(pnt, r.X, r.Y, chev, chevInk)
	toolkit.DrawText(pnt, r.X+2, r.Y, e.Title, theme.OnSurface)
	if e.Expanded && e.Body != nil {
		e.Body.Draw(pnt, theme)
	}
}

// OnEvent toggles on a header click / Enter, and forwards clicks in the open
// body to Body (translated below the header row).
func (e *Expander) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventClick:
		if ev.Y == 0 {
			e.toggle()
			return
		}
		if e.Expanded && e.Body != nil {
			child := ev
			child.Y--
			e.Body.OnEvent(child)
		}
	case toolkit.EventKeyDown:
		if ev.Code == "Enter" {
			e.toggle()
		}
	}
}
