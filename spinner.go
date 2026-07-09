// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Spinner is a cell-native busy indicator: a single animated glyph (Accent),
// optionally followed by a Label. It advances one Frame per EventTick while
// Active, so a host that wants animation runs the App with TickHz > 0. Set
// Active = false to freeze it (e.g. when the work completes).
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Spinner struct {
	toolkit.Base
	Frames []string // animation frames (one glyph each); defaults to braille dots
	Label  string
	Active bool

	frame int
}

// spinnerFrames is the default braille-dot cycle.
func spinnerFrames() []string {
	return []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
}

// NewSpinner builds an active Spinner with the default frames and the given
// label (which may be empty).
func NewSpinner(label string) *Spinner {
	return &Spinner{Frames: spinnerFrames(), Label: label, Active: true}
}

// Draw paints the current frame (Accent) and the label (OnSurface).
func (s *Spinner) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := s.Bounds()
	if len(s.Frames) > 0 {
		toolkit.DrawText(pnt, r.X, r.Y, s.Frames[s.frame%len(s.Frames)], theme.Accent)
	}
	if s.Label != "" {
		toolkit.DrawText(pnt, r.X+2, r.Y, s.Label, theme.OnSurface)
	}
}

// OnEvent advances the animation one frame per tick while Active.
func (s *Spinner) OnEvent(ev toolkit.Event) {
	if ev.Kind == EventTick && s.Active && len(s.Frames) > 0 {
		s.frame = (s.frame + 1) % len(s.Frames)
	}
}
