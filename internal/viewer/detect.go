// Package viewer implements the multi-format document viewer: format detection,
// per-format viewer widgets, and the tabbed Manager that hosts them.
package viewer

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Format identifies how a document's bytes should be rendered.
type Format int

const (
	FormatText Format = iota
	FormatJSON
	FormatYAML
	FormatTOML
	FormatXML
	FormatMarkdown
	FormatCSV
	FormatImage
	FormatBinary
	FormatPDF
)

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "JSON"
	case FormatYAML:
		return "YAML"
	case FormatTOML:
		return "TOML"
	case FormatXML:
		return "XML"
	case FormatMarkdown:
		return "Markdown"
	case FormatCSV:
		return "CSV"
	case FormatImage:
		return "Image"
	case FormatBinary:
		return "Binary"
	case FormatPDF:
		return "PDF"
	default:
		return "Text"
	}
}

// extFormats maps a lowercase file extension (with leading dot) to the format it
// implies. Formats not listed here (or an unrecognized extension) fall through to
// content sniffing in sniffFormat.
var extFormats = map[string]Format{
	".json":     FormatJSON,
	".yaml":     FormatYAML,
	".yml":      FormatYAML,
	".toml":     FormatTOML,
	".xml":      FormatXML,
	".md":       FormatMarkdown,
	".markdown": FormatMarkdown,
	".csv":      FormatCSV,
	".tsv":      FormatCSV,
	".png":      FormatImage,
	".jpg":      FormatImage,
	".jpeg":     FormatImage,
	".gif":      FormatImage,
	".bmp":      FormatImage,
	".webp":     FormatImage,
	".txt":      FormatText,
	".log":      FormatText,
	".pdf":      FormatPDF,
}

// sniffSampleSize is how much of a file openAndDetect reads to sniff the format
// of a file whose extension isn't recognized. Kept small so detection never
// meaningfully delays opening a file, however large it is.
const sniffSampleSize = 512

// DetectFormat determines a file's Format from its path and, when the extension
// isn't recognized, a content sample. data may be nil or empty when ext alone is
// decisive; when it isn't, an empty sample is treated as FormatText.
func DetectFormat(path string, data []byte) Format {
	ext := strings.ToLower(filepath.Ext(path))
	if f, ok := extFormats[ext]; ok {
		return f
	}
	return sniffFormat(data)
}

// sniffFormat classifies a content sample using stdlib content-type sniffing
// (net/http, magic-byte based) refined with a JSON validity check, since JSON
// text is otherwise indistinguishable from plain text by content type alone.
func sniffFormat(data []byte) Format {
	if len(data) == 0 {
		return FormatText
	}
	sample := data
	if len(sample) > sniffSampleSize {
		sample = sample[:sniffSampleSize]
	}
	ct := http.DetectContentType(sample)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return FormatImage
	case strings.HasPrefix(ct, "text/") || looksLikeText(sample):
		if json.Valid(data) {
			return FormatJSON
		}
		if looksLikeXML(sample) {
			return FormatXML
		}
		return FormatText
	default:
		return FormatBinary
	}
}

// looksLikeText is a fallback for content http.DetectContentType calls
// "application/octet-stream" (its default when nothing else matches) but which
// is actually plain text without a recognizable signature. It flags data as
// binary once more than 5% of the sample is non-printable, non-whitespace.
func looksLikeText(sample []byte) bool {
	if len(sample) == 0 {
		return true
	}
	nonPrintable := 0
	for _, b := range sample {
		if b == '\n' || b == '\r' || b == '\t' {
			continue
		}
		if b == 0 || b < 0x20 || b == 0x7f {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(len(sample)) <= 0.05
}

// looksLikeXML is a conservative sniff for an extension-less file: it only
// recognizes an explicit XML declaration ("<?xml ...?>"), not a bare root
// element, so an HTML file (which typically opens with "<!DOCTYPE html>" or
// "<html>" and has no such declaration) doesn't get misclassified as XML.
func looksLikeXML(sample []byte) bool {
	trimmed := bytes.TrimLeft(sample, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("<?xml"))
}

// isProbablyText applies looksLikeText's heuristic to an entire file's
// content rather than just a small sniff sample. Used as a final safety
// check right before committing to a text-based view for anything classified
// as Text/Markdown -- whether by extension (which never inspects content at
// all) or by sniffFormat (which only inspects the first sniffSampleSize
// bytes). Both of those can be fooled by a file that starts with clean text
// but turns to binary partway through: confirmed via manual testing, a PDF's
// small ASCII object/metadata header followed by a large compressed binary
// stream sniffed as Text from its first 512 bytes alone, and handing the
// full ~85KB of mostly-binary content to Fyne's text-shaping widget made the
// whole app unresponsive for a long time. Deliberately not also requiring
// utf8.Valid here: that would misclassify legitimate non-UTF-8 text (e.g.
// Latin-1/Windows-1252 files, which are mostly printable high bytes that
// looksLikeText already tolerates) as binary.
func isProbablyText(data []byte) bool {
	return looksLikeText(data)
}

// openAndDetect opens path (surfacing a missing/unreadable file as an error
// before any tab is created) and determines its Format, reading only a small
// sample when the extension doesn't already decide it. The caller is
// responsible for the full read used to actually load the document.
func openAndDetect(path string) (Format, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if format, ok := extFormats[ext]; ok {
		f, err := os.Open(path)
		if err != nil {
			return FormatText, err
		}
		_ = f.Close()
		return format, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return FormatText, err
	}
	defer f.Close()

	sample := make([]byte, sniffSampleSize)
	n, err := io.ReadFull(f, sample)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return FormatText, err
	}
	return sniffFormat(sample[:n]), nil
}
