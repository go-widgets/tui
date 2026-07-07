// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"unicode/utf8"

	"github.com/go-widgets/toolkit"
)

// InputParser reads bytes from a terminal stdin stream and emits
// [toolkit.Event] values. A single call to [InputParser.Feed] may
// produce zero, one, or many events: typing "abc" yields three
// [toolkit.EventChar] events, pasting a control sequence like
// "\x1b[A" yields one arrow-up event but consumes three bytes.
//
// The toolkit exposes [toolkit.EventKeyDown] for named-key presses
// (Enter, Tab, arrows, Escape, …) and [toolkit.EventChar] for
// printable characters — the parser routes accordingly.
//
// The pending buffer holds the incomplete tail of the last Feed
// call: a lone ESC (0x1B) whose next byte has not arrived, an ESC
// [ … partial CSI sequence still waiting for its final byte
// (0x40..0x7E), or a partial UTF-8 rune (1–3 bytes of a multi-byte
// codepoint split across two Reads). It is consumed automatically
// on the next Feed call, or drained by [InputParser.Flush] on an
// input-idle deadline.
type InputParser struct {
	// pending is the incomplete tail of the previous Feed call.
	pending []byte
}

// NewInputParser returns a fresh parser with an empty buffer.
func NewInputParser() *InputParser {
	return &InputParser{}
}

// Feed hands b bytes to the parser and returns any events that
// completed as a result. Bytes belonging to an incomplete sequence
// (a lone ESC, a partial CSI, a partial UTF-8 rune) are buffered
// internally and consumed on the next Feed call.
//
// The classic Escape-vs-CSI ambiguity is resolved as follows: a
// lone ESC at the end of the buffer is held pending; on the next
// Feed a leading '[' starts a CSI sequence, any other byte causes
// the held ESC to be emitted as "Escape" and the following byte is
// then processed as a fresh input. Callers waiting on an idle
// deadline flush the held ESC via [InputParser.Flush].
func (p *InputParser) Feed(b []byte) []toolkit.Event {
	stream := append(p.pending, b...)
	p.pending = nil
	var events []toolkit.Event

	i := 0
	for i < len(stream) {
		c := stream[i]
		switch {
		case c == 0x1B:
			// ESC — need at least one more byte to disambiguate.
			if i+1 >= len(stream) {
				p.pending = append(p.pending, stream[i:]...)
				return events
			}
			if stream[i+1] == '[' {
				// Look for a CSI final byte in 0x40..0x7E.
				end := -1
				for j := i + 2; j < len(stream); j++ {
					if stream[j] >= 0x40 && stream[j] <= 0x7E {
						end = j
						break
					}
				}
				if end < 0 {
					// Incomplete CSI — buffer the whole sequence.
					p.pending = append(p.pending, stream[i:]...)
					return events
				}
				if evs, ok := decodeCSIN(stream[i+2:end], stream[end]); ok {
					events = append(events, evs...)
				}
				i = end + 1
			} else {
				// ESC followed by a non-'[' byte — emit Escape and
				// reprocess the following byte as fresh input.
				events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Escape"})
				i++
			}
		case c == 0x0D:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Enter"})
			i++
		case c == 0x09:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Tab"})
			i++
		case c == 0x7F || c == 0x08:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Backspace"})
			i++
		case c == 0x03:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+C"})
			i++
		case c == 0x04:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+D"})
			i++
		case c == 0x10:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+P"})
			i++
		case c == 0x13:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+S"})
			i++
		case c == 0x16:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+V"})
			i++
		case c == 0x18:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+X"})
			i++
		case c == 0x19:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+Y"})
			i++
		case c == 0x1A:
			events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Ctrl+Z"})
			i++
		case c >= 0x20 && c <= 0x7E:
			events = append(events, toolkit.Event{Kind: toolkit.EventChar, Code: string(rune(c))})
			i++
		case c >= 0x80:
			// Multi-byte UTF-8. If DecodeRune reports an invalid
			// leading byte AND we have fewer than 4 bytes left,
			// treat it as a possibly-split rune and buffer.
			r, sz := utf8.DecodeRune(stream[i:])
			if r == utf8.RuneError && sz == 1 && len(stream)-i < 4 {
				p.pending = append(p.pending, stream[i:]...)
				return events
			}
			events = append(events, toolkit.Event{Kind: toolkit.EventChar, Code: string(r)})
			i += sz
		default:
			// Unmapped control byte (NUL, LF, form-feed, …) —
			// silently consume so the caller's stream advances.
			i++
		}
	}
	return events
}

