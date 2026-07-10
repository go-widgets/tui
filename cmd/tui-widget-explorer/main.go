// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// tui-widget-explorer is an interactive gallery of the go-widgets/tui
// cell-native widget set: a scrollable list of widget names on the left, a live
// instance of the selected widget on the right. Unlike cmd/tui-widgets (a static
// one-frame snapshot), this is a full tui.App — you can poke each widget:
//
//	↑ / ↓     select a widget (rebuilds the live stage)
//	Tab       move keyboard focus into the stage (type / arrow the widget)
//	Esc       return focus to the list
//	mouse     click / drag the stage widget directly (always live)
//	q, Ctrl+C quit
//
// It is itself composed from library widgets (VBox + HSplit + ListBox +
// Statusbar), so it dogfoods the set it showcases.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// DI seams so tests drive main()/run() without a real event loop.
var (
	runFunc    = run
	osExit     = os.Exit
	newAppFunc = tui.NewApp
	runAppFunc = func(a *tui.App) int { return a.Run() }
)

func main() { osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr)) }

// run parses --theme, composes the explorer, installs key bindings, and runs the
// App. Returns 0 on clean exit, 2 on flag-parse error, else App.Run's code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-widget-explorer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	st := newState()
	app := newAppFunc()
	st.app = app
	app.Root = st.root
	app.TickHz = 8 // animate the Spinner stage
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

// entry is a named widget factory for the gallery list.
type entry struct {
	name string
	make func() toolkit.Widget
}

// state bundles the explorer's mutable widgets.
type state struct {
	list         *tui.ListBox
	body         *tui.HSplit  // Left = list, Right = the live stage widget
	status       *toolkit.Label
	vbox         *tui.VBox    // the real layout
	root         *explorerRoot // wraps vbox + forwards ticks to the stage
	entries      []entry
	stageFocused bool
	app          *tui.App
}

func newState() *state {
	entries := widgetEntries()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	list := tui.NewListBox(names)
	stage := entries[0].make()
	body := &tui.HSplit{Left: list, Right: stage, LeftFrac: 26}
	status := toolkit.NewLabel("")
	header := toolkit.NewLabel(fmt.Sprintf(" tui widget explorer — %d widgets", len(entries)))
	vbox := &tui.VBox{Header: header, Body: body, Footer: status, HeaderH: 1, FooterH: 1}

	s := &state{
		list:    list,
		body:    body,
		status:  status,
		vbox:    vbox,
		entries: entries,
	}
	s.root = &explorerRoot{s: s}
	list.OnSelect = func(int) { s.showSelected() }
	s.syncStatus()
	return s
}

// showSelected swaps the stage to a fresh instance of the selected widget and
// returns focus to the list.
func (s *state) showSelected() {
	s.body.Right = s.entries[s.list.Selected].make()
	s.body.SetBounds(s.body.Bounds()) // re-lay-out so the new stage is bounded
	s.setStageFocus(false)
	s.syncStatus()
}

// setStageFocus routes keyboard input to the stage (true) or the list (false),
// and drives the stage widget's own focus cue when it supports one.
func (s *state) setStageFocus(focused bool) {
	s.stageFocused = focused
	switch w := s.body.Right.(type) {
	case tui.Focusable:
		w.SetFocused(focused)
	case *tui.TextEditor:
		w.Focused = focused
	}
	if s.app != nil {
		if focused {
			s.app.InputTarget = s.body.Right
		} else {
			s.app.InputTarget = nil
		}
	}
}

// syncStatus refreshes the footer with the current widget + focus + hints.
func (s *state) syncStatus() {
	focus := "list"
	if s.stageFocused {
		focus = "stage"
	}
	s.status.Text = fmt.Sprintf(" %s  ·  focus: %s  ·  ↑↓ select · Tab focus · Esc back · q quit",
		s.entries[s.list.Selected].name, focus)
}

// keys returns the App key bindings.
func (s *state) keys() map[string]func(*tui.App) {
	return map[string]func(*tui.App){
		"Ctrl+C": func(a *tui.App) { a.Quit() },
		"q": func(a *tui.App) {
			if !s.stageFocused { // else 'q' reaches a stage Entry
				a.Quit()
				a.Consume()
			}
		},
		"Up": func(a *tui.App) {
			if !s.stageFocused && s.list.Selected > 0 {
				s.list.Selected--
				s.showSelected()
				a.Consume()
			}
		},
		"Down": func(a *tui.App) {
			if !s.stageFocused && s.list.Selected < len(s.entries)-1 {
				s.list.Selected++
				s.showSelected()
				a.Consume()
			}
		},
		"Tab": func(a *tui.App) {
			if !s.stageFocused { // enter the stage; when focused, Tab flows to the widget
				s.setStageFocus(true)
				s.syncStatus()
				a.Consume()
			}
		},
		"Escape": func(a *tui.App) {
			if s.stageFocused {
				s.setStageFocus(false)
				s.syncStatus()
				a.Consume()
			}
		},
	}
}

