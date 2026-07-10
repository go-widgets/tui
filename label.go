// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Align is a Label's horizontal text alignment within its bounds.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Label is a cell-native static text widget: one line of Text, horizontally
// aligned within its bounds and vertically centred. The demos previously
// borrowed toolkit.Label (which renders acceptably in cells but is left-aligned
// only); tui.Label is the native, alignable equivalent. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Label struct {
	toolkit.Base
	Text  string
	Align Align
}

// NewLabel builds a left-aligned Label.
func NewLabel(text string) *Label { return &Label{Text: text} }

// Draw paints the text at the aligned column and vertically-centred row, clamped
// so it never starts left of the widget's own bounds.
func (l *Label) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := l.Bounds()
	n := utf8.RuneCountInString(l.Text)
	x := r.X
	switch l.Align {
	case AlignCenter:
		x = r.X + (r.W-n)/2
	case AlignRight:
		x = r.X + r.W - n
	}
	if x < r.X {
		x = r.X
	}
	toolkit.DrawText(pnt, x, r.Y+(r.H-1)/2, l.Text, theme.OnSurface)
}
