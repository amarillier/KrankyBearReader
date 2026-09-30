package pdf

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestRenderPage_PendingDeleteRemovesOnlyThatHighlight is the real
// regression test for the bug the first approach (a hand-painted "erase
// this rectangle" patch) couldn't fix: deleting an already-saved
// highlight must make ONLY that highlight disappear from the page,
// leaving every other annotation — even one whose own bounding box
// overlaps the deleted one's — exactly as it was. A patch has no way to
// know what should still be showing inside an overlapping region;
// rendering a real, freshly-normalized scratch copy with the deletion
// actually applied (see Document.rebuildDoc/buildNormalizedDoc) does,
// because it's just a real render of a real file.
func TestRenderPage_PendingDeleteRemovesOnlyThatHighlight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path) // 200x200 page (see writeMinimalPDF's own MediaBox)

	// Two OVERLAPPING highlights — B's own rect is fully inside A's — the
	// exact shape of the real-world bug (two arrows crossing near the
	// same text). A patch erasing A's whole bounding box would have no
	// way to know B's own color should still show inside the overlap.
	rectA := types.NewRectangle(10, 10, 190, 190)
	quadA := types.QuadPoints{*types.NewQuadLiteralForRect(rectA)}
	red := color.SimpleColor{R: 1, G: 0, B: 0}
	annA := model.NewHighlightAnnotation(*rectA, 0, "", "", "", 0, &red, 0, 0, 0, "", nil, nil, "", "", quadA)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, annA, nil, false); err != nil {
		t.Fatalf("adding annotation A: %v", err)
	}

	rectB := types.NewRectangle(60, 60, 140, 140)
	quadB := types.QuadPoints{*types.NewQuadLiteralForRect(rectB)}
	blue := color.SimpleColor{R: 0, G: 0, B: 1}
	annB := model.NewHighlightAnnotation(*rectB, 0, "", "", "", 0, &blue, 0, 0, 0, "", nil, nil, "", "", quadB)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, annB, nil, false); err != nil {
		t.Fatalf("adding annotation B: %v", err)
	}

	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer doc.Close()

	var a, b *Highlight
	for _, h := range doc.Highlights {
		switch h.Color {
		case [3]float64{1, 0, 0}:
			a = h
		case [3]float64{0, 0, 1}:
			b = h
		}
	}
	if a == nil || b == nil {
		t.Fatalf("expected to find both highlights, got %+v", doc.Highlights)
	}

	// Sample points: pxOnlyA is inside A but outside B's rect; pxOverlap
	// is inside BOTH (where the patch-based approach would have failed).
	const pageHeightPt = 200.0
	scale := baseRenderDPI / 72.0
	pixelAt := func(xPt, yPt float64) image.Point {
		return image.Pt(int(xPt*scale), int((pageHeightPt-yPt)*scale))
	}
	pxOnlyA := pixelAt(20, 20)
	pxOverlap := pixelAt(100, 100)

	before, err := doc.RenderPage(1, 1.0)
	if err != nil {
		t.Fatalf("RenderPage (before delete): %v", err)
	}
	beforeImg, ok := before.(*image.RGBA)
	if !ok {
		t.Fatalf("expected *image.RGBA, got %T", before)
	}
	beforeOnlyA := beforeImg.RGBAAt(pxOnlyA.X, pxOnlyA.Y)

	if !doc.DeleteHighlight(a) {
		t.Fatalf("DeleteHighlight(a) reported not found")
	}

	after, err := doc.RenderPage(1, 1.0)
	if err != nil {
		t.Fatalf("RenderPage (after delete): %v", err)
	}
	afterImg, ok := after.(*image.RGBA)
	if !ok {
		t.Fatalf("expected *image.RGBA, got %T", after)
	}
	afterOnlyA := afterImg.RGBAAt(pxOnlyA.X, pxOnlyA.Y)
	afterOverlap := afterImg.RGBAAt(pxOverlap.X, pxOverlap.Y)

	if afterOnlyA == beforeOnlyA {
		t.Errorf("expected A's own region (outside the overlap) to change after deleting A, got unchanged %v", afterOnlyA)
	}
	// The overlap pixel is expected to CHANGE from before to after — before
	// deletion it's the blended result of A and B both rendering there
	// (multiply-blend of red and blue came out pure black in practice);
	// after deleting A, only B is left, so the pixel correctly shows B's
	// own color undisturbed, not "unchanged from before" (that would mean
	// A's removal had no visible effect at all, which would itself be
	// wrong). Assert the real thing that matters: B's own blue survived,
	// not smeared into background white or corrupted some other way — the
	// exact failure mode a hand-painted "erase this whole rectangle" patch
	// would have caused here, blanking the overlap instead of preserving B.
	if afterOverlap.B <= afterOverlap.R || afterOverlap.B <= afterOverlap.G || afterOverlap.A == 0 {
		t.Errorf("expected the overlap region to show B's own blue-dominant color after deleting A, got %v", afterOverlap)
	}
}

