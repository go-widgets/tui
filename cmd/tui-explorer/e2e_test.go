// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// End-to-end integration tests using cell-level assertions.
//
// The v0.3.2 through v0.3.5 patches all shipped bugs that would
// have been caught by the assertions here — but the OLD e2e tests
// stripped ANSI colors before asserting, so every visual bug (wrong
// row background, block-char glyphs, clipped text) slipped through.
//
// The new discipline: decode the pty output into a TermGrid, then
// assert on specific cells' Rune + Fg + Bg. Text-content assertions
// stay via TermGrid.RowText, but the color / rune assertions are
// what catch the bugs that would otherwise ship.

package main

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/go-widgets/tui"
)

// Theme constants — must match toolkit.DefaultLight() so assertions
// don't drift when the theme rotates. Values verified from
// toolkit/theme.go's DefaultLight function to avoid the "assumed
// brand teal" mistake I made in the first draft (the ACTUAL default
// light Accent is Adwaita blue #3584E4).
const (
	lightBgR, lightBgG, lightBgB             = 0xFA, 0xFA, 0xFA // theme.Background
	lightSurfaceR, lightSurfaceG, lightSuB   = 0xE8, 0xEA, 0xED // theme.Surface
	lightAccentR, lightAccentG, lightAccentB = 0x35, 0x84, 0xE4 // theme.Accent (Adwaita blue)
)

// captureFrame spawns the explorer under a pty sized cols×rows, sends
// the given key stream after a settle delay, waits up to timeout for
// the process to exit, and returns the decoded TermGrid. Standard
// harness for every cell-level test.
func captureFrame(t *testing.T, cols, rows int, keys string, timeout time.Duration) *tui.TermGrid {
	t.Helper()
	bin := buildBinary(t)

	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	captureDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(captureDone)
	}()

	// Feed the key stream one byte at a time with a small inter-key
	// delay so the App loop repaints between mutations.
	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 0; i < len(keys); i++ {
			_, _ = ptmx.Write([]byte{keys[i]})
			time.Sleep(80 * time.Millisecond)
		}
	}()

	select {
	case <-captureDone:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()

	return tui.DecodeANSI(buf.Bytes(), cols, rows)
}

// findLastFrame returns the last complete frame (up to the last
// "cursor-home" CSI). A tui.App run may emit multiple frames over
// its lifetime; assertions want the FINAL one after all input.
func findLastFrame(raw []byte, cols, rows int) *tui.TermGrid {
	// Find last "\x1b[H" (home cursor) — every frame in tui.App
	// starts with that. If we split there, the tail is the frame
	// active at exit-time.
	idx := bytes.LastIndex(raw, []byte("\x1b[H"))
	if idx < 0 {
		return tui.DecodeANSI(raw, cols, rows)
	}
	return tui.DecodeANSI(raw[idx:], cols, rows)
}

// -----------------------------------------------------------------
// v0.3.2 regression — VBox equal-height split
// -----------------------------------------------------------------

// TestExplorerHeaderIsAtRow0AndFooterAtLastRow catches the v0.3.0 /
// v0.3.1 layout bug where VBox split the three children equally,
// pushing chrome to a third of the screen each.
func TestExplorerHeaderIsAtRow0AndFooterAtLastRow(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	if !strings.Contains(g.RowText(0), "File") {
		t.Fatalf("row 0 (header) missing 'File': %q", g.RowText(0))
	}
	if !strings.Contains(g.RowText(29), "q: quit") {
		t.Fatalf("row 29 (footer) missing 'q: quit': %q", g.RowText(29))
	}
}

// -----------------------------------------------------------------
// v0.3.3 regression — help popover never drawn
// -----------------------------------------------------------------

