// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// tui-explorer is the reference interactive demo for tui.App: a small
// k9s-style file-browser mockup composed from toolkit widgets. The
// layout is
//
//	+-----------------------------------------------------+
//	| MenuBar: File / View / Help                         |
//	+----------------------+------------------------------+
//	| TreeView             | Notebook: content | info     |
//	| (3-level fixed FS)   |                              |
//	|                      |                              |
//	+----------------------+------------------------------+
//	| Statusbar: q: quit  ?: help  /: search  ...         |
//	+-----------------------------------------------------+
//
// Keys wired into App.Keys:
//
//	q, Ctrl+C  → Quit
//	?          → toggle help popover
//	/          → toggle search popover
//	Enter      → sync selected tree node into the notebook body
package main

import (
	"flag"
	"io"
	"os"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit / newAppFunc are dependency-injection seams so
// tests drive main() through run() without spawning a subprocess or
// entering an event loop. Tests override newAppFunc + runAppFunc to
// short-circuit the interactive event loop; production reaches the
// real tui.App via the defaults.
var (
	runFunc    = run
	osExit     = os.Exit
	newAppFunc = tui.NewApp
	runAppFunc = defaultRunApp
)

// defaultRunApp is the production runAppFunc: hand off to tui.App's
// event loop. Named (rather than an inline closure) so its return
// statement is a testable function tests can cover directly.
func defaultRunApp(a *tui.App) int { return a.Run() }

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags (--theme=light|dark), composes the demo, installs
// the key bindings, and hands control to App.Run. Returns 0 on
// clean exit, 2 on flag-parse error, and whatever App.Run returns on
// event-loop error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-explorer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st := newState()
	app := newAppFunc()
	app.Root = st.root
	if *theme == "dark" {
		app.Theme = toolkit.DefaultDark()
	} else {
		app.Theme = toolkit.DefaultLight()
	}
	for k, h := range st.keys() {
		app.Keys[k] = h
	}
	return runAppFunc(app)
}

// state bundles every mutable widget the demo builds so key handlers
// can address them by field name without threading a container tree.
type state struct {
	fileList      *fileList
	content       *toolkit.TextView
	status        *toolkit.Label
	menuBar       *toolkit.Label
	helpPopover   *toolkit.Popover
	searchPopover *toolkit.Popover
	root          *packedVBox
	files         map[string]string
	paths         []string // flat, order-stable list of file paths for fileList indexing
}

// newState builds the interactive demo's widget tree. Cell-mode
// friendly composition:
//
//   - header (Label) — menu-style hint at row 0
//   - body — an HBox-ish split of a fileList (left) + TextView (right)
//     laid out by a hSplit local helper so cell-scale bounds are
//     respected (toolkit.HPaned pixel-tunes its splitter offset).
//   - footer (Label) — key hints at row H-1
//
// The pre-v0.3.4 tree/notebook/HPaned composition rendered poorly in
// cell mode — TreeView row heights + Notebook body padding are
// pixel-scaled. This composition uses only widgets that render at
// exactly 1 cell per glyph.
func newState() *state {
	files := map[string]string{
		"/src/main.go":    "package main\n\nfunc main() {}\n",
		"/src/util.go":    "package util\n\nfunc util() {}\n",
		"/docs/README.md": "# Project\n\nDemo project.\n",
		"/LICENSE":        "BSD-3-Clause\n",
	}
	paths := []string{"/src/main.go", "/src/util.go", "/docs/README.md", "/LICENSE"}

	fl := &fileList{items: paths, selected: 0}
	content := toolkit.NewTextView(files[paths[0]])
	content.Focused = false

	body := &hSplit{left: fl, right: content, leftFrac: 30}

	menuBar := toolkit.NewLabel("File   View   Help")
	status := toolkit.NewLabel("q: quit  ?: help  /: search  Up/Down: navigate  Enter: open")

	helpPopover := toolkit.NewPopover(toolkit.NewLabel(
		"q: quit  ?: help  /: search  Up/Down: navigate  Enter: open",
	))
	helpPopover.Title = "Help"
	searchPopover := toolkit.NewPopover(toolkit.NewSearchEntry(""))
	searchPopover.Title = "Search"

	pv := &packedVBox{
		header:   menuBar,
		body:     body,
		footer:   status,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{helpPopover, searchPopover},
	}

	return &state{
		fileList:      fl,
		content:       content,
		status:        status,
		menuBar:       menuBar,
		helpPopover:   helpPopover,
		searchPopover: searchPopover,
		root:          pv,
		files:         files,
		paths:         paths,
	}
}

// syncContent refreshes the right pane with the file at the currently
// selected fileList index.
func (s *state) syncContent() {
	if s.fileList.selected < 0 || s.fileList.selected >= len(s.paths) {
		s.content.SetText("(no selection)")
		return
	}
	s.content.SetText(s.files[s.paths[s.fileList.selected]])
}

// fileList is a cell-native list widget: one item per row, selection
// highlight, arrow-key navigation. Replaces TreeView + Notebook +
// HPaned which are all pixel-tuned and render poorly in cell mode.
type fileList struct {
	toolkit.Base
	items    []string
	selected int
}

