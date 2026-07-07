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
// TestPackedVBoxOverlaysRenderAndSize verifies the overlay slot
// paths: SetBounds sizes each overlay to the padded body inset, and
// Draw dispatches to every registered overlay.
func TestPackedVBoxOverlaysRenderAndSize(t *testing.T) {
	overlay := toolkit.NewLabel("overlay")
	p := &packedVBox{
		body:     toolkit.NewLabel("body"),
		headerH:  0,
		footerH:  0,
		overlays: []toolkit.Widget{overlay},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})
	b := overlay.Bounds()
	if b.W != 32 || b.H != 16 || b.X != 4 || b.Y != 2 {
		t.Errorf("overlay bounds = %+v, want (4,2,32,16)", b)
	}
	pnt := painter.NewPixelPainter(make([]byte, 40*20*4), 40, 20)
	p.Draw(pnt, toolkit.DefaultLight())
}

// TestPackedVBoxOverlayClampsMinimalSize covers the "bw < 1" and
// "bh < 1" guard branches when the total frame is smaller than the
// header + footer + padding.
func TestPackedVBoxOverlayClampsMinimalSize(t *testing.T) {
	overlay := toolkit.NewLabel("o")
	p := &packedVBox{
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlay},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 4})
	b := overlay.Bounds()
	if b.W < 1 || b.H < 1 {
		t.Errorf("overlay bounds clamped to < 1: %+v", b)
	}
}

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

// -----------------------------------------------------------------
// menuBar (tui-editor local copy)
// -----------------------------------------------------------------

func TestMenuBarItemXRange(t *testing.T) {
	mb := &menuBar{Items: []menuItem{
		{Label: "File"},
		{Label: "Edit"},
		{Label: "View"},
		{Label: "Help"},
	}}
	// Item 0: [1, 5)
	x0, x1 := mb.itemXRange(0)
	if x0 != 1 || x1 != 5 {
		t.Errorf("item 0 range = [%d,%d), want [1,5)", x0, x1)
	}
	// Item 3: 1 + 3*(4+3) = 22, len 4 → [22, 26)
	x0, x1 = mb.itemXRange(3)
	if x0 != 22 || x1 != 26 {
		t.Errorf("item 3 range = [%d,%d), want [22,26)", x0, x1)
	}
	// Out-of-range index.
	x0, x1 = mb.itemXRange(99)
	if x0 != -1 || x1 != -1 {
		t.Errorf("index 99 = (%d,%d), want (-1,-1)", x0, x1)
	}
}

func TestMenuBarDrawRendersItems(t *testing.T) {
	mb := &menuBar{Items: []menuItem{{Label: "X"}}}
	mb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	pnt := painter.NewPixelPainter(make([]byte, 20*1*4), 20, 1)
	mb.Draw(pnt, toolkit.DefaultLight())
	// Empty items still Draws.
	(&menuBar{}).Draw(pnt, toolkit.DefaultLight())
}

func TestMenuBarClickFiresOnClick(t *testing.T) {
	var fired string
	mb := &menuBar{Items: []menuItem{
		{Label: "File", OnClick: func() { fired = "File" }},
		{Label: "Nil"},
	}}
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	if fired != "File" {
		t.Errorf("File click: fired = %q", fired)
	}
	fired = ""
	// Click on nil-callback item — no crash, no fire.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 9, Y: 0})
	if fired != "" {
		t.Errorf("nil-cb fired: %q", fired)
	}
	// Non-click event ignored.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if fired != "" {
		t.Errorf("keydown fired: %q", fired)
	}
	// Click in gap between items — no fire.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 0})
	if fired != "" {
		t.Errorf("gap fired: %q", fired)
	}
}

func TestNewStateWiresMenuItemsToDropdowns(t *testing.T) {
	s := newState()
	if len(s.menuBar.Items) != 4 {
		t.Fatalf("menu items = %d, want 4", len(s.menuBar.Items))
	}
	cases := []struct {
		idx int
		d   **menuDropdown
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
		wantX, _ := s.menuBar.itemXRange(tc.idx)
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

// -----------------------------------------------------------------
// menuDropdown (tui-editor local copy)
// -----------------------------------------------------------------

func TestMenuDropdownSizeAutoFitsBody(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"aaaaaa", "bb"}}
	w, h := d.size()
	if w != 10 || h != 4 {
		t.Errorf("size = (%d, %d), want (10, 4)", w, h)
	}
	e := &menuDropdown{Title: "X"}
	_, h = e.size()
	if h != 3 {
		t.Errorf("empty-body height = %d, want 3", h)
	}
}

func TestMenuDropdownSetBoundsIgnoresParentAndAnchors(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"x"}, AnchorX: 22, AnchorY: 1}
	d.SetBounds(toolkit.Rect{X: 99, Y: 99, W: 99, H: 99})
	got := d.Bounds()
	if got.X != 22 || got.Y != 1 {
		t.Errorf("anchor = (%d, %d), want (22, 1)", got.X, got.Y)
	}
	if got.W != 5 {
		t.Errorf("W = %d, want 5", got.W)
	}
}

