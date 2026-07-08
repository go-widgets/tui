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
//     Ctrl+C → Quit, Ctrl+S → save, Ctrl+P → Palette.
//     Every printable key is consumed so nothing leaks
//     into the buffer.
//   - Edit     — every key falls through to TextView.OnEvent, so
//     arrow keys move the cursor, Backspace deletes, and
//     printable characters insert. Escape returns to View.
//     Ctrl+S saves (consumed, does not insert Ctrl+S into
//     the buffer).
//   - Palette  — keys forward to a SearchEntry inside a Popover.
//     Enter runs the typed command ("save", "quit"); Escape
//     returns to View.
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
	st.tv.Filename = *filePath // drives syntax highlighting of the buffer
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
	st.app = app // enables palette InputTarget routing
	app.Root = st.root
	if *themeName == "dark" {
		app.Theme = toolkit.DefaultDark()
	} else {
		app.Theme = toolkit.DefaultLight()
	}
	for k, h := range st.keys() {
		app.Keys[k] = h
	}
	// Late-bind the File → Quit menu action now that the App exists.
	st.quit = app.Quit
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
	mode         mode
	file         string // "" == unnamed buffer
	dirty        bool
	tv           *tui.TextEditor
	statusbar    *toolkit.Label
	menuBar      *tui.MenuBar
	palette      *tui.Popover
	paletteEn    *tui.Entry
	capture      *paletteCapture // App.InputTarget while the palette is open
	app          *tui.App        // late-bound in run() for InputTarget routing
	fileDropdown *tui.MenuDropdown
	editDropdown *tui.MenuDropdown
	viewDropdown *tui.MenuDropdown
	helpDropdown *tui.MenuDropdown
	root         toolkit.Widget

	// I/O seams so tests exercise load/save without hitting the real
	// filesystem.
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error

	// quit is a late-bound closure the "File → Quit" menu item calls.
	// newState() leaves it as a no-op; run() overwrites it with
	// app.Quit once the App is created (a menu item created before
	// the App exists can't hold a direct app reference).
	quit func()
}

// newState builds the widget tree from tui library widgets: the
// read-write tui.TextEditor at centre, a tui.MenuBar header and a
// toolkit.Label status footer laid out by a tui.VBox (header + footer
// stay 1 cell tall, the editor takes the rest), and the anchored
// tui.MenuDropdowns + command-palette tui.Popover floated as overlays.
func newState() *state {
	tv := tui.NewTextEditor() // ShowGutter=true, one empty line, spans primed
	tv.Focused = true

	statusbar := toolkit.NewLabel("VIEW  |  *scratch*  |  1:1")

	paletteEn := &tui.Entry{Placeholder: "save · quit · new · e <path> · find <text>"}
	palette := &tui.Popover{
		Title: "Command palette",
		Body:  []string{}, // mirrors paletteEn while the palette is open
	}
	capture := &paletteCapture{entry: paletteEn, pop: palette}

	// Forward-declared so state's `quit` closure can be captured
	// inside the fileDropdown before newState() returns.
	s := &state{}

	fileDropdown := &tui.MenuDropdown{
		Title:   "File",
		Body:    []string{"New       Ctrl+N", "Open      :e <path>", "Save      Ctrl+S", "Quit      q"},
		AnchorY: 1,
	}
	fileDropdown.ItemActions = []func(){
		func() { s.newBuffer() },
		func() { s.openPalettePrefill("e ") },
		func() { _ = s.save() },
		func() {
			if s.quit != nil {
				s.quit()
			}
		},
	}
	editDropdown := &tui.MenuDropdown{
		Title:   "Edit",
		Body:    []string{"Undo   Ctrl+Z", "Redo   Ctrl+Y", "Cut    Ctrl+X", "Copy   Ctrl+C", "Paste  Ctrl+V"},
		AnchorY: 1,
	}
	editDropdown.ItemActions = []func(){
		func() { s.undo() },
		func() { s.redo() },
		func() { _ = tv.Cut() },
		func() { _ = tv.Copy() },
		func() { tv.Paste(); s.refreshStatus() },
	}
	viewDropdown := &tui.MenuDropdown{
		Title:   "View",
		Body:    []string{"Toggle line numbers", "Focus editor         i", "Command palette      Ctrl+P"},
		AnchorY: 1,
	}
	viewDropdown.ItemActions = []func(){
		func() { tv.ShowGutter = !tv.ShowGutter },
		nil, // Focus editor — informational, `i` key already does it
		func() { s.setMode(modePalette) },
	}
	helpDropdown := &tui.MenuDropdown{
		Title: "Help",
		Body: []string{
			"i           Insert mode",
			"Esc         View mode",
			"Ctrl+Z/Y    Undo / Redo",
			"Ctrl+P      Palette",
			"Ctrl+S      Save",
			"q           Quit (view)",
		},
		AnchorY: 1,
	}

	mb := &tui.MenuBar{}
	closeOthers := func(keep *tui.MenuDropdown) {
		for _, d := range []*tui.MenuDropdown{fileDropdown, editDropdown, viewDropdown, helpDropdown} {
			if d != keep {
				d.Visible = false
			}
		}
	}
	mb.Items = []tui.MenuItem{
		{Label: "File", OnClick: func() {
			x0, _ := mb.ItemXRange(0)
			fileDropdown.AnchorX = x0
			fileDropdown.Visible = !fileDropdown.Visible
			closeOthers(fileDropdown)
		}},
		{Label: "Edit", OnClick: func() {
			x0, _ := mb.ItemXRange(1)
			editDropdown.AnchorX = x0
			editDropdown.Visible = !editDropdown.Visible
			closeOthers(editDropdown)
		}},
		{Label: "View", OnClick: func() {
			x0, _ := mb.ItemXRange(2)
			viewDropdown.AnchorX = x0
			viewDropdown.Visible = !viewDropdown.Visible
			closeOthers(viewDropdown)
		}},
		{Label: "Help", OnClick: func() {
			x0, _ := mb.ItemXRange(3)
			helpDropdown.AnchorX = x0
			helpDropdown.Visible = !helpDropdown.Visible
			closeOthers(helpDropdown)
		}},
	}

	body := tv

	root := &tui.VBox{
		Header:   mb,
		Body:     body,
		Footer:   statusbar,
		HeaderH:  1,
		FooterH:  1,
		Overlays: []toolkit.Widget{palette, fileDropdown, editDropdown, viewDropdown, helpDropdown},
	}

	// Populate the forward-declared state now that everything's wired.
	*s = state{
		mode:         modeView,
		tv:           tv,
		statusbar:    statusbar,
		menuBar:      mb,
		palette:      palette,
		paletteEn:    paletteEn,
		capture:      capture,
		fileDropdown: fileDropdown,
		editDropdown: editDropdown,
		viewDropdown: viewDropdown,
		helpDropdown: helpDropdown,
		root:         root,
		readFile:     os.ReadFile,
		writeFile:    os.WriteFile,
	}
	return s
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
		s.palette.Body = []string{"> " + s.paletteEn.Text}
		if s.app != nil {
			s.app.InputTarget = s.capture // route typing into the palette entry
		}
	} else {
		s.palette.Visible = false
		if s.app != nil {
			s.app.InputTarget = nil
		}
	}
	s.refreshStatus()
}

