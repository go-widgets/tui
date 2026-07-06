# go-widgets/tui

[![CI](https://github.com/go-widgets/tui/actions/workflows/ci.yml/badge.svg)](https://github.com/go-widgets/tui/actions/workflows/ci.yml)
[![pkg.go.dev](https://img.shields.io/badge/pkg.go.dev-tui-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/go-widgets/tui)
![coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)
![go](https://img.shields.io/badge/Go-1.26.4%2B-00ADD8?logo=go&logoColor=white)
[![license](https://img.shields.io/badge/license-BSD--3--Clause-blue)](./LICENSE)

Terminal I/O layer around [`go-widgets/painter`](https://github.com/go-widgets/painter)'s
`CellPainter`. Renders a widget tree as a self-contained ANSI stream
sized to the current terminal, so the *same widget code* that produces
pixels for a WUI or GUI back-end also produces text for a terminal.

## What it is (and what it isn't)

The package is deliberately minimal — no raw mode, no keyboard event
loop, no alt-screen management. It ships a **snapshot model**: render
one frame to a writer and return. Compose it with a caller-managed
event loop or a CLI that prints once and exits.

```go
import (
    "os"

    "github.com/go-widgets/painter"
    "github.com/go-widgets/tui"
)

widgets := []painter.Widget{
    &painter.Label{Bounds: painter.Rect{X: 2, Y: 1, W: 30, H: 1}, Text: "GO WIDGETS TUI"},
    &painter.Button{Bounds: painter.Rect{X: 2, Y: 3, W: 12, H: 3}, Label: "OK"},
    &painter.ProgressBar{Bounds: painter.Rect{X: 2, Y: 8, W: 30, H: 3}, Value: 0.72},
}
_ = tui.RenderOnce(os.Stdout, widgets, nil) // nil theme = LightTheme
```

An `App` runner with input, resize, and cleanup handling is future
work — this repo intentionally leaves that surface for a follow-up
cycle rather than shipping a half-baked event loop.

## Sizing

Two variants, take your pick:

- `RenderOnceSized(w, cols, rows, widgets, theme)` — explicit
  dimensions. The reliable form for tests, size-aware callers, and
  headless renderers.
- `RenderOnce(w, widgets, theme)` — queries `EnvSize()` (COLUMNS /
  LINES environment variables) and falls back to `DefaultCols` x
  `DefaultRows` (80 x 24) when the environment does not report a size.

The env-vars-only strategy is a deliberate trade-off: it keeps the
package stdlib-only — no `TIOCGWINSZ` ioctl per platform, no cgo, no
dependency on `golang.org/x/term`. Interactive shells that need
dynamic sizes should export `COLUMNS` / `LINES` from their prompt
hook, or pass an explicit size to `RenderOnceSized`.

## Try it

```bash
# Default terminal size (or COLUMNS / LINES if set)
go run ./cmd/tui-snapshot

# Force a specific size + dark theme
go run ./cmd/tui-snapshot --cols=100 --rows=25 --theme=dark
```

The `tui-snapshot` demo mirrors [`painter/cmd/tui-demo`](https://github.com/go-widgets/painter/tree/main/cmd/tui-demo):
same three-widget layout (label, two buttons, progress bar), same
theme selection.

For a broader showcase using the real
[go-widgets/toolkit](https://github.com/go-widgets/toolkit) widgets
(Button, ToggleButton, Switch, Entry, ProgressBar, Alert, Stat,
Timeline …) rendered through the same cell backend:

```bash
# Toolkit widget catalogue in two columns
go run ./cmd/tui-catalogue

# Force size + dark theme
go run ./cmd/tui-catalogue --cols=100 --rows=25 --theme=dark
```

Internally uses `tui.RenderToolkit` (the bridge that accepts
`toolkit.Widget` and translates `toolkit.Theme` to what
`painter.CellPainter` expects) so the on-screen widgets are the
same objects a wasm gallery or a native window would compose.

## Design axiom

Same as the rest of `go-widgets`: **dependency-free, stdlib-only,
100% coverage** (library packages *and* `cmd/`). CI enforces the
coverage gate on every push.

## License

BSD-3-Clause. See [LICENSE](./LICENSE).
