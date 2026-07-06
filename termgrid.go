// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"bytes"
	"strconv"
	"strings"
)

// Cell is one grid position after ANSI-decoding a terminal frame:
// the rune actually written, plus the foreground and background
// colors that were active when it was written. All zero-value colors
// mean "default" (nothing set). Cell exists so consumer tests can
// assert on the visual output at cell-precision — the previous
// text-strip protocol dropped color information and let bugs like
// "highlight rendered as `█` characters" ship undetected.
type Cell struct {
	Rune rune
	Fg   Color
	Bg   Color
}

// Color carries the 24-bit RGB values decoded from a CSI SGR "38;2"
// (foreground) or "48;2" (background) sequence. Zero value means
// "default" — SGR 0 or no color set.
type Color struct {
	R, G, B uint8
	Set     bool // false = default/reset, true = explicitly set
}

// TermGrid is a 2D cell buffer produced by DecodeANSI. Rows and Cols
// are the grid dimensions; Cells is row-major (Cells[y*Cols+x]).
type TermGrid struct {
	Rows  int
	Cols  int
	Cells []Cell
}

// At returns the cell at (x, y). Out-of-bounds coordinates return
// the zero Cell — callers that care about bounds should check dims.
func (g *TermGrid) At(x, y int) Cell {
	if x < 0 || y < 0 || x >= g.Cols || y >= g.Rows {
		return Cell{}
	}
	return g.Cells[y*g.Cols+x]
}

// Row returns the y-th row as a Cell slice. Panics on out-of-bounds
// y — mirrors slice indexing conventions.
func (g *TermGrid) Row(y int) []Cell {
	return g.Cells[y*g.Cols : (y+1)*g.Cols]
}

// RowText joins the Rune of every cell in row y into a string. Non-
// printable / space runes stay as ' '. Useful for coarse "does row Y
// contain this text" assertions without giving up the color info the
// underlying grid still carries.
func (g *TermGrid) RowText(y int) string {
	if y < 0 || y >= g.Rows {
		return ""
	}
	var b strings.Builder
	for x := 0; x < g.Cols; x++ {
		r := g.At(x, y).Rune
		if r == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DecodeANSI parses stream as a sequence of CSI SGR + printable
// runes + cursor-positioning + line breaks, and lays every printable
// rune into a Cell at the current cursor position with the current
// foreground / background color. Only the subset of CSI a
// painter.CellPainter emits is honored:
//
//   - CSI H          → cursor to (1, 1)
//   - CSI r ; c H    → cursor to (c, r), 1-indexed
//   - CSI 0 m        → reset colors
//   - CSI 38 ; 2 ; R ; G ; B m → foreground truecolor
//   - CSI 48 ; 2 ; R ; G ; B m → background truecolor
//   - CSI ?1049 h / l → alt-screen enter/leave (recognised, no-op)
//   - CSI ?25 h / l   → cursor show/hide (recognised, no-op)
//
// Unknown CSI sequences consume their bytes and are ignored so a
// caller can freely pass a frame that also contains cursor styling
// or bracketed paste. \n and \r reset the cursor to column 0 and
// advance the row (LF) or stay on the same row (CR).
func DecodeANSI(stream []byte, cols, rows int) *TermGrid {
	g := &TermGrid{Rows: rows, Cols: cols, Cells: make([]Cell, cols*rows)}
	// Initialise every cell to a space so RowText returns spaces
	// for untouched positions rather than the zero rune.
	for i := range g.Cells {
		g.Cells[i].Rune = ' '
	}
	var (
		cx, cy int
		fg, bg Color
	)
	r := bytes.NewReader(stream)
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if b == 0x1b {
			// Consume '['.
			c, err := r.ReadByte()
			if err != nil {
				break
			}
			if c != '[' {
				continue
			}
			// Read the CSI sequence body until a final byte
			// (0x40..0x7E). Extras: '?' can appear right after '['.
			var body strings.Builder
			for {
				c, err = r.ReadByte()
				if err != nil {
					break
				}
				if c >= 0x40 && c <= 0x7E {
					handleCSI(g, body.String(), c, &cx, &cy, &fg, &bg)
					break
				}
				body.WriteByte(c)
			}
			continue
		}
		if b == '\n' {
			cy++
			cx = 0
			continue
		}
		if b == '\r' {
			cx = 0
			continue
		}
		if b < 0x20 || b == 0x7F {
			continue
		}
		// Decode UTF-8 multi-byte runes so box-drawing characters
		// (── │ ┌ ┐ └ ┘) land as their real rune, not as the raw
		// first byte.
		rn := decodeUTF8Rune(b, r)
		if cy >= 0 && cy < rows && cx >= 0 && cx < cols {
			g.Cells[cy*cols+cx] = Cell{Rune: rn, Fg: fg, Bg: bg}
		}
		cx++
	}
	return g
}

// decodeUTF8Rune consumes the trailing continuation bytes of a UTF-8
// sequence when b is a lead byte. Returns the decoded rune. Falls
// back to rune(b) on invalid sequences.
func decodeUTF8Rune(b byte, r *bytes.Reader) rune {
	if b < 0x80 {
		return rune(b)
	}
	var need int
	var val rune
	switch {
	case b&0xE0 == 0xC0:
		need = 1
		val = rune(b & 0x1F)
	case b&0xF0 == 0xE0:
		need = 2
		val = rune(b & 0x0F)
	case b&0xF8 == 0xF0:
		need = 3
		val = rune(b & 0x07)
	default:
		return rune(b)
	}
	for i := 0; i < need; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return rune(b)
		}
		if c&0xC0 != 0x80 {
			// Not a continuation byte — push back and bail.
			_ = r.UnreadByte()
			return rune(b)
		}
		val = (val << 6) | rune(c&0x3F)
	}
	return val
}

func handleCSI(g *TermGrid, params string, final byte, cx, cy *int, fg, bg *Color) {
	switch final {
	case 'H':
		nums := parseNums(strings.TrimPrefix(params, "?"))
		if len(nums) >= 2 {
			*cy = nums[0] - 1
			*cx = nums[1] - 1
		} else if len(nums) == 1 {
			*cy = nums[0] - 1
			*cx = 0
		} else {
			*cx, *cy = 0, 0
		}
	case 'm':
		nums := parseNums(params)
		for i := 0; i < len(nums); i++ {
			switch nums[i] {
			case 0:
				*fg = Color{}
				*bg = Color{}
			case 38:
				if i+4 < len(nums) && nums[i+1] == 2 {
					*fg = Color{
						R:   uint8(nums[i+2]),
						G:   uint8(nums[i+3]),
						B:   uint8(nums[i+4]),
						Set: true,
					}
					i += 4
				}
			case 48:
				if i+4 < len(nums) && nums[i+1] == 2 {
					*bg = Color{
						R:   uint8(nums[i+2]),
						G:   uint8(nums[i+3]),
						B:   uint8(nums[i+4]),
						Set: true,
					}
					i += 4
				}
			}
		}
	default:
		// Unknown CSI — ignore. Common candidates: '?25l' (hide
		// cursor), '?1049h' (alt-screen), 'K' (erase), etc. The
		// grid contents are unaffected.
	}
	_ = g
}

func parseNums(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}
