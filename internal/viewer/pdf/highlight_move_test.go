package pdf

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestTranslateHighlightGeometry_ShiftsEveryFieldItsOwnKindUses covers each
// geometry field translateHighlightGeometry knows about — Quads, Line,
// Rect, Vertices, CalloutTip — confirming it shifts exactly the ones a
// given Highlight actually has populated (the Go zero value for an unused
// field, e.g. Rect on a markup Highlight, must stay untouched: there's
// nothing meaningful there to shift).
func TestTranslateHighlightGeometry_ShiftsEveryFieldItsOwnKindUses(t *testing.T) {
	calloutTip := &[2]float64{5, 5}
	h := &Highlight{
		Kind:       "FreeText",
		Quads:      [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}},
		Line:       []float64{1, 2, 3, 4},
		Rect:       [4]float64{10, 10, 60, 60},
		Vertices:   [][2]float64{{1, 1}, {2, 2}, {3, 3}},
		CalloutTip: calloutTip,
	}

	translateHighlightGeometry(h, 100, 1000)

	wantQuad := [8]float64{100, 1010, 110, 1010, 100, 1000, 110, 1000}
	if h.Quads[0] != wantQuad {
		t.Errorf("Quads[0] = %v, want %v", h.Quads[0], wantQuad)
	}
	wantLine := []float64{101, 1002, 103, 1004}
	for i := range wantLine {
		if h.Line[i] != wantLine[i] {
			t.Errorf("Line = %v, want %v", h.Line, wantLine)
			break
		}
	}
	wantRect := [4]float64{110, 1010, 160, 1060}
	if h.Rect != wantRect {
		t.Errorf("Rect = %v, want %v", h.Rect, wantRect)
	}
	wantVertices := [][2]float64{{101, 1001}, {102, 1002}, {103, 1003}}
	for i := range wantVertices {
		if h.Vertices[i] != wantVertices[i] {
			t.Errorf("Vertices = %v, want %v", h.Vertices, wantVertices)
			break
		}
	}
	if calloutTip[0] != 105 || calloutTip[1] != 1005 {
		t.Errorf("CalloutTip = %v, want [105 1005]", *calloutTip)
	}
}

// TestTranslateHighlightGeometry_NilCalloutTipStaysNil confirms a plain
// text block (no callout) isn't given one by translation — a nil
// CalloutTip must stay nil, not become a zero-shifted-by-delta pointer.
func TestTranslateHighlightGeometry_NilCalloutTipStaysNil(t *testing.T) {
	h := &Highlight{Kind: "FreeText", Rect: [4]float64{0, 0, 10, 10}}
	translateHighlightGeometry(h, 5, 5)
	if h.CalloutTip != nil {
		t.Errorf("expected CalloutTip to stay nil, got %v", h.CalloutTip)
	}
}

// TestCanMoveHighlight covers the three cases canMoveHighlight's own doc
// comment describes: not yet saved (always movable), already saved with
// enough geometry to re-author (movable), and already saved without it —
// a foreign Polygon with no Vertices, the exact shape that made recoloring
// unsafe before SaveHighlights' own guard (see
// TestSaveHighlights_RecolorForeignPolygonDoesNotDeleteIt) — not movable.
func TestCanMoveHighlight(t *testing.T) {
	notYetSaved := &Highlight{Kind: "Square", Rect: [4]float64{0, 0, 10, 10}}
	if !canMoveHighlight(notYetSaved) {
		t.Error("expected a not-yet-saved (ObjNr == 0) highlight to always be movable")
	}

	savedWithGeometry := &Highlight{ObjNr: 1, Kind: "Square", Rect: [4]float64{0, 0, 10, 10}}
	if !canMoveHighlight(savedWithGeometry) {
		t.Error("expected an already-saved Square (enough geometry to re-author) to be movable")
	}

	foreignPolygon := &Highlight{ObjNr: 1, Kind: "Polygon", Rect: [4]float64{0, 0, 10, 10}} // no Vertices
	if canMoveHighlight(foreignPolygon) {
		t.Error("expected an already-saved Polygon with no Vertices NOT to be movable")
	}
}

