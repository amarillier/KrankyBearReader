package pdf

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestParseQuadPoints_ValidArray(t *testing.T) {
	xRefTable := &model.XRefTable{}
	arr := types.Array{
		types.Float(0), types.Float(10), types.Float(100), types.Float(10),
		types.Float(0), types.Float(0), types.Float(100), types.Float(0),
	}
	got := parseQuadPoints(xRefTable, arr)
	if len(got) != 1 {
		t.Fatalf("expected 1 quad, got %d", len(got))
	}
	want := [8]float64{0, 10, 100, 10, 0, 0, 100, 0}
	if got[0] != want {
		t.Errorf("quad = %v, want %v", got[0], want)
	}
}

func TestParseQuadPoints_MultipleQuads(t *testing.T) {
	xRefTable := &model.XRefTable{}
	arr := make(types.Array, 16)
	for i := range arr {
		arr[i] = types.Float(i)
	}
	got := parseQuadPoints(xRefTable, arr)
	if len(got) != 2 {
		t.Fatalf("expected 2 quads, got %d", len(got))
	}
}

func TestParseQuadPoints_RejectsWrongLength(t *testing.T) {
	xRefTable := &model.XRefTable{}
	cases := []types.Array{
		nil,
		{},
		{types.Float(1), types.Float(2), types.Float(3)}, // not a multiple of 8
	}
	for _, c := range cases {
		if got := parseQuadPoints(xRefTable, c); got != nil {
			t.Errorf("parseQuadPoints(%v) = %v, want nil", c, got)
		}
	}
}

func TestParseColor_DeviceGray(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(0.5)})
	want := [3]float64{0.5, 0.5, 0.5}
	if got != want {
		t.Errorf("parseColor(gray) = %v, want %v", got, want)
	}
}

func TestParseColor_DeviceRGB(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(1), types.Float(0), types.Float(0)})
	want := [3]float64{1, 0, 0}
	if got != want {
		t.Errorf("parseColor(rgb) = %v, want %v", got, want)
	}
}

func TestParseColor_DeviceCMYK(t *testing.T) {
	xRefTable := &model.XRefTable{}
	// Pure black in CMYK (K=1) should come out as RGB black.
	got := parseColor(xRefTable, types.Array{types.Float(0), types.Float(0), types.Float(0), types.Float(1)})
	want := [3]float64{0, 0, 0}
	if got != want {
		t.Errorf("parseColor(cmyk black) = %v, want %v", got, want)
	}
}

func TestParseColor_FallsBackToDefaultOnUnknownLength(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseColor(xRefTable, types.Array{types.Float(1), types.Float(1)})
	if got != defaultHighlightColor {
		t.Errorf("parseColor(2 components) = %v, want default %v", got, defaultHighlightColor)
	}
}

func TestCmykToRGB(t *testing.T) {
	// Pure cyan (C=1, everything else 0) has no red.
	got := cmykToRGB(1, 0, 0, 0)
	want := [3]float64{0, 1, 1}
	if got != want {
		t.Errorf("cmykToRGB(cyan) = %v, want %v", got, want)
	}
}

func TestQuadPixelRect_FlipsYAndScales(t *testing.T) {
	// A quad occupying the top-left inch of a 10x10-point page, at 2x scale
	// (144 DPI): PDF y is measured from the bottom, so the top of the page
	// (near y=10) should map to pixel y near 0.
	q := [8]float64{0, 9, 1, 9, 0, 10, 1, 10}
	got := quadPixelRect(q, 10, 2)
	want := image.Rect(0, 0, 2, 2)
	if got != want {
		t.Errorf("quadPixelRect = %v, want %v", got, want)
	}
}

func TestHighlightGeometry_MissingObjectFallsBackGracefully(t *testing.T) {
	xRefTable := &model.XRefTable{}
	quads, col := highlightGeometry(xRefTable, 0) // objNr <= 0: no indirect ref to re-dereference
	if quads != nil {
		t.Errorf("expected nil quads for objNr<=0, got %v", quads)
	}
	if col != defaultHighlightColor {
		t.Errorf("expected default color for objNr<=0, got %v", col)
	}
}

