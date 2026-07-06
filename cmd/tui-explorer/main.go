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
	"strings"

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
	tree          *toolkit.TreeView
	notebook      *toolkit.Notebook
	contentTV     *toolkit.TextView
	infoLabel     *toolkit.Label
	status        *toolkit.Statusbar
	menuBar       *toolkit.MenuBar
	helpPopover   *toolkit.Popover
	searchPopover *toolkit.Popover
	root          toolkit.Widget
	files         map[string]string
}

// newState builds the full widget tree — 3-level TreeView, two-tab
// Notebook, MenuBar+Statusbar chrome, help/search popovers — and
// returns it composed into a single root VBox.
func newState() *state {
	files := map[string]string{
		"/src/main.go":    "package main\n\nfunc main() {}\n",
		"/src/util.go":    "package util\n\nfunc util() {}\n",
		"/docs/README.md": "# Project\n\nDemo project.\n",
		"/LICENSE":        "BSD-3-Clause\n",
	}

	// 3-level tree: root → dir → file.
	srcDir := &toolkit.TreeNode{Label: "src", Expanded: true, Children: []*toolkit.TreeNode{
		{Label: "main.go", Data: "/src/main.go"},
		{Label: "util.go", Data: "/src/util.go"},
	}}
	docsDir := &toolkit.TreeNode{Label: "docs", Expanded: true, Children: []*toolkit.TreeNode{
		{Label: "README.md", Data: "/docs/README.md"},
	}}
	licenseNode := &toolkit.TreeNode{Label: "LICENSE", Data: "/LICENSE"}
	root := &toolkit.TreeNode{
		Label:    "/",
		Expanded: true,
		Children: []*toolkit.TreeNode{srcDir, docsDir, licenseNode},
	}
	tree := toolkit.NewTreeView(root)

	// Notebook body: "content" tab holds a TextView; "info" tab
	// holds a Label showing the selected node's name.
	contentTV := toolkit.NewTextView("")
	infoLabel := toolkit.NewLabel("(no selection)")
	notebook := toolkit.NewNotebook()
	notebook.AddTab("content", contentTV)
	notebook.AddTab("info", infoLabel)

	// Split: left pane = tree, right pane = notebook.
	hpaned := toolkit.NewHPaned(tree, notebook)

	// Chrome: MenuBar + Statusbar.
	menuBar := toolkit.NewMenuBar()
	menuBar.Names = []string{"File", "View", "Help"}
	menuBar.Menus = []*toolkit.Menu{
		toolkit.NewMenu([]toolkit.MenuItem{{Label: "Open"}, {Label: "Quit"}}),
		toolkit.NewMenu([]toolkit.MenuItem{{Label: "Refresh"}}),
		toolkit.NewMenu([]toolkit.MenuItem{{Label: "About"}}),
	}
	status := toolkit.NewStatusbar([]string{
		"q: quit  ?: help  /: search  ↑↓: navigate",
	})

	// Overlays: help + search popovers, both hidden until toggled.
	helpPopover := toolkit.NewPopover(toolkit.NewLabel(
		"q: quit  ?: help  /: search  Up/Down: navigate  Enter: open",
	))
	helpPopover.Title = "Help"
	searchPopover := toolkit.NewPopover(toolkit.NewSearchEntry(""))
	searchPopover.Title = "Search"

	// Compose: menuBar / hpaned / statusbar stacked vertically.
	vbox := toolkit.NewVBox()
	vbox.Append(menuBar)
	vbox.Append(hpaned)
	vbox.Append(status)

	return &state{
		tree:          tree,
		notebook:      notebook,
		contentTV:     contentTV,
		infoLabel:     infoLabel,
		status:        status,
		menuBar:       menuBar,
		helpPopover:   helpPopover,
		searchPopover: searchPopover,
		root:          vbox,
		files:         files,
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
		"Enter": func(a *tui.App) {
			s.syncSelection()
			a.Refresh()
		},
	}
}

// syncSelection copies the TreeView's Selected node into the
// notebook's content + info tabs: info tab shows the node's Label,
// content tab shows the file body looked up in files[Data]. A nil
// selection or a directory node (no file path) leaves the tabs
// unchanged.
func (s *state) syncSelection() {
	sel := s.tree.Selected
	if sel == nil {
		return
	}
	s.infoLabel.Text = sel.Label
	path, ok := sel.Data.(string)
	if !ok {
		return
	}
	content, ok := s.files[path]
	if !ok {
		return
	}
	s.contentTV.Lines = strings.Split(content, "\n")
}
