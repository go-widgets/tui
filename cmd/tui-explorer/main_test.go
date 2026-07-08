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

	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// The demo is now composed entirely from exported tui.* widgets (ListBox,
// MenuBar, MenuDropdown, Popover, HSplit, VBox), which carry their own 100%
// covered tests in the library. These tests cover only the demo's own logic:
// the state wiring, the key bindings, and the run/main seams.

// TestNewStateFields verifies every state slot is populated so key
// handlers never nil-deref at runtime.
func TestNewStateFields(t *testing.T) {
	s := newState()
	if s.fileList == nil {
		t.Error("state.fileList is nil")
	}
	if s.content == nil {
		t.Error("state.content is nil")
	}
	if s.status == nil {
		t.Error("state.status is nil")
	}
	if s.menuBar == nil {
		t.Error("state.menuBar is nil")
	}
	if s.helpPopover == nil {
		t.Error("state.helpPopover is nil")
	}
	if s.searchPopover == nil {
		t.Error("state.searchPopover is nil")
	}
	if s.root == nil {
		t.Error("state.root is nil")
	}
	if len(s.files) == 0 {
		t.Error("state.files is empty")
	}
	if len(s.paths) == 0 {
		t.Error("state.paths is empty")
	}
}

// TestNewStateWiresFileListOnSelectToSyncContent verifies the closure newState
// installs on the ListBox: bumping Selected then invoking OnSelect must refresh
// the content pane.
func TestNewStateWiresFileListOnSelectToSyncContent(t *testing.T) {
	s := newState()
	// Simulate what an arrow key or click does: change Selected, then fire the
	// callback.
	s.fileList.Selected = 2
	if s.fileList.OnSelect == nil {
		t.Fatal("newState left fileList.OnSelect nil")
	}
	before := s.content.Text()
	s.fileList.OnSelect(s.fileList.Selected)
	if s.content.Text() == before {
		t.Errorf("OnSelect closure did not refresh content: before=%q after=%q",
			before, s.content.Text())
	}
}

// TestKeysReturnsAllHandlers checks every expected key is registered.
func TestKeysReturnsAllHandlers(t *testing.T) {
	s := newState()
	m := s.keys()
	for _, k := range []string{"q", "Ctrl+C", "?", "/", "Up", "Down", "Enter"} {
		if _, ok := m[k]; !ok {
			t.Errorf("keys()[%q] missing", k)
		}
	}
}

func TestKeyHandlersRunWithoutPanic(t *testing.T) {
	s := newState()
	for k, h := range s.keys() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("handler %q panicked: %v", k, r)
				}
			}()
			h(tui.NewApp())
		}()
	}
}

func TestQuitHandlerCallsQuit(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	s.keys()["q"](a)
	if !a.IsQuitting() {
		t.Fatal("q handler did not Quit")
	}
}
func TestCtrlCHandlerCallsQuit(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	s.keys()["Ctrl+C"](a)
	if !a.IsQuitting() {
		t.Fatal("Ctrl+C handler did not Quit")
	}
}

func TestHelpToggleFlipsVisible(t *testing.T) {
	s := newState()
	if s.helpPopover.Visible {
		t.Fatal("help popover should start hidden")
	}
	s.keys()["?"](tui.NewApp())
	if !s.helpPopover.Visible {
		t.Fatal("? did not show help")
	}
	s.keys()["?"](tui.NewApp())
	if s.helpPopover.Visible {
		t.Fatal("second ? did not hide help")
	}
}
func TestSearchToggleFlipsVisible(t *testing.T) {
	s := newState()
	s.keys()["/"](tui.NewApp())
	if !s.searchPopover.Visible {
		t.Fatal("/ did not show search")
	}
	s.keys()["/"](tui.NewApp())
	if s.searchPopover.Visible {
		t.Fatal("second / did not hide search")
	}
}

// TestUpDownMovesSelectionAndSyncsContent covers the arrow-key handlers + the
// syncContent side effect.
func TestUpDownMovesSelectionAndSyncsContent(t *testing.T) {
	s := newState()
	// Start at 0. Down → 1.
	s.keys()["Down"](tui.NewApp())
	if s.fileList.Selected != 1 {
		t.Fatalf("Down: Selected = %d, want 1", s.fileList.Selected)
	}
	wantContent := s.files[s.paths[1]]
	if got := s.content.Text(); got != strings.TrimRight(wantContent, "\n") && got != wantContent {
		t.Errorf("content after Down: %q, want %q", got, wantContent)
	}
	// Up → 0.
	s.keys()["Up"](tui.NewApp())
	if s.fileList.Selected != 0 {
		t.Fatalf("Up: Selected = %d, want 0", s.fileList.Selected)
	}
}

// TestUpAtTopIsNoop covers the "already at top" branch.
func TestUpAtTopIsNoop(t *testing.T) {
	s := newState()
	s.fileList.Selected = 0
	s.keys()["Up"](tui.NewApp())
	if s.fileList.Selected != 0 {
		t.Fatalf("Up at top moved to %d", s.fileList.Selected)
	}
}

// TestDownAtBottomIsNoop covers the "already at bottom" branch.
func TestDownAtBottomIsNoop(t *testing.T) {
	s := newState()
	s.fileList.Selected = len(s.fileList.Items) - 1
	last := s.fileList.Selected
	s.keys()["Down"](tui.NewApp())
	if s.fileList.Selected != last {
		t.Fatalf("Down at bottom moved to %d", s.fileList.Selected)
	}
}

// TestEnterSyncsContent covers the Enter handler.
func TestEnterSyncsContent(t *testing.T) {
	s := newState()
	s.fileList.Selected = 2 // "/docs/README.md"
	s.keys()["Enter"](tui.NewApp())
	if !strings.Contains(s.content.Text(), "Project") {
		t.Errorf("Enter did not sync content: %q", s.content.Text())
	}
}

