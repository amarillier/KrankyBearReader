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

// addShape appends h to d.Highlights, re-sorted by page, and clears the
// render cache — the exact bookkeeping AddHighlight/AddRectShape/
// AddLineShape all share, factored out once a second and third "add a new
// shape" method existed (AddHighlight itself predates this and could be
// rewritten in terms of it, but isn't, to avoid an unrelated diff on
// already-working, already-tested code).
func (d *Document) addShape(h *Highlight) *Highlight {
	d.Highlights = append(d.Highlights, h)
	sort.SliceStable(d.Highlights, func(i, j int) bool { return d.Highlights[i].Page < d.Highlights[j].Page })
	d.cache.Clear()
	return h
}

// AddRectShape creates a new Square- or Circle-kind annotation in memory,
// covering rect ([minX, minY, maxX, maxY], PDF user-space points, origin
// bottom-left — same convention as Highlight.Rect) on page, with rgb as
// its stroke color. Nothing reaches disk until SaveHighlights runs; the
// returned Highlight has ObjNr == 0 until then, same as AddHighlight.
// kind must be "Square" or "Circle" — see genericRectKinds — anything
// else is a caller bug, not a user-facing error, so this doesn't validate
// it.
func (d *Document) AddRectShape(page int, kind string, rect [4]float64, rgb [3]float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: kind, Rect: rect, Color: rgb})
}

// AddLineShape creates a new Line-kind annotation (a straight line or
// arrow) in memory, from line's two endpoints ([x1, y1, x2, y2], PDF
// user-space points — same convention as Highlight.Line) with endStyle
// ([start, end] — see Highlight.LineEndStyle) and rgb as its color.
// Nothing reaches disk until SaveHighlights runs; the returned Highlight
// has ObjNr == 0 until then, same as AddHighlight.
func (d *Document) AddLineShape(page int, line []float64, endStyle [2]string, rgb [3]float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: "Line", Line: line, LineEndStyle: endStyle, Color: rgb})
}

// AddPolygonShape creates a new Polygon-kind annotation in memory — a
// Star or Hexagon (see view_render.go's starVertices/hexagonVertices;
// PDF itself has no dedicated subtype for either, so both are authored
// as a generic Polygon, same as any other closed-shape annotation) — from
// vertices (PDF user-space points, one [2]float64 per vertex, in drawing
// order; the shape closes back to the first), with rgb as its stroke
// color. Rect is derived from vertices' own bounding box, the same
// click-select/outline geometry every genericRectKinds member carries.
// Nothing reaches disk until SaveHighlights runs; the returned Highlight
// has ObjNr == 0 until then, same as AddHighlight/AddRectShape/
// AddLineShape.
func (d *Document) AddPolygonShape(page int, vertices [][2]float64, rgb [3]float64) *Highlight {
	minX, minY, maxX, maxY := verticesBoundsPt(vertices)
	return d.addShape(&Highlight{Page: page, Kind: "Polygon", Vertices: vertices, Rect: [4]float64{minX, minY, maxX, maxY}, Color: rgb})
}

// AddTextShape creates a new FreeText-kind annotation in memory — a plain
// text block (calloutTip nil) or a speech bubble (calloutTip set to the
// tail's far end, in PDF user-space points — see view_render.go's
// calloutTipFor) — covering rect ([minX, minY, maxX, maxY], PDF
// user-space points) on page, with caption as its text content and rgb
// as its stroke/text color. Nothing reaches disk until SaveHighlights
// runs; the returned Highlight has ObjNr == 0 until then, same as every
// other Add* method.
func (d *Document) AddTextShape(page int, rect [4]float64, calloutTip *[2]float64, caption string, rgb [3]float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: "FreeText", Rect: rect, CalloutTip: calloutTip, Contents: caption, Color: rgb})
}

