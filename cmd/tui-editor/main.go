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
	mode         mode
	file         string // "" == unnamed buffer
	dirty        bool
	tv           *tui.TextEditor
	statusbar    *toolkit.Label
	menuBar      *menuBar
	palette      *cellPopover
	paletteEn    *toolkit.SearchEntry
	fileDropdown *menuDropdown
	editDropdown *menuDropdown
	viewDropdown *menuDropdown
	helpDropdown *menuDropdown
	root         toolkit.Widget

	// I/O seams so tests exercise load/save without hitting the real
	// filesystem.
	readFile  func(string) ([]byte, error)
	writeFile func(string, []byte, os.FileMode) error
}

// menuItem + menuBar mirror the tui-explorer helpers. Kept as a local
// copy per the "each demo owns its widgets" convention.
type menuItem struct {
	Label   string
	OnClick func()
}

const menuBarSep = 3 // spaces between items
const menuBarPad = 1 // left padding before the first item

type menuBar struct {
	toolkit.Base
	Items []menuItem
}

func (m *menuBar) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := m.Bounds()
	p.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.Background.R, G: theme.Background.G, B: theme.Background.B, A: theme.Background.A,
	})
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	x := r.X + menuBarPad
	for _, item := range m.Items {
		toolkit.DrawText(p, x, r.Y, item.Label, ink)
		x += len(item.Label) + menuBarSep
	}
}

func (m *menuBar) itemXRange(i int) (int, int) {
	x := menuBarPad
	for k, item := range m.Items {
		if k == i {
			return x, x + len(item.Label)
		}
		x += len(item.Label) + menuBarSep
	}
	return -1, -1
}

func (m *menuBar) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	for i, item := range m.Items {
		x0, x1 := m.itemXRange(i)
		if ev.X >= x0 && ev.X < x1 {
			if item.OnClick != nil {
				item.OnClick()
			}
			return
		}
	}
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
	tv := tui.NewTextEditor() // ShowGutter=true, one empty line, spans primed
	tv.Focused = true

	statusbar := toolkit.NewLabel("VIEW  |  *scratch*  |  1:1")

	paletteEn := toolkit.NewSearchEntry("")
	palette := &cellPopover{
		Title: "Command palette",
		Body:  []string{}, // populated when the user types via paletteEn
	}

	fileDropdown := &menuDropdown{
		Title:   "File",
		Body:    []string{"New       (stub)", "Open      (stub)", "Save      Ctrl+S", "Quit      q"},
		AnchorY: 1,
	}
	editDropdown := &menuDropdown{
		Title:   "Edit",
		Body:    []string{"Undo   (stub)", "Redo   (stub)", "Cut    (stub)", "Copy   (stub)", "Paste  (stub)"},
		AnchorY: 1,
	}
	viewDropdown := &menuDropdown{
		Title:   "View",
		Body:    []string{"Toggle line numbers  (stub)", "Focus editor         i", "Command palette      Ctrl+P"},
		AnchorY: 1,
	}
	helpDropdown := &menuDropdown{
		Title: "Help",
		Body: []string{
			"i           Insert mode",
			"Esc         View mode",
			"Ctrl+P      Palette",
			"Ctrl+S      Save",
			"q           Quit (view)",
		},
		AnchorY: 1,
	}

	mb := &menuBar{}
	closeOthers := func(keep *menuDropdown) {
		for _, d := range []*menuDropdown{fileDropdown, editDropdown, viewDropdown, helpDropdown} {
			if d != keep {
				d.Visible = false
			}
		}
	}
	mb.Items = []menuItem{
		{Label: "File", OnClick: func() {
			x0, _ := mb.itemXRange(0)
			fileDropdown.AnchorX = x0
			fileDropdown.Visible = !fileDropdown.Visible
			closeOthers(fileDropdown)
		}},
		{Label: "Edit", OnClick: func() {
			x0, _ := mb.itemXRange(1)
			editDropdown.AnchorX = x0
			editDropdown.Visible = !editDropdown.Visible
			closeOthers(editDropdown)
		}},
		{Label: "View", OnClick: func() {
			x0, _ := mb.itemXRange(2)
			viewDropdown.AnchorX = x0
			viewDropdown.Visible = !viewDropdown.Visible
			closeOthers(viewDropdown)
		}},
		{Label: "Help", OnClick: func() {
			x0, _ := mb.itemXRange(3)
			helpDropdown.AnchorX = x0
			helpDropdown.Visible = !helpDropdown.Visible
			closeOthers(helpDropdown)
		}},
	}

	body := tv

	root := &packedVBox{
		header:   mb,
		body:     body,
		footer:   statusbar,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{palette, fileDropdown, editDropdown, viewDropdown, helpDropdown},
	}

	return &state{
		mode:         modeView,
		tv:           tv,
		statusbar:    statusbar,
		menuBar:      mb,
		palette:      palette,
		paletteEn:    paletteEn,
		fileDropdown: fileDropdown,
		editDropdown: editDropdown,
		viewDropdown: viewDropdown,
		helpDropdown: helpDropdown,
		root:         root,
		readFile:     os.ReadFile,
		writeFile:    os.WriteFile,
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

// HitTest — invisible popovers must not claim clicks. Same rationale
// as tui-explorer's cellPopover.
func (p *cellPopover) HitTest(px, py int) bool {
	if !p.Visible {
		return false
	}
	return p.Base.HitTest(px, py)
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

// menuDropdown — anchored menu-item popover. Same shape as
// tui-explorer's helper. See its docstring for the rationale.
type menuDropdown struct {
	toolkit.Base
	Title   string
	Body    []string
	Visible bool
	AnchorX int
	AnchorY int
}

func (d *menuDropdown) size() (int, int) {
	w := len(d.Title)
	for _, line := range d.Body {
		if l := len(line); l > w {
			w = l
		}
	}
	w += 4
	h := 2 + len(d.Body)
	if h < 3 {
		h = 3
	}
	return w, h
}

func (d *menuDropdown) SetBounds(_ toolkit.Rect) {
	w, h := d.size()
	d.Base.SetBounds(toolkit.Rect{X: d.AnchorX, Y: d.AnchorY, W: w, H: h})
}

func (d *menuDropdown) HitTest(px, py int) bool {
	if !d.Visible {
		return false
	}
	return d.Base.HitTest(px, py)
}

func (d *menuDropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !d.Visible {
		return
	}
	d.SetBounds(toolkit.Rect{})
	r := d.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: theme.Border.A,
	}, 1)
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	if d.Title != "" {
		toolkit.DrawText(pnt, r.X+2, r.Y, d.Title, ink)
	}
	for i, line := range d.Body {
		toolkit.DrawText(pnt, r.X+2, r.Y+1+i, line, ink)
	}
}

