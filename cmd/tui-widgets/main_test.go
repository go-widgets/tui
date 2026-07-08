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
)

// TestEntriesShape verifies every entry has a non-empty unique name, positive
// dims that fit a slot, and a non-nil make() returning a non-nil widget.
func TestEntriesShape(t *testing.T) {
	all := entries()
	if len(all) == 0 {
		t.Fatal("entries() returned zero widgets")
	}
	seen := map[string]bool{}
	for i, e := range all {
		if e.name == "" {
			t.Errorf("entry %d has empty name", i)
		}
		if seen[e.name] {
			t.Errorf("entry %d has duplicate name %q", i, e.name)
		}
		seen[e.name] = true
		if e.w <= 0 || e.h <= 0 {
			t.Errorf("entry %q has non-positive dims %d×%d", e.name, e.w, e.h)
		}
		if e.w > slotW || e.h > slotH {
			t.Errorf("entry %q dims %d×%d overflow the slot %d×%d", e.name, e.w, e.h, slotW, slotH)
		}
		if e.make == nil {
			t.Errorf("entry %q has nil make()", e.name)
			continue
		}
		if w := e.make(); w == nil {
			t.Errorf("entry %q make() returned nil", e.name)
		}
	}
}

func TestFindEntry(t *testing.T) {
	all := entries()
	if _, ok := findEntry(all, "table"); !ok {
		t.Fatal("findEntry(table) missing")
	}
	if _, ok := findEntry(all, "no-such-widget"); ok {
		t.Fatal("findEntry(unknown) should have returned false")
	}
}

func TestComposeAllShape(t *testing.T) {
	all := entries()
	widgets, cols, rows := composeAll(all)
	if got, want := len(widgets), 2*len(all); got != want {
		t.Errorf("composeAll returned %d widgets, want %d (caption + widget per entry)", got, want)
	}
	if cols < 2*slotW {
		t.Errorf("frame cols %d < 2*slotW %d", cols, 2*slotW)
	}
	expectRows := ((len(all) + gridCols - 1) / gridCols) * (captionH + slotH)
	if rows != expectRows {
		t.Errorf("frame rows %d, want %d", rows, expectRows)
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
	if code := run([]string{"--theme=light", "--cols=82", "--rows=40"}, &lightOut, io.Discard); code != 0 {
		t.Fatalf("light run() = %d, want 0", code)
	}
	if code := run([]string{"--theme=dark", "--cols=82", "--rows=40"}, &darkOut, io.Discard); code != 0 {
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
	if !strings.Contains(stdout.String(), "\x1b[0m") {
		t.Fatal("output missing ANSI SGR reset — likely not a valid frame")
	}
}

func TestRunList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--list) = %d, want 0", code)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != len(entries()) {
		t.Errorf("--list printed %d lines, want %d", len(lines), len(entries()))
	}
	found := false
	for _, l := range lines {
		if l == "treeview" {
			found = true
		}
	}
	if !found {
		t.Fatal("--list output missing sentinel 'treeview'")
	}
}

func TestRunWidgetSingle(t *testing.T) {
	var stdout bytes.Buffer
	if code := run([]string{"--widget=table"}, &stdout, io.Discard); code != 0 {
		t.Fatalf("run(--widget=table) = %d, want 0", code)
	}
	if stdout.Len() == 0 {
		t.Fatal("--widget render produced no output")
	}
}

func TestRunWidgetUnknown(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--widget=no-such-thing"}, &stdout, &stderr); code != 3 {
		t.Fatalf("run(--widget=unknown) = %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "unknown widget") {
		t.Fatalf("stderr missing 'unknown widget' hint: %q", stderr.String())
	}
}

func TestRunBadFlagFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--not-a-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("unknown flag: run() = %d, want 2", code)
	}
}

// errWriter always fails on Write, exercising the render-error branch of run().
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
