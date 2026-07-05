// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// tui-snapshot renders a sample widget tree to stdout as an ANSI
// stream. It is the tui analogue of painter/cmd/tui-demo — the same
// three-widget layout, sized to the current terminal (or a
// caller-supplied --cols/--rows), so pointing a terminal at its
// output shows the tui package end-to-end without any raw-mode or
// event-loop scaffolding.
//
// Run with:
//
//	go run . | cat            # honours $COLUMNS / $LINES if set
//	COLUMNS=80 LINES=24 go run .
//	go run . --cols=100 --rows=20 --theme=dark
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/tui"
)

// runFunc / osExit are dependency-injection seams so tests can drive
// main()'s success and error branches without spawning a subprocess.
var (
	runFunc = run
	osExit  = os.Exit
)

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run splits from main so tests can drive it deterministically. Returns
// 0 on success, non-zero on flag-parse or render error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	cols := fs.Int("cols", 0, "terminal width in cells (0 = auto)")
	rows := fs.Int("rows", 0, "terminal height in cells (0 = auto)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	th := painter.LightTheme()
	if *theme == "dark" {
		th = painter.DarkTheme()
	}

	widgets := []painter.Widget{
		&painter.Label{
			Bounds: painter.Rect{X: 2, Y: 1, W: 40, H: 1},
			Text:   "GO WIDGETS TUI",
		},
		&painter.Button{
			Bounds: painter.Rect{X: 2, Y: 3, W: 12, H: 3},
			Label:  "OK",
		},
		&painter.Button{
			Bounds:  painter.Rect{X: 16, Y: 3, W: 14, H: 3},
			Label:   "CANCEL",
			Pressed: true,
		},
		&painter.ProgressBar{
			Bounds: painter.Rect{X: 2, Y: 8, W: 40, H: 3},
			Value:  0.72,
		},
	}

	var err error
	if *cols > 0 || *rows > 0 {
		err = tui.RenderOnceSized(stdout, *cols, *rows, widgets, th)
	} else {
		err = tui.RenderOnce(stdout, widgets, th)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
