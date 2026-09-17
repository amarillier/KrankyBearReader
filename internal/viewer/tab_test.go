package viewer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareParsedData_RedirectsBinaryContentToHexFallback(t *testing.T) {
	header := []byte("%PDF-1.7\nsome clean looking header text\n")
	binaryTail := make([]byte, 2000)
	for i := range binaryTail {
		binaryTail[i] = byte((i*37 + 11) % 256)
	}
	data := append(header, binaryTail...)

	got := prepareParsedData(FormatText, data)
	if got.format != FormatBinary {
		t.Fatalf("expected mostly-binary content classified as Text to redirect to FormatBinary, got %v", got.format)
	}
	if got.hexTotal != int64(len(data)) {
		t.Errorf("expected hexTotal %d, got %d", len(data), got.hexTotal)
	}
	if len(got.hexSample) > HexViewByteLimit {
		t.Errorf("expected hexSample capped at %d bytes, got %d", HexViewByteLimit, len(got.hexSample))
	}
}

func TestPrepareParsedData_KeepsGenuineTextAsText(t *testing.T) {
	got := prepareParsedData(FormatText, []byte("just a normal plain text file\nwith a couple lines\n"))
	if got.format != FormatText {
		t.Fatalf("expected genuine text to stay FormatText, got %v", got.format)
	}
	if got.raw == nil {
		t.Error("expected raw content to be preserved")
	}
}

func TestPrepareTabData_OversizedFileFallsBackToHex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	// Sparse file: big enough to trip maxParsedFileBytes without actually
	// writing tens of megabytes of real content in the test.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxParsedFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got := prepareTabData(path, FormatText)
	if got.format != FormatBinary {
		t.Fatalf("expected an oversized file to fall back to FormatBinary, got %v", got.format)
	}
}
