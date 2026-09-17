package viewer

import "testing"

func TestDetectFormat_ByExtension(t *testing.T) {
	cases := map[string]Format{
		"doc.json":        FormatJSON,
		"doc.yaml":        FormatYAML,
		"doc.yml":         FormatYAML,
		"doc.toml":        FormatTOML,
		"README.md":       FormatMarkdown,
		"README.markdown": FormatMarkdown,
		"data.csv":        FormatCSV,
		"data.tsv":        FormatCSV,
		"photo.png":       FormatImage,
		"photo.JPG":       FormatImage,
		"notes.txt":       FormatText,
		"app.log":         FormatText,
		"document.pdf":    FormatPDF,
	}
	for name, want := range cases {
		if got := DetectFormat(name, nil); got != want {
			t.Errorf("DetectFormat(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDetectFormat_SniffsUnknownExtension(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Format
	}{
		{"json without extension", []byte(`{"a": 1, "b": [1,2,3]}`), FormatJSON},
		{"plain prose without extension", []byte("just some plain text notes here"), FormatText},
		{"binary with NUL bytes", []byte{0x00, 0x01, 0x02, 'a', 'b', 0x00, 0xff, 0xfe}, FormatBinary},
		{"png magic bytes", []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0}, FormatImage},
		{"empty file", []byte{}, FormatText},
	}
	for _, c := range cases {
		if got := DetectFormat("data.unknownext", c.data); got != c.want {
			t.Errorf("%s: DetectFormat = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLooksLikeText(t *testing.T) {
	if !looksLikeText([]byte("hello\nworld\t!")) {
		t.Error("expected plain text sample to look like text")
	}
	if looksLikeText([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Error("expected control-byte-heavy sample to not look like text")
	}
}

// TestIsProbablyText_CatchesCleanPrefixOverBinaryBody reproduces, in
// miniature, the real bug found via manual testing: a PDF whose first bytes
// (an ASCII object/metadata header) look like clean text, but whose full
// content is mostly binary stream data. sniffFormat only inspects a small
// sample and would misclassify this as Text; isProbablyText is the
// full-content re-check meant to catch exactly this shape before it's
// handed to a text-rendering widget.
func TestIsProbablyText_CatchesCleanPrefixOverBinaryBody(t *testing.T) {
	header := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\nstream\n")
	binaryTail := make([]byte, 5000)
	for i := range binaryTail {
		binaryTail[i] = byte((i*37 + 11) % 256)
	}
	data := append(header, binaryTail...)

	if isProbablyText(data[:len(header)]) == false {
		t.Fatal("sanity check failed: the clean header alone should look like text")
	}
	if isProbablyText(data) {
		t.Error("expected the full clean-header-plus-binary-body content to NOT look like text")
	}
}

func TestIsProbablyText_ToleratesNonUTF8Text(t *testing.T) {
	// Latin-1 encoded text (e.g. "café" with a raw 0xE9 for 'é') is not
	// valid UTF-8, but it's still legitimate plain text that should render
	// as such, not get redirected to a hex view.
	latin1 := []byte("caf\xe9 au lait, r\xe9sum\xe9, na\xefve")
	if !isProbablyText(latin1) {
		t.Error("expected Latin-1 text to still be treated as text")
	}
}
