// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// tui-explorer is the reference interactive demo for tui.App: a small
// k9s-style file-browser mockup composed entirely from the exported
// cell-native widgets in the tui package. The layout is
//
//	+-----------------------------------------------------+
//	| MenuBar: File / View / Help                         |
//	+----------------------+------------------------------+
//	| ListBox              | TextEditor (read-only)       |
//	| (flat file list)     | (syntax-highlighted preview) |
//	|                      |                              |
//	+----------------------+------------------------------+
//	| Statusbar: q: quit  ?: help  /: search  ...         |
//	+-----------------------------------------------------+
//
// The chrome — MenuBar, anchored MenuDropdowns, the HSplit sidebar, the
// help/search Popovers, and the header/body/footer VBox — are all
// tui.* library widgets; the demo owns only the wiring (which file maps
// to which preview) and the key bindings.
//
// Keys wired into App.Keys:
//
//	q, Ctrl+C  → Quit
//	?          → toggle help popover
//	/          → toggle search popover
//	Enter      → sync selected list row into the preview
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
	st.app = app       // enables the search finder's InputTarget routing
	st.quit = app.Quit // late-bind the File → Quit menu action
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
	fileList      *tui.ListBox
	content       *tui.TextEditor
	status        *toolkit.Label
	menuBar       *tui.MenuBar
	body          *tui.HSplit // sidebar/preview split — toggled by View → Toggle sidebar
	helpPopover   *tui.Popover
	searchPopover *tui.Popover
	search        *tui.Entry     // the finder's query input (mirrored into searchPopover)
	capture       *searchCapture // App.InputTarget while the finder is open
	fileDropdown  *tui.MenuDropdown
	viewDropdown  *tui.MenuDropdown
	helpDropdown  *tui.MenuDropdown
	root          *tui.VBox
	files         map[string]string
	paths         []string // full, order-stable file list (fileList.Items is the filtered view)
	searching     bool
	app           *tui.App // late-bound in run() for InputTarget routing
	quit          func()   // late-bound in run() = app.Quit
}

