// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

// SGR mouse-report tests. The InputParser must accept:
//   - `\x1b[<0;X;YM` (left press)   → EventClick with (X-1, Y-1)
//   - `\x1b[<0;X;Ym` (left release) → silently consumed, no event
//   - `\x1b[<32;X;YM` (motion/drag) → silently consumed, no event
//   - `\x1b[<64;X;YM` (scroll)      → silently consumed, no event
//   - `\x1b[<1;X;YM` (middle press) → silently consumed
//   - `\x1b[<2;X;YM` (right press)  → silently consumed (for now)
// and never crash on malformed payloads.

func TestMouseSGRLeftPressEmitsClick(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<0;12;5M"))
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d: %+v", len(got), got)
	}
	want := toolkit.Event{Kind: toolkit.EventClick, X: 11, Y: 4}
	if got[0] != want {
		t.Fatalf("event = %+v, want %+v", got[0], want)
	}
}

func TestMouseSGRLeftPressAtOrigin(t *testing.T) {
	// SGR coords are 1-indexed; the parser subtracts 1. A report of
	// (1,1) maps to (0,0). Guard against off-by-one underflow.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<0;1;1M"))
	if len(got) != 1 || got[0].X != 0 || got[0].Y != 0 {
		t.Fatalf("origin click misdecoded: %+v", got)
	}
}

func TestMouseSGRZeroCoordsAreClamped(t *testing.T) {
	// A defensive report of (0,0) — which no real terminal should
	// emit but we handle gracefully — must clamp to (0,0), not
	// underflow to -1.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<0;0;0M"))
	if len(got) != 1 || got[0].X != 0 || got[0].Y != 0 {
		t.Fatalf("(0,0) misdecoded: %+v", got)
	}
}

func TestMouseSGRLeftReleaseEmitsUp(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<0;5;5m"))
	if len(got) != 1 {
		t.Fatalf("release must emit exactly 1 event, got %+v", got)
	}
	want := toolkit.Event{Kind: toolkit.EventMouseUp, X: 4, Y: 4}
	if got[0] != want {
		t.Fatalf("release event = %+v, want %+v", got[0], want)
	}
}

func TestMouseSGRLeftMotionEmitsDrag(t *testing.T) {
	// bit 0x20 set = drag/motion; low bits = 0 = left button held.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<32;10;7M"))
	if len(got) != 1 {
		t.Fatalf("drag must emit exactly 1 event, got %+v", got)
	}
	want := toolkit.Event{Kind: toolkit.EventMouseDrag, X: 9, Y: 6}
	if got[0] != want {
		t.Fatalf("drag event = %+v, want %+v", got[0], want)
	}
}

func TestMouseSGRScrollWheelIsConsumed(t *testing.T) {
	// bit 0x40 set = scroll wheel.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<64;5;5M"))
	if len(got) != 0 {
		t.Fatalf("scroll must not emit an event, got %+v", got)
	}
}

func TestMouseSGRMiddleAndRightAreConsumed(t *testing.T) {
	// Middle (button code 1) and right (button code 2) presses are
	// consumed silently for now; only left produces an EventClick.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<1;5;5M"))
	if len(got) != 0 {
		t.Fatalf("middle press must not emit an event, got %+v", got)
	}
	got = p.Feed([]byte("\x1b[<2;5;5M"))
	if len(got) != 0 {
		t.Fatalf("right press must not emit an event, got %+v", got)
	}
}

func TestMouseSGRMalformedIsSilentlyConsumed(t *testing.T) {
	// Missing coord fields, non-decimal payload, > 3 semicolons —
	// each must silently consume the bytes without emitting or
	// panicking. The important guarantee is that the byte stream
	// advances past the CSI and the next event flows through.
	cases := [][]byte{
		[]byte("\x1b[<0;5M"),     // only two fields
		[]byte("\x1b[<0;5;5;5M"), // four fields
		[]byte("\x1b[<M"),        // empty payload
		// `?` (0x3F) is a CSI parameter byte so the outer CSI parser
		// keeps scanning until `M`; the inner mouse parser then hits
		// its non-digit guard branch.
		[]byte("\x1b[<0;?;5M"),
		[]byte("\x1b[<?;5;5M"),
		[]byte("\x1b[<0;5;?M"),
	}
	for i, c := range cases {
		p := NewInputParser()
		got := p.Feed(c)
		if len(got) != 0 {
			t.Errorf("case %d %q: want no event, got %+v", i, c, got)
		}
	}
}

func TestMouseSGRLargeCoordinates(t *testing.T) {
	// SGR is specifically the encoding for terminals with > 223
	// columns/rows — verify a big value round-trips.
	p := NewInputParser()
	got := p.Feed([]byte("\x1b[<0;500;300M"))
	if len(got) != 1 || got[0].X != 499 || got[0].Y != 299 {
		t.Fatalf("large coords misdecoded: %+v", got)
	}
}

func TestMouseSGRPresstakesPrecedenceOverInterleavedKeys(t *testing.T) {
	// A byte stream that mixes a keypress and a mouse press must
	// produce both events in order. Guards against the SGR handler
	// eating bytes past its own CSI final.
	p := NewInputParser()
	got := p.Feed([]byte("a\x1b[<0;3;2Mb"))
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d: %+v", len(got), got)
	}
	if got[0].Kind != toolkit.EventChar || got[0].Code != "a" {
		t.Fatalf("event 0 = %+v, want char 'a'", got[0])
	}
	if got[1].Kind != toolkit.EventClick || got[1].X != 2 || got[1].Y != 1 {
		t.Fatalf("event 1 = %+v, want click(2,1)", got[1])
	}
	if got[2].Kind != toolkit.EventChar || got[2].Code != "b" {
		t.Fatalf("event 2 = %+v, want char 'b'", got[2])
	}
}
