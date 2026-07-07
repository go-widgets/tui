// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

// tui-explorer is the reference interactive demo for tui.App: a small
// k9s-style file-browser mockup composed from toolkit widgets. The
// layout is
//
//	+-----------------------------------------------------+
//	| MenuBar: File / View / Help                         |
//	+----------------------+------------------------------+
//	| TreeView             | Notebook: content | info     |
//	| (3-level fixed FS)   |                              |
//	|                      |                              |
//	+----------------------+------------------------------+
//	| Statusbar: q: quit  ?: help  /: search  ...         |
//	+-----------------------------------------------------+
//
// Keys wired into App.Keys:
//
//	q, Ctrl+C  → Quit
//	?          → toggle help popover
//	/          → toggle search popover
//	Enter      → sync selected tree node into the notebook body
package main

import (
	"flag"
	"io"
	"os"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
	"github.com/go-widgets/tui"
)

// runFunc / osExit / newAppFunc are dependency-injection seams so
// tests drive main() through run() without spawning a subprocess or
// entering an event loop. Tests override newAppFunc + runAppFunc to
// short-circuit the interactive event loop; production reaches the
// real tui.App via the defaults.
var (
	runFunc    = run
	osExit     = os.Exit
	newAppFunc = tui.NewApp
	runAppFunc = defaultRunApp
)

// defaultRunApp is the production runAppFunc: hand off to tui.App's
// event loop. Named (rather than an inline closure) so its return
// statement is a testable function tests can cover directly.
func defaultRunApp(a *tui.App) int { return a.Run() }

func main() {
	osExit(runFunc(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags (--theme=light|dark), composes the demo, installs
// the key bindings, and hands control to App.Run. Returns 0 on
// clean exit, 2 on flag-parse error, and whatever App.Run returns on
// event-loop error.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tui-explorer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	theme := fs.String("theme", "light", "theme (light|dark)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st := newState()
	app := newAppFunc()
	app.Root = st.root
	if *theme == "dark" {
		app.Theme = toolkit.DefaultDark()
	} else {
		app.Theme = toolkit.DefaultLight()
	}
	for k, h := range st.keys() {
		app.Keys[k] = h
	}
	return runAppFunc(app)
}

// state bundles every mutable widget the demo builds so key handlers
// can address them by field name without threading a container tree.
type state struct {
	fileList      *fileList
	content       *tui.TextEditor
	status        *toolkit.Label
	menuBar       *menuBar
	helpPopover   *cellPopover
	searchPopover *cellPopover
	fileDropdown  *menuDropdown
	viewDropdown  *menuDropdown
	helpDropdown  *menuDropdown
	root          *packedVBox
	files         map[string]string
	paths         []string // flat, order-stable list of file paths for fileList indexing
}

// menuItem is one clickable label in a menuBar. Layout paints the
// items horizontally separated by menuBarSep spaces; a click inside
// an item's cell range invokes OnClick. OnClick is optional — a nil
// value is a no-op (used by decorative separators or coming-soon slots).
type menuItem struct {
	Label   string
	OnClick func()
}

const menuBarSep = 3 // spaces between items
const menuBarPad = 1 // left padding before the first item

// menuBar is a cell-native horizontal menu. Items render inline with
// menuBarSep-space gaps; a click on an item's Label X-range fires
// its OnClick. Selected-item state is not tracked — dropdowns live
// as separate popovers wired to each item's OnClick.
type menuBar struct {
	toolkit.Base
	Items []menuItem
}

func (m *menuBar) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := m.Bounds()
	p.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.Background.R, G: theme.Background.G, B: theme.Background.B, A: theme.Background.A,
	})
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	x := r.X + menuBarPad
	for _, item := range m.Items {
		toolkit.DrawText(p, x, r.Y, item.Label, ink)
		x += len(item.Label) + menuBarSep
	}
}

// itemXRange returns the [startX, endX) local-X range of the i-th
// item — used both by OnEvent (hit-test) and by callers that want to
// anchor a dropdown at the item's position.
func (m *menuBar) itemXRange(i int) (int, int) {
	x := menuBarPad
	for k, item := range m.Items {
		if k == i {
			return x, x + len(item.Label)
		}
		x += len(item.Label) + menuBarSep
	}
	return -1, -1
}

