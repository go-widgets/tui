// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestCheckButtonToggle(t *testing.T) {
	var last bool
	toggles := 0
	c := NewCheckButton("Wrap", false)
	c.OnToggle = func(v bool) { last = v; toggles++ }

	c.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if !c.Checked || !last || toggles != 1 {
		t.Fatalf("first click: checked=%v last=%v toggles=%d", c.Checked, last, toggles)
	}
	c.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if c.Checked || last || toggles != 2 {
		t.Fatalf("second click: checked=%v last=%v toggles=%d", c.Checked, last, toggles)
	}
	// Enter toggles a (focused) checkbox.
	c.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if !c.Checked || toggles != 3 {
		t.Errorf("Enter did not toggle: checked=%v toggles=%d", c.Checked, toggles)
	}
	// A non-activating key is ignored.
	c.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if toggles != 3 {
		t.Errorf("non-Enter key toggled: %d", toggles)
	}
	// A nil handler must not panic.
	NewCheckButton("x", false).OnEvent(toolkit.Event{Kind: toolkit.EventClick})
}

func TestCheckButtonDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Unchecked + label.
	u := NewCheckButton("Enabled", false)
	u.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 16, H: 1})
	u.Draw(mk(16, 1), theme)

	// Checked + empty label (skips the label draw).
	c := &CheckButton{Checked: true}
	c.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	c.Draw(mk(4, 1), theme)
}
