// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func dmk(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }

func TestKbdDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	k := NewKbd("Ctrl+K")
	k.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	k.Draw(dmk(12, 1), theme)
	// Cap wider than the bounds clamps.
	narrow := NewKbd("Ctrl+Shift+P")
	narrow.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	narrow.Draw(dmk(4, 1), theme)
}

func TestBadgeDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	b := NewBadge("3")
	b.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 6, H: 1})
	b.Draw(dmk(6, 1), theme)
	narrow := NewBadge("999+")
	narrow.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 3, H: 1})
	narrow.Draw(dmk(3, 1), theme)
}

func TestBreadcrumbsDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	// Multiple items: last in accent, separators between.
	b := NewBreadcrumbs([]string{"home", "projects", "widgets"})
	b.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 1})
	b.Draw(dmk(40, 1), theme)
	// Single item: no separator.
	one := NewBreadcrumbs([]string{"root"})
	one.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	one.Draw(dmk(10, 1), theme)
	// Empty: nothing drawn.
	NewBreadcrumbs(nil).Draw(dmk(10, 1), theme)
}

func TestStepsDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	// Current in the middle → done (i<Current), current, and upcoming inks all hit.
	s := NewSteps([]string{"Plan", "Build", "Test", "Ship"}, 1)
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 60, H: 1})
	s.Draw(dmk(60, 1), theme)
	// Single step: no separator.
	one := NewSteps([]string{"Only"}, 0)
	one.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	one.Draw(dmk(12, 1), theme)
}

// TestStepsVerticalDraw asserts, at cell precision and non-zero bounds, that
// Orientation=Vertical stacks "N Label" rows joined by a "│" connector
// (instead of the horizontal " ─ " separator), with per-state ink (done /
// current / upcoming) preserved.
func TestStepsVerticalDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	s := NewSteps([]string{"Plan", "Build", "Ship"}, 1) // Current=1 → "Build" is active
	s.Orientation = toolkit.Vertical
	s.SetBounds(toolkit.Rect{X: 2, Y: 1, W: 10, H: 6}) // room for 3 labels + 2 connectors

	cp := painter.NewCellPainter(14, 9)
	s.Draw(cp, theme)
	at := func(x, y int) painter.Cell { return cp.Cells[y*cp.W+x] }

	// Row 0 (y=1): done step (i=0 < Current=1) → Border ink.
	if c := at(2, 1); c.Rune != '1' || c.Fg != theme.Border {
		t.Errorf("row0 = %+v, want rune '1' fg=Border", c)
	}
	// Connector row (y=2): "│" in Border ink.
	if c := at(2, 2); c.Rune != '│' || c.Fg != theme.Border {
		t.Errorf("connector0 = %+v, want rune '│' fg=Border", c)
	}
	// Row 1 (y=3): current step (i=1 == Current) → Accent ink.
	if c := at(2, 3); c.Rune != '2' || c.Fg != theme.Accent {
		t.Errorf("row1 = %+v, want rune '2' fg=Accent", c)
	}
	// Connector row (y=4).
	if c := at(2, 4); c.Rune != '│' || c.Fg != theme.Border {
		t.Errorf("connector1 = %+v, want rune '│' fg=Border", c)
	}
	// Row 2 (y=5): upcoming step (i=2 > Current) → OnSurface ink.
	if c := at(2, 5); c.Rune != '3' || c.Fg != theme.OnSurface {
		t.Errorf("row2 = %+v, want rune '3' fg=OnSurface", c)
	}

	// A height too short to fit every row clips: the loop breaks before
	// drawing the last step, and the dangling connector guard (y < r.Y+r.H)
	// skips the connector that would overflow. No panic either way.
	tight := NewSteps([]string{"Plan", "Build", "Ship"}, 1)
	tight.Orientation = toolkit.Vertical
	tight.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3}) // fits "Plan" + connector + "Build" only
	cpTight := painter.NewCellPainter(10, 3)
	tight.Draw(cpTight, theme)
	atT := func(x, y int) painter.Cell { return cpTight.Cells[y*cpTight.W+x] }
	if c := atT(0, 0); c.Rune != '1' {
		t.Errorf("clipped row0 = %+v, want rune '1'", c)
	}
	// The connector after "Build" (i=1, not last) never gets drawn because
	// H=3 leaves no row for it (y=3 is out of bounds); the loop then breaks
	// before "Ship" is ever reached.
	if c := atT(0, 2); c.Rune != '2' {
		t.Errorf("clipped row1 = %+v, want rune '2'", c)
	}
}
