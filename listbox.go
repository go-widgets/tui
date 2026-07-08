// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// ListBox is a cell-native single-selection list: one item per row, an
// accent-highlighted selection, arrow / Home / End / PageUp / PageDown
// navigation, click-to-select, and a viewport that scrolls to keep the
// selection visible. It is a toolkit.Widget, so like every widget here it
// renders through painter.Painter -- a terminal cell grid (TUI) or an RGBA
// pixel buffer (WUI/GUI) with the same code.
type ListBox struct {
	toolkit.Base
	Items    []string
	Selected int
	// OnSelect fires after Selected changes (keyboard or click). Optional; a
	// nil callback is a no-op. It receives the new index.
	OnSelect func(int)

	scrollY int
}

// NewListBox returns a ListBox over items with the first row selected.
func NewListBox(items []string) *ListBox { return &ListBox{Items: items} }

// setSelected clamps i into range and, when it changes the selection, updates
// it and fires OnSelect.
func (l *ListBox) setSelected(i int) {
	if len(l.Items) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i > len(l.Items)-1 {
		i = len(l.Items) - 1
	}
	if i == l.Selected {
		return
	}
	l.Selected = i
	if l.OnSelect != nil {
		l.OnSelect(i)
	}
}

// page is one viewport of rows (at least 1) for PageUp/PageDown.
func (l *ListBox) page() int {
	if h := l.Bounds().H; h > 1 {
		return h
	}
	return 1
}

// scrollToSel keeps the selected row inside the visible rows.
func (l *ListBox) scrollToSel() {
	h := l.Bounds().H
	if h <= 0 {
		return
	}
	if l.Selected < l.scrollY {
		l.scrollY = l.Selected
	} else if l.Selected >= l.scrollY+h {
		l.scrollY = l.Selected - h + 1
	}
}

// Draw paints the pane background, then the visible rows with the selected row
// highlighted in the theme accent.
func (l *ListBox) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := l.Bounds()
	l.scrollToSel()
	// toolkit.RGBA is an alias of painter.RGBA, so theme colours pass straight
	// through to the painter.
	p.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	for i := l.scrollY; i < len(l.Items); i++ {
		y := r.Y + (i - l.scrollY)
		if y >= r.Y+r.H {
			break
		}
		ink := theme.OnSurface
		if i == l.Selected {
			p.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Accent)
			ink = theme.Background
		}
		toolkit.DrawText(p, r.X+1, y, l.Items[i], ink)
	}
}

// OnEvent handles list navigation + click selection.
func (l *ListBox) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up":
			l.setSelected(l.Selected - 1)
		case "Down":
			l.setSelected(l.Selected + 1)
		case "Home":
			l.setSelected(0)
		case "End":
			l.setSelected(len(l.Items) - 1)
		case "PageUp":
			l.setSelected(l.Selected - l.page())
		case "PageDown":
			l.setSelected(l.Selected + l.page())
		}
	case toolkit.EventClick:
		i := ev.Y + l.scrollY // screen row -> item index
		if i >= 0 && i < len(l.Items) {
			l.setSelected(i)
		}
	}
}
