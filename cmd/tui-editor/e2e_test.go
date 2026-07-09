// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end integration tests that build cmd/tui-editor, spawn it under a real
// pty, drive it with real key bytes, and assert on the rendered frame.
//
// Enable with: go test -tags integration ./cmd/tui-editor/...
//
// Timing: rather than sleep a fixed amount between keystrokes (which flaked
// under parallel load — a key could arrive before the App had processed the
// previous mode change), every step waits on the RENDERED STATE via
// session.waitFor: send a key, then block until the frame shows the marker that
// proves the key took effect. The capture buffer is mutex-guarded (syncBuf) so
// the test goroutine can poll it while the pty-reader goroutine appends to it.
package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/go-widgets/tui"
)

// syncBuf is a concurrency-safe buffer: the pty-reader goroutine appends via
// Write while the test goroutine snapshots via Bytes.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buf.Bytes()...)
}

// editorSession is a tui-editor spawned under a pty with a concurrent capture.
type editorSession struct {
	t    *testing.T
	ptmx *os.File
	cmd  *exec.Cmd
	buf  *syncBuf
	done chan struct{}
}

// spawnEditor builds + launches the editor under an 80×30 pty and starts
// capturing its output. It skips the test if a pty is unavailable.
func spawnEditor(t *testing.T) *editorSession {
	t.Helper()
	bin := buildBinary(t)
	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	s := &editorSession{t: t, ptmx: ptmx, cmd: c, buf: &syncBuf{}, done: make(chan struct{})}
	go func() { _, _ = io.Copy(s.buf, ptmx); close(s.done) }()
	return s
}

func (s *editorSession) close() { _ = s.ptmx.Close() }

func (s *editorSession) send(keys string) { _, _ = s.ptmx.Write([]byte(keys)) }

// waitFor blocks until the (ANSI-stripped) captured output contains sub, or
// fails the test after timeout. This is the flake-proof replacement for a fixed
// inter-key sleep: it synchronises on what the editor actually rendered.
func (s *editorSession) waitFor(sub string, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(stripANSI(s.buf.Bytes()), sub) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for %q to render.\n---captured---\n%s", sub, stripANSI(s.buf.Bytes()))
}

// waitExit blocks until the editor process exits, or fails after timeout.
func (s *editorSession) waitExit(timeout time.Duration) {
	s.t.Helper()
	select {
	case <-s.done:
	case <-time.After(timeout):
		s.t.Fatal("editor did not exit within timeout")
	}
	_ = s.cmd.Wait()
}

// grid decodes the full captured stream into a cell grid. Because tui.App
// redraws every cell with absolute positioning each frame, the decoded grid
// reflects the final frame.
func (s *editorSession) grid() *tui.TermGrid { return tui.DecodeANSI(s.buf.Bytes(), 80, 30) }

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

const shortTimeout = 4 * time.Second

func TestEditorRendersHeaderAndFooterInPty(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout) // App up + first frame drawn
	s.send("q")                     // VIEW mode: q quits directly
	s.waitExit(shortTimeout)

	g := s.grid()
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

// TestEditorRealInsertionFlowInPty exercises the full interactive path: 'i' to
// enter edit mode, type "hello", Escape, 'q'. Each step waits for the rendered
// state that proves the previous key took effect, so a slow machine can't let a
// keystroke land in the wrong mode.
func TestEditorRealInsertionFlowInPty(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout)
	s.send("i")
	s.waitFor("EDIT", shortTimeout) // confirm edit mode BEFORE typing
	s.send("hello")
	s.waitFor("hello", shortTimeout) // the text reached the buffer + repainted
	s.send("\x1b")
	s.waitFor("VIEW", shortTimeout) // back to view mode
	s.send("q")
	s.waitExit(shortTimeout)

	stripped := stripANSI(s.buf.Bytes())
	if !strings.Contains(stripped, "hello") {
		t.Fatalf("captured frames never showed 'hello'.\n---captured---\n%s", stripped)
	}
	if !strings.Contains(stripped, "EDIT") {
		t.Fatalf("captured frames never showed 'EDIT' mode.\n---captured---\n%s", stripped)
	}
}

// -----------------------------------------------------------------
// Anchored menu-dropdown clicks
// -----------------------------------------------------------------

// TestEditorClickOnFileMenuOpensAnchoredDropdown — clicking "File" in the menu
// bar opens a compact dropdown positioned directly under it (X=1, Y=1).
func TestEditorClickOnFileMenuOpensAnchoredDropdown(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout) // menu bar rendered
	// menuBar items: File Edit View Help. "File" occupies local X ∈ [1,5).
	// Wire X=3 = local X=2 → hits "File".
	s.send("\x1b[<0;3;1M")
	s.waitFor("Ctrl+N", shortTimeout) // File dropdown open ("New  Ctrl+N" is unique to it)
	s.send("q")
	s.waitExit(shortTimeout)

	g := s.grid()
	// Dropdown title "File" on row 1 (AnchorY=1); box corner at col 1 (AnchorX=1).
	if !strings.Contains(g.RowText(1), "File") {
		t.Errorf("row 1 missing 'File' title: %q", g.RowText(1))
	}
	if c := g.At(1, 1).Rune; c != '┌' && c != '│' {
		t.Errorf("dropdown top-left at (1,1) = %c, want ┌ or │", c)
	}
	joined := ""
	for y := 1; y < 8; y++ {
		joined += g.RowText(y) + " | "
	}
	if !strings.Contains(joined, "Quit") {
		t.Errorf("dropdown body missing 'Quit': %q", joined)
	}
}

