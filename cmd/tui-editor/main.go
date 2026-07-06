// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// tui-editor is the reference loom-style interactive demo for
// tui.App: a modal (vi-inspired) text editor built on top of the
// existing toolkit widgets — TextView for the buffer, Statusbar for
// the mode + cursor + filename indicator, MenuBar for the chrome, and
// a Popover-wrapped SearchEntry for the command palette.
//
// Modes:
//
//   - View     — read-only. Keys drive actions: 'i' → Edit, 'q' /
//                Ctrl+C → Quit, Ctrl+S → save, Ctrl+P → Palette.
//                Every printable key is consumed so nothing leaks
//                into the buffer.
//   - Edit     — every key falls through to TextView.OnEvent, so
//                arrow keys move the cursor, Backspace deletes, and
//                printable characters insert. Escape returns to View.
//                Ctrl+S saves (consumed, does not insert Ctrl+S into
//                the buffer).
//   - Palette  — keys forward to a SearchEntry inside a Popover.
//                Enter runs the typed command ("save", "quit"); Escape
//                returns to View.
//
// Run with:
//
//	go run . --file=path/to/file.txt
//	go run . --theme=dark
//
// The demo saves via os.WriteFile (writeFile seam so tests substitute
// a no-op) and loads via os.ReadFile (readFile seam). Both are unset
// when --file is empty; the editor starts on an in-memory buffer.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit / newAppFunc / runAppFunc are dependency-injection
// seams so tests drive main() through run() without spawning a
// subprocess or entering an event loop.
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

// run parses --file + --theme, composes the demo, installs the key
// bindings, and hands control to App.Run. Returns 0 on clean exit,
// 2 on flag-parse error, 4 on file-load error, whatever App.Run
// returns otherwise.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-editor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	filePath := fs.String("file", "", "open the given file at startup")
	themeName := fs.String("theme", "light", "theme (light|dark)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st := newState()
	st.file = *filePath
	if *filePath != "" {
		body, err := st.readFile(*filePath)
		if err != nil {
			fmt.Fprintf(stderr, "tui-editor: read %s: %v\n", *filePath, err)
			return 4
		}
		st.tv.SetText(string(body))
	}
	st.refreshStatus()

	app := newAppFunc()
	app.Root = st.root
	if *themeName == "dark" {
		app.Theme = toolkit.DefaultDark()
	} else {
		app.Theme = toolkit.DefaultLight()
	}
	for k, h := range st.keys() {
		app.Keys[k] = h
	}
	return runAppFunc(app)
}

// mode is the editor's modal state — swap-in-place by handlers.
type mode int

const (
	modeView mode = iota
	modeEdit
	modePalette
)

// state bundles every mutable widget the editor builds so key
// handlers can address them by field name without threading a
// container tree through every call.
type state struct {
	mode      mode
	file      string // "" == unnamed buffer
	dirty     bool
	tv        *toolkit.TextView
	statusbar *toolkit.Label
	menuBar   *toolkit.Label
	palette   *toolkit.Popover
	paletteEn *toolkit.SearchEntry
	root      toolkit.Widget

	// I/O seams so tests exercise load/save without hitting the real
	// filesystem.
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
}

// newState builds the widget tree — TextView at centre with flat
// Label chrome (menu-style header + status footer) — wired through
// packedVBox so header + footer stay 1 cell tall and TextView takes
// the remaining vertical space. A toolkit.VBox would divide equally,
// giving each element a third of the screen — that layout bug
// shipped in v0.3.0 / v0.3.1 and is what v0.3.2 fixes.
func newState() *state {
	tv := toolkit.NewTextView("")
	tv.Focused = true

	menuBar := toolkit.NewLabel("File   Edit   View   Help")
	statusbar := toolkit.NewLabel("VIEW  |  *scratch*  |  1:1")

	paletteEn := toolkit.NewSearchEntry("")
	palette := toolkit.NewPopover(paletteEn)
	palette.Title = "Command palette"

	root := &packedVBox{
		header:   menuBar,
		body:     tv,
		footer:   statusbar,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{palette},
	}

	return &state{
		mode:      modeView,
		tv:        tv,
		statusbar: statusbar,
		menuBar:   menuBar,
		palette:   palette,
		paletteEn: paletteEn,
		root:      root,
		readFile:  os.ReadFile,
		writeFile: os.WriteFile,
	}
}

