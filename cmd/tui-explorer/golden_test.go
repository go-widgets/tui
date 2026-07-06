// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix && integration

// Golden-frame integration test. Captures the ENTIRE 80×30 grid
// after a specific input sequence and compares it byte-for-byte
// against a checked-in golden file. This is the last-resort test:
// any deviation in rune, foreground, or background at any cell
// fails loud with a diff-of-frames.
//
// Rationale for existing alongside the cell-level spot-checks:
// spot-checks catch bugs I've thought of. A golden-frame test
// catches bugs I HAVEN'T thought of — the exact class of bug the
// v0.3.0 through v0.3.6 patches all shipped despite passing every
// prior test.
//
// UPDATE-GOLDEN=1 go test -tags integration -run TestExplorerGolden ./cmd/tui-explorer/
//   → regenerates the golden files. Review the diff, commit if OK.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-widgets/tui"
)

// goldenFrame is a serialisable, diff-friendly form of the grid.
// Cells is row-major: one entry per (y*Cols+x). Storing (R,G,B,Set)
// tuples keeps the JSON diff readable when a color changes.
type goldenFrame struct {
	Cols int          `json:"cols"`
	Rows int          `json:"rows"`
	Cells []goldenCell `json:"cells"`
}

type goldenCell struct {
	Rune int32 `json:"r"`
	Fg   [4]int `json:"fg"` // R, G, B, Set (0/1)
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

// renderASCII returns a human-readable dump of the frame. Every row
// shows its RowText plus a compact color banner ("A" for accent bg,
// "S" for surface bg, "." for unset). This is what a failing test
// prints so a maintainer eyeballs the diff without opening the JSON.
func renderASCII(g *tui.TermGrid) string {
	var b strings.Builder
	for y := 0; y < g.Rows; y++ {
		fmt.Fprintf(&b, "%02d|", y)
		// Color banner — one char per cell.
		for x := 0; x < g.Cols; x++ {
			c := g.At(x, y).Bg
			switch {
			case !c.Set:
				b.WriteByte('.')
			case c.R == 0xFA && c.G == 0xFA && c.B == 0xFA:
				b.WriteByte('B') // Background
			case c.R == 0xE8 && c.G == 0xEA && c.B == 0xED:
				b.WriteByte('S') // Surface
			case c.R == 0x35 && c.G == 0x84 && c.B == 0xE4:
				b.WriteByte('A') // Accent
			case c.R == 0xD0 && c.G == 0xD4 && c.B == 0xD8:
				b.WriteByte('a') // SurfaceAlt (cellPopover overlay bg)
			case c.R == 0xB0 && c.G == 0xB4 && c.B == 0xB8:
				b.WriteByte('b') // Border stroke
			default:
				b.WriteByte('?')
			}
		}
		b.WriteString("|\n")
		fmt.Fprintf(&b, "  |%s|\n", g.RowText(y))
	}
	return b.String()
}

// diffGoldens returns a compact list of cells that differ.
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

// TestExplorerGoldenInitialFrame captures the frame right after
// startup (no input beyond 'q' to quit) and compares against the
// checked-in golden. Any regression in ANY cell of the 80×30 grid
// fails this test with a diff of exactly which cells changed.
func TestExplorerGoldenInitialFrame(t *testing.T) {
	g := captureFrame(t, 80, 30, "q", 3*time.Second)
	compareOrUpdateGolden(t, g, "testdata/explorer-initial.json")
}

// TestExplorerGoldenAfterDownArrow captures the frame after one
// Down arrow (row 2 selected). Catches any regression in how arrow
// navigation updates the display.
func TestExplorerGoldenAfterDownArrow(t *testing.T) {
	g := captureFrame(t, 80, 30, "\x1b[Bq", 3*time.Second)
	compareOrUpdateGolden(t, g, "testdata/explorer-down.json")
}

// TestExplorerGoldenWithHelpPopover captures the frame with the
// help popover visible.
func TestExplorerGoldenWithHelpPopover(t *testing.T) {
	g := captureFrame(t, 80, 30, "?q", 3*time.Second)
	compareOrUpdateGolden(t, g, "testdata/explorer-help.json")
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
		// Also write the ASCII sidecar so a reviewer can eyeball
		// the frame in the git diff without running any tool.
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
