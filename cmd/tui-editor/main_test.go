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

// TestNewStateFields asserts every widget slot on state is populated
// so handlers don't nil-deref.
func TestNewStateFields(t *testing.T) {
	s := newState()
	for name, ok := range map[string]bool{
		"tv":        s.tv != nil,
		"statusbar": s.statusbar != nil,
		"menuBar":   s.menuBar != nil,
		"palette":   s.palette != nil,
		"paletteEn": s.paletteEn != nil,
		"root":      s.root != nil,
		"readFile":  s.readFile != nil,
		"writeFile": s.writeFile != nil,
	} {
		if !ok {
			t.Errorf("state.%s is nil / zero", name)
		}
	}
	if s.mode != modeView {
		t.Errorf("initial mode = %v, want modeView", s.mode)
	}
	if !s.tv.Focused {
		t.Error("TextView should start Focused so key events reach it in edit mode")
	}
}

// TestModeName covers every branch of the modeName switch, including
// the default arm for an unknown value.
func TestModeName(t *testing.T) {
	for _, tc := range []struct {
		m    mode
		want string
	}{
		{modeView, "VIEW"},
		{modeEdit, "EDIT"},
		{modePalette, "PALETTE"},
		{mode(999), "?"},
	} {
		if got := modeName(tc.m); got != tc.want {
			t.Errorf("modeName(%v) = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// TestSetModeUpdatesStateAndVisibility exercises setMode's palette-
// visibility toggle and the statusbar refresh.
func TestSetModeUpdatesStateAndVisibility(t *testing.T) {
	s := newState()
	s.setMode(modeEdit)
	if s.mode != modeEdit {
		t.Fatalf("mode = %v, want modeEdit", s.mode)
	}
	if s.palette.Visible {
		t.Fatal("palette should stay hidden on entering edit mode")
	}
	s.setMode(modePalette)
	if !s.palette.Visible {
		t.Fatal("palette should be visible in palette mode")
	}
	s.setMode(modeView)
	if s.palette.Visible {
		t.Fatal("palette should hide again on returning to view mode")
	}
}

// TestRefreshStatusFormatsSegments covers every branch of
// refreshStatus: named vs unnamed file, dirty vs clean, and the
// cursor-position formatter. Reads the flat Label.Text string that
// replaced the Statusbar's Segments slice in the v0.3.2 layout fix.
func TestRefreshStatusFormatsSegments(t *testing.T) {
	s := newState()
	s.refreshStatus()
	if !strings.Contains(s.statusbar.Text, "VIEW") {
		t.Errorf("status text missing VIEW: %q", s.statusbar.Text)
	}
	if !strings.Contains(s.statusbar.Text, "*scratch*") {
		t.Errorf("status text missing *scratch*: %q", s.statusbar.Text)
	}

	s.file = "notes.md"
	s.dirty = true
	s.tv.CursorLine, s.tv.CursorCol = 4, 6
	s.refreshStatus()
	if !strings.Contains(s.statusbar.Text, "notes.md [+]") {
		t.Errorf("status text missing notes.md [+]: %q", s.statusbar.Text)
	}
	if !strings.Contains(s.statusbar.Text, "5:7") {
		t.Errorf("status text missing 5:7 (1-indexed cursor): %q", s.statusbar.Text)
	}
}

// TestSaveUnnamedBufferIsNoop covers the "file == \"\"" early-return.
func TestSaveUnnamedBufferIsNoop(t *testing.T) {
	s := newState()
	called := false
	s.writeFile = func(string, []byte, os.FileMode) error {
		called = true
		return nil
	}
	if err := s.save(); err != nil {
		t.Fatalf("save on unnamed buffer errored: %v", err)
	}
	if called {
		t.Fatal("writeFile was invoked for an unnamed buffer")
	}
}

// TestSaveNamedBufferWrites verifies the successful save path clears
// the dirty flag AND writes the buffer bytes.
func TestSaveNamedBufferWrites(t *testing.T) {
	s := newState()
	s.file = "out.txt"
	s.dirty = true
	s.tv.SetText("hello\nworld")
	var got []byte
	s.writeFile = func(path string, data []byte, mode os.FileMode) error {
		if path != "out.txt" {
			t.Errorf("writeFile path = %q, want out.txt", path)
		}
		if mode != 0o644 {
			t.Errorf("writeFile mode = %o, want 0o644", mode)
		}
		got = data
		return nil
	}
	if err := s.save(); err != nil {
		t.Fatalf("save errored: %v", err)
	}
	if s.dirty {
		t.Error("dirty flag not cleared on successful save")
	}
	if string(got) != "hello\nworld" {
		t.Errorf("writeFile data = %q, want hello\\nworld", string(got))
	}
}

// TestSaveNamedBufferError propagates the underlying writeFile
// error back to the caller and leaves dirty set.
func TestSaveNamedBufferError(t *testing.T) {
	s := newState()
	s.file = "readonly.txt"
	s.dirty = true
	s.writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("permission denied")
	}
	if err := s.save(); err == nil {
		t.Fatal("save on read-only file returned nil")
	}
	if !s.dirty {
		t.Error("dirty flag should stay set on failed save")
	}
}

// TestRunCommandFind runs the "find <text>" palette command and asserts the
// caret jumps to the match.
func TestRunCommandFind(t *testing.T) {
	s := newState()
	s.tv.SetText("one two\nthree four")
	if err := s.runCommand(tui.NewApp(), "find four"); err != nil {
		t.Fatalf("runCommand(find) errored: %v", err)
	}
	if s.tv.CursorLine != 1 || s.tv.CursorCol != 6 {
		t.Errorf("find four moved caret to (%d,%d), want (1,6)", s.tv.CursorLine, s.tv.CursorCol)
	}
}

// TestRunCommandSave runs the "save" command and asserts writeFile
// was called.
func TestRunCommandSave(t *testing.T) {
	s := newState()
	s.file = "cmd.txt"
	called := false
	s.writeFile = func(string, []byte, os.FileMode) error {
		called = true
		return nil
	}
	if err := s.runCommand(tui.NewApp(), "save"); err != nil {
		t.Fatalf("runCommand(save) errored: %v", err)
	}
	if !called {
		t.Fatal("runCommand(save) did not invoke writeFile")
	}
}

// TestRunCommandSaveError propagates the save error via the palette
// command.
func TestRunCommandSaveError(t *testing.T) {
	s := newState()
	s.file = "bad.txt"
	s.writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("disk full")
	}
	if err := s.runCommand(tui.NewApp(), "save"); err == nil {
		t.Fatal("runCommand(save) should have propagated the error")
	}
}

// TestRunCommandQuit + QuitAlias cover both the "quit" and "q"
// palette entries.
func TestRunCommandQuit(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	_ = s.runCommand(a, "quit")
	if !a.IsQuitting() {
		t.Fatal("runCommand(quit) did not trigger Quit")
	}
}

func TestRunCommandQAlias(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	_ = s.runCommand(a, "q")
	if !a.IsQuitting() {
		t.Fatal("runCommand(q) did not trigger Quit")
	}
}

// TestRunCommandUnknownIsNoop covers the default arm of the switch.
func TestRunCommandUnknownIsNoop(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	if err := s.runCommand(a, "foo-bar"); err != nil {
		t.Fatalf("runCommand(unknown) errored: %v", err)
	}
	if a.IsQuitting() {
		t.Fatal("runCommand(unknown) should not have triggered Quit")
	}
}

// TestKeyBindingsPresent verifies every documented key has a
// registered handler.
func TestKeyBindingsPresent(t *testing.T) {
	s := newState()
	m := s.keys()
	for _, k := range []string{"Ctrl+C", "q", "i", "Escape", "Ctrl+S", "Ctrl+P", "Enter", "Ctrl+Z", "Ctrl+Y"} {
		if _, ok := m[k]; !ok {
			t.Errorf("keys()[%q] missing", k)
		}
	}
}

// TestCtrlCAlwaysQuits covers the mode-agnostic Ctrl+C hatch.
func TestCtrlCAlwaysQuits(t *testing.T) {
	for _, m := range []mode{modeView, modeEdit, modePalette} {
		s := newState()
		s.setMode(m)
		a := tui.NewApp()
		s.keys()["Ctrl+C"](a)
		if !a.IsQuitting() {
			t.Errorf("Ctrl+C in mode %v did not Quit", m)
		}
	}
}

// TestQBindingViewOnly checks the mode-gated q handler quits only
// from view mode.
func TestQBindingViewOnly(t *testing.T) {
	// View mode → Quit.
	s := newState()
	a := tui.NewApp()
	s.keys()["q"](a)
	if !a.IsQuitting() {
		t.Fatal("q in view mode did not Quit")
	}

	// Edit mode → no-op (does not Quit).
	s2 := newState()
	s2.setMode(modeEdit)
	a2 := tui.NewApp()
	s2.keys()["q"](a2)
	if a2.IsQuitting() {
		t.Fatal("q in edit mode wrongly triggered Quit")
	}
}

// TestIBindingEntersEditFromView + non-view branch.
func TestIBindingEntersEditFromView(t *testing.T) {
	s := newState()
	s.keys()["i"](tui.NewApp())
	if s.mode != modeEdit {
		t.Fatalf("i in view mode: mode = %v, want modeEdit", s.mode)
	}
}

func TestIBindingIgnoredInEditMode(t *testing.T) {
	s := newState()
	s.setMode(modeEdit)
	s.keys()["i"](tui.NewApp())
	if s.mode != modeEdit {
		t.Fatalf("i in edit mode changed mode to %v", s.mode)
	}
}

// TestEscapeReturnsToViewFromEditAndPalette.
func TestEscapeReturnsToViewFromEdit(t *testing.T) {
	s := newState()
	s.setMode(modeEdit)
	s.keys()["Escape"](tui.NewApp())
	if s.mode != modeView {
		t.Fatalf("Escape from edit: mode = %v, want modeView", s.mode)
	}
}

func TestEscapeReturnsToViewFromPalette(t *testing.T) {
	s := newState()
	s.setMode(modePalette)
	s.keys()["Escape"](tui.NewApp())
	if s.mode != modeView {
		t.Fatalf("Escape from palette: mode = %v, want modeView", s.mode)
	}
}

func TestEscapeInViewIsNoop(t *testing.T) {
	s := newState()
	s.keys()["Escape"](tui.NewApp())
	if s.mode != modeView {
		t.Fatalf("Escape in view: mode = %v, want modeView", s.mode)
	}
}

// TestCtrlSSavesInAnyMode covers the always-save shortcut.
func TestCtrlSSavesInAnyMode(t *testing.T) {
	s := newState()
	s.file = "any.txt"
	s.dirty = true
	called := false
	s.writeFile = func(string, []byte, os.FileMode) error {
		called = true
		return nil
	}
	s.keys()["Ctrl+S"](tui.NewApp())
	if !called {
		t.Fatal("Ctrl+S did not invoke save")
	}
}

func TestCtrlSSaveErrorIsSilentlyIgnored(t *testing.T) {
	// Ctrl+S handler swallows the error (surfaced later in the palette
	// or on retry). Verify no panic.
	s := newState()
	s.file = "any.txt"
	s.writeFile = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}
	s.keys()["Ctrl+S"](tui.NewApp())
	// no assertion beyond "did not panic"
}

// TestCtrlPTogglesPaletteFromViewAndEdit.
func TestCtrlPFromView(t *testing.T) {
	s := newState()
	s.keys()["Ctrl+P"](tui.NewApp())
	if s.mode != modePalette {
		t.Fatalf("Ctrl+P from view: mode = %v, want modePalette", s.mode)
	}
	if !s.palette.Visible {
		t.Fatal("Ctrl+P did not make the palette visible")
	}
}

func TestCtrlPInPaletteIsNoop(t *testing.T) {
	s := newState()
	s.setMode(modePalette)
	s.keys()["Ctrl+P"](tui.NewApp())
	if s.mode != modePalette {
		t.Fatalf("Ctrl+P inside palette flipped mode to %v", s.mode)
	}
}

// TestCtrlZUndoesLastCharAndRefreshesStatus covers the Ctrl+Z key
// handler: it must roll back the last char AND update the status bar
// so the caret-position segment tracks. The App must be Consume()d
// so Root.OnEvent doesn't run a second undo.
func TestCtrlZUndoesLastCharAndRefreshesStatus(t *testing.T) {
	s := newState()
	// Type "hi" so undo has something to roll back. Insert via
	// tv.OnEvent (matches the App's Root.OnEvent path).
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "h"})
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "i"})
	s.refreshStatus() // baseline: cursor at 1:3
	if s.tv.CursorCol != 2 {
		t.Fatalf("setup: CursorCol = %d, want 2", s.tv.CursorCol)
	}
	if !strings.Contains(s.statusbar.Text, "1:3") {
		t.Fatalf("setup status = %q, want to contain 1:3", s.statusbar.Text)
	}
	// The App's event-loop consumption path (Consume prevents
	// Root.OnEvent from double-firing) is exercised by the
	// integration test; here the handler is called directly.
	s.keys()["Ctrl+Z"](tui.NewApp())
	if s.tv.CursorCol != 1 {
		t.Errorf("Ctrl+Z: CursorCol = %d, want 1", s.tv.CursorCol)
	}
	if !strings.Contains(s.statusbar.Text, "1:2") {
		t.Errorf("Ctrl+Z status = %q, want to contain 1:2", s.statusbar.Text)
	}
}