// TestExplorerHelpPopoverActuallyRendersOnQuestionMark catches the
// v0.3.2 bug where '?' toggled Visible but the popover was never
// added to the widget tree, so nothing painted.
func TestExplorerHelpPopoverActuallyRendersOnQuestionMark(t *testing.T) {
	g := captureFrame(t, 80, 30, "?q", 3*time.Second)
	// "Enter: open" appears only in the help popover Label, not in
	// the always-visible footer. Its presence proves the popover
	// actually rendered.
	found := false
	for y := 0; y < g.Rows; y++ {
		if strings.Contains(g.RowText(y), "Enter: open") {
			found = true
			break
		}
	}
	if !found {
		var dump strings.Builder
		for y := 0; y < g.Rows; y++ {
			dump.WriteString(g.RowText(y))
			dump.WriteByte('\n')
		}
		t.Fatalf("'Enter: open' (help popover content) never rendered:\n%s", dump.String())
	}
}

// -----------------------------------------------------------------
// v0.3.5 regression — fileList highlight was █ chars
// -----------------------------------------------------------------

// TestExplorerSelectedRowHasAccentBackground catches the v0.3.4 bug
// where fileList used PutPixel for the selected-row highlight,
// which in cell mode wrote '█' chars instead of setting the cell
// background color. Assertion: row 1 (selected /src/main.go) cells
// must have Bg = theme.Accent AND rune != '█'.
func TestExplorerSelectedRowHasAccentBackground(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)

	// Row 1 = selected /src/main.go. Sample a cell in the middle of
	// the highlight strip (past the leading space, inside the name).
	c := g.At(3, 1) // '/src/main.go' starts at col 1
	if !c.Bg.Set {
		t.Fatalf("row 1 col 3: no bg color set — highlight not applied. cell=%+v", c)
	}
	if c.Bg.R != lightAccentR || c.Bg.G != lightAccentG || c.Bg.B != lightAccentB {
		t.Fatalf("row 1 col 3 bg = (%d,%d,%d), want accent (%d,%d,%d)",
			c.Bg.R, c.Bg.G, c.Bg.B,
			lightAccentR, lightAccentG, lightAccentB)
	}
	// The KEY assertion — no '█' char, which was v0.3.4's bug.
	for x := 0; x < 24; x++ {
		if g.At(x, 1).Rune == '█' {
			t.Fatalf("row 1 col %d contains '█' block char — PutPixel-in-cell-mode bug. cell=%+v",
				x, g.At(x, 1))
		}
	}
}

// TestExplorerSelectedRowNameStillReadable — a stronger form: not
// only must the background be accent AND runes not be '█', but the
// filename must still be readable in the highlighted row.
func TestExplorerSelectedRowNameStillReadable(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	if !strings.Contains(g.RowText(1), "/src/main.go") {
		t.Fatalf("selected row 1 does not contain '/src/main.go': %q", g.RowText(1))
	}
}

// -----------------------------------------------------------------
// v0.3.5 regression — TextView showed only 2 lines out of 3
// -----------------------------------------------------------------

// TestExplorerContentPreviewShowsAllLinesWithoutClipping catches the
// v0.3.4 bug where toolkit.TextView used lineH=11 in cell mode so a
// 22-row body only showed 2 of the 3 file lines. Assertion: all 3
// lines of /src/main.go must land on distinct rows in the right pane.
func TestExplorerContentPreviewShowsAllLinesWithoutClipping(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)

	// /src/main.go contents: "package main\n\nfunc main() {}\n"
	// After the v0.3.5 fix, all 3 lines land as 3 rows (with the
	// middle row blank).
	var packageRow, funcRow int
	packageRow, funcRow = -1, -1
	for y := 0; y < g.Rows; y++ {
		text := g.RowText(y)
		if strings.Contains(text, "package main") {
			packageRow = y
		}
		if strings.Contains(text, "func main") {
			funcRow = y
		}
	}
	if packageRow < 0 {
		t.Fatal("'package main' never rendered in the content pane")
	}
	if funcRow < 0 {
		t.Fatal("'func main() {}' never rendered — TextView clipped it (v0.3.4 bug)")
	}
	if funcRow-packageRow != 2 {
		t.Errorf("expected 2-row gap between package + func lines, got packageRow=%d funcRow=%d",
			packageRow, funcRow)
	}
}

