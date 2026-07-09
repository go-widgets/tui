// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// VSplit is the vertical counterpart of HSplit: Top takes TopFrac percent of the
// height, a 1-row grip separates it from Bottom, and Bottom takes the rest. The
// grip row doubles as a drag handle -- a click on it starts a drag session;
// subsequent EventMouseDrag events (forwarded by a parent's capture) update
// TopFrac; EventMouseUp ends it. TopFrac is clamped to [VSplitMinFrac,
// VSplitMaxFrac] so neither pane collapses. Useful for editor-over-output or
// list-over-detail layouts.
//
// Events are local (0-based within the split); bounds are absolute for Draw.
// Renders through painter.Painter (cell grid / RGBA buffer).
type VSplit struct {
	toolkit.Base
	Top, Bottom toolkit.Widget
	TopFrac     int
	dragging    bool
}

const (
	// vSplitBarRune is the thin horizontal char for most of the separator row;
	// vSplitGripRune is the "heavy" horizontal for the centred grip strip.
	vSplitBarRune  = '─'
	vSplitGripRune = '━'
	// VSplitMinFrac / VSplitMaxFrac bound TopFrac so neither pane vanishes.
	VSplitMinFrac = 10
	VSplitMaxFrac = 90
)

// gripLocalY returns the local-Y row the grip occupies given the current bounds
// and TopFrac.
func (v *VSplit) gripLocalY() int {
	return v.Bounds().H * v.TopFrac / 100
}

// SetBounds lays out the top and bottom panes, reserving one row for the grip.
func (v *VSplit) SetBounds(r toolkit.Rect) {
	v.Base.SetBounds(r)
	th := r.H * v.TopFrac / 100
	if v.Top != nil {
		v.Top.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: th})
	}
	if v.Bottom != nil {
		// Bottom pane starts one row past the grip.
		v.Bottom.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + th + 1, W: r.W, H: r.H - th - 1})
	}
}

// Draw paints both panes, then the grip row on top (Accent while dragging,
// Border otherwise) so the separator stays visible over either pane's bg.
func (v *VSplit) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if v.Top != nil {
		v.Top.Draw(pnt, theme)
	}
	if v.Bottom != nil {
		v.Bottom.Draw(pnt, theme)
	}
	r := v.Bounds()
	th := v.gripLocalY()
	col := theme.Border
	if v.dragging {
		col = theme.Accent
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y + th, W: r.W, H: 1}, theme.SurfaceAlt)
	// Thin bar across the whole row, with a heavy strip in the middle as a
	// visible grip-handle affordance. Both regions are clickable.
	gx0, gx1 := gripZone(r.W)
	for x := 0; x < r.W; x++ {
		g := vSplitBarRune
		if x >= gx0 && x < gx1 {
			g = vSplitGripRune
		}
		toolkit.DrawText(pnt, r.X+x, r.Y+th, string(g), col)
	}
}

// OnEvent handles resize drags and routes clicks/keys to the panes. A click on
// the grip starts a drag session; a click above/below it goes to that pane
// (bottom pane translated to its local origin); non-mouse events go to Top.
func (v *VSplit) OnEvent(ev toolkit.Event) {
	if v.dragging {
		switch ev.Kind {
		case toolkit.EventMouseDrag:
			h := v.Bounds().H
			if h <= 0 {
				return
			}
			frac := ev.Y * 100 / h
			if frac < VSplitMinFrac {
				frac = VSplitMinFrac
			}
			if frac > VSplitMaxFrac {
				frac = VSplitMaxFrac
			}
			v.TopFrac = frac
			v.SetBounds(v.Bounds())
			return
		case toolkit.EventMouseUp:
			v.dragging = false
			return
		}
	}
	switch ev.Kind {
	case toolkit.EventClick:
		th := v.gripLocalY()
		if ev.Y == th {
			v.dragging = true
			return
		}
		if ev.Y < th {
			if v.Top != nil {
				v.Top.OnEvent(ev)
			}
			return
		}
		if v.Bottom != nil {
			child := ev
			child.Y -= th + 1
			v.Bottom.OnEvent(child)
		}
	case toolkit.EventMouseDrag, toolkit.EventMouseUp:
		// Drag/up outside a session -- ignore.
	default:
		if v.Top != nil {
			v.Top.OnEvent(ev)
		}
	}
}
