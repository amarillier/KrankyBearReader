package viewer

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
)

// HexViewByteLimit caps how much of a binary file is ever read/rendered as a
// hex dump. Callers reading a file for the hex fallback should read at most
// this many bytes, passing the file's real size separately so NewHexView can
// note when it's showing a prefix rather than the whole file.
//
// Kept deliberately small: hexDump's offset/hex/ASCII formatting expands
// every source byte to ~4 rendered characters, so even this cap produces a
// sizeable chunk of monospace text for Fyne to lay out. Confirmed via manual
// testing that a much larger cap (64KB, ~4x this) was slow enough opening
// two such files at once to look like a hang.
const HexViewByteLimit = 16 * 1024

// hexDump renders data as classic offset/hex/ASCII rows, 16 bytes per row.
func hexDump(data []byte) string {
	var b strings.Builder
	for offset := 0; offset < len(data); offset += 16 {
		end := offset + 16
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]

		fmt.Fprintf(&b, "%08x  ", offset)
		for i := 0; i < 16; i++ {
			if i < len(chunk) {
				fmt.Fprintf(&b, "%02x ", chunk[i])
			} else {
				b.WriteString("   ")
			}
			if i == 7 {
				b.WriteByte(' ')
			}
		}
		b.WriteString(" |")
		for _, c := range chunk {
			if c >= 0x20 && c < 0x7f {
				b.WriteByte(c)
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteString("|\n")
	}
	return b.String()
}

// NewHexView renders a hex+ASCII dump of sample, the fallback view for a file
// DetectFormat couldn't otherwise classify. totalSize is the file's real size;
// when it's larger than len(sample), a truncation notice is shown up front.
func NewHexView(sample []byte, totalSize int64) fyne.CanvasObject {
	dump := hexDump(sample)
	if int64(len(sample)) < totalSize {
		dump = fmt.Sprintf("Showing first %d of %d bytes\n\n%s", len(sample), totalSize, dump)
	}
	return newReadOnlyEntry(dump, true)
}
