// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// BannerKind selects a Banner's semantic colour + icon.
type BannerKind int

const (
	// BannerInfo is a neutral heads-up (theme Accent, ℹ).
	BannerInfo BannerKind = iota
	// BannerSuccess signals a completed operation (green, ✓).
	BannerSuccess
	// BannerWarning flags a non-fatal issue (amber, ⚠).
	BannerWarning
	// BannerError signals a failure to address (red, ✗).
	BannerError
)

// Banner is a cell-native inline message strip: a severity-coloured bar filling
// its bounds with an icon + Text in a contrasting ink. Unlike Popover/Dialog it
// is non-modal and inline (stack it above content or in a header). Display-only.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Banner struct {
	toolkit.Base
	Text string
	Kind BannerKind
}

// NewBanner builds a Banner with the given text and severity.
func NewBanner(text string, kind BannerKind) *Banner {
	return &Banner{Text: text, Kind: kind}
}

// bannerFace maps a kind to its bar colour (matching toolkit.Alert's shades).
func bannerFace(kind BannerKind, theme *toolkit.Theme) toolkit.RGBA {
	switch kind {
	case BannerSuccess:
		return toolkit.RGB(0x2E, 0x8B, 0x57) // sea green
	case BannerWarning:
		return toolkit.RGB(0xE0, 0xA0, 0x30) // amber
	case BannerError:
		return toolkit.RGB(0xC0, 0x30, 0x30) // brick red
	default: // BannerInfo (and any out-of-range kind)
		return theme.Accent
	}
}

// bannerIcon maps a kind to its leading glyph.
func bannerIcon(kind BannerKind) string {
	switch kind {
	case BannerSuccess:
		return "✓"
	case BannerWarning:
		return "⚠"
	case BannerError:
		return "✗"
	default:
		return "ℹ"
	}
}

// Draw fills the strip with the severity colour and writes the icon + text in
// the theme Background (legible over any of the faces).
func (b *Banner) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := b.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, bannerFace(b.Kind, theme))
	ty := r.Y + (r.H-1)/2
	toolkit.DrawText(pnt, r.X+1, ty, bannerIcon(b.Kind)+" "+b.Text, theme.Background)
}
