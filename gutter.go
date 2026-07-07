// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import "github.com/go-widgets/toolkit"

// GutterWidth returns the cell width of a line-number gutter for a code pane of
// lineCount lines: the number of digits in the largest line number (a minimum
// of 2) plus one cell of separation before the code. Shared by tui-explorer +
// tui-editor so their gutters line up.
func GutterWidth(lineCount int) int {
	n := lineCount
	if n < 1 {
		n = 1
	}
	digits := 0
	for n > 0 {
		digits++
		n /= 10
	}
	if digits < 2 {
		digits = 2
	}
	return digits + 1
}

// LineNumberInk is the muted ink for a line-number gutter: the theme's Border
// colour, which reads as a dim separator tone against any pane background.
func LineNumberInk(theme *toolkit.Theme) toolkit.RGBA {
	return toolkit.RGBA{R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: 0xFF}
}