func (m *menuBar) OnEvent(ev toolkit.Event) {
	if ev.Kind != toolkit.EventClick {
		return
	}
	for i, item := range m.Items {
		x0, x1 := m.itemXRange(i)
		if ev.X >= x0 && ev.X < x1 {
			if item.OnClick != nil {
				item.OnClick()
			}
			return
		}
	}
}

// cellPopover is a cell-native modal: title on row Y+0, body lines
// starting at row Y+2 (title + 1-row gap), border box around
// everything. Replaces toolkit.Popover whose PopoverPadY = 6 puts
// the title 6 cells below the box top in cell mode + whose child
// widget offset (~15+ cells) puts the body far below the title.
type cellPopover struct {
	toolkit.Base
	Title   string
	Body    []string
	Visible bool
}

// HitTest — an invisible popover must not claim clicks, otherwise
// packedVBox routing would short-circuit every body click into the
// popover's OnEvent (a no-op) and the underlying content would stop
// receiving input. Base.HitTest is bounds-only; we AND with Visible.
func (p *cellPopover) HitTest(px, py int) bool {
	if !p.Visible {
		return false
	}
	return p.Base.HitTest(px, py)
}

// menuDropdown is an anchored variant of cellPopover — used for menu
// bar dropdowns that need to appear directly below the clicked menu
// item, not centred in the body inset. It computes its own bounds
// from AnchorX/AnchorY plus the natural size of its content.
//
// Compared to cellPopover:
//   - No 4-cell inset from container edges
//   - Anchor point is (AnchorX, AnchorY), typically (item.x0, headerH)
//   - Width auto-fits max(Title, longest body line) + 4 padding
//   - Height = 2 border rows + len(Body)
//   - Clicking anywhere on it closes it (dismisses without action)
type menuDropdown struct {
	toolkit.Base
	Title   string
	Body    []string
	Visible bool
	AnchorX int
	AnchorY int
}

func (d *menuDropdown) size() (int, int) {
	w := len(d.Title)
	for _, line := range d.Body {
		if l := len(line); l > w {
			w = l
		}
	}
	w += 4 // 1-cell border each side + 1-cell text padding each side
	h := 2 + len(d.Body)
	if h < 3 {
		h = 3
	}
	return w, h
}

// SetBounds ignores the parent's requested rect and self-positions
// at (AnchorX, AnchorY) with the natural size. packedVBox calls this
// during layout; the dropdown ignores the "centred inset" it would
// otherwise receive.
func (d *menuDropdown) SetBounds(_ toolkit.Rect) {
	w, h := d.size()
	d.Base.SetBounds(toolkit.Rect{X: d.AnchorX, Y: d.AnchorY, W: w, H: h})
}

func (d *menuDropdown) HitTest(px, py int) bool {
	if !d.Visible {
		return false
	}
	return d.Base.HitTest(px, py)
}

func (d *menuDropdown) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !d.Visible {
		return
	}
	// Refresh bounds from the current AnchorX/AnchorY. menuBar
	// updates the anchor right before flipping Visible, but SetBounds
	// only runs at layout time (once at startup), so without this the
	// dropdown would draw at the anchor it had when packedVBox last
	// laid out — usually (0, 0).
	d.SetBounds(toolkit.Rect{})
	r := d.Bounds()
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: theme.Border.A,
	}, 1)
	ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	if d.Title != "" {
		toolkit.DrawText(pnt, r.X+2, r.Y, d.Title, ink)
	}
	// SetBounds sizes r.H = 2 + len(Body), so every body line has a
	// row inside the box — no per-line clip guard needed.
	for i, line := range d.Body {
		toolkit.DrawText(pnt, r.X+2, r.Y+1+i, line, ink)
	}
}

// OnEvent — click anywhere on the dropdown closes it. This mirrors
// standard menu UX where a click either activates a highlighted
// item or dismisses the menu.
func (d *menuDropdown) OnEvent(ev toolkit.Event) {
	if ev.Kind == toolkit.EventClick {
		d.Visible = false
	}
}

