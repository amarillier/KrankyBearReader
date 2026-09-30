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
	"os"
	"sync"

	"github.com/gen2brain/go-fitz"
	"github.com/pdfcpu/pdfcpu/pkg/api"
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
	pageCount  int
	cache      *pageCache
	Bookmarks  *BookmarkManager
	Highlights []*Highlight

	// doc is NOT necessarily opened directly from path — see rebuildDoc's
	// own doc comment for why, and docPath for what it's actually backed
	// by when it isn't.
	doc *fitz.Document

	// docPath is the scratch temp file backing doc, when doc is a
	// normalized copy rather than path itself opened directly — empty
	// when rebuildDoc fell back to opening path directly (see its own
	// doc comment). Tracked so it can be cleaned up on the next rebuild
	// or on Close.
	docPath string

	// pendingHighlightDeletes holds the ObjNr of every on-disk highlight
	// removed via DeleteHighlight since the last save — see SaveHighlights
	// and rebuildDoc.
	pendingHighlightDeletes []int

	// selectedHighlight is drawn with an extra outline by paintHighlights,
	// so the page visibly matches whichever row is selected in the
	// Highlights panel — see SetSelectedHighlight.
	selectedHighlight *Highlight

	textCacheMu sync.Mutex
	textCache   map[int]string // page -> extracted text, filled lazily by PageText

	lineCacheMu sync.Mutex
	lineCache   map[int][]textLine // page -> HTML-derived line positions, filled lazily by pageTextLines

	// searchHighlightPage/Rect mark the find bar's current match's
	// approximate on-page box (PDF-space, origin bottom-left, same
	// convention as Highlight.Rect) for paintHighlights to draw — see
	// SetSearchHighlight. Page 0 means "none".
	searchHighlightPage int
	searchHighlightRect [4]float64
}

// Prepare opens path and loads its page count and bookmarks/TOC. Safe to run
// off the main goroutine.
func Prepare(path string) (*Document, error) {
	d := &Document{
		path:      path,
		cache:     newPageCache(pageCacheCapacity),
		Bookmarks: NewBookmarkManager(),
		textCache: map[int]string{},
		lineCache: map[int][]textLine{},
	}

	if err := d.rebuildDoc(); err != nil {
		return nil, fmt.Errorf("opening PDF: %w", err)
	}
	d.pageCount = d.doc.NumPage()

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
	err := d.closeDoc()
	return err
}

// Path is the file path this Document was opened from.
func (d *Document) Path() string { return d.path }

// PageBoundsPt returns page's (1-based) size in PDF user-space points
// (origin bottom-left) — the same call paintHighlights uses to map a
// highlight's /QuadPoints onto a rendered image, and what the highlight-
// drawing UI uses in reverse, to turn a drawn rectangle back into
// /QuadPoints.
func (d *Document) PageBoundsPt(page int) (w, h float64, err error) {
	b, err := d.doc.Bound(page - 1) // go-fitz is 0-based
	if err != nil {
		return 0, 0, err
	}
	return float64(b.Dx()), float64(b.Dy()), nil
}

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

	// ImageWithAnnotsDPI, not the plain ImageDPI upstream go-fitz ships (see
	// CLAUDE.md/ReleaseNotes): a fork (github.com/amarillier/go-fitz, wired
	// in via go.mod's replace directive) that also calls MuPDF's own
	// fz_run_page, so every ALREADY-SAVED annotation — Stamp included, whose
	// arbitrary vector-art appearance this app has no hand-rolled draw
	// routine for at all — renders pixel-accurately, the same as a real PDF
	// viewer. paintHighlights (below) now only hand-paints annotations that
	// don't exist in the file yet (ObjNr == 0: freshly drawn, not yet
	// saved) — MuPDF already painted everything on disk correctly, so
	// hand-painting those too would double them up.
	//
	// d.doc is a normalized copy, not path opened directly — see
	// rebuildDoc's own doc comment for why that matters here.
	img, err := d.doc.ImageWithAnnotsDPI(page-1, dpi) // go-fitz is 0-based; this API is 1-based like the rest of this app
	if err != nil {
		return nil, fmt.Errorf("rendering page %d: %w", page, err)
	}

	d.paintHighlights(img, page, dpi)

	d.cache.Put(key, img)
	return img, nil
}

