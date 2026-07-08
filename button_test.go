// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestButtonClick(t *testing.T) {
	clicks := 0
	b := NewButton("OK", func() { clicks++ })
	b.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if clicks != 1 {
		t.Fatalf("click count = %d, want 1", clicks)
	}
	// Non-click events are ignored.
	b.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if clicks != 1 {
		t.Errorf("keydown fired OnClick: %d", clicks)
	}
	// A nil handler is a safe no-op.
	NewButton("x", nil).OnEvent(toolkit.Event{Kind: toolkit.EventClick})
}

func TestButtonDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Every resting style + the empty-label branch.
	for _, st := range []ButtonStyle{ButtonDefault, ButtonProminent, ButtonSecondary} {
		b := &Button{Label: "Go", Style: st}
		b.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 3})
		b.Draw(mk(8, 3), theme)
	}
	empty := &Button{Style: ButtonDefault}
	empty.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 1})
	empty.Draw(mk(8, 1), theme)

	// Hover overrides the resting fill.
	h := NewButton("Hi", nil)
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 1})
	h.SetHovered(true)
	h.Draw(mk(8, 1), theme)

	// Press overrides both style and hover.
	p := &Button{Label: "Hit", Style: ButtonSecondary}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 1})
	p.SetHovered(true)
	p.SetPressed(true)
	p.Draw(mk(8, 1), theme)
}
