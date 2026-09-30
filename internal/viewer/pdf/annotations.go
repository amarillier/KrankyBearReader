package pdf

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Highlight is one PDF annotation worth surfacing in the Highlights and
// Notes panel: a highlight/underline/strikeout/squiggly markup, a Line
// (arrow), a text/popup note, or a generic shape (Square, Circle, Polygon,
// PolyLine, Ink, Stamp, FreeText, Caret — see genericRectKinds). Link
// annotations are deliberately excluded — they aren't "highlights or
// notes". A generic shape kind carries only Rect: unlike
// Highlight/Underline/Strikeout/Squiggly's /QuadPoints or Line's /L,
// there's no fixed geometry to read back at all for most of these (Stamp
// especially — an arbitrary PDF content stream, see ReleaseNotes' Future
// ideas), only its overall /Rect, which is enough to list, click-select,
// and delete one, but not enough to hand-paint or recolor it — MuPDF's own
// rendering already paints these correctly from their baked /AP appearance
// stream regardless (see CLAUDE.md's "Real MuPDF annotation rendering"
// section), so this app doesn't need to. This app lists and paints what
// other apps (Preview, Acrobat, ...) already wrote (see paintableMarkupKinds
// and Line/LineEndStyle); it can also caption, add, and delete Highlight-kind
// ones itself — see highlight_edit.go — but doesn't yet author Underline/
// Strikeout/Squiggly/Line/generic-shape geometry, and new Highlight geometry
// comes from a hand-drawn rectangle, not text-snapped selection like
// Preview/Acrobat's (go-fitz has no word/line bounding-box API — see
// ReleaseNotes' Future ideas).
type Highlight struct {
	Page     int
	Kind     string
	Contents string

	// ObjNr is this annotation's PDF indirect object number, captured at
	// load time. SaveHighlights re-dereferences the annotation by this
	// number to write a caption back onto exactly this one, the same way
	// highlightGeometry re-dereferences it to read Quads/Color back. Zero
	// means this Highlight only exists in memory so far — either the rare
	// on-disk annotation with no indirect reference of its own (see
	// highlightGeometry), where captioning is a no-op on save, or (far more
	// commonly) one just added via AddHighlight and not yet saved, which
	// SaveHighlights creates as a new annotation instead of updating one.
	ObjNr int

	// Quads and Color are populated for every kind in paintableMarkupKinds
	// (Highlight, Underline, Strikeout, Squiggly) — every PDF text-markup
	// annotation type that carries /QuadPoints. Not populated for "Note"
	// (Text/Popup) or "Line", which have no quad geometry at all. Quads is
	// one rectangle per QuadPoints entry — usually one per highlighted
	// line/run — in PDF user-space points (origin bottom-left, matching
	// Document.Bound). Empty when the PDF's /QuadPoints couldn't be read
	// (e.g. a malformed or missing entry), in which case the annotation is
	// still listed but not painted on the page. Color is RGB, 0..1 per
	// channel, defaulting to standard highlighter yellow when the PDF
	// doesn't specify one — populated for "Line" too (via lineGeometry),
	// same convention.
	Quads [][8]float64
	Color [3]float64

	// Line and LineEndStyle are populated for Kind == "Line" only (an
	// arrow or plain straight line — see lineGeometry). Line is
	// [x1, y1, x2, y2] in PDF user-space points (the PDF /L entry), nil if
	// it couldn't be read. LineEndStyle is the PDF /LE entry's two ending
	// style names, [start, end] — "None" for both when /LE is absent (the
	// PDF spec's own default), matching how drawLineAnnotation treats an
	// unrecognized style the same as "None" (skip the arrowhead) rather
	// than guessing.
	Line         []float64
	LineEndStyle [2]string

	// Rect is populated for a genericRectKinds Kind only (Square, Circle,
	// Polygon, PolyLine, Ink, Stamp, FreeText, Caret) — its overall
	// [minX, minY, maxX, maxY] bounding box in PDF user-space points,
	// already normalized (the raw /Rect entry isn't guaranteed
	// min-before-max — see parseRectPt). Just enough geometry to
	// click-select (HighlightAt) and outline (paintHighlights) one of
	// these; there's no per-kind geometry parsing to paint or recolor
	// them with, see this struct's own doc comment.
	Rect [4]float64

	// origContents and origColor snapshot Contents/Color as loaded from
	// disk, for an already-saved highlight only (ObjNr > 0) — see
	// SaveHighlights, which now only rewrites /Contents or /C for a
	// highlight whose current value has actually diverged from this
	// snapshot, instead of rewriting every on-disk highlight's dict
	// unconditionally on every save. Found necessary the hard way: a
	// gratuitous rewrite of an untouched annotation's dict forces pdfcpu
	// to fully re-parse-and-re-emit it rather than copying its bytes
	// through unchanged, which is enough to make Preview stop trusting a
	// Preview-authored Line/arrow's own private editing metadata (see
	// CLAUDE.md) — even though nothing about that annotation's own
	// Contents or Color had actually changed.
	origContents string
	origColor    [3]float64
}

// highlightKinds are the annotation types worth listing, and their display
// label. Anything not in this map (Link, Widget, ...) is skipped.
var highlightKinds = map[model.AnnotationType]string{
	model.AnnHighLight: "Highlight",
	model.AnnUnderline: "Underline",
	model.AnnStrikeOut: "Strikeout",
	model.AnnSquiggly:  "Squiggly",
	model.AnnLine:      "Line",
	model.AnnText:      "Note",
	model.AnnPopup:     "Note",
	model.AnnSquare:    "Square",
	model.AnnCircle:    "Circle",
	model.AnnPolygon:   "Polygon",
	model.AnnPolyLine:  "PolyLine",
	model.AnnInk:       "Ink",
	model.AnnStamp:     "Stamp",
	model.AnnFreeText:  "FreeText",
	model.AnnCaret:     "Caret",
}

