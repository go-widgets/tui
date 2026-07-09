// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestDropdownNewAndValue(t *testing.T) {
	d := NewDropdown([]string{"a", "b", "c"}, 1)
	if d.Selected != 1 || d.value() != "b" {
		t.Fatalf("New: selected=%d value=%q", d.Selected, d.value())
	}
	// Out-of-range initial selection clamps to 0.
	if got := NewDropdown([]string{"x"}, 9).Selected; got != 0 {
		t.Errorf("OOB selected = %d, want 0", got)
	}
	// value() out of range → "".
	if v := (&Dropdown{Options: []string{"a"}, Selected: 3}).value(); v != "" {
		t.Errorf("OOB value = %q, want empty", v)
	}
}

func TestDropdownKeyboard(t *testing.T) {
	changed, val := -1, ""
	d := NewDropdown([]string{"red", "green", "blue"}, 0)
	d.OnChange = func(i int, v string) { changed, val = i, v }

	// Collapsed: an unrelated key does nothing; Enter opens (active = Selected).
	d.OnEvent(vkey("x"))
	if d.Open {
		t.Fatal("unrelated key opened the dropdown")
	}
	d.OnEvent(vkey("Enter"))
	if !d.Open || d.active != 0 {
		t.Fatalf("Enter open: open=%v active=%d", d.Open, d.active)
	}
	// Down moves the highlight, clamping at the end.
	d.OnEvent(vkey("Down"))
	d.OnEvent(vkey("Down"))
	d.OnEvent(vkey("Down")) // clamp at 2
	if d.active != 2 {
		t.Fatalf("Down clamp: active=%d, want 2", d.active)
	}
	// Enter commits → Selected=2, OnChange fired, closed.
	d.OnEvent(vkey("Enter"))
	if d.Selected != 2 || changed != 2 || val != "blue" || d.Open {
		t.Fatalf("Enter select: sel=%d changed=%d val=%q open=%v", d.Selected, changed, val, d.Open)
	}

	// Down (collapsed) also opens; Up navigates + clamps at 0.
	d.OnEvent(vkey("Down"))
	if !d.Open || d.active != 2 {
		t.Fatalf("Down open: open=%v active=%d", d.Open, d.active)
	}
	d.OnEvent(vkey("Up"))
	d.OnEvent(vkey("Up"))
	d.OnEvent(vkey("Up")) // clamp at 0
	if d.active != 0 {
		t.Fatalf("Up clamp: active=%d, want 0", d.active)
	}
	// Escape closes without changing.
	changed = -1
	d.OnEvent(vkey("Escape"))
	if d.Open || d.Selected != 2 || changed != -1 {
		t.Fatalf("Escape: open=%v sel=%d changed=%d", d.Open, d.Selected, changed)
	}
	// Selecting the already-selected value fires no OnChange.
	d.OnEvent(vkey("Enter")) // open, active seeded at 2
	changed = -1
	d.OnEvent(vkey("Enter")) // select active(2) == Selected(2)
	if changed != -1 || d.Open {
		t.Errorf("no-change select fired OnChange (%d) or stayed open (%v)", changed, d.Open)
	}
}

func TestDropdownClick(t *testing.T) {
	got := -1
	d := NewDropdown([]string{"a", "b", "c"}, 0)
	d.OnChange = func(i int, _ string) { got = i }

	// Collapsed: click off the control row is a no-op; click row 0 opens.
	d.OnEvent(vclick(0, 5))
	if d.Open {
		t.Fatal("off-row click opened dropdown")
	}
	d.OnEvent(vclick(0, 0))
	if !d.Open {
		t.Fatal("control-row click did not open")
	}
	// Open: click row 0 (control) closes.
	d.OnEvent(vclick(0, 0))
	if d.Open {
		t.Fatal("control-row click did not close")
	}
	// Open, then click option row 2 (local Y=2 → option index 1 = "b").
	d.OnEvent(vclick(0, 0)) // open
	d.OnEvent(vclick(3, 2))
	if d.Selected != 1 || got != 1 || d.Open {
		t.Fatalf("click option: sel=%d got=%d open=%v", d.Selected, got, d.Open)
	}
	// Open, click below the options is a no-op (stays open).
	d.OnEvent(vclick(0, 0)) // open
	d.OnEvent(vclick(0, 99))
	if !d.Open {
		t.Error("out-of-range click closed the dropdown")
	}
	// A non-click/keydown event is ignored in both states.
	d.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag})
	d.Open = false
	d.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag})

	// selectActive with an out-of-range active is guarded (no panic, no change).
	g := NewDropdown([]string{"a", "b"}, 0)
	g.Open, g.active = true, 99
	g.OnEvent(vkey("Enter"))
	if g.Selected != 0 || g.Open {
		t.Errorf("OOB active select: sel=%d open=%v", g.Selected, g.Open)
	}

	// open() clamps a stale out-of-range Selected to a valid highlight.
	oob := NewDropdown([]string{"a", "b"}, 0)
	oob.Selected = 99
	oob.OnEvent(vkey("Enter")) // opens → active clamps to 0
	if oob.active != 0 {
		t.Errorf("open clamp: active=%d, want 0", oob.active)
	}
}

func TestDropdownDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	d := NewDropdown([]string{"one", "two", "three"}, 1)
	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 5})
	d.Draw(mk(12, 5), theme) // collapsed (▼)

	d.open()
	d.active = 0
	d.Draw(mk(12, 5), theme) // open: active highlight + inactive rows

	// Tight height forces the row loop to break past the viewport.
	d.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 2})
	d.Draw(mk(12, 2), theme)

	// Empty options, open → just the control row.
	e := &Dropdown{Open: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	e.Draw(mk(10, 3), theme)
}