// TestRenderPage_ColorChangeShowsImmediatelyWithoutDisturbingOverlap is the
// regression test for the color-change counterpart to the delete test
// above: SetHighlightColor on an already-saved highlight must show its NEW
// color on the very next RenderPage, without needing Save, and — the part
// a naive fix could get wrong — without disturbing a DIFFERENT, still-live
// annotation whose own bounding box overlaps it. Found necessary the hard
// way on a real file: a bare in-place /C update showed NO visual change at
// all (a real annotation's baked /AP appearance stream wins over /C in
// MuPDF's real rendering), which is why buildNormalizedDoc instead deletes
// the color-changed annotation from the scratch copy and paintHighlights
// hand-paints its new color back in — the exact same mechanism proven safe
// for Delete above, reused here.
func TestRenderPage_ColorChangeShowsImmediatelyWithoutDisturbingOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rectA := types.NewRectangle(10, 10, 190, 190)
	quadA := types.QuadPoints{*types.NewQuadLiteralForRect(rectA)}
	red := color.SimpleColor{R: 1, G: 0, B: 0}
	annA := model.NewHighlightAnnotation(*rectA, 0, "", "", "", 0, &red, 0, 0, 0, "", nil, nil, "", "", quadA)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, annA, nil, false); err != nil {
		t.Fatalf("adding annotation A: %v", err)
	}

	rectB := types.NewRectangle(60, 60, 140, 140)
	quadB := types.QuadPoints{*types.NewQuadLiteralForRect(rectB)}
	blue := color.SimpleColor{R: 0, G: 0, B: 1}
	annB := model.NewHighlightAnnotation(*rectB, 0, "", "", "", 0, &blue, 0, 0, 0, "", nil, nil, "", "", quadB)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, annB, nil, false); err != nil {
		t.Fatalf("adding annotation B: %v", err)
	}

	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer doc.Close()

	var a, b *Highlight
	for _, h := range doc.Highlights {
		switch h.Color {
		case [3]float64{1, 0, 0}:
			a = h
		case [3]float64{0, 0, 1}:
			b = h
		}
	}
	if a == nil || b == nil {
		t.Fatalf("expected to find both highlights, got %+v", doc.Highlights)
	}

	const pageHeightPt = 200.0
	scale := baseRenderDPI / 72.0
	pixelAt := func(xPt, yPt float64) image.Point {
		return image.Pt(int(xPt*scale), int((pageHeightPt-yPt)*scale))
	}
	pxOnlyA := pixelAt(20, 20)     // inside A, outside B
	pxOverlap := pixelAt(100, 100) // inside both

	doc.SetHighlightColor(a, [3]float64{0, 1, 0}) // red -> green

	after, err := doc.RenderPage(1, 1.0)
	if err != nil {
		t.Fatalf("RenderPage after color change: %v", err)
	}
	afterImg, ok := after.(*image.RGBA)
	if !ok {
		t.Fatalf("expected *image.RGBA, got %T", after)
	}
	afterOnlyA := afterImg.RGBAAt(pxOnlyA.X, pxOnlyA.Y)
	afterOverlap := afterImg.RGBAAt(pxOverlap.X, pxOverlap.Y)

	if afterOnlyA.G <= afterOnlyA.R || afterOnlyA.G <= afterOnlyA.B || afterOnlyA.A == 0 {
		t.Errorf("expected A's own region to show green-dominant after SetHighlightColor, got %v", afterOnlyA)
	}
	// The real thing that matters for the overlap region: B's own blue must
	// still be part of what renders there (A's own new green blended with
	// B's still-live blue), not B having been silently blanked or
	// corrupted by whatever removed A's stale appearance from the scratch
	// copy — the exact failure mode a less careful fix could reintroduce.
	if afterOverlap.B <= afterOverlap.R || afterOverlap.A == 0 {
		t.Errorf("expected the overlap region to still show B's own blue undisturbed, got %v", afterOverlap)
	}
}

// TestDeleteHighlight_RebuildsDocAfterDelete confirms DeleteHighlight
// actually rebuilds d.doc (via rebuildDoc) from a normalized copy with the
// deletion applied, rather than leaving the previous doc (and its stale
// docPath scratch file) in place — the direct unit-level check behind the
// end-to-end test above. Superseded document_test.go's older
// TestDeleteHighlight_DropsStalePreviewDocument, which checked the
// previewDocument/dropPreviewDocument mechanism the always-normalize
// redesign replaced (see CLAUDE.md: rebuildDoc/buildNormalizedDoc/closeDoc).
func TestDeleteHighlight_RebuildsDocAfterDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(50, 50)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer doc.Close()

	docPathAfterOpen := doc.docPath
	if docPathAfterOpen == "" {
		t.Fatalf("expected Prepare to have normalized into a scratch docPath")
	}

	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight after Prepare, got %d", len(doc.Highlights))
	}
	if !doc.DeleteHighlight(doc.Highlights[0]) {
		t.Fatalf("DeleteHighlight reported not found")
	}

	if doc.docPath == "" {
		t.Errorf("expected rebuildDoc (via DeleteHighlight) to still have a scratch docPath")
	}
	if _, err := os.Stat(docPathAfterOpen); !os.IsNotExist(err) {
		t.Errorf("expected the pre-delete scratch file %q to be removed after rebuild, stat err: %v", docPathAfterOpen, err)
	}
}
