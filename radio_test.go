// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestRadioStandalone(t *testing.T) {
	toggles := 0
	r := NewRadioButton("Opt")
	r.OnToggle = func(bool) { toggles++ }

	r.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if !r.Checked || toggles != 1 {
		t.Fatalf("standalone click: checked=%v toggles=%d", r.Checked, toggles)
	}
	r.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if r.Checked || toggles != 2 {
		t.Fatalf("standalone re-click: checked=%v toggles=%d", r.Checked, toggles)
	}
	// Non-click is ignored.
	r.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if toggles != 2 {
		t.Errorf("keydown toggled: %d", toggles)
	}
	// Nil OnToggle must not panic.
	NewRadioButton("x").OnEvent(toolkit.Event{Kind: toolkit.EventClick})
}

func TestRadioGroup(t *testing.T) {
	g := NewRadioGroup()
	if g.Active != -1 {
		t.Fatalf("NewRadioGroup Active = %d, want -1", g.Active)
	}
	changed := ""
	a, b, c := NewRadioButton("A"), NewRadioButton("B"), NewRadioButton("C")
	a.OnToggle = func(bool) { changed = "A" }
	b.OnToggle = func(bool) { changed = "B" }
	// c has a nil OnToggle to exercise the no-callback branch.
	g.Add(a)
	g.Add(b)
	g.Add(c)

	// Click B: B checked, others cleared, Active = 1, OnToggle fired.
	b.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if !b.Checked || a.Checked || c.Checked || g.Active != 1 || changed != "B" {
		t.Fatalf("click B: a=%v b=%v c=%v active=%d changed=%q", a.Checked, b.Checked, c.Checked, g.Active, changed)
	}
	// Click A flips exclusivity.
	a.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if !a.Checked || b.Checked || g.Active != 0 || changed != "A" {
		t.Fatalf("click A: a=%v b=%v active=%d", a.Checked, b.Checked, g.Active)
	}
	// Click C (nil OnToggle): still activates, no callback panic.
	c.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if !c.Checked || a.Checked || g.Active != 2 {
		t.Fatalf("click C: c=%v active=%d", c.Checked, g.Active)
	}
}

func TestRadioDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// Unchecked + label.
	u := NewRadioButton("Choice")
	u.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 16, H: 1})
	u.Draw(mk(16, 1), theme)

	// Checked + empty label (skips label draw).
	c := &RadioButton{Checked: true}
	c.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	c.Draw(mk(4, 1), theme)
}
