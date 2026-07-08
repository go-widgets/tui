// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestMenuBar(t *testing.T) {
	var fired string
	mb := NewMenuBar(
		MenuItem{Label: "File", OnClick: func() { fired = "File" }},
		MenuItem{Label: "Edit", OnClick: func() { fired = "Edit" }},
		MenuItem{Label: "Help"}, // nil OnClick
	)
	mb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 1})

	// Layout: pad=1 -> "File" at [1,5); sep=3 -> "Edit" at [8,12);
	// -> "Help" at [15,19).
	if x0, x1 := mb.ItemXRange(0); x0 != 1 || x1 != 5 {
		t.Errorf("File range = (%d,%d), want (1,5)", x0, x1)
	}
	if x0, x1 := mb.ItemXRange(1); x0 != 8 || x1 != 12 {
		t.Errorf("Edit range = (%d,%d), want (8,12)", x0, x1)
	}
	if x0, x1 := mb.ItemXRange(9); x0 != -1 || x1 != -1 {
		t.Errorf("out-of-range = (%d,%d), want (-1,-1)", x0, x1)
	}

	// Click inside "Edit" fires its handler.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 9, Y: 0})
	if fired != "Edit" {
		t.Errorf("click Edit: fired = %q, want Edit", fired)
	}
	// Click in the gap between items fires nothing.
	fired = ""
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 0})
	if fired != "" {
		t.Errorf("gap click fired %q", fired)
	}
	// Click on a nil-OnClick item is a no-op (does not panic).
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 16, Y: 0})
	// A non-click event is ignored.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})

	// Draw renders without panicking (background + labels).
	mb.Draw(painter.NewPixelPainter(make([]byte, 40*1*4), 40, 1), toolkit.DefaultLight())
}
