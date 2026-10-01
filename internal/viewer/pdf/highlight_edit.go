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

// AddMarkupShape creates a new text-markup annotation in memory — Underline,
// Strikeout, or Squiggly (kind must be one of paintableMarkupKinds minus
// "Highlight" itself, which keeps its own AddHighlight for its caption
// parameter) — covering quads (PDF user-space points, same [8]float64-per-
// quad convention as AddHighlight) on page, with rgb as its color. No
// caption parameter, matching every other Draw-tool shape added since
// Highlight itself (Square, Circle, Line, Polygon) — captioning any of
// them is done afterward via the Highlights panel's Edit Caption, not at
// draw time. paintHighlights' existing needsHandPaint-gated Underline/
// Strikeout/Squiggly branches (added in 0.5.0 for reading one of these
// back from another app's PDF) already paint whatever this produces with
// no changes of their own — they key off Kind and ObjNr==0, not how the
// Highlight was constructed. Nothing reaches disk until SaveHighlights
// runs; the returned Highlight has ObjNr == 0 until then, same as every
// other Add* method.
func (d *Document) AddMarkupShape(page int, kind string, quads [][8]float64, rgb [3]float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: kind, Quads: quads, Color: rgb})
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
// its stroke color and lineWidthPt (PDF points) as its stroke width — see
// Highlight.LineWidth. Nothing reaches disk until SaveHighlights runs; the
// returned Highlight has ObjNr == 0 until then, same as AddHighlight.
// kind must be "Square" or "Circle" — see genericRectKinds — anything
// else is a caller bug, not a user-facing error, so this doesn't validate
// it.
func (d *Document) AddRectShape(page int, kind string, rect [4]float64, rgb [3]float64, lineWidthPt float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: kind, Rect: rect, Color: rgb, LineWidth: lineWidthPt})
}

// AddLineShape creates a new Line-kind annotation (a straight line or
// arrow) in memory, from line's two endpoints ([x1, y1, x2, y2], PDF
// user-space points — same convention as Highlight.Line) with endStyle
// ([start, end] — see Highlight.LineEndStyle), rgb as its color, and
// lineWidthPt (PDF points) as its stroke width — see Highlight.LineWidth.
// Nothing reaches disk until SaveHighlights runs; the returned Highlight
// has ObjNr == 0 until then, same as AddHighlight.
func (d *Document) AddLineShape(page int, line []float64, endStyle [2]string, rgb [3]float64, lineWidthPt float64) *Highlight {
	return d.addShape(&Highlight{Page: page, Kind: "Line", Line: line, LineEndStyle: endStyle, Color: rgb, LineWidth: lineWidthPt})
}

// AddPolygonShape creates a new Polygon-kind annotation in memory — a
// Star or Hexagon (see view_render.go's starVertices/hexagonVertices;
// PDF itself has no dedicated subtype for either, so both are authored
// as a generic Polygon, same as any other closed-shape annotation) — from
// vertices (PDF user-space points, one [2]float64 per vertex, in drawing
// order; the shape closes back to the first), with rgb as its stroke
// color and lineWidthPt (PDF points) as its stroke width — see
// Highlight.LineWidth. Rect is derived from vertices' own bounding box,
// the same click-select/outline geometry every genericRectKinds member
// carries. Nothing reaches disk until SaveHighlights runs; the returned
// Highlight has ObjNr == 0 until then, same as AddHighlight/AddRectShape/
// AddLineShape.
func (d *Document) AddPolygonShape(page int, vertices [][2]float64, rgb [3]float64, lineWidthPt float64) *Highlight {
	minX, minY, maxX, maxY := verticesBoundsPt(vertices)
	return d.addShape(&Highlight{Page: page, Kind: "Polygon", Vertices: vertices, Rect: [4]float64{minX, minY, maxX, maxY}, Color: rgb, LineWidth: lineWidthPt})
}

