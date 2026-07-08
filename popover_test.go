// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestPopover(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	p := &Popover{Title: "Help", Body: []string{"a", "b"}}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 10})

	// Hidden: HitTest false + Draw is a no-op (early return).
	if p.HitTest(1, 1) {
		t.Error("hidden popover claimed a hit")
	}
	p.Draw(mk(20, 10), toolkit.DefaultLight())

	// Visible: HitTest inside bounds true; Draw renders the box + text.
	p.Visible = true
	if !p.HitTest(1, 1) {
		t.Error("visible popover should claim an in-bounds hit")
	}
	if p.HitTest(50, 50) {
		t.Error("visible popover claimed an out-of-bounds hit")
	}
	p.Draw(mk(20, 10), toolkit.DefaultLight())

	// Body taller than the bounds: the row loop breaks at the box edge.
	tall := &Popover{Title: "T", Body: []string{"1", "2", "3", "4", "5"}, Visible: true}
	tall.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 4}) // need=8, capped to 4
	tall.Draw(mk(20, 4), toolkit.DefaultLight())
}
