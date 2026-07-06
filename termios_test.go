// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/term"
)

// swapTermFns saves the current package-level termios function
// pointers, installs the given replacements, and registers a cleanup
// that puts the originals back. Passing nil for any field leaves
// that pointer untouched.
func swapTermFns(
	t *testing.T,
	isTerminal func(int) bool,
	makeRaw func(int) (*term.State, error),
	restore func(int, *term.State) error,
	getSize func(int) (int, int, error),
) {
	t.Helper()
	savedIsTerminal := isTerminalFn
	savedMakeRaw := makeRawFn
	savedRestore := restoreFn
	savedGetSize := getSizeFn
	if isTerminal != nil {
		isTerminalFn = isTerminal
	}
	if makeRaw != nil {
		makeRawFn = makeRaw
	}
	if restore != nil {
		restoreFn = restore
	}
	if getSize != nil {
		getSizeFn = getSize
	}
	t.Cleanup(func() {
		isTerminalFn = savedIsTerminal
		makeRawFn = savedMakeRaw
		restoreFn = savedRestore
		getSizeFn = savedGetSize
	})
}

// makeFakeTTYFile returns a real *os.File under t.TempDir() that
// tests can use as if it were a terminal. It is a plain regular file,
// so writes succeed and the bytes can be read back with os.ReadFile
// for assertion.
func makeFakeTTYFile(t *testing.T) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-tty")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// stubState is a sentinel *term.State the fakes hand back so tests
// can assert that Restore was called with the exact pointer that
// MakeRaw returned.
var stubState = &term.State{}

func TestOpenTTYHappyPath(t *testing.T) {
	swapTermFns(t,
		func(fd int) bool { return true },
		nil, nil, nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY err: %v", err)
	}
	if tty == nil {
		t.Fatal("OpenTTY returned a nil TTY")
	}
}

func TestOpenTTYNotATerminal(t *testing.T) {
	swapTermFns(t,
		func(fd int) bool { return false },
		nil, nil, nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err == nil {
		t.Fatal("OpenTTY should return an error when isTerminal is false")
	}
	if !errors.Is(err, errNotTerminal) {
		t.Fatalf("OpenTTY err = %v, want errNotTerminal", err)
	}
	if tty != nil {
		t.Fatalf("OpenTTY should return a nil TTY on error, got %v", tty)
	}
}

func TestEnterHappyPath(t *testing.T) {
	makeRawCalled := false
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) {
			makeRawCalled = true
			return stubState, nil
		},
		func(fd int, s *term.State) error { return nil },
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if !makeRawCalled {
		t.Fatal("Enter should call makeRawFn")
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, seqEnterAltScreen) {
		t.Errorf("Enter output %q missing alt-screen enter sequence", got)
	}
	if !strings.Contains(got, seqHideCursor) {
		t.Errorf("Enter output %q missing hide-cursor sequence", got)
	}
}

func TestEnterIdempotent(t *testing.T) {
	callCount := 0
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) {
			callCount++
			return stubState, nil
		},
		func(fd int, s *term.State) error { return nil },
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("first Enter: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("second Enter: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("Enter called makeRawFn %d times, want 1 (second call must be a no-op)", callCount)
	}
}

func TestEnterMakeRawError(t *testing.T) {
	sentinel := errors.New("make raw failed")
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) { return nil, sentinel },
		nil, nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); !errors.Is(err, sentinel) {
		t.Fatalf("Enter err = %v, want %v", err, sentinel)
	}
}

func TestEnterWriteError(t *testing.T) {
	restoreCalled := false
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) { return stubState, nil },
		func(fd int, s *term.State) error {
			restoreCalled = true
			return nil
		},
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	// Close the file so the Enter write fails.
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if err := tty.Enter(); err == nil {
		t.Fatal("Enter should return an error when the file write fails")
	}
	if !restoreCalled {
		t.Fatal("Enter must call restoreFn to roll back raw-mode when the write fails")
	}
}

func TestLeaveIdempotent(t *testing.T) {
	restoreCalls := 0
	swapTermFns(t,
		func(fd int) bool { return true },
		nil,
		func(fd int, s *term.State) error {
			restoreCalls++
			return nil
		},
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	// Never entered — Leave must be a no-op.
	if err := tty.Leave(); err != nil {
		t.Fatalf("Leave (not entered): %v", err)
	}
	if restoreCalls != 0 {
		t.Fatalf("Leave (not entered) should not call restoreFn, got %d calls", restoreCalls)
	}
}

func TestLeaveHappyPath(t *testing.T) {
	var restoreGotState *term.State
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) { return stubState, nil },
		func(fd int, s *term.State) error {
			restoreGotState = s
			return nil
		},
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if err := tty.Leave(); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if restoreGotState != stubState {
		t.Fatalf("Leave passed %p to restoreFn, want stubState %p", restoreGotState, stubState)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, seqLeaveAltScreen) {
		t.Errorf("Leave output %q missing alt-screen leave sequence", got)
	}
	if !strings.Contains(got, seqShowCursor) {
		t.Errorf("Leave output %q missing show-cursor sequence", got)
	}
	// A second Leave must be a no-op (idempotent).
	if err := tty.Leave(); err != nil {
		t.Fatalf("second Leave should be a no-op, got %v", err)
	}
}

func TestLeaveWriteError(t *testing.T) {
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) { return stubState, nil },
		func(fd int, s *term.State) error { return nil },
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if err := tty.Leave(); err == nil {
		t.Fatal("Leave should return an error when the file write fails")
	}
}

func TestLeaveRestoreError(t *testing.T) {
	sentinel := errors.New("restore failed")
	swapTermFns(t,
		func(fd int) bool { return true },
		func(fd int) (*term.State, error) { return stubState, nil },
		func(fd int, s *term.State) error { return sentinel },
		nil,
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if err := tty.Enter(); err != nil {
		t.Fatalf("Enter: %v", err)
	}
	if err := tty.Leave(); !errors.Is(err, sentinel) {
		t.Fatalf("Leave err = %v, want %v", err, sentinel)
	}
}

func TestSizeHappyPath(t *testing.T) {
	swapTermFns(t,
		func(fd int) bool { return true },
		nil, nil,
		func(fd int) (int, int, error) { return 100, 30, nil },
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	cols, rows, err := tty.Size()
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if cols != 100 || rows != 30 {
		t.Fatalf("Size = (%d, %d), want (100, 30)", cols, rows)
	}
}

func TestSizeError(t *testing.T) {
	sentinel := errors.New("get size failed")
	swapTermFns(t,
		func(fd int) bool { return true },
		nil, nil,
		func(fd int) (int, int, error) { return 0, 0, sentinel },
	)
	f := makeFakeTTYFile(t)
	tty, err := OpenTTY(f)
	if err != nil {
		t.Fatalf("OpenTTY: %v", err)
	}
	if _, _, err := tty.Size(); !errors.Is(err, sentinel) {
		t.Fatalf("Size err = %v, want %v", err, sentinel)
	}
}
