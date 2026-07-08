// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// TableColumn is one column: a header Title and an optional fixed Width in
// cells. A Width of 0 marks the column "auto" — it claims an equal share of the
// cells left after the fixed columns.
type TableColumn struct {
	Title string
	Width int // cells; 0 = auto (equal share of the remainder)
}

// Table is a cell-native data grid: a header row on top, then one body row per
// Rows entry with zebra striping and an accent-highlighted selection. Auto
// columns reflow to the widget width. Arrow / Home / End / PageUp / PageDown
// navigate and a click selects a body row; the viewport scrolls to keep the
// selection visible.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Table struct {
	toolkit.Base
	Columns  []TableColumn
	Rows     [][]string
	Selected int // -1 = no selection
	OnSelect func(row int)

	scrollY int
}

// tableEmptyPlaceholder is the label rendered under the header when Rows is
// empty.
const tableEmptyPlaceholder = "(no data)"

// NewTable builds a Table with the given columns and rows, no row selected.
func NewTable(cols []TableColumn, rows [][]string) *Table {
	return &Table{Columns: cols, Rows: rows, Selected: -1}
}

// columnWidths distributes total cells across the columns: fixed columns take
// their Width, auto columns split the remainder equally with the integer
// leftover pushed onto the last auto column.
func (t *Table) columnWidths(total int) []int {
	n := len(t.Columns)
	if n == 0 {
		return nil
	}
	widths := make([]int, n)
	fixedTotal, autoCount, lastAuto := 0, 0, -1
	for i, col := range t.Columns {
		if col.Width > 0 {
			widths[i] = col.Width
			fixedTotal += col.Width
		} else {
			autoCount++
			lastAuto = i
		}
	}
	if autoCount == 0 {
		return widths
	}
	remaining := total - fixedTotal
	if remaining < 0 {
		remaining = 0
	}
	share := remaining / autoCount
	sum := fixedTotal
	for i, col := range t.Columns {
		if col.Width <= 0 {
			widths[i] = share
			sum += share
		}
	}
	widths[lastAuto] += total - sum
	if widths[lastAuto] < 0 {
		widths[lastAuto] = 0
	}
	return widths
}

// setSelected clamps row into range and, when it changes, updates Selected and
// fires OnSelect.
func (t *Table) setSelected(row int) {
	if len(t.Rows) == 0 {
		return
	}
	if row < 0 {
		row = 0
	}
	if row > len(t.Rows)-1 {
		row = len(t.Rows) - 1
	}
	if row == t.Selected {
		return
	}
	t.Selected = row
	if t.OnSelect != nil {
		t.OnSelect(row)
	}
}

// bodyH is the number of body rows visible below the 1-row header.
func (t *Table) bodyH() int {
	if h := t.Bounds().H - 1; h > 0 {
		return h
	}
	return 0
}

// page is one viewport of body rows (at least 1) for PageUp/PageDown.
func (t *Table) page() int {
	if h := t.bodyH(); h > 0 {
		return h
	}
	return 1
}

// scrollToSel keeps the selected row inside the visible body rows.
func (t *Table) scrollToSel() {
	h := t.bodyH()
	if h <= 0 {
		return
	}
	if t.Selected < t.scrollY {
		t.scrollY = t.Selected
	} else if t.Selected >= t.scrollY+h {
		t.scrollY = t.Selected - h + 1
	}
}

// Draw paints the header, the visible body rows, and the column separators.
func (t *Table) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := t.Bounds()
	if r.W <= 0 || r.H <= 0 {
		return
	}
	widths := t.columnWidths(r.W)
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)

	// Header titles.
	hx := r.X
	for i, col := range t.Columns {
		toolkit.DrawText(pnt, hx+1, r.Y, col.Title, theme.OnSurface)
		hx += widths[i]
	}

	bodyY := r.Y + 1
	if len(t.Rows) == 0 {
		tw := utf8.RuneCountInString(tableEmptyPlaceholder)
		toolkit.DrawText(pnt, r.X+(r.W-tw)/2, bodyY, tableEmptyPlaceholder, theme.OnSurface)
	} else {
		t.scrollToSel()
		for i := t.scrollY; i < len(t.Rows); i++ {
			y := bodyY + (i - t.scrollY)
			if y >= r.Y+r.H {
				break
			}
			ink := theme.OnSurface
			switch {
			case i == t.Selected:
				pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Accent)
				ink = theme.Background
			case i%2 == 1:
				pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Surface)
			}
			cx := r.X
			for j := range t.Columns {
				if j < len(t.Rows[i]) {
					toolkit.DrawText(pnt, cx+1, y, t.Rows[i][j], ink)
				}
				cx += widths[j]
			}
		}
	}

	// Column separators between adjacent columns, full height.
	sepX := r.X
	for i := 0; i < len(t.Columns)-1; i++ {
		sepX += widths[i]
		for y := r.Y; y < r.Y+r.H; y++ {
			toolkit.DrawText(pnt, sepX, y, "│", theme.Border)
		}
	}
}

// OnEvent handles row navigation and click selection (the header row is inert).
func (t *Table) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up":
			t.setSelected(t.Selected - 1)
		case "Down":
			t.setSelected(t.Selected + 1)
		case "Home":
			t.setSelected(0)
		case "End":
			t.setSelected(len(t.Rows) - 1)
		case "PageUp":
			t.setSelected(t.Selected - t.page())
		case "PageDown":
			t.setSelected(t.Selected + t.page())
		}
	case toolkit.EventClick:
		if ev.Y < 1 {
			return // header row
		}
		i := ev.Y - 1 + t.scrollY
		if i >= 0 && i < len(t.Rows) {
			t.setSelected(i)
		}
	}
}