// TestCtrlYRedoesAndRefreshesStatus mirrors the undo test for redo.
func TestCtrlYRedoesAndRefreshesStatus(t *testing.T) {
	s := newState()
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "x"})
	// Undo once to seed the redo stack.
	s.undo()
	if s.tv.CursorCol != 0 {
		t.Fatalf("setup undo: CursorCol = %d, want 0", s.tv.CursorCol)
	}
	s.keys()["Ctrl+Y"](tui.NewApp())
	if s.tv.CursorCol != 1 {
		t.Errorf("Ctrl+Y: CursorCol = %d, want 1", s.tv.CursorCol)
	}
	if !strings.Contains(s.statusbar.Text, "1:2") {
		t.Errorf("Ctrl+Y status = %q, want to contain 1:2", s.statusbar.Text)
	}
}

// TestEnterInPaletteRunsCommand.
func TestEnterInPaletteRunsQuitCommand(t *testing.T) {
	s := newState()
	s.setMode(modePalette)
	s.paletteEn.Text = "quit"
	a := tui.NewApp()
	s.keys()["Enter"](a)
	if !a.IsQuitting() {
		t.Fatal("Enter in palette with 'quit' did not Quit")
	}
	if s.mode != modeView {
		t.Fatalf("Enter in palette: mode after = %v, want modeView", s.mode)
	}
	if s.paletteEn.Text != "" {
		t.Errorf("paletteEn.Text = %q, want cleared", s.paletteEn.Text)
	}
}

