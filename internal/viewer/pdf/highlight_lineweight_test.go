package pdf

import (
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
)

func TestHasLineWidth(t *testing.T) {
	for kind, want := range map[string]bool{
		"Square": true, "Circle": true, "Polygon": true, "Ink": true, "PolyLine": true, "Line": true,
		// FreeText is deliberately excluded even though it's an
		// authoredGenericKinds member — see Highlight.LineWidth's own
		// doc comment for why a configurable width would have no
		// visible effect on it.
		"FreeText": false,
		// paintableMarkupKinds members have no /BS stroke-width concept
		// at all — their own mark thickness is a fixed fraction of the
		// quad's height, not a configurable border width.
		"Highlight": false, "Underline": false, "Strikeout": false, "Squiggly": false,
		"Note": false, "Stamp": false, "Caret": false,
	} {
		if got := hasLineWidth(kind); got != want {
			t.Errorf("hasLineWidth(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestLineWeightLabelRoundTrips(t *testing.T) {
	for _, w := range lineWeightPresets {
		label := lineWeightLabel(w)
		got, ok := parseLineWeightLabel(label)
		if !ok {
			t.Fatalf("parseLineWeightLabel(%q) failed to parse", label)
		}
		if got != w {
			t.Errorf("round trip %v -> %q -> %v, want %v", w, label, got, w)
		}
	}
}

func TestParseLineWeightLabel_RejectsGarbage(t *testing.T) {
	if _, ok := parseLineWeightLabel("not a number"); ok {
		t.Error("expected parseLineWeightLabel to reject a non-numeric label")
	}
}

func TestSetHighlightLineWidth_UpdatesFieldAndFlagsChanged(t *testing.T) {
	h := &Highlight{Kind: "Square", Rect: [4]float64{0, 0, 10, 10}, LineWidth: 1}
	h.origLineWidth = h.LineWidth
	d := &Document{cache: newPageCache(pageCacheCapacity), Highlights: []*Highlight{h}}

	d.SetHighlightLineWidth(h, 5)

	if h.LineWidth != 5 {
		t.Errorf("LineWidth = %v, want 5", h.LineWidth)
	}
	if _, _, lineWidthChanged, _ := highlightDictChanges(h); !lineWidthChanged {
		t.Error("expected highlightDictChanges to report lineWidthChanged=true after SetHighlightLineWidth")
	}
}

// TestSaveHighlights_LineWidthRoundTrips confirms a custom line weight
// chosen at draw time actually reaches the saved file and reads back
// correctly — the same round-trip discipline every other shape property
// (Color, Rect, Vertices) already gets.
func TestSaveHighlights_LineWidthRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0}, 5)

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 reloaded shape, got %d", len(doc.Highlights))
	}
	if got := doc.Highlights[0].LineWidth; got != 5 {
		t.Errorf("LineWidth after save+reload = %v, want 5", got)
	}
}

// TestSaveHighlights_LineWidthChangeOnAlreadySavedShapeSurvivesRealSave
// confirms changing an already-saved shape's line weight (not just a
// freshly-drawn one) actually persists through a real save — exercising
// the same delete-and-recreate path (lineWidthChanged, wired alongside
// colorChanged/geometryChanged) the baked-/AP color-change and Move
// regression tests already established, without needing a baked-/AP
// fixture specifically: this app's own AddRectShape-authored annotations
// never bake one (confirmed in the baked-/AP investigation elsewhere in
// this file), so the real risk here is purely "did the recreate/ObjNr-
// tracking plumbing handle lineWidthChanged correctly," not /AP-vs-/C
// precedence.
func TestSaveHighlights_LineWidthChangeOnAlreadySavedShapeSurvivesRealSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0}, 1)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("initial SaveHighlights: %v", err)
	}
	if len(doc.Highlights) != 1 || doc.Highlights[0].ObjNr <= 0 {
		t.Fatalf("expected 1 already-saved Square, got %+v", doc.Highlights)
	}
	saved := doc.Highlights[0]

	doc.SetHighlightLineWidth(saved, 8)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights after line-width change: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected exactly 1 highlight after the change+save, got %d (duplicate added?)", len(doc.Highlights))
	}
	if got := doc.Highlights[0].LineWidth; got != 8 {
		t.Errorf("LineWidth after change+save+reload = %v, want 8", got)
	}
}

