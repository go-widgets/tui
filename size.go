// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"os"
	"strconv"
)

// DefaultCols and DefaultRows are the terminal dimensions used when
// neither the environment nor the caller supplies a size — the classic
// VT100 defaults every terminal emulator honours as a floor.
const (
	DefaultCols = 80
	DefaultRows = 24
)

// EnvSize reads the terminal size from the COLUMNS and LINES environment
// variables. It returns (cols, rows, true) when both parse cleanly and
// hold positive integers; otherwise it returns (0, 0, false) and the
// caller should substitute defaults.
//
// The env-vars-only strategy is a deliberate trade-off: it keeps the
// package stdlib-only (no TIOCGWINSZ ioctl per platform, no cgo, no
// golang.org/x/term dependency). Interactive shells that need dynamic
// sizes should export COLUMNS / LINES from their prompt hook, or pass
// an explicit size to [RenderOnceSized].
func EnvSize() (cols, rows int, ok bool) {
	c, cOK := parsePositive(os.Getenv("COLUMNS"))
	r, rOK := parsePositive(os.Getenv("LINES"))
	if !cOK || !rOK {
		return 0, 0, false
	}
	return c, r, true
}

// SizeOrDefault returns [EnvSize] when the environment reports a size,
// otherwise [DefaultCols]x[DefaultRows]. This is what [RenderOnce]
// consumes.
func SizeOrDefault() (cols, rows int) {
	if c, r, ok := EnvSize(); ok {
		return c, r
	}
	return DefaultCols, DefaultRows
}

// parsePositive parses s as a positive integer. Returns (0, false)
// on empty, invalid, or non-positive input.
func parsePositive(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
