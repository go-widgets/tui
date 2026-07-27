// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"sort"
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
	// Align controls horizontal placement of BOTH the header Title and every
	// body cell in this column. The zero value (AlignLeft) keeps the
	// original left-justified behaviour. Reuses toolkit.Align (aliased as
	// Align in label.go) so the pixel and cell Tables share one vocabulary.
	Align Align
}

// Table is a cell-native data grid: a header row on top, then one body row per
// Rows entry with zebra striping and an accent-highlighted selection. Auto
// columns reflow to the widget width. Arrow / Home / End / PageUp / PageDown
// navigate and a click selects a body row; the viewport scrolls to keep the
// selection visible.
//
// Multi-selection: setting MultiSelect enables Ctrl/Shift-modified clicks
// that build a set of selected rows (see IsSelected / SelectedIndices /
// SetSelection / ClearSelection / ToggleSelect / SelectRange) -- mirrors
// toolkit.ListBox's MultiSelect. Selected remains the anchor/cursor row.
// When MultiSelect is false (the default) Ctrl/Shift are ignored and only
// Selected is ever highlighted, exactly as before this feature existed.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Table struct {
	toolkit.Base
	Columns  []TableColumn
	Rows     [][]string
	Selected int // -1 = no selection
	OnSelect func(row int)
	// MultiSelect enables Ctrl/Shift multi-row selection (see the type doc).
	MultiSelect bool

	scrollY int

	// selected holds the multi-selection set; see ListBox.selected.
	selected map[int]bool
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

// cellTextX returns the column where a cell's text starts within [cellX,
// cellX+cellW), honoring align: AlignLeft (the default) pads 1 cell from the
// left; AlignRight pads 1 cell from the right; AlignCenter centers the text.
// All three clamp to the left pad when the text is too wide to fit, so long
// content never draws left of the cell.
func cellTextX(cellX, cellW int, text string, align Align) int {
	n := utf8.RuneCountInString(text)
	var x int
	switch align {
	case AlignRight:
		x = cellX + cellW - 1 - n
	case AlignCenter:
		x = cellX + (cellW-n)/2
	default: // AlignLeft
		x = cellX + 1
	}
	if x < cellX+1 {
		x = cellX + 1
	}
	return x
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

// scrollToSel keeps the selected row inside the visible body rows. With no
// selection (Selected < 0, the default) there is nothing to scroll to, so it
// leaves scrollY untouched — otherwise scrollY would follow Selected to -1 and
// Draw would index Rows[-1].
func (t *Table) scrollToSel() {
	if t.Selected < 0 {
		return
	}
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
		toolkit.DrawText(pnt, cellTextX(hx, widths[i], col.Title, col.Align), r.Y, col.Title, theme.OnSurface)
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
			hi := i == t.Selected
			if t.MultiSelect {
				hi = t.IsSelected(i)
			}
			switch {
			case hi:
				pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Accent)
				ink = theme.Background
			case i%2 == 1:
				pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Surface)
			}
			cx := r.X
			for j, col := range t.Columns {
				if j < len(t.Rows[i]) {
					toolkit.DrawText(pnt, cellTextX(cx, widths[j], t.Rows[i][j], col.Align), y, t.Rows[i][j], ink)
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
		if i < 0 || i >= len(t.Rows) {
			return
		}
		if !t.MultiSelect {
			t.setSelected(i)
			return
		}
		// MultiSelect: mirrors ListBox's / toolkit.ListBox's onClick.
		switch {
		case ev.Shift:
			t.SelectRange(t.Selected, i)
		case ev.Ctrl:
			t.ToggleSelect(i)
			t.Selected = i
		default:
			t.SetSelection(i)
			t.Selected = i
		}
		if t.OnSelect != nil {
			t.OnSelect(i)
		}
	}
}

// IsSelected reports whether row i is a member of the multi-selection set.
// Independent of MultiSelect + Selected, so it can be queried (and
// pre-seeded via SetSelection/ToggleSelect/SelectRange) even before
// multi-selection is switched on.
func (t *Table) IsSelected(i int) bool { return t.selected[i] }

// SelectedIndices returns the selected rows in ascending order. The
// returned slice is a fresh copy the caller may mutate freely.
func (t *Table) SelectedIndices() []int {
	out := make([]int, 0, len(t.selected))
	for i := range t.selected {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}

// SetSelection replaces the selection set with exactly the given indices.
// Indices outside [0, len(Rows)) are silently dropped.
func (t *Table) SetSelection(indices ...int) {
	set := make(map[int]bool, len(indices))
	for _, i := range indices {
		if i < 0 || i >= len(t.Rows) {
			continue
		}
		set[i] = true
	}
	t.selected = set
}

// ClearSelection empties the selection set. Selected (the anchor/cursor
// row) is left untouched.
func (t *Table) ClearSelection() { t.selected = nil }

// ToggleSelect flips row i's membership in the selection set. Out-of-range
// indices are a no-op.
func (t *Table) ToggleSelect(i int) {
	if i < 0 || i >= len(t.Rows) {
		return
	}
	if t.selected == nil {
		t.selected = make(map[int]bool)
	}
	if t.selected[i] {
		delete(t.selected, i)
	} else {
		t.selected[i] = true
	}
}

// SelectRange selects the inclusive range of rows between a and b (either
// order accepted), replacing the current selection set. The range is
// clamped to [0, len(Rows)); if the table is empty, or the clamped range is
// inverted, the resulting selection is empty.
func (t *Table) SelectRange(a, b int) {
	if a > b {
		a, b = b, a
	}
	if a < 0 {
		a = 0
	}
	if b >= len(t.Rows) {
		b = len(t.Rows) - 1
	}
	set := make(map[int]bool)
	for i := a; i <= b; i++ {
		set[i] = true
	}
	t.selected = set
}