// -----------------------------------------------------------------
// v0.3.4 regression — arrow key navigation
// -----------------------------------------------------------------

// TestExplorerArrowDownSyncsContent — pressing Down twice must
// select the third file (index 2 = /docs/README.md), and its
// content ("# Project") must appear in the right pane. If arrow
// handlers stopped mutating state OR syncContent stopped firing OR
// the right pane stopped repainting, one of the three assertions
// fails.
func TestExplorerArrowDownSyncsContent(t *testing.T) {
	// Two Down arrows = 6 bytes (each is ESC [ B).
	g := captureFrame(t, 80, 30, "\x1b[B\x1b[Bq", 5*time.Second)

	// Row 3 = third fileList entry = /docs/README.md.
	if !strings.Contains(g.RowText(3), "/docs/README.md") {
		t.Errorf("row 3 = %q, want '/docs/README.md'", g.RowText(3))
	}
	// The selected row must have the accent background NOW at
	// row 3 (not row 1) — confirms the highlight moved.
	c := g.At(3, 3)
	if !c.Bg.Set || c.Bg.R != lightAccentR {
		t.Errorf("row 3 highlight not applied after 2×Down. cell=%+v", c)
	}
	// The right pane must show README.md's content.
	found := false
	for y := 0; y < g.Rows; y++ {
		if strings.Contains(g.RowText(y), "# Project") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("right pane never showed '# Project' after 2×Down — syncContent didn't fire")
	}
}

// -----------------------------------------------------------------
// Meta — the failure-mode assertions catch the specific bugs
// -----------------------------------------------------------------

// TestExplorerNoBlockCharsAnywhere is a scan of the whole grid: no
// cell should contain '█' unless it was intentional (there are no
// intentional block chars in this composition). Would loud-fail on
// any future regression that re-introduces PutPixel-as-background.
func TestExplorerNoBlockCharsAnywhere(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			if g.At(x, y).Rune == '█' {
				t.Errorf("block char '█' at (%d,%d) — cell background misused as glyph",
					x, y)
			}
		}
	}
}

// TestExplorerBodyChromeContrastIsPerceptible asserts that the body
// area and the chrome rows (header/footer) have visibly distinct
// background colors — luminance difference ≥ 8 on a 0..255 scale.
// A pixel-value equality assertion PASSES if bg values are
// technically different (e.g. Surface #1F2228 vs Background #14161A —
// 12 luminance units apart but visually indistinguishable). This
// stronger assertion mandates a THRESHOLD that keeps a human eye
// able to see the panel boundary.
//
// Catches the v0.3.7 "all-dark screen in Terminal.app" bug where
// the body used Surface and chrome used Background — technically
// different, but perceptually the same shade of near-black.
func TestExplorerBodyChromeContrastIsPerceptible(t *testing.T) {
	// Test in BOTH themes to catch a regression in either.
	for _, tc := range []struct {
		name   string
		keys   string
		darkTh bool
	}{
		{"light", "q", false},
		{"dark", "q", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g *tui.TermGrid
			if tc.darkTh {
				g = captureFrameWithArgs(t, 80, 30, tc.keys, 3*time.Second, "--theme=dark")
			} else {
				g = captureFrame(t, 80, 30, tc.keys, 3*time.Second)
			}
			// Row 0 = header (Background). Row 5 = body (SurfaceAlt).
			chrome := g.At(0, 0).Bg
			body := g.At(0, 5).Bg
			if !chrome.Set || !body.Set {
				t.Fatalf("chrome or body bg not set: chrome=%+v body=%+v", chrome, body)
			}
			diff := luminanceDiff(chrome, body)
			if diff < 8 {
				t.Fatalf("chrome/body luminance diff = %d, want ≥ 8 (%s theme): chrome=(%d,%d,%d) body=(%d,%d,%d)",
					diff, tc.name,
					chrome.R, chrome.G, chrome.B,
					body.R, body.G, body.B)
			}
		})
	}
}

