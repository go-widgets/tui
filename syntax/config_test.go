// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package syntax

import "testing"

func TestLangConfigLangs(t *testing.T) {
	cases := map[string]string{
		"c.yaml": "yaml", "c.yml": "yaml",
		"c.toml": "toml",
		"main.hcl": "hcl", "x.tf": "hcl", "v.tfvars": "hcl",
		"doc.tex": "latex", "d.latex": "latex", "s.sty": "latex", "c.cls": "latex",
	}
	for in, want := range cases {
		if got := lang(in); got != want {
			t.Errorf("lang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHighlightYAML(t *testing.T) {
	code := "# top comment\n" +
		"name: value\n" +
		"count: 42\n" +
		"enabled: true\n" +
		"items:\n" +
		"  - first\n" +
		"  - \"quoted\"  # inline\n" +
		"ref: &anchor x\n" +
		"flow: [1, 2]\n" +
		"\n"
	lines := Highlight(code, "config.yaml")
	if kindOfText(lines, "# top comment") != Comment {
		t.Error("yaml comment line")
	}
	if kindOfText(lines, "name") != Keyword || kindOfText(lines, "count") != Keyword {
		t.Error("yaml keys not Keyword")
	}
	if kindOfText(lines, "42") != Number {
		t.Error("yaml number")
	}
	if kindOfText(lines, "true") != Keyword {
		t.Error("yaml bool literal")
	}
	if kindOfText(lines, "-") != Punct {
		t.Error("yaml list marker")
	}
	if kindOfText(lines, `"quoted"`) != String {
		t.Error("yaml quoted string")
	}
	if kindOfText(lines, "# inline") != Comment {
		t.Error("yaml inline comment")
	}
	if kindOfText(lines, "[") != Punct {
		t.Error("yaml flow punct")
	}
	if kindOfText(lines, "value") != Plain {
		t.Error("yaml plain value")
	}
}

func TestYamlColon(t *testing.T) {
	if yamlColon("key: v") != 3 {
		t.Error("colon+space")
	}
	if yamlColon("key:") != 3 {
		t.Error("colon at EOL")
	}
	if yamlColon("\"quoted\": v") != -1 {
		t.Error("quoted key -> no split")
	}
	if yamlColon("just a value") != -1 {
		t.Error("space before any colon -> -1")
	}
	if yamlColon("# comment") != -1 {
		t.Error("hash before colon -> -1")
	}
	if yamlColon("plainword") != -1 {
		t.Error("no colon -> -1")
	}
}

func TestHighlightTOML(t *testing.T) {
	code := "# a comment\n" +
		"[section]\n" +
		"[[array.of.tables]]\n" +
		"name = \"wasmbox\"\n" +
		"count=42\n" +
		"enabled = true\n" +
		"  indented = 1\n" + // leading indent exercises the whitespace loop
		"nested = { inline = 1 }\n" +
		"broken = [\n" +
		"\n"
	lines := Highlight(code, "pkg.toml")
	if kindOfText(lines, "# a comment") != Comment {
		t.Error("toml comment")
	}
	if kindOfText(lines, "[section]") != Keyword {
		t.Error("toml section header")
	}
	if kindOfText(lines, "[[array.of.tables]]") != Keyword {
		t.Error("toml array-of-tables header")
	}
	if kindOfText(lines, "name") != Keyword || kindOfText(lines, "count") != Keyword {
		t.Error("toml keys")
	}
	if kindOfText(lines, `"wasmbox"`) != String {
		t.Error("toml string")
	}
	if kindOfText(lines, "42") != Number || kindOfText(lines, "true") != Keyword {
		t.Error("toml number/bool")
	}
	if kindOfText(lines, "=") != Punct {
		t.Error("toml equals")
	}
	// A "[" line with no closing "]" falls back to end-of-line as the header.
	if got := Highlight("[unclosed\n", "x.toml"); kindOfText(got, "[unclosed") != Keyword {
		t.Error("toml unclosed section header")
	}
}

func TestHighlightHCL(t *testing.T) {
	code := "# hash comment\n" +
		"// slash comment\n" +
		"/* block */\n" +
		"resource \"aws_x\" \"y\" {\n" +
		"  count = length(var.zs)\n" +
		"  enabled = true\n" +
		"}\n"
	lines := Highlight(code, "main.hcl")
	if kindOfText(lines, "# hash comment") != Comment {
		t.Error("hcl # comment")
	}
	if kindOfText(lines, "// slash comment") != Comment {
		t.Error("hcl // comment")
	}
	if kindOfText(lines, "/* block */") != Comment {
		t.Error("hcl block comment")
	}
	if kindOfText(lines, `"aws_x"`) != String {
		t.Error("hcl string")
	}
	if kindOfText(lines, "true") != Keyword {
		t.Error("hcl bool")
	}
	if kindOfText(lines, "length") != Func {
		t.Error("hcl function call")
	}
}

func TestHighlightLatex(t *testing.T) {
	code := "% a comment\n" +
		"\\documentclass{article}\n" +
		"\\begin{document}\n" +
		"Hello $x^2$ \\& more~text \\\\\n" +
		"\\end{document}\n"
	lines := Highlight(code, "doc.tex")
	if kindOfText(lines, "% a comment") != Comment {
		t.Error("latex comment")
	}
	if kindOfText(lines, "\\documentclass") != Keyword {
		t.Error("latex command")
	}
	if kindOfText(lines, "{") != Punct || kindOfText(lines, "$") != Punct {
		t.Error("latex delimiters")
	}
	if kindOfText(lines, "\\&") != Keyword {
		t.Error("latex escaped single char")
	}
	if kindOfText(lines, "article") != Plain {
		t.Error("latex plain run")
	}
}