// newState composes the demo out of tui library widgets:
//
//   - header (tui.MenuBar) — File / View / Help at row 0
//   - body (tui.HSplit) — a tui.ListBox (left) + read-only tui.TextEditor
//     (right), with a draggable grip between them
//   - footer (toolkit.Label) — key hints at row H-1
//   - overlays — help/search tui.Popovers + the three anchored
//     tui.MenuDropdowns, floated on top by the tui.VBox
func newState() *state {
	files := map[string]string{
		"/src/main.go":    "package main\n\nfunc main() {}\n",
		"/src/util.go":    "package util\n\nfunc util() {}\n",
		"/docs/README.md": "# Project\n\nDemo project.\n",
		"/LICENSE":        "BSD-3-Clause\n",
	}
	paths := []string{"/src/main.go", "/src/util.go", "/docs/README.md", "/LICENSE"}

	fl := &tui.ListBox{Items: paths} // Selected defaults to 0
	content := &tui.TextEditor{ReadOnly: true, ShowGutter: true}
	content.Filename = paths[0]
	content.SetText(files[paths[0]])

	body := &tui.HSplit{Left: fl, Right: content, LeftFrac: 30}

	search := &tui.Entry{Placeholder: "type to filter files"}

	status := toolkit.NewLabel("q: quit  ?: help  /: search  Up/Down: navigate  Enter: open")

	helpPopover := &tui.Popover{
		Title: "Help",
		Body: []string{
			"q          Quit",
			"?          Toggle this help",
			"/          Toggle search",
			"Up / Down  Navigate file list",
			"Enter      Refresh content pane",
			"",
			"Mouse:",
			"click file  Select + preview",
			"drag grip   Resize sidebar",
			"click menu  Open dropdown",
		},
	}
	searchPopover := &tui.Popover{
		Title: "Find file",
		Body:  []string{"/", "type to filter — Enter accept, Esc cancel"},
	}
	// Anchored dropdowns sit directly below the menu item. AnchorY=1
	// (headerH), AnchorX is patched in by menuBar.ItemXRange when the
	// item is clicked so the dropdown lines up with its label.
	// Forward-declared so menu closures can call state methods.
	s := &state{}

	fileDropdown := &tui.MenuDropdown{
		Title:   "File",
		Body:    []string{"Open      Enter", "Reload    ", "Quit      q"},
		AnchorY: 1,
	}
	fileDropdown.ItemActions = []func(){
		func() { s.syncContent() }, // Open: preview the selected file
		func() { s.syncContent() }, // Reload: re-read the preview from the store
		func() {
			if s.quit != nil {
				s.quit()
			}
		},
	}
	viewDropdown := &tui.MenuDropdown{
		Title:   "View",
		Body:    []string{"Toggle line numbers", "Toggle sidebar", "Refresh"},
		AnchorY: 1,
	}
	viewDropdown.ItemActions = []func(){
		func() { content.ShowGutter = !content.ShowGutter },
		func() { s.toggleSidebar() },
		func() { s.syncContent() },
	}
	helpDropdown := &tui.MenuDropdown{
		Title: "Help",
		Body: []string{
			"click file  Select + preview",
			"drag grip   Resize sidebar",
			"?           Full help modal",
			"q           Quit",
		},
		AnchorY: 1,
	}

	mb := &tui.MenuBar{}
	mb.Items = []tui.MenuItem{
		{Label: "File", OnClick: func() {
			x0, _ := mb.ItemXRange(0)
			fileDropdown.AnchorX = x0
			fileDropdown.Visible = !fileDropdown.Visible
			// Close the other dropdowns so only one is open at a time.
			viewDropdown.Visible = false
			helpDropdown.Visible = false
		}},
		{Label: "View", OnClick: func() {
			x0, _ := mb.ItemXRange(1)
			viewDropdown.AnchorX = x0
			viewDropdown.Visible = !viewDropdown.Visible
			fileDropdown.Visible = false
			helpDropdown.Visible = false
		}},
		{Label: "Help", OnClick: func() {
			x0, _ := mb.ItemXRange(2)
			helpDropdown.AnchorX = x0
			helpDropdown.Visible = !helpDropdown.Visible
			fileDropdown.Visible = false
			viewDropdown.Visible = false
		}},
	}

	pv := &tui.VBox{
		Header:   mb,
		Body:     body,
		Footer:   status,
		HeaderH:  1,
		FooterH:  1,
		Overlays: []toolkit.Widget{helpPopover, searchPopover, fileDropdown, viewDropdown, helpDropdown},
	}

	capture := &searchCapture{st: s}
	*s = state{
		fileList:      fl,
		content:       content,
		status:        status,
		menuBar:       mb,
		body:          body,
		helpPopover:   helpPopover,
		searchPopover: searchPopover,
		search:        search,
		capture:       capture,
		fileDropdown:  fileDropdown,
		viewDropdown:  viewDropdown,
		helpDropdown:  helpDropdown,
		root:          pv,
		files:         files,
		paths:         paths,
	}
	// Wire the ListBox → syncContent bridge so a click (or an arrow key
	// routed through the widget instead of the App key map) refreshes the
	// right pane. The App-level Up/Down handlers stay in place too.
	fl.OnSelect = func(int) { s.syncContent() }
	return s
}

// syncContent refreshes the right pane with the currently selected file. It
// indexes the *visible* list (fileList.Items), which equals paths when unfiltered
// and the match subset while the finder is active.
func (s *state) syncContent() {
	if s.fileList.Selected < 0 || s.fileList.Selected >= len(s.fileList.Items) {
		s.content.Filename = ""
		s.content.SetText("(no selection)")
		return
	}
	path := s.fileList.Items[s.fileList.Selected]
	s.content.Filename = path
	s.content.SetText(s.files[path])
}