func (d *menuDropdown) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventClick {
		d.Visible = false
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
	// Drag capture — see tui-explorer/main.go's packedVBox for the
	// same field group + rationale.
	dragTarget toolkit.Widget
	dragDx     int
	dragDy     int
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
	if p.dragTarget != nil && (ev.Kind == toolkit.EventMouseDrag || ev.Kind == toolkit.EventMouseUp) {
		child := ev
		child.X -= p.dragDx
		child.Y -= p.dragDy
		p.dragTarget.OnEvent(child)
		if ev.Kind == toolkit.EventMouseUp {
			p.dragTarget = nil
			p.dragDx = 0
			p.dragDy = 0
		}
		return
	}
	if ev.Kind == toolkit.EventClick {
		// ev.X/Y are widget-local to packedVBox; route by Y band.
		r := p.Bounds()
		// Overlays hit-test via each widget's HitTest so anchored
		// dropdowns and invisible popovers are handled correctly.
		// Iterate in reverse so later-added overlays (which paint on
		// top) also claim clicks first.
		for i := len(p.overlays) - 1; i >= 0; i-- {
			o := p.overlays[i]
			if !o.HitTest(ev.X, ev.Y) {
				continue
			}
			ob := o.Bounds()
			child := ev
			child.X -= ob.X
			child.Y -= ob.Y
			o.OnEvent(child)
			p.dragTarget, p.dragDx, p.dragDy = o, ob.X, ob.Y
			return
		}
		switch {
		case ev.Y < p.headerH:
			if p.header != nil {
				p.header.OnEvent(ev)
				p.dragTarget, p.dragDx, p.dragDy = p.header, 0, 0
			}
		case ev.Y >= r.H-p.footerH:
			if p.footer != nil {
				child := ev
				child.Y -= r.H - p.footerH
				p.footer.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.footer, 0, r.H-p.footerH
			}
		default:
			if p.body != nil {
				child := ev
				child.Y -= p.headerH
				p.body.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.body, 0, p.headerH
			}
		}
		return
	}
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