func TestPaintableMarkupKinds_HasQuadKindsOnlyNotNote(t *testing.T) {
	want := map[string]bool{"Highlight": true, "Underline": true, "Strikeout": true, "Squiggly": true}
	if len(paintableMarkupKinds) != len(want) {
		t.Fatalf("expected %d paintable kinds, got %d: %v", len(want), len(paintableMarkupKinds), paintableMarkupKinds)
	}
	for k, v := range want {
		if paintableMarkupKinds[k] != v {
			t.Errorf("paintableMarkupKinds[%q] = %v, want %v", k, paintableMarkupKinds[k], v)
		}
	}
	if paintableMarkupKinds["Note"] {
		t.Errorf("expected Note to stay excluded from paintableMarkupKinds — it has no quads to paint")
	}
}

// TestMarkupLineRect_OrdersUnderlineBelowStrikeout guards the fraction
// constants' relative placement, not their exact values: for the same quad,
// Underline (near the text's baseline) must land at a larger pixel Y (lower
// on the page image, since image Y grows downward) than Strikeout (near the
// text's vertical middle) — getting underlineFrac/strikeoutFrac backwards
// would silently draw an underline through mid-text and a strikeout at the
// baseline instead.
func TestMarkupLineRect_OrdersUnderlineBelowStrikeout(t *testing.T) {
	q := [8]float64{0, 10, 10, 10, 0, 0, 10, 0} // quad spanning y=[0,10] on a 10pt-tall page
	under := markupLineRect(q, underlineFrac, 10, 1)
	strike := markupLineRect(q, strikeoutFrac, 10, 1)
	if under.Min.Y <= strike.Min.Y {
		t.Errorf("expected underline (baseline) below strikeout (mid-height) in image space, got under=%v strike=%v", under, strike)
	}
}

// TestMarkupLineRect_StaysWithinQuadWidth confirms the rect spans exactly
// the quad's own X range at the given scale, the same axis-aligned-bbox
// convention quadPixelRect uses.
func TestMarkupLineRect_StaysWithinQuadWidth(t *testing.T) {
	q := [8]float64{0, 10, 10, 10, 0, 0, 10, 0}
	r := markupLineRect(q, underlineFrac, 10, 2) // 2x scale
	if r.Min.X != 0 || r.Max.X != 20 {
		t.Errorf("expected X range [0,20] at 2x scale, got [%d,%d]", r.Min.X, r.Max.X)
	}
}

// TestDrawSquigglyLine_PaintsWithinItsOwnBounds is a smoke test for the
// per-pixel-column wave draw: it must actually paint something inside the
// bounding rect it returns (a regression against a phase/amplitude bug
// that computes a rect but never touches a pixel), and must leave pixels
// well outside the quad's own X range untouched, not flood the whole image.
func TestDrawSquigglyLine_PaintsWithinItsOwnBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	q := [8]float64{0, 8, 10, 8, 0, 4, 10, 4} // quad width 10pt, height 4pt
	col := color.NRGBA{R: 255, A: 200}

	r := drawSquigglyLine(img, q, 20, 1, col)
	if r.Empty() {
		t.Fatalf("expected a non-empty bounding rect, got %v", r)
	}

	painted := false
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				painted = true
			}
		}
	}
	if !painted {
		t.Errorf("expected at least one pixel painted within bounding rect %v, found none", r)
	}

	if got := img.RGBAAt(19, 19); got.A != 0 {
		t.Errorf("expected pixels outside the quad's own width to stay untouched, got %v", got)
	}
}