// DeleteHighlight removes h — geometry, color, caption, all of it —
// immediately from the in-memory list and clears the page cache, so it
// disappears on the very next RenderPage call. For a not-yet-saved
// highlight (ObjNr == 0), that's enough on its own — paintHighlights only
// ever hand-paints those in the first place. For an already-saved one
// (ObjNr > 0), RenderPage's base image comes from MuPDF's own real
// annotation rendering, which would otherwise keep faithfully painting it
// straight from the file regardless of this in-memory list — confirmed
// empirically, not just assumed, the first time this was tried (a
// before/after pixel diff of the same render showed zero difference).
// Queuing its ObjNr in pendingHighlightDeletes (also used for the real
// on-disk removal the next time SaveHighlights runs) and rebuilding d.doc
// (rebuildDoc, in document.go) is what fixes that: the rebuild renders a
// fresh normalized copy with every pending delete actually removed, and
// RenderPage renders from that, rather than hand-painting an "erase this
// region" patch over the previous render — which can't correctly handle
// this highlight's own bounding box overlapping a different, still-live
// annotation (confirmed in practice with the first approach tried here;
// see CLAUDE.md).
func (d *Document) DeleteHighlight(h *Highlight) bool {
	for i, existing := range d.Highlights {
		if existing != h {
			continue
		}
		d.Highlights = append(d.Highlights[:i], d.Highlights[i+1:]...)
		if h.ObjNr > 0 {
			d.pendingHighlightDeletes = append(d.pendingHighlightDeletes, h.ObjNr)
			// Best-effort: on failure d.doc is left as-is (rebuildDoc's own
			// doc comment) rather than breaking the delete outright — the
			// highlight is still gone from d.Highlights and the panel
			// either way, just without the immediate re-render.
			_ = d.rebuildDoc()
		}
		d.cache.Clear()
		return true
	}
	return false
}

// highlightDictChanges reports whether h's Contents or Color (Color only
// for a hasPaintedColor kind — see its own doc comment on why Color is
// meaningless, at its Go zero value, for any other kind) have diverged
// from origContents/origColor, the snapshot taken when h was loaded.
// SaveHighlights uses this to decide whether an existing highlight's dict
// needs touching at all — see its own doc comment on why "at all" matters,
// not just "which fields."
func highlightDictChanges(h *Highlight) (contentsChanged, colorChanged bool) {
	return h.Contents != h.origContents, hasPaintedColor(h.Kind) && h.Color != h.origColor
}

// applyPendingHighlightEdits rewrites each already-saved highlight's
// /Contents and/or /C directly on ctx's own xref table, for exactly the
// highlights highlightDictChanges reports as actually diverged from what
// was loaded — never touching one that hasn't changed at all (see
// highlightDictChanges' and SaveHighlights' own doc comments on why "at
// all" matters, not just "which fields"). Shared between SaveHighlights
// (writing the real file) and buildNormalizedDoc (writing a scratch
// render copy), so a caption/color edit not yet saved shows up identically
// in both: an immediate re-render via the normalized copy, and the actual
// saved file once Save runs.
func applyPendingHighlightEdits(ctx *model.Context, highlights []*Highlight) error {
	for _, h := range highlights {
		if h.ObjNr <= 0 {
			continue
		}
		contentsChanged, colorChanged := highlightDictChanges(h)
		if !contentsChanged && !colorChanged {
			continue
		}
		dict, err := ctx.XRefTable.DereferenceDict(*types.NewIndirectRef(h.ObjNr, 0))
		if err != nil || dict == nil {
			continue
		}
		if contentsChanged {
			s, err := types.EscapedUTF16String(h.Contents)
			if err != nil {
				return fmt.Errorf("encoding caption for p.%d %s: %w", h.Page, h.Kind, err)
			}
			dict.Update("Contents", types.StringLiteral(*s))
		}
		if colorChanged {
			// Color is only ever populated for a kind hasPaintedColor
			// reports true for (flattenHighlights only calls
			// highlightGeometry/lineGeometry for those) — for "Note" it's
			// sitting at its Go zero value, [3]float64{} (black), not that
			// annotation's real /C. Gating on Kind here, the same way
			// flattenHighlights gates populating it in the first place,
			// stops this from silently blackening a Note's real color.
			dict.Update("C", rgbToSimpleColor(h.Color).Array())
		}
	}
	return nil
}

