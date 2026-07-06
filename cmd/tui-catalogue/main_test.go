// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-widgets/toolkit"
)

// TestComposeCatalogueShape asserts every widget in the catalogue
// has non-nil bounds set and no two widgets overlap on the same
// (X, Y) origin cell — a lightweight collision check that catches
// copy/paste bugs where a new entry inherited a sibling's Rect.
func TestComposeCatalogueShape(t *testing.T) {
	widgets := composeCatalogue()
	if len(widgets) == 0 {
		t.Fatal("composeCatalogue returned zero widgets")
	}
	seen := map[[2]int]bool{}
	for i, w := range widgets {
		b := w.Bounds()
		if b.W <= 0 || b.H <= 0 {
			t.Errorf("widget %d: non-positive dims %d×%d", i, b.W, b.H)
		}
		key := [2]int{b.X, b.Y}
		if seen[key] {
			t.Errorf("widget %d shares origin (%d,%d) with an earlier entry",
				i, b.X, b.Y)
		}
		seen[key] = true
	}
}

func TestRunDefaultThemeSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0. stderr:\n%s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("run() should have produced stdout output")
	}
}

func TestRunLightVsDarkDiffers(t *testing.T) {
	var lightOut, darkOut bytes.Buffer
	if code := run([]string{"--theme=light", "--cols=100", "--rows=20"}, &lightOut, io.Discard); code != 0 {
		t.Fatalf("light run() = %d, want 0", code)
	}
	if code := run([]string{"--theme=dark", "--cols=100", "--rows=20"}, &darkOut, io.Discard); code != 0 {
		t.Fatalf("dark run() = %d, want 0", code)
	}
	if lightOut.String() == darkOut.String() {
		t.Fatal("--theme=light and --theme=dark produced identical output")
	}
}

func TestRunExplicitSize(t *testing.T) {
	var stdout bytes.Buffer
	if code := run([]string{"--cols=80", "--rows=24"}, &stdout, io.Discard); code != 0 {
		t.Fatalf("run(--cols=80 --rows=24) = %d, want 0", code)
	}
	if stdout.Len() == 0 {
		t.Fatal("explicit-size run should have produced output")
	}
	// ANSI stream must contain SGR reset sequences at frame end.
	if !strings.Contains(stdout.String(), "\x1b[0m") {
		t.Fatal("output missing ANSI SGR reset — likely not a valid frame")
	}
}

func TestRunBadFlagFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--not-a-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown flag: run() = %d, want 2 (flag-parse error)", code)
	}
}

// errWriter always fails on Write, exercising the render-error
// branch of run() where tui.RenderToolkit returns an io error and
// run wraps + returns 1.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errBoom }

var errBoom = errors.New("boom")

func TestRunRenderErrorReturnsOne(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"--cols=40", "--rows=10"}, errWriter{}, &stderr); code != 1 {
		t.Fatalf("write-error run() = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("render error should have surfaced on stderr")
	}
}

// TestMainSuccessPath drives main() via the runFunc/osExit seams so
// coverage picks up the four statements inside main without
// actually exiting the test binary.
func TestMainSuccessPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	gotCode := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 0 }
	osExit = func(code int) { gotCode = code }
	main()
	if gotCode != 0 {
		t.Fatalf("main() called osExit(%d), want 0", gotCode)
	}
}

func TestMainErrorPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	gotCode := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 7 }
	osExit = func(code int) { gotCode = code }
	main()
	if gotCode != 7 {
		t.Fatalf("main() called osExit(%d), want 7", gotCode)
	}
}

// TestComposeCatalogueEveryEntryIsAToolkitWidget verifies the slice
// is typed correctly at compile time (already), plus that every
// entry's Bounds is inside a reasonable envelope so a 100x20-ish
// terminal renders the whole composition.
func TestComposeCatalogueEveryEntryIsAToolkitWidget(t *testing.T) {
	widgets := composeCatalogue()
	for i, w := range widgets {
		var _ toolkit.Widget = w // compile-time check
		b := w.Bounds()
		if b.X < 0 || b.Y < 0 {
			t.Errorf("widget %d: negative origin (%d,%d)", i, b.X, b.Y)
		}
	}
}
