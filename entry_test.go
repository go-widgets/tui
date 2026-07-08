// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

func vchar(s string) toolkit.Event { return toolkit.Event{Kind: toolkit.EventChar, Code: s} }
func vkey(code string) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code}
}

func TestEntryNewAndEdit(t *testing.T) {
	changes, submits := 0, ""
	e := NewEntry("hi")
	if e.Cursor != 2 {
		t.Fatalf("NewEntry cursor = %d, want 2", e.Cursor)
	}
	e.OnChange = func(s string) { changes++ }
	e.OnSubmit = func(s string) { submits = s }

	// Insert a rune at the caret.
	e.OnEvent(vchar("!"))
	if e.Text != "hi!" || e.Cursor != 3 || changes != 1 {
		t.Fatalf("insert: text=%q cursor=%d changes=%d", e.Text, e.Cursor, changes)
	}
	// Empty EventChar is a no-op.
	e.OnEvent(vchar(""))
	if e.Text != "hi!" {
		t.Errorf("empty char changed text: %q", e.Text)
	}
	// Backspace deletes before the caret; at 0 it is a no-op.
	e.OnEvent(vkey("Backspace"))
	if e.Text != "hi" || e.Cursor != 2 {
		t.Errorf("backspace: text=%q cursor=%d", e.Text, e.Cursor)
	}
	e.OnEvent(vkey("Home"))
	e.OnEvent(vkey("Backspace")) // at col 0, no-op
	if e.Text != "hi" || e.Cursor != 0 {
		t.Errorf("backspace at 0: text=%q cursor=%d", e.Text, e.Cursor)
	}
	// Delete removes the char after the caret; at end it is a no-op.
	e.OnEvent(vkey("Delete"))
	if e.Text != "i" {
		t.Errorf("delete: text=%q", e.Text)
	}
	e.OnEvent(vkey("End"))
	e.OnEvent(vkey("Delete")) // at end, no-op
	if e.Text != "i" {
		t.Errorf("delete at end: text=%q", e.Text)
	}

	// Navigation clamps at both ends.
	e.OnEvent(vkey("ArrowLeft"))
	if e.Cursor != 0 {
		t.Errorf("left: cursor=%d", e.Cursor)
	}
	e.OnEvent(vkey("ArrowLeft")) // at 0, clamp
	e.OnEvent(vkey("ArrowRight"))
	if e.Cursor != 1 {
		t.Errorf("right: cursor=%d", e.Cursor)
	}
	e.OnEvent(vkey("ArrowRight")) // at end, clamp
	if e.Cursor != 1 {
		t.Errorf("right clamp: cursor=%d", e.Cursor)
	}

	// Enter fires OnSubmit; an unknown key is a no-op.
	e.OnEvent(vkey("Enter"))
	if submits != "i" {
		t.Errorf("submit = %q, want i", submits)
	}
	e.OnEvent(vkey("F5"))

	// Enter with no OnSubmit + a mutation with no OnChange must not panic.
	bare := NewEntry("x")
	bare.OnEvent(vkey("Enter"))
	bare.OnEvent(vkey("Backspace"))
	if bare.Text != "" {
		t.Errorf("bare backspace: text=%q", bare.Text)
	}
}

func TestEntryClickPlacesCaret(t *testing.T) {
	e := NewEntry("hello")
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
	// Click at local X=3 -> caret at rune 2 (X-1 past the pad).
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 3, Y: 0})
	if !e.Focused || e.Cursor != 2 {
		t.Fatalf("click: focused=%v cursor=%d, want true/2", e.Focused, e.Cursor)
	}
	// Click before the pad clamps to 0.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 0, Y: 0})
	if e.Cursor != 0 {
		t.Errorf("click at 0: cursor=%d", e.Cursor)
	}
	// Click far right clamps to len.
	e.OnEvent(toolkit.Event{Kind: toolkit.EventClick, X: 99, Y: 0})
	if e.Cursor != 5 {
		t.Errorf("click far right: cursor=%d", e.Cursor)
	}
}

func TestEntryDraw(t *testing.T) {
	mk := func(w, h int) *painter.PixelPainter { return painter.NewPixelPainter(make([]byte, w*h*4), w, h) }

	// Placeholder shown while empty + unfocused.
	e := &Entry{Placeholder: "search…"}
	e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	e.Draw(mk(12, 1), toolkit.DefaultLight())

	// Empty + unfocused + no placeholder -> just the background.
	blank := NewEntry("")
	blank.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	blank.Draw(mk(12, 1), toolkit.DefaultLight())

	// Focused empty -> no placeholder, cursor block drawn.
	fe := NewEntry("")
	fe.Focused = true
	fe.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 1})
	fe.Draw(mk(12, 1), toolkit.DefaultLight())

	// Text longer than the field scrolls: cursor at end pushes scrollX > 0
	// (cursor >= scrollX+avail branch), then Home rewinds it (cursor < scrollX).
	sc := NewEntry("abcdefghij")
	sc.Focused = true
	sc.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 6, H: 1}) // avail = 4
	sc.Draw(mk(6, 1), toolkit.DefaultLight())
	if sc.scrollX == 0 {
		t.Errorf("long text did not scroll: scrollX=%d", sc.scrollX)
	}
	sc.Cursor = 0
	sc.Draw(mk(6, 1), toolkit.DefaultLight())
	if sc.scrollX != 0 {
		t.Errorf("Home did not rewind scroll: scrollX=%d", sc.scrollX)
	}

	// Degenerate 1-cell width: avail floors at 1 and the cursor block falls
	// outside the field (cx >= r.X+r.W), exercising the skip branch.
	tiny := NewEntry("z")
	tiny.Focused = true
	tiny.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 1, H: 1})
	tiny.Draw(mk(1, 1), toolkit.DefaultLight())
}