func TestEnterOutsidePaletteIsNoop(t *testing.T) {
	s := newState()
	s.setMode(modeEdit)
	s.keys()["Enter"](tui.NewApp())
	if s.mode != modeEdit {
		t.Fatalf("Enter in edit changed mode to %v", s.mode)
	}
}

// TestRunNoFileNoTheme drives run() through the flag-parse + compose
// path with a stubbed runAppFunc.
func TestRunNoFileNoTheme(t *testing.T) {
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
	if captured.Theme.Background != toolkit.DefaultLight().Background {
		t.Error("default theme is not light")
	}
	if len(captured.Keys) == 0 {
		t.Fatal("run did not install any Keys")
	}
}

// TestRunDarkTheme covers the --theme=dark branch.
func TestRunDarkTheme(t *testing.T) {
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
		t.Error("--theme=dark did not install dark theme")
	}
}

// TestRunLoadsFile covers the --file=path success branch by
// injecting a state whose readFile returns fixed bytes.
func TestRunLoadsFile(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()

	runAppFunc = func(*tui.App) int { return 0 }

	// The demo instantiates state via newState() inside run(), so we
	// can't inject a readFile stub from outside without swapping the
	// state factory. Instead, write a real temp file + point --file
	// at it. Tests the same code path with negligible overhead.
	tmp, err := os.CreateTemp(t.TempDir(), "editor-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString("loaded body"); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--file=" + tmp.Name()}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--file=%s) = %d, want 0. stderr: %s", tmp.Name(), code, stderr.String())
	}
}