// genericRectKinds are the highlightKinds labels with no per-kind geometry
// parsing at all — just their overall /Rect (see Highlight.Rect's own doc
// comment for why: real shapes here, Stamp especially, have no fixed
// geometry a Quad/Line-style parser could reconstruct, only an arbitrary
// appearance stream MuPDF already renders correctly). Used by
// flattenHighlights (which kinds to read /Rect for) and paintHighlights/
// HighlightAt (which kinds fall back to a plain Rect-based outline/hit
// test instead of a quad- or line-shaped one).
var genericRectKinds = map[string]bool{
	"Square":   true,
	"Circle":   true,
	"Polygon":  true,
	"PolyLine": true,
	"Ink":      true,
	"Stamp":    true,
	"FreeText": true,
	"Caret":    true,
}

// paintableMarkupKinds are the highlightKinds labels that carry real
// per-annotation /QuadPoints geometry and /C color — every text-markup kind
// PDF defines, as opposed to "Note" (Text/Popup) or "Line" (its own /L,
// handled separately — see hasPaintedColor), which have neither. Shared
// between flattenHighlights (which kinds to call highlightGeometry for) and
// paintHighlights (which kinds draw from Quads, one shape per quad, versus
// Line's own single-shape branch).
var paintableMarkupKinds = map[string]bool{
	"Highlight": true,
	"Underline": true,
	"Strikeout": true,
	"Squiggly":  true,
}

// hasPaintedColor reports whether kind's Color field is real — read back
// from the PDF's own /C — and so safe to write back on save or offer for
// recoloring: every paintableMarkupKinds kind, plus "Line" (lineGeometry
// reads /C too, just alongside /L instead of /QuadPoints). Used by
// SaveHighlights and the Highlights panel's color picker, the same two
// places paintableMarkupKinds alone used to gate before Line existed.
func hasPaintedColor(kind string) bool {
	return paintableMarkupKinds[kind] || kind == "Line"
}

// defaultHighlightColor is standard highlighter yellow, used when a PDF's
// highlight annotation doesn't specify its own /C color (rare in practice,
// but not an error).
var defaultHighlightColor = [3]float64{1, 1, 0}

// LoadHighlights reads every highlight/underline/strikeout/squiggly/note
// annotation in path via pdfcpu, sorted by page. Not an error if the PDF has
// none.
//
// Uses api.ReadContextFile (rather than the simpler api.Annotations, used
// before this got Quads/Color) to keep the resulting *model.XRefTable around:
// pdfcpu's own annotation-reading path (pkg/pdfcpu/annotation.go's
// Annotation()) only special-cases Text/Link/Popup when parsing an existing
// PDF's annotations — every other subtype, Highlight included, becomes a
// bare model.Annotation via NewAnnotationForRawType, which has no Quad field
// at all (TextMarkupAnnotation.Quad is only ever populated by pdfcpu's own
// annotation-authoring constructors, e.g. NewHighlightAnnotation, used when
// writing a new annotation — never by the code path that reads one back).
// So getting a real highlight's /QuadPoints back out requires re-dereferencing
// its raw annotation dict ourselves via the object number pgAnnots already
// carries — see highlightGeometry.
func LoadHighlights(path string) ([]*Highlight, error) {
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		return nil, nil
	}
	pgAnnots := pdfcpu.AnnotationsForSelectedPages(ctx, nil) // nil selection = every page
	return flattenHighlights(ctx.XRefTable, pgAnnots), nil
}

// flattenHighlights turns pdfcpu's map[page]map[type]Annot into a flat,
// page-sorted list, keeping only the types in highlightKinds.
func flattenHighlights(xRefTable *model.XRefTable, pgAnnots map[int]model.PgAnnots) []*Highlight {
	var out []*Highlight
	for page, byType := range pgAnnots {
		for typ, annot := range byType {
			label, ok := highlightKinds[typ]
			if !ok {
				continue
			}
			for objNr, a := range annot.Map {
				h := &Highlight{
					Page:  page,
					Kind:  label,
					ObjNr: objNr,
					// Content(), not ContentString(): MarkupAnnotation
					// overrides ContentString() to wrap the text in literal
					// quotes for CLI/debug display (confirmed by reading
					// pdfcpu's source after a test caught it) — not what we
					// want in a UI list.
					Contents: a.Content(),
				}
				switch {
				case paintableMarkupKinds[label]:
					h.Quads, h.Color = highlightGeometry(xRefTable, objNr)
				case label == "Line":
					h.Line, h.LineEndStyle, h.Color = lineGeometry(xRefTable, objNr)
				case genericRectKinds[label]:
					h.Rect = annotationRectPt(xRefTable, objNr)
				}
				h.origContents, h.origColor = h.Contents, h.Color
				out = append(out, h)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Page < out[j].Page })
	return out
}

// annotationRectPt re-dereferences a genericRectKinds annotation's own raw
// PDF dict to read its /Rect directly, the same re-dereference-by-objNr
// technique highlightGeometry/lineGeometry use for /QuadPoints and /L —
// pdfcpu's own read path doesn't populate a generic annotation's Rect
// field back from an existing PDF either. Returns the zero [4]float64 if
// objNr is invalid, the dict can't be read, or /Rect is malformed — the
// same "list without painting" fallback as the other geometry readers;
// HighlightAt and paintHighlights both treat a zero Rect as "nothing to
// hit-test or outline."
func annotationRectPt(xRefTable *model.XRefTable, objNr int) [4]float64 {
	if objNr <= 0 {
		return [4]float64{}
	}
	d, err := xRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		return [4]float64{}
	}
	rect := parseRectPt(xRefTable, d.ArrayEntry("Rect"))
	if rect == nil {
		return [4]float64{}
	}
	minX, maxX := min(rect[0], rect[2]), max(rect[0], rect[2])
	minY, maxY := min(rect[1], rect[3]), max(rect[1], rect[3])
	return [4]float64{minX, minY, maxX, maxY}
}

