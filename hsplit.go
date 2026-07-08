// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// HSplit is a cell-native resizable horizontal split: Left takes LeftFrac
// percent of the width, a 1-cell grip column separates it from Right, and Right
// takes the remainder. The grip glyph doubles as a drag handle -- a click on it
// starts a drag session; subsequent EventMouseDrag events (routed here by a
// parent VBox's drag-capture, or any container that keeps forwarding a captured
// drag) update LeftFrac; EventMouseUp ends the session. LeftFrac is clamped to
// [HSplitMinFrac, HSplitMaxFrac] so neither pane can collapse to zero cells.
//
// Events are local (0-based within the split); bounds are absolute for Draw.
// Renders through painter.Painter, so the same split drives the cell (TUI) and
// RGBA (WUI/GUI) backends.
type HSplit struct {
	toolkit.Base
	Left, Right toolkit.Widget
	LeftFrac    int
	dragging    bool
}

const (
	// hSplitBarRune is the thin vertical char used for most of the separator
	// column; hSplitGripRune is the "heavy" vertical used for the centred grip
	// zone so the user has a visible handle hint on the resize bar.
	hSplitBarRune  = '│'
	hSplitGripRune = '┃'
	// HSplitMinFrac / HSplitMaxFrac bound LeftFrac so neither pane vanishes.
	HSplitMinFrac = 10
	HSplitMaxFrac = 90
)

// gripZone returns [y0, y1) of the grip-handle band: a centred strip ~1/6 of the
// bar's height, min 3 cells, max 7. The full bar is clickable -- the strip is
// just a visual affordance.
func gripZone(barH int) (int, int) {
	h := barH / 6
	if h < 3 {
		h = 3
	}
	if h > 7 {
		h = 7
	}
	if h > barH {
		h = barH
	}
	y0 := (barH - h) / 2
	return y0, y0 + h
}

// gripLocalX returns the local-X column the grip occupies given the current
// bounds and LeftFrac.
func (h *HSplit) gripLocalX() int {
	return h.Bounds().W * h.LeftFrac / 100
}

// SetBounds lays out the left and right panes, reserving one column for the grip.
func (h *HSplit) SetBounds(r toolkit.Rect) {
	h.Base.SetBounds(r)
	lw := r.W * h.LeftFrac / 100
	if h.Left != nil {
		h.Left.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: lw, H: r.H})
	}
	if h.Right != nil {
		// Right pane starts one column past the grip.
		h.Right.SetBounds(toolkit.Rect{X: r.X + lw + 1, Y: r.Y, W: r.W - lw - 1, H: r.H})
	}
}

// Draw paints both panes, then the grip column on top (Accent while dragging,
// Border otherwise) so the separator stays visible over either pane's bg.
func (h *HSplit) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if h.Left != nil {
		h.Left.Draw(pnt, theme)
	}
	if h.Right != nil {
		h.Right.Draw(pnt, theme)
	}
	r := h.Bounds()
	lw := h.gripLocalX()
	col := theme.Border
	if h.dragging {
		col = theme.Accent
	}
	// Fill the grip column with the surrounding pane bg first so the vertical
	// line doesn't inherit a stale character from an adjacent pane render.
	pnt.FillRect(painter.Rect{X: r.X + lw, Y: r.Y, W: 1, H: r.H}, theme.SurfaceAlt)
	// Thin bar for the whole column, with a heavy strip in the middle that
	// serves as a visible grip-handle affordance. Both regions are clickable.
	gy0, gy1 := gripZone(r.H)
	for y := 0; y < r.H; y++ {
		g := hSplitBarRune
		if y >= gy0 && y < gy1 {
			g = hSplitGripRune
		}
		toolkit.DrawText(pnt, r.X+lw, r.Y+y, string(g), col)
	}
}

// OnEvent handles resize drags and routes clicks/keys to the panes. A click on
// the grip starts a drag session; a click left/right of it goes to that pane
// (right pane translated to its local origin); non-mouse events go to Left.
func (h *HSplit) OnEvent(ev toolkit.Event) {
	// Active drag session: drag/up events (routed here by the parent's capture)
	// resize or terminate.
	if h.dragging {
		switch ev.Kind {
		case toolkit.EventMouseDrag:
			w := h.Bounds().W
			if w <= 0 {
				return
			}
			frac := ev.X * 100 / w
			if frac < HSplitMinFrac {
				frac = HSplitMinFrac
			}
			if frac > HSplitMaxFrac {
				frac = HSplitMaxFrac
			}
			h.LeftFrac = frac
			h.SetBounds(h.Bounds())
			return
		case toolkit.EventMouseUp:
			h.dragging = false
			return
		}
	}
	switch ev.Kind {
	case toolkit.EventClick:
		lw := h.gripLocalX()
		if ev.X == lw {
			// Click on the grip -> enter drag mode.
			h.dragging = true
			return
		}
		if ev.X < lw {
			if h.Left != nil {
				h.Left.OnEvent(ev)
			}
			return
		}
		if h.Right != nil {
			child := ev
			child.X -= lw + 1
			h.Right.OnEvent(child)
		}
	case toolkit.EventMouseDrag, toolkit.EventMouseUp:
		// Drag/up outside a session -- ignore.
	default:
		// Non-mouse events (arrow keys, chars) go to the left pane.
		if h.Left != nil {
			h.Left.OnEvent(ev)
		}
	}
}
