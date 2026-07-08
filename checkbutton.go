// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// CheckButton is a cell-native boolean toggle: a "[✓]" / "[ ]" box followed by
// a Label. A click anywhere on the row flips Checked and fires OnToggle. The box
// paints in Accent when checked so the state reads at a glance.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type CheckButton struct {
	toolkit.Base
	Label    string
	Checked  bool
	OnToggle func(checked bool)
}

// checkBoxCells is the width of the "[x] " box prefix, including its trailing
// space, so the label starts at r.X + checkBoxCells.
const checkBoxCells = 4

// NewCheckButton constructs a CheckButton with the given label and initial
// checked state.
func NewCheckButton(label string, checked bool) *CheckButton {
	return &CheckButton{Label: label, Checked: checked}
}

// Draw paints the box (Accent when checked, Border otherwise) and the label.
func (c *CheckButton) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := c.Bounds()
	box, ink := "[ ]", theme.Border
	if c.Checked {
		box, ink = "[✓]", theme.Accent
	}
	toolkit.DrawText(pnt, r.X, r.Y, box, ink)
	if c.Label != "" {
		toolkit.DrawText(pnt, r.X+checkBoxCells, r.Y, c.Label, theme.OnSurface)
	}
}

// OnEvent flips Checked and fires OnToggle on a click; other events are ignored.
func (c *CheckButton) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	c.Checked = !c.Checked
	if c.OnToggle != nil {
		c.OnToggle(c.Checked)
	}
}
