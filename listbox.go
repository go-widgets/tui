// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"sort"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// ListBox is a cell-native single-selection list: one item per row, an
// accent-highlighted selection, arrow / Home / End / PageUp / PageDown
// navigation, click-to-select, and a viewport that scrolls to keep the
// selection visible. It is a toolkit.Widget, so like every widget here it
// renders through painter.Painter -- a terminal cell grid (TUI) or an RGBA
// pixel buffer (WUI/GUI) with the same code.
//
// Multi-selection: setting MultiSelect enables Ctrl/Shift-modified clicks
// that build a set of selected rows (see IsSelected / SelectedIndices /
// SetSelection / ClearSelection / ToggleSelect / SelectRange) -- mirrors
// toolkit.ListBox's MultiSelect. Selected remains the anchor/cursor row --
// the point a Shift-click range is measured from, and the row most recently
// clicked (plain or Ctrl). When MultiSelect is false (the default) none of
// this is reachable: Ctrl/Shift are ignored and only Selected is ever
// highlighted, exactly as before this feature existed.
type ListBox struct {
	toolkit.Base
	Items    []string
	Selected int
	// OnSelect fires after Selected changes (keyboard or click). Optional; a
	// nil callback is a no-op. It receives the new index.
	OnSelect func(int)
	// MultiSelect enables Ctrl/Shift multi-row selection (see the type doc).
	MultiSelect bool

	scrollY int

	// selected holds the multi-selection set. Only consulted for
	// rendering/queries when MultiSelect is true, but the mutator methods
	// work regardless of MultiSelect so callers can drive selection
	// programmatically before switching the widget into multi mode.
	selected map[int]bool
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
		hi := i == l.Selected
		if l.MultiSelect {
			hi = l.IsSelected(i)
		}
		if hi {
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
		if i < 0 || i >= len(l.Items) {
			return
		}
		if !l.MultiSelect {
			l.setSelected(i)
			return
		}
		// MultiSelect: a plain click selects only i (moving the anchor); a
		// Ctrl-click toggles i's membership (and moves the anchor); a
		// Shift-click selects the inclusive range from the current anchor
		// to i, leaving the anchor unchanged so successive Shift-clicks
		// keep extending/shrinking from the same origin. Mirrors
		// toolkit.ListBox.onClick.
		switch {
		case ev.Shift:
			l.SelectRange(l.Selected, i)
		case ev.Ctrl:
			l.ToggleSelect(i)
			l.Selected = i
		default:
			l.SetSelection(i)
			l.Selected = i
		}
		if l.OnSelect != nil {
			l.OnSelect(i)
		}
	}
}

// IsSelected reports whether row i is a member of the multi-selection set.
// Independent of MultiSelect + Selected, so it can be queried (and
// pre-seeded via SetSelection/ToggleSelect/SelectRange) even before
// multi-selection is switched on.
func (l *ListBox) IsSelected(i int) bool { return l.selected[i] }

// SelectedIndices returns the selected rows in ascending order. The
// returned slice is a fresh copy the caller may mutate freely.
func (l *ListBox) SelectedIndices() []int {
	out := make([]int, 0, len(l.selected))
	for i := range l.selected {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}

// SetSelection replaces the selection set with exactly the given indices.
// Indices outside [0, len(Items)) are silently dropped.
func (l *ListBox) SetSelection(indices ...int) {
	set := make(map[int]bool, len(indices))
	for _, i := range indices {
		if i < 0 || i >= len(l.Items) {
			continue
		}
		set[i] = true
	}
	l.selected = set
}

// ClearSelection empties the selection set. Selected (the anchor/cursor
// row) is left untouched.
func (l *ListBox) ClearSelection() { l.selected = nil }

// ToggleSelect flips row i's membership in the selection set. Out-of-range
// indices are a no-op.
func (l *ListBox) ToggleSelect(i int) {
	if i < 0 || i >= len(l.Items) {
		return
	}
	if l.selected == nil {
		l.selected = make(map[int]bool)
	}
	if l.selected[i] {
		delete(l.selected, i)
	} else {
		l.selected[i] = true
	}
}

// SelectRange selects the inclusive range of rows between a and b (either
// order accepted), replacing the current selection set. The range is
// clamped to [0, len(Items)); if the list is empty, or the clamped range is
// inverted, the resulting selection is empty.
func (l *ListBox) SelectRange(a, b int) {
	if a > b {
		a, b = b, a
	}
	if a < 0 {
		a = 0
	}
	if b >= len(l.Items) {
		b = len(l.Items) - 1
	}
	set := make(map[int]bool)
	for i := a; i <= b; i++ {
		set[i] = true
	}
	l.selected = set
}
