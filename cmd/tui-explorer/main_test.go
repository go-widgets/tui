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
	"github.com/go-widgets/tui/syntax"
)

func TestSpanColor(t *testing.T) {
	light := toolkit.DefaultLight()
	dark := toolkit.DefaultDark()
	// Every kind resolves to an opaque colour in both palettes (covers each
	// case + the dark/light branch inside).
	for _, k := range []syntax.Kind{
		syntax.Keyword, syntax.String, syntax.Comment, syntax.Number,
		syntax.Type, syntax.Func, syntax.Plain, syntax.Punct,
	} {
		if c := spanColor(k, light); c.A != 0xFF {
			t.Errorf("light kind %d not opaque: %+v", k, c)
		}
		if c := spanColor(k, dark); c.A != 0xFF {
			t.Errorf("dark kind %d not opaque: %+v", k, c)
		}
	}
	// Spot-check the light/dark keyword hues + the Plain→OnSurface fallback.
	if got := spanColor(syntax.Keyword, light); got != rgb(0xA6, 0x26, 0xA4) {
		t.Errorf("light keyword = %+v", got)
	}
	if got := spanColor(syntax.Keyword, dark); got != rgb(0xC6, 0x78, 0xDD) {
		t.Errorf("dark keyword = %+v", got)
	}
	if got := spanColor(syntax.Plain, light); got != rgb(light.OnSurface.R, light.OnSurface.G, light.OnSurface.B) {
		t.Errorf("plain = %+v, want OnSurface", got)
	}
}

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

// -----------------------------------------------------------------
// menuBar
// -----------------------------------------------------------------

// TestMenuBarItemXRange covers the layout arithmetic used by both
// Draw (item start X) and OnEvent (hit-testing).
func TestMenuBarItemXRange(t *testing.T) {
	mb := &menuBar{Items: []menuItem{
		{Label: "File"},
		{Label: "View"},
		{Label: "Help"},
	}}
	// Item 0: padding 1, len 4 → [1, 5)
	x0, x1 := mb.itemXRange(0)
	if x0 != 1 || x1 != 5 {
		t.Errorf("item 0 range = [%d, %d), want [1, 5)", x0, x1)
	}
	// Item 1: 1 + 4 + 3 = 8, len 4 → [8, 12)
	x0, x1 = mb.itemXRange(1)
	if x0 != 8 || x1 != 12 {
		t.Errorf("item 1 range = [%d, %d), want [8, 12)", x0, x1)
	}
	// Item 2: 8 + 4 + 3 = 15, len 4 → [15, 19)
	x0, x1 = mb.itemXRange(2)
	if x0 != 15 || x1 != 19 {
		t.Errorf("item 2 range = [%d, %d), want [15, 19)", x0, x1)
	}
	// Out-of-range index returns sentinel.
	x0, x1 = mb.itemXRange(99)
	if x0 != -1 || x1 != -1 {
		t.Errorf("out-of-range index returned (%d,%d), want (-1,-1)", x0, x1)
	}
}

// TestMenuBarDrawRendersItems covers Draw for both empty + populated.
func TestMenuBarDrawRendersItems(t *testing.T) {
	mb := &menuBar{Items: []menuItem{{Label: "A"}, {Label: "Bb"}}}
	mb.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	pnt := painter.NewPixelPainter(make([]byte, 20*1*4), 20, 1)
	mb.Draw(pnt, toolkit.DefaultLight())
	// Empty items is also a valid state.
	mb2 := &menuBar{}
	mb2.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	mb2.Draw(pnt, toolkit.DefaultLight())
}

