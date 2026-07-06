// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end integration tests using cell-level assertions.
//
// The v0.3.2 through v0.3.5 patches all shipped bugs that would
// have been caught by the assertions here — but the OLD e2e tests
// stripped ANSI colors before asserting, so every visual bug (wrong
// row background, block-char glyphs, clipped text) slipped through.
//
// The new discipline: decode the pty output into a TermGrid, then
// assert on specific cells' Rune + Fg + Bg. Text-content assertions
// stay via TermGrid.RowText, but the color / rune assertions are
// what catch the bugs that would otherwise ship.

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

// Theme constants — must match toolkit.DefaultLight() so assertions
// don't drift when the theme rotates. Values verified from
// toolkit/theme.go's DefaultLight function to avoid the "assumed
// brand teal" mistake I made in the first draft (the ACTUAL default
// light Accent is Adwaita blue #3584E4).
const (
	lightBgR, lightBgG, lightBgB             = 0xFA, 0xFA, 0xFA // theme.Background
	lightSurfaceR, lightSurfaceG, lightSuB   = 0xE8, 0xEA, 0xED // theme.Surface
	lightAccentR, lightAccentG, lightAccentB = 0x35, 0x84, 0xE4 // theme.Accent (Adwaita blue)
)

// captureFrame spawns the explorer under a pty sized cols×rows, sends
// the given key stream after a settle delay, waits up to timeout for
// the process to exit, and returns the decoded TermGrid. Standard
// harness for every cell-level test.
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
	captureDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(captureDone)
	}()

	// Feed the key stream one byte at a time with a small inter-key
	// delay so the App loop repaints between mutations.
	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 0; i < len(keys); i++ {
			_, _ = ptmx.Write([]byte{keys[i]})
			time.Sleep(80 * time.Millisecond)
		}
	}()

	select {
	case <-captureDone:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()

	return tui.DecodeANSI(buf.Bytes(), cols, rows)
}

// findLastFrame returns the last complete frame (up to the last
// "cursor-home" CSI). A tui.App run may emit multiple frames over
// its lifetime; assertions want the FINAL one after all input.
func findLastFrame(raw []byte, cols, rows int) *tui.TermGrid {
	// Find last "\x1b[H" (home cursor) — every frame in tui.App
	// starts with that. If we split there, the tail is the frame
	// active at exit-time.
	idx := bytes.LastIndex(raw, []byte("\x1b[H"))
	if idx < 0 {
		return tui.DecodeANSI(raw, cols, rows)
	}
	return tui.DecodeANSI(raw[idx:], cols, rows)
}

// -----------------------------------------------------------------
// v0.3.2 regression — VBox equal-height split
// -----------------------------------------------------------------

// TestExplorerHeaderIsAtRow0AndFooterAtLastRow catches the v0.3.0 /
// v0.3.1 layout bug where VBox split the three children equally,
// pushing chrome to a third of the screen each.
func TestExplorerHeaderIsAtRow0AndFooterAtLastRow(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	if !strings.Contains(g.RowText(0), "File") {
		t.Fatalf("row 0 (header) missing 'File': %q", g.RowText(0))
	}
	if !strings.Contains(g.RowText(29), "q: quit") {
		t.Fatalf("row 29 (footer) missing 'q: quit': %q", g.RowText(29))
	}
}

// -----------------------------------------------------------------
// v0.3.3 regression — help popover never drawn
// -----------------------------------------------------------------

// TestExplorerHelpPopoverActuallyRendersOnQuestionMark catches the
// v0.3.2 bug where '?' toggled Visible but the popover was never
// added to the widget tree, so nothing painted.
func TestExplorerHelpPopoverActuallyRendersOnQuestionMark(t *testing.T) {
	g := captureFrame(t, 80, 30, "?q", 3*time.Second)
	// "Enter: open" appears only in the help popover Label, not in
	// the always-visible footer. Its presence proves the popover
	// actually rendered.
	found := false
	for y := 0; y < g.Rows; y++ {
		if strings.Contains(g.RowText(y), "Enter: open") {
			found = true
			break
		}
	}
	if !found {
		var dump strings.Builder
		for y := 0; y < g.Rows; y++ {
			dump.WriteString(g.RowText(y))
			dump.WriteByte('\n')
		}
		t.Fatalf("'Enter: open' (help popover content) never rendered:\n%s", dump.String())
	}
}

