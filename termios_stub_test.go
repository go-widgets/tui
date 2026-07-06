// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build !unix

package tui

import (
	"errors"
	"os"
	"testing"
)

func TestOpenTTYNotSupported(t *testing.T) {
	tty, err := OpenTTY(os.Stdout)
	if !errors.Is(err, ErrNotSupported) {
		t.Fatalf("OpenTTY err = %v, want ErrNotSupported", err)
	}
	if tty != nil {
		t.Fatalf("OpenTTY should return a nil TTY on non-Unix, got %v", tty)
	}
}
