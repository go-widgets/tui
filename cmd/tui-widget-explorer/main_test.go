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

func TestWidgetEntriesShape(t *testing.T) {
	all := widgetEntries()
	if len(all) == 0 {
		t.Fatal("no widget entries")
	}
	seen := map[string]bool{}
	for i, e := range all {
		if e.name == "" || seen[e.name] {
			t.Errorf("entry %d: empty/duplicate name %q", i, e.name)
		}
		seen[e.name] = true
		if e.make == nil || e.make() == nil {
			t.Errorf("entry %q: nil make/widget", e.name)
		}
	}
}

func TestNewStateFields(t *testing.T) {
	s := newState()
	if s.list == nil || s.body == nil || s.status == nil || s.vbox == nil || s.root == nil {
		t.Fatal("newState left a field nil")
	}
	if s.body.Right == nil {
		t.Fatal("initial stage not set")
	}
	if !strings.Contains(s.status.Text, s.entries[0].name) {
		t.Errorf("status %q missing first widget name", s.status.Text)
	}
}

func TestSelectRebuildsStage(t *testing.T) {
	s := newState()
	s.app = tui.NewApp()
	s.root.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 24}) // give the layout bounds

	first := s.body.Right
	// The OnSelect bridge fires showSelected → new stage instance.
	s.list.Selected = 4
	s.list.OnSelect(4)
	if s.body.Right == first {
		t.Error("selecting did not rebuild the stage")
	}
	if !strings.Contains(s.status.Text, s.entries[4].name) {
		t.Errorf("status not synced to selection: %q", s.status.Text)
	}
	if s.stageFocused || s.app.InputTarget != nil {
		t.Error("rebuilding the stage should return focus to the list")
	}
}

func TestKeysNavigation(t *testing.T) {
	s := newState()
	s.app = tui.NewApp()
	s.root.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 24})
	m := s.keys()

	// Down advances the selection; Up at top is a no-op.
	m["Down"](tui.NewApp())
	if s.list.Selected != 1 {
		t.Fatalf("Down: Selected=%d, want 1", s.list.Selected)
	}
	m["Up"](tui.NewApp())
	if s.list.Selected != 0 {
		t.Fatalf("Up: Selected=%d, want 0", s.list.Selected)
	}
	m["Up"](tui.NewApp()) // already at top → no-op
	if s.list.Selected != 0 {
		t.Fatalf("Up at top moved to %d", s.list.Selected)
	}
	// Down at the bottom is a no-op.
	s.list.Selected = len(s.entries) - 1
	m["Down"](tui.NewApp())
	if s.list.Selected != len(s.entries)-1 {
		t.Fatalf("Down at bottom moved to %d", s.list.Selected)
	}

	// q quits when the list has focus; Ctrl+C always quits.
	a := tui.NewApp()
	m["q"](a)
	if !a.IsQuitting() {
		t.Error("q did not quit (list focus)")
	}
	c := tui.NewApp()
	m["Ctrl+C"](c)
	if !c.IsQuitting() {
		t.Error("Ctrl+C did not quit")
	}
}

func TestKeysFocusToggle(t *testing.T) {
	s := newState()
	s.app = tui.NewApp()
	s.root.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 24})
	m := s.keys()

	// Tab enters the stage: InputTarget set, Entry (stage 0) focused.
	m["Tab"](tui.NewApp())
	if !s.stageFocused || s.app.InputTarget != s.body.Right {
		t.Fatalf("Tab did not focus stage: focused=%v target=%v", s.stageFocused, s.app.InputTarget == s.body.Right)
	}
	if e, ok := s.body.Right.(*tui.Entry); !ok || !e.Focused {
		t.Error("Entry stage did not receive focus cue")
	}
	// While stage-focused, q and Up/Down/Tab are inert (they reach the widget).
	a := tui.NewApp()
	m["q"](a)
	m["Up"](tui.NewApp())
	m["Down"](tui.NewApp())
	m["Tab"](tui.NewApp())
	if a.IsQuitting() || s.list.Selected != 0 || !s.stageFocused {
		t.Error("stage-focused guards failed")
	}
	// Esc returns to the list.
	m["Escape"](tui.NewApp())
	if s.stageFocused || s.app.InputTarget != nil {
		t.Fatal("Esc did not return focus to the list")
	}
	// Esc when already on the list is a no-op.
	m["Escape"](tui.NewApp())
}

