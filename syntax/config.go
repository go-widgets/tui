// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package syntax

import "strings"

// This file adds dedicated, line- / prefix-oriented lexers for config +
// markup languages that don't fit the C-family generic lexer: YAML + TOML
// (mapping keys, section headers, "# " comments) and LaTeX (\commands, %
// comments, math/group delimiters). HCL stays on the generic lexer (see
// generic.go); Go on go/scanner.

var yamlLits = set("true", "false", "null", "yes", "no", "on", "off",
	"True", "False", "Null", "YES", "NO", "ON", "OFF")

var tomlLits = set("true", "false", "inf", "nan")

// highlightYAML colours one line at a time: indentation + list markers, a
// leading "# " comment, a "key:" as Keyword, and the value.
func highlightYAML(code string) [][]Span {
	var out [][]Span
	for _, line := range strings.Split(code, "\n") {
		out = append(out, yamlLine(line))
	}
	return out
}

func yamlLine(line string) []Span {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	var sp []Span
	push := func(s string, k Kind) {
		if s != "" {
			sp = append(sp, Span{Text: s, Kind: k})
		}
	}
	i, n := 0, len(line)
	// Indentation.
	j := i
	for i < n && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	push(line[j:i], Plain)
	// A comment line.
	if i < n && line[i] == '#' {
		push(line[i:], Comment)
		return sp
	}
	// Sequence markers "- " (possibly several).
	for i < n && line[i] == '-' && (i+1 == n || line[i+1] == ' ') {
		push("-", Punct)
		i++
		j = i
		for i < n && line[i] == ' ' {
			i++
		}
		push(line[j:i], Plain)
	}
	// "key:" -> Keyword + ":".
	if ci := yamlColon(line[i:]); ci >= 0 {
		push(line[i:i+ci], Keyword)
		push(":", Punct)
		i += ci + 1
	}
	configValue(line[i:], push, yamlLits)
	return sp
}

// yamlColon returns the index in s of a mapping colon (a ':' at end-of-string
// or followed by a space), or -1. Quoted keys aren't split.
func yamlColon(s string) int {
	if len(s) == 0 || s[0] == '"' || s[0] == '\'' {
		return -1
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ') {
			return i
		}
		if s[i] == ' ' || s[i] == '#' {
			return -1 // ran into the value / a comment before any colon
		}
	}
	return -1
}

// highlightTOML colours "# " comments, "[section]" / "[[array]]" headers, and
// "key = value" pairs.
func highlightTOML(code string) [][]Span {
	var out [][]Span
	for _, line := range strings.Split(code, "\n") {
		out = append(out, tomlLine(line))
	}
	return out
}

func tomlLine(line string) []Span {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	var sp []Span
	push := func(s string, k Kind) {
		if s != "" {
			sp = append(sp, Span{Text: s, Kind: k})
		}
	}
	i, n := 0, len(line)
	j := i
	for i < n && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	push(line[j:i], Plain)
	switch {
	case i < n && line[i] == '#':
		push(line[i:], Comment)
		return sp
	case i < n && line[i] == '[':
		// [table] / [[array-of-tables]] header: to the last ']'.
		end := strings.LastIndexByte(line, ']')
		if end < i {
			end = n - 1
		}
		push(line[i:end+1], Keyword)
		configValue(line[end+1:], push, tomlLits)
		return sp
	}
	// key = value.
	if eq := strings.IndexByte(line[i:], '='); eq >= 0 {
		push(strings.TrimRight(line[i:i+eq], " "), Keyword)
		push(line[i+len(strings.TrimRight(line[i:i+eq], " ")):i+eq], Plain) // spaces before '='
		push("=", Punct)
		i += eq + 1
	}
	configValue(line[i:], push, tomlLits)
	return sp
}

// configValue tokenises the value portion of a YAML/TOML line: whitespace, an
// inline "#" comment, quoted strings, numbers, flow punctuation, and bare words
// (Keyword when in lits, else Plain).
func configValue(s string, push func(string, Kind), lits map[string]bool) {
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			j := i
			for i < n && (s[i] == ' ' || s[i] == '\t') {
				i++
			}
			push(s[j:i], Plain)
		case c == '#':
			push(s[i:], Comment)
			return
		case c == '"' || c == '\'':
			i = scanString(s, i, c, push)
		case isDigit(c):
			j := i
			for i < n && isNumChar(s[i]) {
				i++
			}
			push(s[j:i], Number)
		case c == '[' || c == ']' || c == '{' || c == '}' || c == ',' || c == ':' || c == '=':
			push(string(c), Punct)
			i++
		case isIdentStart(c):
			j := i
			for i < n && isIdentChar(s[i]) {
				i++
			}
			w := s[j:i]
			if lits[w] {
				push(w, Keyword)
			} else {
				push(w, Plain)
			}
		default:
			push(string(c), Plain)
			i++
		}
	}
}

// highlightLatex colours % comments, \commands, and group/math delimiters.
func highlightLatex(code string) [][]Span {
	var out []Span
	emit := func(s string, k Kind) {
		if s != "" {
			out = append(out, Span{Text: s, Kind: k})
		}
	}
	i, n := 0, len(code)
	for i < n {
		c := code[i]
		switch {
		case c == '%':
			j := i
			for j < n && code[j] != '\n' {
				j++
			}
			emit(code[i:j], Comment)
			i = j
		case c == '\\':
			j := i + 1
			if j < n && isLetter(code[j]) {
				for j < n && isLetter(code[j]) {
					j++
				}
			} else if j < n {
				j++ // an escaped single char: \{ \$ \% ...
			}
			emit(code[i:j], Keyword)
			i = j
		case latexSpecial(c):
			emit(string(c), Punct)
			i++
		default:
			j := i
			for j < n && code[j] != '%' && code[j] != '\\' && !latexSpecial(code[j]) {
				j++
			}
			emit(code[i:j], Plain)
			i = j
		}
	}
	return splitLines(out)
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func latexSpecial(c byte) bool {
	switch c {
	case '{', '}', '[', ']', '$', '&', '#', '_', '^', '~':
		return true
	}
	return false
}
