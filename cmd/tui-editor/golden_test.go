// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// Golden-frame integration tests for cmd/tui-editor. Same shape as
// cmd/tui-explorer/golden_test.go: capture the whole 80×30 grid,
// serialise to testdata/*.json + a human-readable ASCII sidecar,
// compare on every future run.
//
// UPDATE_GOLDEN=1 go test -tags integration -run TestEditorGolden ./cmd/tui-editor/
//   → regenerates goldens. Review the ASCII sidecars, commit if OK.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/go-widgets/tui"
)

type goldenFrame struct {
	Cols  int          `json:"cols"`
	Rows  int          `json:"rows"`
	Cells []goldenCell `json:"cells"`
}

type goldenCell struct {
	Rune int32  `json:"r"`
	Fg   [4]int `json:"fg"`
	Bg   [4]int `json:"bg"`
}

func toGolden(g *tui.TermGrid) goldenFrame {
	out := goldenFrame{Cols: g.Cols, Rows: g.Rows, Cells: make([]goldenCell, g.Cols*g.Rows)}
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			c := g.At(x, y)
			cc := goldenCell{Rune: int32(c.Rune)}
			if c.Fg.Set {
				cc.Fg = [4]int{int(c.Fg.R), int(c.Fg.G), int(c.Fg.B), 1}
			}
			if c.Bg.Set {
				cc.Bg = [4]int{int(c.Bg.R), int(c.Bg.G), int(c.Bg.B), 1}
			}
			out.Cells[y*g.Cols+x] = cc
		}
	}
	return out
}

func renderASCII(g *tui.TermGrid) string {
	var b strings.Builder
	for y := 0; y < g.Rows; y++ {
		fmt.Fprintf(&b, "%02d|", y)
		for x := 0; x < g.Cols; x++ {
			c := g.At(x, y).Bg
			switch {
			case !c.Set:
				b.WriteByte('.')
			case c.R == 0xFA && c.G == 0xFA && c.B == 0xFA:
				b.WriteByte('B') // Background (light)
			case c.R == 0x14 && c.G == 0x16 && c.B == 0x1A:
				b.WriteByte('B') // Background (dark)
			case c.R == 0xE8 && c.G == 0xEA && c.B == 0xED:
				b.WriteByte('S') // Surface (light)
			case c.R == 0x1F && c.G == 0x22 && c.B == 0x28:
				b.WriteByte('S') // Surface (dark)
			case c.R == 0x35 && c.G == 0x84 && c.B == 0xE4:
				b.WriteByte('A') // Accent (light)
			case c.R == 0x4F && c.G == 0x9D && c.B == 0xF2:
				b.WriteByte('A') // Accent (dark)
			case c.R == 0xD0 && c.G == 0xD4 && c.B == 0xD8:
				b.WriteByte('a') // SurfaceAlt (light)
			case c.R == 0x2A && c.G == 0x2E && c.B == 0x36:
				b.WriteByte('a') // SurfaceAlt (dark)
			case c.R == 0xB0 && c.G == 0xB4 && c.B == 0xB8:
				b.WriteByte('b') // Border (light)
			case c.R == 0x3A && c.G == 0x3E && c.B == 0x46:
				b.WriteByte('b') // Border (dark)
			default:
				b.WriteByte('?')
			}
		}
		b.WriteString("|\n")
		fmt.Fprintf(&b, "  |%s|\n", g.RowText(y))
	}
	return b.String()
}

func diffGoldens(want, got goldenFrame) string {
	if want.Cols != got.Cols || want.Rows != got.Rows {
		return fmt.Sprintf("dimensions differ: want %d×%d, got %d×%d",
			want.Cols, want.Rows, got.Cols, got.Rows)
	}
	var b strings.Builder
	diffs := 0
	for i := range want.Cells {
		if want.Cells[i] != got.Cells[i] {
			x, y := i%want.Cols, i/want.Cols
			fmt.Fprintf(&b, "  (%d,%d) want %+v got %+v\n",
				x, y, want.Cells[i], got.Cells[i])
			diffs++
			if diffs > 30 {
				fmt.Fprintf(&b, "  ... (%d more diffs suppressed)\n", len(want.Cells)-i-1)
				break
			}
		}
	}
	if diffs == 0 {
		return ""
	}
	return b.String()
}

// captureFrame + captureFrameWithArgs mirror the tui-explorer harness
// so this test file is self-contained.
func captureFrame(t *testing.T, cols, rows int, keys string, timeout time.Duration) *tui.TermGrid {
	return captureFrameWithArgs(t, cols, rows, keys, timeout)
}

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

func TestEditorGoldenInitialFrame(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	compareOrUpdateGolden(t, g, "testdata/editor-initial.json")
}