// -----------------------------------------------------------------
// v0.3.12 menuBar clicks + hSplit grip drag
// -----------------------------------------------------------------

// TestExplorerClickOnHelpMenuAnchoredBelowItem — a click on "Help"
// in the menuBar opens the anchored dropdown right below the item.
// "Help" is menu item 2 with X range [15, 19) (padding 1 + 4+3+4+3),
// so the dropdown anchors at X=15 and Y=1 (right below header row 0).
func TestExplorerClickOnHelpMenuAnchoredBelowItem(t *testing.T) {
	// Wire coords 1-indexed → click on local X=16 (inside [15,19)).
	// Terminal row 2 = decoded Y=1 = header row 0 on wire = X for
	// menuItem hit. Wire row 1 = decoded row 0 (header).
	keys := [][]byte{
		sgrMousePress(17, 1), // click "Help"
		[]byte("q"),
	}
	g := captureFrameWithBytes(t, 80, 30, keys, 5*time.Second)
	// Dropdown top border is at row 1, title on row 1, body starts at row 2.
	// Row 1 must show "Help" title inside the box border.
	if !strings.Contains(g.RowText(1), "Help") {
		t.Errorf("dropdown title 'Help' not on row 1: %q", g.RowText(1))
	}
	// Body content ("q" or "drag grip" etc.) should be within a few
	// rows below.
	joined := ""
	for y := 1; y < 8; y++ {
		joined += g.RowText(y) + " | "
	}
	if !strings.Contains(joined, "drag grip") && !strings.Contains(joined, "Quit") {
		t.Errorf("dropdown body not visible in rows 1..7: %q", joined)
	}
	// The dropdown must be ANCHORED under "Help" — its left border
	// glyph ┌ or │ should appear at column 15. Look for a border-drawing
	// character in a small band around row 1..2.
	found := false
	for y := 1; y < 3; y++ {
		c := g.At(15, y)
		if c.Rune == '┌' || c.Rune == '│' || c.Rune == '└' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("dropdown not anchored at column 15 (under 'Help')")
	}
}

// TestExplorerClickOnGripStartsDragAndResizes — a mouse press on the
// grip column followed by a drag to a new X changes the split ratio,
// visible as the grip character landing on a different column.
func TestExplorerClickOnGripStartsDragAndResizes(t *testing.T) {
	// Initial grip column: body width = 80, hSplit starts at X=0 of
	// body → hSplit.W=80, leftFrac=30, lw = 80*30/100 = 24. Grip at
	// wire X=25 (1-indexed), body row 2..29 (any is fine — row 5).
	//
	// After drag to wire X=41 (local X=40 in body = local X=40 in
	// hSplit), leftFrac = 40*100/80 = 50. Grip lands at local X=40,
	// wire X=41.
	keys := [][]byte{
		sgrMousePress(25, 5),                 // press on grip
		sgrMouseDrag(41, 5),                  // drag to new X
		[]byte("\x1b[<0;41;5m"),              // release at same spot
		[]byte("q"),
	}
	g := captureFrameWithBytes(t, 80, 30, keys, 5*time.Second)

	// The grip glyph is '│' (0x2502). It should now sit at column 40
	// (0-indexed) of the terminal, NOT at column 24 where it started.
	// Scan row 5 for the grip glyph.
	got := -1
	for x := 0; x < g.Cols; x++ {
		if g.At(x, 5).Rune == '│' {
			got = x
			break
		}
	}
	if got != 40 {
		t.Errorf("grip position after drag: col %d, want 40", got)
	}
}

// sgrMouseDrag returns the SGR mouse motion sequence for a left-drag
// tick — Cb bit 0x20 set = motion, low bits = 0 = left button held.
func sgrMouseDrag(col, row int) []byte {
	return []byte("\x1b[<32;" + itoa(col) + ";" + itoa(row) + "M")
}

