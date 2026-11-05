package lsp

import (
	"os"
	"strings"
	"unicode/utf8"
)

// Column units.
//
// The server addresses columns in bytes of the UTF-8 document text. Two other
// units meet it at the boundaries: goccy/go-yaml token columns count runes,
// and LSP clients count UTF-16 code units unless another encoding was
// negotiated in initialize. Every conversion between the units lives here so
// that the rest of the server can treat a column as a byte index.

// positionEncoding is the unit of Position.Character on the wire.
type positionEncoding string

const (
	encodingUTF8  positionEncoding = "utf-8"
	encodingUTF16 positionEncoding = "utf-16" // the LSP default; also the zero value's meaning
)

// negotiateEncoding picks the position encoding for a session from the
// encodings the client offered. utf-8 avoids any conversion, so it wins when
// offered; otherwise the LSP default utf-16 applies, which every client
// must support.
func negotiateEncoding(offered []string) positionEncoding {
	for _, e := range offered {
		if positionEncoding(e) == encodingUTF8 {
			return encodingUTF8
		}
	}
	return encodingUTF16
}

// lineAt returns the text of the zero-based line, or "" past the end.
func lineAt(text string, line int) string {
	if line < 0 {
		return ""
	}
	for range line {
		nl := strings.IndexByte(text, '\n')
		if nl < 0 {
			return ""
		}
		text = text[nl+1:]
	}
	if before, _, ok := strings.Cut(text, "\n"); ok {
		return before
	}
	return text
}

// byteColumnFromRune converts a rune index within line to a byte index.
// goccy/go-yaml reports token columns in runes.
func byteColumnFromRune(line string, runeCol int) int {
	if runeCol <= 0 {
		return 0
	}
	i := 0
	for n := 0; n < runeCol && i < len(line); n++ {
		_, size := utf8.DecodeRuneInString(line[i:])
		i += size
	}
	return i
}

// runeColumnFromByte converts a byte index within line to a rune index.
func runeColumnFromByte(line string, byteCol int) int {
	if byteCol <= 0 {
		return 0
	}
	if byteCol > len(line) {
		byteCol = len(line)
	}
	return utf8.RuneCountInString(line[:byteCol])
}

// byteColumnFromClient converts a client column into a byte index within line.
// A column past the end of the line is clamped to the end.
func byteColumnFromClient(line string, col int, enc positionEncoding) int {
	if col <= 0 {
		return 0
	}
	if enc == encodingUTF8 {
		return min(col, len(line))
	}
	units := 0
	for i, r := range line {
		if units >= col {
			return i
		}
		units += unitsOf(r, enc)
	}
	return len(line)
}

// clientColumnFromByte converts a byte index within line into a client column.
func clientColumnFromByte(line string, byteCol int, enc positionEncoding) int {
	if byteCol <= 0 {
		return 0
	}
	if byteCol > len(line) {
		byteCol = len(line)
	}
	if enc == encodingUTF8 {
		return byteCol
	}
	units := 0
	for _, r := range line[:byteCol] {
		units += unitsOf(r, enc)
	}
	return units
}

// unitsOf returns the number of UTF-16 code units r occupies.
func unitsOf(r rune, _ positionEncoding) int {
	if r >= 0x10000 {
		return 2 // surrogate pair
	}
	return 1
}

// decodePosition converts a position received from the client into the
// internal byte-based form.
func (s *Server) decodePosition(text string, p Position) Position {
	line := lineAt(text, p.Line)
	if line == "" {
		return p // unknown text or an empty line: nothing to convert
	}
	p.Character = byteColumnFromClient(line, p.Character, s.encoding)
	return p
}

// encodePosition converts an internal byte-based position into the client's
// encoding.
func (s *Server) encodePosition(text string, p Position) Position {
	line := lineAt(text, p.Line)
	if line == "" {
		return p // unknown text or an empty line: nothing to convert
	}
	p.Character = clientColumnFromByte(line, p.Character, s.encoding)
	return p
}

// encodeRange converts an internal byte-based range into the client's encoding.
func (s *Server) encodeRange(text string, r Range) Range {
	return Range{Start: s.encodePosition(text, r.Start), End: s.encodePosition(text, r.End)}
}

// encodeLocation converts the range of a location that may point at another
// file into the client's encoding.
func (s *Server) encodeLocation(loc Location) Location {
	loc.Range = s.encodeRange(s.textOf(loc.URI), loc.Range)
	return loc
}

// encodeSymbols converts the ranges of a symbol tree into the client's encoding.
func (s *Server) encodeSymbols(text string, syms []DocumentSymbol) {
	for i := range syms {
		syms[i].Range = s.encodeRange(text, syms[i].Range)
		syms[i].SelectionRange = s.encodeRange(text, syms[i].SelectionRange)
		s.encodeSymbols(text, syms[i].Children)
	}
}

// textOf returns the text of an open document, or of the file behind the
// URI when it is not open, or "" when neither is available.
func (s *Server) textOf(uri string) string {
	if doc := s.docs.Get(uri); doc != nil {
		return doc.Text
	}
	data, err := os.ReadFile(uriToPath(uri))
	if err != nil {
		return ""
	}
	return string(data)
}

// tokenRange builds the internal range of a token whose text spans length
// bytes, converting the rune column reported by goccy/go-yaml.
func tokenRange(text string, line, runeCol, length int) Range {
	l := line - 1
	start := byteColumnFromRune(lineAt(text, l), runeCol-1)
	return Range{
		Start: Position{Line: l, Character: start},
		End:   Position{Line: l, Character: start + length},
	}
}