// highlightGeometry re-dereferences a highlight annotation's own raw PDF dict
// (by object number — objNr is negative for the rare annotation with no
// indirect reference of its own, which can't be re-dereferenced this way and
// falls back to no quads) to read /QuadPoints and /C directly, bypassing
// pdfcpu's higher-level annotation model, which — see LoadHighlights' doc
// comment — never carries them back from an existing PDF.
func highlightGeometry(xRefTable *model.XRefTable, objNr int) ([][8]float64, [3]float64) {
	if objNr <= 0 {
		return nil, defaultHighlightColor
	}
	d, err := xRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		return nil, defaultHighlightColor
	}
	quads := parseQuadPoints(xRefTable, d.ArrayEntry("QuadPoints"))
	col := parseColor(xRefTable, d.ArrayEntry("C"))
	return quads, col
}

// lineGeometry re-dereferences a Line annotation's own raw PDF dict the
// same way highlightGeometry does for a text-markup one, to read /L (its
// two endpoints), /LE (its two ending styles — arrowhead, if any, at each
// end), and /C directly: pdfcpu's own read path never populates
// LineAnnotation's own fields back from an existing PDF either, the same
// gap highlightGeometry's own doc comment explains for TextMarkupAnnotation.
func lineGeometry(xRefTable *model.XRefTable, objNr int) (points []float64, endStyles [2]string, col [3]float64) {
	endStyles = [2]string{"None", "None"} // the PDF spec's own default when /LE is absent
	if objNr <= 0 {
		return nil, endStyles, defaultHighlightColor
	}
	d, err := xRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || d == nil {
		return nil, endStyles, defaultHighlightColor
	}
	points = parseLinePoints(xRefTable, d.ArrayEntry("L"))
	if points != nil {
		if rect := parseRectPt(xRefTable, d.ArrayEntry("Rect")); rect != nil {
			points = clampLinePointsToRect(points, rect)
		}
	}
	col = parseColor(xRefTable, d.ArrayEntry("C"))
	if names := d.ArrayEntry("LE"); len(names) == 2 {
		for i, n := range names {
			if nm, ok := n.(types.Name); ok {
				endStyles[i] = string(nm)
			}
		}
	}
	return points, endStyles, col
}

// parseLinePoints converts a PDF /L array (exactly 4 numbers: x1, y1, x2,
// y2) into a flat []float64, or nil for anything malformed — same
// "fall back to listing without painting, don't guess" convention as
// parseQuadPoints.
func parseLinePoints(xRefTable *model.XRefTable, arr types.Array) []float64 {
	if len(arr) != 4 {
		return nil
	}
	pts := make([]float64, 4)
	for i, o := range arr {
		v, err := xRefTable.DereferenceNumber(o)
		if err != nil {
			return nil
		}
		pts[i] = v
	}
	return pts
}

// parseRectPt converts a PDF /Rect array (exactly 4 numbers: llx, lly, urx,
// ury — not necessarily already ordered min-before-max, per the PDF spec)
// into a [4]float64 in that same raw order, or nil for anything malformed.
func parseRectPt(xRefTable *model.XRefTable, arr types.Array) *[4]float64 {
	if len(arr) != 4 {
		return nil
	}
	var r [4]float64
	for i, o := range arr {
		v, err := xRefTable.DereferenceNumber(o)
		if err != nil {
			return nil
		}
		r[i] = v
	}
	return &r
}

// clampLinePointsToRect repairs points' two endpoints against rect (a Line
// annotation's own /Rect) when one of them has drifted outside it. Found
// necessary empirically, not speculatively: a real macOS Preview-authored
// arrow (see ReleaseNotes' Version 0.5.0 notes) had an /L endpoint whose Y
// sat ~145pt above its own /Rect's top edge and off the page entirely —
// almost certainly a stale value left over from an earlier edit that
// resized /Rect without updating /L to match.
//
// For each axis independently: if exactly one endpoint's coordinate on
// that axis falls outside rect while the other's doesn't, the out-of-range
// one is replaced with the in-range one's own value (mirroring), not
// clamped to rect's nearest edge. This isn't an arbitrary choice — the
// still-good endpoint of the real arrow above sat almost exactly at rect's
// midpoint on the bad axis (667.08, against a midpoint of 666.92), which
// only makes sense if the annotation's real border/arrowhead thickness
// pads /Rect symmetrically around a *line whose true coordinate on that
// axis is close to the good endpoint's own value* — clamping to rect's far
// edge instead lands ~half that padding away from the truth. Confirmed by
// actually rendering the real arrow both ways and comparing against the
// same PDF opened in Preview: mirroring produces a short, nearly-flat
// arrow that lands right above its target text, matching Preview's own
// rendering's shape and position far better than edge-clamping's longer,
// steeper, higher-up line did.
//
// If neither or both endpoints are out of range on a given axis, this
// can't help — left untouched, or (both out of range, rare) clamped
// independently as a last resort with no better information to go on.
func clampLinePointsToRect(points []float64, rect *[4]float64) []float64 {
	minX, maxX := min(rect[0], rect[2]), max(rect[0], rect[2])
	minY, maxY := min(rect[1], rect[3]), max(rect[1], rect[3])
	out := make([]float64, 4)
	copy(out, points)
	repairAxis(out, 0, 2, minX, maxX)
	repairAxis(out, 1, 3, minY, maxY)
	return out
}