func TestHasPaintedColor_IncludesLineAlongsideQuadKinds(t *testing.T) {
	for kind, want := range map[string]bool{
		"Highlight": true, "Underline": true, "Strikeout": true, "Squiggly": true,
		"Line": true, "Note": false,
		// Square/Circle/Polygon/FreeText/Ink/PolyLine: this app's own
		// authored shape kinds (see AddRectShape/AddPolygonShape/
		// AddTextShape/AddInkShape/AddPolyLineShape) — real user feedback
		// found "Change Color" refused to work on a freshly-drawn shape at
		// all, and it turned out to just be this gate never having been
		// extended past the original markup/Line kinds. Stamp/Caret stay
		// false: this app never authors those, and has no per-kind
		// geometry to hand-paint a new color with even if it tried.
		"Square": true, "Circle": true, "Polygon": true, "FreeText": true, "Ink": true, "PolyLine": true,
		"Stamp": false, "Caret": false,
	} {
		if got := hasPaintedColor(kind); got != want {
			t.Errorf("hasPaintedColor(%q) = %v, want %v", kind, got, want)
		}
	}
}

// TestPaintHighlights_PolygonWithoutVerticesFallsBackToRectOutline is the
// real regression guard for a bug that would otherwise ship alongside
// enabling "Change Color" for Polygon: an already-saved Polygon this app
// didn't draw itself (e.g. a real Preview-authored one, loaded back with
// Rect populated but Vertices empty — see Highlight.Vertices' own doc
// comment) must still paint SOMETHING when its color is changed, not
// nothing at all — recoloring it routes through the exact same
// needsHandPaint path a freshly-drawn one does, and drawPolygonOutline
// itself correctly paints nothing for an empty vertex list, so
// paintHighlights' own Polygon case must fall back to a plain Rect
// outline rather than calling drawPolygonOutline blindly.
// TestPaintHighlights_PolygonWithoutVerticesSkipsHandPaint supersedes an
// earlier version of this test (same name minus "Skips...", which
// asserted the OPPOSITE: that a Rect-outline fallback got painted).
// Reverted after real user testing found that fallback actively
// misleading rather than merely approximate: recoloring or reweighting an
// already-saved Ink/PolyLine stroke made the real freehand line visibly
// "disappear" into a plain box the instant the change was made, even
// though nothing was actually lost (the real Save always applied the new
// value correctly regardless — see applyPendingHighlightEdits' own
// in-place fallback). Now, when newAnnotationForShape can't produce a
// correct replacement (Vertices-less Polygon/Ink/PolyLine), needsHandPaint
// stays false entirely — paintHighlights paints NOTHING for the highlight
// itself, trusting buildNormalizedDoc to have correspondingly left the old
// annotation in the scratch copy for MuPDF to render as-is (stale value,
// correct shape) rather than deleting it with nothing accurate to put
// back. This test only exercises paintHighlights in isolation (no real
// annotation on disk for a synthetic Highlight constructed by hand), so
// "nothing painted" here is the whole, correct story — the "MuPDF still
// shows the real shape" half is covered by
// TestSaveHighlights_RecolorForeignPolygonDoesNotDeleteIt-style real-file
// tests and highlightsPanel's own notifyIfRenderIsDeferred notice.
func TestPaintHighlights_PolygonWithoutVerticesSkipsHandPaint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)
	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer doc.Close()

	h := &Highlight{
		Page: 1, Kind: "Polygon", ObjNr: 42, // already-saved -- not ObjNr == 0
		Rect: [4]float64{20, 20, 80, 80}, Color: [3]float64{0, 1, 0},
		origColor: [3]float64{1, 0, 0}, // diverged from origColor -> colorChanged
	}
	doc.Highlights = []*Highlight{h}

	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	doc.paintHighlights(img, 1, 72) // scale = 1.0 at 72 DPI

	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				t.Fatalf("expected nothing hand-painted for a colorChanged Polygon with no Vertices (can't produce a correct replacement), found a painted pixel at (%d,%d)", x, y)
			}
		}
	}
}

