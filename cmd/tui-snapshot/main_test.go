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

func TestRunDefaultThemeSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, want 0. stderr:\n%s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("run() should have produced stdout output")
	}
}

func TestRunDarkTheme(t *testing.T) {
	var lightOut, darkOut bytes.Buffer
	if code := run([]string{"--theme=light"}, &lightOut, io.Discard); code != 0 {
		t.Fatalf("light run() = %d, want 0", code)
	}
	if code := run([]string{"--theme=dark"}, &darkOut, io.Discard); code != 0 {
		t.Fatalf("dark run() = %d, want 0", code)
	}
	// Light and dark themes must produce different ANSI streams (different
	// background colours propagate everywhere).
	if lightOut.String() == darkOut.String() {
		t.Fatal("--theme=light and --theme=dark produced identical output")
	}
}

func TestRunExplicitSize(t *testing.T) {
	var stdout bytes.Buffer
	if code := run([]string{"--cols=40", "--rows=10"}, &stdout, io.Discard); code != 0 {
		t.Fatalf("run(--cols=40 --rows=10) = %d, want 0", code)
	}
	if stdout.Len() == 0 {
		t.Fatal("explicit-size run should have produced output")
	}
}

func TestRunBadFlagFails(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--nonexistent-flag"}, io.Discard, &stderr)
	if code == 0 {
		t.Fatal("run() with bad flag should have returned non-zero")
	}
	// flag.ContinueOnError writes usage to fs.Output() (stderr).
	if !strings.Contains(stderr.String(), "flag") {
		t.Fatalf("stderr should mention the flag error, got %q", stderr.String())
	}
}

// errWriter fails with a stable error to exercise the render-fail branch
// of run().
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("stdout is dead") }

func TestRunRenderErrorReturnsOne(t *testing.T) {
	var stderr bytes.Buffer
	code := run(nil, errWriter{}, &stderr)
	if code != 1 {
		t.Fatalf("run() with failing stdout = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "stdout is dead") {
		t.Fatalf("stderr should surface the underlying writer error, got %q", stderr.String())
	}
}

// TestMainSuccessPath drives main() through the runFunc/osExit seams
// so os.Exit is not actually invoked.
func TestMainSuccessPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	got := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 0 }
	osExit = func(code int) { got = code }
	main()
	if got != 0 {
		t.Fatalf("main() called osExit(%d), want 0", got)
	}
}

func TestMainErrorPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	got := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 1 }
	osExit = func(code int) { got = code }
	main()
	if got != 1 {
		t.Fatalf("main() called osExit(%d), want 1", got)
	}
}
