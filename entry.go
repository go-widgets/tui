// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Entry is a cell-native single-line text input: a search box, a command
// prompt, a form field. It edits Text via EventKeyDown (Backspace, Delete,
// ArrowLeft/Right, Home, End, Enter) and EventChar (printable runes), shows a
// muted Placeholder while empty and unfocused, and scrolls horizontally so the
// cursor stays visible in a narrow field. Cursor is a rune index, so multi-byte
// UTF-8 characters move it by one visible column.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Entry struct {
	toolkit.Base
	Text        string
	Placeholder string
	Cursor      int // rune index in [0, len(runes)]
	Focused     bool
	OnChange    func(text string)
	OnSubmit    func(text string)

	scrollX int // first visible rune column; the viewport follows the cursor
}

// NewEntry builds an Entry with initial text and the cursor parked at the end.
func NewEntry(initial string) *Entry {
	return &Entry{Text: initial, Cursor: len([]rune(initial))}
}

// avail is the number of cells available for text: the width minus a 1-cell pad
// on each side (floored at 1).
func (e *Entry) avail() int {
	a := e.Bounds().W - 2
	if a < 1 {
		a = 1
	}
	return a
}

// scrollToCursor keeps the cursor within the visible window.
func (e *Entry) scrollToCursor() {
	a := e.avail()
	if e.Cursor < e.scrollX {
		e.scrollX = e.Cursor
	} else if e.Cursor >= e.scrollX+a {
		e.scrollX = e.Cursor - a + 1
	}
}

// Draw paints the field background, the text (or Placeholder), and — when
// Focused — a reverse-video block cursor.
func (e *Entry) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := e.Bounds()
	e.scrollToCursor()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, theme.Surface)
	a := e.avail()
	runes := []rune(e.Text)
	if len(runes) == 0 && !e.Focused && e.Placeholder != "" {
		toolkit.DrawText(pnt, r.X+1, r.Y, e.Placeholder, LineNumberInk(theme))
	} else {
		end := e.scrollX + a
		if end > len(runes) {
			end = len(runes)
		}
		if e.scrollX < end {
			toolkit.DrawText(pnt, r.X+1, r.Y, string(runes[e.scrollX:end]), theme.OnSurface)
		}
	}
	if e.Focused {
		cx := r.X + 1 + (e.Cursor - e.scrollX)
		if cx >= r.X+1 && cx < r.X+r.W {
			pnt.FillRect(painter.Rect{X: cx, Y: r.Y, W: 1, H: 1}, theme.OnSurface)
		}
	}
}

// OnEvent handles focus + caret placement on click, keyboard navigation and
// editing, and character insertion.
func (e *Entry) OnEvent(ev toolkit.Event) {
	runes := []rune(e.Text)
	switch ev.Kind {
	case toolkit.EventClick:
		e.Focused = true
		c := e.scrollX + (ev.X - 1) // local X, minus the 1-cell pad
		if c < 0 {
			c = 0
		}
		if c > len(runes) {
			c = len(runes)
		}
		e.Cursor = c
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Backspace":
			if e.Cursor > 0 {
				runes = append(runes[:e.Cursor-1], runes[e.Cursor:]...)
				e.Cursor--
				e.setText(string(runes))
			}
		case "Delete":
			if e.Cursor < len(runes) {
				runes = append(runes[:e.Cursor], runes[e.Cursor+1:]...)
				e.setText(string(runes))
			}
		case "ArrowLeft":
			if e.Cursor > 0 {
				e.Cursor--
			}
		case "ArrowRight":
			if e.Cursor < len(runes) {
				e.Cursor++
			}
		case "Home":
			e.Cursor = 0
		case "End":
			e.Cursor = len(runes)
		case "Enter":
			if e.OnSubmit != nil {
				e.OnSubmit(e.Text)
			}
		}
	case toolkit.EventChar:
		ch := []rune(ev.Code)
		if len(ch) == 0 {
			return
		}
		runes = append(runes[:e.Cursor], append(ch, runes[e.Cursor:]...)...)
		e.Cursor += len(ch)
		e.setText(string(runes))
	}
}

// setText updates Text and fires OnChange.
func (e *Entry) setText(s string) {
	e.Text = s
	if e.OnChange != nil {
		e.OnChange(s)
	}
}