// repairAxis mirrors pts[j] from pts[i] (or vice versa) when exactly one of
// them falls outside [lo, hi] — see clampLinePointsToRect's own doc comment
// for why mirroring, not edge-clamping, is the right repair here.
func repairAxis(pts []float64, i, j int, lo, hi float64) {
	iIn := pts[i] >= lo && pts[i] <= hi
	jIn := pts[j] >= lo && pts[j] <= hi
	switch {
	case iIn == jIn:
		return // both in range (nothing to fix) or both out (no good value to mirror from)
	case jIn:
		pts[i] = pts[j]
	default:
		pts[j] = pts[i]
	}
}

// parseQuadPoints converts a PDF /QuadPoints array (8 numbers per quad: 4
// (x,y) pairs) into one [8]float64 per quad. Returns nil for anything that
// isn't a well-formed, non-empty multiple of 8 numbers, or that contains a
// value DereferenceNumber can't resolve — the caller then falls back to
// listing the highlight without painting it, rather than guessing.
func parseQuadPoints(xRefTable *model.XRefTable, arr types.Array) [][8]float64 {
	if len(arr) == 0 || len(arr)%8 != 0 {
		return nil
	}
	nums := make([]float64, len(arr))
	for i, o := range arr {
		v, err := xRefTable.DereferenceNumber(o)
		if err != nil {
			return nil
		}
		nums[i] = v
	}
	quads := make([][8]float64, len(nums)/8)
	for i := range quads {
		copy(quads[i][:], nums[i*8:i*8+8])
	}
	return quads
}

// parseColor converts a PDF /C color array (DeviceGray, DeviceRGB, or
// DeviceCMYK — 1, 3, or 4 components respectively, per the PDF spec's
// annotation color entry) into RGB, 0..1 per channel. Falls back to
// defaultHighlightColor for anything else (missing, empty, or an
// unresolvable component).
func parseColor(xRefTable *model.XRefTable, arr types.Array) [3]float64 {
	vals := make([]float64, len(arr))
	for i, o := range arr {
		v, err := xRefTable.DereferenceNumber(o)
		if err != nil {
			return defaultHighlightColor
		}
		vals[i] = v
	}
	switch len(vals) {
	case 1: // DeviceGray
		return [3]float64{vals[0], vals[0], vals[0]}
	case 3: // DeviceRGB
		return [3]float64{vals[0], vals[1], vals[2]}
	case 4: // DeviceCMYK
		return cmykToRGB(vals[0], vals[1], vals[2], vals[3])
	default:
		return defaultHighlightColor
	}
}

// cmykToRGB is the standard naive (non-color-managed) CMYK->RGB conversion —
// good enough for tinting a highlight overlay, not intended for print-accurate
// color reproduction.
func cmykToRGB(c, m, y, k float64) [3]float64 {
	return [3]float64{
		(1 - c) * (1 - k),
		(1 - m) * (1 - k),
		(1 - y) * (1 - k),
	}
}

// highlightOverlayAlpha is the painted highlight's opacity (0..255) — a
// translucent tint over the text, not an opaque block, matching how a real
// highlighter (and Preview/Acrobat) render one.
const highlightOverlayAlpha = 90

// markupLineAlpha is the opacity for Underline/Strikeout/Squiggly strokes —
// solid-looking, unlike a Highlight's translucent tint (highlightOverlayAlpha
// above): these render as an actual colored line/mark on the text, matching
// Preview/Acrobat, not a wash over it.
const markupLineAlpha = 230

// underlineFrac/strikeoutFrac/squigglyFrac place each line kind's own mark
// as a fraction of its quad's height above the quad's bottom edge — matching
// where Preview/Acrobat draw them relative to the highlighted text's own
// baseline (Underline, Squiggly) or vertical middle (Strikeout), not the
// quad's literal top or bottom edge.
const (
	underlineFrac = 0.08
	strikeoutFrac = 0.45
	squigglyFrac  = 0.12
)

// markupLineThicknessFrac is each line-based mark's own thickness, as a
// fraction of its quad's height (floored to ~1px in quadPixelRect's callers
// below so it never vanishes at low zoom).
const markupLineThicknessFrac = 0.06

// defaultLineWidthPt is the PDF spec's own default /BS /W (border width)
// for a Line annotation when it's absent — used since this app doesn't
// currently read /BS at all (see lineGeometry's doc comment); good enough
// for a cosmetic overlay stroke, not something worth a full /BS parse for
// yet.
const defaultLineWidthPt = 1.0

