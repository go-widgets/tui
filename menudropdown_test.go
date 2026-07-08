// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestMenuDropdown(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	ran := ""
	d := &MenuDropdown{
		Title: "File",
		Body:  []string{"Save", "Quit"},
		ItemActions: []func(){
			func() { ran = "Save" },
			nil, // informational row
		},
		AnchorX: 5, AnchorY: 1,
	}

	// size: widest row "File"/"Save"/"Quit" = 4 + 4 pad = 8; height 2+2 = 4.
	if w, h := d.size(); w != 8 || h != 4 {
		t.Errorf("size = (%d,%d), want (8,4)", w, h)
	}
	// A short title/body still floors the height at 3.
	if _, h := (&MenuDropdown{Title: "x"}).size(); h != 3 {
		t.Errorf("min height = %d, want 3", h)
	}
	// SetBounds self-positions at the anchor with the natural size.
	d.SetBounds(toolkit.Rect{X: 99, Y: 99, W: 1, H: 1})
	if b := d.Bounds(); b.X != 5 || b.Y != 1 || b.W != 8 || b.H != 4 {
		t.Errorf("bounds = %+v, want {5,1,8,4}", b)
	}

	// Hidden: HitTest false + Draw no-op.
	if d.HitTest(5, 1) {
		t.Error("hidden dropdown claimed a hit")
	}
	d.Draw(mk(20, 10), toolkit.DefaultLight())

	// Visible: HitTest inside true, Draw renders.
	d.Visible = true
	if !d.HitTest(6, 2) {
		t.Error("visible dropdown should claim an in-bounds hit")
	}
	d.Draw(mk(20, 10), toolkit.DefaultLight())

	// Click the "Save" row (local Y=1) runs its action + dismisses.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1})
	if ran != "Save" || d.Visible {
		t.Errorf("Save click: ran=%q visible=%v, want Save/false", ran, d.Visible)
	}
	// Click the informational row (nil action) just dismisses.
	ran = ""
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 2})
	if ran != "" || d.Visible {
		t.Errorf("info click: ran=%q visible=%v, want \"\"/false", ran, d.Visible)
	}
	// Click the title row (Y=0 -> idx -1) is a no-op action + dismiss.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 0})
	if d.Visible {
		t.Error("title-row click should still dismiss")
	}
	// Out-of-range row + non-click event.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 99})
	d.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})

	// Empty-title Draw skips the title line.
	e := &MenuDropdown{Body: []string{"only"}, Visible: true, AnchorX: 0, AnchorY: 0}
	e.Draw(mk(20, 10), toolkit.DefaultLight())
}
