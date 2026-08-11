// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end pty tests for keymap-demo, using cell-level assertions.
//
// Each test spawns the real binary under a pseudo-terminal, types a real key
// stream, decodes the final rendered frame into a cell grid, and asserts on the
// exact text in specific rows — proving that a keystroke drove the keymap to
// the RIGHT action (chord completion, scope resolution, hot rebind), not merely
// that "something reacted". Gated behind `integration` so `go test ./...` stays
// fast:
//
//	go test -tags integration ./cmd/keymap-demo/
package main

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/go-widgets/tui"
)

// buildBinary compiles keymap-demo into a temp dir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "keymap-demo.bin")
	c := exec.Command("go", "build", "-o", bin, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// captureFrame spawns the binary under a cols×rows pty, feeds keys one byte at
// a time (so the App repaints between mutations), waits for exit, and returns
// the decoded final frame.
func captureFrame(t *testing.T, cols, rows int, keys string, timeout time.Duration) *tui.TermGrid {
	t.Helper()
	bin := buildBinary(t)

	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, ptmx); close(done) }()

	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 0; i < len(keys); i++ {
			_, _ = ptmx.Write([]byte{keys[i]})
			time.Sleep(80 * time.Millisecond)
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()
	return lastFrame(buf.Bytes(), cols, rows)
}

// lastFrame decodes the frame active at exit — the tail after the last
// cursor-home CSI that begins every App frame.
func lastFrame(raw []byte, cols, rows int) *tui.TermGrid {
	if idx := bytes.LastIndex(raw, []byte("\x1b[H")); idx >= 0 {
		return tui.DecodeANSI(raw[idx:], cols, rows)
	}
	return tui.DecodeANSI(raw, cols, rows)
}

// rowContains fails unless row y of g contains want.
func rowContains(t *testing.T, g *tui.TermGrid, y int, want string) {
	t.Helper()
	if got := g.RowText(y); !strings.Contains(got, want) {
		t.Fatalf("row %d = %q, want it to contain %q", y, got, want)
	}
}

const (
	cols, rows = 64, 16
	ctrlS      = "\x13" // Ctrl+S
	tab        = "\t"   // Tab
)

// TestInitialFrameShowsLiveShortcuts proves the menu row is fed the actions'
// live accelerators from the keymap, the scope defaults to global, and nothing
// has fired yet. It also scans for stray block chars (the cell-precision
// discipline: a coloured run must not render as █).
func TestInitialFrameShowsLiveShortcuts(t *testing.T) {
	g := captureFrame(t, cols, rows, "q", 5*time.Second)
	rowContains(t, g, 0, "keymap-demo")
	rowContains(t, g, 2, "Save[Ctrl+S]")
	rowContains(t, g, 2, "GoToDef[G D]")
	rowContains(t, g, 4, "SCOPE=global")
	rowContains(t, g, 8, "LAST=none")
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			if g.At(x, y).Rune == '█' {
				t.Fatalf("stray block char at (%d,%d)", x, y)
			}
		}
	}
}

// TestAcceleratorCtrlSRunsSave: a Ctrl+S keystroke (a modifier chord) resolves
// to the Save action.
func TestAcceleratorCtrlSRunsSave(t *testing.T) {
	g := captureFrame(t, cols, rows, ctrlS+"q", 5*time.Second)
	rowContains(t, g, 8, "LAST=save")
	rowContains(t, g, 10, "MSG=saved")
}

// TestChordRunsGoDefs: the multi-stroke chord "g" then "d" resolves to GoToDef.
func TestChordRunsGoDefs(t *testing.T) {
	g := captureFrame(t, cols, rows, "gdq", 5*time.Second)
	rowContains(t, g, 8, "LAST=goDefs")
}

// TestChordPartialShowsPending: after only "g", the PENDING row shows the
// half-typed chord and nothing has fired (q is consumed by the quit binding, so
// it never disturbs the pending chord).
func TestChordPartialShowsPending(t *testing.T) {
	g := captureFrame(t, cols, rows, "gq", 5*time.Second)
	rowContains(t, g, 6, "PENDING=G")
	rowContains(t, g, 8, "LAST=none")
}

// TestScopeGlobalResolves: with no widget scope active, "a" resolves to the
// global AllItems.
func TestScopeGlobalResolves(t *testing.T) {
	g := captureFrame(t, cols, rows, "aq", 5*time.Second)
	rowContains(t, g, 8, "LAST=allItems")
}

// TestScopeWidgetShadowsGlobal: Tab activates the widget scope, and the SAME
// "a" now resolves to AllText — the widget binding shadows the global one.
func TestScopeWidgetShadowsGlobal(t *testing.T) {
	g := captureFrame(t, cols, rows, tab+"aq", 5*time.Second)
	rowContains(t, g, 4, "SCOPE=widget")
	rowContains(t, g, 8, "LAST=allText")
}

// TestHotRebind: "r" rebinds GoToDef to "z" at run time; the menu hint updates
// to [Z] and "z" then resolves to GoToDef.
func TestHotRebind(t *testing.T) {
	g := captureFrame(t, cols, rows, "rzq", 5*time.Second)
	rowContains(t, g, 2, "GoToDef[Z]")  // menu hint reflects the live rebind
	rowContains(t, g, 8, "LAST=goDefs") // the new "z" accelerator resolves it
}

// TestPaletteFedFromRegistry: "p" opens the command palette populated from the
// registry's visible actions (all 8).
func TestPaletteFedFromRegistry(t *testing.T) {
	g := captureFrame(t, cols, rows, "pq", 5*time.Second)
	rowContains(t, g, 12, "PALETTE n=8")
}