func (p *cellPopover) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if !p.Visible {
		return
	}
	r := p.Bounds()
	// Tighten the drawn box to just enough rows for title + body.
	need := 3 + len(p.Body) // border-top + title + gap + body + border-bot
	if need > r.H {
		need = r.H
	}
	// Distinct background so the overlay reads as a modal on top of
	// the underlying surface.
	pnt.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	// 1-cell border. StrokeRect draws box-drawing chars.
	pnt.StrokeRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: need}, painter.RGBA{
		R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: theme.Border.A,
	}, 1)
	titleInk := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
	toolkit.DrawText(pnt, r.X+2, r.Y+1, p.Title, titleInk)
	// Body rows start at y=2 (title on 1, gap on... well no gap;
	// title on 1, body 2..end).
	for i, line := range p.Body {
		y := r.Y + 2 + i
		if y >= r.Y+need-1 {
			break
		}
		toolkit.DrawText(pnt, r.X+2, y, line, titleInk)
	}
}

// newState builds the interactive demo's widget tree. Cell-mode
// friendly composition:
//
//   - header (Label) — menu-style hint at row 0
//   - body — an HBox-ish split of a fileList (left) + TextView (right)
//     laid out by a hSplit local helper so cell-scale bounds are
//     respected (toolkit.HPaned pixel-tunes its splitter offset).
//   - footer (Label) — key hints at row H-1
//
// The pre-v0.3.4 tree/notebook/HPaned composition rendered poorly in
// cell mode — TreeView row heights + Notebook body padding are
// pixel-scaled. This composition uses only widgets that render at
// exactly 1 cell per glyph.
func newState() *state {
	files := map[string]string{
		"/src/main.go":    "package main\n\nfunc main() {}\n",
		"/src/util.go":    "package util\n\nfunc util() {}\n",
		"/docs/README.md": "# Project\n\nDemo project.\n",
		"/LICENSE":        "BSD-3-Clause\n",
	}
	paths := []string{"/src/main.go", "/src/util.go", "/docs/README.md", "/LICENSE"}

	fl := &fileList{items: paths, selected: 0}
	content := &tui.TextEditor{ReadOnly: true, ShowGutter: true}
	content.Filename = paths[0]
	content.SetText(files[paths[0]])

	body := &hSplit{left: fl, right: content, leftFrac: 30}

	status := toolkit.NewLabel("q: quit  ?: help  /: search  Up/Down: navigate  Enter: open")

	helpPopover := &cellPopover{
		Title: "Help",
		Body: []string{
			"q          Quit",
			"?          Toggle this help",
			"/          Toggle search",
			"Up / Down  Navigate file list",
			"Enter      Refresh content pane",
			"",
			"Mouse:",
			"click file  Select + preview",
			"drag grip   Resize sidebar",
			"click menu  Open dropdown",
		},
	}
	searchPopover := &cellPopover{
		Title: "Search",
		Body: []string{
			"(v0.3.x: overlay stub — real fuzzy",
			"finder is planned for v0.4)",
		},
	}
	// Anchored dropdowns sit directly below the menu item. AnchorY=1
	// (headerH), AnchorX is patched in by menuBar.itemXRange when the
	// item is clicked so the dropdown lines up with its label.
	fileDropdown := &menuDropdown{
		Title:   "File",
		Body:    []string{"New       (stub)", "Open      (stub)", "Reload    (stub)", "Quit      q"},
		AnchorY: 1,
	}
	viewDropdown := &menuDropdown{
		Title:   "View",
		Body:    []string{"Toggle sidebar  (drag grip)", "Focus preview   Enter", "Refresh         Enter"},
		AnchorY: 1,
	}
	helpDropdown := &menuDropdown{
		Title: "Help",
		Body: []string{
			"click file  Select + preview",
			"drag grip   Resize sidebar",
			"?           Full help modal",
			"q           Quit",
		},
		AnchorY: 1,
	}

	mb := &menuBar{}
	mb.Items = []menuItem{
		{Label: "File", OnClick: func() {
			x0, _ := mb.itemXRange(0)
			fileDropdown.AnchorX = x0
			fileDropdown.Visible = !fileDropdown.Visible
			// Close the other dropdowns so only one is open at a time.
			viewDropdown.Visible = false
			helpDropdown.Visible = false
		}},
		{Label: "View", OnClick: func() {
			x0, _ := mb.itemXRange(1)
			viewDropdown.AnchorX = x0
			viewDropdown.Visible = !viewDropdown.Visible
			fileDropdown.Visible = false
			helpDropdown.Visible = false
		}},
		{Label: "Help", OnClick: func() {
			x0, _ := mb.itemXRange(2)
			helpDropdown.AnchorX = x0
			helpDropdown.Visible = !helpDropdown.Visible
			fileDropdown.Visible = false
			viewDropdown.Visible = false
		}},
	}

	pv := &packedVBox{
		header:   mb,
		body:     body,
		footer:   status,
		headerH:  1,
		footerH:  1,
		overlays: []toolkit.Widget{helpPopover, searchPopover, fileDropdown, viewDropdown, helpDropdown},
	}

	s := &state{
		fileList:      fl,
		content:       content,
		status:        status,
		menuBar:       mb,
		helpPopover:   helpPopover,
		searchPopover: searchPopover,
		fileDropdown:  fileDropdown,
		viewDropdown:  viewDropdown,
		helpDropdown:  helpDropdown,
		root:          pv,
		files:         files,
		paths:         paths,
	}
	// Wire the fileList → syncContent bridge so a click (or an
	// arrow key routed through the widget instead of the App key
	// map) refreshes the right pane. The App-level Up/Down handlers
	// stay in place for backwards compatibility.
	fl.onSelect = func(int) { s.syncContent() }
	return s
}