// paletteCapture is the App.InputTarget while the command palette is open: it
// forwards each keystroke to the entry and mirrors the entry's text into the
// palette popover so the user sees the command as they type. It is never drawn
// (input-only), so Draw is a no-op.
type paletteCapture struct {
	toolkit.Base
	entry *tui.Entry
	pop   *tui.Popover
}

func (c *paletteCapture) Draw(painter.Painter, *toolkit.Theme) {}

func (c *paletteCapture) OnEvent(ev toolkit.Event) {
	c.entry.OnEvent(ev)
	c.pop.Body = []string{"> " + c.entry.Text}
}

// newBuffer resets the editor to a fresh empty *scratch* buffer:
// clears the text, drops the file name + dirty flag, and refreshes
// the status bar. Shared by both the menu path
// (fileDropdown.ItemActions[0]) and the keyboard path (Ctrl+N).
//
// Does NOT prompt to save an unsaved buffer — the demo trusts the
// user; real editors would gate this behind a "buffer dirty?" modal.
func (s *state) newBuffer() {
	s.tv.SetText("")
	s.tv.Filename = ""
	s.file = ""
	s.dirty = false
	s.refreshStatus()
}

// openFile loads path into the buffer via the readFile seam; on a read error the
// buffer is left unchanged and the error is returned.
func (s *state) openFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	body, err := s.readFile(path)
	if err != nil {
		return err
	}
	s.file = path
	s.tv.Filename = path
	s.tv.SetText(string(body))
	s.dirty = false
	s.refreshStatus()
	return nil
}

// openPalettePrefill opens the command palette with prefix already typed (the
// caret parked at the end), so "File → Open" drops the user straight into a
// ":e " command ready for a path.
func (s *state) openPalettePrefill(prefix string) {
	s.paletteEn.Text = prefix
	s.paletteEn.Cursor = len([]rune(prefix))
	s.setMode(modePalette)
}

// undo / redo forward the underlying TextEditor's Ctrl+Z / Ctrl+Y
// handling, then refresh the status bar so the caret-position
// segment tracks the new cursor. Shared by both the menu path
// (editDropdown.ItemActions) and the keyboard path (s.keys()).
func (s *state) undo() {
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+Z"})
	s.refreshStatus()
}

func (s *state) redo() {
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+Y"})
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
	cmd = strings.TrimSpace(cmd)
	switch {
	case cmd == "save":
		return s.save()
	case cmd == "quit", cmd == "q":
		a.Quit()
	case cmd == "new":
		s.newBuffer()
	case strings.HasPrefix(cmd, "find "):
		s.tv.Find(strings.TrimSpace(strings.TrimPrefix(cmd, "find ")))
	case strings.HasPrefix(cmd, "e "):
		return s.openFile(strings.TrimPrefix(cmd, "e "))
	case strings.HasPrefix(cmd, "open "):
		return s.openFile(strings.TrimPrefix(cmd, "open "))
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
				s.paletteEn.Cursor = 0
				s.setMode(modeView)
				a.Consume()
			}
			// In edit mode, Enter falls through to TextView (splits
			// the line at the cursor). Nothing to do here.
		},
		// Ctrl+Z / Ctrl+Y — undo/redo. Upstream's tui.TextEditor
		// already handles these on its own OnEvent path (falling
		// through from App to Root), but its handler doesn't call
		// refreshStatus so the caret-position segment stayed stale.
		// Intercept here, dispatch via s.undo/s.redo (which refresh
		// status), and Consume so Root.OnEvent doesn't run a second
		// undo/redo on the same event.
		"Ctrl+Z": func(a *tui.App) {
			s.undo()
			a.Consume()
		},
		"Ctrl+Y": func(a *tui.App) {
			s.redo()
			a.Consume()
		},
		"Ctrl+N": func(a *tui.App) {
			s.newBuffer()
			a.Consume()
		},
	}
}