// TestClampLinePointsToRect_MirrorsOutOfRangeEndpointFromTheGoodOne guards
// the real bug found via a real PDF (see ReleaseNotes' Version 0.5.0
// notes): a macOS Preview-authored arrow's /L had one endpoint whose Y sat
// ~145pt above its own /Rect's top edge (and off the page entirely) —
// almost certainly stale from an earlier edit that resized /Rect without
// updating /L. The repair must mirror the bad Y from the other, in-range
// endpoint's own Y (667.08, which happens to sit almost exactly at rect's
// vertical midpoint 666.92 — the actual clue this repair is based on, see
// clampLinePointsToRect's doc comment), not clamp it to rect's far edge
// (687.67) — that edge is a full ~20pt further from the good endpoint's
// value than the repair should land.
func TestClampLinePointsToRect_MirrorsOutOfRangeEndpointFromTheGoodOne(t *testing.T) {
	points := []float64{482.39, 667.08, 366.07, 833.33}
	rect := &[4]float64{290.29, 646.17, 491.86, 687.67}

	got := clampLinePointsToRect(points, rect)
	want := []float64{482.39, 667.08, 366.07, 667.08}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("clampLinePointsToRect(...)[%d] = %v, want %v (full: got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

func TestClampLinePointsToRect_LeavesInRangePointsUntouched(t *testing.T) {
	points := []float64{10, 10, 20, 20}
	rect := &[4]float64{0, 0, 100, 100}
	got := clampLinePointsToRect(points, rect)
	for i, v := range points {
		if got[i] != v {
			t.Errorf("clampLinePointsToRect(...)[%d] = %v, want unchanged %v", i, got[i], v)
		}
	}
}

func TestParseRectPt_RejectsWrongLength(t *testing.T) {
	xRefTable := &model.XRefTable{}
	cases := []types.Array{nil, {}, {types.Float(1), types.Float(2)}}
	for _, c := range cases {
		if got := parseRectPt(xRefTable, c); got != nil {
			t.Errorf("parseRectPt(%v) = %v, want nil", c, got)
		}
	}
}

func TestParseRectPt_ValidArray(t *testing.T) {
	xRefTable := &model.XRefTable{}
	got := parseRectPt(xRefTable, types.Array{types.Float(1), types.Float(2), types.Float(3), types.Float(4)})
	want := &[4]float64{1, 2, 3, 4}
	if got == nil || *got != *want {
		t.Errorf("parseRectPt = %v, want %v", got, want)
	}
}

// TestStrokeLine_PaintsBothEndpoints is a smoke test for the shaft-drawing
// helper reused by both a Line annotation's body and its open-arrowhead
// sides: it must actually paint pixels near both endpoints, not just
// somewhere along the way (a regression against an off-by-one in the step
// count leaving an endpoint unpainted).
func TestStrokeLine_PaintsBothEndpoints(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 200}
	r := strokeLine(img, [2]float64{2, 2}, [2]float64{16, 16}, 2, col)
	if r.Empty() {
		t.Fatalf("expected a non-empty bounding rect, got %v", r)
	}
	if img.RGBAAt(2, 2).A == 0 {
		t.Errorf("expected the start point to be painted, found nothing at (2,2)")
	}
	if img.RGBAAt(16, 16).A == 0 {
		t.Errorf("expected the end point to be painted, found nothing at (16,16)")
	}
}

// TestFillTriangle_PaintsInteriorNotJustBounds confirms the scanline fill
// actually paints the triangle's interior (e.g. its centroid), not merely
// pixels along its bounding box's edges.
func TestFillTriangle_PaintsInteriorNotJustBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 200}
	p0, p1, p2 := [2]float64{2, 2}, [2]float64{18, 2}, [2]float64{10, 18}
	r := fillTriangle(img, p0, p1, p2, col)
	if r.Empty() {
		t.Fatalf("expected a non-empty bounding rect, got %v", r)
	}
	cx, cy := int((p0[0]+p1[0]+p2[0])/3), int((p0[1]+p1[1]+p2[1])/3)
	if img.RGBAAt(cx, cy).A == 0 {
		t.Errorf("expected the triangle's centroid (%d,%d) to be painted, found nothing", cx, cy)
	}
}