// TestHighlightBoundsPt covers the three geometry shapes it unions/reads
// from, mirroring paintHighlights' own per-kind branches.
func TestHighlightBoundsPt(t *testing.T) {
	line := &Highlight{Kind: "Line", Line: []float64{10, 50, 40, 10}}
	if rect, ok := highlightBoundsPt(line); !ok || rect != [4]float64{10, 10, 40, 50} {
		t.Errorf("Line bounds = %v (ok=%v), want [10 10 40 50]", rect, ok)
	}

	rectKind := &Highlight{Kind: "Square", Rect: [4]float64{5, 5, 25, 15}}
	if rect, ok := highlightBoundsPt(rectKind); !ok || rect != [4]float64{5, 5, 25, 15} {
		t.Errorf("Square bounds = %v (ok=%v), want [5 5 25 15]", rect, ok)
	}

	multiQuad := &Highlight{Kind: "Highlight", Quads: [][8]float64{
		{0, 20, 10, 20, 0, 10, 10, 10},
		{0, 40, 10, 40, 0, 30, 10, 30},
	}}
	if rect, ok := highlightBoundsPt(multiQuad); !ok || rect != [4]float64{0, 10, 10, 40} {
		t.Errorf("multi-quad bounds = %v (ok=%v), want union [0 10 10 40]", rect, ok)
	}

	empty := &Highlight{Kind: "Note"}
	if _, ok := highlightBoundsPt(empty); ok {
		t.Error("expected ok=false for a highlight with no usable geometry at all")
	}
}

// TestMoveHighlight_UpdatesGeometryAndFlagsChanged confirms MoveHighlight's
// own direct effects: geometry actually shifts, and geometryMoved becomes
// true so SaveHighlights/buildNormalizedDoc know to treat this one as
// needing a delete-and-recreate on the next save/render.
func TestMoveHighlight_UpdatesGeometryAndFlagsChanged(t *testing.T) {
	h := &Highlight{Kind: "Square", Rect: [4]float64{0, 0, 10, 10}}
	d := &Document{cache: newPageCache(pageCacheCapacity), Highlights: []*Highlight{h}}

	d.MoveHighlight(h, 5, -3)

	if h.Rect != [4]float64{5, -3, 15, 7} {
		t.Errorf("Rect after move = %v, want [5 -3 15 7]", h.Rect)
	}
	if !h.geometryMoved {
		t.Error("expected geometryMoved to be true after MoveHighlight")
	}
	if _, _, _, geometryChanged := highlightDictChanges(h); !geometryChanged {
		t.Error("expected highlightDictChanges to report geometryChanged=true after a move")
	}
}

// TestSaveHighlights_MoveRoundTrips builds a real single-page PDF, adds a
// Square via this app's own Add path, saves it (so it has a real ObjNr,
// the "already-saved" case MoveHighlight's own rebuildDoc branch and
// SaveHighlights' delete-and-recreate scheme both exist for), moves it,
// saves again, and confirms a fresh reload sees the NEW position, not the
// original one — the same round-trip discipline
// TestSaveHighlights_ShapeAddRoundTrips already established for Add,
// extended to cover Move actually persisting through a real save, not
// just updating the in-memory struct.
func TestSaveHighlights_MoveRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0}, defaultShapeBorderWidthPt)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("initial SaveHighlights: %v", err)
	}
	if len(doc.Highlights) != 1 || doc.Highlights[0].ObjNr <= 0 {
		t.Fatalf("expected 1 already-saved highlight, got %+v", doc.Highlights)
	}

	h := doc.Highlights[0]
	if !canMoveHighlight(h) {
		t.Fatalf("expected the just-saved Square to be movable")
	}
	doc.MoveHighlight(h, 100, 50)

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights after move: %v", err)
	}

	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight after the move+save round trip, got %d", len(doc.Highlights))
	}
	got := doc.Highlights[0]
	want := [4]float64{110, 60, 160, 110}
	if got.Rect != want {
		t.Errorf("Rect after move+save+reload = %v, want %v", got.Rect, want)
	}
	if got.ObjNr <= 0 {
		t.Error("expected the moved highlight to still have a real ObjNr after reload")
	}
}

// TestSaveHighlights_MoveAfterSavePreservesVertices is Move's own
// counterpart to TestSaveHighlights_RecolorAfterSavePreservesVertices: an
// Ink stroke (not Square, which has no Vertices to lose) saved once, then
// moved in the same session, must keep its real freehand shape rather
// than degrading to a plain Rect outline (the same real bug recoloring
// hit, same root cause — see that test's own doc comment — just reached
// via Move's identical delete-and-recreate path instead of Color's).
func TestSaveHighlights_MoveAfterSavePreservesVertices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddInkShape(1, [][2]float64{{10, 10}, {30, 60}, {60, 20}, {90, 80}}, [3]float64{1, 0, 0}, defaultShapeBorderWidthPt)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("initial SaveHighlights: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight after initial save, got %d", len(doc.Highlights))
	}
	h := doc.Highlights[0]
	if len(h.Vertices) < 2 {
		t.Fatalf("Vertices lost after initial save, got %v", h.Vertices)
	}
	if !canMoveHighlight(h) {
		t.Fatalf("expected the just-saved Ink stroke to be movable")
	}

	doc.MoveHighlight(h, 5, 5)
	if len(h.Vertices) < 2 {
		t.Fatalf("Vertices lost after move (pre-save), got %v", h.Vertices)
	}

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights after move: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected exactly 1 highlight after move+save, got %d (duplicate added?)", len(doc.Highlights))
	}
	got := doc.Highlights[0]
	if len(got.Vertices) < 2 {
		t.Errorf("Vertices lost after move+save, got %v", got.Vertices)
	}
}

