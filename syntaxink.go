// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui/syntax"
)

// SyntaxInk maps a syntax.Kind to a terminal ink for the given theme, choosing
// a dark- or light-appropriate One Dark / One Light-style palette from the
// theme's background luminance. Plain + Punct fall back to the theme
// foreground. Shared by tui-explorer + tui-editor so their code panes colour
// identically.
func SyntaxInk(k syntax.Kind, theme *toolkit.Theme) toolkit.RGBA {
	rgb := func(r, g, b uint8) toolkit.RGBA { return toolkit.RGBA{R: r, G: g, B: b, A: 0xFF} }
	dark := theme.Background.R < 0x80
	switch k {
	case syntax.Keyword:
		if dark {
			return rgb(0xC6, 0x78, 0xDD)
		}
		return rgb(0xA6, 0x26, 0xA4)
	case syntax.String:
		if dark {
			return rgb(0x98, 0xC3, 0x79)
		}
		return rgb(0x50, 0xA1, 0x4F)
	case syntax.Comment:
		if dark {
			return rgb(0x7F, 0x84, 0x8E)
		}
		return rgb(0xA0, 0xA1, 0xA7)
	case syntax.Number:
		if dark {
			return rgb(0xD1, 0x9A, 0x66)
		}
		return rgb(0x98, 0x68, 0x01)
	case syntax.Type:
		if dark {
			return rgb(0x56, 0xB6, 0xC2)
		}
		return rgb(0x01, 0x84, 0xBC)
	case syntax.Func:
		if dark {
			return rgb(0x61, 0xAF, 0xEF)
		}
		return rgb(0x40, 0x78, 0xF2)
	default: // Plain, Punct
		return rgb(theme.OnSurface.R, theme.OnSurface.G, theme.OnSurface.B)
	}
}