// TestDrawArrowHead_SkipsUnrecognizedStyles guards the "None"/unrecognized
// fallback: nothing should be painted for a style this app doesn't draw an
// arrowhead for, matching how an absent /LE (defaulting to "None") is
// already handled.
func TestDrawArrowHead_SkipsUnrecognizedStyles(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 200}
	for _, style := range []string{"None", "Square", "Diamond", ""} {
		if r := drawArrowHead(img, [2]float64{10, 10}, [2]float64{2, 2}, style, 2, col); !r.Empty() {
			t.Errorf("drawArrowHead(style=%q) = %v, want an empty rect (nothing drawn)", style, r)
		}
	}
}

// TestDrawArrowHead_ClosedArrowFillsInteriorOpenArrowDoesNot confirms the
// two recognized arrow styles actually differ visually: "ClosedArrow"
// fills the whole triangle including its interior (fillTriangle), while
// "OpenArrow" only strokes its two outer sides (strokeLine), leaving the
// interior untouched near the triangle's base — its two sides start
// together at the tip (so both styles paint near there regardless) but
// diverge well past the strokes' own width by the base. For
// tip=(30,20)/from=(5,20)/thickness=3, (21,20) sits near the base's own
// midline and was confirmed (via a pixel-map dump while writing this test,
// not derived by hand — the diagonal square-stamped strokes in strokeLine
// don't paint a simple perpendicular-distance band) to be filled for
// ClosedArrow and empty for OpenArrow.
func TestDrawArrowHead_ClosedArrowFillsInteriorOpenArrowDoesNot(t *testing.T) {
	paintedAt := func(style string, x, y int) bool {
		img := image.NewRGBA(image.Rect(0, 0, 40, 40))
		col := color.NRGBA{R: 255, A: 200}
		drawArrowHead(img, [2]float64{30, 20}, [2]float64{5, 20}, style, 3, col)
		return img.RGBAAt(x, y).A > 0
	}
	if !paintedAt("ClosedArrow", 21, 20) {
		t.Errorf("expected ClosedArrow to fill its own interior near the base, found nothing")
	}
	if paintedAt("OpenArrow", 21, 20) {
		t.Errorf("expected OpenArrow to leave its interior unfilled near the base (only the two sides stroked), found paint there")
	}
}

// TestLineBoundsPixelRect_PerfectlyAxisAlignedLineIsNeverEmpty is the real
// regression test for a bug found via hands-on testing against a real
// PDF: a perfectly vertical (or horizontal) Line annotation's own two
// endpoints share one coordinate exactly, so the naive scaled rect has
// that axis's width or height at literally zero — and
// image.Rectangle.Empty() reports ANY zero-width-or-height rectangle as
// empty, which drawRectOutline (paintHighlights' selection indicator)
// treats as "nothing to draw" and skips entirely. Confirmed on the real
// file that surfaced this: selecting that Line in the Highlights panel
// never outlined it on the page at all — zero pixels painted, not just a
// hard-to-notice thin one.
func TestLineBoundsPixelRect_PerfectlyAxisAlignedLineIsNeverEmpty(t *testing.T) {
	vertical := []float64{373.0899, 521.1978, 373.0899, 451.413} // the real annotation's own /L
	horizontal := []float64{100, 200, 300, 200}

	for _, points := range [][]float64{vertical, horizontal} {
		r := lineBoundsPixelRect(points, 756.0, 150.0/72.0)
		if r.Empty() {
			t.Errorf("lineBoundsPixelRect(%v) = %v, want a non-empty rect", points, r)
		}
		if r.Dx() < minLineOutlinePx || r.Dy() < minLineOutlinePx {
			t.Errorf("lineBoundsPixelRect(%v) = %v, want both Dx and Dy >= %d", points, r, minLineOutlinePx)
		}
	}
}

