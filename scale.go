// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Scale is a cell-native horizontal slider over a continuous Min..Max range: a
// SurfaceAlt track, an Accent fill up to the thumb, and a thumb cell at the
// value's position. A click or drag jumps the thumb to that column;
// ArrowLeft/Right step by Step (or a tenth of the range when Step <= 0). Because
// it treats a click and a captured drag identically, a Scale nested in a
// drag-capturing container (VBox / HSplit) is smoothly draggable.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Scale struct {
	toolkit.Base
	Min, Max float64
	Value    float64
	Step     float64 // arrow-key increment; <= 0 means a tenth of the range
	OnChange func(v float64)
}

// NewScale builds a Scale spanning [min, max] with the given initial value.
// Min == Max is allowed but renders a non-interactive track.
func NewScale(min, max, initial float64) *Scale {
	s := &Scale{Min: min, Max: max}
	s.SetValue(initial)
	return s
}

// SetValue clamps v to [Min, Max] before assigning.
func (s *Scale) SetValue(v float64) {
	if v < s.Min {
		v = s.Min
	}
	if v > s.Max {
		v = s.Max
	}
	s.Value = v
}

// fraction returns the value's position in [0,1] (0 when Max == Min).
func (s *Scale) fraction() float64 {
	if s.Max > s.Min {
		return (s.Value - s.Min) / (s.Max - s.Min)
	}
	return 0
}

// Draw paints the track, the Accent fill up to the thumb, and the thumb cell.
func (s *Scale) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	ty := r.Y + (r.H-1)/2 // the track/thumb row
	pnt.FillRect(painter.Rect{X: r.X, Y: ty, W: r.W, H: 1}, theme.SurfaceAlt)
	thumbX := r.X + int(s.fraction()*float64(r.W-1))
	pnt.FillRect(painter.Rect{X: r.X, Y: ty, W: thumbX - r.X + 1, H: 1}, theme.Accent)
	pnt.FillRect(painter.Rect{X: thumbX, Y: ty, W: 1, H: 1}, theme.OnSurface)
}

// setFromX maps a local x-column to a value and fires OnChange.
func (s *Scale) setFromX(evx int) {
	r := s.Bounds()
	if r.W <= 1 {
		s.SetValue(s.Min)
	} else {
		pos := float64(evx) / float64(r.W-1)
		if pos < 0 {
			pos = 0
		}
		if pos > 1 {
			pos = 1
		}
		s.SetValue(s.Min + pos*(s.Max-s.Min))
	}
	if s.OnChange != nil {
		s.OnChange(s.Value)
	}
}

// OnEvent handles click/drag positioning and arrow-key stepping.
func (s *Scale) OnEvent(ev toolkit.Event) {
	if s.Max <= s.Min {
		return // non-interactive track
	}
	switch ev.Kind {
	case toolkit.EventClick, toolkit.EventMouseDrag:
		if s.Bounds().W <= 0 {
			return
		}
		s.setFromX(ev.X)
	case toolkit.EventKeyDown:
		step := s.Step
		if step <= 0 {
			step = (s.Max - s.Min) / 10
		}
		switch ev.Code {
		case "ArrowRight":
			s.SetValue(s.Value + step)
		case "ArrowLeft":
			s.SetValue(s.Value - step)
		default:
			return
		}
		if s.OnChange != nil {
			s.OnChange(s.Value)
		}
	}
}
