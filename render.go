// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"fmt"
	"io"

	"github.com/go-widgets/painter"
)

// RenderOnce paints the widget tree into a fresh CellPainter sized to
// [SizeOrDefault], fills the background with theme.Background, calls
// each widget's Draw, and writes the resulting ANSI stream to w. It is
// the snapshot form: no raw mode, no event loop; one frame then return.
//
// The theme argument may be nil, in which case [painter.LightTheme] is
// used. This keeps CLI callers ergonomic (`tui.RenderOnce(os.Stdout,
// widgets, nil)`) without forcing them to import painter just to name
// the default theme.
func RenderOnce(w io.Writer, widgets []painter.Widget, theme *painter.Theme) error {
	cols, rows := SizeOrDefault()
	return RenderOnceSized(w, cols, rows, widgets, theme)
}

// RenderOnceSized is like [RenderOnce] but uses the explicit (cols,
// rows) dimensions instead of querying the environment. Cols and rows
// must both be positive; a non-positive value falls back to the
// corresponding default so callers cannot accidentally produce an
// empty render.
func RenderOnceSized(w io.Writer, cols, rows int, widgets []painter.Widget, theme *painter.Theme) error {
	if cols <= 0 {
		cols = DefaultCols
	}
	if rows <= 0 {
		rows = DefaultRows
	}
	if theme == nil {
		theme = painter.LightTheme()
	}

	cp := painter.NewCellPainter(cols, rows)
	cp.FillRect(painter.Rect{X: 0, Y: 0, W: cols, H: rows}, theme.Background)
	for _, widget := range widgets {
		widget.Draw(cp, theme)
	}
	if _, err := cp.WriteANSI(w); err != nil {
		return fmt.Errorf("tui: write ANSI: %w", err)
	}
	return nil
}
