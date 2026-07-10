// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestPaginationNewAndSetPage(t *testing.T) {
	p := NewPagination(5, 3)
	if p.Count != 5 || p.Page != 3 {
		t.Fatalf("New: count=%d page=%d", p.Count, p.Page)
	}
	// Count floored at 1; page clamped into range.
	if got := NewPagination(0, 9); got.Count != 1 || got.Page != 1 {
		t.Errorf("floor/clamp: count=%d page=%d", got.Count, got.Page)
	}

	changed := -1
	p.OnChange = func(pg int) { changed = pg }
	p.SetPage(1)
	if p.Page != 1 || changed != 1 {
		t.Errorf("SetPage(1): page=%d changed=%d", p.Page, changed)
	}
	p.SetPage(-5) // clamp low, already at 1 → no change/no fire
	changed = -1
	p.SetPage(0)
	if p.Page != 1 || changed != -1 {
		t.Errorf("clamp-low no-change fired: page=%d changed=%d", p.Page, changed)
	}
	p.SetPage(99) // clamp high to Count
	if p.Page != 5 {
		t.Errorf("clamp high: page=%d", p.Page)
	}
	// No OnChange handler must not panic.
	NewPagination(3, 1).SetPage(2)
}

func TestPaginationEvents(t *testing.T) {
	p := NewPagination(4, 2)
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 9, H: 1})

	// Keys: Right/Left step.
	p.OnEvent(vkey("Right"))
	if p.Page != 3 {
		t.Fatalf("Right: page=%d, want 3", p.Page)
	}
	p.OnEvent(vkey("Left"))
	if p.Page != 2 {
		t.Fatalf("Left: page=%d, want 2", p.Page)
	}
	// Clicks: left cap (x<=1) prev, right cap (x>=W-2) next, middle no-op.
	p.OnEvent(vclick(0, 0))
	if p.Page != 1 {
		t.Fatalf("left cap: page=%d, want 1", p.Page)
	}
	p.OnEvent(vclick(8, 0))
	if p.Page != 2 {
		t.Fatalf("right cap: page=%d, want 2", p.Page)
	}
	p.OnEvent(vclick(4, 0)) // middle
	if p.Page != 2 {
		t.Errorf("middle click changed page: %d", p.Page)
	}
	// Unrelated key ignored.
	p.OnEvent(vkey("Enter"))
	if p.Page != 2 {
		t.Errorf("Enter changed page: %d", p.Page)
	}
}

func TestPaginationDraw(t *testing.T) {
	p := NewPagination(8, 3)
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 9, H: 1})
	p.Draw(painter.NewPixelPainter(make([]byte, 9*4), 9, 1), toolkit.DefaultLight())
}
