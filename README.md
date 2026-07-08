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

## Interactive `tui.App` runner (v0.3.x)

For a full interactive experience — raw mode + alt-screen + input
loop + resize + cleanup — instantiate a `tui.App`:

```go
import (
    "os"
    "github.com/go-widgets/toolkit"
    "github.com/go-widgets/tui"
)

func main() {
    app := tui.NewApp()
    app.Root = buildScene()          // toolkit.Widget hierarchy
    app.Keys["q"] = func(a *tui.App) { a.Quit() }
    app.Keys["Ctrl+C"] = func(a *tui.App) { a.Quit() }
    os.Exit(app.Run())
}
```

`App.Run()` enters alt-screen, sets raw mode, spawns a stdin reader,
dispatches events to `Root.OnEvent` (with global handlers running
first), reacts to `SIGWINCH` for resize, and always restores the
terminal on exit — even on panic, via a deferred `TTY.Leave` inside
a defer/recover chain.

Keys handlers may call `a.Consume()` to prevent the current event
from also reaching `Root` — needed for mode-switching editors
(pressing `i` to enter edit mode without also inserting `i` into
the buffer).

Two reference demos ship in `cmd/`:

- `cmd/tui-explorer` — k9s-style file browser (file list + preview
  pane + arrow-key navigation + help/search popovers).
- `cmd/tui-editor` — loom-style modal text editor (View / Edit /
  Palette modes + file open + Ctrl+S save + Ctrl+P palette).

Both are exercised by real pty-based e2e tests
(`//go:build unix && integration`) that spawn the binary under a
real terminal, send real key bytes, and assert on the rendered
frame. **These tests catch layout + event-loop bugs that seam-based
unit tests miss** — see the v0.3.2 / v0.3.3 / v0.3.4 release notes
for the concrete regressions this protocol caught.

### Cell-mode widget guidance

Most `toolkit` widgets are designed for pixel rendering — their
internal pad constants (`AlertPadY = 8`, `MenuBarH = 22`, …) are
pixels in `PixelPainter` mode. In `tui.RenderToolkit` /
`tui.App` cell mode, those same integers count CELLS, so widgets
that lean on large pad constants render at cell-inappropriate sizes.

So for the widgets whose pixel geometry doesn't survive the pixels→cells
reinterpretation, `tui` ships its own **cell-native** versions (see the catalog
below). A handful of simple `toolkit` widgets already render at ~1 cell per
glyph and are safe to use directly in `tui.App`: `Label`, `TextView`. Others
(`Alert`, `Card`, `Stat`, `HeaderBar`, `Toast`, `Banner`) are usable but
visually inflated, and the pixel-only structural widgets (`toolkit.MenuBar`,
`Notebook`, `HPaned`, …) render poorly — use the `tui` equivalents instead.

## Cell-native widget set

Every widget below is a `toolkit.Widget` that renders through `painter.Painter`,
so the *same* instance drives a terminal cell grid (TUI) **and** an RGBA pixel
buffer (WUI/GUI) with no code change. All are 100%-covered and tuned so one glyph
occupies one cell — no pixel padding leaking into the layout.