// TestRunLoadsFileError covers the "readFile returned error" branch.
func TestRunLoadsFileError(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	runAppFunc = func(*tui.App) int { return 0 }

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--file=/does/not/exist/anywhere"}, &stdout, &stderr); code != 4 {
		t.Fatalf("run with missing --file = %d, want 4", code)
	}
	if !strings.Contains(stderr.String(), "read") {
		t.Fatalf("stderr missing 'read' hint: %q", stderr.String())
	}
}

// TestRunBadFlag returns 2.
func TestRunBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(--nope) = %d, want 2", code)
	}
}

// TestRunPropagatesExitCode confirms runAppFunc's return value flows
// back through run().
func TestRunPropagatesExitCode(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()
	runAppFunc = func(*tui.App) int { return 9 }

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 9 {
		t.Fatalf("run() = %d, want 9 (from stubbed runAppFunc)", code)
	}
}

// TestDefaultRunAppInvokesRun exercises defaultRunApp against a
// tui.App with an openTTY-error seam so Run() bails out fast.
func TestDefaultRunAppInvokesRun(t *testing.T) {
	a := tui.NewApp()
	a.SetOpenTTYFn(func(*os.File) (tui.TTY, error) {
		return nil, errors.New("test: no tty")
	})
	if code := defaultRunApp(a); code == 0 {
		t.Fatal("defaultRunApp against an openTTY-error App returned 0; want non-zero")
	}
}

