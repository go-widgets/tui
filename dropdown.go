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

// Draw paints the control row and, when Open, the option list below it.
func (d *Dropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := d.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, theme.Surface)
	toolkit.DrawText(pnt, r.X+1, r.Y, d.value(), theme.OnSurface)
	ind := "▼"
	if d.Open {
		ind = "▲"
	}
	toolkit.DrawText(pnt, r.X+r.W-2, r.Y, ind, theme.Border)
	if !d.Open {
		return
	}
	for i, opt := range d.Options {
		y := r.Y + 1 + i
		if y >= r.Y+r.H {
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

// OnEvent toggles/navigates the list. Collapsed: Enter/Down/click opens. Open:
// Up/Down move the highlight, Enter/click selects, Escape closes.
func (d *Dropdown) OnEvent(ev toolkit.Event) {
	if !d.Open {
		switch ev.Kind {
		case toolkit.EventClick:
			if ev.Y == 0 {
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
		if ev.Y == 0 {
			d.Open = false
			return
		}
		if i := ev.Y - 1; i >= 0 && i < len(d.Options) {
			d.active = i
			d.selectActive()
		}
	}
}
