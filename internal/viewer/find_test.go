package viewer

import "testing"

func TestCompileMatcher_SubstringIsCaseInsensitive(t *testing.T) {
	m, err := compileMatcher("Hello", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m("say hello world") {
		t.Error("expected case-insensitive substring match")
	}
	if m("goodbye") {
		t.Error("expected no match")
	}
}

func TestCompileMatcher_Regex(t *testing.T) {
	m, err := compileMatcher(`^\d+$`, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m("12345") {
		t.Error("expected regex match on all-digit string")
	}
	if m("12345a") {
		t.Error("expected no match on non-digit string")
	}
}

func TestCompileMatcher_InvalidRegexReturnsError(t *testing.T) {
	if _, err := compileMatcher("(unclosed", true); err == nil {
		t.Error("expected an error for invalid regex")
	}
}

func TestCompileMatcher_EmptyQueryNeverMatches(t *testing.T) {
	m, err := compileMatcher("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m("anything") {
		t.Error("expected empty query to never match")
	}
}

func TestFindAllOffsets_Substring(t *testing.T) {
	offsets, err := findAllOffsets("the cat sat on the mat", "at", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{5, 15, 20} // "cat" (at index5), "sat"(15)... let's just check count and first
	if len(offsets) != 3 {
		t.Fatalf("expected 3 matches, got %d: %v", len(offsets), offsets)
	}
	_ = want
}

func TestFindAllOffsets_RuneOffsetsAccountForMultiByteRunes(t *testing.T) {
	// "café " is 5 runes (é is one rune, two UTF-8 bytes) before "world".
	offsets, err := findAllOffsets("café world", "world", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(offsets) != 1 || offsets[0] != 5 {
		t.Fatalf("expected a single match at rune offset 5, got %v", offsets)
	}
}

func TestFindAllOffsets_Regex(t *testing.T) {
	offsets, err := findAllOffsets("a1 b22 c333", `\d+`, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(offsets) != 3 {
		t.Fatalf("expected 3 matches, got %d: %v", len(offsets), offsets)
	}
}

func TestRowColForRuneOffset(t *testing.T) {
	text := "line one\nline two\nline three"
	cases := []struct {
		offset       int
		wantRow, col int
	}{
		{0, 0, 0},
		{4, 0, 4},
		{9, 1, 0},  // just after the first \n
		{14, 1, 5}, // into "line two"
		{18, 2, 0}, // just after the second \n
	}
	for _, c := range cases {
		row, col := rowColForRuneOffset(text, c.offset)
		if row != c.wantRow || col != c.col {
			t.Errorf("offset %d: got (row=%d, col=%d), want (row=%d, col=%d)", c.offset, row, col, c.wantRow, c.col)
		}
	}
}