func TestEditorGoldenAfterEnteringEditMode(t *testing.T) {
	g := captureFrame(t, 80, 30, "i\x1bq", 4*time.Second)
	compareOrUpdateGolden(t, g, "testdata/editor-edit-mode-then-exit.json")
}

func TestEditorGoldenDarkTheme(t *testing.T) {
	g := captureFrameWithArgs(t, 80, 30, "q", 3*time.Second, "--theme=dark")
	compareOrUpdateGolden(t, g, "testdata/editor-dark.json")
}

// TestEditorHighlightsGoFileEndToEnd drives the REAL editor binary in a pty on
// a real .go file and asserts the buffer is syntax-highlighted at the cell
// level: the "func" keyword must carry the light keyword hue (#A626A4), not the
// default foreground. (We target "func" on line 3 rather than "package" on line
// 1, whose first glyph sits under the block cursor.) The golden frames use
// plain buffers, so this is the only end-to-end proof that highlighting
// survives the full render pipeline.
func TestEditorHighlightsGoFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sample.go")
	if err := os.WriteFile(f, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := captureFrameWithArgs(t, 80, 30, "q", 3*time.Second, "--file="+f)
	keyword := tui.Color{R: 0xA6, G: 0x26, B: 0xA4, Set: true}
	for y := 0; y < g.Rows; y++ {
		row := g.RowText(y)
		x := strings.Index(row, "func ")
		if x < 0 {
			continue
		}
		if got := g.At(x, y).Fg; got != keyword {
			t.Fatalf("'func' cell (%d,%d) fg = %+v, want keyword %+v", x, y, got, keyword)
		}
		// A plain identifier on the same line keeps the default foreground.
		if fx := strings.Index(row, "main"); fx >= 0 && g.At(fx, y).Fg == keyword {
			t.Fatalf("'main' should not be keyword-coloured")
		}
		return
	}
	t.Skip("'func' not visible in the captured frame")
}

// TestEditorUndoEndToEnd drives the REAL editor binary in a pty: enter edit
// mode ('i'), type "xy", undo with Ctrl+Z (0x1A), leave edit mode (Esc), quit.
// The buffer must revert to "x" -- proving undo works through the parser (which
// must emit "Ctrl+Z") and the tui.TextEditor OnEvent path.
func TestEditorUndoEndToEnd(t *testing.T) {
	g := captureFrame(t, 80, 30, "ixy\x1a\x1bq", 4*time.Second)
	var full strings.Builder
	for y := 0; y < g.Rows; y++ {
		full.WriteString(g.RowText(y))
		full.WriteByte('\n')
	}
	grid := full.String()
	if strings.Contains(grid, "xy") {
		t.Fatalf("undo did not remove the 'y' — grid still contains \"xy\"")
	}
	if !strings.Contains(grid, "x") {
		t.Fatal("expected the surviving 'x' in the buffer, none visible")
	}
}

func compareOrUpdateGolden(t *testing.T, g *tui.TermGrid, path string) {
	t.Helper()
	got := toGolden(g)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		asciiPath := strings.TrimSuffix(path, ".json") + ".txt"
		if err := os.WriteFile(asciiPath, []byte(renderASCII(g)), 0o644); err != nil {
			t.Fatalf("write ASCII sidecar: %v", err)
		}
		t.Logf("golden updated: %s (+ %s)", path, asciiPath)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", path, err)
	}
	var want goldenFrame
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("unmarshal golden %s: %v", path, err)
	}

	if diff := diffGoldens(want, got); diff != "" {
		asciiPath := strings.TrimSuffix(path, ".json") + ".txt"
		wantASCII, _ := os.ReadFile(asciiPath)
		t.Errorf("frame diverged from golden %s:\n%s\n---want-ascii---\n%s\n---got-ascii---\n%s",
			path, diff, string(wantASCII), renderASCII(g))
	}
}

// TestEditorBodyChromeContrast — same discipline as tui-explorer.
func TestEditorBodyChromeContrast(t *testing.T) {
	for _, tc := range []struct {
		name   string
		darkTh bool
	}{
		{"light", false},
		{"dark", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g *tui.TermGrid
			if tc.darkTh {
				g = captureFrameWithArgs(t, 80, 30, "q", 3*time.Second, "--theme=dark")
			} else {
				g = captureFrame(t, 80, 30, "q", 3*time.Second)
			}
			chrome := g.At(0, 0).Bg
			body := g.At(0, 5).Bg
			if !chrome.Set || !body.Set {
				t.Fatalf("bg not set: chrome=%+v body=%+v", chrome, body)
			}
			if d := luminanceDiff(chrome, body); d < 16 {
				t.Fatalf("chrome/body contrast %d < 16 (imperceptible boundary): chrome=(%d,%d,%d) body=(%d,%d,%d)",
					d, chrome.R, chrome.G, chrome.B, body.R, body.G, body.B)
			}
		})
	}
}

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
