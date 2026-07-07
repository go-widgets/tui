// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"testing"
	"unicode/utf8"

	"github.com/go-widgets/toolkit"
)

// eventEq is a lightweight comparator used throughout the CSI
// tests: two events are equal when their Kind and Code match, X
// and Y are unused by the terminal-input parser and stay at zero.
func eventEq(a, b toolkit.Event) bool {
	return a.Kind == b.Kind && a.Code == b.Code
}

// wantEvents fails the test unless got matches want in order and
// length. Reported diffs are per-index so a mismatched sequence is
// easy to eyeball in a test-failure log.
func wantEvents(t *testing.T, got []toolkit.Event, want []toolkit.Event) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count: got %d %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if !eventEq(got[i], want[i]) {
			t.Fatalf("event[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func keyEvent(code string) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventKeyDown, Code: code}
}

func charEvent(s string) toolkit.Event {
	return toolkit.Event{Kind: toolkit.EventChar, Code: s}
}

// TestFeedPrintableASCII covers the 0x20..0x7E branch: each byte
// yields exactly one EventChar carrying that rune as its Code.
func TestFeedPrintableASCII(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte("aZ0 !~"))
	wantEvents(t, got, []toolkit.Event{
		charEvent("a"),
		charEvent("Z"),
		charEvent("0"),
		charEvent(" "),
		charEvent("!"),
		charEvent("~"),
	})
}

// TestFeedEmpty covers the "no bytes, no events, no crash" path
// used by callers that poll a non-blocking Read that returns 0.
func TestFeedEmpty(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed(nil); len(got) != 0 {
		t.Fatalf("Feed(nil): got %+v, want none", got)
	}
	if got := p.Feed([]byte{}); len(got) != 0 {
		t.Fatalf("Feed([]): got %+v, want none", got)
	}
}

// TestFeedControlBytes covers every explicitly-mapped control
// byte from the parser spec (Enter, Tab, both Backspace forms,
// Ctrl+C/D/Z) in a single table-driven pass.
func TestFeedControlBytes(t *testing.T) {
	cases := []struct {
		name string
		in   byte
		code string
	}{
		{"Enter", 0x0D, "Enter"},
		{"Tab", 0x09, "Tab"},
		{"BackspaceDEL", 0x7F, "Backspace"},
		{"BackspaceCtrlH", 0x08, "Backspace"},
		{"CtrlC", 0x03, "Ctrl+C"},
		{"CtrlD", 0x04, "Ctrl+D"},
		{"CtrlY", 0x19, "Ctrl+Y"},
		{"CtrlZ", 0x1A, "Ctrl+Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewInputParser()
			got := p.Feed([]byte{c.in})
			wantEvents(t, got, []toolkit.Event{keyEvent(c.code)})
		})
	}
}

// TestFeedUnmappedControlSilentlyConsumed covers the default
// branch of the byte-dispatch switch. NUL and LF are unmapped by
// design; they must be consumed without producing an event.
func TestFeedUnmappedControlSilentlyConsumed(t *testing.T) {
	p := NewInputParser()
	// 0x00 (NUL), 0x0A (LF), 0x0C (form feed) sandwiched between
	// two printable chars; parser must emit only the printables.
	got := p.Feed([]byte{'x', 0x00, 0x0A, 0x0C, 'y'})
	wantEvents(t, got, []toolkit.Event{charEvent("x"), charEvent("y")})
}

// TestFeedCSINamedSequences covers every named CSI sequence in the
// mapping table — one row per key so a regression in the decode
// table lights up the specific case.
func TestFeedCSINamedSequences(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		code string
	}{
		{"Up", []byte{0x1B, '[', 'A'}, "Up"},
		{"Down", []byte{0x1B, '[', 'B'}, "Down"},
		{"Right", []byte{0x1B, '[', 'C'}, "Right"},
		{"Left", []byte{0x1B, '[', 'D'}, "Left"},
		{"Home", []byte{0x1B, '[', 'H'}, "Home"},
		{"End", []byte{0x1B, '[', 'F'}, "End"},
		{"Delete", []byte{0x1B, '[', '3', '~'}, "Delete"},
		{"PageUp", []byte{0x1B, '[', '5', '~'}, "PageUp"},
		{"PageDown", []byte{0x1B, '[', '6', '~'}, "PageDown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewInputParser()
			got := p.Feed(c.in)
			wantEvents(t, got, []toolkit.Event{keyEvent(c.code)})
		})
	}
}

