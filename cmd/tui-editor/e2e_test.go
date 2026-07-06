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

	stripped := stripANSI(buf.Bytes())
	rows := splitRows(stripped)
	if len(rows) < 30 {
		t.Fatalf("got %d rendered rows, want ≥ 30\n---raw---\n%s", len(rows), stripped)
	}

	if !strings.Contains(rows[0], "File") {
		t.Errorf("row 0 (header) missing 'File': %q", rows[0])
	}
	// Footer carries the mode name — VIEW at startup.
	if !strings.Contains(rows[29], "VIEW") {
		t.Errorf("row 29 (footer) missing 'VIEW': %q", rows[29])
	}
	if !strings.Contains(rows[29], "*scratch*") {
		t.Errorf("row 29 (footer) missing '*scratch*': %q", rows[29])
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

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(b []byte) string { return ansiRE.ReplaceAllString(string(b), "") }
func splitRows(s string) []string {
	return strings.Split(s, "\n")
}