// AddInkShape creates a new Ink-kind annotation in memory — a single
// freehand stroke (PDF's own /InkList supports several per annotation,
// but one drag gesture naturally produces exactly one, the same "one drag,
// one annotation" convention every other Draw-tool shape already follows)
// — from points (PDF user-space points, one [2]float64 per sampled point
// along the drag, in order; an OPEN path, unlike Polygon's closed one —
// see drawPolylineOutline), with rgb as its stroke color and lineWidthPt
// (PDF points) as its stroke width — see Highlight.LineWidth. Rect is
// derived from points' own bounding box, the same click-select/outline
// geometry every genericRectKinds member carries, reusing
// verticesBoundsPt (it only computes a bounding box, which doesn't care
// whether the path closes). Nothing reaches disk until SaveHighlights
// runs; the returned Highlight has ObjNr == 0 until then, same as every
// other Add* method.
func (d *Document) AddInkShape(page int, points [][2]float64, rgb [3]float64, lineWidthPt float64) *Highlight {
	minX, minY, maxX, maxY := verticesBoundsPt(points)
	return d.addShape(&Highlight{Page: page, Kind: "Ink", Vertices: points, Rect: [4]float64{minX, minY, maxX, maxY}, Color: rgb, LineWidth: lineWidthPt})
}

// AddPolyLineShape creates a new PolyLine-kind annotation in memory — the
// exact same freehand-drag capture as AddInkShape (points, one [2]float64
// per sampled point along the drag, in order; an open path), just saved
// under PDF's own distinct /PolyLine subtype instead of /Ink — see
// newAnnotationForShape's own "PolyLine" case doc comment for why this
// isn't (yet) a real click-vertex-then-finish tool. lineWidthPt (PDF
// points) is its stroke width — see Highlight.LineWidth. Nothing reaches
// disk until SaveHighlights runs; the returned Highlight has ObjNr == 0
// until then, same as every other Add* method.
func (d *Document) AddPolyLineShape(page int, points [][2]float64, rgb [3]float64, lineWidthPt float64) *Highlight {
	minX, minY, maxX, maxY := verticesBoundsPt(points)
	return d.addShape(&Highlight{Page: page, Kind: "PolyLine", Vertices: points, Rect: [4]float64{minX, minY, maxX, maxY}, Color: rgb, LineWidth: lineWidthPt})
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

// highlightDictChanges reports whether h's Contents, Color, LineWidth, or
// geometry have diverged from how h was loaded. Color only counts for a
// hasPaintedColor kind, LineWidth only for a hasLineWidth kind — see
// those gates' own doc comments on why each field is meaningless, at its
// Go zero value, for any other kind. geometryChanged is h.geometryMoved
// directly (see its own doc comment on why that's a flag rather than a
// snapshot comparison like the other three). SaveHighlights and
// buildNormalizedDoc use this to decide whether an existing highlight's
// annotation object needs touching at all — see SaveHighlights' own doc
// comment on why "at all" matters, not just "which fields."
func highlightDictChanges(h *Highlight) (contentsChanged, colorChanged, lineWidthChanged, geometryChanged bool) {
	return h.Contents != h.origContents,
		hasPaintedColor(h.Kind) && h.Color != h.origColor,
		hasLineWidth(h.Kind) && h.LineWidth != h.origLineWidth,
		h.geometryMoved
}

// applyPendingHighlightEdits rewrites each already-saved highlight's own
// /Contents directly on ctx's own xref table, for exactly the highlights
// highlightDictChanges reports as having an actually-diverged caption —
// never touching one that hasn't changed at all (see highlightDictChanges'
// and SaveHighlights' own doc comments on why "at all" matters, not just
// "which fields"). Shared between SaveHighlights (writing the real file)
// and buildNormalizedDoc (writing a scratch render copy, which passes a nil
// recreate — see below — since it never re-authors anything, only deletes).
//
// A color or line-width change is handled two different ways depending on
// whether recreate (keyed by the same *Highlight pointers as highlights)
// has an entry for a given highlight. A highlight IN recreate was already
// deleted by the caller (SaveHighlights' own removeObjNrs) and a
// replacement is about to be added separately, so DereferenceDict on its
// old ObjNr finds nothing and is skipped harmlessly here — patching /C or
// /BS on an object that no longer exists would be a wasted no-op. A
// colorChanged or lineWidthChanged highlight NOT in recreate (nil map, as
// buildNormalizedDoc always passes; or one newAnnotationForShape couldn't
// re-author, e.g. a foreign Polygon with no Vertices — see SaveHighlights'
// own doc comment) instead falls back to patching /C or /BS /W in place
// directly, same as this function always did for color before the
// delete+recreate scheme existed for anything recreate CAN handle.
// Confirmed this fallback is cosmetically a no-op against a real baked /AP
// appearance stream (every real renderer, MuPDF included, prefers it over
// /C or the border width it implies) — but it's non-destructive, which is
// what matters for a kind this app has no way to properly re-author at
// all.
//
// A geometryChanged highlight (MoveHighlight was called on it — see
// Highlight.geometryMoved) has no equivalent in-place fallback at all:
// there's no `/Rect`-only patch that would actually move what's painted
// (the whole reason Move needs this same delete+recreate machinery in the
// first place — see ReleaseNotes' own "move an existing shape" note). In
// practice this never matters: canMoveHighlight only ever lets a move
// start on a highlight newAnnotationForShape can already rebuild, so a
// geometryChanged highlight is always in recreate by the time this runs.
func applyPendingHighlightEdits(ctx *model.Context, highlights []*Highlight, recreate map[*Highlight]model.AnnotationRenderer) error {
	for _, h := range highlights {
		if h.ObjNr <= 0 {
			continue
		}
		if _, ok := recreate[h]; ok {
			continue
		}
		contentsChanged, colorChanged, lineWidthChanged, _ := highlightDictChanges(h)
		if !contentsChanged && !colorChanged && !lineWidthChanged {
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
			dict.Update("C", rgbToSimpleColor(h.Color).Array())
		}
		if lineWidthChanged {
			if bs := dict.DictEntry("BS"); bs != nil {
				bs.Update("W", types.Float(h.LineWidth))
			} else {
				dict.Update("BS", types.Dict{"Type": types.Name("Border"), "W": types.Float(h.LineWidth), "S": types.Name("S")})
			}
		}
	}
	return nil
}

// highlightHasBakedAppearance reports whether the real, on-disk annotation
// at path with object number objNr carries a baked /AP appearance stream
// — the one thing that makes applyPendingHighlightEdits' own in-place /C
// or /BS fallback (used for a colorChanged/lineWidthChanged highlight
// newAnnotationForShape can't rebuild, e.g. a Vertices-less Polygon/Ink/
// PolyLine) a cosmetic no-op rather than something MuPDF's own default-
// appearance synthesis already picks up immediately. This app never bakes
// an /AP for anything it authors itself (confirmed empirically — see
// CLAUDE.md), so a Polygon/Ink/PolyLine this app originally drew, even
// reloaded in a later session with no Vertices in memory, has none —
// meaning the in-place patch DOES take effect right away for that case,
// and highlightsPanel's own notifyIfRenderIsDeferred uses this to avoid
// showing a "you'll need to save first" notice that would actively
// contradict what the page is already, correctly, showing.
//
// Does its own standalone, lightweight read of path (not the live ctx
// SaveHighlights/buildNormalizedDoc already have open, which isn't
// reachable from the UI layer this is called from) — acceptable since
// this only ever runs once, right after a color/line-weight change on
// exactly this narrow edge case, not on every render.
func highlightHasBakedAppearance(path string, objNr int) bool {
	if objNr <= 0 {
		return false
	}
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		return false
	}
	dict, err := ctx.XRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || dict == nil {
		return false
	}
	_, found := dict.Find("AP")
	return found
}

