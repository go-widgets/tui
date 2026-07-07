// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package syntax

import "testing"

func TestLangMultilang(t *testing.T) {
	cases := map[string]string{
		"app.js": "javascript", "m.mjs": "javascript", "c.cjs": "javascript",
		"t.ts": "javascript", "c.tsx": "javascript", "a.jsx": "javascript",
		"s.py": "python", "lib.rb": "ruby",
		"run.sh": "shell", "x.bash": "shell", "z.zsh": "shell",
		"m.c": "c", "h.h": "c", "x.cpp": "c", "y.cc": "c", "z.hpp": "c", "w.cxx": "c",
		"lib.rs": "rust", "data.json": "json",
		"unknown.xyz": "plain",
	}
	for in, want := range cases {
		if got := lang(in); got != want {
			t.Errorf("lang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHighlightJavaScript(t *testing.T) {
	code := "// hi\n/* b\nlock */\nfunction add(a) {\n\treturn `t${a}` + \"s\" + 'c' + 42;\n}\nconst s = null\n"
	lines := Highlight(code, "app.js")
	cases := []struct {
		text string
		kind Kind
	}{
		{"function", Keyword},
		{"const", Keyword},
		{"null", Keyword},
		{"return", Keyword},
		{"add", Func},
		{"`t${a}`", String},
		{`"s"`, String},
		{"'c'", String},
		{"42", Number},
		{"// hi", Comment},
		{"/* b", Comment},   // block comment line 1
		{"lock */", Comment}, // block comment line 2
		{"{", Punct},
	}
	for _, c := range cases {
		if got := kindOfText(lines, c.text); got != c.kind {
			t.Errorf("js %q = %d, want %d", c.text, got, c.kind)
		}
	}
}

func TestHighlightPython(t *testing.T) {
	code := "# comment\ndef greet(name):\n\treturn \"hi \" + name  # inline\n"
	lines := Highlight(code, "greet.py")
	if kindOfText(lines, "def") != Keyword {
		t.Error("python def not Keyword")
	}
	if kindOfText(lines, "greet") != Func {
		t.Error("python greet( not Func")
	}
	if kindOfText(lines, "# comment") != Comment {
		t.Error("python comment not Comment")
	}
	if kindOfText(lines, `"hi "`) != String {
		t.Error("python string not String")
	}
	// Python has no block comments -> a "/*" is just punctuation, not a comment.
	if kindOfText(Highlight("x = a /* b", "z.py"), "/") != Punct {
		t.Error("python '/' should be Punct (no block comments)")
	}
}

func TestHighlightRubyShellCRustJSON(t *testing.T) {
	// Ruby (# comments, keywords).
	rb := Highlight("def f\n  puts \"x\" # c\nend\n", "a.rb")
	if kindOfText(rb, "def") != Keyword || kindOfText(rb, "end") != Keyword {
		t.Error("ruby keywords")
	}
	// Shell (# comments).
	sh := Highlight("for x in a; do echo $x; done # c\n", "s.sh")
	if kindOfText(sh, "for") != Keyword || kindOfText(sh, "# c") != Comment {
		t.Error("shell keyword/comment")
	}
	// C (types + // and /* */).
	c := Highlight("int main(void) { return 0; } // e\n", "m.c")
	if kindOfText(c, "int") != Type || kindOfText(c, "return") != Keyword || kindOfText(c, "main") != Func {
		t.Error("c type/keyword/func")
	}
	// Rust (types + keywords).
	rs := Highlight("fn main() -> Result { let x: i32 = 1; }\n", "m.rs")
	if kindOfText(rs, "fn") != Keyword || kindOfText(rs, "i32") != Type || kindOfText(rs, "Result") != Type {
		t.Error("rust keyword/type")
	}
	// JSON (true/false/null keywords, strings, numbers).
	js := Highlight("{\"a\": true, \"b\": 3.5, \"c\": null}\n", "d.json")
	if kindOfText(js, "true") != Keyword || kindOfText(js, `"a"`) != String || kindOfText(js, "3.5") != Number {
		t.Error("json keyword/string/number")
	}
}

func TestGenericEdgeCases(t *testing.T) {
	// Unterminated block comment runs to EOF (end-clamp) without panicking.
	Highlight("code /* unterminated", "x.c")
	// Unterminated string runs to EOF.
	Highlight("s = \"open", "x.js")
	// Trailing content after the last token (blank line) -> trailing gap.
	lines := Highlight("x\n\n", "x.py")
	if kindOfText(lines, "x") != Plain {
		t.Error("generic trailing-gap dropped ident")
	}
	// A non-ASCII / default-branch byte is emitted as Plain.
	Highlight("€ = 1", "x.js")
	// Escaped delimiter inside a string exercises scanString's escape branch.
	Highlight("s = \"a\\\"b\"", "x.js") // "a\"b"
	// A trailing backslash at EOF makes the scan overshoot -> the j>n clamp.
	Highlight("s = \"x\\", "x.js") // "x\  (backslash then EOF)
}
