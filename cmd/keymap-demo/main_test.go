// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// char / key / ctrl build the toolkit events the tui InputParser would deliver
// for a printable rune, a named key, and a Ctrl combination (modifiers packed
// into Code, matching csi.go).
func char(s string) toolkit.Event { return toolkit.Event{Kind: toolkit.EventChar, Code: s} }
func key(code string) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code}
}

func TestNewStateWiresActionsAndBindings(t *testing.T) {
	s := newState()
	if s.reg.Len() != 8 {
		t.Fatalf("registered %d actions, want 8", s.reg.Len())
	}
	// Declared shortcuts are bound at global scope.
	if ch, ok := s.km.ShortcutFor("save"); !ok || ch.String() != "Ctrl+S" {
		t.Fatalf("save shortcut = %v ok=%v", ch, ok)
	}
	if ch, _ := s.km.ShortcutFor("goDefs"); ch.String() != "G D" {
		t.Fatalf("goDefs shortcut = %v, want G D", ch)
	}
	// The scope pair shares one accelerator across two scopes.
	if a, _ := s.km.Conflict(toolkit.MustParseChord("a"), toolkit.ScopeGlobal); a != "allItems" {
		t.Fatalf("global a -> %q, want allItems", a)
	}
	if a, _ := s.km.Conflict(toolkit.MustParseChord("a"), toolkit.ScopeWidget); a != "allText" {
		t.Fatalf("widget a -> %q, want allText", a)
	}
}

func TestMenuTextShowsLiveShortcuts(t *testing.T) {
	s := newState()
	got := s.menuText()
	for _, want := range []string{"MENU", "Save[Ctrl+S]", "Open[Ctrl+O]", "GoToDef[G D]", "Find[/]"} {
		if !strings.Contains(got, want) {
			t.Errorf("menuText()=%q missing %q", got, want)
		}
	}
}

func TestOnEventIgnoresNonKeyAndUnparsable(t *testing.T) {
	s := newState()
	s.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	s.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: ""}) // ParseAccelerator error
	if s.last != "" {
		t.Fatalf("non-key / unparsable events set last=%q", s.last)
	}
}

func TestOnEventAcceleratorRunsSave(t *testing.T) {
	s := newState()
	s.OnEvent(key("Ctrl+S"))
	if s.last != "save" || s.msg != "saved" {
		t.Fatalf("Ctrl+S -> last=%q msg=%q", s.last, s.msg)
	}
}

func TestOnEventOpenAndFind(t *testing.T) {
	s := newState()
	s.OnEvent(key("Ctrl+O"))
	if s.last != "open" || s.msg != "opened" {
		t.Fatalf("Ctrl+O -> last=%q msg=%q", s.last, s.msg)
	}
	s.OnEvent(char("/"))
	if s.last != "find" || s.msg != "find" {
		t.Fatalf("/ -> last=%q msg=%q", s.last, s.msg)
	}
}

func TestOnEventChordRunsGoDefs(t *testing.T) {
	s := newState()
	s.OnEvent(char("g"))
	if s.last != "" {
		t.Fatalf("after g, last=%q, want empty (partial)", s.last)
	}
	if p := s.km.Pending().String(); p != "G" {
		t.Fatalf("pending after g = %q, want G", p)
	}
	s.OnEvent(char("d"))
	if s.last != "goDefs" {
		t.Fatalf("g d -> last=%q, want goDefs", s.last)
	}
}

func TestOnEventScopeResolution(t *testing.T) {
	s := newState()
	// Global scope by default.
	s.OnEvent(char("a"))
	if s.last != "allItems" {
		t.Fatalf("a (global) -> %q, want allItems", s.last)
	}
	// Tab activates the widget scope, which shadows the global binding.
	s.OnEvent(key("Tab"))
	if !s.scopeWidget {
		t.Fatal("Tab did not activate the widget scope")
	}
	s.OnEvent(char("a"))
	if s.last != "allText" {
		t.Fatalf("a (widget) -> %q, want allText", s.last)
	}
}

func TestOnEventHotRebind(t *testing.T) {
	s := newState()
	s.OnEvent(char("r")) // rebinds goDefs to z
	if !strings.Contains(s.menuText(), "GoToDef[Z]") {
		t.Fatalf("menu did not reflect the rebind: %q", s.menuText())
	}
	s.OnEvent(char("z"))
	if s.last != "goDefs" {
		t.Fatalf("z after rebind -> %q, want goDefs", s.last)
	}
}

func TestOnEventPaletteFedFromRegistry(t *testing.T) {
	s := newState()
	s.OnEvent(char("p"))
	if !s.palette.Visible().Get() {
		t.Fatal("p did not open the palette")
	}
	if len(s.palette.Commands) != 8 {
		t.Fatalf("palette has %d commands, want 8 (all visible actions)", len(s.palette.Commands))
	}
}