// TestRenderPage_LineWidthChangeOnReopenedInkShowsRealShapeNotABox is a
// real, user-reported bug: an Ink/PolyLine stroke that's already saved
// from an EARLIER session (so THIS session's Document never captured its
// real Vertices — a fresh Prepare/LoadHighlights can never populate
// Vertices for an already-saved Ink, this app's own or foreign, see
// Highlight.Vertices' own doc comment) had its line weight changed, and
// the real freehand line visibly "disappeared" into a plain box the
// instant the change was made — even though the real Save always applied
// the new weight correctly regardless (confirmed separately: the file
// itself was never at risk, only the live preview). Root cause:
// buildNormalizedDoc used to delete the annotation from the scratch copy
// unconditionally on any colorChanged/lineWidthChanged, relying on
// paintHighlights' own hand-paint to show the result — but for a
// Vertices-less Polygon/Ink/PolyLine, that hand-paint is only ever a
// Rect-outline approximation, which reads as "the shape vanished," not
// "the shape updated."
//
// Fixed by only deleting (and so only hand-painting) when
// newAnnotationForShape can actually produce a correct replacement —
// otherwise the old, merely stale-valued annotation stays in the scratch
// copy for MuPDF to keep rendering. A pleasant surprise found while
// verifying this fix, not assumed going in: for exactly this case — a
// Polygon/Ink/PolyLine THIS APP itself originally authored — the shape
// updates correctly and IMMEDIATELY, no save needed at all, because this
// app never bakes an /AP for anything it authors (confirmed empirically —
// see CLAUDE.md), so applyPendingHighlightEdits' own in-place /BS patch
// (added for the case where recreate isn't possible) is NOT a cosmetic
// no-op here the way it would be against a real baked /AP — MuPDF's own
// default-appearance synthesis picks the new width up right away. So the
// real, correct expectation for THIS test isn't "stays exactly
// unchanged" (that's only true for a genuinely foreign, baked-/AP
// shape — see highlightHasBakedAppearance/notifyIfRenderIsDeferred for
// where that distinction is actually used), it's "shows the real,
// now-thicker polyline, not a filled/outlined box approximating its
// bounding rect."
//
// Distinguishes the two by comparing against a box-fallback's own
// INDEPENDENTLY computed pixel count for the exact same rect/color/width
// (via the same drawRectOutline/shapeStrokeWidthPx helpers the old,
// reverted fallback used) rather than just checking "did the pixel count
// change at all," which can't tell a real thicker line apart from a
// regression back to the box.
func TestRenderPage_LineWidthChangeOnReopenedInkShowsRealShapeNotABox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	// Round 1: draw + save, exactly like an earlier session would have.
	round1 := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	round1.AddInkShape(1, [][2]float64{{10, 10}, {30, 60}, {60, 20}, {90, 80}}, [3]float64{1, 0, 0}, 1)
	if err := round1.SaveHighlights(path); err != nil {
		t.Fatalf("round 1 save: %v", err)
	}

	// Round 2: a FRESH Prepare, simulating closing and reopening the tab —
	// this Document has never run AddInkShape/SaveHighlights itself, so
	// Vertices is whatever LoadHighlights alone produces (empty).
	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare round 2: %v", err)
	}
	defer doc.Close()
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(doc.Highlights))
	}
	h := doc.Highlights[0]
	if len(h.Vertices) != 0 {
		t.Fatalf("expected a reopened Ink stroke to load with no Vertices (the precondition for this bug), got %v", h.Vertices)
	}
	if highlightHasBakedAppearance(path, h.ObjNr) {
		t.Fatalf("expected this app's own authored Ink stroke to have no baked /AP")
	}

	imgBefore, err := doc.RenderPage(1, 2.0)
	if err != nil {
		t.Fatalf("RenderPage before weight change: %v", err)
	}
	beforeCount := countNonWhitePixelsForTest(imgBefore)

	const newWidth = 8.0
	doc.SetHighlightLineWidth(h, newWidth)
	imgAfter, err := doc.RenderPage(1, 2.0)
	if err != nil {
		t.Fatalf("RenderPage after weight change: %v", err)
	}
	afterCount := countNonWhitePixelsForTest(imgAfter)

	if afterCount <= beforeCount {
		t.Errorf("expected the render to show a visibly THICKER line after the weight change, got before=%d after=%d", beforeCount, afterCount)
	}

	// The precise check: a box-outline fallback strokes all the way
	// around the shape's own bounding Rect, including along its top
	// edge — but the real zigzag path ((10,10)->(30,60)->(60,20)->(90,80))
	// never comes near PDF point (15, 78), well inside the box's top-left
	// corner. Sample the SAME point in an independently-computed
	// box-fallback image (same Rect/color/width, drawn with the exact
	// primitives the old, reverted fallback used) to confirm it really
	// would be painted there, then confirm the REAL render is NOT — a
	// targeted, semantically-meaningful signal that doesn't depend on
	// guessing an exact aggregate pixel-count ratio (anti-aliasing and
	// stroke-stamping overlap make that fragile in practice — an earlier
	// draft of this test tried exactly that and the real threshold turned
	// out to need hand-tuning per run).
	_, pageH, err := doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	// RenderPage's own scale (see its source): dpi = baseRenderDPI * zoom,
	// scale = dpi / 72 — baseRenderDPI is this package's own unexported
	// constant (150.0 at the time of writing, not 72), reused directly
	// here (same package) rather than hand-copying its current value, so
	// this stays correct if that constant ever changes.
	scale := (baseRenderDPI * 2.0) / 72.0
	r := rectPixelRect(h.Rect, pageH, scale)
	const safeX, safeYPdf = 15.0, 78.0
	safeXPx := int(safeX * scale)
	safeYPx := int((pageH - safeYPdf) * scale)

	boxImg := image.NewRGBA(imgAfter.Bounds())
	draw.Draw(boxImg, boxImg.Bounds(), image.White, image.Point{}, draw.Src)
	drawRectOutline(boxImg, r, color.NRGBA{R: 255, A: markupLineAlpha}, shapeStrokeWidthPx(newWidth, scale))
	if br, bg, bb, _ := boxImg.At(safeXPx, safeYPx).RGBA(); br == 0xffff && bg == 0xffff && bb == 0xffff {
		t.Fatalf("test setup error: expected the independently-computed box fallback to paint near its own top edge at pixel (%d,%d)", safeXPx, safeYPx)
	}

	if ar, ag, ab, _ := imgAfter.At(safeXPx, safeYPx).RGBA(); ar != 0xffff || ag != 0xffff || ab != 0xffff {
		t.Errorf("expected pixel (%d,%d) — on a box-fallback's own top edge, nowhere near the real zigzag path — to stay white in the real render; got non-white (r=%d g=%d b=%d), suggesting a box was painted instead of the real line", safeXPx, safeYPx, ar>>8, ag>>8, ab>>8)
	}
}

func countNonWhitePixelsForTest(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || bl != 0xffff {
				n++
			}
		}
	}
	return n
}

// TestHandleHighlightDrawn_UsesCurrentLineWidth confirms the view layer
// actually threads v.lineWidthPt through to the newly-drawn shape, not
// just a hardcoded default.
func TestHandleHighlightDrawn_UsesCurrentLineWidth(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH))

	v.drawKind = "Square"
	v.lineWidthPt = 5
	v.handleHighlightDrawn(1, fyne.NewPos(20, 10), fyne.NewPos(80, 60), widgetSize)

	if len(v.doc.Highlights) != 1 {
		t.Fatalf("expected 1 new highlight, got %d", len(v.doc.Highlights))
	}
	if got := v.doc.Highlights[0].LineWidth; got != 5 {
		t.Errorf("LineWidth = %v, want 5 (v.lineWidthPt)", got)
	}
}
