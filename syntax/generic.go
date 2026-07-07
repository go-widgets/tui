// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package syntax

// langSpec is a data-driven description of a simple language's lexical rules,
// consumed by tokenizeGeneric. It is deliberately approximate -- enough to
// colour keywords, comments, strings + numbers for a preview, not a full
// grammar. Go keeps its own go/scanner path; these cover the common C-family +
// scripting languages.
type langSpec struct {
	keywords     map[string]bool
	types        map[string]bool
	lineComments []string // e.g. {"//"} or {"#", "//"}; nil = none
	blockOpen    string   // e.g. "/*"; "" = no block comments
	blockClose   string   // e.g. "*/"
	backtick     bool     // supports `...` (template) strings
}

// matchesLineComment reports whether any of the spec's line-comment markers
// starts at src[i].
func matchesLineComment(spec *langSpec, src string, i int) bool {
	for _, lc := range spec.lineComments {
		if hasPrefixAt(src, i, lc) {
			return true
		}
	}
	return false
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// cFamily is the //-line + /* */-block comment convention shared by C, Rust +
// JavaScript below; declared once and copied into each spec.
var (
	jsSpec = &langSpec{
		keywords: set("break", "case", "catch", "class", "const", "continue", "debugger",
			"default", "delete", "do", "else", "export", "extends", "finally", "for",
			"function", "if", "import", "in", "instanceof", "new", "return", "super",
			"switch", "this", "throw", "try", "typeof", "var", "void", "while", "with",
			"yield", "let", "static", "async", "await", "of", "true", "false", "null",
			"undefined"),
		types:       set("string", "number", "boolean", "object", "symbol", "bigint", "any", "unknown", "never", "void"),
		lineComments: []string{"//"}, blockOpen: "/*", blockClose: "*/", backtick: true,
	}
	pySpec = &langSpec{
		keywords: set("False", "None", "True", "and", "as", "assert", "async", "await",
			"break", "class", "continue", "def", "del", "elif", "else", "except",
			"finally", "for", "from", "global", "if", "import", "in", "is", "lambda",
			"nonlocal", "not", "or", "pass", "raise", "return", "try", "while", "with",
			"yield", "match", "case"),
		types:       set("int", "float", "str", "bool", "bytes", "list", "dict", "set", "tuple", "complex"),
		lineComments: []string{"#"},
	}
	rubySpec = &langSpec{
		keywords: set("begin", "break", "case", "class", "def", "do", "else", "elsif",
			"end", "ensure", "false", "for", "if", "in", "module", "next", "nil", "not",
			"or", "and", "redo", "rescue", "retry", "return", "self", "super", "then",
			"true", "unless", "until", "when", "while", "yield", "require", "attr_accessor",
			"attr_reader", "attr_writer", "puts", "lambda", "proc"),
		lineComments: []string{"#"},
	}
	shellSpec = &langSpec{
		keywords: set("if", "then", "else", "elif", "fi", "case", "esac", "for", "while",
			"until", "do", "done", "in", "function", "select", "time", "return", "exit",
			"export", "local", "readonly", "declare", "set", "unset", "echo", "cd", "source"),
		lineComments: []string{"#"},
	}
	cSpec = &langSpec{
		keywords: set("auto", "break", "case", "const", "continue", "default", "do",
			"else", "enum", "extern", "for", "goto", "if", "inline", "register",
			"restrict", "return", "sizeof", "static", "struct", "switch", "typedef",
			"union", "volatile", "while", "class", "namespace", "template", "public",
			"private", "protected", "virtual", "new", "delete", "using", "nullptr", "true", "false"),
		types: set("bool", "char", "double", "float", "int", "long", "short", "signed",
			"unsigned", "void", "size_t", "int8_t", "int16_t", "int32_t", "int64_t",
			"uint8_t", "uint16_t", "uint32_t", "uint64_t"),
		lineComments: []string{"//"}, blockOpen: "/*", blockClose: "*/",
	}
	rustSpec = &langSpec{
		keywords: set("as", "async", "await", "break", "const", "continue", "crate",
			"dyn", "else", "enum", "extern", "false", "fn", "for", "if", "impl", "in",
			"let", "loop", "match", "mod", "move", "mut", "pub", "ref", "return", "self",
			"Self", "static", "struct", "super", "trait", "true", "type", "unsafe", "use",
			"where", "while"),
		types: set("bool", "char", "str", "String", "i8", "i16", "i32", "i64", "i128",
			"isize", "u8", "u16", "u32", "u64", "u128", "usize", "f32", "f64", "Vec",
			"Option", "Result", "Box"),
		lineComments: []string{"//"}, blockOpen: "/*", blockClose: "*/",
	}
	jsonSpec = &langSpec{
		keywords: set("true", "false", "null"),
	}
	// HCL (Terraform / pkgx / libhcl): # and // line comments, /* */ blocks,
	// "..." strings, numbers, a few literal keywords + directive words. Blocks
	// (resource "x" {}) read as identifiers -> a call before "(" or a bare
	// ident; that is fine for a preview.
	hclSpec = &langSpec{
		keywords:     set("true", "false", "null", "for", "in", "if", "endif", "else", "endfor"),
		lineComments: []string{"#", "//"},
		blockOpen:    "/*", blockClose: "*/",
	}
)

// specs maps a lang id (from lang()) to its langSpec. Go + markdown have
// dedicated paths and are not here.
var specs = map[string]*langSpec{
	"javascript": jsSpec,
	"python":     pySpec,
	"ruby":       rubySpec,
	"shell":      shellSpec,
	"c":          cSpec,
	"rust":       rustSpec,
	"json":       jsonSpec,
	"hcl":        hclSpec,
}

// tokenizeGeneric lexes src per spec into a flat span list (Text may contain
// "\n" for block comments / whitespace); splitLines breaks it into lines.
func tokenizeGeneric(src string, spec *langSpec) []Span {
	var out []Span
	emit := func(s string, k Kind) {
		if s != "" {
			out = append(out, Span{Text: s, Kind: k})
		}
	}
	i, n := 0, len(src)
	for i < n {
		c := src[i]
		switch {
		case matchesLineComment(spec, src, i):
			j := i
			for j < n && src[j] != '\n' {
				j++
			}
			emit(src[i:j], Comment)
			i = j
		case spec.blockOpen != "" && hasPrefixAt(src, i, spec.blockOpen):
			j := i + len(spec.blockOpen)
			for j < n && !hasPrefixAt(src, j, spec.blockClose) {
				j++
			}
			end := j + len(spec.blockClose)
			if end > n {
				end = n
			}
			emit(src[i:end], Comment)
			i = end
		case c == '"' || c == '\'':
			i = scanString(src, i, c, emit)
		case c == '`' && spec.backtick:
			i = scanString(src, i, '`', emit)
		case isDigit(c):
			j := i
			for j < n && isNumChar(src[j]) {
				j++
			}
			emit(src[i:j], Number)
			i = j
		case isIdentStart(c):
			j := i
			for j < n && isIdentChar(src[j]) {
				j++
			}
			word := src[i:j]
			kind := Plain
			switch {
			case spec.keywords[word]:
				kind = Keyword
			case spec.types[word]:
				kind = Type
			case j < n && src[j] == '(':
				kind = Func
			}
			emit(word, kind)
			i = j
		case isSpace(c):
			j := i
			for j < n && isSpace(src[j]) {
				j++
			}
			emit(src[i:j], Plain)
			i = j
		case isPunct(c):
			emit(string(c), Punct)
			i++
		default:
			emit(string(c), Plain)
			i++
		}
	}
	return out
}

// scanString consumes a quoted literal opened at i with delimiter q (honouring
// backslash escapes), emits it as a String span, and returns the new index.
func scanString(src string, i int, q byte, emit func(string, Kind)) int {
	n := len(src)
	j := i + 1
	for j < n && src[j] != q {
		if src[j] == '\\' {
			j++
		}
		j++
	}
	if j < n {
		j++
	}
	if j > n {
		j = n
	}
	emit(src[i:j], String)
	return j
}

func hasPrefixAt(s string, i int, prefix string) bool {
	return i+len(prefix) <= len(s) && s[i:i+len(prefix)] == prefix
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isNumChar(c byte) bool {
	return isDigit(c) || c == '.' || c == 'x' || c == 'X' || c == '_' ||
		(c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentChar(c byte) bool { return isIdentStart(c) || isDigit(c) }
func isSpace(c byte) bool     { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isPunct(c byte) bool {
	switch c {
	case '{', '}', '(', ')', '[', ']', '.', ',', ';', ':', '=', '+', '-', '*',
		'/', '%', '<', '>', '!', '&', '|', '^', '~', '?', '@', '$', '#':
		return true
	}
	return false
}