func TestDrawRendersGridBothPaletteStates(t *testing.T) {
	s := newState()
	s.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 60, H: 20})

	frame := func() *tui.TermGrid {
		cp := painter.NewCellPainter(60, 20)
		s.Draw(cp, toolkit.DefaultLight())
		var buf bytes.Buffer
		_, _ = cp.WriteANSI(&buf)
		return tui.DecodeANSI(buf.Bytes(), 60, 20)
	}

	g := frame()
	if !strings.Contains(g.RowText(0), "keymap-demo") {
		t.Errorf("row 0 = %q", g.RowText(0))
	}
	if !strings.Contains(g.RowText(4), "SCOPE=global") {
		t.Errorf("row 4 = %q", g.RowText(4))
	}
	if !strings.Contains(g.RowText(8), "LAST=none") {
		t.Errorf("row 8 = %q", g.RowText(8))
	}

	// Open the palette and re-render: the palette row is now populated.
	s.OnEvent(char("p"))
	g = frame()
	if !strings.Contains(g.RowText(12), "PALETTE n=8") {
		t.Errorf("row 12 (palette open) = %q", g.RowText(12))
	}

	// With the widget scope active the SCOPE row flips.
	s.scopeWidget = true
	g = frame()
	if !strings.Contains(g.RowText(4), "SCOPE=widget") {
		t.Errorf("row 4 (widget scope) = %q", g.RowText(4))
	}
}

// --- run / main / seam coverage (mirrors the tui-explorer harness) ---

func TestRunDefaultThemeInstallsRootAndKeys(t *testing.T) {
	origNew, origRun := newAppFunc, runAppFunc
	defer func() { newAppFunc, runAppFunc = origNew, origRun }()

	var captured *tui.App
	newAppFunc = func() *tui.App { captured = tui.NewApp(); return captured }
	runAppFunc = func(*tui.App) int { return 0 }

	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 0 {
		t.Fatalf("run(nil) = %d, want 0", code)
	}
	if captured.Root == nil {
		t.Fatal("no root installed")
	}
	if _, ok := captured.Keys["q"]; !ok {
		t.Fatal("q not bound")
	}
	if captured.Theme.Background != toolkit.DefaultLight().Background {
		t.Error("default theme is not light")
	}
}

func TestRunThemeDark(t *testing.T) {
	origNew, origRun := newAppFunc, runAppFunc
	defer func() { newAppFunc, runAppFunc = origNew, origRun }()
	var captured *tui.App
	newAppFunc = func() *tui.App { captured = tui.NewApp(); return captured }
	runAppFunc = func(*tui.App) int { return 0 }
	var out, errb bytes.Buffer
	if code := run([]string{"--theme=dark"}, &out, &errb); code != 0 {
		t.Fatalf("run(--theme=dark) = %d", code)
	}
	if captured.Theme.Background != toolkit.DefaultDark().Background {
		t.Error("--theme=dark did not apply dark")
	}
}

func TestRunQuitKeysConsumeAndQuit(t *testing.T) {
	origNew, origRun := newAppFunc, runAppFunc
	defer func() { newAppFunc, runAppFunc = origNew, origRun }()
	var captured *tui.App
	newAppFunc = func() *tui.App { captured = tui.NewApp(); return captured }
	runAppFunc = func(*tui.App) int { return 0 }
	var out, errb bytes.Buffer
	_ = run(nil, &out, &errb)

	captured.Keys["q"](captured)
	if !captured.IsQuitting() {
		t.Fatal("q handler did not quit")
	}
	// Ctrl+C uses the same handler.
	captured.Keys["Ctrl+C"](captured)
}

func TestRunPropagatesExitCode(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	runAppFunc = func(*tui.App) int { return 5 }
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 5 {
		t.Fatalf("run() = %d, want 5", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--nope"}, &out, &errb); code != 2 {
		t.Fatalf("run(--nope) = %d, want 2", code)
	}
}

func TestDefaultRunAppSeam(t *testing.T) {
	a := tui.NewApp()
	a.SetOpenTTYFn(func(*os.File) (tui.TTY, error) { return nil, errors.New("no tty") })
	if code := defaultRunApp(a); code == 0 {
		t.Fatal("defaultRunApp with a TTY error returned 0")
	}
}

func TestMainSuccessAndError(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()

	got := -1
	osExit = func(code int) { got = code }
	runFunc = func([]string, io.Writer, io.Writer) int { return 0 }
	main()
	if got != 0 {
		t.Fatalf("main success -> osExit(%d), want 0", got)
	}
	runFunc = func([]string, io.Writer, io.Writer) int { return 7 }
	main()
	if got != 7 {
		t.Fatalf("main error -> osExit(%d), want 7", got)
	}
}
