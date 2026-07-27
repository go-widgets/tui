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

// TestDropdownOpenUpDraw exercises the OpenUp layout at cell precision: the
// control row sits at the BOTTOM of Bounds and the option list extends
// upward above it, clipped to Bounds. Non-zero bounds (X=2, Y=3) to catch
// any accidental use of absolute instead of Bounds-relative coordinates.
func TestDropdownOpenUpDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	d := NewDropdown([]string{"red", "green", "blue"}, 1)
	d.OpenUp = true
	d.SetBounds(toolkit.Rect{X: 2, Y: 3, W: 10, H: 5})
	d.open()
	d.active = 2 // highlight "blue"

	cp := painter.NewCellPainter(14, 10)
	d.Draw(cp, theme)

	at := func(x, y int) painter.Cell { return cp.Cells[y*cp.W+x] }

	// Control row is the LAST row of Bounds: Y = 3+5-1 = 7. Selected=1 →
	// value() = "green".
	if r := at(3, 7).Rune; r != 'g' {
		t.Fatalf("control row rune = %q, want 'g' (green)", r)
	}
	// Options extend upward from the control row: row 6 = option 0 ("red"),
	// row 5 = option 1 ("green", the active/highlighted one), row 4 = option
	// 2 ("blue"). Bounds top is Y=3, so all three fit.
	if got := at(3, 6).Rune; got != 'r' {
		t.Errorf("option 0 row (y=6) rune = %q, want 'r' (red)", got)
	}
	if got := at(3, 5).Rune; got != 'g' {
		t.Errorf("option 1 row (y=5) rune = %q, want 'g' (green)", got)
	}
	// The active option (index 2, "blue") is drawn with Accent background +
	// Background ink — assert both, not just the rune, for true cell
	// precision.
	activeCell := at(3, 4)
	if activeCell.Rune != 'b' {
		t.Errorf("option 2 row (y=4) rune = %q, want 'b' (blue)", activeCell.Rune)
	}
	if activeCell.Bg != theme.Accent {
		t.Errorf("active option bg = %+v, want theme.Accent %+v", activeCell.Bg, theme.Accent)
	}
	if activeCell.Fg != theme.Background {
		t.Errorf("active option fg = %+v, want theme.Background %+v", activeCell.Fg, theme.Background)
	}
	// A non-active option row uses SurfaceAlt background.
	inactiveCell := at(2, 6) // start of "red" row's background fill
	if inactiveCell.Bg != theme.SurfaceAlt {
		t.Errorf("inactive option bg = %+v, want theme.SurfaceAlt %+v", inactiveCell.Bg, theme.SurfaceAlt)
	}

	// Height too small to fit every option upward: only rows within Bounds
	// are painted (no panic, no out-of-bounds write).
	tight := NewDropdown([]string{"a", "b", "c"}, 0)
	tight.OpenUp = true
	tight.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 2})
	tight.open()
	tight.Draw(painter.NewCellPainter(10, 2), theme)
}

// TestDropdownOpenUpClick exercises OnEvent's OpenUp hit-testing: the control
// row is local Y = H-1, and options extend upward from it (local Y
// decreasing). Coordinates passed to OnEvent are local to Bounds regardless
// of the Bounds origin (mirrors the rest of the OnEvent test suite).
func TestDropdownOpenUpClick(t *testing.T) {
	got := -1
	d := NewDropdown([]string{"a", "b", "c"}, 2) // start on "c" so selecting "a" is a real change
	d.OpenUp = true
	d.OnChange = func(i int, _ string) { got = i }
	// H=6 leaves 2 unused local rows (0,1) above the 3 option rows (2,3,4)
	// and the control row (5), so a click there is genuinely out of range.
	d.SetBounds(toolkit.Rect{X: 5, Y: 5, W: 8, H: 6}) // control row local Y=5

	// Collapsed: click off the control row is a no-op; click the control row
	// (local Y=5) opens.
	d.OnEvent(vclick(0, 0))
	if d.Open {
		t.Fatal("off-row click opened dropdown (OpenUp)")
	}
	d.OnEvent(vclick(0, 5))
	if !d.Open {
		t.Fatal("control-row click did not open (OpenUp)")
	}
	// Open: click the control row again closes.
	d.OnEvent(vclick(0, 5))
	if d.Open {
		t.Fatal("control-row click did not close (OpenUp)")
	}
	// Open, then click option row 0 (local Y=4, immediately above the
	// control row at local Y=5) → option index 0 = "a".
	d.OnEvent(vclick(0, 5)) // open
	d.OnEvent(vclick(0, 4))
	if d.Selected != 0 || got != 0 || d.Open {
		t.Fatalf("OpenUp click option: sel=%d got=%d open=%v", d.Selected, got, d.Open)
	}
	// Open, click above the options (out of range upward) is a no-op.
	d.OnEvent(vclick(0, 5)) // open
	d.OnEvent(vclick(0, 0))
	if !d.Open {
		t.Error("out-of-range upward click closed the dropdown")
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