// toggleSidebar collapses the sidebar (LeftFrac 0) or restores it (30), then
// re-lays-out the split so the panes reflow immediately.
func (s *state) toggleSidebar() {
	if s.body.LeftFrac == 0 {
		s.body.LeftFrac = 30
	} else {
		s.body.LeftFrac = 0
	}
	s.body.SetBounds(s.body.Bounds())
}

// searchCapture is the App.InputTarget while the finder is open: each keystroke
// edits the query entry, then re-filters the list. Never drawn (input-only).
type searchCapture struct {
	toolkit.Base
	st *state
}

func (c *searchCapture) Draw(painter.Painter, *toolkit.Theme) {}

func (c *searchCapture) OnEvent(ev toolkit.Event) {
	c.st.search.OnEvent(ev)
	c.st.applyFilter()
}

// applyFilter narrows fileList.Items to the paths containing the (lower-cased)
// query, refreshes the finder overlay and the preview.
func (s *state) applyFilter() {
	q := strings.ToLower(s.search.Text)
	if q == "" {
		s.fileList.Items = s.paths
	} else {
		matches := []string{}
		for _, p := range s.paths {
			if strings.Contains(strings.ToLower(p), q) {
				matches = append(matches, p)
			}
		}
		s.fileList.Items = matches
	}
	if s.fileList.Selected >= len(s.fileList.Items) {
		s.fileList.Selected = 0
	}
	s.searchPopover.Body = []string{
		"/" + s.search.Text,
		fmt.Sprintf("%d match(es) — Enter accept, Esc cancel", len(s.fileList.Items)),
	}
	s.syncContent()
}

// openSearch enters finder mode: reset the query, show the overlay, and route
// typing into the query entry via InputTarget.
func (s *state) openSearch() {
	s.searching = true
	s.search.Text = ""
	s.search.Cursor = 0
	s.searchPopover.Visible = true
	if s.app != nil {
		s.app.InputTarget = s.capture
	}
	s.applyFilter()
}

// closeSearch leaves finder mode. When restore is true (Esc / cancel) the full
// file list comes back; when false (Enter / accept) the current match subset is
// kept so the user browses the filtered results.
func (s *state) closeSearch(restore bool) {
	s.searching = false
	s.searchPopover.Visible = false
	if s.app != nil {
		s.app.InputTarget = nil
	}
	if restore {
		s.fileList.Items = s.paths
	}
	s.syncContent() // guards an out-of-range selection on its own
}

// keys returns the App key bindings. Extracted as a method rather than an
// inline map literal so tests can exercise each handler in isolation without
// stepping through Run().
func (s *state) keys() map[string]func(*tui.App) {
	return map[string]func(*tui.App){
		// Ctrl+C always quits. The letter keys (q / ? / /) are guarded by
		// !searching so, while the finder is open, they fall through to the
		// query entry instead of triggering their command.
		"Ctrl+C": func(a *tui.App) { a.Quit() },
		"q": func(a *tui.App) {
			if !s.searching {
				a.Quit()
				a.Consume()
			}
		},
		"?": func(a *tui.App) {
			if !s.searching {
				s.helpPopover.Visible = !s.helpPopover.Visible
				a.Refresh()
				a.Consume()
			}
		},
		"/": func(a *tui.App) {
			if !s.searching {
				s.openSearch()
				a.Refresh()
				a.Consume()
			}
		},
		"Escape": func(a *tui.App) {
			if s.searching {
				s.closeSearch(true) // cancel — restore the full list
				a.Refresh()
				a.Consume()
			}
		},
		"Up": func(a *tui.App) {
			if s.fileList.Selected > 0 {
				s.fileList.Selected--
				s.syncContent()
				a.Consume()
			}
		},
		"Down": func(a *tui.App) {
			if s.fileList.Selected < len(s.fileList.Items)-1 {
				s.fileList.Selected++
				s.syncContent()
				a.Consume()
			}
		},
		"Enter": func(a *tui.App) {
			if s.searching {
				s.closeSearch(false) // accept — keep the filtered list
			} else {
				s.syncContent()
			}
			a.Refresh()
			a.Consume()
		},
	}
}
