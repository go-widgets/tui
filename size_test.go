// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import "testing"

func TestEnvSizeReadsBothVars(t *testing.T) {
	t.Setenv("COLUMNS", "132")
	t.Setenv("LINES", "50")
	cols, rows, ok := EnvSize()
	if !ok {
		t.Fatal("EnvSize should have reported ok on well-formed COLUMNS/LINES")
	}
	if cols != 132 || rows != 50 {
		t.Fatalf("EnvSize = (%d, %d), want (132, 50)", cols, rows)
	}
}

func TestEnvSizeUnsetFails(t *testing.T) {
	t.Setenv("COLUMNS", "")
	t.Setenv("LINES", "")
	if _, _, ok := EnvSize(); ok {
		t.Fatal("EnvSize should report ok=false when both env vars are unset")
	}
}

func TestEnvSizeMalformedFails(t *testing.T) {
	// COLUMNS is malformed → whole call fails, even though LINES is fine.
	t.Setenv("COLUMNS", "not-a-number")
	t.Setenv("LINES", "24")
	if _, _, ok := EnvSize(); ok {
		t.Fatal("EnvSize should report ok=false when COLUMNS does not parse")
	}
	// Symmetric: LINES malformed → whole call fails.
	t.Setenv("COLUMNS", "80")
	t.Setenv("LINES", "-5")
	if _, _, ok := EnvSize(); ok {
		t.Fatal("EnvSize should report ok=false when LINES is non-positive")
	}
}

func TestSizeOrDefaultFromEnv(t *testing.T) {
	t.Setenv("COLUMNS", "132")
	t.Setenv("LINES", "50")
	cols, rows := SizeOrDefault()
	if cols != 132 || rows != 50 {
		t.Fatalf("SizeOrDefault = (%d, %d), want env (132, 50)", cols, rows)
	}
}

func TestSizeOrDefaultFallback(t *testing.T) {
	t.Setenv("COLUMNS", "")
	t.Setenv("LINES", "")
	cols, rows := SizeOrDefault()
	if cols != DefaultCols || rows != DefaultRows {
		t.Fatalf("SizeOrDefault fallback = (%d, %d), want (%d, %d)",
			cols, rows, DefaultCols, DefaultRows)
	}
}

func TestParsePositiveEmpty(t *testing.T) {
	if _, ok := parsePositive(""); ok {
		t.Fatal("parsePositive(\"\") should report ok=false")
	}
}

func TestParsePositiveInvalid(t *testing.T) {
	if _, ok := parsePositive("nope"); ok {
		t.Fatal("parsePositive(\"nope\") should report ok=false")
	}
}

func TestParsePositiveZero(t *testing.T) {
	if _, ok := parsePositive("0"); ok {
		t.Fatal("parsePositive(\"0\") should report ok=false (non-positive)")
	}
}

func TestParsePositiveValid(t *testing.T) {
	n, ok := parsePositive("42")
	if !ok || n != 42 {
		t.Fatalf("parsePositive(\"42\") = (%d, %v), want (42, true)", n, ok)
	}
}
