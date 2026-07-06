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

// TestUpDownMovesSelectionAndSyncsContent covers the arrow-key
// handlers + the syncContent side effect.
func TestUpDownMovesSelectionAndSyncsContent(t *testing.T) {
	s := newState()
	// Start at 0. Down → 1.
	s.keys()["Down"](tui.NewApp())
	if s.fileList.selected != 1 {
		t.Fatalf("Down: selected = %d, want 1", s.fileList.selected)
	}
	wantContent := s.files[s.paths[1]]
	if got := s.content.Text(); got != strings.TrimRight(wantContent, "\n") && got != wantContent {
		t.Errorf("content after Down: %q, want %q", got, wantContent)
	}
	// Up → 0.
	s.keys()["Up"](tui.NewApp())
	if s.fileList.selected != 0 {
		t.Fatalf("Up: selected = %d, want 0", s.fileList.selected)
	}
}

// TestUpAtTopIsNoop covers the "already at top" branch.
func TestUpAtTopIsNoop(t *testing.T) {
	s := newState()
	s.fileList.selected = 0
	s.keys()["Up"](tui.NewApp())
	if s.fileList.selected != 0 {
		t.Fatalf("Up at top moved to %d", s.fileList.selected)
	}
}

// TestDownAtBottomIsNoop covers the "already at bottom" branch.
func TestDownAtBottomIsNoop(t *testing.T) {
	s := newState()
	s.fileList.selected = len(s.fileList.items) - 1
	last := s.fileList.selected
	s.keys()["Down"](tui.NewApp())
	if s.fileList.selected != last {
		t.Fatalf("Down at bottom moved to %d", s.fileList.selected)
	}
}

// TestEnterSyncsContent covers the Enter handler.
func TestEnterSyncsContent(t *testing.T) {
	s := newState()
	s.fileList.selected = 2 // "/docs/README.md"
	s.keys()["Enter"](tui.NewApp())
	if !strings.Contains(s.content.Text(), "Project") {
		t.Errorf("Enter did not sync content: %q", s.content.Text())
	}
}

// TestSyncContentOutOfRange covers the "selected out of range"
// early-return branch.
func TestSyncContentOutOfRange(t *testing.T) {
	s := newState()
	s.fileList.selected = -1
	s.syncContent()
	if !strings.Contains(s.content.Text(), "no selection") {
		t.Errorf("out-of-range selection didn't show (no selection): %q", s.content.Text())
	}
}
func TestSyncContentOutOfRangeHigh(t *testing.T) {
	s := newState()
	s.fileList.selected = 999
	s.syncContent()
	if !strings.Contains(s.content.Text(), "no selection") {
		t.Errorf("high-out-of-range selection didn't show (no selection): %q", s.content.Text())
	}
}

// TestFileListDrawRendersItems + selection highlight branch.
func TestFileListDrawRendersItems(t *testing.T) {
	fl := &fileList{items: []string{"a", "b", "c"}, selected: 1}
	fl.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 20*3*4), 20, 3)
	fl.Draw(pnt, toolkit.DefaultLight())
}

// TestFileListDrawOverflow covers the "row Y past bounds" break.
func TestFileListDrawOverflow(t *testing.T) {
	fl := &fileList{items: []string{"a", "b", "c", "d", "e"}, selected: 0}
	fl.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 2})
	pnt := painter.NewPixelPainter(make([]byte, 20*2*4), 20, 2)
	fl.Draw(pnt, toolkit.DefaultLight())
}

// TestFileListOnEventUpDown covers OnEvent's Up/Down branches +
// non-key event no-op.
func TestFileListOnEventUpDown(t *testing.T) {
	fl := &fileList{items: []string{"a", "b", "c"}, selected: 1}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if fl.selected != 0 {
		t.Errorf("Up: %d, want 0", fl.selected)
	}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if fl.selected != 1 {
		t.Errorf("Down: %d, want 1", fl.selected)
	}
	// Non-key event is a no-op.
	before := fl.selected
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventClick})
	if fl.selected != before {
		t.Errorf("non-key event mutated selection: %d → %d", before, fl.selected)
	}
	// Unknown key is a no-op too (default arm of switch).
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Left"})
	if fl.selected != before {
		t.Errorf("unknown key mutated selection: %d → %d", before, fl.selected)
	}
}