| Widget | What it is |
|--------|-----------|
| `TextEditor` | read-write code editor: syntax highlight, gutter, undo/redo, search, selection (see below) |
| `Entry` | single-line text input with placeholder, horizontal scroll, rune-indexed caret |
| `Button` | clickable action; `Default` / `Prominent` / `Secondary` styles; hover + press states |
| `CheckButton` | `[✓]` / `[ ]` boolean toggle with a label |
| `RadioButton` / `RadioGroup` | `(•)` / `( )` toggle; group for mutual exclusion |
| `Scale` | draggable slider over `Min..Max`; click / drag / arrow-key stepping |
| `ProgressBar` | continuous `Fraction` fill with an optional centred label |
| `LevelBar` | discrete `Value`/`Max` segments (battery / signal / steps) |
| `ListBox` | scrollable single-select list with `OnSelect` |
| `TreeView` | collapsible hierarchy (chevrons, keyboard nav, scroll) for file trees / outlines |
| `Table` | data grid: header, auto/fixed columns, zebra rows, selection, scroll |
| `MenuBar` | top menu strip; `ItemXRange` for anchoring dropdowns |
| `MenuDropdown` | anchored, self-sizing dropdown; per-row actions |
| `Toolbar` | action strip of labelled buttons + separators (below a MenuBar) |
| `Popover` | bordered modal overlay (title + body), hidden unless `Visible` |
| `Statusbar` | footer strip of segments; last segment fills; lazy `SetSegment` |
| `Notebook` | tabbed container; label-sized tabs; routes events to the active page |
| `VBox` | header / body / footer layout with inset, hit-tested overlays + drag-capture |
| `HSplit` | resizable horizontal split with a draggable grip column |

The `cmd/tui-explorer` + `cmd/tui-editor` demos are built from these widgets.

## `tui.TextEditor`

A cell-native, **read-write** code editor widget with syntax highlighting, a
line-number gutter, undo/redo, search, and selection — the buffer behind
`cmd/tui-editor` (read-write) and `cmd/tui-explorer`'s preview (`ReadOnly`).
Being a `painter.Painter` consumer, the *same* widget renders to a terminal
cell grid (TUI) **and** an RGBA pixel buffer (WUI/GUI) with no code change.

```go
ed := tui.NewTextEditor()   // gutter on, one empty line
ed.Filename = "main.go"     // extension picks the syntax language
ed.SetText(src)
ed.Focused = true
app.Root = ed               // it is a toolkit.Widget
```

Fields: `Filename` (drives the highlighter's language; `""` = plain),
`ReadOnly` (viewer mode — navigation still works, edits are ignored),
`ShowGutter` (line numbers), `Focused` (draws the caret). The viewport scrolls
to follow the caret automatically.

Keys (delivered through `OnEvent`):

| Key | Action |
|-----|--------|
| printable · `Enter` · `Backspace` · `Delete` | insert · split line · delete back / forward |
| `←` `→` `↑` `↓` | move the caret |
| `Home` · `End` | line start · line end |
| `PageUp` · `PageDown` | move one viewport |
| `Tab` · `Shift+Tab` | indent · dedent (caret line or whole selection) |
| `Alt+↑` · `Alt+↓` | move the current line up · down |
| `Ctrl+Z` · `Ctrl+Y` | undo · redo |
| `Ctrl+X` · `Ctrl+V` | cut · paste |
| mouse click · drag | position caret · extend selection |

Methods: `SetText` / `Text`, `Find` / `FindNext`, `Replace` / `ReplaceAll`,
`Copy` / `Cut` / `Paste`, `SelectedText` / `DeleteSelection`.

Highlighting lives in the `tui/syntax` sub-package and covers Go (via the
standard library's `go/scanner`), JavaScript/TypeScript, Python, Ruby, shell,
C/C++, Rust, JSON, YAML, HCL, TOML, LaTeX and Markdown — with **no external
dependency**. `tui.SyntaxInk` maps a token kind to a theme-aware colour.

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

For the interactive demos (`tui.App` powered):

```bash
# k9s-style file browser with arrow navigation
go run ./cmd/tui-explorer

# vi-style modal editor
go run ./cmd/tui-editor --file=path/to/file.txt
```

To run the pty-based end-to-end integration tests:

```bash
go test -tags integration ./cmd/tui-explorer/... ./cmd/tui-editor/...
```

These verify the rendered frame after real key input in a real
pty, catching layout + interaction bugs that unit tests miss.

## Design axiom

Same as the rest of `go-widgets`: **dependency-free, stdlib-only,
100% coverage** (library packages *and* `cmd/`). CI enforces the
coverage gate on every push.

## License

BSD-3-Clause. See [LICENSE](./LICENSE).
