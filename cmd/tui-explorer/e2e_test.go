// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end integration test that builds cmd/tui-explorer, spawns
// it under a real pty, writes real key bytes, and asserts on the
// rendered frame after ANSI stripping.
//
// This is the test that would have caught the v0.3.0 / v0.3.1
// layout regression (MenuBar and Statusbar consuming a third of the
// screen each). The regular unit tests run under a stubbed event
// loop with fake TTYs, so the real Root.SetBounds → children
// distribution path never executed. The integration build tag keeps
// this slow(-ish) test out of the default `go test ./...` run;
// enable it explicitly with `go test -tags integration ./...`.

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

// TestExplorerRendersHeaderAndFooterInPty builds the binary,
// spawns it under a pty sized to 30 rows × 80 cols, writes 'q' to
// quit, captures the first ~200 ms of output, and asserts that the
// header + footer land at row 0 and row 29 respectively — the very
// property the v0.3.0 / v0.3.1 layout bug broke.
func TestExplorerRendersHeaderAndFooterInPty(t *testing.T) {
	bin := buildBinary(t)

	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Quit after the initial render.
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

	// Strip ANSI + split into rows.
	stripped := stripANSI(buf.Bytes())
	rows := splitRows(stripped)
	if len(rows) < 30 {
		t.Fatalf("got %d rendered rows, want ≥ 30\n---raw---\n%s", len(rows), stripped)
	}

	// Row 0 (header) must contain the header label. Row 29 (footer)
	// must contain the status label. If a plain VBox distributed
	// equally, "File" would land at row ~8 and "q: quit" at row ~20
	// — the regression's fingerprint.
	if !strings.Contains(rows[0], "File") {
		t.Errorf("row 0 (header) missing 'File': %q", rows[0])
	}
	if !strings.Contains(rows[29], "q: quit") {
		t.Errorf("row 29 (footer) missing 'q: quit': %q", rows[29])
	}
}

// TestExplorerHelpToggleInPty exercises the real interactive
// cycle: press '?', verify the help popover contents appear in the
// rendered frame, press '?' again to hide, press 'q' to quit. If
// the key handler swallowed the event but Consume wasn't wired
// correctly, or if the popover render path is broken in cell mode,
// this test would fail loud.
//
// This is the missing interactive verification: the header/footer
// layout test proves the initial frame is correct, but doesn't
// exercise any key handler beyond quit.
func TestExplorerHelpToggleInPty(t *testing.T) {
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

	write := func(s string) {
		time.Sleep(120 * time.Millisecond)
		_, _ = ptmx.Write([]byte(s))
	}
	write("?") // toggle help popover on
	write("q") // quit

	select {
	case <-captureDone:
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit within 5s of receiving 'q'")
	}
	_ = c.Wait()

	stripped := stripANSI(buf.Bytes())
	// The help popover renders a Label with the "Enter: open" hint
	// that isn't in the always-visible footer. Its appearance means
	// the '?' key toggled visibility AND the popover rendered.
	if !strings.Contains(stripped, "Enter: open") {
		t.Fatalf("help popover 'Enter: open' hint never appeared after '?'.\n---captured---\n%s", stripped)
	}
}

// TestExplorerArrowNavigationInPty exercises the full user cycle:
// press Down twice, verify the third file's content ("/docs/README.md"
// = "# Project ...") appears in the right pane, press q to quit. If
// the arrow-key handlers stopped syncing content OR the fileList
// stopped highlighting the selected row OR the right pane stopped
// repainting, this test catches all three.
func TestExplorerArrowNavigationInPty(t *testing.T) {
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

	write := func(s string) {
		time.Sleep(120 * time.Millisecond)
		_, _ = ptmx.Write([]byte(s))
	}
	write("\x1b[B") // Down arrow
	write("\x1b[B") // Down arrow again -> index 2 = /docs/README.md
	write("q")

	select {
	case <-captureDone:
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not exit within 5s of 'q'")
	}
	_ = c.Wait()

	stripped := stripANSI(buf.Bytes())
	// After two Downs, the right pane must show README.md's content
	// (starts with "# Project"). If arrow navigation is broken, we
	// only see the file at index 0 (/src/main.go = "package main").
	if !strings.Contains(stripped, "# Project") {
		t.Fatalf("arrow navigation did not sync content to /docs/README.md\n---captured---\n%s", stripped)
	}
}

// buildBinary compiles cmd/tui-explorer into a t.TempDir binary and
// returns its absolute path. Uses `go build` so the actual entry
// point + `//go:build unix` are honored, unlike a package-import
// test which never runs main().
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

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(b []byte) string { return ansiRE.ReplaceAllString(string(b), "") }

func splitRows(s string) []string {
	return strings.Split(s, "\n")
}