// Flush emits any pending ESC (or partial CSI) as an "Escape"
// event and clears the buffer. Callers invoke it on an input-idle
// deadline to resolve the Escape-vs-CSI ambiguity in favour of a
// plain Escape. A partial UTF-8 rune is discarded silently — its
// tail bytes would be indistinguishable from an unrelated new
// keystroke by the time an idle timeout fires.
func (p *InputParser) Flush() []toolkit.Event {
	var events []toolkit.Event
	if len(p.pending) > 0 && p.pending[0] == 0x1B {
		events = append(events, toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Escape"})
	}
	p.pending = nil
	return events
}

// decodeCSIN maps the parameter bytes and final byte of a CSI
// sequence (the portion after "ESC [") to zero or more [toolkit.Event]
// values. The second return is false for any unmapped sequence,
// signalling the caller to silently consume without emitting.
//
// A single CSI can produce zero events (e.g. a mouse drag we suppress,
// a mouse release we suppress) or one event (a keypress, a mouse press
// promoted to EventClick).
func decodeCSIN(params []byte, final byte) ([]toolkit.Event, bool) {
	// SGR mouse report: `CSI < Cb ; Cx ; Cy M|m`.
	// M = press (or drag while pressed), m = release.
	// Cb bit 0..1 encodes the button (0=left, 1=middle, 2=right),
	// bit 5 (0x20) signals "motion" (drag), bit 6 (0x40) signals a
	// scroll-wheel event (button 4=up, 5=down when combined). We
	// only emit an EventClick on a genuine press (M, no motion bit)
	// for the moment — releases and drags are silently consumed.
	if len(params) > 0 && params[0] == '<' && (final == 'M' || final == 'm') {
		if ev, ok := decodeMouseSGR(params[1:], final); ok {
			return []toolkit.Event{ev}, true
		}
		return nil, true // recognised, no event produced
	}
	if ev, ok := decodeCSI(params, final); ok {
		return []toolkit.Event{ev}, true
	}
	return nil, false
}

// decodeCSI maps the parameter bytes and final byte of a CSI
// sequence (the portion after "ESC [") to a [toolkit.Event]. The
// second return is false for any unmapped sequence, signalling the
// caller to silently consume without emitting.
func decodeCSI(params []byte, final byte) (toolkit.Event, bool) {
	if len(params) == 0 {
		switch final {
		case 'A':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"}, true
		case 'B':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"}, true
		case 'C':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Right"}, true
		case 'D':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Left"}, true
		case 'H':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Home"}, true
		case 'F':
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "End"}, true
		}
		return toolkit.Event{}, false
	}
	if final == '~' {
		switch string(params) {
		case "3":
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Delete"}, true
		case "5":
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "PageUp"}, true
		case "6":
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "PageDown"}, true
		}
	}
	return toolkit.Event{}, false
}

// decodeMouseSGR parses the `Cb ; Cx ; Cy` payload of a SGR mouse
// report (the parameters between `CSI <` and the final `M`/`m`) and
// maps it to one of three toolkit events:
//
//   - left press (Cb & 0x63 == 0, final 'M')  → EventClick
//   - left drag  (Cb & 0x63 == 0, motion bit) → EventMouseDrag
//   - left release (final 'm', button code 0) → EventMouseUp
//
// Every other combination (scroll wheel, middle/right button) is
// recognised — the boolean returns false to signal "no event, but
// this WAS a mouse sequence so consume the bytes silently". X/Y are
// 0-indexed cell coords (SGR reports are 1-indexed on the wire).
func decodeMouseSGR(payload []byte, final byte) (toolkit.Event, bool) {
	// Split on ';' into exactly three decimal fields.
	var cb, cx, cy int
	field := 0
	for _, c := range payload {
		if c == ';' {
			field++
			if field > 2 {
				return toolkit.Event{}, false
			}
			continue
		}
		if c < '0' || c > '9' {
			return toolkit.Event{}, false
		}
		v := int(c - '0')
		switch field {
		case 0:
			cb = cb*10 + v
		case 1:
			cx = cx*10 + v
		case 2:
			cy = cy*10 + v
		}
	}
	if field != 2 {
		return toolkit.Event{}, false
	}
	// Scroll wheel: bit 0x40 set. The low bits encode direction —
	// 0 = up, 1 = down, 2/3 = horizontal (ignored). Map vertical
	// wheel presses to EventKeyDown Up/Down so widgets that already
	// handle arrow-key nav (fileList, cellTextEdit) respond to the
	// wheel without needing to know it exists. Terminals emit these
	// only on 'M' (no release phase for wheels), so an 'm' here is
	// a stray release the terminal shouldn't be sending — consume it.
	if cb&0x40 != 0 {
		if final != 'M' {
			return toolkit.Event{}, false
		}
		switch cb {
		case 64:
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Up"}, true
		case 65:
			return toolkit.Event{Kind: toolkit.EventKeyDown, Code: "Down"}, true
		}
		return toolkit.Event{}, false
	}
	// low bits != 0 = middle/right button — silently consumed.
	if cb&0x03 != 0 {
		return toolkit.Event{}, false
	}
	// SGR coords are 1-indexed; the toolkit convention is 0-indexed.
	if cx > 0 {
		cx--
	}
	if cy > 0 {
		cy--
	}
	// Caller guarantees final ∈ {'M', 'm'}. Motion bit differentiates
	// press vs drag; release is always the lowercase form.
	if final == 'm' {
		return toolkit.Event{Kind: toolkit.EventMouseUp, X: cx, Y: cy}, true
	}
	if cb&0x20 != 0 {
		return toolkit.Event{Kind: toolkit.EventMouseDrag, X: cx, Y: cy}, true
	}
	return toolkit.Event{Kind: toolkit.EventClick, X: cx, Y: cy}, true
}
