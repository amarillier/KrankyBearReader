package pdf

import (
	"image"
	"image/color"
	"image/draw"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Highlight is one PDF annotation worth surfacing in the Highlights and
// Notes panel: a highlight/underline/strikeout/squiggly markup, or a
// text/popup note. Link annotations are deliberately excluded — they aren't
// "highlights or notes". This app doesn't author new annotations, only lists
// and (for Highlight specifically — see Quads) paints what other apps
// (Preview, Acrobat, ...) already wrote.
type Highlight struct {
	Page     int
	Kind     string
	Contents string

	// Quads and Color are populated for Kind == "Highlight" only (Underline/
	// Strikeout/Squiggly/Note painting is a possible future step, not
	// implemented yet — see ReleaseNotes' Future ideas). Quads is one
	// rectangle per QuadPoints entry — usually one per highlighted line/run —
	// in PDF user-space points (origin bottom-left, matching Document.Bound).
	// Empty when the PDF's /QuadPoints couldn't be read (e.g. a malformed or
	// missing entry), in which case the highlight is still listed but not
	// painted on the page. Color is RGB, 0..1 per channel, defaulting to
	// standard highlighter yellow when the PDF doesn't specify one.
	Quads [][8]float64
	Color [3]float64
}

// highlightKinds are the annotation types worth listing, and their display
// label. Anything not in this map (Link, Widget, Stamp, ...) is skipped.
var highlightKinds = map[model.AnnotationType]string{
	model.AnnHighLight: "Highlight",
	model.AnnUnderline: "Underline",
	model.AnnStrikeOut: "Strikeout",
	model.AnnSquiggly:  "Squiggly",
	model.AnnText:      "Note",
	model.AnnPopup:     "Note",
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
					Page: page,
					Kind: label,
					// Content(), not ContentString(): MarkupAnnotation
					// overrides ContentString() to wrap the text in literal
					// quotes for CLI/debug display (confirmed by reading
					// pdfcpu's source after a test caught it) — not what we
					// want in a UI list.
					Contents: a.Content(),
				}
				if typ == model.AnnHighLight {
					h.Quads, h.Color = highlightGeometry(xRefTable, objNr)
				}
				out = append(out, h)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Page < out[j].Page })
	return out
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

// paintHighlights composites every Highlight-kind annotation on page directly
// onto img (already rendered at dpi by go-fitz), one translucent rectangle
// per quad. Silently does nothing if there are no highlights on this page, or
// if the page's own bounds can't be read — this is cosmetic, not something
// worth failing a page render over.
func (d *Document) paintHighlights(img *image.RGBA, page int, dpi float64) {
	var pageHeightPt float64
	haveBounds := false

	scale := dpi / 72.0
	for _, h := range d.Highlights {
		if h.Page != page || len(h.Quads) == 0 {
			continue
		}
		if !haveBounds {
			bounds, err := d.doc.Bound(page - 1) // go-fitz is 0-based
			if err != nil {
				return
			}
			pageHeightPt = float64(bounds.Dy())
			haveBounds = true
		}
		col := color.NRGBA{
			R: uint8(h.Color[0] * 255),
			G: uint8(h.Color[1] * 255),
			B: uint8(h.Color[2] * 255),
			A: highlightOverlayAlpha,
		}
		for _, q := range h.Quads {
			drawTranslucentRect(img, quadPixelRect(q, pageHeightPt, scale), col)
		}
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
	minX, maxX := q[0], q[0]
	minY, maxY := q[1], q[1]
	for i := 1; i < 4; i++ {
		x, y := q[i*2], q[i*2+1]
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	return image.Rect(
		int(minX*scale), int((pageHeightPt-maxY)*scale),
		int(maxX*scale), int((pageHeightPt-minY)*scale),
	)
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
