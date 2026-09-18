package viewer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"reader/internal/viewer/pdf"
)

// tabData is the result of reading and parsing a file, off the main
// goroutine. It holds only plain data — no Fyne widgets — because
// constructing widgets (even just measuring text for one, e.g. a Table's
// column width) touches Fyne's renderer/font internals and isn't safe outside
// the main goroutine. buildTabView turns this into the actual view, and must
// only ever run inside fyne.Do.
type tabData struct {
	format Format
	err    error  // non-nil on a read or parse failure
	raw    []byte // raw bytes, when available, for the plain-text fallback on err

	structured interface{} // decoded JSON/YAML/TOML
	xmlRoot    xmlElement  // decoded XML
	header     []string    // decoded CSV/TSV
	rows       [][]string

	imagePath string // FormatImage: read directly from disk by the view, not here

	hexSample []byte // FormatBinary
	hexTotal  int64

	pdfDoc *pdf.Document // FormatPDF
}

// tabHooks are the optional per-tab behaviors a format's view can provide
// beyond its content, extracted by manager.go once a tab finishes loading:
// Close releases resources when the tab closes (only PDF tabs need this
// today — a persistent *fitz.Document per open tab), and TypedKey handles
// page-navigation-style keys while this tab is the one currently selected
// (so e.g. Space/PageDown page-turn a PDF but don't do anything unexpected
// in a JSON or CSV tab).
type tabHooks struct {
	close    func()
	typedKey func(*fyne.KeyEvent)
}

// newTabContent returns the CanvasObject to use as a tab's Content: a loading
// placeholder shown immediately, replaced once a background goroutine finishes
// reading and parsing the file. Mirrors releasenotes.go's async-load pattern:
// never block the caller, and do the actual widget construction (not just the
// final swap) inside fyne.Do — building a widget off the main goroutine can
// crash outright (confirmed: an off-thread widget.Table column-width call
// segfaulted deep in Fyne's text shaper during manual testing of this
// feature), not just risk a stale repaint.
func newTabContent(win fyne.Window, path string, format Format, onReady func(tabHooks)) fyne.CanvasObject {
	loadingBar := widget.NewProgressBarInfinite()
	loading := container.NewVBox(widget.NewLabel("Loading "+filepath.Base(path)+"..."), loadingBar)
	holder := container.NewStack(loading)

	go func() {
		data := prepareTabData(path, format)
		fyne.Do(func() {
			content, hooks := buildTabView(win, path, data)
			loadingBar.Stop()
			holder.Objects = []fyne.CanvasObject{content}
			holder.Refresh()
			win.Canvas().Refresh(holder)
			if onReady != nil {
				onReady(hooks)
			}
		})
	}()

	return holder
}

// maxParsedFileBytes caps how large a file this app will read fully into
// memory to parse/render as JSON/YAML/TOML/CSV/Markdown/Text. A file over
// this size — of any of those formats — falls back to the same bounded hex
// view as genuinely unrecognized binary content, rather than risking a slow
// or memory-heavy full parse of an unexpectedly huge file. General defense
// alongside isProbablyText's content check: that one guards against a file
// that *looks* like the wrong content; this one guards against a file that's
// simply too big to be worth fully reading, regardless of what it contains.
const maxParsedFileBytes = 25 * 1024 * 1024 // 25MB

// prepareTabData reads and parses path according to format. Safe to run off
// the main goroutine: it touches only the filesystem and pure decoders
// (encoding/json, yaml, toml, encoding/csv), never a Fyne widget.
func prepareTabData(path string, format Format) tabData {
	switch format {
	case FormatImage:
		if _, err := os.Stat(path); err != nil {
			return tabData{format: format, err: err}
		}
		return tabData{format: format, imagePath: path}

	case FormatBinary:
		return prepareHexData(path)

	case FormatPDF:
		doc, err := pdf.Prepare(path)
		if err != nil {
			return tabData{format: format, err: err}
		}
		return tabData{format: format, pdfDoc: doc}

	default:
		if info, err := os.Stat(path); err == nil && info.Size() > maxParsedFileBytes {
			return prepareHexData(path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return tabData{format: format, err: err}
		}
		return prepareParsedData(format, data)
	}
}

func prepareHexData(path string) tabData {
	info, err := os.Stat(path)
	if err != nil {
		return tabData{format: FormatBinary, err: err}
	}
	f, err := os.Open(path)
	if err != nil {
		return tabData{format: FormatBinary, err: err}
	}
	defer f.Close()

	sample := make([]byte, HexViewByteLimit)
	n, err := io.ReadFull(f, sample)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return tabData{format: FormatBinary, err: err}
	}
	return tabData{format: FormatBinary, hexSample: sample[:n], hexTotal: info.Size()}
}

