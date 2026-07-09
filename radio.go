// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// RadioButton is a cell-native circular toggle paired with a label: "(•)"
// checked / "( )" unchecked, then the Label. Grouped via RadioGroup for
// mutual exclusion; a standalone RadioButton toggles like a CheckButton.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type RadioButton struct {
	toolkit.Base
	Label    string
	Checked  bool
	OnToggle func(checked bool)

	group   *RadioGroup
	index   int
	focused bool
}

// SetFocused implements Focusable — a focused radio draws its mark in Accent and
// activates on Enter.
func (r *RadioButton) SetFocused(v bool) { r.focused = v }

// radioBoxCells is the width of the "(•) " box prefix incl. its trailing space,
// so the label starts at r.X + radioBoxCells.
const radioBoxCells = 4

// NewRadioButton constructs a standalone RadioButton. Add it to a RadioGroup for
// mutual-exclusion behaviour.
func NewRadioButton(label string) *RadioButton {
	return &RadioButton{Label: label}
}

// Draw paints the mark (Accent when checked, Border otherwise) and the label.
func (r *RadioButton) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	b := r.Bounds()
	box, ink := "( )", theme.Border
	if r.Checked {
		box, ink = "(•)", theme.Accent
	} else if r.focused {
		ink = theme.Accent // focus cue on the empty mark
	}
	toolkit.DrawText(pnt, b.X, b.Y, box, ink)
	if r.Label != "" {
		toolkit.DrawText(pnt, b.X+radioBoxCells, b.Y, r.Label, theme.OnSurface)
	}
}

// OnEvent, on a click or Enter (so a focused radio is keyboard-activatable),
// routes through the group (siblings clear) if grouped, else toggles locally.
func (r *RadioButton) OnEvent(ev toolkit.Event) {
	act := ev.Kind == toolkit.EventClick ||
		(ev.Kind == toolkit.EventKeyDown && ev.Code == "Enter")
	if !act {
		return
	}
	if r.group != nil {
		r.group.activate(r.index)
		return
	}
	r.Checked = !r.Checked
	if r.OnToggle != nil {
		r.OnToggle(r.Checked)
	}
}

// RadioGroup makes a set of RadioButtons mutually exclusive. Active is the index
// of the checked member, or -1 when none has been clicked yet.
type RadioGroup struct {
	Members []*RadioButton
	Active  int
}

// NewRadioGroup builds an empty group with Active = -1.
func NewRadioGroup() *RadioGroup { return &RadioGroup{Active: -1} }

// Add appends r to the group and records its membership so a click on any member
// can clear the others.
func (g *RadioGroup) Add(r *RadioButton) {
	r.group = g
	r.index = len(g.Members)
	g.Members = append(g.Members, r)
}

// activate checks member idx, clears the rest, and fires its OnToggle.
func (g *RadioGroup) activate(idx int) {
	g.Active = idx
	for i, m := range g.Members {
		m.Checked = i == idx
	}
	if cb := g.Members[idx].OnToggle; cb != nil {
		cb(true)
	}
}
