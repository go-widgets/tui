// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Focusable is a widget that can hold keyboard focus within a FocusRing. The
// ring calls SetFocused as focus moves; the widget renders a focus cue while
// focused and acts on the keys the ring forwards to it (a Button/CheckButton/
// RadioButton fires on Enter, an Entry edits its text).
type Focusable interface {
	toolkit.Widget
	SetFocused(focused bool)
}

// The exported input widgets are all focusable.
var (
	_ Focusable = (*Entry)(nil)
	_ Focusable = (*Button)(nil)
	_ Focusable = (*CheckButton)(nil)
	_ Focusable = (*RadioButton)(nil)
	_ Focusable = (*SpinButton)(nil)
)

// FocusRing gives a set of Focusables a shared keyboard focus: Tab advances and
// Shift+Tab retreats (both wrapping); a click focuses the widget it lands on;
// every other event is forwarded to the focused member. Arrow keys are NOT
// consumed by the ring — they reach the focused widget, so a focused Dropdown or
// Scale keeps its own arrow behaviour. Layout is the caller's job (position each
// member's bounds); the ring manages focus + routing and draws its members.
//
// A form wires its inputs into a FocusRing and hands it to the App as Root (or
// as an InputTarget); Tab then walks the fields exactly as a GUI form does.
type FocusRing struct {
	toolkit.Base
	Items []Focusable
	idx   int
}

// NewFocusRing builds a ring over items, focusing the first.
func NewFocusRing(items ...Focusable) *FocusRing {
	r := &FocusRing{Items: items}
	if len(items) > 0 {
		items[0].SetFocused(true)
	}
	return r
}

// Focused returns the focused member's index, or -1 when the ring is empty.
func (r *FocusRing) Focused() int {
	if len(r.Items) == 0 {
		return -1
	}
	return r.idx
}

// Focus moves focus to member i (clamped), updating SetFocused on the old and
// new members.
func (r *FocusRing) Focus(i int) {
	if len(r.Items) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(r.Items) {
		i = len(r.Items) - 1
	}
	r.Items[r.idx].SetFocused(false)
	r.idx = i
	r.Items[i].SetFocused(true)
}

// Next / Prev move focus to the following / previous member, wrapping.
func (r *FocusRing) Next() {
	if n := len(r.Items); n > 0 {
		r.Focus((r.idx + 1) % n)
	}
}

func (r *FocusRing) Prev() {
	if n := len(r.Items); n > 0 {
		r.Focus((r.idx - 1 + n) % n)
	}
}

// Draw paints every member (positioned by the caller).
func (r *FocusRing) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	for _, it := range r.Items {
		it.Draw(pnt, theme)
	}
}

// OnEvent moves focus on Tab/Shift+Tab, focuses + forwards a click to the member
// it hits, and forwards every other event to the focused member.
func (r *FocusRing) OnEvent(ev toolkit.Event) {
	if len(r.Items) == 0 {
		return
	}
	if ev.Kind == toolkit.EventKeyDown {
		switch ev.Code {
		case "Tab":
			r.Next()
			return
		case "Shift+Tab":
			r.Prev()
			return
		}
	}
	if ev.Kind == toolkit.EventClick {
		for i, it := range r.Items {
			if it.HitTest(ev.X, ev.Y) {
				r.Focus(i)
				it.OnEvent(ev)
				return
			}
		}
		return
	}
	r.Items[r.idx].OnEvent(ev)
}