// paintHighlights composites every highlight/underline/strikeout/squiggly/
// line annotation on page directly onto img (already rendered at dpi by
// go-fitz), one shape per annotation — a translucent fill for Highlight, a
// solid line/mark for Underline/Strikeout/Squiggly (one per quad), and an
// arrow/line shaft plus optional arrowheads for Line (see each kind's own
// draw helper). Silently does nothing if there are no highlights on this
// page, or if the page's own bounds can't be read — this is cosmetic, not
// something worth failing a page render over.
func (d *Document) paintHighlights(img *image.RGBA, page int, dpi float64) {
	var pageHeightPt float64
	haveBounds := false

	scale := dpi / 72.0
	for _, h := range d.Highlights {
		if h.Page != page {
			continue
		}
		hasGenericRect := genericRectKinds[h.Kind] && h.Rect != [4]float64{}
		if len(h.Quads) == 0 && (h.Kind != "Line" || len(h.Line) != 4) && !hasGenericRect {
			continue // nothing this app knows how to paint has geometry here
		}
		if !haveBounds {
			_, height, err := d.PageBoundsPt(page)
			if err != nil {
				return
			}
			pageHeightPt = height
			haveBounds = true
		}
		fillCol := color.NRGBA{
			R: uint8(h.Color[0] * 255),
			G: uint8(h.Color[1] * 255),
			B: uint8(h.Color[2] * 255),
			A: highlightOverlayAlpha,
		}
		lineCol := fillCol
		lineCol.A = markupLineAlpha

		// needsHandPaint is true whenever MuPDF's own render (d.doc,
		// ImageWithAnnotsDPI) has nothing correct to show for h, so this
		// app must paint it itself:
		//   - ObjNr == 0: freshly drawn via AddHighlight, doesn't exist in
		//     the file at all yet.
		//   - a pending, not-yet-saved COLOR change on an already-saved
		//     highlight: rebuildDoc's own buildNormalizedDoc (document.go)
		//     deletes this exact annotation from the normalized scratch
		//     copy d.doc renders from, specifically so this hand-paint
		//     path can safely draw its NEW color without doubling up a
		//     stale baked appearance underneath it — see that function's
		//     own doc comment for why an in-place /C edit alone can't work
		//     here (a real annotation's baked /AP appearance stream wins
		//     over /C in MuPDF's real rendering, confirmed empirically).
		// Either way, the selection outline below still needs to run —
		// it's a UI affordance over an already-rendered page, not
		// something MuPDF has any notion of.
		_, colorChanged := highlightDictChanges(h)
		needsHandPaint := h.ObjNr == 0 || colorChanged

		if hasGenericRect {
			// No fill/hand-paint here at all — see Highlight.Rect's own
			// doc comment: there's no per-kind geometry to draw with, and
			// MuPDF's own render (see needsHandPaint's doc comment above)
			// already paints this kind correctly from its baked /AP
			// appearance stream. Only a selection outline, same UI
			// affordance as every other kind gets.
			if h == d.selectedHighlight {
				drawRectOutline(img, rectPixelRect(h.Rect, pageHeightPt, scale), selectionOutlineColor, selectionOutlineWidth)
			}
			continue
		}

		if h.Kind == "Line" {
			var r image.Rectangle
			if needsHandPaint {
				r = drawLineAnnotation(img, h.Line, h.LineEndStyle, defaultLineWidthPt, lineCol, pageHeightPt, scale)
			} else {
				r = lineBoundsPixelRect(h.Line, pageHeightPt, scale)
			}
			if h == d.selectedHighlight {
				drawRectOutline(img, r, selectionOutlineColor, selectionOutlineWidth)
			}
			continue
		}

		for _, q := range h.Quads {
			var r image.Rectangle
			switch h.Kind {
			case "Highlight":
				r = quadPixelRect(q, pageHeightPt, scale)
				if needsHandPaint {
					drawTranslucentRect(img, r, fillCol)
				}
			case "Underline":
				r = markupLineRect(q, underlineFrac, pageHeightPt, scale)
				if needsHandPaint {
					drawTranslucentRect(img, r, lineCol)
				}
			case "Strikeout":
				r = markupLineRect(q, strikeoutFrac, pageHeightPt, scale)
				if needsHandPaint {
					drawTranslucentRect(img, r, lineCol)
				}
			case "Squiggly":
				if needsHandPaint {
					r = drawSquigglyLine(img, q, pageHeightPt, scale, lineCol)
				} else {
					r = quadPixelRect(q, pageHeightPt, scale) // approximate bounds only — MuPDF already painted the real wave
				}
			default:
				continue // Note (Text/Popup) has no quads to reach here at all
			}
			if h == d.selectedHighlight {
				drawRectOutline(img, r, selectionOutlineColor, selectionOutlineWidth)
			}
		}
	}
}

// lineBoundsPixelRect approximates a Line annotation's own pixel bounding
// box from just its two endpoints (same PDF-space-to-pixel scale-and-flip-Y
// as quadPixelRect) — used only for an already-saved Line's selection
// outline (see paintHighlights), which doesn't need to hug an arrowhead's
// exact triangular extent the way actually painting one does.
func lineBoundsPixelRect(points []float64, pageHeightPt, scale float64) image.Rectangle {
	if len(points) != 4 {
		return image.Rectangle{}
	}
	minX, maxX := min(points[0], points[2]), max(points[0], points[2])
	minY, maxY := min(points[1], points[3]), max(points[1], points[3])
	r := image.Rect(
		int(minX*scale), int((pageHeightPt-maxY)*scale),
		int(maxX*scale), int((pageHeightPt-minY)*scale),
	)
	return inflateToMinExtent(r, minLineOutlinePx)
}

// minLineOutlinePx is the minimum width AND height (already-scaled image
// pixels) lineBoundsPixelRect's returned rectangle is inflated to. Found
// necessary via real hands-on testing, not speculatively: a perfectly
// vertical (or horizontal) Line annotation's own two endpoints share one
// coordinate exactly, so the raw scaled rect has that axis's width or
// height at literally zero — and image.Rectangle.Empty() reports ANY
// zero-width-or-height rectangle as empty, which drawRectOutline
// (paintHighlights' selection indicator) treats as "nothing to draw" and
// skips entirely. Confirmed with a direct pixel count on the exact real
// annotation that surfaced this: zero pixels painted, not just a
// hard-to-notice thin line — selecting that Line in the Highlights panel
// visibly never outlined it on the page at all.
const minLineOutlinePx = 6