// TestFeedCSIUnknownConsumed covers three CSI decode-table misses:
// an unknown final byte with no params (Z), an unknown ~-final
// param (9~), and a non-'~' final with params (1;2A). All three
// must be silently consumed with no event and the parser must
// advance past the sequence so subsequent input parses cleanly.
func TestFeedCSIUnknownConsumed(t *testing.T) {
	cases := [][]byte{
		{0x1B, '[', 'Z'},           // no-params + unmapped final
		{0x1B, '[', '9', '~'},      // ~-final + unmapped param
		{0x1B, '[', '1', ';', '2', 'A'}, // params + non-~ final
	}
	for i, seq := range cases {
		// Append 'q' so we can prove the parser advanced past the
		// consumed CSI (else we'd emit no events for both reasons).
		buf := append([]byte{}, seq...)
		buf = append(buf, 'q')
		p := NewInputParser()
		got := p.Feed(buf)
		wantEvents(t, got, []toolkit.Event{charEvent("q")})
		if len(p.pending) != 0 {
			t.Fatalf("case %d: pending not drained: %v", i, p.pending)
		}
	}
}

// TestFeedCSISplitAcrossFeeds covers the incomplete-CSI branch:
// feed the sequence one byte at a time. The parser must buffer
// silently until the final byte arrives, then emit the event.
func TestFeedCSISplitAcrossFeeds(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte{0x1B}); len(got) != 0 {
		t.Fatalf("Feed(ESC): got %+v, want none", got)
	}
	if got := p.Feed([]byte{'['}); len(got) != 0 {
		t.Fatalf("Feed('['): got %+v, want none", got)
	}
	if got := p.Feed([]byte{'3'}); len(got) != 0 {
		t.Fatalf("Feed('3'): got %+v, want none", got)
	}
	got := p.Feed([]byte{'~'})
	wantEvents(t, got, []toolkit.Event{keyEvent("Delete")})
	if len(p.pending) != 0 {
		t.Fatalf("pending not drained after final byte: %v", p.pending)
	}
}

// TestFeedESCAloneHeldPending covers the "lone ESC at end of
// buffer" branch: it must NOT emit yet, staying in pending so
// the next Feed can disambiguate CSI vs Escape.
func TestFeedESCAloneHeldPending(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte{0x1B}); len(got) != 0 {
		t.Fatalf("lone ESC feed: got %+v, want none", got)
	}
	if len(p.pending) != 1 || p.pending[0] != 0x1B {
		t.Fatalf("pending: got %v, want [0x1B]", p.pending)
	}
}

// TestFlushEscapePending covers the Flush branch that emits an
// Escape when the buffered tail is (or begins with) an ESC.
func TestFlushEscapePending(t *testing.T) {
	p := NewInputParser()
	p.Feed([]byte{0x1B})
	got := p.Flush()
	wantEvents(t, got, []toolkit.Event{keyEvent("Escape")})
	if len(p.pending) != 0 {
		t.Fatalf("Flush left pending: %v", p.pending)
	}
}

// TestFlushEmpty covers the Flush no-op path — empty buffer, no
// events emitted, no crash.
func TestFlushEmpty(t *testing.T) {
	p := NewInputParser()
	if got := p.Flush(); len(got) != 0 {
		t.Fatalf("Flush() empty: got %+v, want none", got)
	}
}

// TestFlushPartialUTF8DiscardedSilently covers the Flush branch
// where the buffered tail is NOT an ESC — a partial multi-byte
// rune left over from a split UTF-8 read. Flush must clear the
// buffer without emitting.
func TestFlushPartialUTF8DiscardedSilently(t *testing.T) {
	p := NewInputParser()
	// 0xC3 is a UTF-8 leading byte awaiting a continuation.
	p.Feed([]byte{0xC3})
	if len(p.pending) == 0 {
		t.Fatal("expected pending after partial UTF-8 feed")
	}
	if got := p.Flush(); len(got) != 0 {
		t.Fatalf("Flush partial UTF-8: got %+v, want none", got)
	}
	if len(p.pending) != 0 {
		t.Fatalf("Flush left pending: %v", p.pending)
	}
}

// TestFeedESCThenNonBracketSameFeed covers the "ESC followed by a
// non-'[' byte in the SAME buffer" branch: emit Escape then
// process the follower as a fresh input.
func TestFeedESCThenNonBracketSameFeed(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte{0x1B, 'a'})
	wantEvents(t, got, []toolkit.Event{keyEvent("Escape"), charEvent("a")})
}

