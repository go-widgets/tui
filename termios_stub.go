// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build !unix

package tui

import (
	"errors"
	"os"
)

// TTY is the abstraction the App runner uses to talk to a real
// terminal. On non-Unix platforms the concrete TTY implementation is
// not available and OpenTTY returns [ErrNotSupported]; the interface
// is exported so callers can still write portable code that consumes
// a TTY handed to them from a Unix host.
type TTY interface {
	Enter() error
	Leave() error
	Size() (cols, rows int, err error)
}

// ErrNotSupported is returned by [OpenTTY] on platforms where the
// interactive TTY setup path is not implemented (Windows, Plan 9,
// WebAssembly). It is a package-level sentinel so callers can
// errors.Is against it to fall back to a non-interactive rendering
// path.
var ErrNotSupported = errors.New("tui: interactive TTY setup is Unix-only in v0.3.0")

// OpenTTY on non-Unix platforms always returns [ErrNotSupported].
// The Unix implementation lives in termios_unix.go.
func OpenTTY(f *os.File) (TTY, error) {
	_ = f
	return nil, ErrNotSupported
}