// TestSyncContentOutOfRange covers the "Selected out of range" early-return
// branch.
func TestSyncContentOutOfRange(t *testing.T) {
	s := newState()
	s.fileList.Selected = -1
	s.syncContent()
	if !strings.Contains(s.content.Text(), "no selection") {
		t.Errorf("out-of-range selection didn't show (no selection): %q", s.content.Text())
	}
}
func TestSyncContentOutOfRangeHigh(t *testing.T) {
	s := newState()
	s.fileList.Selected = 999
	s.syncContent()
	if !strings.Contains(s.content.Text(), "no selection") {
		t.Errorf("high-out-of-range selection didn't show (no selection): %q", s.content.Text())
	}
}

// TestNewStateWiresMenuItemsToDropdowns verifies each menu item toggles + anchors
// its dropdown and that opening one closes the others.
func TestNewStateWiresMenuItemsToDropdowns(t *testing.T) {
	s := newState()
	// menuBar items: File / View / Help — in that order.
	if len(s.menuBar.Items) != 3 {
		t.Fatalf("menu items = %d, want 3", len(s.menuBar.Items))
	}
	cases := []struct {
		idx  int
		name string
		d    **tui.MenuDropdown
	}{
		{0, "File", &s.fileDropdown},
		{1, "View", &s.viewDropdown},
		{2, "Help", &s.helpDropdown},
	}
	for _, tc := range cases {
		if (*tc.d).Visible {
			t.Fatalf("%s dropdown starts visible", tc.name)
		}
		s.menuBar.Items[tc.idx].OnClick()
		wantX, _ := s.menuBar.ItemXRange(tc.idx)
		if (*tc.d).AnchorX != wantX {
			t.Errorf("%s AnchorX = %d, want %d", tc.name, (*tc.d).AnchorX, wantX)
		}
		if !(*tc.d).Visible {
			t.Errorf("%s.OnClick() did not open its dropdown", tc.name)
		}
		s.menuBar.Items[tc.idx].OnClick()
		if (*tc.d).Visible {
			t.Errorf("%s.OnClick() second call did not close its dropdown", tc.name)
		}
	}
	// Opening one closes the others (mutual exclusion).
	s.fileDropdown.Visible = true
	s.viewDropdown.Visible = true
	s.menuBar.Items[2].OnClick() // Help
	if s.fileDropdown.Visible || s.viewDropdown.Visible {
		t.Errorf("opening Help did not close file/view dropdowns")
	}
	if !s.helpDropdown.Visible {
		t.Errorf("Help dropdown did not open")
	}
}

// TestNewStateViewToggleLineNumbers covers the View dropdown's first
// ItemAction, which flips the preview's line-number gutter.
func TestNewStateViewToggleLineNumbers(t *testing.T) {
	s := newState()
	before := s.content.ShowGutter
	if len(s.viewDropdown.ItemActions) < 1 || s.viewDropdown.ItemActions[0] == nil {
		t.Fatal("viewDropdown row 0 has no ItemAction")
	}
	s.viewDropdown.ItemActions[0]()
	if s.content.ShowGutter == before {
		t.Errorf("Toggle line numbers did not flip ShowGutter (was %v)", before)
	}
	s.viewDropdown.ItemActions[0]()
	if s.content.ShowGutter != before {
		t.Errorf("second Toggle did not restore ShowGutter (was %v)", before)
	}
}

func TestRunDefaultThemeSucceeds(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	origNew := newAppFunc
	defer func() { newAppFunc = origNew }()

	var captured *tui.App
	newAppFunc = func() *tui.App {
		a := tui.NewApp()
		captured = a
		return a
	}
	runAppFunc = func(*tui.App) int { return 0 }

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run(nil) = %d, want 0. stderr:\n%s", code, stderr.String())
	}
	if captured.Theme == nil {
		t.Fatal("no theme installed")
	}
	if len(captured.Keys) == 0 {
		t.Fatal("no keys installed")
	}
	if captured.Theme.Background != toolkit.DefaultLight().Background {
		t.Error("default is not light")
	}
}
func TestRunThemeDarkApplied(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	origNew := newAppFunc
	defer func() { newAppFunc = origNew }()

	var captured *tui.App
	newAppFunc = func() *tui.App {
		a := tui.NewApp()
		captured = a
		return a
	}
	runAppFunc = func(*tui.App) int { return 0 }

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--theme=dark"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--theme=dark) = %d, want 0", code)
	}
	if captured.Theme.Background != toolkit.DefaultDark().Background {
		t.Error("--theme=dark did not apply dark")
	}
}
func TestRunPropagatesExitCode(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	runAppFunc = func(*tui.App) int { return 5 }
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 5 {
		t.Fatalf("run() = %d, want 5", code)
	}
}
func TestRunBadFlagFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--not-a-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(--not-a-flag) = %d, want 2", code)
	}
}
func TestDefaultRunAppSeamCallsRun(t *testing.T) {
	a := tui.NewApp()
	a.SetOpenTTYFn(func(*os.File) (tui.TTY, error) { return nil, errors.New("no tty") })
	if code := defaultRunApp(a); code == 0 {
		t.Fatal("defaultRunApp with openTTY error returned 0")
	}
}
func TestMainSuccessPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	got := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 0 }
	osExit = func(code int) { got = code }
	main()
	if got != 0 {
		t.Fatalf("main() called osExit(%d), want 0", got)
	}
}
func TestMainErrorPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	got := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 7 }
	osExit = func(code int) { got = code }
	main()
	if got != 7 {
		t.Fatalf("main() called osExit(%d), want 7", got)
	}
}
