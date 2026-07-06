// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"fmt"
	"io"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// RenderToolkit paints a toolkit.Widget tree into a fresh CellPainter
// sized to [SizeOrDefault], fills the background with theme.Background,
// calls each widget's Draw, and writes the resulting ANSI stream to w.
//
// It is the toolkit-widget counterpart of [RenderOnce] (which renders
// painter.Widget). The two exist side-by-side because toolkit.Theme
// carries fields (SurfaceAlt, OnBackground, Extra) that painter.Theme
// does not, so a toolkit widget cannot be rendered through RenderOnce
// without losing theme surface.
//
// The theme argument may be nil, in which case [toolkit.DefaultLight]
// is used.
func RenderToolkit(w io.Writer, widgets []toolkit.Widget, theme *toolkit.Theme) error {
	cols, rows := SizeOrDefault()
	return RenderToolkitSized(w, cols, rows, widgets, theme)
}

// RenderToolkitSized is like [RenderToolkit] but uses the explicit
// (cols, rows) dimensions instead of querying the environment. Cols
// and rows must both be positive; a non-positive value falls back to
// the corresponding default so callers cannot accidentally produce an
// empty render.
func RenderToolkitSized(w io.Writer, cols, rows int, widgets []toolkit.Widget, theme *toolkit.Theme) error {
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	if theme == nil {
		theme = toolkit.DefaultLight()
	}

	cp := painter.NewCellPainter(cols, rows)
	cp.FillRect(painter.Rect{X: 0, Y: 0, W: cols, H: rows}, toPainterRGBA(theme.Background))
	for _, widget := range widgets {
		widget.Draw(cp, theme)
	}
	if _, err := cp.WriteANSI(w); err != nil {
		return fmt.Errorf("tui: write ANSI: %w", err)
	}
	return nil
}

// toPainterRGBA converts a toolkit RGBA to a painter RGBA. The two
// types have the same fields (R, G, B, A byte) but are distinct
// Go types, so callers can't pass one where the other is expected
// without an explicit conversion.
func toPainterRGBA(c toolkit.RGBA) painter.RGBA {
	return painter.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}
