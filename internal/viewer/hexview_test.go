package viewer

import (
	"strings"
	"testing"
)

func TestHexDump_OffsetAndASCIIColumns(t *testing.T) {
	dump := hexDump([]byte("Hello, World!"))
	if !strings.HasPrefix(dump, "00000000  ") {
		t.Fatalf("expected dump to start with an 8-digit offset, got: %q", dump)
	}
	if !strings.Contains(dump, "48 65 6c 6c 6f") { // "Hello" in hex
		t.Errorf("expected hex bytes for 'Hello' in dump, got: %q", dump)
	}
	if !strings.Contains(dump, "|Hello, World!") {
		t.Errorf("expected ASCII column with original text, got: %q", dump)
	}
}

func TestHexDump_NonPrintableBytesShownAsDot(t *testing.T) {
	dump := hexDump([]byte{0x00, 0x01, 'A'})
	if !strings.Contains(dump, "|..A|") {
		t.Errorf("expected non-printable bytes rendered as '.', got: %q", dump)
	}
}
