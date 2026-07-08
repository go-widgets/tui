// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// TreeNode is one entry in a TreeView. Children nest arbitrarily deep; Expanded
// controls whether they render. Data is anything the host wants to hang off the
// node (a path, an id, the model object) — the widget never reads it.
type TreeNode struct {
	Label    string
	Expanded bool
	Children []*TreeNode
	Data     any
}

// TreeView renders a hierarchical TreeNode set as indented cell rows: a ▼/▶
// chevron on nodes with children, then the label. A click on the chevron
// toggles Expanded; a click elsewhere selects the row and fires OnActivate. The
// arrow keys navigate (Up/Down move the selection, Left collapses, Right
// expands) and Enter activates the selection. Long trees scroll to keep the
// selection visible.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type TreeView struct {
	toolkit.Base
	Root       *TreeNode
	Selected   *TreeNode
	OnActivate func(node *TreeNode)

	rows    []treeRow // transient flat view, rebuilt every Draw / OnEvent
	scrollY int
}

type treeRow struct {
	node  *TreeNode
	depth int
}

// treeIndentW is the per-depth indent in cells; the chevron/label sit at
// depth*treeIndentW, the label two cells further right.
const treeIndentW = 2

// NewTreeView builds a TreeView rooted at root (nil for an empty view).
func NewTreeView(root *TreeNode) *TreeView { return &TreeView{Root: root} }

// flatten rebuilds rows by walking Root depth-first, skipping the children of
// collapsed nodes.
func (t *TreeView) flatten() {
	t.rows = t.rows[:0]
	if t.Root != nil {
		t.walk(t.Root, 0)
	}
}

func (t *TreeView) walk(n *TreeNode, depth int) {
	t.rows = append(t.rows, treeRow{n, depth})
	if !n.Expanded {
		return
	}
	for _, c := range n.Children {
		t.walk(c, depth+1)
	}
}

// indexOfSelected returns the row index of Selected, or -1.
func (t *TreeView) indexOfSelected() int {
	for i, row := range t.rows {
		if row.node == t.Selected {
			return i
		}
	}
	return -1
}

// scrollToSel keeps the selected row within the viewport.
func (t *TreeView) scrollToSel() {
	h := t.Bounds().H
	if h < 1 {
		h = 1
	}
	i := t.indexOfSelected()
	if i < 0 {
		return
	}
	if i < t.scrollY {
		t.scrollY = i
	} else if i >= t.scrollY+h {
		t.scrollY = i - h + 1
	}
}

// Draw paints the visible rows (chevron + indented label), highlighting the
// selection.
func (t *TreeView) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	t.flatten()
	t.scrollToSel()
	r := t.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	for i := t.scrollY; i < len(t.rows); i++ {
		y := r.Y + (i - t.scrollY)
		if y >= r.Y+r.H {
			break
		}
		row := t.rows[i]
		ink := theme.OnSurface
		if row.node == t.Selected {
			pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Accent)
			ink = theme.Background
		}
		x := r.X + row.depth*treeIndentW
		if len(row.node.Children) > 0 {
			chev := "▶"
			if row.node.Expanded {
				chev = "▼"
			}
			toolkit.DrawText(pnt, x, y, chev, ink)
		}
		toolkit.DrawText(pnt, x+treeIndentW, y, row.node.Label, ink)
	}
}

// moveSel shifts the selection by delta rows, seeding it at the top when nothing
// is selected yet.
func (t *TreeView) moveSel(delta int) {
	if len(t.rows) == 0 {
		return
	}
	i := t.indexOfSelected()
	if i < 0 {
		t.Selected = t.rows[0].node
		return
	}
	j := i + delta
	if j < 0 || j >= len(t.rows) {
		return
	}
	t.Selected = t.rows[j].node
}

// OnEvent handles click (chevron toggle / row select) and keyboard navigation.
func (t *TreeView) OnEvent(ev toolkit.Event) {
	t.flatten()
	switch ev.Kind {
	case toolkit.EventClick:
		if ev.Y < 0 {
			return
		}
		idx := ev.Y + t.scrollY
		if idx >= len(t.rows) {
			return
		}
		row := t.rows[idx]
		chevX := row.depth * treeIndentW
		if len(row.node.Children) > 0 && ev.X >= chevX && ev.X < chevX+treeIndentW {
			row.node.Expanded = !row.node.Expanded
			return
		}
		t.Selected = row.node
		if t.OnActivate != nil {
			t.OnActivate(row.node)
		}
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up":
			t.moveSel(-1)
		case "Down":
			t.moveSel(1)
		case "Left":
			if t.Selected != nil && len(t.Selected.Children) > 0 && t.Selected.Expanded {
				t.Selected.Expanded = false
			}
		case "Right":
			if t.Selected != nil && len(t.Selected.Children) > 0 && !t.Selected.Expanded {
				t.Selected.Expanded = true
			}
		case "Enter":
			if t.Selected != nil && t.OnActivate != nil {
				t.OnActivate(t.Selected)
			}
		}
	}
}
