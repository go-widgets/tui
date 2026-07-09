// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package tui is the terminal-I/O layer around
// [github.com/go-widgets/painter]'s CellPainter. It renders a widget
// tree into an ANSI cell stream sized to the current terminal, so the
// same widget code that produces pixels for a WUI or GUI back-end
// also produces text for a terminal — no widget changes required.
//
// The package is deliberately minimal: no raw mode, no keyboard event
// loop, no alt-screen management. It ships a "snapshot" model —
// render one frame to a writer and return — that composes cleanly
// with either a caller-managed event loop or a CLI that prints once
// and exits. Higher-level facilities (an [App] runner with input,
// resize, and cleanup handling) live in a follow-up cycle.
//
// Sizing follows the caller's preference:
//
//   - [RenderOnceSized] takes explicit (cols, rows) — the reliable form
//     used by tests, size-aware callers, and headless renderers.
//   - [RenderOnce] queries the size from environment variables
//     ([EnvSize]) and falls back to [DefaultCols]x[DefaultRows] when
//     the environment does not report a size. This keeps the package
//     stdlib-only — no ioctl, no cgo, no dependency on golang.org/x/term.
//
// Both variants write a self-contained ANSI stream to the caller's
// writer via [painter.CellPainter.WriteANSI]; the caller is
// responsible for the surrounding terminal state (raw mode, alt
// screen, cursor visibility) if any.
package tui

import "github.com/go-widgets/toolkit"

// EventTick is the [toolkit.EventKind] the App emits at a caller-configured tick
// rate. Widgets that animate (e.g. Spinner) match on this kind to advance a
// frame counter and repaint. The value sits above the toolkit's own iota range
// so it can never collide with a widget-produced Click / KeyDown / Char event.
// Defined here (not in the unix-only app.go) so cross-platform widgets can
// reference it on every backend, including js/wasm.
const EventTick toolkit.EventKind = 100