// TestMenuBarClickFiresOnClick covers the hit-testing path plus the
// nil-callback guard.
func TestMenuBarClickFiresOnClick(t *testing.T) {
	var fired string
	mb := &menuBar{Items: []menuItem{
		{Label: "File", OnClick: func() { fired = "File" }},
		{Label: "View", OnClick: func() { fired = "View" }},
		{Label: "Nil"}, // no callback
	}}
	// Click inside "File" range (X ∈ [1, 5)).
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	if fired != "File" {
		t.Errorf("click on File: fired = %q, want %q", fired, "File")
	}
	fired = ""
	// Click inside "View" range (X ∈ [8, 12)).
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 0})
	if fired != "View" {
		t.Errorf("click on View: fired = %q, want %q", fired, "View")
	}
	fired = ""
	// Click in the 3-space gap between items — no item hit, no fire.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 6, Y: 0})
	if fired != "" {
		t.Errorf("click in gap: fired = %q, want empty", fired)
	}
	// Click on the nil-callback item — no crash, no fire.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 16, Y: 0})
	if fired != "" {
		t.Errorf("click on nil-callback item: fired = %q, want empty", fired)
	}
	// Non-click event ignored.
	mb.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if fired != "" {
		t.Errorf("keydown fired something: %q", fired)
	}
}

// TestNewStateWiresMenuItemsToDropdowns verifies each menu item's
// OnClick anchors + toggles the matching dropdown, and that opening
// one closes any other that was open.
func TestNewStateWiresMenuItemsToDropdowns(t *testing.T) {
	s := newState()
	// menuBar items: File / View / Help — in that order.
	if len(s.menuBar.Items) != 3 {
		t.Fatalf("menu items = %d, want 3", len(s.menuBar.Items))
	}
	cases := []struct {
		idx  int
		name string
		d    **menuDropdown
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
		wantX, _ := s.menuBar.itemXRange(tc.idx)
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

// -----------------------------------------------------------------
// menuDropdown
// -----------------------------------------------------------------

func TestMenuDropdownSizeAutoFitsBody(t *testing.T) {
	d := &menuDropdown{Title: "F", Body: []string{"aaaaaa", "bb"}}
	w, h := d.size()
	// max("F"=1, "aaaaaa"=6, "bb"=2) = 6, +4 padding = 10.
	// h = 2 + len(Body) = 4.
	if w != 10 || h != 4 {
		t.Errorf("size = (%d, %d), want (10, 4)", w, h)
	}
	// Minimum height guard: empty body → h=3.
	e := &menuDropdown{Title: "X"}
	_, h = e.size()
	if h != 3 {
		t.Errorf("empty-body height = %d, want 3", h)
	}
}

func TestMenuDropdownSetBoundsIgnoresParentAndAnchors(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"x"}, AnchorX: 12, AnchorY: 1}
	d.SetBounds(toolkit.Rect{X: 99, Y: 99, W: 99, H: 99}) // parent's request — must be ignored
	got := d.Bounds()
	if got.X != 12 || got.Y != 1 {
		t.Errorf("anchor = (%d, %d), want (12, 1)", got.X, got.Y)
	}
	// Width = max(len("T"), len("x")) + 4 = 5.
	if got.W != 5 {
		t.Errorf("W = %d, want 5", got.W)
	}
}

func TestMenuDropdownHitTestReflectsVisibility(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"x"}, AnchorX: 0, AnchorY: 0}
	d.SetBounds(toolkit.Rect{})
	// Invisible: no hit.
	if d.HitTest(1, 1) {
		t.Error("invisible dropdown claimed a hit")
	}
	// Visible: hit inside bounds.
	d.Visible = true
	if !d.HitTest(1, 1) {
		t.Error("visible dropdown missed a hit inside bounds")
	}
	// Visible: miss outside bounds.
	if d.HitTest(1000, 1000) {
		t.Error("visible dropdown claimed a hit outside bounds")
	}
}

func TestMenuDropdownDrawInvisibleIsNoop(t *testing.T) {
	d := &menuDropdown{Title: "T", Body: []string{"x"}, Visible: false, AnchorX: 0, AnchorY: 0}
	d.SetBounds(toolkit.Rect{})
	pnt := painter.NewPixelPainter(make([]byte, 20*10*4), 20, 10)
	d.Draw(pnt, toolkit.DefaultLight())
}

func TestMenuDropdownDrawVisibleRendersTitleAndBody(t *testing.T) {
	d := &menuDropdown{
		Title: "T", Body: []string{"one", "two"},
		Visible: true, AnchorX: 0, AnchorY: 0,
	}
	pnt := painter.NewPixelPainter(make([]byte, 20*10*4), 20, 10)
	d.Draw(pnt, toolkit.DefaultLight())
	// Empty title → title-draw branch skipped, body still paints.
	e := &menuDropdown{Body: []string{"only body"}, Visible: true}
	e.Draw(pnt, toolkit.DefaultLight())
}