// TestDrawLineAnnotation_MalformedPointsPaintsNothing mirrors the
// "malformed geometry -> list without painting" convention parseQuadPoints
// and parseLinePoints already follow.
func TestDrawLineAnnotation_MalformedPointsPaintsNothing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 200}
	r := drawLineAnnotation(img, []float64{1, 2, 3}, [2]string{"None", "None"}, 1, col, 20, 1)
	if !r.Empty() {
		t.Errorf("expected an empty rect for malformed points, got %v", r)
	}
}

// TestDrawEllipseOutline_PaintsRingNotFill confirms a Circle's hand-paint
// preview (paintHighlights' own needsHandPaint gate, for a freshly-drawn,
// not-yet-saved Circle) actually draws a ring — painted near the
// bounding box's own edges, all four compass points, but NOT at the
// center (a stroke, matching Preview's own default appearance for a
// freshly-drawn Circle/Square shape, not a filled disc).
func TestDrawEllipseOutline_PaintsRingNotFill(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	r := image.Rect(10, 10, 90, 90) // center (50,50), rx=ry=40
	col := color.NRGBA{R: 0, G: 255, B: 0, A: 255}

	drawEllipseOutline(img, r, col, 3)

	painted := func(x, y int) bool { return img.RGBAAt(x, y).A > 0 }
	for _, pt := range [][2]int{{50, 10}, {50, 89}, {10, 50}, {89, 50}} {
		if !painted(pt[0], pt[1]) {
			t.Errorf("expected the ring painted at compass point %v, found nothing", pt)
		}
	}
	if painted(50, 50) {
		t.Errorf("expected the center NOT painted (a stroke, not a fill), found paint there")
	}
}

// TestDrawEllipseOutline_DegenerateRectPaintsNothing guards the same
// "nothing to draw" case lineBoundsPixelRect's own fix had to handle for
// a Line — a zero-width or zero-height bounding box has no ellipse to
// stroke at all, so this must return cleanly rather than divide by zero
// or loop forever.
func TestDrawEllipseOutline_DegenerateRectPaintsNothing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 255}
	drawEllipseOutline(img, image.Rect(5, 5, 5, 15), col, 2) // zero width
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if img.RGBAAt(x, y).A > 0 {
				t.Fatalf("expected nothing painted for a zero-width rect, found paint at (%d,%d)", x, y)
			}
		}
	}
}

// TestDrawPolygonOutline_ClosesTheLoop confirms a Star/Hexagon's own
// hand-paint preview strokes every edge INCLUDING the one connecting the
// last vertex back to the first — the "closes back to the first vertex"
// contract Highlight.Vertices' own doc comment promises. A square-ish
// 4-vertex polygon's closing edge (from (90,10) back to (10,10)) is
// checked directly at its own midpoint, a point no OTHER edge could
// plausibly paint.
func TestDrawPolygonOutline_ClosesTheLoop(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	col := color.NRGBA{R: 0, G: 0, B: 255, A: 255}
	vertices := [][2]float64{{10, 10}, {10, 90}, {90, 90}, {90, 10}}

	drawPolygonOutline(img, vertices, col, 3)

	if img.RGBAAt(50, 10).A == 0 {
		t.Errorf("expected the closing edge (90,10)->(10,10) painted at its midpoint (50,10), found nothing")
	}
}

func TestDrawPolygonOutline_FewerThanTwoVerticesPaintsNothing(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	col := color.NRGBA{R: 255, A: 255}
	drawPolygonOutline(img, [][2]float64{{5, 5}}, col, 2)
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if img.RGBAAt(x, y).A > 0 {
				t.Fatalf("expected nothing painted for a single-vertex polygon, found paint at (%d,%d)", x, y)
			}
		}
	}
}
