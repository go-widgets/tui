// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Button is a cell-native clickable action: a filled block with a centred
// label. The resting face comes from its Style (Default / Prominent /
// Secondary); hover and press states override the fill so the user sees
// feedback before OnClick fires. A parent container drives SetHovered /
// SetPressed (enter/leave logic stays in one place, not every leaf).
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Button struct {
	toolkit.Base
	Label   string
	OnClick func()
	Style   ButtonStyle // resting appearance; default ButtonDefault

	hovered bool
	pressed bool
	focused bool
}

// ButtonStyle selects a button's resting fill, giving a layout a visual
// hierarchy. Hover + press still override the fill on top of the style.
type ButtonStyle int

const (
	// ButtonDefault is a Surface-faced button (the plain look).
	ButtonDefault ButtonStyle = iota
	// ButtonProminent is filled with Accent -- the primary action ("OK").
	ButtonProminent
	// ButtonSecondary is filled with SurfaceAlt -- a muted key between the two.
	ButtonSecondary
)

// NewButton constructs a Button with the given label and click handler (which
// may be nil -- a no-op button still renders).
func NewButton(label string, onClick func()) *Button {
	return &Button{Label: label, OnClick: onClick}
}

// SetHovered / SetPressed are wired by the parent's mouse dispatcher so the
// button can render its hover / press visual states.
func (b *Button) SetHovered(v bool) { b.hovered = v }
func (b *Button) SetPressed(v bool) { b.pressed = v }

// SetFocused implements Focusable — a focused button highlights (like hover) and
// activates on Enter.
func (b *Button) SetFocused(v bool) { b.focused = v }

// Draw paints the face (cycling Surface / SurfaceAlt / Accent by style and
// interaction state) and the centred label, with an on-accent ink that stays
// legible over an Accent fill.
func (b *Button) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := b.Bounds()
	face := theme.Surface
	ink := theme.OnSurface
	switch b.Style {
	case ButtonProminent:
		face, ink = theme.Accent, theme.Background
	case ButtonSecondary:
		face = theme.SurfaceAlt
	}
	switch {
	case b.pressed:
		face, ink = theme.Accent, theme.Background
	case b.hovered, b.focused:
		face = theme.SurfaceAlt
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, face)
	if b.Label != "" {
		tw := utf8.RuneCountInString(b.Label)
		tx := r.X + (r.W-tw)/2
		ty := r.Y + (r.H-1)/2
		toolkit.DrawText(pnt, tx, ty, b.Label, ink)
	}
}

// OnEvent fires OnClick on a click or an Enter key (so a focused button is
// keyboard-activatable); other events are ignored.
func (b *Button) OnEvent(ev toolkit.Event) {
	activate := ev.Kind == toolkit.EventClick ||
		(ev.Kind == toolkit.EventKeyDown && ev.Code == "Enter")
	if activate && b.OnClick != nil {
		b.OnClick()
	}
}