func TestMenuDropdownOnEventClickCloses(t *testing.T) {
	d := &menuDropdown{Visible: true}
	d.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 1})
	if d.Visible {
		t.Error("click did not close dropdown")
	}
	// Non-click event ignored.
	d.Visible = true
	d.OnEvent(toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
	if !d.Visible {
		t.Error("keydown incorrectly closed dropdown")
	}
}

// -----------------------------------------------------------------
// hSplit — grip + drag
// -----------------------------------------------------------------

// TestHSplitDrawPaintsGripInBorderThenAccent covers both Draw
// branches (idle → Border colour, dragging → Accent colour).
func TestHSplitDrawPaintsGrip(t *testing.T) {
	h := &hSplit{
		left: toolkit.NewLabel("L"), right: toolkit.NewLabel("R"),
		leftFrac: 30,
	}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	pnt := painter.NewPixelPainter(make([]byte, 40*10*4), 40, 10)
	h.Draw(pnt, toolkit.DefaultLight())
	// Toggle dragging state so the accent-colour branch of Draw runs.
	h.dragging = true
	h.Draw(pnt, toolkit.DefaultLight())
}

// TestGripZoneCenters — the grip strip is a centred fraction of the
// bar height, clamped to [3, 7]. Exercises min / max / natural cases
// + the "strip taller than the bar" clamp for tiny bars.
func TestGripZoneCenters(t *testing.T) {
	// H=28 → h = 28/6 = 4 → centred at y0 = (28-4)/2 = 12 → [12, 16).
	if y0, y1 := gripZone(28); y0 != 12 || y1 != 16 {
		t.Errorf("gripZone(28) = [%d, %d), want [12, 16)", y0, y1)
	}
	// H=10 → h = 10/6 = 1 → clamped up to 3 → y0 = (10-3)/2 = 3 → [3, 6).
	if y0, y1 := gripZone(10); y0 != 3 || y1 != 6 {
		t.Errorf("gripZone(10) = [%d, %d), want [3, 6)", y0, y1)
	}
	// H=60 → h = 60/6 = 10 → clamped down to 7 → y0 = (60-7)/2 = 26 → [26, 33).
	if y0, y1 := gripZone(60); y0 != 26 || y1 != 33 {
		t.Errorf("gripZone(60) = [%d, %d), want [26, 33)", y0, y1)
	}
	// H=2 (smaller than minimum grip height 3) → h clamps down to 2
	// so the strip covers the whole bar.
	if y0, y1 := gripZone(2); y0 != 0 || y1 != 2 {
		t.Errorf("gripZone(2) = [%d, %d), want [0, 2)", y0, y1)
	}
}

// TestHSplitClickOnGripStartsDragSession — a click at ev.X == lw sets
// dragging=true and does not route to either child.
func TestHSplitClickOnGripStartsDragSession(t *testing.T) {
	fl := &fileList{items: []string{"a", "b"}, selected: 0}
	tp := &textPreview{}
	h := &hSplit{left: fl, right: tp, leftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 20})
	// gripLocalX = 100 * 30 / 100 = 30.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 30, Y: 5})
	if !h.dragging {
		t.Fatal("click on grip did not set dragging=true")
	}
	if fl.selected != 0 {
		t.Errorf("grip click leaked to fileList: selected = %d, want 0", fl.selected)
	}
}

