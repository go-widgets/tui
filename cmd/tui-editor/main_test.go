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
	for _, k := range []string{"Ctrl+C", "q", "i", "Escape", "Ctrl+S", "Ctrl+P", "Enter"} {
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

// TestPackedVBoxLayoutHeaderBodyFooter drives the local layout
// helper directly: given a 80×30 bounds, header must land at y=0
// with H=1, footer at y=29 with H=1, body between. Catches the
// regression that shipped in v0.3.0 / v0.3.1 where a plain
// toolkit.VBox distributed each child equally, wrecking the
// interactive demo's chrome.
func TestPackedVBoxLayoutHeaderBodyFooter(t *testing.T) {
	h := toolkit.NewLabel("H")
	b := toolkit.NewLabel("B")
	f := toolkit.NewLabel("F")
	p := &packedVBox{header: h, body: b, footer: f, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 30})

	if got := h.Bounds(); got.Y != 0 || got.H != 1 || got.W != 80 {
		t.Errorf("header bounds = %+v, want (0,0,80,1)", got)
	}
	if got := f.Bounds(); got.Y != 29 || got.H != 1 || got.W != 80 {
		t.Errorf("footer bounds = %+v, want (0,29,80,1)", got)
	}
	if got := b.Bounds(); got.Y != 1 || got.H != 28 || got.W != 80 {
		t.Errorf("body bounds = %+v, want (0,1,80,28)", got)
	}
}

// TestPackedVBoxHandlesNilChildren covers the nil-guard branches so
// SetBounds / Draw / OnEvent never panic on partially-populated
// helpers (a future demo may compose header + body without a footer).
func TestPackedVBoxHandlesNilChildren(t *testing.T) {
	p := &packedVBox{}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
	p.OnEvent(toolkit.Event{})
}

// TestPackedVBoxDrawAllChildren renders every child so their Draw
// methods are covered through the layout helper's dispatch.
func TestPackedVBoxDrawAllChildren(t *testing.T) {
	h := toolkit.NewLabel("H")
	b := toolkit.NewLabel("B")
	f := toolkit.NewLabel("F")
	p := &packedVBox{header: h, body: b, footer: f, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}

// TestPackedVBoxForwardsEventsToBody covers the OnEvent forwarding
// branch: an event delivered to packedVBox reaches the body widget.
func TestPackedVBoxForwardsEventsToBody(t *testing.T) {
	tv := toolkit.NewTextView("")
	tv.Focused = true
	p := &packedVBox{body: tv}
	before := tv.Text()
	p.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "x"})
	if tv.Text() == before {
		t.Fatal("event was not forwarded to body TextView")
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
