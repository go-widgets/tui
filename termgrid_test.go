// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import "testing"

// TestDecodePlainText lays 5 ASCII chars into a 10x1 grid.
func TestDecodePlainText(t *testing.T) {
	g := DecodeANSI([]byte("hello"), 10, 1)
	if got := g.RowText(0); got[:5] != "hello" {
		t.Errorf("row 0 = %q, want prefix 'hello'", got)
	}
}

// TestDecodeCUPSetsCursor exercises the CSI H parser.
func TestDecodeCUPSetsCursor(t *testing.T) {
	// Move to (row 2, col 5) then write 'X'. Grid is 1-indexed CUP,
	// so the 'X' lands at (col=4, row=1) in zero-indexed.
	g := DecodeANSI([]byte("\x1b[2;5HX"), 10, 3)
	if g.At(4, 1).Rune != 'X' {
		t.Errorf("cell (4,1) = %+v, want Rune=X", g.At(4, 1))
	}
}
func TestDecodeCUPHomeNoParams(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[HX"), 10, 3)
	if g.At(0, 0).Rune != 'X' {
		t.Errorf("cell (0,0) = %+v, want Rune=X", g.At(0, 0))
	}
}
func TestDecodeCUPSingleParam(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[3HX"), 10, 5)
	// Single-param CUP: row = 3 - 1 = 2, col stays at 0.
	if g.At(0, 2).Rune != 'X' {
		t.Errorf("cell (0,2) = %+v, want Rune=X", g.At(0, 2))
	}
}

// TestDecodeSGRForeground reads a truecolor SGR and applies it.
func TestDecodeSGRForeground(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[38;2;255;0;0mA"), 5, 1)
	c := g.At(0, 0)
	if c.Rune != 'A' {
		t.Errorf("rune = %v, want A", c.Rune)
	}
	if !c.Fg.Set || c.Fg.R != 255 || c.Fg.G != 0 || c.Fg.B != 0 {
		t.Errorf("fg = %+v, want R=255", c.Fg)
	}
}
func TestDecodeSGRBackground(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[48;2;13;148;136mA"), 5, 1)
	c := g.At(0, 0)
	if !c.Bg.Set || c.Bg.R != 13 || c.Bg.G != 148 || c.Bg.B != 136 {
		t.Errorf("bg = %+v, want R=13 G=148 B=136 (teal)", c.Bg)
	}
}
func TestDecodeSGRResetClearsColors(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[38;2;255;0;0mA\x1b[0mB"), 5, 1)
	if a := g.At(0, 0); !a.Fg.Set {
		t.Errorf("first cell fg not set: %+v", a)
	}
	if b := g.At(1, 0); b.Fg.Set {
		t.Errorf("second cell fg still set after reset: %+v", b)
	}
}
func TestDecodeSGRUnknownParamsIgnored(t *testing.T) {
	// Unknown SGR params (1 = bold) should be silently skipped.
	g := DecodeANSI([]byte("\x1b[1;38;2;10;20;30mA"), 5, 1)
	if c := g.At(0, 0); !c.Fg.Set || c.Fg.R != 10 {
		t.Errorf("fg after bold prefix = %+v", c.Fg)
	}
}

// TestDecodeUnknownCSISkipped: a CSI ending in an unknown byte
// should consume its parameters and not affect the grid.
func TestDecodeUnknownCSISkipped(t *testing.T) {
	g := DecodeANSI([]byte("\x1b[?25lX"), 5, 1)
	if g.At(0, 0).Rune != 'X' {
		t.Errorf("X missing after ?25l (cursor hide): %+v", g.At(0, 0))
	}
}

// TestDecodeUTF8Boxdraw checks that '│' (U+2502) lands as a single
// rune, not the raw 0xE2 lead byte.
func TestDecodeUTF8Boxdraw(t *testing.T) {
	g := DecodeANSI([]byte("│"), 5, 1)
	if g.At(0, 0).Rune != '│' {
		t.Errorf("rune = %v, want '│'", g.At(0, 0).Rune)
	}
}
func TestDecodeUTF8TwoByte(t *testing.T) {
	g := DecodeANSI([]byte("é"), 5, 1) // é = C3 A9
	if g.At(0, 0).Rune != 'é' {
		t.Errorf("rune = %v, want 'é'", g.At(0, 0).Rune)
	}
}
func TestDecodeUTF8FourByte(t *testing.T) {
	// U+1F600 (😀) = F0 9F 98 80
	g := DecodeANSI([]byte("😀"), 5, 1)
	if g.At(0, 0).Rune != '😀' {
		t.Errorf("rune = %v, want '😀'", g.At(0, 0).Rune)
	}
}
func TestDecodeUTF8TruncatedFallsBack(t *testing.T) {
	// Truncated 2-byte sequence (0xC3 without continuation) falls
	// back to rune(0xC3).
	g := DecodeANSI([]byte{0xC3}, 5, 1)
	if g.At(0, 0).Rune != rune(0xC3) {
		t.Errorf("truncated fallback = %v, want 0xC3", g.At(0, 0).Rune)
	}
}
func TestDecodeUTF8BadContinuationBytePushedBack(t *testing.T) {
	// Lead byte 0xC3 followed by 'A' (non-continuation): should
	// fall back to 0xC3 as the rune AND process 'A' as the next
	// character.
	g := DecodeANSI([]byte{0xC3, 'A'}, 5, 1)
	if g.At(0, 0).Rune != rune(0xC3) {
		t.Errorf("cell 0 = %v, want 0xC3", g.At(0, 0).Rune)
	}
	if g.At(1, 0).Rune != 'A' {
		t.Errorf("cell 1 = %v, want 'A' after fallback", g.At(1, 0).Rune)
	}
}

// TestDecodeNewlineAdvancesRow verifies '\n' moves cursor to next row.
func TestDecodeNewlineAdvancesRow(t *testing.T) {
	g := DecodeANSI([]byte("A\nB"), 5, 3)
	if g.At(0, 0).Rune != 'A' {
		t.Errorf("row 0: %+v", g.At(0, 0))
	}
	if g.At(0, 1).Rune != 'B' {
		t.Errorf("row 1: %+v", g.At(0, 1))
	}
}
func TestDecodeCRResetsColumn(t *testing.T) {
	g := DecodeANSI([]byte("AB\rC"), 5, 1)
	if g.At(0, 0).Rune != 'C' {
		t.Errorf("cell 0 = %v, want 'C' (CR overwrote 'A')", g.At(0, 0).Rune)
	}
	if g.At(1, 0).Rune != 'B' {
		t.Errorf("cell 1 = %v, want 'B' (unchanged)", g.At(1, 0).Rune)
	}
}

// TestDecodeControlCharsSkipped: 0x00..0x1F (except ESC/LF/CR) and
// 0x7F should not land in the grid.
func TestDecodeControlCharsSkipped(t *testing.T) {
	g := DecodeANSI([]byte{'A', 0x01, 0x7F, 'B'}, 5, 1)
	if g.At(0, 0).Rune != 'A' {
		t.Errorf("cell 0 = %v", g.At(0, 0).Rune)
	}
	if g.At(1, 0).Rune != 'B' {
		t.Errorf("cell 1 = %v — control chars must not advance cursor", g.At(1, 0).Rune)
	}
}

// TestDecodeOutOfBoundsWritesDropped: cursor past grid edge writes
// nothing (no panic).
func TestDecodeOutOfBoundsWritesDropped(t *testing.T) {
	// CUP to (row 5, col 3) then write, but grid is only 4 rows.
	g := DecodeANSI([]byte("\x1b[5;3HX"), 10, 4)
	// Nothing should land at that position (it's out of range).
	// Just verify no panic + grid stays default-space.
	for y := 0; y < g.Rows; y++ {
		for x := 0; x < g.Cols; x++ {
			if g.At(x, y).Rune != ' ' {
				t.Fatalf("unexpected non-space at (%d,%d): %v", x, y, g.At(x, y).Rune)
			}
		}
	}
}

// TestAtOutOfBoundsReturnsZero.
func TestAtOutOfBoundsReturnsZero(t *testing.T) {
	g := DecodeANSI([]byte("x"), 3, 3)
	if got := g.At(-1, 0); got != (Cell{}) {
		t.Errorf("At(-1, 0) = %+v, want zero", got)
	}
	if got := g.At(0, -1); got != (Cell{}) {
		t.Errorf("At(0, -1) = %+v, want zero", got)
	}
	if got := g.At(10, 0); got != (Cell{}) {
		t.Errorf("At(10, 0) = %+v, want zero", got)
	}
	if got := g.At(0, 10); got != (Cell{}) {
		t.Errorf("At(0, 10) = %+v, want zero", got)
	}
}

// TestRowTextOutOfBounds.
func TestRowTextOutOfBounds(t *testing.T) {
	g := DecodeANSI([]byte("x"), 3, 3)
	if got := g.RowText(-1); got != "" {
		t.Errorf("RowText(-1) = %q, want empty", got)
	}
	if got := g.RowText(10); got != "" {
		t.Errorf("RowText(10) = %q, want empty", got)
	}
}
func TestRowTextIncludesSpaces(t *testing.T) {
	g := DecodeANSI([]byte("A"), 5, 1)
	got := g.RowText(0)
	if len(got) != 5 || got[0] != 'A' {
		t.Errorf("RowText = %q, want 'A    '", got)
	}
}

// TestRowReturnsSlice.
func TestRowReturnsSlice(t *testing.T) {
	g := DecodeANSI([]byte("ab"), 3, 2)
	row := g.Row(0)
	if len(row) != 3 {
		t.Errorf("Row 0 len = %d, want 3", len(row))
	}
	if row[0].Rune != 'a' || row[1].Rune != 'b' {
		t.Errorf("Row 0 = %+v", row)
	}
}

// TestDecodeTruncatedCSIRecoverable: ESC alone at EOF exits cleanly.
func TestDecodeTruncatedCSIAtEOF(t *testing.T) {
	// ESC alone at EOF must not panic; no cells written.
	g := DecodeANSI([]byte{0x1b}, 5, 1)
	if g.At(0, 0).Rune != ' ' {
		t.Errorf("cell 0 = %v, want space", g.At(0, 0).Rune)
	}
}
func TestDecodeTruncatedCSIBodyAtEOF(t *testing.T) {
	// ESC [ then EOF: body reader hits EOF, still exits cleanly.
	g := DecodeANSI([]byte("\x1b["), 5, 1)
	if g.At(0, 0).Rune != ' ' {
		t.Errorf("cell 0 = %v", g.At(0, 0).Rune)
	}
}
func TestDecodeCSIWithoutBracket(t *testing.T) {
	// ESC followed by a non-'[' byte: consume the ESC + next byte
	// and continue. The 'X' after should still land.
	g := DecodeANSI([]byte("\x1bZX"), 5, 1)
	if g.At(0, 0).Rune != 'X' {
		t.Errorf("cell 0 = %v, want 'X'", g.At(0, 0).Rune)
	}
}

// TestParseNumsHandlesTrailingBadNum: numbers with bad chars stop
// early rather than propagating an error.
func TestParseNumsHandlesTrailingBadNum(t *testing.T) {
	got := parseNums("1;bad;3")
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("parseNums truncation = %v, want [1]", got)
	}
}
func TestParseNumsEmpty(t *testing.T) {
	if got := parseNums(""); got != nil {
		t.Errorf("parseNums(\"\") = %v, want nil", got)
	}
}
// TestRowTextZeroRuneWritesSpace exercises the "r == 0" branch that
// initialisation-to-space normally hides.
func TestRowTextZeroRuneWritesSpace(t *testing.T) {
	g := &TermGrid{Rows: 1, Cols: 3, Cells: make([]Cell, 3)}
	// Cells left with zero-value Rune → RowText should render them as
	// spaces, not as literal U+0000.
	if got := g.RowText(0); got != "   " {
		t.Errorf("RowText on zero-rune cells = %q, want 3 spaces", got)
	}
}

// TestDecodeUTF8InvalidLeadByte hits the default case of the switch
// (b in 0x80..0xBF is a continuation byte with no lead — invalid).
func TestDecodeUTF8InvalidLeadByte(t *testing.T) {
	// 0x80 is a continuation byte, not a lead byte. Falls through to
	// the default branch of decodeUTF8Rune → returns rune(0x80).
	g := DecodeANSI([]byte{0x80}, 5, 1)
	if g.At(0, 0).Rune != rune(0x80) {
		t.Errorf("invalid lead = %v, want 0x80 fallback", g.At(0, 0).Rune)
	}
}

func TestParseNumsEmptyElement(t *testing.T) {
	// ";5;" → [0, 5, 0]
	got := parseNums(";5;")
	if len(got) != 3 || got[0] != 0 || got[1] != 5 || got[2] != 0 {
		t.Errorf("parseNums(\";5;\") = %v", got)
	}
}