// SaveHighlights writes every pending highlight change — captions edited via
// SetHighlightCaption, colors changed via SetHighlightColor, new ones added
// via AddHighlight, and removals via DeleteHighlight — into a fresh read of
// d.path, saving the result to outputPath. Existing highlights' own
// geometry and every other annotation kind are left untouched, and so is
// an existing highlight's own /Contents or /C when Contents/Color hasn't
// actually diverged from what was loaded (see origContents/origColor on
// Highlight) — deliberately, not just as an optimization: rewriting an
// untouched annotation's dict forces pdfcpu to fully re-emit it rather
// than copy its bytes through unchanged, which is enough to make Preview
// stop trusting a Preview-authored annotation's own private editing
// metadata (see CLAUDE.md's note on this, found the hard way on a real
// file).
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

	if err := applyPendingHighlightEdits(ctx, d.Highlights); err != nil {
		return err
	}

	if len(d.pendingHighlightDeletes) > 0 {
		if err := removeAnnotationsRepairingIfNeeded(ctx, d.pendingHighlightDeletes); err != nil {
			return fmt.Errorf("removing highlights: %w", err)
		}
	}

	newByPage := map[int][]model.AnnotationRenderer{}
	for _, h := range d.Highlights {
		if h.ObjNr > 0 {
			continue // already on disk
		}
		ann := newAnnotationForShape(h)
		if ann == nil {
			continue // no geometry to write -- shouldn't happen for anything created via this app's own Add* methods
		}
		newByPage[h.Page] = append(newByPage[h.Page], ann)
	}
	if len(newByPage) > 0 {
		if _, err := pdfcpu.AddAnnotationsMap(ctx, newByPage, false); err != nil {
			return fmt.Errorf("adding highlights: %w", err)
		}
	}

	// Deliberately NOT calling api.OptimizeContext here — see CLAUDE.md.
	// It was added to flatten an already-inconsistent incremental-update
	// chain (real symptom: a Line's arrowhead and a Highlight's color
	// both stopped rendering after several rounds of saves), and it did
	// fix that. It also turned out to actively cause worse damage of its
	// own: pdfcpu's optimizer prunes any object it doesn't see a live
	// reference to — but an object referenced ONLY from inside an
	// annotation's opaque, app-private data (e.g. Preview's own
	// AAPL:AKExtras metadata) looks unreferenced to it, so it got pruned
	// silently. That's degraded Preview's own rendering of an annotation
	// it authored, and on a real file it eventually made ANY future
	// annotation removal on that page fail outright ("missing xref table
	// entry" — see removeAnnotationsRepairingIfNeeded) once enough saves
	// had compounded. A cosmetic "needs repair" warning MuPDF silently
	// handles on its own is a far smaller problem than a hard, blocking
	// save failure — not worth risking to fix.
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

		// The real file now has every pending edit actually applied, so
		// rebuild d.doc from the freshly-saved bytes — the reloaded
		// Highlights all have origContents/origColor matching their new
		// Contents/Color (see LoadHighlights), so this rebuild is pure
		// normalization, not a re-application of anything (see rebuildDoc's
		// own doc comment for why normalizing at all, even with nothing
		// pending, still matters). Best-effort: a failure here leaves the
		// previous d.doc in place rather than failing the save itself, which
		// already succeeded.
		_ = d.rebuildDoc()

		// Every already-rendered page in the cache was rendered from
		// d.path's PREVIOUS on-disk bytes, which this save just replaced —
		// found necessary the hard way, not preemptively: a page viewed
		// before Save to PDF kept showing its pre-save render until the
		// tab was fully closed and reopened, since nothing was clearing
		// this cache to force a fresh RenderPage call for pages already in
		// it. RenderPage's own cache key is only (page, zoom), with no
		// notion of "which save wrote the file this came from" — so the
		// only correct fix is to drop everything, the same as
		// AddHighlight/DeleteHighlight/SetHighlightColor already do for
		// the one page they know they changed, just for every page at
		// once, since a save can change what's on any of them, not just
		// the highlights this app knows it edited.
		d.cache.Clear()
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

