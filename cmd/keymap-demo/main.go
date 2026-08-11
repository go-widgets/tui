// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// keymap-demo is the reference interactive demo for the toolkit Action +
// Keymap layer driven from a terminal. A single toolkit.ActionRegistry and
// toolkit.Keymap are the one source of truth: the same Actions feed the menu
// shortcut hints, the command palette, and the keyboard bindings.
//
// It demonstrates, live in the terminal, the four behaviours the layer exists
// to provide:
//
//   - accelerators: Ctrl+S runs "Save" (a modifier chord parsed from the key
//     stream);
//   - multi-stroke chords: "g" then "d" runs "GoToDef" (the PENDING row shows
//     the half-typed chord between strokes);
//   - scopes: "a" runs "AllItems" globally, but with the widget scope active
//     (toggled with Tab) it runs "AllText" instead — the widget binding
//     shadows the global one;
//   - hot rebinding: "r" rebinds "GoToDef" to "z" at run time; the menu hint
//     updates and "z" then runs it.
//
// The screen is a fixed grid of labelled rows so its state is trivially
// assertable cell-by-cell:
//
//	row 0  keymap-demo (q quits)
//	row 2  MENU Save[Ctrl+S] Open[Ctrl+O] GoToDef[G D] Find[/]
//	row 4  SCOPE=global
//	row 6  PENDING=
//	row 8  LAST=none
//	row 10 MSG=
//	row 12 PALETTE ...
package main

import (
	"flag"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit / newAppFunc / runAppFunc are dependency-injection seams so
// tests drive main() through run() without spawning a subprocess or entering
// the interactive event loop.
var (
	runFunc    = run
	osExit     = os.Exit
	newAppFunc = tui.NewApp
	runAppFunc = defaultRunApp
)

// defaultRunApp is the production runAppFunc: hand off to the tui.App loop.
func defaultRunApp(a *tui.App) int { return a.Run() }

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags (--theme=light|dark), composes the demo, installs the quit
// bindings, and hands control to App.Run. Returns 0 on clean exit, 2 on a
// flag-parse error, and whatever App.Run returns otherwise.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("keymap-demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	s := newState()
	app := newAppFunc()
	app.Root = s
	if *theme == "dark" {
		app.Theme = toolkit.DefaultDark()
	} else {
		app.Theme = toolkit.DefaultLight()
	}
	// q / Ctrl+C quit. They Consume the event so the quit key never reaches
	// the keymap — a half-typed chord stays intact for inspection.
	quit := func(a *tui.App) { a.Consume(); a.Quit() }
	app.Keys["q"] = quit
	app.Keys["Ctrl+C"] = quit
	return runAppFunc(app)
}

// state is the demo's root widget: it owns the single ActionRegistry + Keymap,
// the display labels, and the current scope/selection, and it both draws the
// grid and routes every key through the keymap.
type state struct {
	toolkit.Base

	reg     *toolkit.ActionRegistry
	km      *toolkit.Keymap
	palette *toolkit.CommandPalette

	last        string // id of the last completed action
	msg         string // last action's status message
	scopeWidget bool   // whether the widget scope is active

	titleLbl   *tui.Label
	menuLbl    *tui.Label
	scopeLbl   *tui.Label
	pendingLbl *tui.Label
	lastLbl    *tui.Label
	msgLbl     *tui.Label
	paletteLbl *tui.Label

	menuIDs []string // actions whose shortcut hints the MENU row shows
}