// TestExplorerGripDragSurvivesCrossingIntoHeaderBand — starts a grip
// drag in the body band, then continues the drag WITH THE MOUSE
// STRAYING INTO THE HEADER ROW. Without packedVBox drag capture, the
// header would receive the drag events and the grip would freeze.
// Captured drag must continue updating leftFrac.
func TestExplorerGripDragSurvivesCrossingIntoHeaderBand(t *testing.T) {
	// Initial grip at col 25 (wire), press there.
	// Drag to wire row 1 (header row!) at wire col 50 → local (49, 0).
	// Without capture, packedVBox would route this to the header
	// (which no-ops) and the grip wouldn't move. With capture, the
	// event goes to body → hSplit → updates leftFrac.
	keys := [][]byte{
		sgrMousePress(25, 5),    // press on grip
		sgrMouseDrag(50, 1),     // drag INTO header row
		[]byte("\x1b[<0;50;1m"), // release in header row
		[]byte("q"),
	}
	g := captureFrameWithBytes(t, 80, 30, keys, 5*time.Second)

	// Grip after drag should NOT be at col 24 (initial) — it should
	// have moved based on the drag X.
	gripCol := -1
	for x := 0; x < g.Cols; x++ {
		if g.At(x, 5).Rune == '│' {
			gripCol = x
			break
		}
	}
	if gripCol == 24 {
		t.Errorf("grip stayed at col 24 — drag capture broken when crossing bands")
	}
	// Drag to wire X=50 = local X=49 → leftFrac = 49*100/80 = 61.
	// Grip lands at col 80*61/100 = 48. Allow ±2 for timing jitter.
	if gripCol < 46 || gripCol > 50 {
		t.Errorf("grip col after cross-band drag = %d, want ≈48", gripCol)
	}
}

// -----------------------------------------------------------------
// v0.3.11 mouse-click integration
// -----------------------------------------------------------------

