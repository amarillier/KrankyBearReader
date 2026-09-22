// Package pdf renders PDF documents (via github.com/gen2brain/go-fitz, a
// cgo binding to MuPDF) as a tab content for KrankyBearReader's viewer:
// continuous-scroll and page-by-page reading, zoom, and a table-of-contents
// / bookmarks side panel. Ported from ../KrankyBearPDF/pdfviewer's proven
// rendering techniques (DPI-scaled re-render per zoom, a virtualized
// lazy-rendered continuous-scroll layout, pdfcpu-backed bookmarks), adapted
// for a tabbed, multi-document app: each open PDF gets one persistent
// *fitz.Document (closed when its tab closes) and a real LRU page cache,
// rather than pdfviewer's single-document open-render-close-per-call
// approach.
package pdf

import (
	"fmt"
	"image"
	"sync"

	"github.com/gen2brain/go-fitz"
)

// baseRenderDPI is the DPI used at 100% zoom. Matches pdfviewer's renderer.go
// exactly -- this constant, and the DPI formula built on it, are what make
// zoom crisp (a real MuPDF re-render at higher DPI) rather than blurry
// upscaling of a fixed-resolution bitmap.
const baseRenderDPI = 150.0

const (
	minRenderDPI = 50.0
	maxRenderDPI = 600.0
)

// pageCacheCapacity mirrors pdfviewer's 10-entry cache, now with real LRU
// eviction instead of "clear the whole map when full".
const pageCacheCapacity = 10

// Document is one open PDF: a persistent MuPDF handle, its page cache, and
// its bookmarks/TOC. Safe to construct via Prepare off the main goroutine
// (go-fitz and pdfcpu calls are not Fyne calls); Close must be called when
// the owning tab closes, to release the underlying cgo/MuPDF resources.
type Document struct {
	path       string
	doc        *fitz.Document
	pageCount  int
	cache      *pageCache
	Bookmarks  *BookmarkManager
	Highlights []*Highlight

	textCacheMu sync.Mutex
	textCache   map[int]string // page -> extracted text, filled lazily by PageText
}

// Prepare opens path and loads its page count and bookmarks/TOC. Safe to run
// off the main goroutine.
func Prepare(path string) (*Document, error) {
	fd, err := fitz.New(path)
	if err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}

	d := &Document{
		path:      path,
		doc:       fd,
		pageCount: fd.NumPage(),
		cache:     newPageCache(pageCacheCapacity),
		Bookmarks: NewBookmarkManager(),
		textCache: map[int]string{},
	}

	// Both non-fatal: a PDF with no outline/bookmarks, or no annotations, is
	// normal, not an error (matches pdfviewer's own LoadBookmarks handling).
	_ = d.Bookmarks.LoadBookmarks(path)
	d.Highlights, _ = LoadHighlights(path)

	return d, nil
}

// PageText returns page's extracted text (1-based), caching it since find
// re-scans pages on every keystroke and re-extracting would be wasteful.
func (d *Document) PageText(page int) (string, error) {
	d.textCacheMu.Lock()
	defer d.textCacheMu.Unlock()

	if t, ok := d.textCache[page]; ok {
		return t, nil
	}
	t, err := d.doc.Text(page - 1) // go-fitz is 0-based
	if err != nil {
		return "", err
	}
	d.textCache[page] = t
	return t, nil
}

// Close releases the underlying MuPDF document. Must be called exactly once
// when the owning tab closes.
func (d *Document) Close() error {
	d.cache.Clear()
	return d.doc.Close()
}

// Path is the file path this Document was opened from.
func (d *Document) Path() string { return d.path }

// PageCount is the total number of pages.
func (d *Document) PageCount() int { return d.pageCount }

// RenderPage renders page (1-based) at the given zoom (1.0 = 100%, i.e.
// baseRenderDPI), serving from cache when possible. zoom is clamped to a
// DPI range of [minRenderDPI, maxRenderDPI], matching pdfviewer's renderer.
func (d *Document) RenderPage(page int, zoom float64) (image.Image, error) {
	if page < 1 || page > d.pageCount {
		return nil, fmt.Errorf("page %d out of range (1-%d)", page, d.pageCount)
	}

	key := cacheKey(page, zoom)
	if img, ok := d.cache.Get(key); ok {
		return img, nil
	}

	dpi := baseRenderDPI * zoom
	if dpi < minRenderDPI {
		dpi = minRenderDPI
	}
	if dpi > maxRenderDPI {
		dpi = maxRenderDPI
	}

	img, err := d.doc.ImageDPI(page-1, dpi) // go-fitz is 0-based; this API is 1-based like the rest of this app
	if err != nil {
		return nil, fmt.Errorf("rendering page %d: %w", page, err)
	}

	// go-fitz's own render (see CLAUDE.md/ReleaseNotes on this) never paints
	// annotations, highlights included — this is what makes them appear on
	// the page at all, composited in Go rather than by MuPDF. Baked in before
	// caching so it only runs once per unique (page, zoom), same as the base
	// render.
	d.paintHighlights(img, page, dpi)

	d.cache.Put(key, img)
	return img, nil
}