// syncContent refreshes the right pane with the file at the currently
// selected fileList index.
func (s *state) syncContent() {
	if s.fileList.selected < 0 || s.fileList.selected >= len(s.paths) {
		s.content.Filename = ""
		s.content.SetText("(no selection)")
		return
	}
	path := s.paths[s.fileList.selected]
	s.content.Filename = path
	s.content.SetText(s.files[path])
}

// fileList is a cell-native list widget: one item per row, selection
// highlight, arrow-key navigation. Replaces TreeView + Notebook +
// HPaned which are all pixel-tuned and render poorly in cell mode.
type fileList struct {
	toolkit.Base
	items    []string
	selected int
	// onSelect fires after selected changes via either a keyboard
	// event or a click. Optional — a nil callback is a no-op. The
	// callback receives the new index; wire it to whatever refresh
	// the caller needs (e.g. syncContent + App.Refresh).
	onSelect func(int)
}

func (f *fileList) Draw(p painter.Painter, theme *toolkit.Theme) {
	r := f.Bounds()
	// Paint the full pane background first so surface color fills
	// even the rows past len(items).
	p.FillRect(painter.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	for i, item := range f.items {
		y := r.Y + i
		if y >= r.Y+r.H {
			break
		}
		ink := toolkit.RGBA{R: theme.OnSurface.R, G: theme.OnSurface.G, B: theme.OnSurface.B, A: theme.OnSurface.A}
		if i == f.selected {
			// Selected row: accent-fill background via FillRect (fills
			// cell backgrounds, not '█' glyphs), ink switches to
			// theme.Background for contrast. Prior versions used
			// PutPixel which in cell mode paints solid-block chars —
			// which reads as "█████" instead of a highlight strip.
			p.FillRect(painter.Rect{X: r.X, Y: y, W: r.W, H: 1}, painter.RGBA{
				R: theme.Accent.R, G: theme.Accent.G, B: theme.Accent.B, A: theme.Accent.A,
			})
			ink = toolkit.RGBA{R: theme.Background.R, G: theme.Background.G, B: theme.Background.B, A: theme.Background.A}
		}
		toolkit.DrawText(p, r.X+1, y, item, ink)
	}
}

func (f *fileList) OnEvent(ev toolkit.Event) {
	switch ev.Kind {
	case toolkit.EventKeyDown:
		switch ev.Code {
		case "Up":
			if f.selected > 0 {
				f.selected--
				if f.onSelect != nil {
					f.onSelect(f.selected)
				}
			}
		case "Down":
			if f.selected < len(f.items)-1 {
				f.selected++
				if f.onSelect != nil {
					f.onSelect(f.selected)
				}
			}
		}
	case toolkit.EventClick:
		// Coordinates are widget-local per the toolkit contract; the
		// container translates surface coords → local before dispatch.
		if ev.Y < 0 || ev.Y >= len(f.items) {
			return
		}
		f.selected = ev.Y
		if f.onSelect != nil {
			f.onSelect(f.selected)
		}
	}
}

// hSplit is a cell-native horizontal split: left widget takes
// leftFrac percent of the width, a 1-cell grip column at X=lw
// separates left from right, right widget takes the remainder.
//
// The grip glyph doubles as a drag handle: a click on it starts a
// drag session; subsequent EventMouseDrag events (routed here by
// the packedVBox drag-capture) update leftFrac; EventMouseUp ends
// the session. leftFrac is clamped to [hSplitMinFrac, hSplitMaxFrac]
// so neither pane can collapse to zero cells and vanish.
type hSplit struct {
	toolkit.Base
	left, right toolkit.Widget
	leftFrac    int
	dragging    bool
}

const (
	// hSplitBarRune is the thin vertical char used for most of the
	// separator column; hSplitGripRune is the "heavy" vertical used
	// for the centred grip zone so the user has a visible handle
	// hint on the resize bar.
	hSplitBarRune   = '│'
	hSplitGripRune  = '┃'
	hSplitMinFrac   = 10
	hSplitMaxFrac   = 90
	hSplitGripFocus = 4 // reserved for future keyboard resize
)

// gripZone returns [y0, y1) of the grip-handle band: a centred strip
// ≈ 1/6 of the bar's height, min 3 cells, max 7. The full bar is
// clickable — the strip is just a visual affordance.
func gripZone(barH int) (int, int) {
	h := barH / 6
	if h < 3 {
		h = 3
	}
	if h > 7 {
		h = 7
	}
	if h > barH {
		h = barH
	}
	y0 := (barH - h) / 2
	return y0, y0 + h
}

// gripLocalX returns the local-X column the grip occupies given the
// current bounds and leftFrac.
func (h *hSplit) gripLocalX() int {
	return h.Bounds().W * h.leftFrac / 100
}

func (h *hSplit) SetBounds(r toolkit.Rect) {
	h.Base.SetBounds(r)
	lw := r.W * h.leftFrac / 100
	if h.left != nil {
		h.left.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: lw, H: r.H})
	}
	if h.right != nil {
		// right pane starts one column past the grip.
		h.right.SetBounds(toolkit.Rect{X: r.X + lw + 1, Y: r.Y, W: r.W - lw - 1, H: r.H})
	}
}

