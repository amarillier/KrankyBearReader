package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// AddHighlight creates a new Highlight-kind annotation in memory, covering
// quads (PDF user-space points, origin bottom-left — one [8]float64 per
// quad, same convention as Highlight.Quads) on page, with rgb and an
// optional caption. Nothing reaches disk until SaveHighlights runs; the
// returned Highlight has ObjNr == 0 until then (see Highlight.ObjNr).
//
// Re-sorts d.Highlights by page afterward — LoadHighlights' own initial
// order is already page-sorted (see flattenHighlights), and the Highlights
// panel's list just displays d.Highlights in whatever order it's in, so
// without this a highlight added on an earlier page (e.g. page 13, after
// scrolling back from page 77) would show up at the bottom of the list,
// after every higher-numbered page's entries — simply because it was the
// most recently appended, not because of where it actually is in the
// document. A stable sort keeps same-page entries (including this new one
// relative to any others already on its page) in their existing relative
// order.
//
// Clears the page cache so the new highlight paints on the very next
// RenderPage call at every zoom level — simpler and cheap enough (a
// 10-entry LRU) than tracking which cache keys belong to this one page.
func (d *Document) AddHighlight(page int, quads [][8]float64, rgb [3]float64, caption string) *Highlight {
	h := &Highlight{
		Page:     page,
		Kind:     "Highlight",
		Contents: caption,
		Quads:    quads,
		Color:    rgb,
	}
	d.Highlights = append(d.Highlights, h)
	sort.SliceStable(d.Highlights, func(i, j int) bool { return d.Highlights[i].Page < d.Highlights[j].Page })
	d.cache.Clear()
	return h
}

// DeleteHighlight removes h — geometry, color, caption, all of it —
// immediately from the in-memory list and clears the page cache, so it
// disappears on the very next RenderPage call. If h was already on disk
// (ObjNr > 0), its object number is queued in pendingHighlightDeletes for
// removal from the PDF too, applied the next time SaveHighlights runs.
func (d *Document) DeleteHighlight(h *Highlight) bool {
	for i, existing := range d.Highlights {
		if existing != h {
			continue
		}
		d.Highlights = append(d.Highlights[:i], d.Highlights[i+1:]...)
		if h.ObjNr > 0 {
			d.pendingHighlightDeletes = append(d.pendingHighlightDeletes, h.ObjNr)
		}
		d.cache.Clear()
		return true
	}
	return false
}