// newState builds the registry, the keymap, and the labels. Every affordance
// is derived from the same actions.
func newState() *state {
	s := &state{
		reg:        toolkit.NewActionRegistry(),
		km:         toolkit.NewKeymap(),
		palette:    toolkit.NewCommandPalette(nil),
		titleLbl:   tui.NewLabel(""),
		menuLbl:    tui.NewLabel(""),
		scopeLbl:   tui.NewLabel(""),
		pendingLbl: tui.NewLabel(""),
		lastLbl:    tui.NewLabel(""),
		msgLbl:     tui.NewLabel(""),
		paletteLbl: tui.NewLabel(""),
		menuIDs:    []string{"save", "open", "goDefs", "find"},
	}

	save := s.reg.Add("save", "Save", func() { s.msg = "saved" })
	save.Shortcut = toolkit.MustParseChord("Ctrl+S")
	open := s.reg.Add("open", "Open", func() { s.msg = "opened" })
	open.Shortcut = toolkit.MustParseChord("Ctrl+O")
	find := s.reg.Add("find", "Find", func() { s.msg = "find" })
	find.Shortcut = toolkit.MustParseChord("/")
	goDefs := s.reg.Add("goDefs", "GoToDef", func() { s.msg = "goDefs" })
	goDefs.Shortcut = toolkit.MustParseChord("g d")
	pal := s.reg.Add("palette", "Palette", func() {
		s.palette.Open()
		s.palette.SetActions(s.reg)
	})
	pal.Shortcut = toolkit.MustParseChord("p")
	reb := s.reg.Add("rebind", "Rebind", func() {
		_ = s.km.Rebind("goDefs", toolkit.MustParseChord("z"), toolkit.ScopeGlobal)
		s.msg = "rebound goDefs to Z"
	})
	reb.Shortcut = toolkit.MustParseChord("r")
	// The scope pair shares one accelerator across two scopes; bound by hand
	// (no declared Shortcut) so BindDefaults leaves them out.
	s.reg.Add("allItems", "AllItems", func() { s.msg = "allItems" })
	s.reg.Add("allText", "AllText", func() { s.msg = "allText" })

	// The declared shortcuts are statically distinct, so BindDefaults cannot
	// conflict here; the demo binds them in one pass.
	_ = s.reg.BindDefaults(s.km, toolkit.ScopeGlobal)
	_ = s.km.Bind(toolkit.MustParseChord("a"), "allItems", toolkit.ScopeGlobal)
	_ = s.km.Bind(toolkit.MustParseChord("a"), "allText", toolkit.ScopeWidget)
	return s
}

// menuText renders the MENU row: each menu action's label with its LIVE
// shortcut hint read from the keymap, so a rebind is reflected immediately.
func (s *state) menuText() string {
	var b strings.Builder
	b.WriteString("MENU")
	for _, id := range s.menuIDs {
		a := s.reg.Action(id)
		mi := a.MenuItem(s.km)
		b.WriteString(" ")
		b.WriteString(a.Label)
		b.WriteString("[")
		b.WriteString(mi.Shortcut)
		b.WriteString("]")
	}
	return b.String()
}

// Draw paints the fixed grid of state rows.
func (s *state) Draw(p painter.Painter, theme *toolkit.Theme) {
	b := s.Bounds()
	row := func(lbl *tui.Label, text string, r int) {
		lbl.Text = text
		lbl.SetBounds(toolkit.Rect{X: b.X, Y: b.Y + r, W: b.W, H: 1})
		lbl.Draw(p, theme)
	}
	row(s.titleLbl, "keymap-demo (q quits)", 0)
	row(s.menuLbl, s.menuText(), 2)
	scope := "global"
	if s.scopeWidget {
		scope = "widget"
	}
	row(s.scopeLbl, "SCOPE="+scope, 4)
	row(s.pendingLbl, "PENDING="+s.km.Pending().String(), 6)
	last := s.last
	if last == "" {
		last = "none"
	}
	row(s.lastLbl, "LAST="+last, 8)
	row(s.msgLbl, "MSG="+s.msg, 10)
	pal := ""
	if s.palette.Visible {
		pal = "PALETTE n=" + strconv.Itoa(len(s.palette.Commands))
	}
	row(s.paletteLbl, pal, 12)
}

// OnEvent routes every key through the keymap: Tab toggles the widget scope,
// any other key is parsed into an accelerator and fed to Keymap.Feed with the
// currently-active scopes; a completed binding runs its action and records it
// as LAST.
func (s *state) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventKeyDown && ev.Kind != toolkit.EventChar {
		return
	}
	if ev.Code == "Tab" {
		s.scopeWidget = !s.scopeWidget
		return
	}
	acc, err := toolkit.ParseAccelerator(ev.Code)
	if err != nil {
		return
	}
	clean := toolkit.Event{
		Kind:  toolkit.EventKeyDown,
		Code:  acc.Key,
		Ctrl:  acc.Ctrl,
		Shift: acc.Shift,
		Alt:   acc.Alt,
		Meta:  acc.Meta,
	}
	mask := toolkit.ActiveScopes()
	if s.scopeWidget {
		mask = toolkit.ActiveScopes(toolkit.ScopeWidget)
	}
	id, st := s.km.Feed(clean, mask)
	if st == toolkit.Complete {
		s.last = id
		s.reg.Run(id)
	}
}
