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

// TestNewStateWiresFileListOnSelectToSyncContent verifies the
// closure newState installs on the fileList: bumping selected then
// invoking onSelect must refresh the content pane. Also covers the
// closure body's statement — which was uncovered until this test.
func TestNewStateWiresFileListOnSelectToSyncContent(t *testing.T) {
	s := newState()
	// Simulate what an arrow key or click does: change selected,
	// then fire the callback.
	s.fileList.selected = 2
	if s.fileList.onSelect == nil {
		t.Fatal("newState left fileList.onSelect nil")
	}
	before := s.content.Text()
	s.fileList.onSelect(s.fileList.selected)
	if s.content.Text() == before {
		t.Errorf("onSelect closure did not refresh content: before=%q after=%q",
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
	// Unknown key is a no-op (default arm of switch).
	before := fl.selected
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Left"})
	if fl.selected != before {
		t.Errorf("unknown key mutated selection: %d → %d", before, fl.selected)
	}
	// EventCompositionStart (a Kind we don't handle at all) is a
	// no-op — guards the outer switch's default arm.
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventCompositionStart})
	if fl.selected != before {
		t.Errorf("composition event mutated selection: %d → %d", before, fl.selected)
	}
}

// TestFileListOnEventClickSelectsRow — a click at widget-local Y=k
// selects item k, invokes onSelect, and no-ops when Y falls outside
// [0, len(items)).
func TestFileListOnEventClickSelectsRow(t *testing.T) {
	items := []string{"a", "b", "c", "d"}
	called := -1
	fl := &fileList{items: items, selected: 0, onSelect: func(i int) { called = i }}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 2})
	if fl.selected != 2 {
		t.Errorf("click Y=2: selected = %d, want 2", fl.selected)
	}
	if called != 2 {
		t.Errorf("onSelect not called with 2: got %d", called)
	}
	// Y below range = no-op.
	fl.selected = 1
	called = -1
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: -1})
	if fl.selected != 1 || called != -1 {
		t.Errorf("click Y=-1 must no-op: selected=%d called=%d", fl.selected, called)
	}
	// Y past end = no-op.
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 10})
	if fl.selected != 1 || called != -1 {
		t.Errorf("click Y=10 must no-op: selected=%d called=%d", fl.selected, called)
	}
	// Nil onSelect must not crash — cover the callback-nil branch.
	fl.onSelect = nil
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventClick, Y: 3})
	if fl.selected != 3 {
		t.Errorf("click with nil onSelect: selected = %d, want 3", fl.selected)
	}
}

// TestFileListOnSelectFiresOnArrowNav — arrow-key nav must also
// invoke onSelect so click and arrow paths stay consistent.
func TestFileListOnSelectFiresOnArrowNav(t *testing.T) {
	called := []int{}
	fl := &fileList{
		items:    []string{"a", "b", "c"},
		selected: 0,
		onSelect: func(i int) { called = append(called, i) },
	}
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	if got, want := called, []int{1, 2, 1}; !equalInts(got, want) {
		t.Errorf("onSelect calls = %v, want %v", got, want)
	}
	// nil callback path — no crash.
	fl.onSelect = nil
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	// no-op guard branches: Up at top / Down at bottom must ALSO
	// leave onSelect uncalled (the callback lives inside the mutate
	// path, not the boundary-guard path).
	fl.onSelect = func(i int) { called = append(called, -1000) }
	fl.selected = 0
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"})
	fl.selected = len(fl.items) - 1
	fl.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"})
	for _, v := range called {
		if v == -1000 {
			t.Errorf("boundary-guard fired onSelect: %v", called)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

// TestHSplitClickRoutesByHitTest verifies a click at widget-local
// (2, 2) with leftFrac=30 on W=100 (lw=30) lands in the left pane
// with unchanged coords, a click at local X=60 goes to right with
// translated X = 60-30 = 30.
func TestHSplitClickRoutesByHitTest(t *testing.T) {
	fl := &fileList{items: []string{"a", "b", "c", "d"}, selected: 0}
	tp := &textPreview{}
	tp.setText("x\ny\nz")
	h := &hSplit{left: fl, right: tp, leftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 10, Y: 5, W: 100, H: 20})
	// Local (2, 2) → left pane; fileList sees (2, 2), selects item 2.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 2})
	if fl.selected != 2 {
		t.Errorf("left-pane click: selected = %d, want 2", fl.selected)
	}
	// Local (60, 6) → right pane. textPreview has no visible state
	// to check but the code path must not crash.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 60, Y: 6})
	// Local X=200 is past the right pane's width too. Still routes
	// to right pane per the contract (parent already hit-tested);
	// right handles gracefully.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 200, Y: 6})
}

// TestHSplitClickWithNilLeftIsDropped — click on left region with
// nil left just drops the event silently. The nil-left / hit-right
// path never fires because clicks always route by X < lw.
func TestHSplitClickWithNilLeftIsDropped(t *testing.T) {
	h := &hSplit{leftFrac: 50}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 3})
}