// explorerRoot wraps the VBox layout and forwards ticks to the live stage so a
// Spinner animates (the VBox would otherwise route ticks to the list). All other
// events + Draw delegate to the VBox.
type explorerRoot struct {
	toolkit.Base
	s *state
}

func (e *explorerRoot) SetBounds(r toolkit.Rect) {
	e.Base.SetBounds(r)
	e.s.vbox.SetBounds(r)
}

func (e *explorerRoot) Draw(pnt painter.Painter, theme *toolkit.Theme) { e.s.vbox.Draw(pnt, theme) }

func (e *explorerRoot) OnEvent(ev toolkit.Event) {
	if ev.Kind == tui.EventTick {
		if st := e.s.body.Right; st != nil {
			st.OnEvent(ev)
		}
		return
	}
	e.s.vbox.OnEvent(ev)
}

// widgetEntries lists every cell-native widget the explorer showcases. It omits
// the two non-visual/self-positioning kinds — MenuDropdown (anchors itself,
// ignoring bounds) and FocusRing (a focus manager with no layout of its own);
// both are demonstrated in cmd/tui-editor / cmd/tui-explorer.
func widgetEntries() []entry {
	return []entry{
		{"Entry", func() toolkit.Widget { return &tui.Entry{Placeholder: "type here…"} }},
		{"Button", func() toolkit.Widget { return tui.NewButton("Click me", func() {}) }},
		{"CheckButton", func() toolkit.Widget { return tui.NewCheckButton("Enabled", true) }},
		{"RadioButton", func() toolkit.Widget {
			r := tui.NewRadioButton("Option A")
			r.Checked = true
			return r
		}},
		{"Scale", func() toolkit.Widget { return tui.NewScale(0, 100, 50) }},
		{"Dropdown", func() toolkit.Widget {
			return tui.NewDropdown([]string{"UTF-8", "Latin-1", "UTF-16"}, 0)
		}},
		{"ProgressBar", func() toolkit.Widget { return &tui.ProgressBar{Fraction: 0.6, Label: "60%"} }},
		{"LevelBar", func() toolkit.Widget {
			l := tui.NewLevelBar(5)
			l.Value = 3
			return l
		}},
		{"Spinner", func() toolkit.Widget { return tui.NewSpinner("Working…") }},
		{"ListBox", func() toolkit.Widget {
			return tui.NewListBox([]string{"apple", "banana", "cherry", "date", "elderberry", "fig"})
		}},
		{"TreeView", func() toolkit.Widget {
			root := &tui.TreeNode{Label: "/", Expanded: true, Children: []*tui.TreeNode{
				{Label: "src", Expanded: true, Children: []*tui.TreeNode{{Label: "main.go"}, {Label: "util.go"}}},
				{Label: "README.md"},
			}}
			tv := tui.NewTreeView(root)
			tv.Selected = root
			return tv
		}},
		{"Table", func() toolkit.Widget {
			cols := []tui.TableColumn{{Title: "Name", Width: 16}, {Title: "Kind"}}
			rows := [][]string{{"main.go", "source"}, {"README.md", "text"}, {"go.mod", "module"}}
			return tui.NewTable(cols, rows)
		}},
		{"MenuBar", func() toolkit.Widget {
			mb := &tui.MenuBar{}
			mb.Items = []tui.MenuItem{{Label: "File"}, {Label: "Edit"}, {Label: "View"}}
			return mb
		}},
		{"Toolbar", func() toolkit.Widget {
			return tui.NewToolbar([]tui.ToolbarItem{{Label: "New"}, {Separator: true}, {Label: "Save"}, {Label: "Del", Disabled: true}})
		}},
		{"Statusbar", func() toolkit.Widget { return tui.NewStatusbar([]string{"Ready", "Ln 1", "UTF-8"}) }},
		{"Popover", func() toolkit.Widget {
			return &tui.Popover{Title: "Popover", Body: []string{"a bordered", "overlay box"}, Visible: true}
		}},
		{"Dialog", func() toolkit.Widget {
			d := tui.NewDialog("Confirm", []string{"Save your changes?"}, "Yes", "No")
			d.Visible = true
			return d
		}},
		{"Notebook", func() toolkit.Widget {
			n := tui.NewNotebook()
			n.AddTab("One", toolkit.NewLabel("page one"))
			n.AddTab("Two", toolkit.NewLabel("page two"))
			return n
		}},
		{"HSplit", func() toolkit.Widget {
			return &tui.HSplit{Left: toolkit.NewLabel("left"), Right: toolkit.NewLabel("right"), LeftFrac: 40}
		}},
		{"VSplit", func() toolkit.Widget {
			return &tui.VSplit{Top: toolkit.NewLabel("top"), Bottom: toolkit.NewLabel("bottom"), TopFrac: 40}
		}},
		{"VBox", func() toolkit.Widget {
			return &tui.VBox{
				Header: toolkit.NewLabel("header"), Body: toolkit.NewLabel("body"),
				Footer: toolkit.NewLabel("footer"), HeaderH: 1, FooterH: 1,
			}
		}},
		{"TextEditor", func() toolkit.Widget {
			t := tui.NewTextEditor()
			t.SetText("edit me\nsecond line")
			return t
		}},
	}
}