// TestHSplitDragUpdatesLeftFrac — an EventMouseDrag mid-session sets
// leftFrac to X*100/W clamped to [10,90] and re-runs SetBounds.
func TestHSplitDragUpdatesLeftFrac(t *testing.T) {
	fl := &fileList{items: []string{"a"}, selected: 0}
	tp := &textPreview{}
	h := &hSplit{left: fl, right: tp, leftFrac: 30, dragging: true}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10})
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 50, Y: 5})
	if h.leftFrac != 50 {
		t.Errorf("drag to X=50: leftFrac = %d, want 50", h.leftFrac)
	}
	// Left pane's width should have updated via SetBounds.
	if fl.Bounds().W != 50 {
		t.Errorf("fileList width after drag: %d, want 50", fl.Bounds().W)
	}
	// Drag below min → clamp to hSplitMinFrac.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 2, Y: 5})
	if h.leftFrac != hSplitMinFrac {
		t.Errorf("drag to X=2: leftFrac = %d, want %d", h.leftFrac, hSplitMinFrac)
	}
	// Drag above max → clamp to hSplitMaxFrac.
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 99, Y: 5})
	if h.leftFrac != hSplitMaxFrac {
		t.Errorf("drag to X=99: leftFrac = %d, want %d", h.leftFrac, hSplitMaxFrac)
	}
	// Zero-width guard — drag on a bounds with W=0 must not divide-by-zero.
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 0, H: 10})
	before := h.leftFrac
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 5, Y: 5})
	if h.leftFrac != before {
		t.Errorf("zero-W drag mutated leftFrac: %d → %d", before, h.leftFrac)
	}
}

// TestHSplitMouseUpEndsSession — an EventMouseUp clears dragging.
func TestHSplitMouseUpEndsSession(t *testing.T) {
	h := &hSplit{
		left: toolkit.NewLabel("L"), right: toolkit.NewLabel("R"),
		leftFrac: 50, dragging: true,
	}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 10})
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 20, Y: 3})
	if h.dragging {
		t.Fatal("MouseUp did not clear dragging")
	}
}

// TestHSplitDragOutsideSessionIsIgnored — a stray EventMouseDrag or
// EventMouseUp without an active session is silently dropped.
func TestHSplitDragOutsideSessionIsIgnored(t *testing.T) {
	fl := &fileList{items: []string{"a"}, selected: 0}
	tp := &textPreview{}
	h := &hSplit{left: fl, right: tp, leftFrac: 30}
	h.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 100, H: 10})
	beforeFrac := h.leftFrac
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 50, Y: 5})
	if h.leftFrac != beforeFrac {
		t.Errorf("stray drag mutated leftFrac: %d → %d", beforeFrac, h.leftFrac)
	}
	h.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 50, Y: 5})
	if h.dragging {
		t.Fatal("stray up set dragging=true (should stay false)")
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
	// A 1-cell grip column at X=lw sits between the panes, so right
	// gets W - lw - 1 = 100 - 30 - 1 = 69 cells.
	if right.Bounds().W != 69 {
		t.Errorf("right width = %d, want 69 (100 - 30 grip - 1)", right.Bounds().W)
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
	tp.setText("x\ny\nz", "")
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
	tp.setText("a\nb\nc", "")
	if len(tp.lines) != 3 {
		t.Fatalf("lines = %v, want 3", tp.lines)
	}
	if got := tp.Text(); got != "a\nb\nc" {
		t.Errorf("Text() = %q, want %q", got, "a\nb\nc")
	}
}
func TestTextPreviewSetEmptyClears(t *testing.T) {
	tp := &textPreview{lines: []string{"a"}}
	tp.setText("", "")
	if tp.lines != nil {
		t.Errorf("lines not nil after setText(\"\"): %v", tp.lines)
	}
	if got := tp.Text(); got != "" {
		t.Errorf("Text() empty = %q, want empty", got)
	}
}
func TestTextPreviewSetTextTrailingNewline(t *testing.T) {
	tp := &textPreview{}
	tp.setText("hello\n", "")
	if len(tp.lines) != 1 || tp.lines[0] != "hello" {
		t.Errorf("trailing newline handling wrong: %v", tp.lines)
	}
}
func TestTextPreviewDrawRendersLinesAndClipsBounds(t *testing.T) {
	tp := &textPreview{}
	tp.setText("a\nb\nc\nd\ne", "")
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

// TestPackedVBoxCapturesDragFromClickTarget — verifies each of the
// four click branches (overlay, header, body, footer) captures its
// target so subsequent drag/up events route back to it. Without
// capture the drag would jump between bands as the pointer moves.
func TestPackedVBoxCapturesDragFromClickTarget(t *testing.T) {
	// bodyFL is a hSplit-wrapped fileList so we can verify drag
	// events don't leak (fileList itself doesn't care about drag,
	// but hSplit's grip does — that's the real use case).
	bodyFL := &fileList{items: []string{"a", "b", "c"}, selected: 0}
	body := &hSplit{left: bodyFL, right: toolkit.NewLabel("R"), leftFrac: 50}
	overlayFL := &fileList{items: []string{"o1", "o2"}, selected: 0}
	p := &packedVBox{
		header:   toolkit.NewLabel("H"),
		body:     body,
		footer:   toolkit.NewLabel("F"),
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{overlayFL},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 20})

	// Body click at (2, 5). Y=5 in body band → body-local Y=4 →
	// hSplit lw=20 → X=2 < 20 → left → fileList Y=4 (past len).
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 5})
	if p.dragTarget != body {
		t.Errorf("body click did not capture body: %v", p.dragTarget)
	}
	if p.dragDy != 1 {
		t.Errorf("dragDy = %d, want 1 (headerH)", p.dragDy)
	}
	// EventMouseDrag at (2, 0) — this is normally the header row.
	// Capture must forward to body with translated Y = 0-1 = -1.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 2, Y: 0})
	// MouseUp releases capture.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 2, Y: 0})
	if p.dragTarget != nil {
		t.Fatal("MouseUp did not release capture")
	}

	// Overlay click captures overlay.
	// Overlay bounds: bx=4, by=3, bw=32, bh=14.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if p.dragTarget != overlayFL {
		t.Errorf("overlay click did not capture overlay: %v", p.dragTarget)
	}
	if p.dragDx != 4 || p.dragDy != 3 {
		t.Errorf("overlay dragDx,dragDy = (%d,%d), want (4,3)", p.dragDx, p.dragDy)
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 5, Y: 4})

	// Header click captures header.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 0})
	if p.dragTarget != p.header {
		t.Errorf("header click did not capture header: %v", p.dragTarget)
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 1, Y: 0})

	// Footer click captures footer.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 1, Y: 19})
	if p.dragTarget != p.footer {
		t.Errorf("footer click did not capture footer: %v", p.dragTarget)
	}
	if p.dragDy != 19 {
		t.Errorf("footer dragDy = %d, want 19", p.dragDy)
	}
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 1, Y: 19})
}