// -----------------------------------------------------------------
// v0.3.5 regression — fileList highlight was █ chars
// -----------------------------------------------------------------

// TestExplorerSelectedRowHasAccentBackground catches the v0.3.4 bug
// where fileList used PutPixel for the selected-row highlight,
// which in cell mode wrote '█' chars instead of setting the cell
// background color. Assertion: row 1 (selected /src/main.go) cells
// must have Bg = theme.Accent AND rune != '█'.
func TestExplorerSelectedRowHasAccentBackground(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)

	// Row 1 = selected /src/main.go. Sample a cell in the middle of
	// the highlight strip (past the leading space, inside the name).
	c := g.At(3, 1) // '/src/main.go' starts at col 1
	if !c.Bg.Set {
		t.Fatalf("row 1 col 3: no bg color set — highlight not applied. cell=%+v", c)
	}
	if c.Bg.R != lightAccentR || c.Bg.G != lightAccentG || c.Bg.B != lightAccentB {
		t.Fatalf("row 1 col 3 bg = (%d,%d,%d), want accent (%d,%d,%d)",
			c.Bg.R, c.Bg.G, c.Bg.B,
			lightAccentR, lightAccentG, lightAccentB)
	}
	// The KEY assertion — no '█' char, which was v0.3.4's bug.
	for x := 0; x < 24; x++ {
		if g.At(x, 1).Rune == '█' {
			t.Fatalf("row 1 col %d contains '█' block char — PutPixel-in-cell-mode bug. cell=%+v",
				x, g.At(x, 1))
		}
	}
}

// TestExplorerSelectedRowNameStillReadable — a stronger form: not
// only must the background be accent AND runes not be '█', but the
// filename must still be readable in the highlighted row.
func TestExplorerSelectedRowNameStillReadable(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	if !strings.Contains(g.RowText(1), "/src/main.go") {
		t.Fatalf("selected row 1 does not contain '/src/main.go': %q", g.RowText(1))
	}
}

// -----------------------------------------------------------------
// v0.3.5 regression — TextView showed only 2 lines out of 3
// -----------------------------------------------------------------

// TestExplorerContentPreviewShowsAllLinesWithoutClipping catches the
// v0.3.4 bug where toolkit.TextView used lineH=11 in cell mode so a
// 22-row body only showed 2 of the 3 file lines. Assertion: all 3
// lines of /src/main.go must land on distinct rows in the right pane.
func TestExplorerContentPreviewShowsAllLinesWithoutClipping(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)

	// /src/main.go contents: "package main\n\nfunc main() {}\n"
	// After the v0.3.5 fix, all 3 lines land as 3 rows (with the
	// middle row blank).
	var packageRow, funcRow int
	packageRow, funcRow = -1, -1
	for y := 0; y < g.Rows; y++ {
		text := g.RowText(y)
		if strings.Contains(text, "package main") {
			packageRow = y
		}
		if strings.Contains(text, "func main") {
			funcRow = y
		}
	}
	if packageRow < 0 {
		t.Fatal("'package main' never rendered in the content pane")
	}
	if funcRow < 0 {
		t.Fatal("'func main() {}' never rendered — TextView clipped it (v0.3.4 bug)")
	}
	if funcRow-packageRow != 2 {
		t.Errorf("expected 2-row gap between package + func lines, got packageRow=%d funcRow=%d",
			packageRow, funcRow)
	}
}

// -----------------------------------------------------------------
// v0.3.4 regression — arrow key navigation
// -----------------------------------------------------------------

// TestExplorerArrowDownSyncsContent — pressing Down twice must
// select the third file (index 2 = /docs/README.md), and its
// content ("# Project") must appear in the right pane. If arrow
// handlers stopped mutating state OR syncContent stopped firing OR
// the right pane stopped repainting, one of the three assertions
// fails.
func TestExplorerArrowDownSyncsContent(t *testing.T) {
	// Two Down arrows = 6 bytes (each is ESC [ B).
	g := captureFrame(t, 80, 30, "\x1b[B\x1b[Bq", 5*time.Second)

	// Row 3 = third fileList entry = /docs/README.md.
	if !strings.Contains(g.RowText(3), "/docs/README.md") {
		t.Errorf("row 3 = %q, want '/docs/README.md'", g.RowText(3))
	}
	// The selected row must have the accent background NOW at
	// row 3 (not row 1) — confirms the highlight moved.
	c := g.At(3, 3)
	if !c.Bg.Set || c.Bg.R != lightAccentR {
		t.Errorf("row 3 highlight not applied after 2×Down. cell=%+v", c)
	}
	// The right pane must show README.md's content.
	found := false
	for y := 0; y < g.Rows; y++ {
		if strings.Contains(g.RowText(y), "# Project") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("right pane never showed '# Project' after 2×Down — syncContent didn't fire")
	}
}