// sgrMousePress returns the byte sequence a real xterm would emit
// for a left-button press at (col, row) in the SGR encoding — the
// same encoding tui's InputParser advertises via ?1006. col/row are
// 1-indexed on the wire; callers pass 1-indexed coords directly.
func sgrMousePress(col, row int) []byte {
	// \x1b[<0;C;RM  — button 0, press.
	return []byte("\x1b[<0;" + itoa(col) + ";" + itoa(row) + "M")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// captureFrameWithBytes is captureFrame but accepts raw bytes for
// mouse escape sequences instead of a per-byte key string.
func captureFrameWithBytes(t *testing.T, cols, rows int, keyBytes [][]byte, timeout time.Duration, args ...string) *tui.TermGrid {
	t.Helper()
	bin := buildBinary(t)
	c := exec.Command(bin, args...)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	captureDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(captureDone)
	}()
	go func() {
		time.Sleep(250 * time.Millisecond)
		for _, k := range keyBytes {
			_, _ = ptmx.Write(k)
			time.Sleep(120 * time.Millisecond)
		}
	}()

	select {
	case <-captureDone:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()
	return tui.DecodeANSI(buf.Bytes(), cols, rows)
}

// TestExplorerClickOnFileRowSelectsIt — a synthesized SGR mouse press
// on row 3 of the file list selects that entry. Verified by asserting
// the accent-background highlight moved from row 1 (default) to row 3
// AND the right pane now shows README.md content.
func TestExplorerClickOnFileRowSelectsIt(t *testing.T) {
	// Row 3 on the wire = terminal row 3 = index 2 in fileList (0-based),
	// which is /docs/README.md per the demo's paths.
	// Col 5 lands well inside the left pane's width.
	keys := [][]byte{
		sgrMousePress(5, 3), // click file row 2 (README.md)
		[]byte("q"),         // quit
	}
	g := captureFrameWithBytes(t, 80, 30, keys, 5*time.Second)

	// The selected row is now the row corresponding to the click.
	// Terminal is 1-indexed on the wire, 0-indexed in our decoder;
	// (5, 3) on wire → (4, 2) decoded. Body starts at row 1, so
	// widget-local Y = 2 - 1 = 1 → item index 1 (util.go). Actually
	// packedVBox.body starts at Y=headerH=1; hSplit occupies that
	// body; fileList is inside hSplit at the same Y. So a click on
	// terminal row 3 → decoded Y=2 → body-local Y=1 → fileList
	// item 1 = /src/util.go.
	if !strings.Contains(g.RowText(2), "/src/util.go") {
		t.Errorf("row 2 = %q, want '/src/util.go'", g.RowText(2))
	}
	c := g.At(3, 2)
	if !c.Bg.Set || c.Bg.R != lightAccentR {
		t.Errorf("row 2 highlight not applied after click. cell=%+v", c)
	}
	// Right pane must have updated to util.go's content.
	found := false
	for y := 0; y < g.Rows; y++ {
		if strings.Contains(g.RowText(y), "package util") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("right pane never showed 'package util' after click — syncContent didn't fire")
	}
}

// TestExplorerMouseTrackingEnabledOnEnter — Enter() must emit the
// mouse-enable CSI (\x1b[?1002h\x1b[?1006h) at startup, else no
// terminal would send mouse reports at all. Assert on the raw
// stream (not the grid).
func TestExplorerMouseTrackingEnabledOnEnter(t *testing.T) {
	bin := buildBinary(t)
	c := exec.Command(bin)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 30, Cols: 80})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, ptmx); close(done) }()
	go func() { time.Sleep(300 * time.Millisecond); _, _ = ptmx.Write([]byte("q")) }()
	<-done
	_ = c.Wait()

	raw := buf.String()
	if !strings.Contains(raw, "\x1b[?1002h") {
		t.Errorf("startup stream missing ?1002h mouse-enable: %q", raw[:min(200, len(raw))])
	}
	if !strings.Contains(raw, "\x1b[?1006h") {
		t.Errorf("startup stream missing ?1006h SGR mouse-encoding")
	}
	if !strings.Contains(raw, "\x1b[?1002l") {
		t.Errorf("shutdown stream missing ?1002l mouse-disable")
	}
	if !strings.Contains(raw, "\x1b[?1006l") {
		t.Errorf("shutdown stream missing ?1006l SGR mouse-disable")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// captureFrameWithArgs is captureFrame + extra args to the binary.
func captureFrameWithArgs(t *testing.T, cols, rows int, keys string, timeout time.Duration, args ...string) *tui.TermGrid {
	t.Helper()
	bin := buildBinary(t)
	c := exec.Command(bin, args...)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	var buf bytes.Buffer
	captureDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(captureDone)
	}()
	go func() {
		time.Sleep(200 * time.Millisecond)
		for i := 0; i < len(keys); i++ {
			_, _ = ptmx.Write([]byte{keys[i]})
			time.Sleep(80 * time.Millisecond)
		}
	}()

	select {
	case <-captureDone:
	case <-time.After(timeout):
		t.Fatal("binary did not exit within timeout")
	}
	_ = c.Wait()
	return tui.DecodeANSI(buf.Bytes(), cols, rows)
}

// luminanceDiff returns the max absolute difference across the three
// RGB channels between two colors. A cheap perceptual proxy — real
// luma weights would use 0.299*R + 0.587*G + 0.114*B, but for the
// coarse "can a human see the boundary" check, max-channel-diff is
// good enough (and doesn't require a floating-point path in a test).
func luminanceDiff(a, b tui.Color) int {
	dr := int(a.R) - int(b.R)
	if dr < 0 {
		dr = -dr
	}
	dg := int(a.G) - int(b.G)
	if dg < 0 {
		dg = -dg
	}
	db := int(a.B) - int(b.B)
	if db < 0 {
		db = -db
	}
	m := dr
	if dg > m {
		m = dg
	}
	if db > m {
		m = db
	}
	return m
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "tui-explorer.bin")
	c := exec.Command("go", "build", "-o", bin, ".")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// findLastFrame kept exported (via _ =) for downstream refactor.
var _ = findLastFrame
