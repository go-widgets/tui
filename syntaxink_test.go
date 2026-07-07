// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui/syntax"
)

func TestSyntaxInk(t *testing.T) {
	light := toolkit.DefaultLight()
	dark := toolkit.DefaultDark()
	// Every kind resolves to an opaque ink in both palettes (covers each case
	// + the dark/light branch within each).
	for _, k := range []syntax.Kind{
		syntax.Keyword, syntax.String, syntax.Comment, syntax.Number,
		syntax.Type, syntax.Func, syntax.Plain, syntax.Punct,
	} {
		if c := SyntaxInk(k, light); c.A != 0xFF {
			t.Errorf("light kind %d not opaque: %+v", k, c)
		}
		if c := SyntaxInk(k, dark); c.A != 0xFF {
			t.Errorf("dark kind %d not opaque: %+v", k, c)
		}
	}
	// Spot-check the two keyword hues + the Plain -> OnSurface fallback.
	if got := SyntaxInk(syntax.Keyword, light); got != (toolkit.RGBA{R: 0xA6, G: 0x26, B: 0xA4, A: 0xFF}) {
		t.Errorf("light keyword = %+v", got)
	}
	if got := SyntaxInk(syntax.Keyword, dark); got != (toolkit.RGBA{R: 0xC6, G: 0x78, B: 0xDD, A: 0xFF}) {
		t.Errorf("dark keyword = %+v", got)
	}
	if got := SyntaxInk(syntax.Plain, light); got != (toolkit.RGBA{R: light.OnSurface.R, G: light.OnSurface.G, B: light.OnSurface.B, A: 0xFF}) {
		t.Errorf("plain = %+v, want OnSurface", got)
	}
}