// inflateToMinExtent grows r symmetrically on whichever axis (or both)
// falls short of minPx, so a degenerate zero-width or zero-height
// rectangle — lineBoundsPixelRect's own doc comment explains when that
// happens — never reaches image.Rectangle.Empty()'s "nothing to draw"
// case.
func inflateToMinExtent(r image.Rectangle, minPx int) image.Rectangle {
	if dx := r.Dx(); dx < minPx {
		grow := (minPx - dx + 1) / 2
		r.Min.X -= grow
		r.Max.X += grow
	}
	if dy := r.Dy(); dy < minPx {
		grow := (minPx - dy + 1) / 2
		r.Min.Y -= grow
		r.Max.Y += grow
	}
	return r
}

// rectPixelRect converts a genericRectKinds highlight's already-normalized
// Rect ([minX, minY, maxX, maxY], PDF user-space points) into an
// image-pixel rectangle — same scale-and-flip-Y convention as
// quadPixelRect/lineBoundsPixelRect, just for a rect that's already
// min-before-max instead of two arbitrary endpoints or four quad corners.
func rectPixelRect(rect [4]float64, pageHeightPt, scale float64) image.Rectangle {
	return image.Rect(
		int(rect[0]*scale), int((pageHeightPt-rect[3])*scale),
		int(rect[2]*scale), int((pageHeightPt-rect[1])*scale),
	)
}

// markupLineRect converts one quad into a thin image-pixel rectangle for a
// straight line-based mark (Underline, Strikeout), positioned heightFrac of
// the quad's own height above its bottom edge — the same PDF-space-to-pixel
// scale-and-flip-Y as quadPixelRect, just offset and made line-thin instead
// of spanning the whole quad height.
func markupLineRect(q [8]float64, heightFrac, pageHeightPt, scale float64) image.Rectangle {
	minX, minY, maxX, maxY := quadBoundsPt(q)
	h := maxY - minY
	thickness := h * markupLineThicknessFrac
	if thickness*scale < 1 {
		thickness = 1 / scale
	}
	lineY := minY + h*heightFrac
	top, bottom := lineY+thickness/2, lineY-thickness/2
	return image.Rect(
		int(minX*scale), int((pageHeightPt-top)*scale),
		int(maxX*scale), int((pageHeightPt-bottom)*scale),
	)
}

// drawSquigglyLine paints a wavy stroke across quad's width at squigglyFrac
// of its height, one pixel column at a time (a sine wave in PDF-space,
// converted to pixels per column) since, unlike Underline/Strikeout, a
// squiggly mark isn't representable as a single axis-aligned rectangle.
// Returns the wave's own bounding rectangle, so the caller can still draw a
// selection outline around it the same way it does for the other kinds.
func drawSquigglyLine(img *image.RGBA, q [8]float64, pageHeightPt, scale float64, col color.NRGBA) image.Rectangle {
	minX, minY, maxX, maxY := quadBoundsPt(q)
	h := maxY - minY
	baseY := minY + h*squigglyFrac
	amp := h * 0.05
	if amp*scale < 1 {
		amp = 1 / scale
	}
	wavelength := h * 0.8
	if wavelength*scale < 2 {
		wavelength = 2 / scale
	}
	thicknessPx := int(h * markupLineThicknessFrac * scale)
	if thicknessPx < 1 {
		thicknessPx = 1
	}

	bounds := img.Bounds()
	x0, x1 := int(minX*scale), int(maxX*scale)
	for xPx := x0; xPx < x1; xPx++ {
		xPt := float64(xPx) / scale
		phase := 2 * math.Pi * (xPt - minX) / wavelength
		yPt := baseY + amp*math.Sin(phase)
		yPx := int((pageHeightPt - yPt) * scale)
		seg := image.Rect(xPx, yPx, xPx+1, yPx+thicknessPx)
		draw.Draw(img, seg.Intersect(bounds), image.NewUniform(col), image.Point{}, draw.Over)
	}
	return image.Rect(
		x0, int((pageHeightPt-(baseY+amp))*scale)-thicknessPx,
		x1, int((pageHeightPt-(baseY-amp))*scale)+thicknessPx,
	)
}

// arrowHeadLengthFactor/arrowHeadHalfWidthFactor size an arrowhead relative
// to its line's own stroke thickness (with a floor in drawArrowHead so a
// thin line still gets a visible arrowhead), so a thick hand-drawn arrow
// gets a proportionally bigger head instead of one fixed size for every
// line width.
const (
	arrowHeadLengthFactor    = 3.2
	arrowHeadHalfWidthFactor = 1.6
)

// drawLineAnnotation paints a Line annotation's shaft (points, PDF
// user-space, origin bottom-left — the PDF /L convention) plus an
// arrowhead at either end per endStyles, and returns the whole shape's
// pixel bounding box for the caller's selection outline. Does nothing (and
// returns an empty rect) if points isn't a well-formed pair — the same
// "list without painting" fallback as the quad-based kinds when their own
// geometry can't be read.
func drawLineAnnotation(img *image.RGBA, points []float64, endStyles [2]string, widthPt float64, col color.NRGBA, pageHeightPt, scale float64) image.Rectangle {
	if len(points) != 4 {
		return image.Rectangle{}
	}
	a := [2]float64{points[0] * scale, (pageHeightPt - points[1]) * scale}
	b := [2]float64{points[2] * scale, (pageHeightPt - points[3]) * scale}
	thicknessPx := widthPt * scale
	if thicknessPx < 1 {
		thicknessPx = 1
	}

	r := strokeLine(img, a, b, thicknessPx, col)
	r = r.Union(drawArrowHead(img, a, b, endStyles[0], thicknessPx, col)) // start end: tip=a, shaft points back toward b
	r = r.Union(drawArrowHead(img, b, a, endStyles[1], thicknessPx, col)) // end end: tip=b, shaft points back toward a
	return r
}

