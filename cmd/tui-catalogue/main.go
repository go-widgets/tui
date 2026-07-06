// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// tui-catalogue renders the complete go-widgets/toolkit widget
// catalogue (every kind with a public constructor) into a scrollable
// cell-grid frame written to stdout as an ANSI stream. Companion to
// svg/cmd/gallery-render: same "one entry per widget kind" pattern
// applied to the terminal backend via tui.RenderToolkit.
//
// The default frame is 120 cells wide and however tall the entries()
// list requires (roughly 4 × ceil(N/2) rows for N widgets). Pipe
// through `less -R` if your terminal's scrollback can't hold the
// whole frame:
//
//	go run . | less -R
//
// Force a specific size + dark theme:
//
//	go run . --cols=140 --rows=200 --theme=dark
//
// Or narrow the selection with --widget=NAME to render one widget at
// a time (useful to eyeball a single kind at a larger scale):
//
//	go run . --widget=alert --cols=60 --rows=6
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
// on success, 1 on render error, 2 on flag-parse error, 3 when
// --widget names an entry that doesn't exist.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-catalogue", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	cols := fs.Int("cols", 0, "terminal width in cells (0 = auto default 120)")
	rows := fs.Int("rows", 0, "terminal height in cells (0 = auto default fits every widget)")
	widget := fs.String("widget", "", "render only the named widget (e.g. 'alert'). See --list for the full name set")
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
			fmt.Fprintf(stderr, "tui-catalogue: unknown widget %q — use --list to see the full set\n", *widget)
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

// entry describes one widget kind in the catalogue: its name (which
// doubles as the caption label above the widget when composed into
// the grid), its rendered cell dimensions, and a factory that builds
// a fresh instance. Mirrors svg/cmd/gallery-render's `entry` struct.
type entry struct {
	name string
	w, h int
	make func() toolkit.Widget
}

// findEntry returns the entry with the given name, or false if not
// found.
func findEntry(all []entry, name string) (entry, bool) {
	for _, e := range all {
		if e.name == name {
			return e, true
		}
	}
	return entry{}, false
}

// Grid layout constants. Two columns of equal slots, each cell
// margin gap on the right. Widget bounds may be smaller than the
// slot; the caption always spans the full slot width.
//
// slotH is deliberately generous (20 cells): toolkit widgets carry
// pad constants (AlertPadY = 8, CardPadY = 6 …) that were tuned for
// the pixel backend. In cell mode the same integer values are
// interpreted as CELL counts, so a widget with an 8-cell top pad
// plus a 7-cell glyph height needs at least 16 rows to render its
// text inside its bounds. Undersize the widget and the text bleeds
// into the next slot.
const (
	slotW      = 58
	slotH      = 20
	captionH   = 1
	gridCols   = 2
	slotMargin = 2
)

// composeAll lays out every catalogue entry in a 2-column stack:
// column 0 at x = 0, column 1 at x = slotW + slotMargin. Row height
// is captionH + slotH. Returns the full widget list (captions AND
// widget instances) plus the frame dimensions large enough to
// contain the composition — so callers can pass those dimensions to
// tui.RenderToolkitSized when they don't override with --cols/--rows.
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