func (h *hSplit) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if h.left != nil {
		h.left.Draw(pnt, theme)
	}
	if h.right != nil {
		h.right.Draw(pnt, theme)
	}
	// Grip column, painted on top so it stays visible whichever
	// pane's background it lands over. Border color unless the user
	// is actively dragging — then Accent for visible feedback.
	r := h.Bounds()
	lw := h.gripLocalX()
	col := toolkit.RGBA{R: theme.Border.R, G: theme.Border.G, B: theme.Border.B, A: theme.Border.A}
	if h.dragging {
		col = toolkit.RGBA{R: theme.Accent.R, G: theme.Accent.G, B: theme.Accent.B, A: theme.Accent.A}
	}
	// Fill the grip column with the surrounding pane bg first so
	// the vertical line doesn't inherit a stale character from an
	// adjacent pane render.
	pnt.FillRect(painter.Rect{X: r.X + lw, Y: r.Y, W: 1, H: r.H}, painter.RGBA{
		R: theme.SurfaceAlt.R, G: theme.SurfaceAlt.G, B: theme.SurfaceAlt.B, A: theme.SurfaceAlt.A,
	})
	// Thin bar for the entire column, with a heavy strip in the
	// middle (≈ 1/6 of the bar's height) that serves as a visible
	// grip handle affordance. Both regions are clickable.
	gy0, gy1 := gripZone(r.H)
	for y := 0; y < r.H; y++ {
		g := hSplitBarRune
		if y >= gy0 && y < gy1 {
			g = hSplitGripRune
		}
		toolkit.DrawText(pnt, r.X+lw, r.Y+y, string(g), col)
	}
}