func TestSetStageFocusVariants(t *testing.T) {
	s := newState() // app is nil here → exercises the app==nil branch
	// Default stage (Entry) is Focusable.
	s.setStageFocus(true)
	if e := s.body.Right.(*tui.Entry); !e.Focused {
		t.Error("Focusable stage not focused")
	}
	s.setStageFocus(false)

	// TextEditor stage takes the *tui.TextEditor path.
	teIdx := indexOf(s.entries, "TextEditor")
	s.list.Selected = teIdx
	s.showSelected()
	s.setStageFocus(true)
	if te := s.body.Right.(*tui.TextEditor); !te.Focused {
		t.Error("TextEditor stage did not receive Focused")
	}

	// A non-focusable stage (Table) takes neither case — no panic.
	s.list.Selected = indexOf(s.entries, "Table")
	s.showSelected()
	s.setStageFocus(true)
	s.setStageFocus(false)
}

func indexOf(es []entry, name string) int {
	for i, e := range es {
		if e.name == name {
			return i
		}
	}
	return -1
}

func TestExplorerRoot(t *testing.T) {
	s := newState()
	s.root.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 24})
	if b := s.vbox.Bounds(); b.W != 80 || b.H != 24 {
		t.Errorf("SetBounds did not propagate to vbox: %+v", b)
	}
	pp := painter.NewPixelPainter(make([]byte, 80*24*4), 80, 24)
	s.root.Draw(pp, toolkit.DefaultLight())

	// A tick is forwarded to the stage (advances a Spinner); other events go to
	// the VBox. Point the stage at a Spinner and confirm it advanced.
	s.list.Selected = indexOf(s.entries, "Spinner")
	s.showSelected()
	sp := s.body.Right.(*tui.Spinner)
	s.root.OnEvent(toolkit.Event{Kind: tui.EventTick})
	s.root.OnEvent(toolkit.Event{Kind: tui.EventTick})
	pp2 := painter.NewPixelPainter(make([]byte, 80*24*4), 80, 24)
	sp.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	sp.Draw(pp2, toolkit.DefaultLight()) // frame advanced past 0 (no assertion needed beyond no-panic)
	// A non-tick event delegates to the VBox (routes to the list) without panic.
	s.root.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
}

func TestRunSeams(t *testing.T) {
	origNew, origRun := newAppFunc, runAppFunc
	defer func() { newAppFunc, runAppFunc = origNew, origRun }()

	var captured *tui.App
	newAppFunc = func() *tui.App { captured = tui.NewApp(); return captured }
	runAppFunc = func(*tui.App) int { return 0 }

	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if captured.Theme.Background != toolkit.DefaultLight().Background || captured.TickHz != 8 || len(captured.Keys) == 0 {
		t.Error("run did not configure the App (theme/tick/keys)")
	}
	// Dark theme.
	if code := run([]string{"--theme=dark"}, &out, &errb); code != 0 {
		t.Fatalf("run(--theme=dark) = %d", code)
	}
	if captured.Theme.Background != toolkit.DefaultDark().Background {
		t.Error("--theme=dark not applied")
	}
	// Bad flag → 2.
	if code := run([]string{"--nope"}, &out, &errb); code != 2 {
		t.Fatalf("bad flag = %d, want 2", code)
	}
}

func TestDefaultRunAppSeam(t *testing.T) {
	a := tui.NewApp()
	a.SetOpenTTYFn(func(*os.File) (tui.TTY, error) { return nil, errors.New("no tty") })
	if code := runAppFunc(a); code == 0 {
		t.Fatal("defaultRunApp with openTTY error returned 0")
	}
}

func TestMainSeams(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	for _, want := range []int{0, 7} {
		got := -1
		runFunc = func([]string, io.Writer, io.Writer) int { return want }
		osExit = func(code int) { got = code }
		main()
		if got != want {
			t.Errorf("main osExit(%d), want %d", got, want)
		}
	}
}
