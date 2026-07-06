// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

package tui

import (
	"errors"
	"os"

	"golang.org/x/term"
)

// TTY is the abstraction the App runner uses to talk to a real
// terminal. Implementations set raw mode, hide the cursor, switch
// to the alt-screen, and reverse everything on Close. The mock
// implementation used in tests satisfies the same interface without
// touching a real terminal.
type TTY interface {
	// Enter puts the terminal in raw mode, saves the current
	// termios state, switches to the alt-screen, and hides the
	// cursor. Idempotent — a second call while entered is a no-op.
	Enter() error

	// Leave reverses Enter: restores the alt-screen, shows the
	// cursor, and restores the saved termios state. Always safe to
	// call, including from a defer or a panic recovery.
	Leave() error

	// Size returns the current terminal dimensions in cells.
	Size() (cols, rows int, err error)
}

// The ANSI control sequences we emit around a full-screen session.
// Any modern xterm-compatible terminal (including tmux, iTerm2,
// Terminal.app, Windows Terminal via WSL) honours them.
const (
	seqEnterAltScreen = "\x1b[?1049h"
	seqLeaveAltScreen = "\x1b[?1049l"
	seqHideCursor     = "\x1b[?25l"
	seqShowCursor     = "\x1b[?25h"
	// seqEnableMouse turns on button-event tracking (?1002 —
	// press+release+drag, no bare motion) and switches the report
	// encoding to SGR (?1006 — decimal semicolon-separated so
	// coordinates > 223 are representable). Together these are the
	// modern portable combo honoured by xterm, iTerm2, Terminal.app,
	// tmux, kitty, alacritty, and Windows Terminal.
	seqEnableMouse  = "\x1b[?1002h\x1b[?1006h"
	seqDisableMouse = "\x1b[?1002l\x1b[?1006l"
)

// Package-level indirection so tests can fake the underlying TTY
// system calls without a real controlling terminal. Production code
// leaves them pointing at golang.org/x/term.
var (
	isTerminalFn = term.IsTerminal
	makeRawFn    = term.MakeRaw
	restoreFn    = term.Restore
	getSizeFn    = term.GetSize
)

// errNotTerminal is returned by OpenTTY when the given *os.File does
// not refer to a terminal device.
var errNotTerminal = errors.New("tui: file is not a terminal")

// unixTTY is the Unix TTY implementation. It writes ANSI setup and
// teardown to file and delegates termios changes to golang.org/x/term.
type unixTTY struct {
	fd       int
	file     *os.File
	oldState *term.State
	entered  bool
}

// OpenTTY opens the terminal associated with the given file (which
// must be a tty — os.Stdout in normal use). Returns an error on
// non-Unix platforms or if the file is not a tty.
func OpenTTY(f *os.File) (TTY, error) {
	fd := int(f.Fd())
	if !isTerminalFn(fd) {
		return nil, errNotTerminal
	}
	return &unixTTY{fd: fd, file: f}, nil
}

// Enter implements TTY.Enter. It calls term.MakeRaw to place the
// terminal in raw mode (saving the previous termios state) and then
// writes the alt-screen + hide-cursor escape sequences to the output
// file. A second call while already entered is a no-op.
func (t *unixTTY) Enter() error {
	if t.entered {
		return nil
	}
	state, err := makeRawFn(t.fd)
	if err != nil {
		return err
	}
	if _, err := t.file.WriteString(seqEnterAltScreen + seqHideCursor + seqEnableMouse); err != nil {
		// Best-effort roll-back of the raw-mode change we just made
		// so we do not leave the terminal in an unusable state when
		// the write to the output file fails.
		_ = restoreFn(t.fd, state)
		return err
	}
	t.oldState = state
	t.entered = true
	return nil
}

// Leave implements TTY.Leave. It writes the leave-alt-screen +
// show-cursor escape sequences and restores the saved termios state.
// A call while not entered is a no-op. Both the write and the
// termios restore are always attempted; the write error takes
// precedence in the returned value.
func (t *unixTTY) Leave() error {
	if !t.entered {
		return nil
	}
	_, writeErr := t.file.WriteString(seqDisableMouse + seqLeaveAltScreen + seqShowCursor)
	restoreErr := restoreFn(t.fd, t.oldState)
	t.entered = false
	t.oldState = nil
	if writeErr != nil {
		return writeErr
	}
	return restoreErr
}

// Size implements TTY.Size using term.GetSize on the file descriptor.
func (t *unixTTY) Size() (cols, rows int, err error) {
	return getSizeFn(t.fd)
}