func (h *hSplit) OnEvent(ev toolkit.Event) {
	// Drag session: subsequent drag/up events (routed here by the
	// parent packedVBox capture) resize or terminate.
	if h.dragging {
		switch ev.Kind {
		case toolkit.EventMouseDrag:
			w := h.Bounds().W
			if w <= 0 {
				return
			}
			frac := ev.X * 100 / w
			if frac < hSplitMinFrac {
				frac = hSplitMinFrac
			}
			if frac > hSplitMaxFrac {
				frac = hSplitMaxFrac
			}
			h.leftFrac = frac
			h.SetBounds(h.Bounds())
			return
		case toolkit.EventMouseUp:
			h.dragging = false
			return
		}
	}
	switch ev.Kind {
	case toolkit.EventClick:
		lw := h.gripLocalX()
		if ev.X == lw {
			// Click on the grip → enter drag mode. Any subsequent
			// EventMouseDrag from the parent updates leftFrac.
			h.dragging = true
			return
		}
		if ev.X < lw {
			if h.left != nil {
				h.left.OnEvent(ev)
			}
			return
		}
		if h.right != nil {
			child := ev
			child.X -= lw + 1
			h.right.OnEvent(child)
		}
	case toolkit.EventMouseDrag, toolkit.EventMouseUp:
		// Drag/up outside a session — ignore.
	default:
		// Non-mouse events (arrow keys, chars) still go to left pane.
		if h.left != nil {
			h.left.OnEvent(ev)
		}
	}
}

// packedVBox lays out three children with fixed-height header +
// footer and a body that fills the remaining vertical space, plus
// optional overlays that draw on top of everything else. Suitable
// for terminal-scale layouts where toolkit.VBox's equal-height
// distribution would make every element unusably big.
//
// SetBounds re-runs the layout at every resize; Draw paints in
// draw-order (body → header → footer → overlays), so overlays land
// on top and are visible even when their bounds intersect other
// children; OnEvent forwards to body (the main interactive area).
//
// Overlays are widget references appended to `overlays` at wire
// time. A widget's own `Visible` field (Popover, Toast, …) gates
// whether Draw actually paints — the packedVBox unconditionally
// dispatches Draw to every registered overlay every frame.
type packedVBox struct {
	toolkit.Base
	header   toolkit.Widget
	body     toolkit.Widget
	footer   toolkit.Widget
	headerH  int
	footerH  int
	overlays []toolkit.Widget
	// Drag capture: when EventClick is dispatched to a target, that
	// target owns subsequent EventMouseDrag / EventMouseUp events
	// until MouseUp releases the capture. Without this, a drag that
	// wanders across Y bands would jump between header/body/footer
	// mid-stroke — e.g. resizing the sidebar and having the drag
	// events end up in the footer as soon as the pointer strays.
	dragTarget toolkit.Widget
	dragDx     int
	dragDy     int
}

func (p *packedVBox) SetBounds(r toolkit.Rect) {
	p.Base.SetBounds(r)
	if p.header != nil {
		p.header.SetBounds(toolkit.Rect{X: r.X, Y: r.Y, W: r.W, H: p.headerH})
	}
	if p.footer != nil {
		p.footer.SetBounds(toolkit.Rect{X: r.X, Y: r.Y + r.H - p.footerH, W: r.W, H: p.footerH})
	}
	if p.body != nil {
		p.body.SetBounds(toolkit.Rect{
			X: r.X,
			Y: r.Y + p.headerH,
			W: r.W,
			H: r.H - p.headerH - p.footerH,
		})
	}
	// Overlays: centred in the body area with 4-cell padding.
	for _, o := range p.overlays {
		bx := r.X + 4
		by := r.Y + p.headerH + 2
		bw := r.W - 8
		bh := r.H - p.headerH - p.footerH - 4
		if bw < 1 {
			bw = 1
		}
		if bh < 1 {
			bh = 1
		}
		o.SetBounds(toolkit.Rect{X: bx, Y: by, W: bw, H: bh})
	}
}