func TestNewStateWiresMenuItemsToDropdowns(t *testing.T) {
	s := newState()
	if len(s.menuBar.Items) != 4 {
		t.Fatalf("menu items = %d, want 4", len(s.menuBar.Items))
	}
	cases := []struct {
		idx int
		d   **tui.MenuDropdown
	}{
		{0, &s.fileDropdown},
		{1, &s.editDropdown},
		{2, &s.viewDropdown},
		{3, &s.helpDropdown},
	}
	// Open each dropdown in turn: AnchorX must be set to the item's
	// x-range, Visible must flip true, and clicking twice must close.
	for i, tc := range cases {
		s.menuBar.Items[tc.idx].OnClick()
		wantX, _ := s.menuBar.ItemXRange(tc.idx)
		if (*tc.d).AnchorX != wantX {
			t.Errorf("item %d dropdown AnchorX = %d, want %d",
				i, (*tc.d).AnchorX, wantX)
		}
		if !(*tc.d).Visible {
			t.Errorf("item %d OnClick did not open its dropdown", i)
		}
		s.menuBar.Items[tc.idx].OnClick()
		if (*tc.d).Visible {
			t.Errorf("item %d OnClick second call did not close", i)
		}
	}
	// Opening one dropdown must close any other that was open — the
	// closeOthers helper's mutual-exclusion behaviour.
	s.fileDropdown.Visible = true
	s.editDropdown.Visible = true
	s.menuBar.Items[2].OnClick() // open View
	if s.fileDropdown.Visible || s.editDropdown.Visible {
		t.Errorf("opening View did not close file/edit dropdowns")
	}
	if !s.viewDropdown.Visible {
		t.Errorf("View dropdown did not open")
	}
}

// TestNewStateViewToggleLineNumbers — flip closure body coverage.
func TestNewStateViewToggleLineNumbers(t *testing.T) {
	s := newState()
	before := s.tv.ShowGutter
	if len(s.viewDropdown.ItemActions) < 1 || s.viewDropdown.ItemActions[0] == nil {
		t.Fatal("viewDropdown row 0 has no ItemAction")
	}
	s.viewDropdown.ItemActions[0]()
	if s.tv.ShowGutter == before {
		t.Errorf("Toggle line numbers did not flip ShowGutter")
	}
	s.viewDropdown.ItemActions[0]()
	if s.tv.ShowGutter != before {
		t.Errorf("second toggle did not restore ShowGutter")
	}
}

// TestNewStateEditUndoRedoActionsRouteThroughTextEditor covers the
// Edit → Undo / Redo closures that call tv.OnEvent with Ctrl+Z/Y.
func TestNewStateEditUndoRedoActionsRouteThroughTextEditor(t *testing.T) {
	s := newState()
	// Prime an editable state — insert a char so the undo stack has
	// something to pop back. The TextEditor exposes no direct
	// Undo()/Redo() method (v0.10.x); the menu wiring dispatches via
	// EventKeyDown so keyboard + menu share the same code path.
	s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "x"})
	if s.tv.CursorCol != 1 {
		t.Fatalf("insert setup: CursorCol = %d, want 1", s.tv.CursorCol)
	}
	// Edit → Undo (row 0) must fire.
	s.editDropdown.ItemActions[0]()
	if s.tv.CursorCol != 0 {
		t.Errorf("Undo did not roll back cursor: col = %d, want 0", s.tv.CursorCol)
	}
	// Edit → Redo (row 1) must restore.
	s.editDropdown.ItemActions[1]()
	if s.tv.CursorCol != 1 {
		t.Errorf("Redo did not restore cursor: col = %d, want 1", s.tv.CursorCol)
	}
	// Rows 2..4 (Cut / Copy / Paste) are now wired to tv methods.
	// Just check each is non-nil; behaviour is exercised in
	// TestEditCutCopyPasteRoutesThroughTextEditor below.
	for i := 2; i < 5; i++ {
		if s.editDropdown.ItemActions[i] == nil {
			t.Errorf("edit row %d expected action, got nil (stub)", i)
		}
	}
}

