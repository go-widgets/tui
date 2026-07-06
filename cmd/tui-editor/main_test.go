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

func TestNewStateWiresMenuItemsToPopovers(t *testing.T) {
	s := newState()
	if len(s.menuBar.Items) != 4 {
		t.Fatalf("menu items = %d, want 4", len(s.menuBar.Items))
	}
	cases := []struct {
		idx int
		pop **cellPopover
	}{
		{0, &s.filePopover},
		{1, &s.editPopover},
		{2, &s.viewPopover},
		{3, &s.helpPopover},
	}
	for i, tc := range cases {
		s.menuBar.Items[tc.idx].OnClick()
		if !(*tc.pop).Visible {
			t.Errorf("item %d OnClick did not open its popover", i)
		}
		s.menuBar.Items[tc.idx].OnClick()
		if (*tc.pop).Visible {
			t.Errorf("item %d OnClick second call did not close", i)
		}
	}
}

// TestPackedVBoxCapturesDragFromClickTarget — mirrors tui-explorer's
// capture test for the tui-editor's packedVBox copy.
func TestPackedVBoxCapturesDragFromClickTarget(t *testing.T) {
	body := &cellTextEdit{Lines: []string{"a", "b"}}
	overlay := &cellTextEdit{Lines: []string{"o"}}
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
	body := &cellTextEdit{Lines: []string{"line0", "line1", "line2"}}
	invisible := &cellPopover{Title: "P", Body: []string{"x"}, Visible: false}
	p := &packedVBox{
		body:     body,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{invisible},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
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
	body := &cellTextEdit{Lines: []string{"a"}}
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
	body := &cellTextEdit{Lines: []string{"line0", "line1", "line2"}}
	overlay := &cellTextEdit{Lines: []string{"o"}, Focused: true}
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

	// Click INSIDE overlay: local (5, 4) → overlay-local (1, 1).
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if overlay.CursorLine != 0 || overlay.CursorCol != 1 {
		t.Errorf("overlay click: cursor=(%d,%d), want (1,0)", overlay.CursorCol, overlay.CursorLine)
	}
	// Body OUTSIDE overlay X-strip: (1, 3) — X=1 < 4 so overlay
	// skipped. Y=3 in body band → body-local Y=2 → cellTextEdit at
	// line 2, col 1.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 3})
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
	body := &cellTextEdit{Lines: []string{"aaa"}}
	p := &packedVBox{body: body, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 10})
	// Body region click. Y=2 → body-local Y=1 → cellTextEdit clamps
	// to line 0 (only 1 line in Lines), col 2.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 2})
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
	overlay := &cellTextEdit{Lines: []string{"a"}, Focused: true}
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

// cellTextEdit unit tests.
func TestCellTextEditSetTextAndText(t *testing.T) {
	e := &cellTextEdit{}
	e.SetText("a\nb\nc")
	if len(e.Lines) != 3 {
		t.Fatalf("SetText lines = %v, want 3", e.Lines)
	}
	if got := e.Text(); got != "a\nb\nc" {
		t.Errorf("Text() = %q", got)
	}
}
// TestCellTextEditTextOnNilLines exercises the "len==0 return \"\""
// early-return branch.
func TestCellTextEditTextOnNilLines(t *testing.T) {
	e := &cellTextEdit{} // Lines is nil
	if got := e.Text(); got != "" {
		t.Errorf("Text() on nil = %q, want empty", got)
	}
}

func TestCellTextEditSetTextEmpty(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"foo"}}
	e.SetText("")
	if len(e.Lines) != 1 || e.Lines[0] != "" {
		t.Errorf("SetText(\"\"): %v, want [\"\"]", e.Lines)
	}
	if got := e.Text(); got != "" {
		t.Errorf("Text() empty = %q", got)
	}
}
func TestCellTextEditSetTextTrailingNewline(t *testing.T) {
	e := &cellTextEdit{}
	e.SetText("hello\n")
	if len(e.Lines) != 1 || e.Lines[0] != "hello" {
		t.Errorf("trailing newline: %v", e.Lines)
	}
}
func TestCellTextEditDrawEmpty(t *testing.T) {
	e := &cellTextEdit{Lines: []string{""}, Focused: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	e.Draw(pnt, toolkit.DefaultLight())
}
func TestCellTextEditDrawWithContent(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a", "b", "c"}, CursorLine: 1, CursorCol: 0, Focused: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 5})
	pnt := painter.NewPixelPainter(make([]byte, 10*5*4), 10, 5)
	e.Draw(pnt, toolkit.DefaultLight())
}
func TestCellTextEditDrawUnfocusedNoCursor(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a"}, Focused: false}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	e.Draw(pnt, toolkit.DefaultLight())
}
func TestCellTextEditDrawClipsBeyondBounds(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a", "b", "c", "d", "e"}, Focused: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3}) // 3 rows, 5 lines
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	e.Draw(pnt, toolkit.DefaultLight())
}
func TestCellTextEditDrawCursorPastRight(t *testing.T) {
	// Cursor at column past bounds width — Draw skips the cursor.
	e := &cellTextEdit{Lines: []string{"short"}, CursorLine: 0, CursorCol: 50, Focused: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	e.Draw(pnt, toolkit.DefaultLight())
}
func TestCellTextEditDrawCursorPastBottom(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a"}, CursorLine: 50, CursorCol: 0, Focused: true}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	e.Draw(pnt, toolkit.DefaultLight())
}

// cellTextEdit editing OnEvent tests.
func TestCellTextEditInsertChar(t *testing.T) {
	e := &cellTextEdit{Lines: []string{""}}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "a"})
	if e.Lines[0] != "a" || e.CursorCol != 1 {
		t.Errorf("after 'a': lines=%v col=%d", e.Lines, e.CursorCol)
	}
}
func TestCellTextEditInsertBeyondEndClamps(t *testing.T) {
	// Cursor past line length: insert clamps back.
	e := &cellTextEdit{Lines: []string{"abc"}, CursorCol: 100}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "X"})
	if e.Lines[0] != "abcX" {
		t.Errorf("cursor clamp on insert: %v", e.Lines)
	}
}
func TestCellTextEditNilLinesGetsBootstrapped(t *testing.T) {
	// Event on empty Lines slice bootstraps to [""].
	e := &cellTextEdit{}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "x"})
	if len(e.Lines) != 1 || e.Lines[0] != "x" {
		t.Errorf("bootstrap: %v", e.Lines)
	}
}
func TestCellTextEditBackspaceMidLine(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"abc"}, CursorCol: 2}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
	if e.Lines[0] != "ac" || e.CursorCol != 1 {
		t.Errorf("backspace: %v col=%d", e.Lines, e.CursorCol)
	}
}
func TestCellTextEditBackspaceAtLineStartMerges(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a", "b"}, CursorLine: 1, CursorCol: 0}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
	if len(e.Lines) != 1 || e.Lines[0] != "ab" {
		t.Errorf("backspace merge: %v", e.Lines)
	}
	if e.CursorLine != 0 || e.CursorCol != 1 {
		t.Errorf("cursor after merge: line=%d col=%d", e.CursorLine, e.CursorCol)
	}
}
func TestCellTextEditBackspaceAtDocStartIsNoop(t *testing.T) {
	e := &cellTextEdit{Lines: []string{""}, CursorLine: 0, CursorCol: 0}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
	if len(e.Lines) != 1 || e.CursorLine != 0 || e.CursorCol != 0 {
		t.Errorf("backspace at 0,0 mutated state: %+v", e)
	}
}
func TestCellTextEditEnterSplitsLine(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"abc"}, CursorCol: 1}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if len(e.Lines) != 2 || e.Lines[0] != "a" || e.Lines[1] != "bc" {
		t.Errorf("enter split: %v", e.Lines)
	}
	if e.CursorLine != 1 || e.CursorCol != 0 {
		t.Errorf("cursor after enter: %d:%d", e.CursorLine, e.CursorCol)
	}
}
func TestCellTextEditEnterClampsPastLineEnd(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a"}, CursorCol: 50}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if len(e.Lines) != 2 || e.Lines[0] != "a" || e.Lines[1] != "" {
		t.Errorf("enter clamped: %v", e.Lines)
	}
}
func TestCellTextEditArrows(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"abc", "de"}, CursorLine: 0, CursorCol: 2}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Right"})
	if e.CursorCol != 3 {
		t.Errorf("Right: col=%d", e.CursorCol)
	}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if e.CursorLine != 1 || e.CursorCol != 2 {
		t.Errorf("Down: line=%d col=%d", e.CursorLine, e.CursorCol)
	}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Left"})
	if e.CursorCol != 1 {
		t.Errorf("Left: col=%d", e.CursorCol)
	}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if e.CursorLine != 0 || e.CursorCol != 1 {
		t.Errorf("Up: line=%d col=%d", e.CursorLine, e.CursorCol)
	}
}
func TestCellTextEditArrowsAtEdgesAreNoops(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"abc"}, CursorLine: 0, CursorCol: 0}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Left"})
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if e.CursorLine != 0 || e.CursorCol != 0 {
		t.Errorf("Left/Up at 0,0: %+v", e)
	}
	e.CursorCol = 3
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Right"})
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if e.CursorLine != 0 || e.CursorCol != 3 {
		t.Errorf("Right/Down at end: %+v", e)
	}
}
func TestCellTextEditArrowClampsCol(t *testing.T) {
	// Move Down from a long line to a shorter one — col clamps.
	e := &cellTextEdit{Lines: []string{"abcde", "f"}, CursorLine: 0, CursorCol: 4}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if e.CursorLine != 1 || e.CursorCol != 1 {
		t.Errorf("Down col clamp: line=%d col=%d", e.CursorLine, e.CursorCol)
	}
	// And back Up: same.
	e.CursorLine = 1
	e.CursorCol = 1
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if e.CursorLine != 0 || e.CursorCol != 1 {
		t.Errorf("Up col: line=%d col=%d", e.CursorLine, e.CursorCol)
	}
}
// TestCellTextEditUpClampsCol covers the "col > len(prev line)" clamp
// on Up (the Down variant is covered by TestCellTextEditArrowClampsCol).
func TestCellTextEditUpClampsCol(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"a", "bcde"}, CursorLine: 1, CursorCol: 3}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if e.CursorLine != 0 || e.CursorCol != 1 {
		t.Errorf("Up col clamp: line=%d col=%d, want 0:1", e.CursorLine, e.CursorCol)
	}
}
func TestCellTextEditOtherEventIgnored(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"abc"}, CursorCol: 1}
	before := e.CursorCol
	// EventCompositionStart is neither EventChar, EventKeyDown, nor
	// EventClick — the outer switch's default arm is silent.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventCompositionStart})
	if e.CursorCol != before {
		t.Errorf("Composition mutated state: col=%d", e.CursorCol)
	}
	// Unknown keydown code — no-op.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "F13"})
	if e.CursorCol != before {
		t.Errorf("F13 mutated state: col=%d", e.CursorCol)
	}
}

