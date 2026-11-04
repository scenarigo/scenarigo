package lsp

import (
	"os"
	"sort"
	"sync"
	"unicode/utf8"
	"unsafe"
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
	return linesOf(text).text(text, line)
}

// lastLines is the line index of the last text that lineAt looked a line up
// in. The server converts the positions of a document one at a time, many
// of them for one publication of its diagnostics, and going through the text
// from its start for each made large files quadratic.
var lastLines struct {
	sync.Mutex
	text  string
	lines lineIndex
}

// linesOf returns the line index of text, reusing the last one built for
// the same string. Strings are compared by their data and length, which is
// immediate on every platform, where comparing their bytes is not; holding
// the text keeps its memory from being reused for another string.
func linesOf(text string) lineIndex {
	lastLines.Lock()
	defer lastLines.Unlock()
	if lastLines.lines.starts == nil || !sameString(lastLines.text, text) {
		lastLines.text, lastLines.lines = text, newLineIndex(text)
	}
	return lastLines.lines
}

// forgetLines drops the line index of a text, so that a closed document is
// not held in memory.
func forgetLines(text string) {
	lastLines.Lock()
	defer lastLines.Unlock()
	if sameString(lastLines.text, text) {
		lastLines.text, lastLines.lines = "", lineIndex{}
	}
}

func sameString(a, b string) bool {
	return len(a) == len(b) && unsafe.StringData(a) == unsafe.StringData(b)
}

// lineIndex holds the byte offset at which each line of a text starts, and
// which lines are ASCII only: on those, byte, rune and UTF-16 columns are
// the same, and converting a column does not walk the line. On the other
// lines, the byte and UTF-16 columns of every markEvery-th rune are kept, so
// that a conversion walks markEvery runes at most: walking a long line from
// its start for each of its values made it quadratic.
type lineIndex struct {
	starts []int
	ascii  []bool
	marks  map[int][]lineMark // by line, for lines that are not ASCII only
}

// lineMark holds the byte and UTF-16 columns of the k-th multiple of
// markEvery runes of a line, at index k-1 of its marks.
type lineMark struct {
	bytes, units int
}

const markEvery = 64

func newLineIndex(text string) lineIndex {
	idx := lineIndex{starts: []int{0}}
	ascii := true
	for i := range len(text) {
		switch c := text[i]; {
		case c == '\n':
			idx.starts = append(idx.starts, i+1)
			idx.ascii = append(idx.ascii, ascii)
			ascii = true
		case c >= utf8.RuneSelf:
			ascii = false
		}
	}
	idx.ascii = append(idx.ascii, ascii)
	for line, ascii := range idx.ascii {
		if ascii {
			continue
		}
		// Runes are counted as byteColumnFromRune and clientColumnFromByte
		// count them: an invalid byte is a rune of one byte.
		var marks []lineMark
		runes, units := 0, 0
		for i, r := range idx.text(text, line) {
			if runes > 0 && runes%markEvery == 0 {
				marks = append(marks, lineMark{bytes: i, units: units})
			}
			runes++
			units += unitsOf(r, encodingUTF16)
		}
		if marks != nil {
			if idx.marks == nil {
				idx.marks = map[int][]lineMark{}
			}
			idx.marks[line] = marks
		}
	}
	return idx
}

// lineStart returns the byte offset of the start of a 0-based line, or -1.
func (idx lineIndex) lineStart(line int) int {
	if line < 0 || line >= len(idx.starts) {
		return -1
	}
	return idx.starts[line]
}

// isASCII reports whether a 0-based line holds ASCII characters only.
func (idx lineIndex) isASCII(line int) bool {
	return line >= 0 && line < len(idx.ascii) && idx.ascii[line]
}

// byteColumn converts a rune column of a 0-based line of text into a byte
// column, as byteColumnFromRune does.
func (idx lineIndex) byteColumn(text string, line, runeCol int) int {
	l := idx.text(text, line)
	if idx.isASCII(line) {
		return min(max(runeCol, 0), len(l))
	}
	start, runes := 0, 0
	if marks := idx.marks[line]; runeCol >= markEvery && len(marks) > 0 {
		k := min(runeCol/markEvery, len(marks))
		start, runes = marks[k-1].bytes, k*markEvery
	}
	return start + byteColumnFromRune(l[start:], runeCol-runes)
}

// clientColumn converts a byte column of a 0-based line of text into a
// column of the client's encoding, as clientColumnFromByte does.
func (idx lineIndex) clientColumn(text string, line, byteCol int, enc positionEncoding) int {
	l := idx.text(text, line)
	if idx.isASCII(line) || enc == encodingUTF8 {
		return min(max(byteCol, 0), len(l))
	}
	if byteCol <= 0 {
		return 0
	}
	byteCol = min(byteCol, len(l))
	marks := idx.marks[line]
	k := sort.Search(len(marks), func(i int) bool { return marks[i].bytes > byteCol })
	start, units := 0, 0
	if k > 0 {
		start, units = marks[k-1].bytes, marks[k-1].units
	}
	return units + clientColumnFromByte(l[start:], byteCol-start, enc)
}

// text returns a 0-based line of the text the index was built from, without
// its line break, or "" past the end. Unlike lineAt, it does not go through
// the text from its start, so that looking up every value of a large file
// stays linear.
func (idx lineIndex) text(text string, line int) string {
	start := idx.lineStart(line)
	if start < 0 || start > len(text) {
		return ""
	}
	end := len(text)
	if line+1 < len(idx.starts) {
		end = min(idx.starts[line+1]-1, len(text))
	}
	return text[start:max(end, start)]
}

// offset converts a goccy/go-yaml position (1-based line, 1-based rune
// column) into a byte offset of text, or -1.
func (idx lineIndex) offset(text string, line, runeCol int) int {
	start := idx.lineStart(line - 1)
	if start < 0 {
		return -1
	}
	return start + idx.byteColumn(text, line-1, runeCol-1)
}

// position converts a byte offset into an internal position.
func (idx lineIndex) position(offset int) Position {
	line := sort.Search(len(idx.starts), func(i int) bool { return idx.starts[i] > offset }) - 1
	line = max(line, 0)
	return Position{Line: line, Character: offset - idx.starts[line]}
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
	lines := linesOf(text)
	if lines.text(text, p.Line) == "" {
		return p // unknown text or an empty line: nothing to convert
	}
	p.Character = lines.clientColumn(text, p.Line, p.Character, s.encoding)
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
	start := linesOf(text).byteColumn(text, l, runeCol-1)
	return Range{
		Start: Position{Line: l, Character: start},
		End:   Position{Line: l, Character: start + length},
	}
}