// TestFileListUpAtTopIsNoop + DownAtBottom cover the edge guards.
func TestFileListUpAtTopIsNoop(t *testing.T) {
	fl := &fileList{items: []string{"a", "b"}, selected: 0}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if fl.selected != 0 {
		t.Errorf("Up at top: %d, want 0", fl.selected)
	}
}
func TestFileListDownAtBottomIsNoop(t *testing.T) {
	fl := &fileList{items: []string{"a", "b"}, selected: 1}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if fl.selected != 1 {
		t.Errorf("Down at bottom: %d, want 1", fl.selected)
	}
}

// TestHSplitLayoutDistributesCorrectly covers the left/right split.
func TestHSplitLayoutDistributesCorrectly(t *testing.T) {
	left := toolkit.NewLabel("L")
	right := toolkit.NewLabel("R")
	h := &hSplit{left: left, right: right, leftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10})
	if left.Bounds().W != 30 {
		t.Errorf("left width = %d, want 30", left.Bounds().W)
	}
	if right.Bounds().W != 70 {
		t.Errorf("right width = %d, want 70", right.Bounds().W)
	}
}

// TestHSplitNilChildrenNoop covers the nil-guard branches.
func TestHSplitNilChildrenNoop(t *testing.T) {
	h := &hSplit{}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 100*10*4), 100, 10)
	h.Draw(pnt, toolkit.DefaultLight())
	h.OnEvent(toolkit.Event{})
}

// TestHSplitDrawAllChildren covers the Draw dispatch.
func TestHSplitDrawAllChildren(t *testing.T) {
	h := &hSplit{left: toolkit.NewLabel("L"), right: toolkit.NewLabel("R"), leftFrac: 50}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	h.Draw(pnt, toolkit.DefaultLight())
}

// TestHSplitOnEventForwardsToLeft covers the event-forward branch.
func TestHSplitOnEventForwardsToLeft(t *testing.T) {
	fl := &fileList{items: []string{"a", "b"}, selected: 0}
	h := &hSplit{left: fl, leftFrac: 50}
	h.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	if fl.selected != 1 {
		t.Errorf("hSplit did not forward Down to left: %d", fl.selected)
	}
}

// packedVBox tests preserved from v0.3.3.
func TestPackedVBoxLayoutHeaderBodyFooter(t *testing.T) {
	h := toolkit.NewLabel("H")
	b := toolkit.NewLabel("B")
	f := toolkit.NewLabel("F")
	p := &packedVBox{header: h, body: b, footer: f, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 80, H: 30})
	if got := h.Bounds(); got.Y != 0 || got.H != 1 {
		t.Errorf("header bounds = %+v, want (0,0,W,1)", got)
	}
	if got := f.Bounds(); got.Y != 29 || got.H != 1 {
		t.Errorf("footer bounds = %+v, want (0,29,W,1)", got)
	}
	if got := b.Bounds(); got.Y != 1 || got.H != 28 {
		t.Errorf("body bounds = %+v, want (0,1,W,28)", got)
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
	p := &packedVBox{body: toolkit.NewLabel("body"), overlays: []toolkit.Widget{overlay}}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})
	b := overlay.Bounds()
	if b.W != 32 || b.H != 16 || b.X != 4 || b.Y != 2 {
		t.Errorf("overlay bounds = %+v", b)
	}
	pnt := painter.NewPixelPainter(make([]byte, 40*20*4), 40, 20)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestPackedVBoxOverlayClampsMinimalSize(t *testing.T) {
	overlay := toolkit.NewLabel("o")
	p := &packedVBox{headerH: 1, footerH: 1, overlays: []toolkit.Widget{overlay}}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 4, H: 4})
	b := overlay.Bounds()
	if b.W < 1 || b.H < 1 {
		t.Errorf("overlay clamped < 1: %+v", b)
	}
}
func TestPackedVBoxForwardsEventsToBody(t *testing.T) {
	tv := toolkit.NewTextView("")
	tv.Focused = true
	p := &packedVBox{body: tv}
	before := tv.Text()
	p.OnEvent(toolkit.Event{Kind: toolkit.EventChar, Code: "x"})
	if tv.Text() == before {
		t.Fatal("event not forwarded to body TextView")
	}
}

// run() + main() coverage.
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