// strokeLine draws a thicknessPx-wide line between pixel points a and b by
// stamping a small filled square every ~half-pixel step along its length —
// the same pragmatic "good enough for a cosmetic overlay, not a general
// rasterizer" approach quadPixelRect's own doc comment already applies
// elsewhere in this file, rather than pulling in a real line-stroking
// algorithm. Reused for both a Line annotation's shaft and its arrowheads'
// open-style sides (see drawArrowHead).
func strokeLine(img *image.RGBA, a, b [2]float64, thicknessPx float64, col color.NRGBA) image.Rectangle {
	dx, dy := b[0]-a[0], b[1]-a[1]
	length := math.Hypot(dx, dy)
	if length == 0 {
		return image.Rectangle{}
	}
	half := thicknessPx / 2
	bounds := img.Bounds()
	steps := int(length*2) + 1 // ~half-pixel stamping — dense enough not to leave gaps
	r := image.Rectangle{}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		px, py := a[0]+dx*t, a[1]+dy*t
		seg := image.Rect(int(px-half), int(py-half), int(px+half)+1, int(py+half)+1)
		draw.Draw(img, seg.Intersect(bounds), image.NewUniform(col), image.Point{}, draw.Over)
		if i == 0 {
			r = seg
		} else {
			r = r.Union(seg)
		}
	}
	return r
}

// drawArrowHead paints an arrowhead at tip, pointing away from from (both
// pixel coordinates), sized relative to thicknessPx. "ClosedArrow"/
// "RClosedArrow" get a filled triangle (fillTriangle); "OpenArrow"/
// "ROpenArrow" get the same two sides stroked but unfilled (strokeLine);
// "None" or any style this app doesn't recognize (Square, Circle, Diamond,
// Butt, Slash — real but rare) draws nothing, matching how an unrecognized
// /LE name is treated the same as "None" elsewhere in this file, not
// guessed at.
func drawArrowHead(img *image.RGBA, tip, from [2]float64, style string, thicknessPx float64, col color.NRGBA) image.Rectangle {
	switch style {
	case "OpenArrow", "ClosedArrow", "ROpenArrow", "RClosedArrow":
	default:
		return image.Rectangle{}
	}

	dx, dy := tip[0]-from[0], tip[1]-from[1]
	length := math.Hypot(dx, dy)
	if length == 0 {
		return image.Rectangle{}
	}
	ux, uy := dx/length, dy/length // unit vector pointing toward the tip
	px, py := -uy, ux              // perpendicular unit vector

	headLen := math.Max(thicknessPx*arrowHeadLengthFactor, 8)
	headHalfWidth := math.Max(thicknessPx*arrowHeadHalfWidthFactor, 4)
	base := [2]float64{tip[0] - ux*headLen, tip[1] - uy*headLen}
	left := [2]float64{base[0] + px*headHalfWidth, base[1] + py*headHalfWidth}
	right := [2]float64{base[0] - px*headHalfWidth, base[1] - py*headHalfWidth}

	switch style {
	case "ClosedArrow", "RClosedArrow":
		return fillTriangle(img, tip, left, right, col)
	default: // OpenArrow, ROpenArrow
		return strokeLine(img, tip, left, thicknessPx, col).Union(strokeLine(img, tip, right, thicknessPx, col))
	}
}

// fillTriangle rasterizes a filled triangle (pixel coordinates) via a
// standard per-scanline edge-intersection scan, sampling each row's
// midpoint — simple and exact for the small, arrowhead-sized triangles
// this file ever draws, without pulling in a general polygon-fill library.
func fillTriangle(img *image.RGBA, p0, p1, p2 [2]float64, col color.NRGBA) image.Rectangle {
	bounds := img.Bounds()
	minY := int(math.Floor(math.Min(p0[1], math.Min(p1[1], p2[1]))))
	maxY := int(math.Ceil(math.Max(p0[1], math.Max(p1[1], p2[1]))))
	edges := [3][2][2]float64{{p0, p1}, {p1, p2}, {p2, p0}}

	r := image.Rectangle{}
	for y := minY; y <= maxY; y++ {
		yc := float64(y) + 0.5
		var xs []float64
		for _, e := range edges {
			ay, by := e[0][1], e[1][1]
			if (yc >= ay && yc < by) || (yc >= by && yc < ay) {
				t := (yc - ay) / (by - ay)
				xs = append(xs, e[0][0]+t*(e[1][0]-e[0][0]))
			}
		}
		if len(xs) < 2 {
			continue
		}
		sort.Float64s(xs)
		row := image.Rect(int(math.Round(xs[0])), y, int(math.Round(xs[len(xs)-1]))+1, y+1).Intersect(bounds)
		if row.Empty() {
			continue
		}
		draw.Draw(img, row, image.NewUniform(col), image.Point{}, draw.Over)
		if r.Empty() {
			r = row
		} else {
			r = r.Union(row)
		}
	}
	return r
}

// SetSelectedHighlight marks h (nil to clear) as the one paintHighlights
// outlines distinctly, matching the Highlights panel's own selection.
// Clears the render cache so both the previously- and newly-selected
// highlight's pages repaint with the change — the caller (highlightsPanel's
// list.OnSelected) is responsible for actually forcing those specific
// pages to re-render (view.repaintPage), the same way AddHighlight/
// DeleteHighlight/SetHighlightColor leave that to their callers too.
func (d *Document) SetSelectedHighlight(h *Highlight) {
	if d.selectedHighlight == h {
		return
	}
	d.selectedHighlight = h
	d.cache.Clear()
}