func TestMenuDropdownHitTestReflectsVisibility(t *testing.T) {
	d := &menuDropdown{Body: []string{"x"}}
	d.SetBounds(toolkit.Rect{})
	if d.HitTest(1, 1) {
		t.Error("invisible dropdown claimed a hit")
	}
	d.Visible = true
	if !d.HitTest(1, 1) {
		t.Error("visible dropdown missed a hit inside bounds")
	}
	if d.HitTest(1000, 1000) {
		t.Error("visible dropdown claimed a hit outside bounds")
	}
}

func TestMenuDropdownDrawInvisibleIsNoop(t *testing.T) {
	d := &menuDropdown{Body: []string{"x"}}
	d.SetBounds(toolkit.Rect{})
	pnt := painter.NewPixelPainter(make([]byte, 20*10*4), 20, 10)
	d.Draw(pnt, toolkit.DefaultLight())
}

func TestMenuDropdownDrawVisibleRendersTitleAndBody(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"one", "two"}, Visible: true}
	pnt := painter.NewPixelPainter(make([]byte, 20*10*4), 20, 10)
	d.Draw(pnt, toolkit.DefaultLight())
	// Empty title → title-draw branch skipped.
	(&menuDropdown{Body: []string{"only"}, Visible: true}).Draw(pnt, toolkit.DefaultLight())
}

// TestMenuDropdownItemActionFiresOnRowClick covers every branch of
// the OnEvent handler: action fires, nil-action row, out-of-range,
// no ItemActions defined, and the non-click ignore path.
func TestMenuDropdownItemActionFiresOnRowClick(t *testing.T) {
	fired := -1
	d := &menuDropdown{
		Body: []string{"a", "b", "c"},
		ItemActions: []func(){
			func() { fired = 0 },
			nil, // informational row
			func() { fired = 2 },
		},
		Visible: true,
	}
	// Body row 0 (local Y=1) fires ItemActions[0].
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 1})
	if fired != 0 || d.Visible {
		t.Errorf("row 0: fired=%d visible=%v", fired, d.Visible)
	}
	// Nil-action row closes but fires nothing.
	fired = -1
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 2})
	if fired != -1 || d.Visible {
		t.Errorf("nil row: fired=%d visible=%v", fired, d.Visible)
	}
	// Out-of-range row (past end) closes without firing.
	fired = -1
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 99})
	if fired != -1 || d.Visible {
		t.Errorf("oob row: fired=%d visible=%v", fired, d.Visible)
	}
	// Title row (Y=0) → idx=-1 → nothing fires, still closes.
	fired = -1
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 0})
	if fired != -1 || d.Visible {
		t.Errorf("title row: fired=%d visible=%v", fired, d.Visible)
	}
	// Dropdown without any ItemActions defined — must not crash.
	e := &menuDropdown{Body: []string{"a"}, Visible: true}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 1})
	if e.Visible {
		t.Error("no-actions dropdown did not close on body click")
	}
}

func TestMenuDropdownOnEventClickCloses(t *testing.T) {
	d := &menuDropdown{Visible: true}
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if d.Visible {
		t.Error("click did not close dropdown")
	}
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if !d.Visible {
		t.Error("keydown incorrectly closed dropdown")
	}
}

// TestPackedVBoxCapturesDragFromClickTarget — mirrors tui-explorer's
// capture test for the tui-editor's packedVBox copy.
func TestPackedVBoxCapturesDragFromClickTarget(t *testing.T) {
	body := &tui.TextEditor{Lines: []string{"a", "b"}}
	overlay := &tui.TextEditor{Lines: []string{"o"}}
	p := &packedVBox{
		header:   toolkit.NewLabel("H"),
		body:     body,
		footer:   toolkit.NewLabel("F"),
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlay},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})

	// Body click captures body.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 5})
	if p.dragTarget != body {
		t.Errorf("body click did not capture body")
	}
	// Drag lands where body already got clicked; MouseUp releases.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 3, Y: 8})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 3, Y: 8})
	if p.dragTarget != nil {
		t.Fatal("MouseUp did not release capture")
	}

	// Overlay click captures overlay.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if p.dragTarget != overlay {
		t.Errorf("overlay click did not capture overlay")
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 5, Y: 4})

	// Header click captures header.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 0})
	if p.dragTarget != p.header {
		t.Errorf("header click did not capture header")
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 0, Y: 0})

	// Footer click captures footer.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 19})
	if p.dragTarget != p.footer {
		t.Errorf("footer click did not capture footer")
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 0, Y: 19})
}

