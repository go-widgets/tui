// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// sampleTree builds:
//
//	root (expanded)
//	├─ a (expanded)  ├─ a1  └─ a2
//	├─ b (collapsed) └─ b1   (hidden)
//	└─ c (leaf)
//
// visible flatten = [root, a, a1, a2, b, c].
func sampleTree() *TreeNode {
	a := &TreeNode{Label: "a", Expanded: true, Children: []*TreeNode{{Label: "a1"}, {Label: "a2"}}}
	b := &TreeNode{Label: "b", Children: []*TreeNode{{Label: "b1"}}}
	c := &TreeNode{Label: "c"}
	return &TreeNode{Label: "root", Expanded: true, Children: []*TreeNode{a, b, c}}
}

func mkP(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }

func TestTreeViewFlatten(t *testing.T) {
	tv := NewTreeView(sampleTree())
	tv.flatten()
	got := make([]string, len(tv.rows))
	for i, r := range tv.rows {
		got[i] = r.node.Label
	}
	want := []string{"root", "a", "a1", "a2", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("flatten = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flatten[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Nil root flattens to nothing.
	empty := NewTreeView(nil)
	empty.flatten()
	if len(empty.rows) != 0 {
		t.Errorf("nil-root flatten = %v", empty.rows)
	}
}

func TestTreeViewDrawAndScroll(t *testing.T) {
	tv := NewTreeView(sampleTree())
	tv.Selected = tv.Root.Children[2] // "c", the last visible row
	tv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 2})
	tv.Draw(mkP(20, 2), toolkit.DefaultLight())
	// c is row 5; with H=2 the viewport scrolls to keep it visible.
	if tv.scrollY != 4 {
		t.Fatalf("scroll to last: scrollY=%d, want 4", tv.scrollY)
	}
	// Selecting the root rewinds the scroll (i < scrollY branch).
	tv.Selected = tv.Root
	tv.Draw(mkP(20, 2), toolkit.DefaultLight())
	if tv.scrollY != 0 {
		t.Fatalf("scroll to root: scrollY=%d, want 0", tv.scrollY)
	}
	// No selection: scrollToSel is a no-op (index -1).
	tv.Selected = nil
	tv.scrollY = 3
	tv.Draw(mkP(20, 2), toolkit.DefaultLight())
	if tv.scrollY != 3 {
		t.Errorf("nil selection moved scroll: %d", tv.scrollY)
	}
	// Tall pane draws every row (no break), covering leaf + both chevrons.
	tv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 10})
	tv.Draw(mkP(20, 10), toolkit.DefaultLight())

	// Zero-height bounds floor the viewport at 1 row (h < 1 guard).
	tv.Selected = tv.Root.Children[2]
	tv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 0})
	tv.Draw(mkP(20, 1), toolkit.DefaultLight())
}

func TestTreeViewClick(t *testing.T) {
	activated := ""
	tv := NewTreeView(sampleTree())
	tv.OnActivate = func(n *TreeNode) { activated = n.Label }
	tv.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 10})
	tv.flatten()

	// Click "a"'s chevron (row 1, depth 1 -> chevX=2) toggles Expanded.
	a := tv.Root.Children[0]
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 1})
	if a.Expanded {
		t.Fatalf("chevron click did not collapse a")
	}
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 2, Y: 1})
	if !a.Expanded {
		t.Fatalf("chevron click did not re-expand a")
	}

	// Click "a"'s label (past the chevron) selects + activates it.
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 1})
	if tv.Selected != a || activated != "a" {
		t.Fatalf("label click: selected=%v activated=%q", tv.Selected, activated)
	}
	// Click a leaf row ("a1", row 2) selects it (no chevron branch).
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 5, Y: 2})
	if tv.Selected.Label != "a1" {
		t.Errorf("leaf click selected %q", tv.Selected.Label)
	}
	// Out-of-range clicks are no-ops.
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: -1})
	tv.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 99})

	// A nil OnActivate handler must not panic.
	bare := NewTreeView(sampleTree())
	bare.flatten()
	bare.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 10, Y: 0})
}

func TestTreeViewKeys(t *testing.T) {
	activated := ""
	tv := NewTreeView(sampleTree())
	tv.OnActivate = func(n *TreeNode) { activated = n.Label }
	tv.flatten()

	// Down with no selection seeds the first row.
	tv.OnEvent(vkey("Down"))
	if tv.Selected != tv.Root {
		t.Fatalf("seed selection = %v, want root", tv.Selected)
	}
	// Down advances; Up rewinds.
	tv.OnEvent(vkey("Down"))
	if tv.Selected.Label != "a" {
		t.Fatalf("down = %q, want a", tv.Selected.Label)
	}
	tv.OnEvent(vkey("Up"))
	if tv.Selected != tv.Root {
		t.Fatalf("up = %q, want root", tv.Selected.Label)
	}
	// Up at the top is a no-op (j < 0).
	tv.OnEvent(vkey("Up"))
	if tv.Selected != tv.Root {
		t.Errorf("up at top moved: %q", tv.Selected.Label)
	}

	// Right expands root (already expanded -> no-op path first): collapse it,
	// then Right re-expands.
	tv.OnEvent(vkey("Left")) // root expanded -> collapse
	if tv.Root.Expanded {
		t.Fatalf("Left did not collapse root")
	}
	tv.OnEvent(vkey("Right")) // collapsed -> expand
	if !tv.Root.Expanded {
		t.Fatalf("Right did not expand root")
	}
	tv.OnEvent(vkey("Right")) // already expanded -> no-op guard
	tv.flatten()

	// Down to bottom then Down again is a no-op (j >= len).
	tv.Selected = tv.Root.Children[2] // "c", last row
	tv.OnEvent(vkey("Down"))
	if tv.Selected.Label != "c" {
		t.Errorf("down at bottom moved: %q", tv.Selected.Label)
	}

	// Left/Right on a leaf are no-ops (no children).
	tv.Selected = tv.Root.Children[0].Children[0] // "a1"
	tv.OnEvent(vkey("Left"))
	tv.OnEvent(vkey("Right"))

	// Enter activates the selection.
	tv.OnEvent(vkey("Enter"))
	if activated != "a1" {
		t.Errorf("Enter activated %q, want a1", activated)
	}
	// Enter with no selection / no handler is a no-op.
	tv.Selected = nil
	tv.OnEvent(vkey("Enter"))
	nn := NewTreeView(sampleTree())
	nn.Selected = nn.Root
	nn.OnEvent(vkey("Enter")) // nil OnActivate

	// An unrelated key is a no-op; navigating an empty tree is safe.
	tv.OnEvent(vkey("F1"))
	NewTreeView(nil).OnEvent(vkey("Down"))
}