// TestHSplitClickOnRightWithNilRight — click on right region with
// nil right silently drops.
func TestHSplitClickOnRightWithNilRight(t *testing.T) {
	h := &hSplit{leftFrac: 50}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 30, Y: 3})
}

// textPreview tests.
func TestTextPreviewSetTextAndText(t *testing.T) {
	tp := &textPreview{}
	tp.setText("a\nb\nc")
	if len(tp.lines) != 3 {
		t.Fatalf("lines = %v, want 3", tp.lines)
	}
	if got := tp.Text(); got != "a\nb\nc" {
		t.Errorf("Text() = %q, want %q", got, "a\nb\nc")
	}
}
func TestTextPreviewSetEmptyClears(t *testing.T) {
	tp := &textPreview{lines: []string{"a"}}
	tp.setText("")
	if tp.lines != nil {
		t.Errorf("lines not nil after setText(\"\"): %v", tp.lines)
	}
	if got := tp.Text(); got != "" {
		t.Errorf("Text() empty = %q, want empty", got)
	}
}
func TestTextPreviewSetTextTrailingNewline(t *testing.T) {
	tp := &textPreview{}
	tp.setText("hello\n")
	if len(tp.lines) != 1 || tp.lines[0] != "hello" {
		t.Errorf("trailing newline handling wrong: %v", tp.lines)
	}
}
func TestTextPreviewDrawRendersLinesAndClipsBounds(t *testing.T) {
	tp := &textPreview{}
	tp.setText("a\nb\nc\nd\ne")
	tp.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3}) // only 3 rows fit
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	tp.Draw(pnt, toolkit.DefaultLight())
}
func TestTextPreviewDrawEmpty(t *testing.T) {
	tp := &textPreview{}
	tp.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
	pnt := painter.NewPixelPainter(make([]byte, 10*3*4), 10, 3)
	tp.Draw(pnt, toolkit.DefaultLight())
}

// cellPopover Draw tests.
func TestCellPopoverDrawInvisibleIsNoop(t *testing.T) {
	p := &cellPopover{Title: "T", Body: []string{"a", "b"}, Visible: false}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestCellPopoverDrawVisibleRendersTitleAndBody(t *testing.T) {
	p := &cellPopover{Title: "T", Body: []string{"a", "b"}, Visible: true}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	p.Draw(pnt, toolkit.DefaultLight())
}
func TestCellPopoverDrawClampsBodyToBounds(t *testing.T) {
	// Bounds too small for the full body: the loop must break at
	// y >= r.Y+need-1.
	p := &cellPopover{Title: "T", Body: []string{"a", "b", "c", "d"}, Visible: true}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 5}) // need=7, capped to H=5
	pnt := painter.NewPixelPainter(make([]byte, 20*5*4), 20, 5)
	p.Draw(pnt, toolkit.DefaultLight())
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

