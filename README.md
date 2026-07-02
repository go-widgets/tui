# go-widgets/tui

![status](https://img.shields.io/badge/status-planned-9a6700)
[![license](https://img.shields.io/badge/license-BSD--3--Clause-blue)](./LICENSE)

**Status: planned.** This repo is a placeholder while the design is
sketched.

## What it will be

A pure-Go widget set that renders into a **terminal cell grid** the
same way [go-widgets/toolkit](https://github.com/go-widgets/toolkit)
renders into an RGBA byte buffer. Same design axiom:

- **Byte-buffer first**: `Draw(cells []Cell, gridW int, theme *Theme)`
  where `Cell` is `struct { Rune rune; Fg, Bg RGBA }`. The caller
  owns the grid; the widget composes cells.
- **No dependency at the toolkit level**: the ANSI writer /
  raw-mode setup / event pump live in a separate `tui/host`
  subpackage. The core widget set can be tested purely against
  in-memory cell grids.
- **Coherent theming**: reuse `toolkit.Theme` — a widget's ink
  colour maps to the closest 256-colour / true-colour ANSI cell.
- **100 % statement coverage**.

## What it will NOT be

- **Not a full-screen framework** like tview / bubbletea. Those
  own the terminal + the event loop; this repo owns just the
  widget rendering + the (rune, fg, bg) grid abstraction. Bring
  your own host loop.
- **Not a re-implementation** of every toolkit widget. The
  practical MVP set: Button, Label, Entry, TextView (single-line
  wrap), Menu, MenuBar, Statusbar, ListBox, ProgressBar. Pixel-
  precise widgets (ColorChooser, Calendar) don't map cleanly to
  a cell grid.

## Why this repo exists NOW

Reserving the name in the `go-widgets` org so external consumers
can watch it + a future implementation lands at the expected URL.
No code is committed until the API stabilises against a real
consumer. If you have a use case, please open an issue — the design
is not yet frozen.

## License

BSD-3-Clause.
