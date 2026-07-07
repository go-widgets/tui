// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

// Package syntax is a tiny, dependency-free syntax highlighter for the TUI
// previews. It tokenises source into per-line coloured Spans; the caller maps
// each Span's Kind to a terminal colour.
//
// Rather than hand-roll a Go lexer (or pull in a heavyweight highlighter such
// as github.com/alecthomas/chroma, whose transitive deps would break this
// repo's dependency-free axiom), the Go path reuses the standard library's
// go/scanner -- the canonical Go tokeniser, zero extra dependencies. Markdown
// and the plain fallback are trivial line passes.
package syntax

import (
	"go/scanner"
	"go/token"
	"path"
	"strings"
)

// Kind categorises a run of source text.
type Kind int

const (
	// Plain is uncoloured text (identifiers, whitespace, operators left in the
	// foreground colour).
	Plain Kind = iota
	// Keyword is a language keyword (Go's `func`; a Markdown heading line).
	Keyword
	// String is a string / rune / raw-string literal.
	String
	// Comment is a line or block comment.
	Comment
	// Number is a numeric literal.
	Number
	// Type is a predeclared type name (int, string, …).
	Type
	// Func is an identifier used as a call (immediately followed by "(").
	Func
	// Punct is a bracket / operator / separator.
	Punct
)

// Span is a run of text sharing a single Kind.
type Span struct {
	Text string
	Kind Kind
}

// Highlight tokenises code and returns one []Span per line (split on "\n").
// The language is chosen from filename's extension; unknown extensions fall
// back to a single Plain span per line.
func Highlight(code, filename string) [][]Span {
	switch id := lang(filename); id {
	case "go":
		return highlightGo(code)
	case "markdown":
		return highlightMarkdown(code)
	default:
		if spec, ok := specs[id]; ok {
			return splitLines(tokenizeGeneric(code, spec))
		}
		return highlightPlain(code)
	}
}

// lang maps a filename to a supported language id. Go + markdown have dedicated
// paths; the rest resolve to a langSpec in specs (see generic.go); anything
// unknown is "plain".
func lang(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".go":
		return "go"
	case ".md", ".markdown":
		return "markdown"
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".c", ".h", ".cc", ".cpp", ".hpp", ".cxx":
		return "c"
	case ".rs":
		return "rust"
	case ".json":
		return "json"
	default:
		return "plain"
	}
}

// goTypes is the set of Go predeclared type names (go/scanner reports these as
// plain IDENTs, so we upgrade them ourselves).
var goTypes = map[string]bool{
	"bool": true, "byte": true, "rune": true, "string": true, "error": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"uintptr": true, "float32": true, "float64": true, "complex64": true,
	"complex128": true, "any": true,
}

// tokRange is one scanned token's byte range + resolved Kind.
type tokRange struct {
	start, end int
	kind       Kind
}

// highlightGo lexes Go source with go/scanner, fills the whitespace gaps
// between tokens with Plain, and splits the result into per-line spans. The
// scanner's error handler is a no-op so malformed source still highlights.
func highlightGo(code string) [][]Span {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(code))
	var sc scanner.Scanner
	sc.Init(file, []byte(code), func(token.Position, string) {}, scanner.ScanComments)

	var ranges []tokRange
	prevIdent := -1 // index of the immediately preceding plain IDENT, else -1
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		text := lit
		if text == "" {
			text = tok.String()
		}
		start := file.Offset(pos)
		end := start + len(text)
		if end > len(code) {
			end = len(code)
		}
		ranges = append(ranges, tokRange{start: start, end: end, kind: kindOf(tok, lit)})
		idx := len(ranges) - 1
		// An identifier immediately followed by "(" is a call -> Func.
		if tok == token.LPAREN && prevIdent == idx-1 {
			ranges[prevIdent].kind = Func
		}
		if tok == token.IDENT && ranges[idx].kind == Plain {
			prevIdent = idx
		} else {
			prevIdent = -1
		}
	}

	// go/scanner yields tokens in order without overlap, so the gap before each
	// token (whitespace the scanner skipped) is Plain and prev only moves
	// forward.
	var flat []Span
	prev := 0
	for _, r := range ranges {
		if r.start > prev {
			flat = append(flat, Span{Text: code[prev:r.start], Kind: Plain})
		}
		if r.end > r.start {
			flat = append(flat, Span{Text: code[r.start:r.end], Kind: r.kind})
			prev = r.end
		}
	}
	if prev < len(code) {
		flat = append(flat, Span{Text: code[prev:], Kind: Plain})
	}
	return splitLines(flat)
}

// kindOf maps a go/token to a Kind. IDENTs are Plain unless they name a
// predeclared type (the call-site Func upgrade happens in highlightGo).
func kindOf(tok token.Token, lit string) Kind {
	switch {
	case tok == token.COMMENT:
		return Comment
	case tok == token.STRING || tok == token.CHAR:
		return String
	case tok == token.INT || tok == token.FLOAT || tok == token.IMAG:
		return Number
	case tok.IsKeyword():
		return Keyword
	case tok == token.IDENT:
		if goTypes[lit] {
			return Type
		}
		return Plain
	default:
		return Punct
	}
}

// splitLines turns a flat span list (Text may contain "\n" from block comments
// / raw strings / whitespace) into one []Span per line.
func splitLines(toks []Span) [][]Span {
	lines := [][]Span{{}}
	for _, t := range toks {
		parts := strings.Split(t.Text, "\n")
		for pi, part := range parts {
			if pi > 0 {
				lines = append(lines, []Span{})
			}
			if part != "" {
				last := len(lines) - 1
				lines[last] = append(lines[last], Span{Text: part, Kind: t.Kind})
			}
		}
	}
	return lines
}

// highlightMarkdown colours ATX heading lines (leading "#") as Keyword.
func highlightMarkdown(code string) [][]Span {
	var out [][]Span
	for _, line := range strings.Split(code, "\n") {
		if line == "" {
			out = append(out, []Span{})
			continue
		}
		kind := Plain
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			kind = Keyword
		}
		out = append(out, []Span{{Text: line, Kind: kind}})
	}
	return out
}

// highlightPlain returns one Plain span per non-empty line.
func highlightPlain(code string) [][]Span {
	var out [][]Span
	for _, line := range strings.Split(code, "\n") {
		if line == "" {
			out = append(out, []Span{})
			continue
		}
		out = append(out, []Span{{Text: line, Kind: Plain}})
	}
	return out
}