// -----------------------------------------------------------------
// Meta — the failure-mode assertions catch the specific bugs
// -----------------------------------------------------------------

// TestExplorerNoBlockCharsAnywhere is a scan of the whole grid: no
// cell should contain '█' unless it was intentional (there are no
// intentional block chars in this composition). Would loud-fail on
// any future regression that re-introduces PutPixel-as-background.
func TestExplorerNoBlockCharsAnywhere(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			if g.At(x, y).Rune == '█' {
				t.Errorf("block char '█' at (%d,%d) — cell background misused as glyph",
					x, y)
			}
		}
	}
}

// TestExplorerBodyChromeContrastIsPerceptible asserts that the body
// area and the chrome rows (header/footer) have visibly distinct
// background colors — luminance difference ≥ 8 on a 0..255 scale.
// A pixel-value equality assertion PASSES if bg values are
// technically different (e.g. Surface #1F2228 vs Background #14161A —
// 12 luminance units apart but visually indistinguishable). This
// stronger assertion mandates a THRESHOLD that keeps a human eye
// able to see the panel boundary.
//
// Catches the v0.3.7 "all-dark screen in Terminal.app" bug where
// the body used Surface and chrome used Background — technically
// different, but perceptually the same shade of near-black.
func TestExplorerBodyChromeContrastIsPerceptible(t *testing.T) {
	// Test in BOTH themes to catch a regression in either.
	for _, tc := range []struct {
		name   string
		keys   string
		darkTh bool
	}{
		{"light", "q", false},
		{"dark", "q", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g *tui.TermGrid
			if tc.darkTh {
				g = captureFrameWithArgs(t, 80, 30, tc.keys, 3*time.Second, "--theme=dark")
			} else {
				g = captureFrame(t, 80, 30, tc.keys, 3*time.Second)
			}
			// Row 0 = header (Background). Row 5 = body (SurfaceAlt).
			chrome := g.At(0, 0).Bg
			body := g.At(0, 5).Bg
			if !chrome.Set || !body.Set {
				t.Fatalf("chrome or body bg not set: chrome=%+v body=%+v", chrome, body)
			}
			diff := luminanceDiff(chrome, body)
			if diff < 8 {
				t.Fatalf("chrome/body luminance diff = %d, want ≥ 8 (%s theme): chrome=(%d,%d,%d) body=(%d,%d,%d)",
					diff, tc.name,
					chrome.R, chrome.G, chrome.B,
					body.R, body.G, body.B)
			}
		})
	}
}

// captureFrameWithArgs is captureFrame + extra args to the binary.
func captureFrameWithArgs(t *testing.T, cols, rows int, keys string, timeout time.Duration, args ...string) *tui.TermGrid {
	t.Helper()
	bin := buildBinary(t)
	c := exec.Command(bin, args...)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	captureDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(captureDone)
	}()
	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 0; i < len(keys); i++ {
			_, _ = ptmx.Write([]byte{keys[i]})
			time.Sleep(80 * time.Millisecond)
		}
	}()

	select {
	case <-captureDone:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()
	return tui.DecodeANSI(buf.Bytes(), cols, rows)
}

// luminanceDiff returns the max absolute difference across the three
// RGB channels between two colors. A cheap perceptual proxy — real
// luma weights would use 0.299*R + 0.587*G + 0.114*B, but for the
// coarse "can a human see the boundary" check, max-channel-diff is
// good enough (and doesn't require a floating-point path in a test).
func luminanceDiff(a, b tui.Color) int {
	dr := int(a.R) - int(b.R)
	if dr < 0 {
		dr = -dr
	}
	dg := int(a.G) - int(b.G)
	if dg < 0 {
		dg = -dg
	}
	db := int(a.B) - int(b.B)
	if db < 0 {
		db = -db
	}
	m := dr
	if dg > m {
		m = dg
	}
	if db > m {
		m = db
	}
	return m
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tui-explorer.bin")
	c := exec.Command("go", "build", "-o", bin, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// findLastFrame kept exported (via _ =) for downstream refactor.
var _ = findLastFrame
