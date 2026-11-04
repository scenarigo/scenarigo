package lsp

import (
	"strings"
	"testing"
)

func TestColumnConversions(t *testing.T) {
	// "日本語" is 3 runes, 9 bytes, 3 UTF-16 units; "😀" is 1 rune, 4 bytes, 2 UTF-16 units.
	line := "a: 日本語 😀 b"
	tests := []struct {
		name     string
		byteCol  int
		runeCol  int
		utf16Col int
	}{
		{name: "start", byteCol: 0, runeCol: 0, utf16Col: 0},
		{name: "before kanji", byteCol: 3, runeCol: 3, utf16Col: 3},
		{name: "after kanji", byteCol: 12, runeCol: 6, utf16Col: 6},
		{name: "after emoji", byteCol: 17, runeCol: 8, utf16Col: 9},
		{name: "end", byteCol: len(line), runeCol: 10, utf16Col: 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := byteColumnFromRune(line, tt.runeCol); got != tt.byteCol {
				t.Errorf("byteColumnFromRune(%d) = %d, want %d", tt.runeCol, got, tt.byteCol)
			}
			if got := runeColumnFromByte(line, tt.byteCol); got != tt.runeCol {
				t.Errorf("runeColumnFromByte(%d) = %d, want %d", tt.byteCol, got, tt.runeCol)
			}
			if got := clientColumnFromByte(line, tt.byteCol, encodingUTF16); got != tt.utf16Col {
				t.Errorf("clientColumnFromByte(%d, utf-16) = %d, want %d", tt.byteCol, got, tt.utf16Col)
			}
			if got := byteColumnFromClient(line, tt.utf16Col, encodingUTF16); got != tt.byteCol {
				t.Errorf("byteColumnFromClient(%d, utf-16) = %d, want %d", tt.utf16Col, got, tt.byteCol)
			}
			if got := byteColumnFromClient(line, tt.byteCol, encodingUTF8); got != tt.byteCol {
				t.Errorf("byteColumnFromClient(%d, utf-8) = %d, want identity", tt.byteCol, got)
			}
		})
	}

	t.Run("past the end is clamped", func(t *testing.T) {
		if got := byteColumnFromClient(line, 100, encodingUTF16); got != len(line) {
			t.Errorf("got %d, want %d", got, len(line))
		}
		if got := clientColumnFromByte(line, 100, encodingUTF16); got != 11 {
			t.Errorf("got %d, want 11", got)
		}
	})
}

func TestLineAt(t *testing.T) {
	text := "first\nsecond\nthird"
	for i, want := range []string{"first", "second", "third", ""} {
		if got := lineAt(text, i); got != want {
			t.Errorf("lineAt(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestNegotiateEncoding(t *testing.T) {
	tests := []struct {
		offered []string
		want    positionEncoding
	}{
		{offered: nil, want: encodingUTF16},
		{offered: []string{"utf-16"}, want: encodingUTF16},
		{offered: []string{"utf-16", "utf-8", "utf-32"}, want: encodingUTF8},
		{offered: []string{"utf-32"}, want: encodingUTF16},
	}
	for _, tt := range tests {
		if got := negotiateEncoding(tt.offered); got != tt.want {
			t.Errorf("negotiateEncoding(%v) = %s, want %s", tt.offered, got, tt.want)
		}
	}
}

func TestTokenRange(t *testing.T) {
	text := "title: 日本語\n  - {title: 日本語, protocol: http}\n"
	// goccy reports line 2, column 19 (1-based, runes) for "protocol".
	r := tokenRange(text, 2, 19, len("protocol"))
	want := Range{Start: Position{Line: 1, Character: 24}, End: Position{Line: 1, Character: 32}}
	if r != want {
		t.Errorf("tokenRange = %+v, want %+v", r, want)
	}
}

// TestLineAtTextsInTurn checks lines of texts looked up in turn: lineAt
// keeps the line index of the last text, which must not answer for another
// one, even of the same length.
func TestLineAtTextsInTurn(t *testing.T) {
	tests := []struct {
		text string
		line int
		want string
	}{
		{"a\nbc\n", 0, "a"},
		{"ab\nc\n", 0, "ab"}, // the same length, other line breaks
		{"a\nbc\n", 1, "bc"},
		{"ab\nc\n", 1, "c"},
		{"a\nbc\n", 2, ""},
		{"a\nbc\n", 3, ""},
		{"a\nbc\n", -1, ""},
		{"a\r\nb", 0, "a\r"},
		{"a\r\nb", 1, "b"},
		{"", 0, ""},
		{"no newline", 0, "no newline"},
	}
	for _, tt := range tests {
		if got := lineAt(tt.text, tt.line); got != tt.want {
			t.Errorf("lineAt(%q, %d) = %q, want %q", tt.text, tt.line, got, tt.want)
		}
	}
}

// TestLineIndexColumns compares the column conversions of the line index,
// which start from the last mark on lines that are not ASCII only, with
// walking the line from its start, at every column of lines shorter and
// longer than the marks, past their end, and of a line past the last one.
func TestLineIndexColumns(t *testing.T) {
	lines := []string{
		"",
		"plain ascii",
		strings.Repeat("a", 3*markEvery+5),
		strings.Repeat("あ", 3*markEvery), // a multiple of markEvery runes
		strings.Repeat("あ", 3*markEvery+1),
		"x" + strings.Repeat("😀é", 2*markEvery), // surrogate pairs and two-byte runes
		strings.Repeat("a", markEvery-1) + "é" + strings.Repeat("b", markEvery+2),
		// Invalid UTF-8, a rune of one byte each: a lone byte, a truncated
		// rune, a surrogate.
		"\xff" + strings.Repeat("a\xe3\x81", markEvery) + "\xed\xa0\x80",
		"tab\tand\r",
	}
	text := strings.Join(lines, "\n") // the last line has no line break
	idx := newLineIndex(text)
	for i, l := range append(lines, "") {
		errs := 0
		for col := -2; col <= len(l)+3 && errs < 5; col++ {
			if got, want := idx.byteColumn(text, i, col), byteColumnFromRune(l, col); got != want {
				t.Errorf("line %d: byteColumn(%d) = %d, want %d", i, col, got, want)
				errs++
			}
			for _, enc := range []positionEncoding{encodingUTF16, encodingUTF8, ""} {
				if got, want := idx.clientColumn(text, i, col, enc), clientColumnFromByte(l, col, enc); got != want {
					t.Errorf("line %d: clientColumn(%d, %q) = %d, want %d", i, col, enc, got, want)
					errs++
				}
			}
		}
	}
}
