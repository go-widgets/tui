// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// SpinButton is a cell-native integer field with steppers: a ◂ decrement cap on
// the left, the value, and a ▸ increment cap on the right, all within [Min, Max].
// Clicking a cap (or, while focused, Up/Right/'+' and Down/Left/'-') steps the
// value by Step and fires OnChange on a real change. It is Focusable, so it
// drops into a FocusRing as a form control (the caps render in Accent when
// focused).
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type SpinButton struct {
	toolkit.Base
	Min, Max int
	Value    int
	Step     int
	OnChange func(v int)

	focused bool
}

// NewSpinButton builds a SpinButton over [min, max] with the given initial value
// (clamped) and step (floored at 1 so a stepper always moves).
func NewSpinButton(min, max, initial, step int) *SpinButton {
	if step <= 0 {
		step = 1
	}
	s := &SpinButton{Min: min, Max: max, Step: step}
	s.SetValue(initial)
	return s
}

// SetValue clamps v to [Min, Max] and assigns it (no OnChange — direct setter).
func (s *SpinButton) SetValue(v int) {
	if v < s.Min {
		v = s.Min
	}
	if v > s.Max {
		v = s.Max
	}
	s.Value = v
}

// SetFocused implements Focusable — a focused spinner renders its caps in Accent
// and responds to arrow / +/- keys.
func (s *SpinButton) SetFocused(v bool) { s.focused = v }

// step changes the value by delta*Step (clamped) and fires OnChange on a change.
func (s *SpinButton) step(delta int) {
	old := s.Value
	s.SetValue(s.Value + delta*s.Step)
	if s.Value != old && s.OnChange != nil {
		s.OnChange(s.Value)
	}
}

// Draw paints the ◂ / ▸ caps and the centred value.
func (s *SpinButton) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	cap := theme.Border
	if s.focused {
		cap = theme.Accent
	}
	toolkit.DrawText(pnt, r.X, r.Y, "◂", cap)
	toolkit.DrawText(pnt, r.X+r.W-1, r.Y, "▸", cap)
	val := strconv.Itoa(s.Value)
	toolkit.DrawText(pnt, r.X+(r.W-len(val))/2, r.Y, val, theme.OnSurface)
}

// OnEvent steps on a cap click or (when focused) an arrow / +/- key.
func (s *SpinButton) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventClick:
		switch {
		case ev.X <= 1:
			s.step(-1)
		case ev.X >= s.Bounds().W-2:
			s.step(1)
		}
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up", "ArrowUp", "Right", "ArrowRight":
			s.step(1)
		case "Down", "ArrowDown", "Left", "ArrowLeft":
			s.step(-1)
		}
	case toolkit.EventChar:
		switch ev.Code {
		case "+":
			s.step(1)
		case "-":
			s.step(-1)
		}
	}
}