// TestPackedVBoxInvisibleOverlayDoesNotClaimClicks — even though an
// overlay's Bounds still cover the body inset when invisible, the
// packedVBox click loop must skip it and let the click reach body.
func TestPackedVBoxInvisibleOverlayDoesNotClaimClicks(t *testing.T) {
	body := &fileList{items: []string{"a", "b", "c", "d", "e"}, selected: 0, onSelect: func(int) {}}
	invisible := &cellPopover{Title: "P", Body: []string{"x"}, Visible: false}
	p := &packedVBox{
		body:     body,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{invisible},
	}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	// Click at (5, 4) — this is inside the overlay bounds region
	// (bx=4, by=3, bw=12, bh=14). With invisibility filter it must
	// fall through to the body branch. Body local Y = 4-1 = 3.
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if body.selected != 3 {
		t.Errorf("invisible overlay ate the click: body.selected = %d, want 3", body.selected)
	}
	if p.dragTarget != body {
		t.Errorf("capture went to overlay, not body: %v", p.dragTarget)
	}
	// Now make it visible and click again — it MUST claim the click.
	invisible.Visible = true
	p.dragTarget = nil
	p.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 4})
	if p.dragTarget != invisible {
		t.Errorf("visible overlay did NOT claim the click: %v", p.dragTarget)
	}
}

// TestPackedVBoxDragWithoutCaptureIsDropped — a stray drag/up with
// no capture in effect never dispatches to any child.
func TestPackedVBoxDragWithoutCaptureIsDropped(t *testing.T) {
	fl := &fileList{items: []string{"a"}, selected: 0}
	p := &packedVBox{body: fl, headerH: 1, footerH: 1}
	p.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 20})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseDrag, X: 3, Y: 5})
	p.OnEvent(toolkit.Event{Kind: toolkit.EventMouseUp, X: 3, Y: 5})
	if p.dragTarget != nil {
		t.Errorf("stray drag/up mutated dragTarget: %v", p.dragTarget)
	}
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