func (f *fileList) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := f.Bounds()
	for i, item := range f.items {
		y := r.Y + i
		if y >= r.Y+r.H {
			break
		}
		ink := theme.OnSurface
		if i == f.selected {
			// Selected row: accent-fill background, background-color ink.
			for x := r.X; x < r.X+r.W; x++ {
				p.PutPixel(x, y, theme.Accent)
			}
			ink = theme.Background
		}
		toolkit.DrawText(p, r.X+1, y, item, ink)
	}
}

func (f *fileList) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventKeyDown {
		return
	}
	switch ev.Code {
	case "Up":
		if f.selected > 0 {
			f.selected--
		}
	case "Down":
		if f.selected < len(f.items)-1 {
			f.selected++
		}
	}
}

// hSplit is a cell-native horizontal split: left widget takes
// leftFrac percent of the width, right widget takes the rest.
type hSplit struct {
	toolkit.Base
	left, right toolkit.Widget
	leftFrac    int
}

func (h *hSplit) SetBounds(r toolkit.Rect) {
	h.Base.SetBounds(r)
	lw := r.W * h.leftFrac / 100
	if h.left != nil {
		h.left.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: lw, H: r.H})
	}
	if h.right != nil {
		h.right.SetBounds(toolkit.Rect{X: r.X + lw, Y: r.Y, W: r.W - lw, H: r.H})
	}
}

func (h *hSplit) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if h.left != nil {
		h.left.Draw(pnt, theme)
	}
	if h.right != nil {
		h.right.Draw(pnt, theme)
	}
}

func (h *hSplit) OnEvent(ev toolkit.Event) {
	// Forward to left pane (fileList) for arrow-key navigation.
	// The right pane is read-only content, no events needed.
	if h.left != nil {
		h.left.OnEvent(ev)
	}
}

// packedVBox lays out three children with fixed-height header +
// footer and a body that fills the remaining vertical space, plus
// optional overlays that draw on top of everything else. Suitable
// for terminal-scale layouts where toolkit.VBox's equal-height
// distribution would make every element unusably big.
//
// SetBounds re-runs the layout at every resize; Draw paints in
// draw-order (body → header → footer → overlays), so overlays land
// on top and are visible even when their bounds intersect other
// children; OnEvent forwards to body (the main interactive area).
//
// Overlays are widget references appended to `overlays` at wire
// time. A widget's own `Visible` field (Popover, Toast, …) gates
// whether Draw actually paints — the packedVBox unconditionally
// dispatches Draw to every registered overlay every frame.
type packedVBox struct {
	toolkit.Base
	header   toolkit.Widget
	body     toolkit.Widget
	footer   toolkit.Widget
	headerH  int
	footerH  int
	overlays []toolkit.Widget
}

func (p *packedVBox) SetBounds(r toolkit.Rect) {
	p.Base.SetBounds(r)
	if p.header != nil {
		p.header.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: p.headerH})
	}
	if p.footer != nil {
		p.footer.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + r.H - p.footerH, W: r.W, H: p.footerH})
	}
	if p.body != nil {
		p.body.SetBounds(toolkit.Rect{
			X: r.X,
			Y: r.Y + p.headerH,
			W: r.W,
			H: r.H - p.headerH - p.footerH,
		})
	}
	// Overlays: centred in the body area with 4-cell padding.
	for _, o := range p.overlays {
		bx := r.X + 4
		by := r.Y + p.headerH + 2
		bw := r.W - 8
		bh := r.H - p.headerH - p.footerH - 4
		if bw < 1 {
			bw = 1
		}
		if bh < 1 {
			bh = 1
		}
		o.SetBounds(toolkit.Rect{X: bx, Y: by, W: bw, H: bh})
	}
}

func (p *packedVBox) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if p.body != nil {
		p.body.Draw(pnt, theme)
	}
	if p.header != nil {
		p.header.Draw(pnt, theme)
	}
	if p.footer != nil {
		p.footer.Draw(pnt, theme)
	}
	for _, o := range p.overlays {
		o.Draw(pnt, theme)
	}
}

func (p *packedVBox) OnEvent(ev toolkit.Event) {
	if p.body != nil {
		p.body.OnEvent(ev)
	}
}

// keys returns the App-level key bindings the demo installs. Kept
// as a method rather than an inline map literal so tests can exercise
// each handler in isolation without stepping through Run().
func (s *state) keys() map[string]func(*tui.App) {
	return map[string]func(*tui.App){
		"q":      func(a *tui.App) { a.Quit() },
		"Ctrl+C": func(a *tui.App) { a.Quit() },
		"?": func(a *tui.App) {
			s.helpPopover.Visible = !s.helpPopover.Visible
			a.Refresh()
		},
		"/": func(a *tui.App) {
			s.searchPopover.Visible = !s.searchPopover.Visible
			a.Refresh()
		},
		"Up": func(a *tui.App) {
			if s.fileList.selected > 0 {
				s.fileList.selected--
				s.syncContent()
				a.Consume()
			}
		},
		"Down": func(a *tui.App) {
			if s.fileList.selected < len(s.fileList.items)-1 {
				s.fileList.selected++
				s.syncContent()
				a.Consume()
			}
		},
		"Enter": func(a *tui.App) {
			s.syncContent()
			a.Refresh()
		},
	}
}
