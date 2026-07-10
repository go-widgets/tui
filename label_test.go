// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func TestLabelDraw(t *testing.T) {
	theme := toolkit.DefaultLight()
	// Left / center / right place the text at the expected start column; use a
	// CellPainter through the render path and inspect the row text.
	check := func(a Align, w int, wantStart int) {
		l := &Label{Text: "hi", Align: a}
		l.SetBounds(toolkit.Rect{X: 0, Y: 0, W: w, H: 1})
		cp := painter.NewCellPainter(w, 1)
		l.Draw(cp, theme)
		row := ""
		for x := 0; x < w; x++ {
			row += string(cp.Cells[x].Rune)
		}
		// wantStart is the index of 'h'.
		if got := indexOfRune(row, 'h'); got != wantStart {
			t.Errorf("align %d: 'h' at col %d, want %d (row %q)", a, got, wantStart, row)
		}
	}
	check(AlignLeft, 10, 0)   // left → col 0
	check(AlignCenter, 10, 4) // (10-2)/2 = 4
	check(AlignRight, 10, 8)  // 10-2 = 8

	// Overflow (text wider than bounds) clamps the start to col 0.
	wide := &Label{Text: "toolong", Align: AlignCenter}
	wide.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 1})
	cp := painter.NewCellPainter(4, 1)
	wide.Draw(cp, theme)
	if cp.Cells[0].Rune != 't' {
		t.Errorf("overflow clamp: col 0 = %q, want 't'", cp.Cells[0].Rune)
	}

	// Multi-row bounds vertically-centre the text (row 1 of 3); no panic.
	ml := NewLabel("mid")
	ml.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	ml.Draw(painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3), theme)
}

func indexOfRune(s string, r rune) int {
	for i, c := range []rune(s) {
		if c == r {
			return i
		}
	}
	return -1
}
