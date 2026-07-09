// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func newConfirm() *Dialog {
	return NewDialog("Confirm", []string{"Save changes?"}, "Yes", "No")
}

func TestDialogSizeAndHitTest(t *testing.T) {
	d := newConfirm()
	// buttonsWidth: "Yes"(3)+2 + "No"(2)+2 + 1 gap = 10.
	if got := d.buttonsWidth(); got != 10 {
		t.Errorf("buttonsWidth = %d, want 10", got)
	}
	// size: widest = "Save changes?" (13) +4 = 17; h = 1 message + 5 = 6.
	if w, h := d.size(); w != 17 || h != 6 {
		t.Errorf("size = (%d,%d), want (17,6)", w, h)
	}
	// Wide title dominates the width.
	if w, _ := NewDialog("A very wide dialog title indeed", nil, "OK").size(); w != len("A very wide dialog title indeed")+4 {
		t.Errorf("wide-title size = %d", w)
	}
	// Buttons wider than title/message dominate.
	if w, _ := NewDialog("x", []string{"y"}, "Accept", "Decline").size(); w != (8+10)+4 {
		t.Errorf("wide-buttons size = %d, want %d", w, (8+10)+4)
	}

	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})
	// Hidden → no hit; visible → hit inside the box.
	if d.HitTest(20, 10) {
		t.Error("hidden dialog claimed a hit")
	}
	d.Visible = true
	if !d.HitTest(20, 10) {
		t.Error("visible dialog should claim an in-bounds hit")
	}
}

func TestDialogBoxClamps(t *testing.T) {
	d := newConfirm()
	// Bounds smaller than the natural box → clamp to bounds.
	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 3})
	b := d.box()
	if b.W != 8 || b.H != 3 {
		t.Errorf("clamped box = %dx%d, want 8x3", b.W, b.H)
	}
}

func TestDialogKeyTraversalAndActivate(t *testing.T) {
	var gotIdx int
	var gotLabel string
	d := newConfirm()
	d.OnAction = func(i int, l string) { gotIdx, gotLabel = i, l }
	d.Visible = true

	// Right / Tab advance (wrapping); Left / Shift+Tab retreat.
	d.OnEvent(vkey("ArrowRight"))
	if d.Active != 1 {
		t.Fatalf("Right: Active=%d, want 1", d.Active)
	}
	d.OnEvent(vkey("Tab"))
	if d.Active != 0 {
		t.Fatalf("Tab wrap: Active=%d, want 0", d.Active)
	}
	d.OnEvent(vkey("ArrowLeft"))
	if d.Active != 1 {
		t.Fatalf("Left wrap: Active=%d, want 1", d.Active)
	}
	d.OnEvent(vkey("Shift+Tab"))
	if d.Active != 0 {
		t.Fatalf("Shift+Tab: Active=%d, want 0", d.Active)
	}
	// An unrelated key is a no-op.
	d.OnEvent(vkey("F5"))

	// Enter activates the focused button (Yes) and hides.
	d.OnEvent(vkey("Enter"))
	if gotIdx != 0 || gotLabel != "Yes" || d.Visible {
		t.Fatalf("Enter: idx=%d label=%q visible=%v", gotIdx, gotLabel, d.Visible)
	}
}

func TestDialogEscapeCancels(t *testing.T) {
	gotIdx := 99
	d := newConfirm()
	d.OnAction = func(i int, _ string) { gotIdx = i }
	d.Visible = true
	d.OnEvent(vkey("Escape"))
	if gotIdx != -1 || d.Visible {
		t.Fatalf("Escape: idx=%d visible=%v, want -1/false", gotIdx, d.Visible)
	}

	// Escape / Enter with no handler + no buttons must not panic, and the
	// out-of-range activate guard leaves OnAction unfired.
	empty := NewDialog("t", nil)
	empty.Visible = true
	empty.OnEvent(vkey("Enter")) // 0 buttons → guard skips OnAction
	if empty.Visible {
		t.Error("Enter with no buttons did not hide")
	}
	NewDialog("t", nil, "OK").OnEvent(vkey("Escape")) // nil OnAction, no panic
	(&Dialog{Buttons: []string{"OK"}}).move(1)        // move on a struct dialog
}

func TestDialogClickActivates(t *testing.T) {
	got := -2
	d := newConfirm()
	d.OnAction = func(i int, _ string) { got = i }
	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})
	d.Visible = true
	// box centred: X=11,Y=7,W=17,H=6. Button row y=11; "Yes" x∈[14,19), "No" x∈[20,24).
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 15, Y: 11})
	if got != 0 || d.Visible {
		t.Fatalf("click Yes: got=%d visible=%v", got, d.Visible)
	}
	// Re-open; click "No".
	got, d.Visible = -2, true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 21, Y: 11})
	if got != 1 {
		t.Fatalf("click No: got=%d", got)
	}
	// Click off the button row is a no-op.
	got, d.Visible = -2, true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 15, Y: 8})
	// Click on the row but between/outside buttons is a no-op.
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 35, Y: 11})
	if got != -2 || !d.Visible {
		t.Errorf("stray clicks activated: got=%d visible=%v", got, d.Visible)
	}
}

func TestDialogDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	d := newConfirm()
	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})
	d.Draw(mk(40, 20), theme) // hidden → no-op
	d.Visible = true
	d.Active = 1
	d.Draw(mk(40, 20), theme) // renders box + active/inactive buttons
}
