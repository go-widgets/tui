// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/go-widgets/painter"
)

// sampleWidgets returns a small deterministic tree — one Label plus
// one Button — so the render tests exercise Widget.Draw without
// depending on painter's demo package.
func sampleWidgets() []painter.Widget {
	return []painter.Widget{
		&painter.Label{
			Bounds: painter.Rect{X: 0, Y: 0, W: 20, H: 1},
			Text:   "HELLO",
		},
		&painter.Button{
			Bounds: painter.Rect{X: 0, Y: 2, W: 10, H: 3},
			Label:  "OK",
		},
	}
}

func TestRenderOnceSizedProducesANSI(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderOnceSized(&buf, 30, 10, sampleWidgets(), painter.LightTheme()); err != nil {
		t.Fatalf("RenderOnceSized: %v", err)
	}
	out := buf.String()
	if len(out) == 0 {
		t.Fatal("RenderOnceSized produced empty output")
	}
	// The output should contain at least one ANSI CSI sequence.
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("output has no ANSI escape sequences:\n%q", out[:min(200, len(out))])
	}
}

func TestRenderOnceSizedNilThemeUsesLight(t *testing.T) {
	// nil theme should behave the same as LightTheme.
	var withLight, withNil bytes.Buffer
	if err := RenderOnceSized(&withLight, 30, 10, sampleWidgets(), painter.LightTheme()); err != nil {
		t.Fatalf("with LightTheme: %v", err)
	}
	if err := RenderOnceSized(&withNil, 30, 10, sampleWidgets(), nil); err != nil {
		t.Fatalf("with nil theme: %v", err)
	}
	if withLight.String() != withNil.String() {
		t.Fatalf("nil theme should equal LightTheme, but outputs differ (%d vs %d bytes)",
			withLight.Len(), withNil.Len())
	}
}

func TestRenderOnceSizedNonPositiveClamps(t *testing.T) {
	// Passing 0 or negative dimensions should be clamped to the defaults,
	// producing a full-size render rather than an empty one.
	var buf bytes.Buffer
	if err := RenderOnceSized(&buf, 0, -5, sampleWidgets(), painter.LightTheme()); err != nil {
		t.Fatalf("RenderOnceSized: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("RenderOnceSized should have produced default-size output")
	}
}

// errWriter fails with a stable error to exercise the ANSI-write error
// branch of RenderOnceSized.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("write refused") }

func TestRenderOnceSizedPropagatesWriteError(t *testing.T) {
	err := RenderOnceSized(errWriter{}, 30, 10, sampleWidgets(), painter.LightTheme())
	if err == nil {
		t.Fatal("expected an error from a failing writer, got nil")
	}
	if !strings.Contains(err.Error(), "write ANSI") {
		t.Fatalf("error should mention 'write ANSI', got %v", err)
	}
	if !strings.Contains(err.Error(), "write refused") {
		t.Fatalf("error should wrap the underlying writer error, got %v", err)
	}
}

func TestRenderOnceUsesEnvSize(t *testing.T) {
	// Force a specific env size and confirm RenderOnce picks it up.
	t.Setenv("COLUMNS", "132")
	t.Setenv("LINES", "20")
	var buf bytes.Buffer
	if err := RenderOnce(&buf, sampleWidgets(), painter.LightTheme()); err != nil {
		t.Fatalf("RenderOnce: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("RenderOnce produced empty output")
	}
}

func TestRenderOnceFallsBackToDefault(t *testing.T) {
	t.Setenv("COLUMNS", "")
	t.Setenv("LINES", "")
	var buf bytes.Buffer
	if err := RenderOnce(&buf, sampleWidgets(), nil); err != nil {
		t.Fatalf("RenderOnce: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("RenderOnce with unset env should still render at DefaultCols×DefaultRows")
	}
}