// packedVBox — same shape as cmd/tui-explorer's local helper:
// header (fixed) / body (expand) / footer (fixed) + overlay slots
// for popovers. Overlays draw on top of everything else every
// frame; the widget's own Visible field gates whether Draw
// actually paints.
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

// refreshStatus updates the Statusbar segments with the current
// mode name, filename (or "*scratch*" when unnamed) with a dirty
// marker, and the cursor position (1-indexed).
func (s *state) refreshStatus() {
	name := s.file
	if name == "" {
		name = "*scratch*"
	}
	if s.dirty {
		name += " [+]"
	}
	pos := fmt.Sprintf("%d:%d", s.tv.CursorLine+1, s.tv.CursorCol+1)
	s.statusbar.Text = fmt.Sprintf("%s  |  %s  |  %s", modeName(s.mode), name, pos)
}

// modeName returns the human-readable label for m, used in the
// Statusbar. Kept as a switch (rather than a map) so a future
// regression that adds a mode without a label trips at compile time
// via the missing case in the linear scan.
func modeName(m mode) string {
	switch m {
	case modeView:
		return "VIEW"
	case modeEdit:
		return "EDIT"
	case modePalette:
		return "PALETTE"
	default:
		return "?"
	}
}

// setMode swaps the editor's mode and refreshes the Statusbar.
// Handlers call this instead of assigning s.mode directly so the
// status readout always tracks.
func (s *state) setMode(m mode) {
	s.mode = m
	if m == modePalette {
		s.palette.Visible = true
	} else {
		s.palette.Visible = false
	}
	s.refreshStatus()
}

// save writes the buffer to s.file via the writeFile seam. Returns
// nil on success (including the "unnamed buffer, nothing to save"
// case), or the underlying write error.
func (s *state) save() error {
	if s.file == "" {
		return nil
	}
	if err := s.writeFile(s.file, []byte(s.tv.Text()), 0o644); err != nil {
		return err
	}
	s.dirty = false
	s.refreshStatus()
	return nil
}

// runCommand parses + executes a palette command. Recognized
// commands: "save" (calls s.save), "quit" (calls a.Quit). Unknown
// commands are ignored. Returns an error only when save fails.
func (s *state) runCommand(a *tui.App, cmd string) error {
	switch strings.TrimSpace(cmd) {
	case "save":
		return s.save()
	case "quit", "q":
		a.Quit()
	}
	return nil
}

// keys returns the App-level key bindings. All mode-dependent
// dispatching lives inside these handlers via s.mode checks; the App
// itself sees only a flat map.
//
// Handlers call a.Consume() on every event they fully handle so
// nothing leaks into the underlying TextView (e.g. the 'i' key
// entering edit mode must not also insert 'i' at the cursor).
func (s *state) keys() map[string]func(*tui.App) {
	return map[string]func(*tui.App){
		// Global escape hatch — always quits regardless of mode.
		"Ctrl+C": func(a *tui.App) { a.Quit() },

		// Mode-aware handlers below use switch on s.mode so a single
		// binding does the right thing everywhere.
		"q": func(a *tui.App) {
			if s.mode == modeView {
				a.Quit()
				a.Consume()
			}
		},
		"i": func(a *tui.App) {
			if s.mode == modeView {
				s.setMode(modeEdit)
				a.Consume()
			}
		},
		"Escape": func(a *tui.App) {
			if s.mode == modeEdit || s.mode == modePalette {
				s.setMode(modeView)
				a.Consume()
			}
		},
		"Ctrl+S": func(a *tui.App) {
			if err := s.save(); err == nil {
				a.Refresh()
			}
			a.Consume()
		},
		"Ctrl+P": func(a *tui.App) {
			if s.mode != modePalette {
				s.setMode(modePalette)
			}
			a.Consume()
		},
		"Enter": func(a *tui.App) {
			if s.mode == modePalette {
				_ = s.runCommand(a, s.paletteEn.Text)
				s.paletteEn.Text = ""
				s.setMode(modeView)
				a.Consume()
			}
			// In edit mode, Enter falls through to TextView (splits
			// the line at the cursor). Nothing to do here.
		},
	}
}
