// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"strconv"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Pagination is a cell-native page selector: "‹ Page/Count ›" with a left
// (previous) and right (next) cap. Clicking a cap — or Left/Right while it has
// input — steps Page within [1, Count] and fires OnChange on a real change.
//
// A toolkit.Widget rendering through painter.Painter (cell grid / RGBA buffer).
type Pagination struct {
	toolkit.Base
	Page, Count int // 1-based
	OnChange    func(page int)
}

// NewPagination builds a pager over count pages (floored at 1) starting at page
// (clamped into range).
func NewPagination(count, page int) *Pagination {
	if count < 1 {
		count = 1
	}
	p := &Pagination{Count: count}
	p.SetPage(page)
	return p
}

// SetPage clamps page to [1, Count] and, on a change, assigns it and fires
// OnChange.
func (p *Pagination) SetPage(page int) {
	if page < 1 {
		page = 1
	}
	if page > p.Count {
		page = p.Count
	}
	if page == p.Page {
		return
	}
	p.Page = page
	if p.OnChange != nil {
		p.OnChange(page)
	}
}

// Draw paints the ‹ / › caps and the centred "Page/Count" label.
func (p *Pagination) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	r := p.Bounds()
	toolkit.DrawText(pnt, r.X, r.Y, "‹", theme.Accent)
	toolkit.DrawText(pnt, r.X+r.W-1, r.Y, "›", theme.Accent)
	label := strconv.Itoa(p.Page) + "/" + strconv.Itoa(p.Count)
	toolkit.DrawText(pnt, r.X+(r.W-len(label))/2, r.Y, label, theme.OnSurface)
}

// OnEvent steps the page on a cap click or a Left/Right key.
func (p *Pagination) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventClick:
		switch {
		case ev.X <= 1:
			p.SetPage(p.Page - 1)
		case ev.X >= p.Bounds().W-2:
			p.SetPage(p.Page + 1)
		}
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Left", "ArrowLeft":
			p.SetPage(p.Page - 1)
		case "Right", "ArrowRight":
			p.SetPage(p.Page + 1)
		}
	}
}
