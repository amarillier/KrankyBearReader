package viewer

import "testing"

func TestDetectDelimiter(t *testing.T) {
	cases := map[string]rune{
		"a,b,c\n1,2,3\n4,5,6\n":           ',',
		"a\tb\tc\n1\t2\t3\n4\t5\t6\n":     '\t',
		"a;b;c\n1;2;3\n4;5;6\n":           ';',
		"a|b|c\n1|2|3\n4|5|6\n":           '|',
		"just one line, no clear pattern": ',', // fallback
		"":                                ',', // fallback (no lines)
	}
	for sample, want := range cases {
		if got := DetectDelimiter([]byte(sample)); got != want {
			t.Errorf("DetectDelimiter(%q) = %q, want %q", sample, got, want)
		}
	}
}

func TestParseDelimited(t *testing.T) {
	header, rows, err := ParseDelimited([]byte("name,age\nAda,36\nGrace,85\n"), ',')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(header) != 2 || header[0] != "name" || header[1] != "age" {
		t.Fatalf("unexpected header: %v", header)
	}
	if len(rows) != 2 || rows[0][0] != "Ada" || rows[1][0] != "Grace" {
		t.Fatalf("unexpected rows: %v", rows)
	}
}

func TestParseDelimited_HeaderOnly(t *testing.T) {
	header, rows, err := ParseDelimited([]byte("a,b,c\n"), ',')
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(header) != 3 {
		t.Fatalf("unexpected header: %v", header)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no data rows, got %v", rows)
	}
}
