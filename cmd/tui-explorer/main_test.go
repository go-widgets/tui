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
// so key handlers don't nil-deref.
func TestNewStateFields(t *testing.T) {
	s := newState()
	if s.tree == nil {
		t.Error("state.tree is nil")
	}
	if s.notebook == nil {
		t.Error("state.notebook is nil")
	}
	if s.contentTV == nil {
		t.Error("state.contentTV is nil")
	}
	if s.infoLabel == nil {
		t.Error("state.infoLabel is nil")
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
}

// TestKeysReturnsAllHandlers checks the expected key names are all
// present in the map returned by state.keys.
func TestKeysReturnsAllHandlers(t *testing.T) {
	s := newState()
	m := s.keys()
	for _, k := range []string{"q", "Ctrl+C", "?", "/", "Enter"} {
		if _, ok := m[k]; !ok {
			t.Errorf("keys()[%q] missing", k)
		}
	}
}

// TestKeyHandlersRunWithoutPanic invokes every handler with a fresh
// tui.App to guarantee none blow up on their happy path.
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

// TestQuitHandlerCallsQuit covers the "q" binding using tui.App's
// IsQuitting introspection.
func TestQuitHandlerCallsQuit(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	s.keys()["q"](a)
	if !a.IsQuitting() {
		t.Fatal("q handler did not call Quit (IsQuitting still false)")
	}
}

// TestCtrlCHandlerCallsQuit covers the "Ctrl+C" binding.
func TestCtrlCHandlerCallsQuit(t *testing.T) {
	s := newState()
	a := tui.NewApp()
	s.keys()["Ctrl+C"](a)
	if !a.IsQuitting() {
		t.Fatal("Ctrl+C handler did not call Quit")
	}
}

// TestHelpToggleFlipsVisible verifies the "?" handler toggles the
// help popover's Visible field on and off.
func TestHelpToggleFlipsVisible(t *testing.T) {
	s := newState()
	if s.helpPopover.Visible {
		t.Fatal("help popover should start hidden")
	}
	s.keys()["?"](tui.NewApp())
	if !s.helpPopover.Visible {
		t.Fatal("help popover should be visible after ? toggle")
	}
	s.keys()["?"](tui.NewApp())
	if s.helpPopover.Visible {
		t.Fatal("help popover should be hidden after second ? toggle")
	}
}

// TestSearchToggleFlipsVisible mirrors the help toggle for the
// "/" binding.
func TestSearchToggleFlipsVisible(t *testing.T) {
	s := newState()
	if s.searchPopover.Visible {
		t.Fatal("search popover should start hidden")
	}
	s.keys()["/"](tui.NewApp())
	if !s.searchPopover.Visible {
		t.Fatal("search popover should be visible after / toggle")
	}
	s.keys()["/"](tui.NewApp())
	if s.searchPopover.Visible {
		t.Fatal("search popover should be hidden after second / toggle")
	}
}

// TestEnterHandlerSyncsSelection covers syncSelection's success path.
func TestEnterHandlerSyncsSelection(t *testing.T) {
	s := newState()
	var licenseNode *toolkit.TreeNode
	for _, c := range s.tree.Root.Children {
		if c.Label == "LICENSE" {
			licenseNode = c
			break
		}
	}
	if licenseNode == nil {
		t.Fatal("test fixture: LICENSE node missing from tree")
	}
	s.tree.Selected = licenseNode
	s.keys()["Enter"](tui.NewApp())
	if s.infoLabel.Text != "LICENSE" {
		t.Errorf("infoLabel.Text = %q, want %q", s.infoLabel.Text, "LICENSE")
	}
	if len(s.contentTV.Lines) == 0 {
		t.Error("contentTV.Lines empty after Enter on a valid selection")
	}
	joined := strings.Join(s.contentTV.Lines, "\n")
	if !strings.Contains(joined, "BSD-3-Clause") {
		t.Errorf("contentTV missing file body, got %q", joined)
	}
}

// TestEnterHandlerNilSelection covers the "sel == nil" return branch
// of syncSelection.
func TestEnterHandlerNilSelection(t *testing.T) {
	s := newState()
	before := s.infoLabel.Text
	s.tree.Selected = nil
	s.keys()["Enter"](tui.NewApp())
	if s.infoLabel.Text != before {
		t.Errorf("infoLabel.Text mutated on nil selection: was %q, now %q", before, s.infoLabel.Text)
	}
}

// TestEnterHandlerNonStringData covers the "path type-assert failed"
// return branch of syncSelection.
func TestEnterHandlerNonStringData(t *testing.T) {
	s := newState()
	s.tree.Selected = &toolkit.TreeNode{Label: "weird", Data: 42}
	s.keys()["Enter"](tui.NewApp())
	if s.infoLabel.Text != "weird" {
		t.Errorf("infoLabel.Text = %q, want %q", s.infoLabel.Text, "weird")
	}
}

// TestEnterHandlerUnknownPath covers the "path not in files map"
// return branch of syncSelection.
func TestEnterHandlerUnknownPath(t *testing.T) {
	s := newState()
	s.tree.Selected = &toolkit.TreeNode{Label: "ghost", Data: "/nowhere"}
	s.keys()["Enter"](tui.NewApp())
	if s.infoLabel.Text != "ghost" {
		t.Errorf("infoLabel.Text = %q, want %q", s.infoLabel.Text, "ghost")
	}
}

// TestRunDefaultThemeSucceeds drives run(nil, ...) with a stubbed
// runAppFunc so we skip the interactive event loop.
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
	runAppFunc = func(a *tui.App) int { return 0 }

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run(nil) = %d, want 0. stderr:\n%s", code, stderr.String())
	}
	if captured == nil {
		t.Fatal("newAppFunc was not called")
	}
	if captured.Theme == nil {
		t.Fatal("run did not install a theme")
	}
	if captured.Root == nil {
		t.Fatal("run did not install a Root")
	}
	if len(captured.Keys) == 0 {
		t.Fatal("run did not install any Keys")
	}
	if captured.Theme.Background != toolkit.DefaultLight().Background {
		t.Error("default theme is not the light theme")
	}
}