// entries lists every widget kind the catalogue renders. The
// selection matches svg/cmd/gallery-render (which ships 64 entries)
// modulo two exclusions:
//
//   - image — a bitmap render (checker pattern) that doesn't map
//     cleanly to the ANSI cell backend at small sizes.
//   - filechooser — needs a wider slot than the two-column layout
//     provides + a live TreeView activation to populate its list
//     pane meaningfully; renders as an empty frame in a snapshot.
//
// Both are still reachable via --widget=NAME if a caller wants a
// large-size single-widget render.
func entries() []entry {
	return []entry{
		// Pre-v0.7 core
		{"button", 40, 12, func() toolkit.Widget {
			return toolkit.NewButton("Click me", nil)
		}},
		{"label", 40, 8, func() toolkit.Widget {
			return &toolkit.Label{Text: "label text"}
		}},
		{"entry", 40, 12, func() toolkit.Widget {
			return toolkit.NewEntry("editable text")
		}},
		{"checkbutton", 40, 10, func() toolkit.Widget {
			return toolkit.NewCheckButton("Enable feature", true)
		}},
		{"progressbar", 40, 12, func() toolkit.Widget {
			p := toolkit.NewProgressBar()
			p.Fraction = 0.66
			return p
		}},
		{"listbox", 40, 18, func() toolkit.Widget {
			return toolkit.NewListBox([]string{"apple", "banana", "cherry", "date"})
		}},
		{"dropdown", 40, 12, func() toolkit.Widget {
			return toolkit.NewDropDown([]string{"UTF-8", "Latin-1"}, 0)
		}},
		{"expander", 40, 16, func() toolkit.Widget {
			body := toolkit.NewLabel("expanded body")
			e := toolkit.NewExpander("Details", body)
			e.Expanded = true
			return e
		}},
		{"treeview", 40, 18, func() toolkit.Widget {
			root := &toolkit.TreeNode{Label: "/", Expanded: true, Children: []*toolkit.TreeNode{
				{Label: "src"}, {Label: "README.md"},
			}}
			return toolkit.NewTreeView(root)
		}},
		{"radiobutton", 40, 10, func() toolkit.Widget {
			r := toolkit.NewRadioButton("Enable option")
			r.Checked = true
			return r
		}},
		{"togglebutton", 40, 12, func() toolkit.Widget {
			return toolkit.NewToggleButton("Muted", true)
		}},
		{"spinbutton", 40, 12, func() toolkit.Widget {
			return toolkit.NewSpinButton(0, 100, 42, 1)
		}},
		{"statusbar", 55, 10, func() toolkit.Widget {
			return toolkit.NewStatusbar([]string{"Ready", "Line 42", "UTF-8"})
		}},
		{"textview", 40, 16, func() toolkit.Widget {
			return toolkit.NewTextView("First line.\nSecond line.\nThird line.")
		}},
		{"notification", 40, 14, func() toolkit.Widget {
			n := toolkit.NewNotification("Saved successfully")
			n.Visible = true
			return n
		}},
		{"tooltip", 30, 10, func() toolkit.Widget {
			t := toolkit.NewTooltip("Undo (Ctrl+Z)")
			t.Visible = true
			return t
		}},
		// v0.7 (Wave 1)
		{"switch", 20, 10, func() toolkit.Widget {
			return toolkit.NewSwitch(true)
		}},
		{"badge", 20, 10, func() toolkit.Widget {
			return toolkit.NewBadge("42")
		}},
		{"kbd", 25, 12, func() toolkit.Widget {
			return toolkit.NewKbd("Ctrl+K")
		}},
		{"alert", 55, 16, func() toolkit.Widget {
			return toolkit.NewAlert("Configuration saved.", toolkit.AlertSuccess)
		}},
		{"card", 45, 20, func() toolkit.Widget {
			return toolkit.NewCard("Card title", "Body line", "footer")
		}},
		{"breadcrumbs", 50, 10, func() toolkit.Widget {
			return toolkit.NewBreadcrumbs([]string{"home", "projects", "widgets"})
		}},
		{"steps", 55, 20, func() toolkit.Widget {
			return toolkit.NewSteps([]string{"Plan", "Build", "Test", "Ship"}, 1)
		}},
		{"headerbar", 55, 16, func() toolkit.Widget {
			h := toolkit.NewHeaderBar("Files")
			h.Subtitle = "~/Documents"
			return h
		}},
		{"table", 55, 20, func() toolkit.Widget {
			cols := []toolkit.TableColumn{
				{Title: "Name", Width: 20}, {Title: "Kind"},
			}
			rows := [][]string{{"main.go", "source"}, {"README.md", "text"}}
			return toolkit.NewTable(cols, rows)
		}},
		// v0.8 (Wave 2)
		{"avatar", 20, 10, func() toolkit.Widget {
			return toolkit.NewAvatar("DL")
		}},
		{"skeleton", 40, 16, func() toolkit.Widget {
			return toolkit.NewSkeleton(toolkit.SkeletonText, 3)
		}},
		{"rating", 30, 10, func() toolkit.Widget {
			return toolkit.NewRating(3, 5)
		}},
		{"toast", 50, 12, func() toolkit.Widget {
			t := toolkit.NewToast("Copied to clipboard", toolkit.ToastSuccess)
			t.Visible = true
			return t
		}},
		{"banner", 55, 12, func() toolkit.Widget {
			b := toolkit.NewBanner("Update available.")
			b.ButtonLabel = "Install"
			return b
		}},
		{"popover", 40, 18, func() toolkit.Widget {
			child := toolkit.NewLabel("Popover content")
			p := toolkit.NewPopover(child)
			p.Title = "Menu"
			p.Visible = true
			return p
		}},
		{"actionrow", 55, 16, func() toolkit.Widget {
			a := toolkit.NewActionRow("Language")
			a.Subtitle = "English (US)"
			return a
		}},
		{"viewswitcher", 55, 12, func() toolkit.Widget {
			return toolkit.NewViewSwitcher([]string{"Inbox", "Sent", "Archive"}, 0)
		}},
		{"chatbubble", 40, 12, func() toolkit.Widget {
			return toolkit.NewChatBubble("Hello, world!", toolkit.ChatFromUser)
		}},
		{"searchentry", 40, 12, func() toolkit.Widget {
			return toolkit.NewSearchEntry("query")
		}},
		{"diff", 55, 18, func() toolkit.Widget {
			return toolkit.NewDiff([]toolkit.DiffLine{
				{Text: "package main", Kind: toolkit.DiffContext},
				{Text: "old line", Kind: toolkit.DiffRemoved},
				{Text: "new line", Kind: toolkit.DiffAdded},
			})
		}},
		{"pagination", 55, 12, func() toolkit.Widget {
			return toolkit.NewPagination(2, 5)
		}},
		// v0.9 (Wave 3)
		{"splitbutton", 35, 12, func() toolkit.Widget {
			return toolkit.NewSplitButton("Deploy", nil)
		}},
		{"iconbutton", 20, 12, func() toolkit.Widget {
			return toolkit.NewIconButton("+", nil)
		}},
		{"stat", 40, 20, func() toolkit.Widget {
			s := toolkit.NewStat("Requests / min", "12,845")
			s.Change = "+8.3%"
			s.Trend = toolkit.StatUp
			return s
		}},
		{"timeline", 50, 20, func() toolkit.Widget {
			return toolkit.NewTimeline([]toolkit.TimelineEvent{
				{Title: "PR opened", Kind: toolkit.TimelineDefault},
				{Title: "Reviewed", Detail: "LGTM", Kind: toolkit.TimelineSuccess},
			})
		}},
		{"dropzone", 50, 18, func() toolkit.Widget {
			return toolkit.NewDropZone("Drop files here")
		}},
		{"chip", 30, 12, func() toolkit.Widget {
			c := toolkit.NewChip("frontend")
			c.Closable = true
			return c
		}},
		{"formfield", 45, 20, func() toolkit.Widget {
			e := toolkit.NewEntry("value")
			f := toolkit.NewFormField("Username", e)
			f.Help = "min 3 chars"
			return f
		}},
		{"progresscircle", 25, 20, func() toolkit.Widget {
			p := toolkit.NewProgressCircle()
			p.Fraction = 0.66
			return p
		}},
		// v0.9.1 catalogue-completion (pre-v0.7 widgets that were
		// missing from cmd/gallery-render's entries())
		{"calendar", 55, 20, func() toolkit.Widget {
			c := toolkit.NewCalendar(2026, 7, 6)
			c.SetToday(2026, 7, 6)
			return c
		}},
		{"colorchooser", 55, 20, func() toolkit.Widget {
			return toolkit.NewColorChooser(toolkit.RGB(0x0d, 0x94, 0x88))
		}},
		{"scale", 45, 10, func() toolkit.Widget {
			return toolkit.NewScale(0, 100, 65)
		}},
		{"levelbar", 45, 10, func() toolkit.Widget {
			l := toolkit.NewLevelBar(10)
			l.Value = 7
			return l
		}},
		{"spinner", 25, 12, func() toolkit.Widget {
			s := toolkit.NewSpinner()
			s.Active = true
			return s
		}},
		{"notebook", 55, 20, func() toolkit.Widget {
			n := toolkit.NewNotebook()
			n.AddTab("One", toolkit.NewLabel("first"))
			n.AddTab("Two", toolkit.NewLabel("second"))
			n.AddTab("Three", toolkit.NewLabel("third"))
			return n
		}},
		{"menubar", 55, 10, func() toolkit.Widget {
			m := toolkit.NewMenuBar()
			m.Names = []string{"File", "Edit", "View", "Help"}
			m.Menus = []*toolkit.Menu{
				toolkit.NewMenu([]toolkit.MenuItem{{Label: "New"}}),
				toolkit.NewMenu([]toolkit.MenuItem{{Label: "Copy"}}),
				toolkit.NewMenu([]toolkit.MenuItem{{Label: "Zoom in"}}),
				toolkit.NewMenu([]toolkit.MenuItem{{Label: "About"}}),
			}
			return m
		}},
		{"menu", 40, 18, func() toolkit.Widget {
			return toolkit.NewMenu([]toolkit.MenuItem{
				{Label: "New"}, {Label: "Open"}, {Separator: true}, {Label: "Quit"},
			})
		}},
		{"dialog", 55, 20, func() toolkit.Widget {
			ok := toolkit.NewButton("OK", nil)
			body := toolkit.NewLabel("dialog body")
			return toolkit.NewDialog("Confirm", body, ok)
		}},
		{"messagedialog", 55, 20, func() toolkit.Widget {
			return toolkit.NewMessageDialog("Notice", "Operation completed.", nil)
		}},
		{"frame", 45, 16, func() toolkit.Widget {
			body := toolkit.NewLabel("framed content")
			return toolkit.NewFrame(body)
		}},
		{"hbox", 55, 10, func() toolkit.Widget {
			h := toolkit.NewHBox()
			h.Spacing = 4
			h.Append(toolkit.NewLabel("left"))
			h.Append(toolkit.NewLabel("mid"))
			h.Append(toolkit.NewLabel("right"))
			return h
		}},
		{"vbox", 40, 18, func() toolkit.Widget {
			v := toolkit.NewVBox()
			v.Spacing = 1
			v.Append(toolkit.NewLabel("top"))
			v.Append(toolkit.NewLabel("mid"))
			v.Append(toolkit.NewLabel("bottom"))
			return v
		}},
		{"grid", 40, 14, func() toolkit.Widget {
			g := toolkit.NewGrid(2, 2)
			g.Attach(toolkit.NewLabel("a1"), 0, 0)
			g.Attach(toolkit.NewLabel("b1"), 1, 0)
			g.Attach(toolkit.NewLabel("a2"), 0, 1)
			g.Attach(toolkit.NewLabel("b2"), 1, 1)
			return g
		}},
		{"hpaned", 55, 14, func() toolkit.Widget {
			return toolkit.NewHPaned(toolkit.NewLabel("left"), toolkit.NewLabel("right"))
		}},
		{"vpaned", 40, 18, func() toolkit.Widget {
			return toolkit.NewVPaned(toolkit.NewLabel("top"), toolkit.NewLabel("bottom"))
		}},
		{"scrollview", 45, 18, func() toolkit.Widget {
			body := toolkit.NewTextView("Line 1\nLine 2\nLine 3\nLine 4\nLine 5")
			body.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 30})
			return toolkit.NewScrollView(body)
		}},
	}
}