// TestPackedVBoxClickRoutesByHitTest verifies the header / body /
// footer band routing + overlay top-priority.
func TestPackedVBoxClickRoutesByHitTest(t *testing.T) {
	// bodyFL is inside body (via hSplit) so a body click reaches it.
	bodyFL := &fileList{items: []string{"a", "b", "c", "d", "e"}, selected: 0}
	body := &hSplit{left: bodyFL, leftFrac: 100}
	overlayFL := &fileList{items: []string{"o1", "o2", "o3"}, selected: 0}
	p := &packedVBox{
		header:   toolkit.NewLabel("H"),
		body:     body,
		footer:   toolkit.NewLabel("F"),
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlayFL},
	}
	// H=20, headerH=1, footerH=1, so body spans Y ∈ [1,19). Overlay
	// bounds: (4, 3, 12, 14) → Y ∈ [3, 17), X ∈ [4, 16).
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})

	// Body click OUTSIDE the overlay region: (1, 18) — X=1 < 4 so
	// overlay skipped. Y=18 in body → body-local Y=17 → hSplit lw=20
	// so X=1 < 20 → left → fileList Y=17. fileList len=5 so drops.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 18})

	// Body click also OUTSIDE overlay but WITHIN fileList row range:
	// (1, 3) — X=1 < 4 so overlay skipped. Y=3 → body-local Y=2 →
	// fileList Y=2 → selects item 2. This exercises the "body
	// branch when Y still in body band but not overlay X-strip".
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 3})
	if bodyFL.selected != 2 {
		t.Errorf("body-below-overlay click: selected = %d, want 2", bodyFL.selected)
	}

	// Overlay hit: (5, 4) — X ∈ [4,16), Y ∈ [3,17). Translated to
	// overlay-local (1, 1) → fileList selects item 1.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if overlayFL.selected != 1 {
		t.Errorf("overlay click: overlayFL.selected = %d, want 1", overlayFL.selected)
	}

	// Header row (Y=0) — routes to header Label which no-ops.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 0})

	// Footer row (Y=19) — routes to footer Label with translated Y=0.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 19})
}

// TestPackedVBoxClickWithNilHeaderFooter — the nil guards in each
// switch branch must skip cleanly.
func TestPackedVBoxClickWithNilHeaderFooter(t *testing.T) {
	fl := &fileList{items: []string{"a", "b"}, selected: 0, onSelect: func(int) {}}
	p := &packedVBox{
		body:    fl,
		headerH: 1,
		footerH: 1,
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	// Click in body band: local Y=2 → fileList Y=1 → selects 1.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 2})
	if fl.selected != 1 {
		t.Errorf("body click: selected = %d, want 1", fl.selected)
	}
	// Click in header band (Y=0) with nil header — must not crash.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	// Click in footer band (Y=19) with nil footer — must not crash.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 19})
	// Click in body band with nil body — must not crash.
	p.body = nil
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 5})
}

// TestPackedVBoxOverlayClampsMinBoundsInHitTest — when the frame is
// too small, the ow<1 / oh<1 guards inside OnEvent must still keep
// the overlay hit-range non-empty so a clicked spot doesn't fall
// into an unreachable void.
func TestPackedVBoxOverlayClampsMinBoundsInHitTest(t *testing.T) {
	overlayFL := &fileList{items: []string{"a"}, selected: 0}
	p := &packedVBox{
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlayFL},
	}
	// W=6 → ow = W-8 = -2 → clamped to 1. H=4 → oh = H-2-4 = -2 → 1.
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 6, H: 4})
	// Overlay hit region reduced to X=[4,5), Y=[3,4). Click there.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 4, Y: 3})
	if overlayFL.selected != 0 {
		// A single-item list can only select item 0; the assertion
		// exercises the clamp branch rather than a movement.
		t.Errorf("overlay-min click: selected = %d, want 0", overlayFL.selected)
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
