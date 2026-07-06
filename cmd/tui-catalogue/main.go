// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// tui-catalogue renders a two-column showcase of toolkit widgets to
// stdout as an ANSI stream. It is the tui analogue of
// svg/cmd/gallery-render — instead of writing SVG+PNG snapshots
// per widget, it composes a compact selection of widgets side-by-
// side into a cell-grid frame the size of the current terminal (or
// caller-supplied --cols/--rows).
//
// Run with:
//
//	go run . | cat            # honours $COLUMNS / $LINES if set
//	COLUMNS=100 LINES=32 go run .
//	go run . --cols=120 --rows=40 --theme=dark
//
// Distinct from cmd/tui-snapshot which showcases the tiny
// painter.Widget prototype set (Button, Label, ProgressBar). This
// one uses tui.RenderToolkit to render the real toolkit widget
// catalogue against the cell backend — same widgets a wasm gallery
// or a native window would show, sized down to a terminal frame.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit are dependency-injection seams so tests drive
// main() through run() without spawning a subprocess.
var (
	runFunc = run
	osExit  = os.Exit
)

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags and dispatches to tui.RenderToolkit. Returns 0
// on success, 1 on render error, 2 on flag-parse error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-catalogue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	cols := fs.Int("cols", 0, "terminal width in cells (0 = auto from $COLUMNS)")
	rows := fs.Int("rows", 0, "terminal height in cells (0 = auto from $LINES)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	th := toolkit.DefaultLight()
	if *theme == "dark" {
		th = toolkit.DefaultDark()
	}
	widgets := composeCatalogue()

	var err error
	if *cols > 0 || *rows > 0 {
		err = tui.RenderToolkitSized(stdout, *cols, *rows, widgets, th)
	} else {
		err = tui.RenderToolkit(stdout, widgets, th)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// composeCatalogue builds the widget slice that populates the two-
// column catalogue. Kept in a separate function so tests can assert
// on it independently of the flag-parsing / theme selection
// machinery.
//
// Layout: 100-cell surface split into a left column (cells 2..48)
// and a right column (cells 52..98). Ten widgets rendered top-to-
// bottom, one per row block. Every kind covers a distinct Draw
// codepath — button (accent fill), entry (text ink on Surface),
// alert (semantic accent banner), stat (double-strike Value fake-
// bold), etc. — so the catalogue is a real "does the cell backend
// render every important widget" check rather than a decoration.
func composeCatalogue() []toolkit.Widget {
	widgets := []toolkit.Widget{}

	// Left column ---------------------------------------------------
	title1 := toolkit.NewLabel("BUTTONS")
	title1.SetBounds(toolkit.Rect{X: 2, Y: 1, W: 40, H: 1})
	widgets = append(widgets, title1)

	btn := toolkit.NewButton("Save", nil)
	btn.SetBounds(toolkit.Rect{X: 2, Y: 2, W: 12, H: 3})
	widgets = append(widgets, btn)

	tog := toolkit.NewToggleButton("Muted", true)
	tog.SetBounds(toolkit.Rect{X: 16, Y: 2, W: 12, H: 3})
	widgets = append(widgets, tog)

	sw := toolkit.NewSwitch(true)
	sw.SetBounds(toolkit.Rect{X: 30, Y: 3, W: 8, H: 2})
	widgets = append(widgets, sw)

	title2 := toolkit.NewLabel("INPUT")
	title2.SetBounds(toolkit.Rect{X: 2, Y: 6, W: 40, H: 1})
	widgets = append(widgets, title2)

	entry := toolkit.NewEntry("query text")
	entry.SetBounds(toolkit.Rect{X: 2, Y: 7, W: 40, H: 3})
	widgets = append(widgets, entry)

	title3 := toolkit.NewLabel("FEEDBACK")
	title3.SetBounds(toolkit.Rect{X: 2, Y: 11, W: 40, H: 1})
	widgets = append(widgets, title3)

	pb := toolkit.NewProgressBar()
	pb.Fraction = 0.62
	pb.SetBounds(toolkit.Rect{X: 2, Y: 12, W: 40, H: 2})
	widgets = append(widgets, pb)

	alert := toolkit.NewAlert("Deployed successfully.", toolkit.AlertSuccess)
	alert.SetBounds(toolkit.Rect{X: 2, Y: 15, W: 40, H: 3})
	widgets = append(widgets, alert)

	// Right column --------------------------------------------------
	title4 := toolkit.NewLabel("DATA")
	title4.SetBounds(toolkit.Rect{X: 52, Y: 1, W: 40, H: 1})
	widgets = append(widgets, title4)

	stat := toolkit.NewStat("Requests / min", "12,845")
	stat.Change = "+8.3%"
	stat.Trend = toolkit.StatUp
	stat.SetBounds(toolkit.Rect{X: 52, Y: 2, W: 40, H: 5})
	widgets = append(widgets, stat)

	title5 := toolkit.NewLabel("TIMELINE")
	title5.SetBounds(toolkit.Rect{X: 52, Y: 8, W: 40, H: 1})
	widgets = append(widgets, title5)

	timeline := toolkit.NewTimeline([]toolkit.TimelineEvent{
		{Title: "PR opened", Kind: toolkit.TimelineDefault},
		{Title: "Reviewed", Detail: "LGTM", Kind: toolkit.TimelineSuccess},
		{Title: "Build failed", Kind: toolkit.TimelineError},
	})
	timeline.SetBounds(toolkit.Rect{X: 52, Y: 9, W: 40, H: 8})
	widgets = append(widgets, timeline)

	return widgets
}