// TestPackedVBoxInvisibleOverlayDoesNotClaimClicks — same coverage
// as tui-explorer's twin test.
func TestPackedVBoxInvisibleOverlayDoesNotClaimClicks(t *testing.T) {
	body := &tui.TextEditor{Lines: []string{"line0", "line1", "line2"}}
	invisible := &cellPopover{Title: "P", Body: []string{"x"}, Visible: false}
	p := &packedVBox{
		body:     body,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{invisible},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	// X=6 → body col 5 (the editor's 1-cell left pad offsets clicks by 1).
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 4})
	if body.CursorLine != 2 || body.CursorCol != 5 {
		t.Errorf("invisible overlay ate the click: cursor = (%d,%d), want (5,2)",
			body.CursorCol, body.CursorLine)
	}
	if p.dragTarget != body {
		t.Errorf("capture went to overlay, not body: %v", p.dragTarget)
	}
	invisible.Visible = true
	p.dragTarget = nil
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if p.dragTarget != invisible {
		t.Errorf("visible overlay did NOT claim the click: %v", p.dragTarget)
	}
}

func TestPackedVBoxDragWithoutCaptureIsDropped(t *testing.T) {
	body := &tui.TextEditor{Lines: []string{"a"}}
	p := &packedVBox{body: body, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 3, Y: 5})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 3, Y: 5})
	if p.dragTarget != nil {
		t.Errorf("stray drag/up mutated dragTarget: %v", p.dragTarget)
	}
}

// TestPackedVBoxClickRoutesByHitTest — click routing exercises the
// header / body / footer band + overlay top-priority.
func TestPackedVBoxClickRoutesByHitTest(t *testing.T) {
	body := &tui.TextEditor{Lines: []string{"line0", "line1", "line2"}}
	overlay := &tui.TextEditor{Lines: []string{"o"}, Focused: true}
	p := &packedVBox{
		header:   toolkit.NewLabel("H"),
		body:     body,
		footer:   toolkit.NewLabel("F"),
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlay},
	}
	// H=20, headerH=1, footerH=1 → body Y ∈ [1,19). Overlay bounds:
	// bx=4, by=3, ow=12, oh=14 → Y ∈ [3,17), X ∈ [4,16).
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})

	// Click INSIDE overlay: local (6, 4) → overlay-local (2, 1); the 1-cell
	// left pad maps X=2 to col 1.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 4})
	if overlay.CursorLine != 0 || overlay.CursorCol != 1 {
		t.Errorf("overlay click: cursor=(%d,%d), want (1,0)", overlay.CursorCol, overlay.CursorLine)
	}
	// Body OUTSIDE overlay X-strip: (2, 3) — X=2 < 4 so overlay
	// skipped. Y=3 in body band → body-local Y=2 → line 2, col 1
	// (X=2 minus the 1-cell left pad).
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 3})
	if body.CursorLine != 2 || body.CursorCol != 1 {
		t.Errorf("body click below overlay: cursor=(%d,%d), want (1,2)", body.CursorCol, body.CursorLine)
	}
	// Header row (Y=0) — Label OnEvent no-ops.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 0})
	// Footer row (Y=19) — Label OnEvent no-ops.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 19})
}

// TestPackedVBoxClickWithNilHeaderAndFooter covers nil-branch guards.
func TestPackedVBoxClickWithNilHeaderAndFooter(t *testing.T) {
	body := &tui.TextEditor{Lines: []string{"aaa"}}
	p := &packedVBox{body: body, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 10})
	// Body region click. Y=2 → body-local Y=1 → clamps to line 0 (only
	// 1 line), col 2 (X=3 minus the 1-cell left pad).
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 2})
	if body.CursorLine != 0 || body.CursorCol != 2 {
		t.Errorf("body click: cursor=(%d,%d), want (2,0)", body.CursorCol, body.CursorLine)
	}
	// Header-band click with nil header — no crash.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	// Footer-band click with nil footer — no crash.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 9})
	// Body-band click with nil body — no crash.
	p.body = nil
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 5})
}

// TestPackedVBoxOverlayClampsMinBoundsInHitTest — ow<1/oh<1 guards.
func TestPackedVBoxOverlayClampsMinBoundsInHitTest(t *testing.T) {
	overlay := &tui.TextEditor{Lines: []string{"a"}, Focused: true}
	p := &packedVBox{
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlay},
	}
	// W=6 → ow = -2 → clamped to 1. H=4 → oh = -2 → 1.
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 6, H: 4})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 4, Y: 3})
	if overlay.CursorLine != 0 || overlay.CursorCol != 0 {
		t.Errorf("min-overlay click: cursor=(%d,%d), want (0,0)", overlay.CursorCol, overlay.CursorLine)
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

// cellPopover tests.
func TestCellPopoverInvisibleNoop(t *testing.T) {
	p := &cellPopover{Title: "T", Body: []string{"a"}, Visible: false}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 30*10*4), 30, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestCellPopoverVisibleRenders(t *testing.T) {
	p := &cellPopover{Title: "T", Body: []string{"a", "b"}, Visible: true}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 30*10*4), 30, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestCellPopoverBodyClampedToBounds(t *testing.T) {
	p := &cellPopover{Title: "T", Body: []string{"a", "b", "c", "d"}, Visible: true}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 5}) // need=7, capped
	pnt := painter.NewPixelPainter(make([]byte, 20*5*4), 20, 5)
	p.Draw(pnt, toolkit.DefaultLight())
}