func prepareParsedData(format Format, data []byte) tabData {
	switch format {
	case FormatJSON:
		v, err := DecodeJSON(data)
		if err != nil {
			return tabData{format: format, err: fmt.Errorf("parsing JSON: %w", err), raw: data}
		}
		return tabData{format: format, structured: v}
	case FormatYAML:
		v, err := DecodeYAML(data)
		if err != nil {
			return tabData{format: format, err: fmt.Errorf("parsing YAML: %w", err), raw: data}
		}
		return tabData{format: format, structured: v}
	case FormatTOML:
		v, err := DecodeTOML(data)
		if err != nil {
			return tabData{format: format, err: fmt.Errorf("parsing TOML: %w", err), raw: data}
		}
		return tabData{format: format, structured: v}
	case FormatXML:
		root, err := DecodeXML(data)
		if err != nil {
			return tabData{format: format, err: fmt.Errorf("parsing XML: %w", err), raw: data}
		}
		return tabData{format: format, xmlRoot: root}
	case FormatCSV:
		delim := DetectDelimiter(data)
		header, rows, err := ParseDelimited(data, delim)
		if err != nil {
			return tabData{format: format, err: fmt.Errorf("parsing delimited data: %w", err), raw: data}
		}
		return tabData{format: format, header: header, rows: rows}
	default: // FormatMarkdown, FormatText, and the ultimate fallback for anything unexpected
		if !isProbablyText(data) {
			// Extension-based and sniff-based classification both trusted
			// this file to be text without ever checking its full content
			// (see isProbablyText's doc comment) — reroute to the hex view
			// rather than handing potentially-binary content to Fyne's text
			// widgets.
			return binaryFallbackTabData(data)
		}
		return tabData{format: format, raw: data}
	}
}

// binaryFallbackTabData builds Binary tabData from content already fully
// read into memory (as opposed to prepareHexData, which reads capped bytes
// directly from disk) — used when a file initially classified as
// Text/Markdown turns out, on full inspection, not to be safe to render as
// text after all.
func binaryFallbackTabData(data []byte) tabData {
	sample := data
	if len(sample) > HexViewByteLimit {
		sample = sample[:HexViewByteLimit]
	}
	return tabData{format: FormatBinary, hexSample: sample, hexTotal: int64(len(data))}
}

// buildTabView turns prepared data into the actual widget tree, plus any
// per-tab hooks (close/typedKey) the caller needs to register. Must only run
// on the main goroutine (inside fyne.Do).
func buildTabView(win fyne.Window, path string, d tabData) (fyne.CanvasObject, tabHooks) {
	if d.err != nil {
		return newLoadErrorView(win, path, d.err, d.raw), tabHooks{}
	}
	switch d.format {
	case FormatImage:
		return NewImageView(d.imagePath), tabHooks{}
	case FormatBinary:
		return NewHexView(d.hexSample, d.hexTotal), tabHooks{}
	case FormatPDF:
		handle := pdf.NewView(win, d.pdfDoc)
		return handle.Content, tabHooks{close: handle.Close, typedKey: handle.TypedKey}
	case FormatJSON, FormatYAML, FormatTOML:
		return NewStructuredTreeView(d.structured), tabHooks{}
	case FormatXML:
		return NewXMLTreeView(d.xmlRoot), tabHooks{}
	case FormatCSV:
		return NewCSVView(d.header, d.rows), tabHooks{}
	case FormatMarkdown:
		return NewMarkdownView(win, string(d.raw)), tabHooks{}
	default: // FormatText
		return NewTextView(win, string(d.raw)), tabHooks{}
	}
}

// newLoadErrorView renders a read/parse failure inline. When rawData is
// available (the file was read, only interpreting it failed), a "View as
// Plain Text" button lets the user fall back to seeing the raw content.
func newLoadErrorView(win fyne.Window, path string, err error, rawData []byte) fyne.CanvasObject {
	msg := widget.NewLabel(fmt.Sprintf("Couldn't open %s:\n\n%v", filepath.Base(path), err))
	msg.Wrapping = fyne.TextWrapWord
	box := container.NewVBox(msg)

	if rawData != nil {
		label := "View as Plain Text"
		if !isProbablyText(rawData) {
			label = "View as Hex" // this content didn't parse as the expected format AND doesn't look like text either
		}
		var fallbackView fyne.CanvasObject
		box.Add(widget.NewButton(label, func() {
			if fallbackView != nil {
				return
			}
			if isProbablyText(rawData) {
				fallbackView = NewTextView(win, string(rawData))
			} else {
				sample := rawData
				if len(sample) > HexViewByteLimit {
					sample = sample[:HexViewByteLimit]
				}
				fallbackView = NewHexView(sample, int64(len(rawData)))
			}
			box.Add(fallbackView)
		}))
	}
	return container.NewVScroll(box)
}