func (p *packedVBox) Draw(pnt painter.Painter, theme *toolkit.Theme) {
	if p.body != nil {
		p.body.Draw(pnt, theme)
	}
	if p.header != nil {
		p.header.Draw(pnt, theme)
	}
	if p.footer != nil {
		p.footer.Draw(pnt, theme)
	}
	for _, o := range p.overlays {
		o.Draw(pnt, theme)
	}
}

func (p *packedVBox) OnEvent(ev toolkit.Event) {
	// Drag capture: while dragTarget is set, forward drag/up events
	// to the captured widget with the same translation as the click.
	if p.dragTarget != nil && (ev.Kind == toolkit.EventMouseDrag || ev.Kind == toolkit.EventMouseUp) {
		child := ev
		child.X -= p.dragDx
		child.Y -= p.dragDy
		p.dragTarget.OnEvent(child)
		if ev.Kind == toolkit.EventMouseUp {
			p.dragTarget = nil
			p.dragDx = 0
			p.dragDy = 0
		}
		return
	}
	if ev.Kind == toolkit.EventClick {
		// ev.X/Y are widget-local to packedVBox. Vertical layout:
		//   header at Y ∈ [0, headerH)
		//   body   at Y ∈ [headerH, H - footerH)
		//   footer at Y ∈ [H - footerH, H)
		r := p.Bounds()
		// Overlays sit on top of the body area with per-widget bounds.
		// Delegated to each widget's HitTest so invisible cellPopover
		// / menuDropdown overlays (which override HitTest to return
		// false when !Visible) get skipped, and anchored dropdowns
		// with non-inset bounds get correctly targeted. Iterate in
		// reverse — later-added overlays paint on top so they should
		// also claim clicks first when overlapping.
		for i := len(p.overlays) - 1; i >= 0; i-- {
			o := p.overlays[i]
			if !o.HitTest(ev.X, ev.Y) {
				continue
			}
			ob := o.Bounds()
			child := ev
			child.X -= ob.X
			child.Y -= ob.Y
			o.OnEvent(child)
			p.dragTarget, p.dragDx, p.dragDy = o, ob.X, ob.Y
			return
		}
		switch {
		case ev.Y < p.headerH:
			if p.header != nil {
				p.header.OnEvent(ev)
				p.dragTarget, p.dragDx, p.dragDy = p.header, 0, 0
			}
		case ev.Y >= r.H-p.footerH:
			if p.footer != nil {
				child := ev
				child.Y -= r.H - p.footerH
				p.footer.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.footer, 0, r.H-p.footerH
			}
		default:
			if p.body != nil {
				child := ev
				child.Y -= p.headerH
				p.body.OnEvent(child)
				p.dragTarget, p.dragDx, p.dragDy = p.body, 0, p.headerH
			}
		}
		return
	}
	if p.body != nil {
		p.body.OnEvent(ev)
	}
}

// keys returns the App-level key bindings the demo installs. Kept
// as a method rather than an inline map literal so tests can exercise
// each handler in isolation without stepping through Run().
func (s *state) keys() map[string]func(*tui.App) {
	return map[string]func(*tui.App){
		"q":      func(a *tui.App) { a.Quit() },
		"Ctrl+C": func(a *tui.App) { a.Quit() },
		"?": func(a *tui.App) {
			s.helpPopover.Visible = !s.helpPopover.Visible
			a.Refresh()
		},
		"/": func(a *tui.App) {
			s.searchPopover.Visible = !s.searchPopover.Visible
			a.Refresh()
		},
		"Up": func(a *tui.App) {
			if s.fileList.selected > 0 {
				s.fileList.selected--
				s.syncContent()
				a.Consume()
			}
		},
		"Down": func(a *tui.App) {
			if s.fileList.selected < len(s.fileList.items)-1 {
				s.fileList.selected++
				s.syncContent()
				a.Consume()
			}
		},
		"Enter": func(a *tui.App) {
			s.syncContent()
			a.Refresh()
		},
	}
}