// TestEditorViewMenuTogglesLineNumbers — click "View" then its first body row
// ("Toggle line numbers"). The editor starts with ShowGutter=true, so the action
// turns the gutter OFF — verified via cell (1,1) no longer holding the digit '1'.
func TestEditorViewMenuTogglesLineNumbers(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout)
	// "View" is menu item 2, starting at local X=15. Wire X=17 → local 16 ∈ [15,19).
	s.send("\x1b[<0;17;1M")
	s.waitFor("Toggle line numbers", shortTimeout) // View dropdown open
	// First body row = dropdown-local Y=1. Wire (X=20, Y=3) → hits ItemActions[0].
	s.send("\x1b[<0;20;3M")
	s.send("q")
	s.waitExit(shortTimeout)

	g := s.grid()
	if r := g.At(1, 1).Rune; r == '1' {
		t.Errorf("gutter still on after toggle: cell (1,1) = %c", r)
	}
}

// TestEditorEditMenuUndoRestoresBuffer — type "abc" in edit mode, Escape, then
// click Edit → Undo. The buffer must roll back one char.
func TestEditorEditMenuUndoRestoresBuffer(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout)
	s.send("i")
	s.waitFor("EDIT", shortTimeout)
	s.send("abc")
	s.waitFor("abc", shortTimeout) // three chars rendered in the buffer
	s.send("\x1b")
	s.waitFor("VIEW", shortTimeout)
	// "Edit" starts at local X=8. Wire X=10 → local 9 ∈ [8,12).
	s.send("\x1b[<0;10;1M")
	s.waitFor("Undo", shortTimeout) // Edit dropdown open ("Undo  Ctrl+Z")
	// Body row 0 (Undo). Wire (X=12, Y=3) → dropdown-local (·,1) → idx=0.
	s.send("\x1b[<0;12;3M")
	s.send("q")
	s.waitExit(shortTimeout)

	g := s.grid()
	// Gutter col 1 = "1"; buffer chars at cols 3,4,5. After Undo: 'a','b',blank.
	if r := g.At(3, 1).Rune; r != 'a' {
		t.Errorf("col 3 row 1 = %c, want 'a'", r)
	}
	if r := g.At(4, 1).Rune; r != 'b' {
		t.Errorf("col 4 row 1 = %c, want 'b'", r)
	}
	if r := g.At(5, 1).Rune; r == 'c' {
		t.Errorf("col 5 row 1 still 'c' — Undo via menu did not fire")
	}
}

// TestEditorKeyboardCtrlZRefreshesStatus — type "abc" in edit mode, then Ctrl+Z.
// The status bar's cursor segment must roll back to "1:3". In edit mode the demo
// does NOT refresh the status on typing, so "1:3" appears ONLY after Ctrl+Z runs
// through s.undo() → refreshStatus — making it a valid, non-transient marker.
func TestEditorKeyboardCtrlZRefreshesStatus(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout)
	s.send("i")
	s.waitFor("EDIT", shortTimeout)
	s.send("abc")
	s.waitFor("abc", shortTimeout)
	s.send("\x1a") // Ctrl+Z
	s.waitFor("1:3", shortTimeout)
	s.send("\x1b")
	s.waitFor("VIEW", shortTimeout)
	s.send("q")
	s.waitExit(shortTimeout)

	if stripped := stripANSI(s.buf.Bytes()); !strings.Contains(stripped, "1:3") {
		t.Errorf("Ctrl+Z did not refresh status to 1:3:\n%s", stripped)
	}
}

// TestEditorPaletteTypesAndRunsCommand — the end-to-end proof that the command
// palette accepts typed input (a dead stub before App.InputTarget): Ctrl+P to
// open, type "quit", Enter — the editor must exit and the palette must have
// echoed "> quit". If the input were inert, "quit" would land in VIEW mode ('q'
// quitting instantly with no "> quit" echo) and the echo assertion would fail.
func TestEditorPaletteTypesAndRunsCommand(t *testing.T) {
	s := spawnEditor(t)
	defer s.close()

	s.waitFor("VIEW", shortTimeout)
	s.send("\x10") // Ctrl+P
	s.waitFor("PALETTE", shortTimeout)
	s.send("quit")
	s.waitFor("> quit", shortTimeout) // palette mirrored the typed command
	s.send("\r")                      // Enter → run "quit" → a.Quit()
	s.waitExit(shortTimeout)

	if str := stripANSI(s.buf.Bytes()); !strings.Contains(str, "> quit") {
		t.Errorf("captured frames never echoed '> quit'.\n---\n%s", str)
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(b []byte) string { return ansiRE.ReplaceAllString(string(b), "") }
func splitRows(s string) []string {
	return strings.Split(s, "\n")
}
