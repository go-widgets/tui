// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestLevelBar(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }
	theme := toolkit.DefaultLight()

	// NewLevelBar floors Max at 1.
	if l := NewLevelBar(0); l.Max != 1 {
		t.Errorf("NewLevelBar(0) Max = %d, want 1", l.Max)
	}

	// Some filled + some empty segments.
	l := NewLevelBar(5)
	l.Value = 3
	l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	l.Draw(mk(20, 1), theme)

	// Narrow bounds floor cellW at 1.
	narrow := NewLevelBar(10)
	narrow.Value = 4
	narrow.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	narrow.Draw(mk(4, 1), theme)

	// Max < 1 (set directly on the field) draws nothing.
	zero := &LevelBar{Max: 0}
	zero.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	zero.Draw(mk(20, 1), theme)
}
