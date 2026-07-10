// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestBannerFaceAndIcon(t *testing.T) {
	theme := toolkit.DefaultLight()
	// Info defers to the theme accent; the others carry fixed shades.
	if bannerFace(BannerInfo, theme) != theme.Accent {
		t.Error("Info face should be the theme accent")
	}
	cases := []struct {
		k    BannerKind
		face toolkit.RGBA
		icon string
	}{
		{BannerSuccess, toolkit.RGB(0x2E, 0x8B, 0x57), "✓"},
		{BannerWarning, toolkit.RGB(0xE0, 0xA0, 0x30), "⚠"},
		{BannerError, toolkit.RGB(0xC0, 0x30, 0x30), "✗"},
	}
	for _, c := range cases {
		if bannerFace(c.k, theme) != c.face {
			t.Errorf("kind %d face = %v, want %v", c.k, bannerFace(c.k, theme), c.face)
		}
		if bannerIcon(c.k) != c.icon {
			t.Errorf("kind %d icon = %q, want %q", c.k, bannerIcon(c.k), c.icon)
		}
	}
	// Info icon + an out-of-range kind both fall to the default.
	if bannerIcon(BannerInfo) != "ℹ" || bannerIcon(BannerKind(99)) != "ℹ" {
		t.Error("default icon should be ℹ")
	}
	if bannerFace(BannerKind(99), theme) != theme.Accent {
		t.Error("out-of-range kind face should default to accent")
	}
}

func TestBannerDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()
	for _, k := range []BannerKind{BannerInfo, BannerSuccess, BannerWarning, BannerError} {
		b := NewBanner("message", k)
		b.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 1})
		b.Draw(mk(30, 1), theme)
	}
	// Multi-row bounds centre the text row.
	b := NewBanner("tall", BannerInfo)
	b.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
	b.Draw(mk(30, 3), theme)
}