// TestFeedESCThenNonBracketAcrossFeeds covers the same branch
// spread over two Feed calls — the pending ESC must merge with
// the next byte, emit Escape, then process 'a'.
func TestFeedESCThenNonBracketAcrossFeeds(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte{0x1B}); len(got) != 0 {
		t.Fatalf("Feed(ESC): got %+v, want none", got)
	}
	got := p.Feed([]byte{'a'})
	wantEvents(t, got, []toolkit.Event{keyEvent("Escape"), charEvent("a")})
}

// TestFeedESCThenBracketAcrossFeeds covers the CSI-start branch
// after the pending ESC merges with the next byte '['.
func TestFeedESCThenBracketAcrossFeeds(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte{0x1B}); len(got) != 0 {
		t.Fatalf("Feed(ESC): got %+v, want none", got)
	}
	got := p.Feed([]byte{'[', 'A'})
	wantEvents(t, got, []toolkit.Event{keyEvent("Up")})
}

// TestFeedUTF8SingleFeed covers a multi-byte rune arriving whole:
// "é" is C3 A9 in UTF-8 — one EventChar with the decoded rune.
func TestFeedUTF8SingleFeed(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte("é"))
	wantEvents(t, got, []toolkit.Event{charEvent("é")})
}

// TestFeedUTF8SplitAcrossFeeds covers a rune whose bytes fall on a
// Read boundary: the first Feed must buffer, the second must
// emit the completed rune.
func TestFeedUTF8SplitAcrossFeeds(t *testing.T) {
	full := []byte("é") // 2 bytes: 0xC3 0xA9
	p := NewInputParser()
	if got := p.Feed(full[:1]); len(got) != 0 {
		t.Fatalf("Feed(partial): got %+v, want none", got)
	}
	if len(p.pending) == 0 {
		t.Fatal("expected pending after partial UTF-8 feed")
	}
	got := p.Feed(full[1:])
	wantEvents(t, got, []toolkit.Event{charEvent("é")})
}

// TestFeedUTF8InvalidWithFourBytesRemainingEmits covers the sz=1
// utf8.RuneError branch where the buffer is >= 4 bytes: DecodeRune
// commits to "this really is invalid, don't wait for more". The
// parser must emit a single RuneError char and advance one byte
// so the following bytes are re-decoded independently.
func TestFeedUTF8InvalidWithFourBytesRemainingEmits(t *testing.T) {
	p := NewInputParser()
	// Four 0xFF bytes — each is an invalid UTF-8 leading byte.
	// The first three would ordinarily be buffered under the
	// partial-rune rule, but with four remaining DecodeRune emits
	// RuneError for the first, then the next byte gets re-decoded
	// (again invalid, three remaining → buffered).
	got := p.Feed([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	// First byte flushed as RuneError; last three buffered.
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(got), got)
	}
	if got[0].Kind != toolkit.EventChar {
		t.Fatalf("kind: got %v, want EventChar", got[0].Kind)
	}
	if got[0].Code != string(utf8.RuneError) {
		t.Fatalf("code: got %q, want RuneError rune", got[0].Code)
	}
	if len(p.pending) != 3 {
		t.Fatalf("pending: got %d bytes, want 3", len(p.pending))
	}
}

// TestFeedChainedKeystrokes covers ordering across three distinct
// event kinds in a single Feed — the "abc\x1b[A\x0d" example from
// the parser spec.
func TestFeedChainedKeystrokes(t *testing.T) {
	p := NewInputParser()
	got := p.Feed([]byte("abc\x1b[A\x0d"))
	wantEvents(t, got, []toolkit.Event{
		charEvent("a"),
		charEvent("b"),
		charEvent("c"),
		keyEvent("Up"),
		keyEvent("Enter"),
	})
}

// TestFeedCSIIncompleteAfterBracketBuffered covers the
// "ESC [ … no final byte yet" branch: after the opening two
// bytes plus any params, if the final byte hasn't arrived the
// entire partial sequence must sit in pending.
func TestFeedCSIIncompleteAfterBracketBuffered(t *testing.T) {
	p := NewInputParser()
	if got := p.Feed([]byte{0x1B, '[', '3'}); len(got) != 0 {
		t.Fatalf("partial CSI: got %+v, want none", got)
	}
	if len(p.pending) != 3 {
		t.Fatalf("pending: got %d bytes %v, want 3 (ESC,[,3)", len(p.pending), p.pending)
	}
}
