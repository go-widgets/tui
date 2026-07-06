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
	tv        *cellTextEdit
	statusbar *toolkit.Label
	menuBar   *toolkit.Label
	palette   *cellPopover
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
// the remaining vertical space. The body wraps TextView in a
// cellBodyFill helper so the pane background uses theme.SurfaceAlt
// (perceptible against the near-black chrome in dark mode); a bare
// toolkit.TextView draws only its own border + cursor and leaves
// the pane background inheriting the frame Background fill, which
// makes the panel boundary imperceptible in dark theme.
func newState() *state {
	tv := &cellTextEdit{Focused: true, Lines: []string{""}}

	menuBar := toolkit.NewLabel("File   Edit   View   Help")
	statusbar := toolkit.NewLabel("VIEW  |  *scratch*  |  1:1")

	paletteEn := toolkit.NewSearchEntry("")
	palette := &cellPopover{
		Title: "Command palette",
		Body:  []string{}, // populated when the user types via paletteEn
	}

	body := tv

	root := &packedVBox{
		header:   menuBar,
		body:     body,
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

// cellTextEdit is a cell-native editable text buffer: 1 cell per
// glyph, arrow-key navigation, insert on EventChar, Backspace, Enter
// to split a line. Fills its bounds with SurfaceAlt so the pane
// boundary is visible against near-black chrome rows in dark theme.
// Replaces toolkit.TextView which uses lineH = GlyphHeight + 4 = 11
// cells per line in cell mode (only 2 lines visible in a 22-row
// body) + fills with Surface (imperceptible from Background).
type cellTextEdit struct {
	toolkit.Base
	Lines      []string
	CursorLine int
	CursorCol  int
	Focused    bool
}

func (t *cellTextEdit) Text() string {
	if len(t.Lines) == 0 {
		return ""
	}
	total := 0
	for _, l := range t.Lines {
		total += len(l) + 1
	}
	buf := make([]byte, 0, total)
	for i, l := range t.Lines {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, l...)
	}
	return string(buf)
}

func (t *cellTextEdit) SetText(s string) {
	t.Lines = nil
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			t.Lines = append(t.Lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		t.Lines = append(t.Lines, s[start:])
	}
	if len(t.Lines) == 0 {
		t.Lines = []string{""}
	}
	t.CursorLine = 0
	t.CursorCol = 0
}

func (t *cellTextEdit) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := t.Bounds()
	// SurfaceAlt-filled pane background — perceptibly different from
	// chrome (Background) so the panel edge is visible in any theme.
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	for i, line := range t.Lines {
		y := r.Y + i
		if y >= r.Y+r.H {
			break
		}
		toolkit.DrawText(pnt, r.X+1, y, line, ink)
	}
	// Cursor: reverse-video single cell at the caret position.
	if t.Focused && t.CursorLine >= 0 && t.CursorLine < r.H {
		cx := r.X + 1 + t.CursorCol
		cy := r.Y + t.CursorLine
		if cx < r.X+r.W && cy < r.Y+r.H {
			pnt.FillRect(painter.Rect{X: cx, Y: cy, W: 1, H: 1}, painter.RGBA{
				R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A,
			})
		}
	}
}

func (t *cellTextEdit) OnEvent(ev toolkit.Event) {
	if len(t.Lines) == 0 {
		t.Lines = []string{""}
	}
	switch ev.Kind {
	case toolkit.EventChar:
		line := t.Lines[t.CursorLine]
		if t.CursorCol > len(line) {
			t.CursorCol = len(line)
		}
		t.Lines[t.CursorLine] = line[:t.CursorCol] + ev.Code + line[t.CursorCol:]
		t.CursorCol += len(ev.Code)
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Backspace":
			line := t.Lines[t.CursorLine]
			if t.CursorCol > 0 && t.CursorCol <= len(line) {
				t.Lines[t.CursorLine] = line[:t.CursorCol-1] + line[t.CursorCol:]
				t.CursorCol--
			} else if t.CursorCol == 0 && t.CursorLine > 0 {
				prev := t.Lines[t.CursorLine-1]
				t.CursorCol = len(prev)
				t.Lines[t.CursorLine-1] = prev + line
				t.Lines = append(t.Lines[:t.CursorLine], t.Lines[t.CursorLine+1:]...)
				t.CursorLine--
			}
		case "Enter":
			line := t.Lines[t.CursorLine]
			if t.CursorCol > len(line) {
				t.CursorCol = len(line)
			}
			head, tail := line[:t.CursorCol], line[t.CursorCol:]
			t.Lines[t.CursorLine] = head
			t.Lines = append(t.Lines[:t.CursorLine+1], append([]string{tail}, t.Lines[t.CursorLine+1:]...)...)
			t.CursorLine++
			t.CursorCol = 0
		case "Up":
			if t.CursorLine > 0 {
				t.CursorLine--
				if t.CursorCol > len(t.Lines[t.CursorLine]) {
					t.CursorCol = len(t.Lines[t.CursorLine])
				}
			}
		case "Down":
			if t.CursorLine < len(t.Lines)-1 {
				t.CursorLine++
				if t.CursorCol > len(t.Lines[t.CursorLine]) {
					t.CursorCol = len(t.Lines[t.CursorLine])
				}
			}
		case "Left":
			if t.CursorCol > 0 {
				t.CursorCol--
			}
		case "Right":
			if t.CursorCol < len(t.Lines[t.CursorLine]) {
				t.CursorCol++
			}
		}
	}
}

// cellPopover — cell-native modal, same shape as
// cmd/tui-explorer's helper. Reproduced here (each demo owns its
// local copy) rather than shared because these demos are meant to
// be forkable starting points, not a package dependency.
type cellPopover struct {
	toolkit.Base
	Title   string
	Body    []string
	Visible bool
}

func (p *cellPopover) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !p.Visible {
		return
	}
	r := p.Bounds()
	need := 3 + len(p.Body)
	if need > r.H {
		need = r.H
	}
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, painter.RGBA{
		R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: theme.Border.A,
	}, 1)
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	toolkit.DrawText(pnt, r.X+2, r.Y+1, p.Title, ink)
	for i, line := range p.Body {
		y := r.Y + 2 + i
		if y >= r.Y+need-1 {
			break
		}
		toolkit.DrawText(pnt, r.X+2, y, line, ink)
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
