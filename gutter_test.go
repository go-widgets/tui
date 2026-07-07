// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

func TestGutterWidth(t *testing.T) {
	cases := map[int]int{
		0:    3, // n<1 clamps to 1 digit -> min 2 + 1
		1:    3, // 1 digit -> min 2 + 1
		9:    3,
		10:   3, // 2 digits + 1
		99:   3,
		100:  4, // 3 digits + 1
		1234: 5, // 4 digits + 1
	}
	for lines, want := range cases {
		if got := GutterWidth(lines); got != want {
			t.Errorf("GutterWidth(%d) = %d, want %d", lines, got, want)
		}
	}
}

func TestLineNumberInk(t *testing.T) {
	th := toolkit.DefaultLight()
	got := LineNumberInk(th)
	if got != (toolkit.RGBA{R: th.Border.R, G: th.Border.G, B: th.Border.B, A: 0xFF}) {
		t.Errorf("LineNumberInk = %+v, want opaque Border", got)
	}
}
