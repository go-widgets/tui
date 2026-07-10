// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Kbd is a cell-native keycap: the Keys string on a SurfaceAlt "cap" background,
// for rendering shortcut hints (e.g. Ctrl+K) inline in help text. Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Kbd struct {
	toolkit.Base
	Keys string
}

// NewKbd builds a Kbd for the given key string.
func NewKbd(keys string) *Kbd { return &Kbd{Keys: keys} }

// Draw paints the cap (a 1-cell pad on each side of the keys) and the keys.
func (k *Kbd) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := k.Bounds()
	w := utf8.RuneCountInString(k.Keys) + 2
	if w > r.W {
		w = r.W
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: w, H: 1}, theme.SurfaceAlt)
	toolkit.DrawText(pnt, r.X+1, r.Y, k.Keys, theme.OnSurface)
}
