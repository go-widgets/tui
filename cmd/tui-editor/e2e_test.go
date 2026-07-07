// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end integration test that builds cmd/tui-editor, spawns
// it under a real pty, writes 'q' to quit, captures the initial
// frame, and asserts the header + footer land at row 0 and row 29
// respectively.
//
// Enable with: go test -tags integration ./cmd/tui-editor/...
//
// Companion to cmd/tui-explorer/e2e_test.go — the same regression
// caught there (VBox equal-height split) also affected the editor
// since it used the identical VBox composition. The layout fix that
// shipped in v0.3.2 replaced VBox with a local packedVBox helper on
// both demos; this test proves it worked in the editor.

package main

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/go-widgets/tui"
)

func TestEditorRendersHeaderAndFooterInPty(t *testing.T) {
	bin := buildBinary(t)

	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// The editor starts in VIEW mode; 'q' quits directly.
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = ptmx.Write([]byte("q"))
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("binary did not exit within 3s of receiving 'q'")
	}
	_ = c.Wait()

	// Decode into a cell grid — since painter v0.1.1 emits absolute
	// cursor positioning per row instead of \n, splitting on \n
	// would collapse the frame to one line. The grid places every
	// rune at its true (x,y).
	g := tui.DecodeANSI(buf.Bytes(), 80, 30)
	if !strings.Contains(g.RowText(0), "File") {
		t.Errorf("row 0 (header) missing 'File': %q", g.RowText(0))
	}
	if !strings.Contains(g.RowText(29), "VIEW") {
		t.Errorf("row 29 (footer) missing 'VIEW': %q", g.RowText(29))
	}
	if !strings.Contains(g.RowText(29), "*scratch*") {
		t.Errorf("row 29 (footer) missing '*scratch*': %q", g.RowText(29))
	}
}

// TestEditorRealInsertionFlowInPty exercises the full interactive
// path: press 'i' to enter edit mode, type "hello", verify "hello"
// lands in the rendered frame, press Escape to leave edit mode,
// press 'q' to quit. If any step failed silently (raw-mode wrong,
// key not routed, TextView not repainted), this test fails loud.
//
// This is the missing verification the v0.3.0 / v0.3.1 pipeline
// never had — a real interactive cycle in a real pty. Even the
// v0.3.2 layout-only e2e test could pass without editing actually
// working, since it only asserts on the initial frame.
func TestEditorRealInsertionFlowInPty(t *testing.T) {
	bin := buildBinary(t)

	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
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

	// Scripted interaction — small delays so the App loop repaints
	// between each keystroke and the pty captures every frame.
	write := func(s string) {
		time.Sleep(120 * time.Millisecond)
		_, _ = ptmx.Write([]byte(s))
	}
	write("i")           // enter edit mode
	write("hello")       // type five chars
	write("\x1b")        // Escape → back to view mode
	write("q")           // quit

	select {
	case <-captureDone:
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit within 5s of receiving 'q'")
	}
	_ = c.Wait()

	stripped := stripANSI(buf.Bytes())
	if !strings.Contains(stripped, "hello") {
		t.Fatalf("captured frames never showed 'hello' after editing.\n---captured---\n%s", stripped)
	}
	// EDIT mode label should have appeared in the footer at some
	// point during the run (after 'i', before Escape).
	if !strings.Contains(stripped, "EDIT") {
		t.Fatalf("captured frames never showed 'EDIT' mode indicator.\n---captured---\n%s", stripped)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tui-editor.bin")
	c := exec.Command("go", "build", "-o", bin, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// -----------------------------------------------------------------
// v0.3.13 anchored menu-dropdown click
// -----------------------------------------------------------------

// TestEditorClickOnFileMenuOpensAnchoredDropdown — clicking on the
// "File" item in the menu bar opens a compact dropdown positioned
// directly under it (X=1, Y=1).
func TestEditorClickOnFileMenuOpensAnchoredDropdown(t *testing.T) {
	bin := buildBinary(t)
	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, ptmx); close(done) }()
	go func() {
		time.Sleep(250 * time.Millisecond)
		// menuBar items: File Edit View Help. "File" occupies local
		// X ∈ [1, 5). Wire X=3 = local X=2 → hits "File".
		_, _ = ptmx.Write([]byte("\x1b[<0;3;1M")) // click File
		time.Sleep(200 * time.Millisecond)
		_, _ = ptmx.Write([]byte("q"))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()

	g := tui.DecodeANSI(buf.Bytes(), 80, 30)
	// Dropdown title "File" is on row 1 (AnchorY=1). Dropdown box
	// has a top-left corner glyph ┌ at col 1 (AnchorX=1).
	if !strings.Contains(g.RowText(1), "File") {
		t.Errorf("row 1 missing 'File' title: %q", g.RowText(1))
	}
	if c := g.At(1, 1).Rune; c != '┌' && c != '│' {
		t.Errorf("dropdown top-left at (1,1) = %c, want ┌ or │", c)
	}
	// Body should contain a "Quit" line.
	joined := ""
	for y := 1; y < 8; y++ {
		joined += g.RowText(y) + " | "
	}
	if !strings.Contains(joined, "Quit") {
		t.Errorf("dropdown body missing 'Quit': %q", joined)
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(b []byte) string { return ansiRE.ReplaceAllString(string(b), "") }
func splitRows(s string) []string {
	return strings.Split(s, "\n")
}
