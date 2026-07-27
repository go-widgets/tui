// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Dropdown is a cell-native value picker (combobox): a one-row control showing
// the currently-selected Option and a ▼ indicator; when Open it lists the
// options below (within the widget's bounds) with the active one highlighted.
// Enter / Down / a click opens it; Up/Down move the highlight; Enter or a click
// selects (firing OnChange only on a real change) and closes; Escape closes
// without changing. Unlike MenuDropdown (a menu of one-shot actions), a Dropdown
// persists a chosen value — the core form select control.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Dropdown struct {
	toolkit.Base
	Options  []string
	Selected int
	Open     bool
	// OpenUp makes the option list appear ABOVE the control row instead of
	// below it — set it when the dropdown sits near the bottom edge so the
	// list has room. Mirrors toolkit.DropDown.OpenUp; the zero value (false)
	// keeps the original below-the-control layout.
	OpenUp   bool
	OnChange func(idx int, value string)

	active int // highlighted option while Open
}

// NewDropdown builds a Dropdown over options with the given initial selection
// (clamped into range).
func NewDropdown(options []string, selected int) *Dropdown {
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	return &Dropdown{Options: options, Selected: selected}
}

// value returns the selected option, or "" when the selection is out of range.
func (d *Dropdown) value() string {
	if d.Selected >= 0 && d.Selected < len(d.Options) {
		return d.Options[d.Selected]
	}
	return ""
}

// open expands the list, seeding the highlight at the current selection.
func (d *Dropdown) open() {
	d.Open = true
	d.active = d.Selected
	if d.active < 0 || d.active >= len(d.Options) {
		d.active = 0
	}
}

// selectActive commits the highlighted option (firing OnChange on a real change)
// and closes.
func (d *Dropdown) selectActive() {
	if d.active >= 0 && d.active < len(d.Options) && d.active != d.Selected {
		d.Selected = d.active
		if d.OnChange != nil {
			d.OnChange(d.Selected, d.value())
		}
	}
	d.Open = false
}

// controlY and dir return, respectively, the row the control occupies and the
// direction (+1 down, -1 up) the option list extends from it: below the
// control at the top of Bounds by default, or above the control at the
// bottom of Bounds when OpenUp is set.
func (d *Dropdown) controlY() int {
	r := d.Bounds()
	if d.OpenUp {
		return r.Y + r.H - 1
	}
	return r.Y
}

func (d *Dropdown) dir() int {
	if d.OpenUp {
		return -1
	}
	return 1
}

// Draw paints the control row and, when Open, the option list extending from
// it (below by default, above when OpenUp is set).
func (d *Dropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := d.Bounds()
	cy, dir := d.controlY(), d.dir()
	pnt.FillRect(painter.Rect{X: r.X, Y: cy, W: r.W, H: 1}, theme.Surface)
	toolkit.DrawText(pnt, r.X+1, cy, d.value(), theme.OnSurface)
	ind := "▼"
	if d.Open {
		ind = "▲"
	}
	toolkit.DrawText(pnt, r.X+r.W-2, cy, ind, theme.Border)
	if !d.Open {
		return
	}
	for i, opt := range d.Options {
		y := cy + dir*(1+i)
		if y < r.Y || y >= r.Y+r.H {
			break
		}
		ink := theme.OnSurface
		if i == d.active {
			pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.Accent)
			ink = theme.Background
		} else {
			pnt.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, theme.SurfaceAlt)
		}
		toolkit.DrawText(pnt, r.X+1, y, opt, ink)
	}
}

// controlLocalY is the control row's Y offset local to Bounds: 0 at the top
// by default, or the bottom row (H-1) when OpenUp is set.
func (d *Dropdown) controlLocalY() int {
	if d.OpenUp {
		return d.Bounds().H - 1
	}
	return 0
}

// OnEvent toggles/navigates the list. Collapsed: Enter/Down/click opens. Open:
// Up/Down move the highlight, Enter/click selects, Escape closes. Click
// coordinates are local to Bounds; the control row and option offsets follow
// controlLocalY/dir so hit-testing matches the OpenUp layout Draw produces.
func (d *Dropdown) OnEvent(ev toolkit.Event) {
	cly := d.controlLocalY()
	if !d.Open {
		switch ev.Kind {
		case toolkit.EventClick:
			if ev.Y == cly {
				d.open()
			}
		case toolkit.EventKeyDown:
			if ev.Code == "Enter" || ev.Code == "Down" || ev.Code == "ArrowDown" {
				d.open()
			}
		}
		return
	}
	switch ev.Kind {
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up", "ArrowUp":
			if d.active > 0 {
				d.active--
			}
		case "Down", "ArrowDown":
			if d.active < len(d.Options)-1 {
				d.active++
			}
		case "Enter":
			d.selectActive()
		case "Escape":
			d.Open = false
		}
	case toolkit.EventClick:
		if ev.Y == cly {
			d.Open = false
			return
		}
		if i := d.dir()*(ev.Y-cly) - 1; i >= 0 && i < len(d.Options) {
			d.active = i
			d.selectActive()
		}
	}
}
