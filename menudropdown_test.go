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

// TestMenuDropdownNoCheckGutter confirms a MenuDropdown with no
// Checkable/RadioGroup rows lays out exactly as before the feature existed:
// no gutter reserved, size unaffected.
func TestMenuDropdownNoCheckGutter(t *testing.T) {
	d := &MenuDropdown{Title: "File", Body: []string{"Save", "Quit"}}
	if d.hasCheckGutter() {
		t.Fatal("plain dropdown reported a check gutter")
	}
	if w, h := d.size(); w != 8 || h != 4 {
		t.Errorf("size = (%d,%d), want (8,4) (unaffected by the check feature)", w, h)
	}
}

// TestMenuDropdownCheckable mirrors toolkit.MenuItem's Checkable semantics
// (see toolkit.Menu): clicking a checkable row flips its Checked entry and
// still runs its action; the row renders a ✓ glyph in a reserved gutter
// while checked.
func TestMenuDropdownCheckable(t *testing.T) {
	theme := toolkit.DefaultLight()
	ran := ""
	d := &MenuDropdown{
		Title:     "View",
		Body:      []string{"Bold", "Plain"},
		Checkable: []bool{true, false},
		ItemActions: []func(){
			func() { ran = "Bold" },
			nil,
		},
		Visible: true,
		AnchorX: 1, AnchorY: 1,
	}
	if !d.hasCheckGutter() {
		t.Fatal("dropdown with a Checkable row should reserve a check gutter")
	}
	// size grows by the 2-cell gutter: widest("View"/"Bold"/"Plain")=5 + 2
	// gutter + 4 pad = 11; height 2+2 = 4.
	if w, h := d.size(); w != 11 || h != 4 {
		t.Errorf("size = (%d,%d), want (11,4)", w, h)
	}

	cp := painter.NewCellPainter(20, 8)
	d.Draw(cp, theme)
	r := d.Bounds()
	at := func(x, y int) rune { return cp.Cells[y*cp.W+x].Rune }

	// Row 0 ("Bold") starts unchecked: no glyph in the gutter, label shifted
	// right by the gutter width (2).
	row0Y := r.Y + 1
	if got := at(r.X+2, row0Y); got != ' ' {
		t.Errorf("unchecked row glyph cell = %q, want blank", got)
	}
	if got := at(r.X+2+2, row0Y); got != 'B' {
		t.Errorf("unchecked row label start = %q, want 'B'", got)
	}

	// Click row 0: toggles Checked + still runs its ItemActions entry.
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1})
	if !d.isChecked(0) {
		t.Fatal("click on checkable row did not set Checked")
	}
	if ran != "Bold" {
		t.Errorf("checkable click ran=%q, want Bold", ran)
	}
	if d.Visible {
		t.Error("click should still dismiss the dropdown")
	}

	// Redraw: the ✓ glyph now occupies the gutter.
	d.Visible = true
	cp2 := painter.NewCellPainter(20, 8)
	d.Draw(cp2, theme)
	at2 := func(x, y int) rune { return cp2.Cells[y*cp2.W+x].Rune }
	if got := at2(r.X+2, row0Y); got != '✓' {
		t.Errorf("checked row glyph = %q, want '✓'", got)
	}

	// A second click un-checks it (toggle) without re-running the action a
	// second time in a way that would flip ran to something else -- it
	// still fires Bold again, which is expected (mirrors toolkit.Menu).
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1})
	if d.isChecked(0) {
		t.Error("second click did not un-check the row")
	}

	// Row 1 ("Plain") is not Checkable: clicking it never sets Checked and
	// the row's own Checked slot stays absent (short slice).
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 2})
	if d.isChecked(1) {
		t.Error("non-checkable row reported checked")
	}
}

// TestMenuDropdownRadioGroup mirrors toolkit.MenuItem's RadioGroup
// semantics: clicking a radio-grouped row selects it exclusively among its
// siblings (same RadioGroup value) and renders a • glyph instead of ✓.
// RadioGroup implies checkable, so Checkable need not also be set.
func TestMenuDropdownRadioGroup(t *testing.T) {
	theme := toolkit.DefaultLight()
	d := &MenuDropdown{
		Title:      "Size",
		Body:       []string{"Small", "Medium", "Large", "Other"},
		RadioGroup: []int{1, 1, 1, 0}, // "Other" is outside the group
		Visible:    true,
		AnchorX:    0, AnchorY: 0,
	}
	if !d.hasCheckGutter() {
		t.Fatal("dropdown with a radio-grouped row should reserve a check gutter")
	}

	// Select "Medium" (row 1): it becomes checked, its siblings (0, 2) are
	// not, and the unrelated row 3 (group 0) is untouched.
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 2}) // row index 1
	for i, want := range map[int]bool{0: false, 1: true, 2: false, 3: false} {
		if got := d.isChecked(i); got != want {
			t.Errorf("after selecting Medium: isChecked(%d) = %v, want %v", i, got, want)
		}
	}

	// Selecting "Small" (row 0) moves the exclusive check within the group.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1}) // row index 0
	for i, want := range map[int]bool{0: true, 1: false, 2: false, 3: false} {
		if got := d.isChecked(i); got != want {
			t.Errorf("after selecting Small: isChecked(%d) = %v, want %v", i, got, want)
		}
	}

	// Draw renders a • (bullet) glyph for the checked radio row, not ✓.
	d.Visible = true
	cp := painter.NewCellPainter(20, 8)
	d.Draw(cp, theme)
	r := d.Bounds()
	got := cp.Cells[(r.Y+1)*cp.W+r.X+2].Rune
	if got != '•' {
		t.Errorf("radio glyph = %q, want '•'", got)
	}

	// "Other" (row 3, RadioGroup 0 == "no group") behaves as a plain,
	// non-checkable informational row: clicking it never sets Checked.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 4}) // row index 3
	if d.isChecked(3) {
		t.Error("row outside any radio group reported checked")
	}
}

// TestMenuDropdownSetCheckedGrowsSlice exercises setChecked's growth path
// when Checked starts shorter than the index being set (e.g. driven
// programmatically before any Draw/OnEvent has grown it).
func TestMenuDropdownSetCheckedGrowsSlice(t *testing.T) {
	d := &MenuDropdown{Body: []string{"a", "b", "c"}, Checkable: []bool{true, true, true}}
	if len(d.Checked) != 0 {
		t.Fatalf("Checked should start empty, got %v", d.Checked)
	}
	d.setChecked(2, true)
	if len(d.Checked) != 3 || !d.Checked[2] {
		t.Fatalf("setChecked(2,true) = %v, want len 3 with [2]=true", d.Checked)
	}
	if d.isChecked(0) || d.isChecked(1) {
		t.Error("setChecked(2,...) should not affect rows 0/1")
	}
}
