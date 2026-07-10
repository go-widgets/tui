// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end tests: build tui-widget-explorer, drive it under a real pty by
// rendered state (not fixed sleeps), and assert on the decoded frame.
//
//	go test -tags integration ./cmd/tui-widget-explorer/...
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
)

type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ansiRE.ReplaceAllString(s.buf.String(), "")
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

type session struct {
	t    *testing.T
	ptmx *os.File
	cmd  *exec.Cmd
	buf  *syncBuf
	done chan struct{}
}

func spawn(t *testing.T) *session {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "explorer.bin")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	s := &session{t: t, ptmx: ptmx, cmd: c, buf: &syncBuf{}, done: make(chan struct{})}
	go func() { _, _ = io.Copy(s.buf, ptmx); close(s.done) }()
	return s
}

func (s *session) send(keys string) { _, _ = s.ptmx.Write([]byte(keys)) }
func (s *session) close()           { _ = s.ptmx.Close() }

func (s *session) waitFor(sub string, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.buf.text(), sub) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for %q.\n---\n%s", sub, s.buf.text())
}

func (s *session) waitExit(timeout time.Duration) {
	s.t.Helper()
	select {
	case <-s.done:
	case <-time.After(timeout):
		s.t.Fatal("explorer did not exit")
	}
	_ = s.cmd.Wait()
}

const to = 5 * time.Second

// TestExplorerRendersListAndStage — the header, the widget list, and the live
// stage of the first widget (Entry, showing its placeholder) all render.
func TestExplorerRendersListAndStage(t *testing.T) {
	s := spawn(t)
	defer s.close()
	s.waitFor("widget explorer", to) // header
	txt := s.buf.text()
	for _, want := range []string{"Entry", "Button", "type here"} {
		if !strings.Contains(txt, want) {
			t.Errorf("frame missing %q:\n%s", want, txt)
		}
	}
	s.send("q")
	s.waitExit(to)
}

// TestExplorerTypesIntoEntryStage — the interactive proof: Tab focuses the stage
// (the Entry), typed characters land in it, Esc returns to the list, q quits. If
// the stage weren't live, "hello" would never render.
func TestExplorerTypesIntoEntryStage(t *testing.T) {
	s := spawn(t)
	defer s.close()
	s.waitFor("focus: list", to) // footer shows list focus
	s.send("\t")                 // Tab → focus the Entry stage
	s.waitFor("focus: stage", to)
	s.send("hello")
	s.waitFor("hello", to) // typed text reached the Entry
	s.send("\x1b")         // Esc → back to the list
	s.waitFor("focus: list", to)
	s.send("q")
	s.waitExit(to)
}

// TestExplorerNavigatesAndSwapsStage — the Down arrow moves the selection and
// swaps the live stage. Stepped one press at a time and synced on the footer
// (which de-flakes the input and proves each Down registers): Down to Button
// shows its "Click me" stage, Down again to CheckButton shows "[✓] Enabled" —
// proving both navigation and the per-selection stage rebuild.
func TestExplorerNavigatesAndSwapsStage(t *testing.T) {
	s := spawn(t)
	defer s.close()
	s.waitFor("focus: list", to)

	s.send("\x1b[B") // Down → Button
	s.waitFor("Button  ·  focus", to)
	s.waitFor("Click me", to) // Button's live stage

	s.send("\x1b[B") // Down → CheckButton
	s.waitFor("CheckButton  ·  focus", to)
	s.waitFor("[✓] Enabled", to) // CheckButton's live stage

	s.send("q")
	s.waitExit(to)
}
