// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Dialog is a cell-native modal: a content-sized box centred within its bounds,
// with a Title, Message lines, and a row of action Buttons. It manages focus
// across its own buttons — Left/Right and Tab/Shift+Tab move the focused button,
// Enter activates it, Escape cancels, and a click activates the clicked button.
// OnAction fires with the chosen button's index and label (index -1 on Escape /
// cancel).
//
// Being a modal, the host both adds it to a container's overlays (so it is drawn
// and bounded) AND sets it as App.InputTarget while Visible (so it captures the
// keyboard) — the same pattern the command palette uses.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Dialog struct {
	toolkit.Base
	Title    string
	Message  []string
	Buttons  []string
	Active   int  // focused button index
	Visible  bool
	OnAction func(idx int, label string)
}

// NewDialog builds a Dialog with the given title, message lines, and button
// labels, focusing the first button.
func NewDialog(title string, message []string, buttons ...string) *Dialog {
	return &Dialog{Title: title, Message: message, Buttons: buttons}
}

// HitTest — a hidden dialog claims no clicks (so routing falls through to the
// widgets beneath it).
func (d *Dialog) HitTest(px, py int) bool {
	if !d.Visible {
		return false
	}
	return d.Base.HitTest(px, py)
}

// buttonsWidth is the total cell width of the button row (each label + 2 pad,
// 1-cell gaps between).
func (d *Dialog) buttonsWidth() int {
	total := 0
	for i, b := range d.Buttons {
		total += utf8.RuneCountInString(b) + 2
		if i > 0 {
			total++
		}
	}
	return total
}

// size returns the box's natural (width, height) in cells: the widest of Title /
// Message / button row plus border+pad, and a row each for the title, the
// message lines, a gap and the button row.
func (d *Dialog) size() (int, int) {
	w := utf8.RuneCountInString(d.Title)
	for _, m := range d.Message {
		if l := utf8.RuneCountInString(m); l > w {
			w = l
		}
	}
	if bw := d.buttonsWidth(); bw > w {
		w = bw
	}
	return w + 4, len(d.Message) + 5
}

// box returns the dialog box centred within the widget's bounds, clamped to fit.
func (d *Dialog) box() painter.Rect {
	r := d.Bounds()
	w, h := d.size()
	if w > r.W {
		w = r.W
	}
	if h > r.H {
		h = r.H
	}
	return painter.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h}
}

// buttonRow returns the row and the per-button (x, width) of the centred button
// strip inside box b.
func (d *Dialog) buttonRow(b painter.Rect) (y int, xs, ws []int) {
	y = b.Y + b.H - 2
	x := b.X + (b.W-d.buttonsWidth())/2
	for _, label := range d.Buttons {
		w := utf8.RuneCountInString(label) + 2
		xs = append(xs, x)
		ws = append(ws, w)
		x += w + 1
	}
	return y, xs, ws
}

// Draw paints the box, title, message and button strip (the Active button in
// Accent), when Visible.
func (d *Dialog) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !d.Visible {
		return
	}
	b := d.box()
	pnt.FillRect(b, theme.SurfaceAlt)
	pnt.StrokeRect(b, theme.Border, 1)
	toolkit.DrawText(pnt, b.X+2, b.Y+1, d.Title, theme.OnSurface)
	for i, m := range d.Message {
		toolkit.DrawText(pnt, b.X+2, b.Y+2+i, m, theme.OnSurface)
	}
	y, xs, ws := d.buttonRow(b)
	for i, label := range d.Buttons {
		face, ink := theme.Surface, theme.OnSurface
		if i == d.Active {
			face, ink = theme.Accent, theme.Background
		}
		pnt.FillRect(painter.Rect{X: xs[i], Y: y, W: ws[i], H: 1}, face)
		toolkit.DrawText(pnt, xs[i]+1, y, label, ink)
	}
}

// move shifts the focused button by delta, wrapping.
func (d *Dialog) move(delta int) {
	if n := len(d.Buttons); n > 0 {
		d.Active = ((d.Active+delta)%n + n) % n
	}
}

// activate fires OnAction for button i and hides the dialog.
func (d *Dialog) activate(i int) {
	if d.OnAction != nil && i >= 0 && i < len(d.Buttons) {
		d.OnAction(i, d.Buttons[i])
	}
	d.Visible = false
}

// cancel fires OnAction(-1, "") and hides the dialog.
func (d *Dialog) cancel() {
	if d.OnAction != nil {
		d.OnAction(-1, "")
	}
	d.Visible = false
}

// OnEvent handles button traversal (Left/Right, Tab/Shift+Tab), activation
// (Enter / click) and cancel (Escape).
func (d *Dialog) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "ArrowLeft", "Shift+Tab":
			d.move(-1)
		case "ArrowRight", "Tab":
			d.move(1)
		case "Enter":
			d.activate(d.Active)
		case "Escape":
			d.cancel()
		}
	case toolkit.EventClick:
		y, xs, ws := d.buttonRow(d.box())
		if ev.Y != y {
			return
		}
		for i := range d.Buttons {
			if ev.X >= xs[i] && ev.X < xs[i]+ws[i] {
				d.Active = i
				d.activate(i)
				return
			}
		}
	}
}
