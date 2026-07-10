// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// tui-widgets renders the go-widgets/tui CELL-NATIVE widget set (every kind with
// a public constructor) into a scrollable cell-grid frame written to stdout as
// an ANSI stream. Every entry is a tui.* widget that renders one glyph per cell,
// so the gallery reflects how the widgets actually look in a terminal (unlike
// the pixel toolkit widgets, whose geometry does not map to a character grid).
//
// The default frame is auto-sized to fit every entry. Pipe through `less -R` if
// your terminal's scrollback can't hold the whole frame:
//
//	go run . | less -R
//
// Force a specific size + dark theme:
//
//	go run . --cols=90 --rows=120 --theme=dark
//
// Or render one widget at a larger scale:
//
//	go run . --widget=table --cols=50 --rows=8
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit are dependency-injection seams so tests drive main() through
// run() without spawning a subprocess.
var (
	runFunc = run
	osExit  = os.Exit
)

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags and renders the gallery via tui.RenderToolkitSized. Returns 0
// on success, 1 on render error, 2 on flag-parse error, 3 when --widget names an
// entry that doesn't exist.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-widgets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	cols := fs.Int("cols", 0, "terminal width in cells (0 = auto)")
	rows := fs.Int("rows", 0, "terminal height in cells (0 = auto, fits every widget)")
	widget := fs.String("widget", "", "render only the named widget (see --list)")
	list := fs.Bool("list", false, "print widget names one per line and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	all := entries()

	if *list {
		for _, e := range all {
			fmt.Fprintln(stdout, e.name)
		}
		return 0
	}

	th := toolkit.DefaultLight()
	if *theme == "dark" {
		th = toolkit.DefaultDark()
	}

	var widgets []toolkit.Widget
	frameCols, frameRows := 0, 0
	if *widget != "" {
		e, ok := findEntry(all, *widget)
		if !ok {
			fmt.Fprintf(stderr, "tui-widgets: unknown widget %q — use --list to see the full set\n", *widget)
			return 3
		}
		w := e.make()
		w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: e.w, H: e.h})
		widgets = []toolkit.Widget{w}
		frameCols, frameRows = e.w, e.h
	} else {
		widgets, frameCols, frameRows = composeAll(all)
	}
	if *cols > 0 {
		frameCols = *cols
	}
	if *rows > 0 {
		frameRows = *rows
	}

	if err := tui.RenderToolkitSized(stdout, frameCols, frameRows, widgets, th); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// entry describes one widget kind: its name (also the caption above the widget),
// its rendered cell dimensions, and a factory that builds a fresh instance.
type entry struct {
	name string
	w, h int
	make func() toolkit.Widget
}

// findEntry returns the entry with the given name, or false.
func findEntry(all []entry, name string) (entry, bool) {
	for _, e := range all {
		if e.name == name {
			return e, true
		}
	}
	return entry{}, false
}

// Grid layout: two columns of equal slots, sized for cell-native widgets (one
// glyph per cell, so far smaller than the pixel catalogue's slots).
const (
	slotW      = 40
	slotH      = 8
	captionH   = 1
	gridCols   = 2
	slotMargin = 2
)

// composeAll lays out every entry in a 2-column stack and returns the caption +
// widget instances plus a frame size large enough to contain them.
func composeAll(all []entry) (widgets []toolkit.Widget, cols, rows int) {
	rowH := captionH + slotH
	cols = 2*slotW + slotMargin
	nrows := (len(all) + gridCols - 1) / gridCols
	rows = nrows * rowH
	for i, e := range all {
		col := i % gridCols
		row := i / gridCols
		x := col * (slotW + slotMargin)
		y := row * rowH

		caption := toolkit.NewLabel(e.name)
		caption.SetBounds(toolkit.Rect{X: x, Y: y, W: slotW, H: captionH})
		widgets = append(widgets, caption)

		w := e.make()
		wx := x
		if e.w < slotW {
			wx = x + (slotW-e.w)/2
		}
		w.SetBounds(toolkit.Rect{X: wx, Y: y + captionH, W: e.w, H: e.h})
		widgets = append(widgets, w)
	}
	return widgets, cols, rows
}

