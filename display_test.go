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
