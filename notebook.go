// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// NotebookTab is one entry in a Notebook: a Label painted on the tab and the
// Page widget shown in the body when that tab is active.
type NotebookTab struct {
	Label string
	Page  toolkit.Widget
}

// Notebook is a cell-native tabbed container. A tab strip on the side chosen by
// TabSide (top by default) hosts the tabs; the rest is the active page's body.
// For Top/Bottom the strip is one row tall and tabs size to their labels; for
// Left/Right the strip is a column (widest label + padding) and each tab takes
// one row. A click on a tab swaps Active and fires OnTabChanged; every other
// event routes to the active page (translated into the body).
//
// TabSide reuses toolkit.TabSide so the pixel and cell Notebooks share one
// vocabulary (toolkit.TabTop / TabBottom / TabLeft / TabRight).
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Notebook struct {
	toolkit.Base
	Tabs         []NotebookTab
	Active       int
	TabSide      toolkit.TabSide
	OnTabChanged func(idx int)
}

// notebookStripH is the horizontal (Top/Bottom) tab-strip height in cells.
const notebookStripH = 1

// NewNotebook returns an empty Notebook (no tabs, Active = 0).
func NewNotebook() *Notebook { return &Notebook{} }

// AddTab appends a tab with the given label and page widget.
func (n *Notebook) AddTab(label string, page toolkit.Widget) {
	n.Tabs = append(n.Tabs, NotebookTab{Label: label, Page: page})
}

// horizontal reports whether the strip runs along the top or bottom edge.
func (n *Notebook) horizontal() bool {
	return n.TabSide == toolkit.TabTop || n.TabSide == toolkit.TabBottom
}

// tabRanges returns the cumulative start columns of the tabs for a horizontal
// strip (length len(Tabs)+1); tab i spans [xs[i], xs[i+1]). Each tab is its
// label plus a 1-cell pad on each side.
func (n *Notebook) tabRanges() []int {
	xs := make([]int, len(n.Tabs)+1)
	x := 0
	for i, t := range n.Tabs {
		xs[i] = x
		x += utf8.RuneCountInString(t.Label) + 2
	}
	xs[len(n.Tabs)] = x
	return xs
}

// stripW is the column width of a vertical (Left/Right) strip: the widest label
// plus a 1-cell pad on each side (minimum 1).
func (n *Notebook) stripW() int {
	w := 1
	for _, t := range n.Tabs {
		if l := utf8.RuneCountInString(t.Label) + 2; l > w {
			w = l
		}
	}
	return w
}

// tabRect is the i-th tab's rect (surface coordinates).
func (n *Notebook) tabRect(i int) toolkit.Rect {
	r := n.Bounds()
	if n.horizontal() {
		xs := n.tabRanges()
		y := r.Y
		if n.TabSide == toolkit.TabBottom {
			y = r.Y + r.H - notebookStripH
		}
		return toolkit.Rect{X: r.X + xs[i], Y: y, W: xs[i+1] - xs[i], H: notebookStripH}
	}
	sw := n.stripW()
	x := r.X
	if n.TabSide == toolkit.TabRight {
		x = r.X + r.W - sw
	}
	return toolkit.Rect{X: x, Y: r.Y + i, W: sw, H: 1}
}

// strip is the full tab-strip band on the chosen side.
func (n *Notebook) strip() toolkit.Rect {
	r := n.Bounds()
	switch n.TabSide {
	case toolkit.TabBottom:
		return toolkit.Rect{X: r.X, Y: r.Y + r.H - notebookStripH, W: r.W, H: notebookStripH}
	case toolkit.TabLeft:
		return toolkit.Rect{X: r.X, Y: r.Y, W: n.stripW(), H: r.H}
	case toolkit.TabRight:
		sw := n.stripW()
		return toolkit.Rect{X: r.X + r.W - sw, Y: r.Y, W: sw, H: r.H}
	default: // TabTop
		return toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: notebookStripH}
	}
}

// bodyRect is the page area — the bounds minus the strip band.
func (n *Notebook) bodyRect() toolkit.Rect {
	r := n.Bounds()
	switch n.TabSide {
	case toolkit.TabBottom:
		return toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H - notebookStripH}
	case toolkit.TabLeft:
		sw := n.stripW()
		return toolkit.Rect{X: r.X + sw, Y: r.Y, W: r.W - sw, H: r.H}
	case toolkit.TabRight:
		return toolkit.Rect{X: r.X, Y: r.Y, W: r.W - n.stripW(), H: r.H}
	default: // TabTop
		return toolkit.Rect{X: r.X, Y: r.Y + notebookStripH, W: r.W, H: r.H - notebookStripH}
	}
}

// Draw paints the tab strip (active tab in Accent) and the active page's body.
func (n *Notebook) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	s := n.strip()
	pnt.FillRect(painter.Rect{X: s.X, Y: s.Y, W: s.W, H: s.H}, theme.SurfaceAlt)
	for i, tab := range n.Tabs {
		fill, ink := theme.SurfaceAlt, theme.OnSurface
		if i == n.Active {
			fill, ink = theme.Accent, theme.Background
		}
		tr := n.tabRect(i)
		pnt.FillRect(painter.Rect{X: tr.X, Y: tr.Y, W: tr.W, H: tr.H}, fill)
		toolkit.DrawText(pnt, tr.X+1, tr.Y, tab.Label, ink)
	}
	if n.Active >= 0 && n.Active < len(n.Tabs) {
		if page := n.Tabs[n.Active].Page; page != nil {
			page.SetBounds(n.bodyRect())
			page.Draw(pnt, theme)
		}
	}
}

// OnEvent selects a tab on a strip click, else routes the event to the active
// page (translated into the body's local frame).
func (n *Notebook) OnEvent(ev toolkit.Event) {
	r := n.Bounds()
	if ev.Kind == toolkit.EventClick {
		ax, ay := ev.X+r.X, ev.Y+r.Y // reconstruct surface coords for hit-testing
		for i := range n.Tabs {
			if n.tabRect(i).Contains(ax, ay) {
				n.Active = i
				if n.OnTabChanged != nil {
					n.OnTabChanged(i)
				}
				return
			}
		}
		if !n.bodyRect().Contains(ax, ay) {
			return // click in empty strip space
		}
	}
	if n.Active >= 0 && n.Active < len(n.Tabs) {
		if page := n.Tabs[n.Active].Page; page != nil {
			body := n.bodyRect()
			child := ev
			child.X = ev.X + r.X - body.X
			child.Y = ev.Y + r.Y - body.Y
			page.OnEvent(child)
		}
	}
}