// SaveHighlights writes every pending highlight change — captions edited via
// SetHighlightCaption, new ones added via AddHighlight, and removals via
// DeleteHighlight — into a fresh read of d.path, saving the result to
// outputPath. Existing highlights' own geometry/color and every other
// annotation kind are left untouched.
//
// pdfcpu's public API has no "update an existing annotation" call, only
// Add/Remove (see ReleaseNotes' Future ideas), so captions bypass it the
// same way highlightGeometry bypasses pdfcpu's read-back gap for
// Quads/Color: re-dereference each existing highlight's raw dict by its own
// ObjNr and mutate /Contents directly, via Dict.Update (not
// InsertString/Insert, which silently no-ops when the key already exists —
// a real highlight almost always already carries a, possibly empty,
// /Contents entry from whatever app created it). Removals and additions do
// go through pdfcpu's real Remove/Add API (pdfcpu.RemoveAnnotations,
// pdfcpu.AddAnnotationsMap) against the very same *model.Context, so caption
// edits, removals, and additions all land in one write.
//
// Rereads d.path fresh rather than reusing any earlier-opened *model.Context
// (LoadHighlights' own ctx is not kept around): object numbers are only
// meaningful against the specific file revision they were read from, and
// this keeps that revision's window as short as possible.
//
// On success, if outputPath is d.path itself (an overwrite, not a Save-As
// copy), reloads d.Highlights from the file just written and clears
// pendingHighlightDeletes — every ObjNr==0 highlight just became a real
// annotation with its own ObjNr, and every pending delete was just applied,
// so both must be re-derived from the new on-disk state or the next save
// would re-add or re-delete them. A Save-As copy leaves d.path (and so this
// bookkeeping) untouched, matching bookmarkPanel's save behavior: it's an
// export of the current state, not a commit to it, and repeating it against
// unchanged in-memory state must not duplicate anything either.
func (d *Document) SaveHighlights(outputPath string) error {
	ctx, err := api.ReadContextFile(d.path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", d.path, err)
	}

	for _, h := range d.Highlights {
		if h.ObjNr <= 0 {
			continue
		}
		dict, err := ctx.XRefTable.DereferenceDict(*types.NewIndirectRef(h.ObjNr, 0))
		if err != nil || dict == nil {
			continue
		}
		s, err := types.EscapedUTF16String(h.Contents)
		if err != nil {
			return fmt.Errorf("encoding caption for p.%d %s: %w", h.Page, h.Kind, err)
		}
		dict.Update("Contents", types.StringLiteral(*s))
		if h.Kind == "Highlight" {
			// Color is only ever populated for Kind == "Highlight"
			// (flattenHighlights only calls highlightGeometry for
			// model.AnnHighLight) — for every other kind it's sitting at
			// its Go zero value, [3]float64{} (black), not that
			// annotation's real /C. Gating on Kind here, the same way
			// flattenHighlights gates populating it in the first place,
			// stops SaveHighlights from silently blackening every
			// Underline/Strikeout/Squiggly/Note's real color.
			dict.Update("C", rgbToSimpleColor(h.Color).Array())
		}
	}

	if len(d.pendingHighlightDeletes) > 0 {
		if _, err := pdfcpu.RemoveAnnotations(ctx, nil, nil, d.pendingHighlightDeletes, false); err != nil {
			return fmt.Errorf("removing highlights: %w", err)
		}
	}

	newByPage := map[int][]model.AnnotationRenderer{}
	for _, h := range d.Highlights {
		if h.ObjNr > 0 || len(h.Quads) == 0 {
			continue // already on disk, or has no geometry to write
		}
		minX, minY, maxX, maxY := quadBoundsPt(h.Quads[0])
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
		col := rgbToSimpleColor(h.Color)
		ann := model.NewHighlightAnnotation(*rect, 0, h.Contents, "", "", 0, &col, 0, 0, 0, "", nil, nil, "", "", quad)
		newByPage[h.Page] = append(newByPage[h.Page], ann)
	}
	if len(newByPage) > 0 {
		if _, err := pdfcpu.AddAnnotationsMap(ctx, newByPage, false); err != nil {
			return fmt.Errorf("adding highlights: %w", err)
		}
	}

	if err := writeContextAtomically(ctx, outputPath); err != nil {
		return err
	}

	if outputPath == d.path {
		reloaded, err := LoadHighlights(d.path)
		if err != nil {
			return fmt.Errorf("reloading %s after save: %w", d.path, err)
		}
		d.Highlights = reloaded
		d.pendingHighlightDeletes = nil
	}
	return nil
}

// writeContextAtomically writes ctx to outputPath via a temp file in the
// same directory, renaming over the destination only once the write fully
// succeeds — so a save that fails partway (full disk, permissions, a crash)
// never leaves outputPath truncated or corrupt. Needed because
// api.WriteContextFile writes straight to outputPath with no such staging,
// unlike the higher-level api.Add/Remove*File functions (bookmarks,
// annotations), which stage internally via pdfcpu's own unexported
// openStagedOutput.
func writeContextAtomically(ctx *model.Context, outputPath string) error {
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".highlight-*.pdf.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close() // api.WriteContextFile opens outFile itself

	if err := api.WriteContextFile(ctx, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("finalizing %s: %w", outputPath, err)
	}
	return nil
}

// SetHighlightCaption updates h's in-memory caption (its /Contents comment)
// immediately — mirrors BookmarkManager.RenameBookmark. Nothing reaches disk
// until SaveHighlights runs. Unlike a bookmark title, blank is accepted: it
// clears the caption.
func (d *Document) SetHighlightCaption(h *Highlight, caption string) {
	h.Contents = caption
}

// SetHighlightColor updates h's in-memory color (rgb, 0..1 per channel)
// immediately and clears the page cache so the new color paints on the
// very next RenderPage call — mirrors AddHighlight/DeleteHighlight. Nothing
// reaches disk until SaveHighlights runs, which rewrites every existing
// highlight's /C from this same field (see SaveHighlights).
func (d *Document) SetHighlightColor(h *Highlight, rgb [3]float64) {
	h.Color = rgb
	d.cache.Clear()
}

// rgbToSimpleColor converts our internal 0..1 RGB representation
// (Highlight.Color's convention) into pdfcpu's own color type, used both
// for a brand-new highlight's initial /C (via model.NewHighlightAnnotation)
// and for rewriting an existing one's /C in SaveHighlights.
func rgbToSimpleColor(rgb [3]float64) color.SimpleColor {
	return color.SimpleColor{R: float32(rgb[0]), G: float32(rgb[1]), B: float32(rgb[2])}
}