// TestCellTextEditClickPositionsCursor — a click at widget-local
// (col, row) inside the content sets (CursorCol, CursorLine) to
// exactly that cell. Out-of-range coords clamp to the nearest valid
// cell instead of overshooting.
func TestCellTextEditClickPositionsCursor(t *testing.T) {
	e := &cellTextEdit{Lines: []string{"hello", "world!", "foo"}, CursorLine: 0, CursorCol: 0}
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 1})
	if e.CursorLine != 1 || e.CursorCol != 3 {
		t.Errorf("click(3,1): (%d, %d), want (3, 1)", e.CursorCol, e.CursorLine)
	}
	// X past end-of-line clamps to len(line).
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 100, Y: 0})
	if e.CursorLine != 0 || e.CursorCol != 5 { // len("hello") = 5
		t.Errorf("click(100,0) end-clamp: (%d, %d), want (5, 0)", e.CursorCol, e.CursorLine)
	}
	// Y past last line clamps to last line.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 999})
	if e.CursorLine != 2 || e.CursorCol != 1 {
		t.Errorf("click(1,999) row-clamp: (%d, %d), want (1, 2)", e.CursorCol, e.CursorLine)
	}
	// Y negative clamps to 0.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: -3})
	if e.CursorLine != 0 || e.CursorCol != 2 {
		t.Errorf("click(2,-3) row-neg: (%d, %d), want (2, 0)", e.CursorCol, e.CursorLine)
	}
	// X negative clamps to 0.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: -5, Y: 0})
	if e.CursorLine != 0 || e.CursorCol != 0 {
		t.Errorf("click(-5,0) col-neg: (%d, %d), want (0, 0)", e.CursorCol, e.CursorLine)
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