// entries lists every cell-native widget the gallery renders. tui.MenuDropdown
// is intentionally omitted: it self-positions at its AnchorX/AnchorY and ignores
// the bounds a container hands it, so it cannot sit in the auto-grid — see it in
// action in cmd/tui-editor / cmd/tui-explorer instead (or render any single
// widget with --widget=NAME at a larger scale).
func entries() []entry {
	return []entry{
		{"button", 12, 3, func() toolkit.Widget {
			b := tui.NewButton("OK", nil)
			b.Style = tui.ButtonProminent
			return b
		}},
		{"checkbutton", 20, 1, func() toolkit.Widget {
			return tui.NewCheckButton("Enabled", true)
		}},
		{"radiobutton", 20, 1, func() toolkit.Widget {
			r := tui.NewRadioButton("Selected")
			r.Checked = true
			return r
		}},
		{"entry", 24, 1, func() toolkit.Widget {
			return &tui.Entry{Text: "query.txt", Placeholder: "search"}
		}},
		{"scale", 24, 1, func() toolkit.Widget {
			return tui.NewScale(0, 100, 60)
		}},
		{"progressbar", 24, 1, func() toolkit.Widget {
			return &tui.ProgressBar{Fraction: 0.6, Label: "60%"}
		}},
		{"levelbar", 24, 1, func() toolkit.Widget {
			l := tui.NewLevelBar(5)
			l.Value = 3
			return l
		}},
		{"listbox", 24, 5, func() toolkit.Widget {
			l := tui.NewListBox([]string{"apple", "banana", "cherry", "date"})
			l.Selected = 1
			return l
		}},
		{"treeview", 28, 6, func() toolkit.Widget {
			root := &tui.TreeNode{Label: "/", Expanded: true, Children: []*tui.TreeNode{
				{Label: "src", Expanded: true, Children: []*tui.TreeNode{{Label: "main.go"}, {Label: "util.go"}}},
				{Label: "README.md"},
			}}
			tv := tui.NewTreeView(root)
			tv.Selected = root.Children[0]
			return tv
		}},
		{"table", 38, 5, func() toolkit.Widget {
			cols := []tui.TableColumn{{Title: "Name", Width: 16}, {Title: "Kind"}}
			rows := [][]string{{"main.go", "source"}, {"README.md", "text"}, {"go.mod", "module"}}
			tb := tui.NewTable(cols, rows)
			tb.Selected = 0
			return tb
		}},
		{"menubar", 30, 1, func() toolkit.Widget {
			mb := &tui.MenuBar{}
			mb.Items = []tui.MenuItem{{Label: "File"}, {Label: "Edit"}, {Label: "View"}}
			return mb
		}},
		{"toolbar", 30, 1, func() toolkit.Widget {
			return tui.NewToolbar([]tui.ToolbarItem{
				{Label: "New"}, {Separator: true}, {Label: "Save"}, {Label: "Del", Disabled: true},
			})
		}},
		{"statusbar", 38, 1, func() toolkit.Widget {
			return tui.NewStatusbar([]string{"Ready", "Ln 42", "UTF-8"})
		}},
		{"popover", 28, 6, func() toolkit.Widget {
			return &tui.Popover{Title: "Popover", Body: []string{"a bordered", "overlay box"}, Visible: true}
		}},
		{"notebook", 30, 6, func() toolkit.Widget {
			n := tui.NewNotebook()
			n.AddTab("One", toolkit.NewLabel("page one"))
			n.AddTab("Two", toolkit.NewLabel("page two"))
			return n
		}},
		{"hsplit", 38, 5, func() toolkit.Widget {
			return &tui.HSplit{Left: toolkit.NewLabel("sidebar"), Right: toolkit.NewLabel("content"), LeftFrac: 40}
		}},
		{"vbox", 30, 6, func() toolkit.Widget {
			return &tui.VBox{
				Header: toolkit.NewLabel("header"),
				Body:   toolkit.NewLabel("body"),
				Footer: toolkit.NewLabel("footer"),
				HeaderH: 1, FooterH: 1,
			}
		}},
		{"texteditor", 30, 6, func() toolkit.Widget {
			t := tui.NewTextEditor()
			t.Filename = "main.go"
			t.SetText("func main() {\n\tprintln(\"hi\")\n}")
			t.ReadOnly = true
			return t
		}},
		{"dialog", 34, 6, func() toolkit.Widget {
			d := tui.NewDialog("Confirm", []string{"Save your changes?"}, "Yes", "No")
			d.Visible = true
			return d
		}},
		{"spinner", 16, 1, func() toolkit.Widget {
			return tui.NewSpinner("Loading…")
		}},
		{"dropdown", 16, 1, func() toolkit.Widget {
			return tui.NewDropdown([]string{"UTF-8", "Latin-1", "UTF-16"}, 0)
		}},
		{"vsplit", 30, 6, func() toolkit.Widget {
			return &tui.VSplit{Top: toolkit.NewLabel("top"), Bottom: toolkit.NewLabel("bottom"), TopFrac: 40}
		}},
		{"spinbutton", 10, 1, func() toolkit.Widget { return tui.NewSpinButton(0, 100, 42, 1) }},
		{"expander", 24, 4, func() toolkit.Widget {
			e := tui.NewExpander("Details", toolkit.NewLabel("body line"))
			e.Expanded = true
			return e
		}},
		{"banner", 30, 1, func() toolkit.Widget { return tui.NewBanner("Saved successfully", tui.BannerSuccess) }},
		{"label", 20, 1, func() toolkit.Widget { return &tui.Label{Text: "centered label", Align: tui.AlignCenter} }},
	}
}