// lineHitTestPadPt widens a Line annotation's own bounding box by this
// many PDF-space points on every side for HighlightAt's hit test — a
// perfectly horizontal, vertical, or thin diagonal line otherwise has a
// literal bounding box only as wide/tall as its own stroke, which would
// make it nearly unclickable; Quad-based kinds don't need this since a
// highlighted line of text is never that thin. Widened from an original
// 4.0 after real hands-on testing found a perfectly vertical line
// genuinely hard to click precisely at a reduced (Fit Width) zoom, where
// this padding's fixed PDF-space size shrinks to just a handful of screen
// pixels each side.
const lineHitTestPadPt = 6.0

// HighlightAt returns the highlight/underline/strikeout/squiggly/line/
// generic-shape annotation on page whose bounding box contains the
// PDF-space point (x, y — origin bottom-left, same convention as
// Highlight.Quads), or nil. Used to click-select an annotation directly
// on the page (see view.handleHighlightTapped), the reverse of selecting
// it in the Highlights panel's list. A "Note" (Text/Popup) entry is never
// matched — it has no Quads, Line, or Rect geometry (see genericRectKinds,
// which Note isn't a member of), so none of the branches below have
// anything to test against for one.
//
// When multiple highlights overlap, returns the last match in
// d.Highlights (iterated in reverse) — paintHighlights draws the list in
// order, so the last one drawn is the topmost one visually, and that's the
// one a click should hit first.
func (d *Document) HighlightAt(page int, x, y float64) *Highlight {
	for i := len(d.Highlights) - 1; i >= 0; i-- {
		h := d.Highlights[i]
		if h.Page != page {
			continue
		}
		if h.Kind == "Line" && len(h.Line) == 4 {
			minX, maxX := min(h.Line[0], h.Line[2])-lineHitTestPadPt, max(h.Line[0], h.Line[2])+lineHitTestPadPt
			minY, maxY := min(h.Line[1], h.Line[3])-lineHitTestPadPt, max(h.Line[1], h.Line[3])+lineHitTestPadPt
			if x >= minX && x <= maxX && y >= minY && y <= maxY {
				return h
			}
			continue
		}
		if genericRectKinds[h.Kind] {
			if h.Rect == [4]float64{} {
				continue
			}
			if x >= h.Rect[0] && x <= h.Rect[2] && y >= h.Rect[1] && y <= h.Rect[3] {
				return h
			}
			continue
		}
		for _, q := range h.Quads {
			minX, minY, maxX, maxY := quadBoundsPt(q)
			if x >= minX && x <= maxX && y >= minY && y <= maxY {
				return h
			}
		}
	}
	return nil
}

// selectionOutlineColor/Width mark the currently-selected highlight (see
// SetSelectedHighlight) with a solid border on top of its usual translucent
// fill, so which row is selected in the Highlights panel is visible on the
// page too, not just in the list.
var selectionOutlineColor = color.NRGBA{R: 0, G: 120, B: 255, A: 255}

const selectionOutlineWidth = 3

// drawRectOutline draws a solid border of the given width just inside r's
// edges — four filled strips rather than a general stroke primitive, which
// is all a purely axis-aligned rectangle (see quadPixelRect's own doc
// comment on why quads are treated as axis-aligned) ever needs.
func drawRectOutline(img *image.RGBA, r image.Rectangle, col color.NRGBA, width int) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	sides := [4]image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+width), // top
		image.Rect(r.Min.X, r.Max.Y-width, r.Max.X, r.Max.Y), // bottom
		image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y), // left
		image.Rect(r.Max.X-width, r.Min.Y, r.Max.X, r.Max.Y), // right
	}
	for _, side := range sides {
		draw.Draw(img, side.Intersect(img.Bounds()), image.NewUniform(col), image.Point{}, draw.Over)
	}
}

// quadPixelRect converts one PDF /QuadPoints quad (4 (x,y) pairs, PDF
// user-space points, origin bottom-left) into an image-pixel rectangle at
// scale (pixels per point), flipping Y since image space is top-left-origin.
// Uses the quad's own bounding box rather than its 4 points as a polygon:
// real-world highlight quads are effectively always axis-aligned (unrotated
// text), so this is exact for the common case and a reasonable approximation
// for the rare rotated one, without needing a general polygon rasterizer.
func quadPixelRect(q [8]float64, pageHeightPt, scale float64) image.Rectangle {
	minX, minY, maxX, maxY := quadBoundsPt(q)
	return image.Rect(
		int(minX*scale), int((pageHeightPt-maxY)*scale),
		int(maxX*scale), int((pageHeightPt-minY)*scale),
	)
}

// quadBoundsPt returns q's axis-aligned bounding box in PDF user-space
// points — the same "use the bounding box, not the 4 points as a polygon"
// simplification quadPixelRect's own doc comment explains, factored out so
// SaveHighlights' write path (turning a drawn rectangle back into
// /QuadPoints) can share it instead of re-deriving the same min/max.
func quadBoundsPt(q [8]float64) (minX, minY, maxX, maxY float64) {
	minX, maxX = q[0], q[0]
	minY, maxY = q[1], q[1]
	for i := 1; i < 4; i++ {
		x, y := q[i*2], q[i*2+1]
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	return minX, minY, maxX, maxY
}

// drawTranslucentRect alpha-blends col over img within r (clipped to img's
// own bounds), via image/draw's standard source-over compositing.
func drawTranslucentRect(img *image.RGBA, r image.Rectangle, col color.NRGBA) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, image.NewUniform(col), image.Point{}, draw.Over)
}