// TestSaveHighlights_MoveForeignPolygonDoesNotDeleteIt is Move's own
// counterpart to TestSaveHighlights_RecolorForeignPolygonDoesNotDeleteIt:
// confirms canMoveHighlight's guard actually prevents the same
// delete-with-nothing-to-put-back regression for geometry that the
// recolor fix needed for color. Since the UI (handleHighlightMoveHitTest)
// already refuses to start a move for a highlight canMoveHighlight
// rejects, this test calls MoveHighlight directly (bypassing that guard)
// to confirm SaveHighlights' own recreate-computation is ALSO safe on its
// own, not just reachable-in-practice.
func TestSaveHighlights_MoveForeignPolygonDoesNotDeleteIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.NewRectangle(10, 10, 60, 60)
	vertices := types.NewNumberArray(10, 10, 60, 10, 35, 60)
	red := color.SimpleColor{R: 1, G: 0, B: 0}
	ann := model.NewPolygonAnnotation(*rect, 0, "", "", "", 0, &red, "", nil, nil, "", "",
		vertices, nil, nil, nil, nil, 1, model.BSSolid, false, 0)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test polygon: %v", err)
	}

	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(doc.Highlights))
	}
	h := doc.Highlights[0]

	doc.MoveHighlight(h, 20, 20) // bypasses canMoveHighlight deliberately -- see doc comment
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}
	doc.Close()

	fresh, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare after save: %v", err)
	}
	defer fresh.Close()
	if len(fresh.Highlights) != 1 {
		t.Fatalf("expected the polygon to survive the move attempt, got %d highlights (it vanished — regression)", len(fresh.Highlights))
	}
}

// TestHandleHighlightMoveHitTestAndMoved exercises the real view-layer
// wiring (not just MoveHighlight/canMoveHighlight in isolation): selects a
// highlight, confirms handleHighlightMoveHitTest reports a hit with the
// right widget-space bounds for a point inside it, then drives
// handleHighlightMoved the same way a real move-drag's DragEnd would and
// confirms the highlight's own PDF-space geometry actually shifted by the
// right amount. writeMinimalPDF's page is 200x200 and the widget size here
// is pinned 1:1 to it, so widget points equal PDF points on X and are
// simply flipped (pageH - y) on Y — easy to hand-verify.
func TestHandleHighlightMoveHitTestAndMoved(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH))

	h := v.doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0}, defaultShapeBorderWidthPt)
	v.doc.SetSelectedHighlight(h)

	// A point inside the Square's own PDF rect [10,10]-[60,60]: (30,30) PDF.
	inside := fyne.NewPos(30, float32(pageH)-30)
	tl, br, ok := v.handleHighlightMoveHitTest(1, inside, widgetSize)
	if !ok {
		t.Fatal("expected a hit on the selected Square at a point inside its own Rect")
	}
	wantTL := fyne.NewPos(10, float32(pageH)-60)
	wantBR := fyne.NewPos(60, float32(pageH)-10)
	if tl != wantTL || br != wantBR {
		t.Errorf("hit test bounds = %v/%v, want %v/%v", tl, br, wantTL, wantBR)
	}

	// A point well outside it must miss.
	outside := fyne.NewPos(150, float32(pageH)-150)
	if _, _, ok := v.handleHighlightMoveHitTest(1, outside, widgetSize); ok {
		t.Error("expected no hit for a point outside the selected Square's own Rect")
	}

	// Drive the move itself: widget-space delta (+20 X, -5 Y) is PDF-space
	// (+20, +5) once Y is un-flipped (1:1 scale).
	start := inside
	end := fyne.NewPos(inside.X+20, inside.Y-5)
	v.handleHighlightMoved(1, start, end, widgetSize)

	want := [4]float64{30, 15, 80, 65}
	if h.Rect != want {
		t.Errorf("Rect after handleHighlightMoved = %v, want %v", h.Rect, want)
	}
}