// TestEditCutCopyPasteRoutesThroughTextEditor exercises the Cut /
// Copy / Paste menu wiring against a live TextEditor. Selection is
// primed via the mouse-drag path (Click sets anchor, MouseDrag
// activates) since upstream TextEditor keeps the anchor field
// private.
//
// tui.GutterWidth(1) = 3 (2 min digits + 1 sep). clickAt subtracts
// leftPad from the widget-local X, so char-col 0 lives at widget X
// = 3 exactly.
func TestEditCutCopyPasteRoutesThroughTextEditor(t *testing.T) {
	s := newState()
	s.tv.SetText("hello world")
	pad := tui.GutterWidth(len(s.tv.Lines))

	// primeSelection selects text-col 0..end via a click+drag path.
	primeSelection := func(endCol int) {
		s.tv.CursorLine, s.tv.CursorCol = 0, 0
		s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: pad, Y: 0})
		s.tv.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: pad + endCol, Y: 0})
	}

	// Copy — buffer unchanged, clipboard now holds "hello".
	primeSelection(5)
	s.editDropdown.ItemActions[3]()
	if got := s.tv.Text(); got != "hello world" {
		t.Errorf("Copy changed buffer: %q", got)
	}
	if got := s.tv.SelectedText(); got != "hello" {
		t.Errorf("Copy selection = %q, want %q", got, "hello")
	}

	// Cut — buffer loses "hello".
	primeSelection(5)
	s.editDropdown.ItemActions[2]()
	if got := s.tv.Text(); got != " world" {
		t.Errorf("Cut: buffer = %q, want %q", got, " world")
	}

	// Paste — puts the clipboard back at the caret. After Cut the
	// caret is at col 0 (start of "world"), clipboard holds "hello".
	s.tv.CursorLine, s.tv.CursorCol = 0, 0
	s.editDropdown.ItemActions[4]()
	if got := s.tv.Text(); got != "hello world" {
		t.Errorf("Paste: buffer = %q, want %q", got, "hello world")
	}
}

// TestNewStateFileSaveActionCallsSaveSeam wires File → Save row 2 to
// s.save(), which itself delegates to s.writeFile. Confirm the seam
// fires by injecting a recording writeFile.
func TestNewStateFileSaveActionCallsSaveSeam(t *testing.T) {
	s := newState()
	s.file = "/tmp/mock" // gives save() a target
	called := false
	s.writeFile = func(_ string, _ []byte, _ os.FileMode) error {
		called = true
		return nil
	}
	s.fileDropdown.ItemActions[2]()
	if !called {
		t.Error("File → Save row 2 did not call writeFile seam")
	}
	// Row 0 (New) resets to a fresh scratch buffer.
	s.tv.SetText("dirty content")
	s.file = "/tmp/mock"
	s.fileDropdown.ItemActions[0]()
	if s.tv.Text() != "" || s.file != "" {
		t.Errorf("File → New did not reset buffer: text=%q file=%q", s.tv.Text(), s.file)
	}
	// Row 1 (Open) drops into the palette prefilled with an ":e " command.
	s.fileDropdown.ItemActions[1]()
	if s.mode != modePalette || s.paletteEn.Text != "e " {
		t.Errorf("File → Open: mode=%v text=%q, want modePalette/\"e \"", s.mode, s.paletteEn.Text)
	}
}

// TestNewStateFileQuitActionCallsQuitSeam wires the late-bound quit
// closure; before run() sets it, calling it must be a no-op (guarded
// by `if s.quit != nil`).
func TestNewStateFileQuitActionCallsQuitSeam(t *testing.T) {
	s := newState()
	// Row 3 (Quit) with s.quit unset must not panic.
	s.fileDropdown.ItemActions[3]()
	called := 0
	s.quit = func() { called++ }
	s.fileDropdown.ItemActions[3]()
	if called != 1 {
		t.Errorf("Quit action did not fire s.quit: called = %d", called)
	}
}