// removeAnnotationsRepairingIfNeeded calls pdfcpu.RemoveAnnotations, and if
// that fails, repairs any dangling indirect reference found anywhere in
// ctx's own xref table (repairDanglingIndirectReferences) and retries
// exactly once. Found necessary the hard way, not preemptively: pdfcpu's
// own removal path validates the FULL object graph reachable from a
// page's annotations before allowing a removal — including inside opaque,
// app-private data this app has no reason to understand, e.g. Preview's
// own AAPL:AKExtras annotation metadata — and hard-fails on the very first
// unresolvable reference it finds, with no lenient mode. A real file hit
// this: some earlier save (most likely one of this app's own now-removed
// OptimizeContext calls — see CLAUDE.md) had silently pruned an object
// only reachable via a private blob's own embedded copy of a reference,
// and the very next annotation deletion on that page failed outright with
// "missing xref table entry" — blocking deletion entirely until repaired.
//
// The repair pass only runs on this fallback path, not proactively on
// every call: it's a full linear scan of the document's own object graph,
// real cost on a large PDF, and most saves never hit this at all.
func removeAnnotationsRepairingIfNeeded(ctx *model.Context, objNrs []int) error {
	_, err := pdfcpu.RemoveAnnotations(ctx, nil, nil, objNrs, false)
	if err == nil {
		return nil
	}
	if _, repairErr := repairDanglingIndirectReferences(ctx); repairErr != nil {
		return fmt.Errorf("%w (repair attempt also failed: %v)", err, repairErr)
	}
	if _, err := pdfcpu.RemoveAnnotations(ctx, nil, nil, objNrs, false); err != nil {
		return fmt.Errorf("%w (still failing after repair)", err)
	}
	return nil
}

