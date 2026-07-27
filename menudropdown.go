// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// MenuDropdown is an anchored, self-sizing dropdown menu -- the panel that
// opens under a MenuBar item. It positions itself at (AnchorX, AnchorY) and
// sizes to its content, ignoring any bounds a container hands it. A click on a
// Body row runs the matching ItemActions entry (nil / short slice = an
// informational row: the click still dismisses the menu but runs nothing) and
// hides the menu.
//
// Checkable, Checked and RadioGroup are parallel to Body and mirror
// toolkit.MenuItem's checkable/radio semantics (see toolkit.Menu): clicking a
// row whose Checkable is true flips its Checked entry (in addition to still
// running its ItemActions entry, if any) and the row renders a ✓ glyph in a
// left-hand gutter while Checked. A row whose RadioGroup is non-zero instead
// belongs to a mutually exclusive set -- clicking it sets its own Checked to
// true and clears Checked on every other Body row sharing the same
// RadioGroup value, rendering a • glyph instead of a check mark; RadioGroup
// implies checkable behaviour, so Checkable need not also be set. A short or
// nil slice means "not checkable" / "no radio group" for the missing rows.
// The gutter is only reserved (shifting every row right) when at least one
// row is checkable or radio-grouped; a MenuDropdown with no such rows lays
// out exactly as before this feature existed.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type MenuDropdown struct {
	toolkit.Base
	Title       string
	Body        []string
	ItemActions []func() // parallel to Body; nil / short = informational row
	Checkable   []bool   // parallel to Body; short/nil = not checkable
	Checked     []bool   // parallel to Body; short/nil = unchecked
	RadioGroup  []int    // parallel to Body; short/nil or 0 = no radio group
	Visible     bool
	AnchorX     int
	AnchorY     int
}

// isCheckable reports whether Body row i wants a check/bullet glyph slot --
// either explicitly Checkable or a member of a radio group (RadioGroup
// implies checkable).
func (d *MenuDropdown) isCheckable(i int) bool {
	return (i < len(d.Checkable) && d.Checkable[i]) || d.radioGroup(i) != 0
}

// radioGroup returns Body row i's RadioGroup value, or 0 (no group) if the
// slice is short.
func (d *MenuDropdown) radioGroup(i int) int {
	if i < len(d.RadioGroup) {
		return d.RadioGroup[i]
	}
	return 0
}

// isChecked returns Body row i's Checked value, or false if the slice is
// short.
func (d *MenuDropdown) isChecked(i int) bool {
	return i < len(d.Checked) && d.Checked[i]
}

// setChecked grows d.Checked as needed and sets row i's Checked value.
func (d *MenuDropdown) setChecked(i int, v bool) {
	for len(d.Checked) <= i {
		d.Checked = append(d.Checked, false)
	}
	d.Checked[i] = v
}

// selectRadio sets Body row idx's Checked to true and clears Checked on
// every other row sharing idx's RadioGroup.
func (d *MenuDropdown) selectRadio(idx int) {
	group := d.radioGroup(idx)
	for i := range d.Body {
		if d.radioGroup(i) == group {
			d.setChecked(i, i == idx)
		}
	}
}

// hasCheckGutter reports whether any Body row wants a check/bullet glyph,
// i.e. whether Draw/size must reserve the 2-cell gutter before the label.
func (d *MenuDropdown) hasCheckGutter() bool {
	for i := range d.Body {
		if d.isCheckable(i) {
			return true
		}
	}
	return false
}

// size returns the natural (width, height) in cells: the widest of Title / Body
// plus padding (and the check gutter, when any Body row is checkable), and a
// row per Body line (minimum height 3).
func (d *MenuDropdown) size() (int, int) {
	w := utf8.RuneCountInString(d.Title)
	for _, line := range d.Body {
		if l := utf8.RuneCountInString(line); l > w {
			w = l
		}
	}
	if d.hasCheckGutter() {
		w += 2 // "✓ " / "• " gutter reserved before each Body row's label
	}
	w += 4 // 1-cell border + 1-cell text pad on each side
	h := 2 + len(d.Body)
	if h < 3 {
		h = 3
	}
	return w, h
}

// SetBounds ignores the requested rect and self-positions at the anchor with
// the natural size (a container's layout pass calls this; the dropdown opts out
// of the normal flow).
func (d *MenuDropdown) SetBounds(_ toolkit.Rect) {
	w, h := d.size()
	d.Base.SetBounds(toolkit.Rect{X: d.AnchorX, Y: d.AnchorY, W: w, H: h})
}

// HitTest — hidden dropdowns claim no clicks.
func (d *MenuDropdown) HitTest(px, py int) bool {
	if !d.Visible {
		return false
	}
	return d.Base.HitTest(px, py)
}

// Draw paints the anchored box (when Visible): Title on the top row, Body rows
// below.
func (d *MenuDropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !d.Visible {
		return
	}
	d.SetBounds(toolkit.Rect{}) // refresh the anchored geometry
	r := d.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.SurfaceAlt)
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.Border, 1)
	if d.Title != "" {
		toolkit.DrawText(pnt, r.X+2, r.Y, d.Title, theme.OnSurface)
	}
	gutter := 0
	if d.hasCheckGutter() {
		gutter = 2
	}
	for i, line := range d.Body {
		y := r.Y + 1 + i
		if d.isCheckable(i) && d.isChecked(i) {
			glyph := "✓"
			if d.radioGroup(i) != 0 {
				glyph = "•"
			}
			toolkit.DrawText(pnt, r.X+2, y, glyph, theme.OnSurface)
		}
		toolkit.DrawText(pnt, r.X+2+gutter, y, line, theme.OnSurface)
	}
}

// OnEvent runs the clicked Body row's checkable/radio toggle (if any) and
// action (if any), and dismisses the menu.
func (d *MenuDropdown) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	idx := ev.Y - 1 // Body rows start at local Y=1 (Title on 0)
	if idx >= 0 && idx < len(d.Body) {
		switch {
		case d.radioGroup(idx) != 0:
			d.selectRadio(idx)
		case idx < len(d.Checkable) && d.Checkable[idx]:
			d.setChecked(idx, !d.isChecked(idx))
		}
	}
	if idx >= 0 && idx < len(d.ItemActions) && d.ItemActions[idx] != nil {
		d.ItemActions[idx]()
	}
	d.Visible = false
}