// TestNewStateViewCommandPaletteActionSwitchesMode covers the
// View → Command palette closure calling setMode(modePalette).
func TestNewStateViewCommandPaletteActionSwitchesMode(t *testing.T) {
	s := newState()
	if s.mode != modeView {
		t.Fatalf("start mode = %v, want modeView", s.mode)
	}
	s.viewDropdown.ItemActions[2]()
	if s.mode != modePalette {
		t.Errorf("View → Command palette: mode = %v, want modePalette", s.mode)
	}
	if !s.palette.Visible {
		t.Error("palette not marked visible after menu action")
	}
}

// TestPaletteCaptureAndInputTarget covers the modal command palette: entering
// palette mode routes App.InputTarget to the capture, typing through the capture
// edits the entry and mirrors into the popover, and leaving clears the target.
func TestPaletteCaptureAndInputTarget(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	s.app = a

	s.setMode(modePalette)
	if a.InputTarget != s.capture {
		t.Fatal("palette mode did not route InputTarget to the capture")
	}
	s.capture.Draw(nil, nil) // input-only widget: Draw is a no-op
	s.capture.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "h"})
	s.capture.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "i"})
	if s.paletteEn.Text != "hi" {
		t.Errorf("capture did not edit the entry: %q", s.paletteEn.Text)
	}
	if len(s.palette.Body) != 1 || s.palette.Body[0] != "> hi" {
		t.Errorf("capture did not mirror into the popover: %v", s.palette.Body)
	}

	s.setMode(modeView)
	if a.InputTarget != nil {
		t.Error("leaving palette mode did not clear InputTarget")
	}
}

// TestRunCommandNewOpen covers the palette's "new", "e <path>" and "open <path>"
// commands plus openFile's empty-path and read-error branches.
func TestRunCommandNewOpen(t *testing.T) {
	s := newState()
	a := tui.NewApp()

	// "new" resets to a fresh scratch buffer.
	s.tv.SetText("stuff")
	s.file = "x"
	_ = s.runCommand(a, "new")
	if s.tv.Text() != "" || s.file != "" {
		t.Errorf("new: text=%q file=%q", s.tv.Text(), s.file)
	}

	// "e <path>" loads via the readFile seam.
	s.readFile = func(p string) ([]byte, error) {
		if p != "/doc.txt" {
			t.Errorf("readFile path = %q", p)
		}
		return []byte("loaded"), nil
	}
	if err := s.runCommand(a, "e /doc.txt"); err != nil {
		t.Fatalf("e: %v", err)
	}
	if s.tv.Text() != "loaded" || s.file != "/doc.txt" {
		t.Errorf("e: text=%q file=%q", s.tv.Text(), s.file)
	}

	// "open <path>" is an alias.
	s.readFile = func(string) ([]byte, error) { return []byte("again"), nil }
	_ = s.runCommand(a, "open /other")
	if s.tv.Text() != "again" {
		t.Errorf("open: %q", s.tv.Text())
	}

	// An all-whitespace path is a no-op (readFile must not be called).
	s.readFile = func(string) ([]byte, error) {
		t.Fatal("readFile called for an empty path")
		return nil, nil
	}
	if err := s.openFile("   "); err != nil {
		t.Errorf("empty openFile: %v", err)
	}

	// A read error leaves the buffer unchanged and is returned.
	s.tv.SetText("keep")
	s.readFile = func(string) ([]byte, error) { return nil, errors.New("nope") }
	if err := s.openFile("/bad"); err == nil {
		t.Error("openFile did not return the read error")
	}
	if s.tv.Text() != "keep" {
		t.Errorf("errored open changed the buffer: %q", s.tv.Text())
	}
}

// TestMainSuccessPath + TestMainErrorPath drive main via the
// runFunc/osExit seams.
func TestMainSuccessPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	gotCode := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 0 }
	osExit = func(code int) { gotCode = code }
	main()
	if gotCode != 0 {
		t.Fatalf("main() called osExit(%d), want 0", gotCode)
	}
}

func TestMainErrorPath(t *testing.T) {
	origRun, origExit := runFunc, osExit
	defer func() { runFunc, osExit = origRun, origExit }()
	gotCode := -1
	runFunc = func([]string, io.Writer, io.Writer) int { return 4 }
	osExit = func(code int) { gotCode = code }
	main()
	if gotCode != 4 {
		t.Fatalf("main() called osExit(%d), want 4", gotCode)
	}
}