// rebuildDoc closes d.doc (if any) and reopens it: NOT path directly, but a
// freshly-normalized copy — read path via pdfcpu, apply every pending
// highlight edit (deletes, color/caption changes not yet saved — see
// applyPendingHighlightEdits), write that to a scratch temp file, and open
// THAT with go-fitz. Called once from Prepare, and again after anything
// that changes what should render (DeleteHighlight, SetHighlightColor,
// SetHighlightCaption, a successful SaveHighlights).
//
// Why bother normalizing even when there's nothing pending to apply: a
// real PDF that's been through several rounds of saving (this app's own,
// interleaved with another app's, e.g. Preview's) can accumulate a
// cross-reference table MuPDF has to silently repair just to open — and
// under repair, MuPDF isn't guaranteed to resolve every object to its
// truly-latest revision. Confirmed in practice, more than once: a
// Highlight rendering with no color at all, a Line annotation rendering an
// old straight-line revision of an arrow that had since been curved in
// Preview — both on a file MuPDF printed a repair warning for, both gone
// after one clean read+write round-trip through pdfcpu. Rendering from a
// freshly-normalized copy on every open sidesteps needing to *detect*
// "does this file need repair" at all (tried that separately — go-fitz's
// own repair-detection turned out to leak state across unrelated documents
// in the same process, unsafe for a long-running multi-tab app — see
// CLAUDE.md) by just always producing a clean copy, unconditionally.
//
// Deliberately does NOT call api.OptimizeContext (see CLAUDE.md's own
// section on why: it prunes any object it doesn't see a live reference
// to, and doesn't understand an annotation's opaque private data, e.g.
// Preview's own AAPL:AKExtras metadata, as containing real references —
// pruning one of those degrades Preview's own rendering and can eventually
// make annotation deletion fail outright). A plain read-via-pdfcpu,
// write-fresh round trip already produces one clean, non-incremental
// revision on its own, without that risk.
//
// Falls back to opening path directly if normalizing fails for any reason
// — so a file this app can't safely round-trip through pdfcpu can still be
// viewed, just without this fix — UNLESS a doc is already open, in which
// case the existing one is left alone rather than disrupting an already-
// working render over a transient rebuild failure.
func (d *Document) rebuildDoc() error {
	fd, docPath, err := d.buildNormalizedDoc()
	if err != nil {
		if d.doc != nil {
			return err
		}
		raw, rawErr := fitz.New(d.path)
		if rawErr != nil {
			return rawErr
		}
		d.doc = raw
		d.docPath = ""
		return nil
	}

	_ = d.closeDoc()
	d.doc = fd
	d.docPath = docPath
	return nil
}

// buildNormalizedDoc reads d.path via pdfcpu, applies every pending
// highlight edit, writes the result to a fresh scratch temp file, and
// opens that with go-fitz — the actual work rebuildDoc's own doc comment
// describes. Returns the temp file's path alongside the opened document so
// the caller can track it for later cleanup.
//
// A pending COLOR change on an already-saved highlight is applied by
// deleting that annotation from this scratch copy entirely (folded into
// the same removeAnnotationsRepairingIfNeeded call pendingHighlightDeletes
// already uses), not by patching its /C in place. Found necessary the hard
// way: a real Preview/Acrobat-authored annotation already carries its own
// baked /AP appearance stream, and MuPDF's real annotation rendering (see
// CLAUDE.md) always paints from that stream when present — confirmed
// empirically (a before/after pixel dump on a real file's actual
// highlight showed the exact same pixels after rewriting /C to a
// different color) that updating /C alone has NO visual effect once /AP
// exists, no matter how many times the file is re-normalized. Deleting
// the annotation from this scratch copy removes its baked appearance
// entirely, so paintHighlights (annotations.go) can safely hand-paint it
// in its new color on top — the exact same code path already used for a
// brand-new, not-yet-saved highlight, now also covering "existing, but
// its rendered color is stale" for the same reason: MuPDF has nothing of
// its own to show for it in this scratch copy either way. This is safe
// against a DIFFERENT overlapping annotation the same way Delete already
// is (see CLAUDE.md's "Making a deleted highlight disappear immediately"
// section): it's a real, full MuPDF render of a file with only this one
// annotation missing, not a hand-painted patch over a region.
func (d *Document) buildNormalizedDoc() (*fitz.Document, string, error) {
	ctx, err := api.ReadContextFile(d.path)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", d.path, err)
	}

	removeObjNrs := append([]int{}, d.pendingHighlightDeletes...)
	for _, h := range d.Highlights {
		if h.ObjNr <= 0 {
			continue
		}
		if _, colorChanged := highlightDictChanges(h); colorChanged {
			removeObjNrs = append(removeObjNrs, h.ObjNr)
		}
	}
	if len(removeObjNrs) > 0 {
		if err := removeAnnotationsRepairingIfNeeded(ctx, removeObjNrs); err != nil {
			return nil, "", fmt.Errorf("removing pending highlights: %w", err)
		}
	}
	// Only a caption edit can still land here (a color-changed highlight
	// was just deleted above, so DereferenceDict on its ObjNr will find
	// nothing and applyPendingHighlightEdits' own existing nil-dict check
	// skips it harmlessly) — captions aren't painted at all (see
	// paintHighlights), so this has no rendering effect either way, but
	// keeps this scratch copy's behavior symmetric with the real save.
	if err := applyPendingHighlightEdits(ctx, d.Highlights); err != nil {
		return nil, "", fmt.Errorf("applying pending highlight edits: %w", err)
	}

	tmp, err := os.CreateTemp("", "kbr-render-*.pdf")
	if err != nil {
		return nil, "", fmt.Errorf("creating render temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close() // api.WriteContextFile opens the destination itself
	if err := api.WriteContextFile(ctx, tmpPath); err != nil {
		os.Remove(tmpPath)
		return nil, "", fmt.Errorf("writing normalized copy: %w", err)
	}

	fd, err := fitz.New(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return nil, "", fmt.Errorf("opening normalized copy: %w", err)
	}
	return fd, tmpPath, nil
}

// closeDoc releases d.doc and deletes its backing scratch temp file, if it
// has one (see docPath's own doc comment) — called before rebuildDoc opens
// a replacement, and from Close when the owning tab closes.
func (d *Document) closeDoc() error {
	var err error
	if d.doc != nil {
		err = d.doc.Close()
		d.doc = nil
	}
	if d.docPath != "" {
		os.Remove(d.docPath)
		d.docPath = ""
	}
	return err
}