// repairDanglingIndirectReferences scans every object already in ctx's
// own xref table and follows every indirect reference reachable from it —
// including inside opaque, app-private data this app has no reason to
// understand — inserting an empty placeholder dict for any reference that
// doesn't resolve to anything. It doesn't need to know what the missing
// object WAS; an empty dict is enough to satisfy "does this reference
// resolve to something," which is all pdfcpu's own removal validator
// actually checks (see removeAnnotationsRepairingIfNeeded's doc comment).
func repairDanglingIndirectReferences(ctx *model.Context) (repaired int, err error) {
	visited := map[int]bool{}

	var walkObject func(types.Object) error
	var walkRef func(types.IndirectRef) error

	walkRef = func(ref types.IndirectRef) error {
		objNr := ref.ObjectNumber.Value()
		if visited[objNr] {
			return nil
		}
		visited[objNr] = true
		if _, found := ctx.XRefTable.FindTableEntryLight(objNr); !found {
			if _, err := ctx.XRefTable.IndRefForObject(objNr, types.Dict{}); err != nil {
				return err
			}
			repaired++
			return nil
		}
		resolved, err := ctx.XRefTable.Dereference(ref)
		if err != nil || resolved == nil {
			return nil // already broken some other way -- not this pass's job
		}
		return walkObject(resolved)
	}

	walkObject = func(o types.Object) error {
		switch v := o.(type) {
		case types.IndirectRef:
			return walkRef(v)
		case types.Dict:
			for _, val := range v {
				if err := walkObject(val); err != nil {
					return err
				}
			}
		case types.StreamDict:
			for _, val := range v.Dict {
				if err := walkObject(val); err != nil {
					return err
				}
			}
		case types.Array:
			for _, val := range v {
				if err := walkObject(val); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for objNr := range ctx.XRefTable.Table {
		if err := walkRef(*types.NewIndirectRef(objNr, 0)); err != nil {
			return repaired, err
		}
	}

	return repaired, nil
}

// SetHighlightCaption updates h's in-memory caption (its /Contents comment)
// immediately — mirrors BookmarkManager.RenameBookmark. Unlike a bookmark
// title, blank is accepted: it clears the caption. Nothing reaches disk
// until SaveHighlights runs. Deliberately does NOT rebuild d.doc the way
// SetHighlightColor does: a caption isn't painted on the page at all (only
// listed in the Highlights panel), so there's nothing a rebuild could make
// visible sooner — it would just be a real read+write+reopen round trip
// paid for zero visual benefit.
func (d *Document) SetHighlightCaption(h *Highlight, caption string) {
	h.Contents = caption
}

// SetHighlightColor updates h's in-memory color (rgb, 0..1 per channel)
// immediately and clears the page cache so the new color paints on the
// very next RenderPage call — mirrors AddHighlight/DeleteHighlight. Nothing
// reaches disk until SaveHighlights runs, which rewrites every existing
// highlight's /C from this same field (see SaveHighlights). For an
// already-saved highlight (ObjNr > 0), RenderPage's base image comes from
// MuPDF's own real annotation rendering (see CLAUDE.md) — rebuilding d.doc
// (rebuildDoc) is what makes buildNormalizedDoc delete this highlight from
// the scratch copy it renders from, so paintHighlights can hand-paint its
// new color instead of MuPDF faithfully repainting its old, still-on-disk
// one. A bare /C update alone would NOT be enough here — confirmed
// empirically on a real annotation with its own baked /AP appearance
// stream, MuPDF's real rendering paints from that stream regardless of
// /C — see buildNormalizedDoc's own doc comment. Best-effort: a rebuild
// failure leaves the previous d.doc in place, same as
// DeleteHighlight/SaveHighlights.
func (d *Document) SetHighlightColor(h *Highlight, rgb [3]float64) {
	h.Color = rgb
	if h.ObjNr > 0 {
		_ = d.rebuildDoc()
	}
	d.cache.Clear()
}

// rgbToSimpleColor converts our internal 0..1 RGB representation
// (Highlight.Color's convention) into pdfcpu's own color type, used both
// for a brand-new highlight's initial /C (via model.NewHighlightAnnotation)
// and for rewriting an existing one's /C in SaveHighlights.
func rgbToSimpleColor(rgb [3]float64) color.SimpleColor {
	return color.SimpleColor{R: float32(rgb[0]), G: float32(rgb[1]), B: float32(rgb[2])}
}

// defaultShapeBorderWidthPt is the /BS /W (border width) a freshly-drawn
// Square/Circle/Line shape is authored with — the same PDF spec default
// this app already assumes when READING a Line with no /BS at all (see
// defaultLineWidthPt); used here for WRITING one, since AddRectShape/
// AddLineShape don't currently expose a way to choose a different width.
const defaultShapeBorderWidthPt = 1.0

// lineRectPadPt pads a freshly-drawn Line's own /Rect beyond its raw /L
// endpoints — a real annotation's /Rect is always somewhat larger than
// its /L span, to leave room for the border stroke and any arrowhead
// (confirmed by inspecting real Preview-authored lines earlier this
// project — see CLAUDE.md's own note on a stale /L needing repair against
// /Rect). A plain fixed pad is enough for a freshly-authored shape (no
// stale-data problem to work around here, unlike that repair case).
const lineRectPadPt = 10.0

// newAnnotationForShape builds the pdfcpu annotation to write for a
// not-yet-saved (ObjNr == 0) Highlight, based on its Kind: Highlight
// (AddHighlight's own quad-based geometry), Square/Circle (AddRectShape's
// own Rect-based geometry), or Line (AddLineShape's own two-endpoint
// geometry) — the three kinds this app can currently author (see
// ReleaseNotes' Future ideas for what's still authoring-only-via-other-
// apps). Returns nil for anything with no geometry to write, or any other
// Kind (Underline/Strikeout/Squiggly/Note/every genericRectKinds member
// besides Square/Circle) — this app doesn't create those itself, so
// SaveHighlights' own caller just skips a nil rather than treating it as
// an error.
func newAnnotationForShape(h *Highlight) model.AnnotationRenderer {
	col := rgbToSimpleColor(h.Color)
	switch h.Kind {
	case "Highlight":
		if len(h.Quads) == 0 {
			return nil
		}
		minX, minY, maxX, maxY := quadBoundsPt(h.Quads[0])
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
		return model.NewHighlightAnnotation(*rect, 0, h.Contents, "", "", 0, &col, 0, 0, 0, "", nil, nil, "", "", quad)

	case "Square", "Circle":
		if h.Rect == [4]float64{} {
			return nil
		}
		rect := types.NewRectangle(h.Rect[0], h.Rect[1], h.Rect[2], h.Rect[3])
		if h.Kind == "Square" {
			return model.NewSquareAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
				nil, 0, 0, 0, 0, defaultShapeBorderWidthPt, model.BSSolid, false, 0)
		}
		return model.NewCircleAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			nil, 0, 0, 0, 0, defaultShapeBorderWidthPt, model.BSSolid, false, 0)

	case "Line":
		if len(h.Line) != 4 {
			return nil
		}
		minX, maxX := min(h.Line[0], h.Line[2])-lineRectPadPt, max(h.Line[0], h.Line[2])+lineRectPadPt
		minY, maxY := min(h.Line[1], h.Line[3])-lineRectPadPt, max(h.Line[1], h.Line[3])+lineRectPadPt
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		p1 := types.Point{X: h.Line[0], Y: h.Line[1]}
		p2 := types.Point{X: h.Line[2], Y: h.Line[3]}
		beginStyle := lineEndingStyleFromName(h.LineEndStyle[0])
		endStyle := lineEndingStyleFromName(h.LineEndStyle[1])
		return model.NewLineAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			p1, p2, beginStyle, endStyle, 0, 0, 0, nil, nil, false, false, 0, 0, nil, defaultShapeBorderWidthPt, model.BSSolid)

	case "Polygon":
		if len(h.Vertices) < 3 {
			return nil
		}
		minX, minY, maxX, maxY := verticesBoundsPt(h.Vertices)
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		flat := make([]float64, 0, len(h.Vertices)*2)
		for _, v := range h.Vertices {
			flat = append(flat, v[0], v[1])
		}
		vertices := types.NewNumberArray(flat...)
		return model.NewPolygonAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			vertices, nil, nil, nil, nil, defaultShapeBorderWidthPt, model.BSSolid, false, 0)

	case "FreeText":
		if h.Rect == [4]float64{} {
			return nil
		}
		rect := types.NewRectangle(h.Rect[0], h.Rect[1], h.Rect[2], h.Rect[3])
		var callOutLine types.Array
		var intent *model.FreeTextIntent
		if h.CalloutTip != nil {
			// Anchor at the box's own bottom-left corner, matching
			// drawFreeTextPreview's own hand-paint anchor exactly — no
			// arrowhead/ending style either end (callOutLineEndingStyle
			// nil below), so which end pdfcpu/MuPDF considers "start" vs
			// "end" has no visual effect.
			callOutLine = types.NewNumberArray(h.CalloutTip[0], h.CalloutTip[1], h.Rect[0], h.Rect[1])
			it := model.IntentFreeTextCallout
			intent = &it
		}
		// borderWidth is always defaultShapeBorderWidthPt, even for a
		// plain text block — NOT because a plain text block should have
		// a visible border (Preview's own default "Text" tool has none),
		// but because setting it to 0 (omitting /BS entirely) turned out
		// to have NO effect on MuPDF's own rendering: confirmed
		// empirically that MuPDF draws a border around a FreeText with
		// no baked /AP unconditionally, regardless of /BS's presence or
		// width. Preview achieves its own borderless look by baking a
		// custom appearance stream with no border path at all — a much
		// bigger authoring effort (real content-stream + font-resource
		// authoring) than a border-width parameter, and not attempted
		// here. See drawFreeTextPreview's own doc comment for why the
		// in-app preview also deliberately keeps the border, rather than
		// looking different from what Save will actually produce.
		return model.NewFreeTextAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			"", types.AlignLeft, "", 0, &col, "", intent, callOutLine, nil, 0, 0, 0, 0, defaultShapeBorderWidthPt, model.BSSolid, false, 0)

	default:
		return nil
	}
}

// lineEndingStyleFromName maps the PDF /LE name strings this app already
// uses everywhere else (Highlight.LineEndStyle, drawArrowHead's own
// switch) to pdfcpu's model.LineEndingStyle enum — needed only when
// authoring a brand-new Line annotation (newAnnotationForShape above);
// reading one back never needs this, since lineGeometry keeps the raw
// /LE name string as-is. Falls back to LENone for anything unrecognized,
// matching how this app already treats an absent/unrecognized /LE
// everywhere else.
func lineEndingStyleFromName(name string) *model.LineEndingStyle {
	les := model.LENone
	switch name {
	case "Square":
		les = model.LESquare
	case "Circle":
		les = model.LECircle
	case "Diamond":
		les = model.LEDiamond
	case "OpenArrow":
		les = model.LEOpenArrow
	case "ClosedArrow":
		les = model.LEClosedArrow
	case "Butt":
		les = model.LEButt
	case "ROpenArrow":
		les = model.LEROpenArrow
	case "RClosedArrow":
		les = model.LERClosedArrow
	case "Slash":
		les = model.LESlash
	}
	return &les
}