// TestRunThemeDarkApplied drives run --theme=dark.
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
	runAppFunc = func(a *tui.App) int { return 0 }

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--theme=dark"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(--theme=dark) = %d, want 0", code)
	}
	if captured.Theme.Background != toolkit.DefaultDark().Background {
		t.Error("--theme=dark did not install the dark theme")
	}
}

// TestRunPropagatesAppExitCode verifies run's return value passes
// through from runAppFunc — i.e., the last statement in run is
// reachable and its value observable.
func TestRunPropagatesAppExitCode(t *testing.T) {
	origRun := runAppFunc
	defer func() { runAppFunc = origRun }()

	runAppFunc = func(a *tui.App) int { return 5 }

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 5 {
		t.Fatalf("run() = %d, want 5 (from stubbed runAppFunc)", code)
	}
}

// TestRunBadFlagReturnsTwo covers the flag-parse error branch.
func TestRunBadFlagReturnsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--not-a-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(--not-a-flag) = %d, want 2", code)
	}
}

// TestPackedVBoxLayoutHeaderBodyFooter drives the layout helper
// directly: given 80×30, header at y=0 h=1, footer at y=29 h=1,
// body y=1 h=28. Catches the regression that shipped in v0.3.0 /
// v0.3.1 where a plain toolkit.VBox divided the three children
// equally, wrecking the interactive demo's chrome.
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
func TestPackedVBoxHandlesNilChildren(t *testing.T) {
	p := &packedVBox{}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
	p.OnEvent(toolkit.Event{})
}
func TestPackedVBoxDrawAllChildren(t *testing.T) {
	h := toolkit.NewLabel("H")
	b := toolkit.NewLabel("B")
	f := toolkit.NewLabel("F")
	p := &packedVBox{header: h, body: b, footer: f, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestPackedVBoxOverlaysRenderAndSize(t *testing.T) {
	overlay := toolkit.NewLabel("overlay")
	p := &packedVBox{
		body:     toolkit.NewLabel("body"),
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

// TestMainSuccessPath drives main() via the runFunc/osExit seams so
// coverage picks up the statements inside main without actually
// exiting the test binary.
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
	runFunc = func([]string, io.Writer, io.Writer) int { return 7 }
	osExit = func(code int) { gotCode = code }
	main()
	if gotCode != 7 {
		t.Fatalf("main() called osExit(%d), want 7", gotCode)
	}
}

// TestDefaultRunAppSeamCallsRun exercises defaultRunApp — the
// production runAppFunc that hands off to tui.App.Run. We can't
// drive a real event loop here (no TTY, no goroutines to unwind
// deterministically), so we build a tui.App with an openTTYFn seam
// that returns an error, which makes Run() bail out with a non-zero
// code before touching stdin/stdout. This proves the "return a.Run()"
// path is reachable AND that the wrapper propagates the exit code.
func TestDefaultRunAppSeamCallsRun(t *testing.T) {
	a := tui.NewApp()
	// Force Run() to hit its openTTYFn error branch — it opens the
	// TTY very early, returns an error, and Run bails out with a
	// non-zero exit code. We just need the "return a.Run()"
	// statement to be observable.
	a = withOpenTTYError(a)
	code := defaultRunApp(a)
	if code == 0 {
		t.Fatal("defaultRunApp against an openTTY-error App returned 0; want non-zero")
	}
}

// withOpenTTYError swaps the App's openTTYFn seam to return a fixed
// error so Run() bails out immediately, without touching a real TTY.
// tui.App exposes SetOpenTTYFn (added in v0.3.0) so consumer code can
// wire an alternate TTY factory — that's what powers this test.
func withOpenTTYError(a *tui.App) *tui.App {
	a.SetOpenTTYFn(func(*os.File) (tui.TTY, error) {
		return nil, errors.New("test: no tty available")
	})
	return a
}
