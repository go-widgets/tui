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

// Notebook is a cell-native tabbed container: a 1-row tab strip on top, the
// active page's body below. Tabs size to their labels. A click on a tab swaps
// Active and fires OnTabChanged; every other event routes to the active page
// (translated below the strip).
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Notebook struct {
	toolkit.Base
	Tabs         []NotebookTab
	Active       int
	OnTabChanged func(idx int)
}

// notebookStripH is the tab-strip height in cells.
const notebookStripH = 1

// NewNotebook returns an empty Notebook (no tabs, Active = 0).
func NewNotebook() *Notebook { return &Notebook{} }

// AddTab appends a tab with the given label and page widget.
func (n *Notebook) AddTab(label string, page toolkit.Widget) {
	n.Tabs = append(n.Tabs, NotebookTab{Label: label, Page: page})
}

// tabRanges returns the cumulative start columns of the tabs (length
// len(Tabs)+1); tab i spans [xs[i], xs[i+1]). Each tab is its label plus a
// 1-cell pad on each side.
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

// Draw paints the tab strip (active tab in Accent) and the active page's body.
func (n *Notebook) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := n.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: notebookStripH}, theme.SurfaceAlt)
	xs := n.tabRanges()
	for i, tab := range n.Tabs {
		fill, ink := theme.SurfaceAlt, theme.OnSurface
		if i == n.Active {
			fill, ink = theme.Accent, theme.Background
		}
		pnt.FillRect(painter.Rect{X: r.X + xs[i], Y: r.Y, W: xs[i+1] - xs[i], H: notebookStripH}, fill)
		toolkit.DrawText(pnt, r.X+xs[i]+1, r.Y, tab.Label, ink)
	}
	if n.Active >= 0 && n.Active < len(n.Tabs) {
		if page := n.Tabs[n.Active].Page; page != nil {
			page.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + notebookStripH, W: r.W, H: r.H - notebookStripH})
			page.Draw(pnt, theme)
		}
	}
}

// OnEvent selects a tab on a strip click, else routes the event to the active
// page (translated below the strip).
func (n *Notebook) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventClick && ev.Y < notebookStripH {
		xs := n.tabRanges()
		for i := range n.Tabs {
			if ev.X >= xs[i] && ev.X < xs[i+1] {
				n.Active = i
				if n.OnTabChanged != nil {
					n.OnTabChanged(i)
				}
				break
			}
		}
		return
	}
	if n.Active >= 0 && n.Active < len(n.Tabs) {
		if page := n.Tabs[n.Active].Page; page != nil {
			child := ev
			child.Y -= notebookStripH
			page.OnEvent(child)
		}
	}
}
