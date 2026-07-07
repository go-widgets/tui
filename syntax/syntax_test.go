// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package syntax

import "testing"

// kindOfText returns the Kind of the first span whose Text == want across all
// lines, or -1 if not found.
func kindOfText(lines [][]Span, want string) Kind {
	for _, ln := range lines {
		for _, s := range ln {
			if s.Text == want {
				return s.Kind
			}
		}
	}
	return -1
}

func TestHighlightGo(t *testing.T) {
	code := "package main\n" +
		"// line comment\n" +
		"/* block\ncomment */\n" +
		"func add(a int) string {\n" +
		"\treturn \"x\" + `raw` // c\n" +
		"}\n" +
		"const n = 42\n" +
		"const f = 3.14\n" +
		"const c = 3i\n" +
		"const r = 'a'\n" +
		"const g = (1 + 2)\n"
	lines := Highlight(code, "main.go")

	cases := []struct {
		text string
		kind Kind
	}{
		{"package", Keyword},
		{"func", Keyword},
		{"return", Keyword},
		{"const", Keyword},
		{"int", Type},
		{"string", Type},
		{"add", Func}, // identifier followed by "("
		{`"x"`, String},
		{"`raw`", String},
		{"'a'", String},
		{"42", Number},
		{"3.14", Number},
		{"3i", Number},
		{"{", Punct},
		{"+", Punct},
		{"// line comment", Comment},
		{"// c", Comment},
		{"/* block", Comment},  // block comment, first line
		{"comment */", Comment}, // block comment, second line
		{"n", Plain},            // a plain identifier
	}
	for _, c := range cases {
		if got := kindOfText(lines, c.text); got != c.kind {
			t.Errorf("kind of %q = %d, want %d", c.text, got, c.kind)
		}
	}
}

func TestHighlightGoNoTrailingNewline(t *testing.T) {
	// No trailing newline makes go/scanner insert a final auto-semicolon at
	// EOF, whose "\n" text runs one byte past the source -- exercising the
	// end-of-buffer clamp in highlightGo.
	lines := Highlight("package x", "f.go")
	if kindOfText(lines, "package") != Keyword {
		t.Fatal("package keyword missing / clamp mishandled")
	}
}

func TestHighlightGoEdgeCases(t *testing.T) {
	// A blank final line leaves content after the last token -> the trailing
	// gap is emitted as a Plain span (and must not panic).
	lines := Highlight("x\n\n", "a.go")
	if kindOfText(lines, "x") != Plain {
		t.Error("trailing-gap case dropped the identifier")
	}
	// Malformed source drives go/scanner's error handler (a no-op) without the
	// highlighter failing.
	Highlight("y := '", "a.go")    // unterminated rune literal
	Highlight("\x00 var z", "a.go") // illegal NUL byte
}

func TestHighlightMarkdown(t *testing.T) {
	lines := Highlight("# Title\n\nbody text\n", "README.md")
	if got := kindOfText(lines, "# Title"); got != Keyword {
		t.Errorf("markdown heading kind = %d, want Keyword", got)
	}
	if got := kindOfText(lines, "body text"); got != Plain {
		t.Errorf("markdown body kind = %d, want Plain", got)
	}
	// "# Title", "", "body text", "" -> 4 lines; the blank ones are empty slices.
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(lines))
	}
	if len(lines[1]) != 0 {
		t.Errorf("blank markdown line should be empty, got %v", lines[1])
	}
}

func TestHighlightPlain(t *testing.T) {
	lines := Highlight("BSD-3-Clause\n\nx\n", "LICENSE")
	if got := kindOfText(lines, "BSD-3-Clause"); got != Plain {
		t.Errorf("license kind = %d, want Plain", got)
	}
	if len(lines[1]) != 0 {
		t.Error("blank plain line should be empty")
	}
}

func TestLang(t *testing.T) {
	cases := map[string]string{
		"main.go":    "go",
		"README.MD":  "markdown",
		"a.markdown": "markdown",
		"LICENSE":    "plain",
		"notes.txt":  "plain",
		"":           "plain",
	}
	for in, want := range cases {
		if got := lang(in); got != want {
			t.Errorf("lang(%q) = %q, want %q", in, got, want)
		}
	}
}