// SaveHighlights writes every pending highlight change — captions edited via
// SetHighlightCaption, colors changed via SetHighlightColor, new ones added
// via AddHighlight, and removals via DeleteHighlight — into a fresh read of
// d.path, saving the result to outputPath. Existing highlights' own
// geometry and every other annotation kind are left untouched, and so is
// an existing highlight's own /Contents when it hasn't actually diverged
// from what was loaded (see origContents on Highlight) — deliberately, not
// just as an optimization: rewriting an untouched annotation's dict forces
// pdfcpu to fully re-emit it rather than copy its bytes through unchanged,
// which is enough to make Preview stop trusting a Preview-authored
// annotation's own private editing metadata (see CLAUDE.md's note on this,
// found the hard way on a real file).
//
// A COLOR change, or a geometry change from the Move gesture
// (MoveHighlight/Highlight.geometryMoved), on an already-saved highlight is
// NOT applied by patching /C or /Rect/Quads/etc. in place, even though
// that's tempting for color and was tried first. A real Preview/Acrobat-
// authored annotation already carries its own baked /AP appearance stream,
// and every real PDF renderer (MuPDF included) always paints from that
// stream when present, completely ignoring /C or any geometry entry — so a
// bare in-place field rewrite is invisible, not just in this app's own live
// render (fixed separately via buildNormalizedDoc's hand-paint trick) but
// in the ACTUAL SAVED FILE too: confirmed for color with a real baked-/AP
// test annotation that patching /C alone leaves a fresh reopen showing the
// OLD color forever, silently reverting whatever the live preview showed
// right up until that point — geometry has the exact same problem, which
// is why "move an existing shape" was never just a /Rect/QuadPoints patch
// (see ReleaseNotes' own note on this from before Move existed). Instead,
// exactly like buildNormalizedDoc already does for its own scratch render
// copy, a colorChanged or geometryChanged highlight's old annotation object
// is deleted outright and a brand-new one is authored in its place via
// newAnnotationForShape, in its current (possibly moved) geometry and
// current color — the same construction already used for a genuinely new
// (ObjNr == 0) shape, which never bakes an /AP at all and so always renders
// from its own geometry/color going forward, for this app and any other
// real viewer alike. This does mean a recolored or moved annotation loses
// whatever other-app-private data it carried (e.g. Preview's own
// AAPL:AKExtras) and gets a new ObjNr — an accepted cost of actually
// changing what's rendered, not a side effect anyone would expect to
// survive a deliberate recolor or move anyway.
//
// pdfcpu's public API has no "update an existing annotation" call, only
// Add/Remove (see ReleaseNotes' Future ideas), so a caption-only edit
// bypasses it the same way highlightGeometry bypasses pdfcpu's read-back
// gap for Quads/Color: re-dereference each existing highlight's raw dict by
// its own ObjNr and mutate /Contents directly, via Dict.Update (not
// InsertString/Insert, which silently no-ops when the key already exists —
// a real highlight almost always already carries a, possibly empty,
// /Contents entry from whatever app created it). Removals, color/geometry-
// driven replacements, and genuinely new shapes all go through pdfcpu's
// real Remove/Add API (pdfcpu.RemoveAnnotations, pdfcpu.AddAnnotationToPage)
// against the very same *model.Context, so every kind of change lands in
// one write.
//
// Rereads d.path fresh rather than reusing any earlier-opened *model.Context
// (LoadHighlights' own ctx is not kept around): object numbers are only
// meaningful against the specific file revision they were read from, and
// this keeps that revision's window as short as possible.
//
// On success, if outputPath is d.path itself (an overwrite, not a Save-As
// copy), reloads d.Highlights from the file just written and clears
// pendingHighlightDeletes — every ObjNr==0 highlight (including a
// colorChanged/geometryChanged one, just deleted and re-added above) just
// became a real annotation with its own fresh ObjNr (already captured
// directly from pdfcpu.AddAnnotationToPage's own return value, in the add
// loop above — not re-derived from this reload), and every pending delete
// was just applied, so both must be re-derived from the new on-disk state
// or the next save would re-add or re-delete them.
//
// Before replacing d.Highlights outright, carries Vertices/CalloutTip
// forward from the OLD (pre-reload) highlight sharing the same ObjNr onto
// its freshly-reloaded replacement — matched by ObjNr, which is why the
// add loop above setting it accurately, immediately, matters so much.
// Found necessary via real hands-on testing, not anticipated: pdfcpu's
// own read path never populates Vertices/CalloutTip back from an existing
// PDF (see those fields' own doc comments) — so WITHOUT this, this exact
// reload would silently wipe them for every Polygon/Ink/PolyLine/speech-
// bubble shape this app itself just drew, the instant ANY save ran, even
// within the same session. The user-visible symptom that surfaced this: a
// freshly-drawn Ink stroke, saved once, then recolored in the same
// session — paintHighlights' own "no Vertices, fall back to a plain Rect
// outline" degradation (there for a genuinely foreign Polygon/Ink with no
// Vertices at all — see newAnnotationForShape's own doc comment) fired
// for a shape THIS APP drew and still had the real geometry for in
// memory, turning a freehand stroke into a rectangle right after its
// first save.
//
// A Save-As copy leaves d.path (and so this bookkeeping) untouched,
// matching bookmarkPanel's save behavior: it's an export of the current
// state, not a commit to it, and repeating it against unchanged in-memory
// state must not duplicate anything either.
func (d *Document) SaveHighlights(outputPath string) error {
	ctx, err := api.ReadContextFile(d.path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", d.path, err)
	}

	// Precompute the replacement annotation for every colorChanged,
	// lineWidthChanged, or geometryChanged (moved), already-saved
	// highlight BEFORE deleting anything — newAnnotationForShape
	// returns nil for a kind/geometry combo it can't author (e.g. a Polygon
	// loaded from another app: Vertices is only ever populated for a
	// freshly-drawn one, see Highlight.Vertices' own doc comment), and a
	// highlight in that bucket must NOT be deleted at all, or it would
	// vanish from the file entirely with nothing added back in its place —
	// worse than the stale-color bug this replacement scheme fixes.
	// Confirmed this was a real, not hypothetical, risk: an earlier version
	// of this fix deleted first and only then tried to build a replacement,
	// and a throwaway test (deleted after use) with a foreign, Vertices-less
	// Polygon showed it really did disappear after a recolor + save.
	recreate := map[*Highlight]model.AnnotationRenderer{}
	removeObjNrs := append([]int{}, d.pendingHighlightDeletes...)
	for _, h := range d.Highlights {
		if h.ObjNr <= 0 {
			continue
		}
		_, colorChanged, lineWidthChanged, geometryChanged := highlightDictChanges(h)
		if !colorChanged && !lineWidthChanged && !geometryChanged {
			continue
		}
		if ann := newAnnotationForShape(h); ann != nil {
			recreate[h] = ann
			removeObjNrs = append(removeObjNrs, h.ObjNr)
		}
		// else: can't re-author this one — leave its old object in place
		// entirely (not even queued for removal); applyPendingHighlightEdits
		// below falls back to patching its /C in place for it, same as
		// before this whole replacement scheme existed. Cosmetically a
		// no-op against a baked /AP (see this function's own doc comment),
		// but harmless and non-destructive, which is what matters here.
	}
	if len(removeObjNrs) > 0 {
		if err := removeAnnotationsRepairingIfNeeded(ctx, removeObjNrs); err != nil {
			return fmt.Errorf("removing highlights: %w", err)
		}
	}

	// Patches /Contents for any caption edit, and falls back to patching
	// /C in place for a colorChanged highlight NOT in recreate (couldn't be
	// re-authored, see above) — every highlight that IS in recreate was
	// just deleted above, so DereferenceDict on its ObjNr finds nothing and
	// this function's own nil-dict check skips it harmlessly.
	if err := applyPendingHighlightEdits(ctx, d.Highlights, recreate); err != nil {
		return err
	}

	// Adds via pdfcpu.AddAnnotationToPage, one highlight at a time, rather
	// than batching through pdfcpu.AddAnnotationsMap — deliberately, not
	// just a style choice: AddAnnotationToPage hands back the new
	// annotation's own *types.IndirectRef directly, which is what lets
	// h.ObjNr be set HERE, immediately, instead of only being discoverable
	// after the reload below. See that reload's own doc comment for why
	// this matters: without an accurate ObjNr set here, there would be no
	// way to know which freshly-reloaded Highlight corresponds to which
	// in-memory one, and Vertices/CalloutTip (never recoverable by reading
	// the file back — see those fields' own doc comments) would be lost
	// for every shape this save just added or recreated, not just ones
	// loaded from another app.
	for _, h := range d.Highlights {
		var ann model.AnnotationRenderer
		switch {
		case recreate[h] != nil:
			ann = recreate[h]
		case h.ObjNr == 0:
			ann = newAnnotationForShape(h)
		default:
			continue // already on disk, unchanged (or couldn't be re-authored, see recreate above)
		}
		if ann == nil {
			continue // no geometry to write -- shouldn't happen for anything created via this app's own Add* methods
		}
		ref, _, err := pdfcpu.AddAnnotationToPage(ctx, h.Page, ann, false)
		if err != nil {
			return fmt.Errorf("adding highlight p.%d %s: %w", h.Page, h.Kind, err)
		}
		h.ObjNr = ref.ObjectNumber.Value()
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
		byObjNr := make(map[int]*Highlight, len(d.Highlights))
		for _, h := range d.Highlights {
			if h.ObjNr > 0 {
				byObjNr[h.ObjNr] = h
			}
		}

		reloaded, err := LoadHighlights(d.path)
		if err != nil {
			return fmt.Errorf("reloading %s after save: %w", d.path, err)
		}
		for _, h := range reloaded {
			if old, ok := byObjNr[h.ObjNr]; ok {
				h.Vertices = old.Vertices
				h.CalloutTip = old.CalloutTip
			}
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

// SetHighlightLineWidth updates h's in-memory stroke/border width (PDF
// points — see Highlight.LineWidth) immediately and clears the page cache
// so the new width paints on the very next RenderPage call — mirrors
// SetHighlightColor exactly, including the same rebuildDoc-for-an-
// already-saved-highlight reasoning: a bare /BS /W update alone would NOT
// be enough for one with its own baked /AP appearance stream, the same
// "MuPDF's real rendering paints from the baked stream regardless"
// finding SetHighlightColor's own doc comment explains for /C. Callers
// are expected to have already checked hasLineWidth(h.Kind) — this
// doesn't re-check it, matching SetHighlightColor's own lack of a
// hasPaintedColor guard (the UI decides whether the action is offered at
// all).
func (d *Document) SetHighlightLineWidth(h *Highlight, widthPt float64) {
	h.LineWidth = widthPt
	if h.ObjNr > 0 {
		_ = d.rebuildDoc()
	}
	d.cache.Clear()
}

// canMoveHighlight reports whether h's geometry can actually be
// repositioned via MoveHighlight and (if already saved) persisted on the
// next Save to PDF. A highlight not yet saved (ObjNr == 0) is always
// movable — its geometry lives only in memory, drawn by this app itself,
// so every field newAnnotationForShape might need is already populated.
// An already-saved one needs newAnnotationForShape to actually produce a
// replacement annotation for its current kind+geometry — not automatic:
// a Polygon, Ink, or FreeText loaded from another app never carries
// Vertices/CalloutTip (only this app's own freshly-drawn shapes do — see
// those fields' own doc comments), so newAnnotationForShape can't rebuild one,
// and moving it would have nothing to persist on save even though the
// live preview would misleadingly show it in its new position (the same
// "foreign Polygon" finding SaveHighlights' own doc comment describes for
// recoloring — moving inherits the identical risk, so it gets the
// identical guard). Used by the page's own move-drag gesture
// (view_render.go's handleHighlightMoveHitTest) to decide whether a drag
// starting on the selected highlight begins a move at all, rather than
// letting the user drag something that can't actually be repositioned.
func canMoveHighlight(h *Highlight) bool {
	return h.ObjNr == 0 || newAnnotationForShape(h) != nil
}

// translateHighlightGeometry shifts every geometry field h's Kind actually
// populates by (dxPt, dyPt) PDF user-space points — safe to call
// unconditionally regardless of Kind, since a given Highlight only ever
// has the fields its own Kind uses non-zero (see each field's own doc
// comment on Highlight). Covers Quads (every markup kind), Line, Rect
// (every genericRectKinds member, Polygon/FreeText included), Vertices,
// and CalloutTip — the same complete set newAnnotationForShape reads from
// to author a replacement, so a moved highlight's replacement always
// reflects its new position exactly.
func translateHighlightGeometry(h *Highlight, dxPt, dyPt float64) {
	for i := range h.Quads {
		q := &h.Quads[i]
		for j := 0; j < 8; j += 2 {
			q[j] += dxPt
			q[j+1] += dyPt
		}
	}
	if len(h.Line) == 4 {
		h.Line[0] += dxPt
		h.Line[1] += dyPt
		h.Line[2] += dxPt
		h.Line[3] += dyPt
	}
	if h.Rect != [4]float64{} {
		h.Rect[0] += dxPt
		h.Rect[1] += dyPt
		h.Rect[2] += dxPt
		h.Rect[3] += dyPt
	}
	for i := range h.Vertices {
		h.Vertices[i][0] += dxPt
		h.Vertices[i][1] += dyPt
	}
	if h.CalloutTip != nil {
		h.CalloutTip[0] += dxPt
		h.CalloutTip[1] += dyPt
	}
}

// MoveHighlight repositions h by (dxPt, dyPt) PDF user-space points —
// mirrors SetHighlightColor's own shape: updates in-memory geometry
// immediately, clears the page cache so the new position paints on the
// very next RenderPage call, and (for an already-saved highlight)
// rebuilds d.doc so buildNormalizedDoc deletes it from the scratch render
// copy, letting paintHighlights hand-paint it at its new position instead
// of MuPDF faithfully repainting its old, still-on-disk one — the exact
// same reasoning SetHighlightColor's own doc comment explains for color,
// just for position. Callers (view_render.go's handleHighlightMoved) are
// expected to have already checked canMoveHighlight — this doesn't
// re-check it, matching SetHighlightColor's own lack of a hasPaintedColor
// guard (the UI decides whether the action is offered at all).
func (d *Document) MoveHighlight(h *Highlight, dxPt, dyPt float64) {
	translateHighlightGeometry(h, dxPt, dyPt)
	h.geometryMoved = true
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

	case "Underline", "Strikeout", "Squiggly":
		if len(h.Quads) == 0 {
			return nil
		}
		minX, minY, maxX, maxY := quadBoundsPt(h.Quads[0])
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
		switch h.Kind {
		case "Underline":
			return model.NewUnderlineAnnotation(*rect, 0, h.Contents, "", "", 0, &col, 0, 0, 0, "", nil, nil, "", "", quad)
		case "Strikeout":
			return model.NewStrikeOutAnnotation(*rect, 0, h.Contents, "", "", 0, &col, 0, 0, 0, "", nil, nil, "", "", quad)
		default: // "Squiggly"
			return model.NewSquigglyAnnotation(*rect, 0, h.Contents, "", "", 0, &col, 0, 0, 0, "", nil, nil, "", "", quad)
		}

	case "Square", "Circle":
		if h.Rect == [4]float64{} {
			return nil
		}
		rect := types.NewRectangle(h.Rect[0], h.Rect[1], h.Rect[2], h.Rect[3])
		if h.Kind == "Square" {
			return model.NewSquareAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
				nil, 0, 0, 0, 0, h.LineWidth, model.BSSolid, false, 0)
		}
		return model.NewCircleAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			nil, 0, 0, 0, 0, h.LineWidth, model.BSSolid, false, 0)

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
			p1, p2, beginStyle, endStyle, 0, 0, 0, nil, nil, false, false, 0, 0, nil, h.LineWidth, model.BSSolid)

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
			vertices, nil, nil, nil, nil, h.LineWidth, model.BSSolid, false, 0)

	case "Ink":
		if len(h.Vertices) < 2 {
			return nil
		}
		minX, minY, maxX, maxY := verticesBoundsPt(h.Vertices)
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		flat := make([]float64, 0, len(h.Vertices)*2)
		for _, v := range h.Vertices {
			flat = append(flat, v[0], v[1])
		}
		return model.NewInkAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			[]model.InkPath{flat}, h.LineWidth, model.BSSolid)

	case "PolyLine":
		// Authored from the exact same freehand-drag capture as Ink (see
		// AddPolyLineShape/handleFreehandDrawn) — a real deliberate
		// click-vertex-then-finish PolyLine tool is a bigger, separate
		// effort (see ReleaseNotes' Future ideas), so this is a densely-
		// sampled freehand path saved under PDF's own distinct /PolyLine
		// subtype rather than /Ink, not a different drawing gesture.
		if len(h.Vertices) < 2 {
			return nil
		}
		minX, minY, maxX, maxY := verticesBoundsPt(h.Vertices)
		rect := types.NewRectangle(minX, minY, maxX, maxY)
		flat := make([]float64, 0, len(h.Vertices)*2)
		for _, v := range h.Vertices {
			flat = append(flat, v[0], v[1])
		}
		vertices := types.NewNumberArray(flat...)
		return model.NewPolyLineAnnotation(*rect, 0, h.Contents, "", "", 0, &col, "", nil, nil, "", "",
			vertices, nil, nil, nil, nil, h.LineWidth, model.BSSolid, nil, nil)

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
