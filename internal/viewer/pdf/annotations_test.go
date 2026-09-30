package pdf

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/color"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func makeTestAnnotation(contents string) model.AnnotationRenderer {
	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	return model.NewHighlightAnnotation(*rect, 0, contents, "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
}

func TestFlattenHighlights_SortsByPageAndKeepsOnlyKnownKinds(t *testing.T) {
	pgAnnots := map[int]model.PgAnnots{
		3: {
			model.AnnHighLight: model.Annot{Map: model.AnnotMap{1: makeTestAnnotation("page three highlight")}},
		},
		1: {
			model.AnnUnderline: model.Annot{Map: model.AnnotMap{2: makeTestAnnotation("page one underline")}},
			// Link annotations aren't "highlights or notes" and must be excluded.
			model.AnnLink: model.Annot{Map: model.AnnotMap{3: makeTestAnnotation("a link, not a highlight")}},
		},
	}

	got := flattenHighlights(&model.XRefTable{}, pgAnnots)

	if len(got) != 2 {
		t.Fatalf("expected 2 highlights (Link excluded), got %d: %+v", len(got), got)
	}
	if got[0].Page != 1 || got[1].Page != 3 {
		t.Errorf("expected results sorted by page (1, 3), got (%d, %d)", got[0].Page, got[1].Page)
	}
	if got[0].Kind != "Underline" {
		t.Errorf("expected Kind %q, got %q", "Underline", got[0].Kind)
	}
	if got[0].Contents != "page one underline" {
		t.Errorf("expected Contents %q, got %q", "page one underline", got[0].Contents)
	}
}

// TestLoadHighlights_LineGeometryAndEndStyles confirms a real Line
// annotation (an arrow, in this case) round-trips its /L endpoints, /LE
// ending styles, and /C color back through LoadHighlights the same way a
// Highlight's /QuadPoints and /C already did — the same read-back gap
// highlightGeometry's doc comment explains applies to Line too (pdfcpu's
// own Annotation() never populates LineAnnotation.P1/P2 from an existing
// PDF), worked around by lineGeometry re-dereferencing the raw dict.
func TestLoadHighlights_LineGeometryAndEndStyles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(120, 60)
	p1, p2 := types.Point{X: 10, Y: 10}, types.Point{X: 100, Y: 50}
	none, closedArrow := model.LENone, model.LEClosedArrow
	red := color.SimpleColor{R: 1, G: 0, B: 0}
	ann := model.NewLineAnnotation(*rect, 0, "", "", "", 0, &red, "", nil, nil, "", "",
		p1, p2, &none, &closedArrow, 0, 0, 0, nil, nil, false, false, 0, 0, nil, 0, 0)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test line: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 || highlights[0].Kind != "Line" {
		t.Fatalf("expected 1 Line highlight, got %+v", highlights)
	}
	h := highlights[0]
	if want := [3]float64{1, 0, 0}; h.Color != want {
		t.Errorf("expected Line color %v, got %v", want, h.Color)
	}
	if want := [2]string{"None", "ClosedArrow"}; h.LineEndStyle != want {
		t.Errorf("expected LineEndStyle %v, got %v", want, h.LineEndStyle)
	}
	if len(h.Line) != 4 {
		t.Fatalf("expected 4 line coordinates, got %v", h.Line)
	}
	if h.Line[0] != 10 || h.Line[1] != 10 || h.Line[2] != 100 || h.Line[3] != 50 {
		t.Errorf("expected Line [10 10 100 50] (within Rect, no clamping needed), got %v", h.Line)
	}
}

func TestFlattenHighlights_NoAnnotationsReturnsEmpty(t *testing.T) {
	got := flattenHighlights(&model.XRefTable{}, map[int]model.PgAnnots{})
	if len(got) != 0 {
		t.Errorf("expected no highlights, got %d", len(got))
	}
}

// writeMinimalPDF writes a hand-rolled single blank-page PDF (no xref
// stream, a plain classic xref table with correctly computed byte offsets)
// — good enough for pdfcpu to read/write, not for go-fitz to render, but
// this package's write path (SaveHighlightCaptions) never touches go-fitz.
func writeMinimalPDF(t *testing.T, path string) {
	t.Helper()

	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> >>",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1) // 1-indexed; [0] is the free list head, unused
	for i, body := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objs)+1)
	buf.WriteString("0000000000 65535 f \n") // each xref entry line must be exactly 20 bytes
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xrefOffset)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing minimal test PDF: %v", err)
	}
}

// TestSaveHighlights_CaptionRoundTrips builds a real single-page PDF, adds
// a real highlight annotation via pdfcpu's own Add path (not a fabricated
// in-memory Highlight), captions it via the exact path the UI uses
// (SetHighlightCaption + SaveHighlights), and confirms a fresh LoadHighlights
// of the saved file reads the caption back — proving the low-level
// Dict.Update("Contents", ...) mutation in SaveHighlights actually reaches
// disk and round-trips through pdfcpu's own reader, not just that it
// compiles.
func TestSaveHighlights_CaptionRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	h := highlights[0]
	if h.ObjNr <= 0 {
		t.Fatalf("expected a positive ObjNr, got %d", h.ObjNr)
	}

	doc := &Document{path: path, Highlights: highlights}
	doc.SetHighlightCaption(h, "my caption")

	outPath := filepath.Join(dir, "captioned.pdf")
	if err := doc.SaveHighlights(outPath); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	reloaded, err := LoadHighlights(outPath)
	if err != nil {
		t.Fatalf("LoadHighlights (reloaded): %v", err)
	}
	if len(reloaded) != 1 {
		t.Fatalf("expected 1 highlight after reload, got %d", len(reloaded))
	}
	if got := reloaded[0].Contents; got != "my caption" {
		t.Errorf("expected caption %q to round-trip, got %q", "my caption", got)
	}
}

// TestSaveHighlights_AddRoundTrips exercises the other new write path:
// AddHighlight's brand-new, never-saved Highlight (ObjNr == 0) must come
// out through model.NewHighlightAnnotation + pdfcpu.AddAnnotationsMap as a
// real annotation with the right page, geometry, and color once saved.
func TestSaveHighlights_AddRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	quads := [][8]float64{{10, 20, 110, 20, 10, 10, 110, 10}}
	doc.AddHighlight(1, quads, [3]float64{1, 0, 0}, "new one")

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	// Overwrote d.path, so SaveHighlights must have reloaded doc.Highlights
	// in place — the whole point of the reload being tied to outputPath ==
	// d.path (see SaveHighlights' doc comment) is that a second save from
	// this same *Document, right after the first, must not re-add it.
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected doc.Highlights to hold 1 reloaded highlight, got %d", len(doc.Highlights))
	}
	if doc.Highlights[0].ObjNr <= 0 {
		t.Fatalf("expected the reloaded highlight to have a real ObjNr, got %d", doc.Highlights[0].ObjNr)
	}

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("second SaveHighlights: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected still 1 highlight after a second save (no duplicate add), got %d", len(doc.Highlights))
	}

	got := doc.Highlights[0]
	if got.Page != 1 || got.Contents != "new one" || got.Color != [3]float64{1, 0, 0} {
		t.Errorf("expected page 1, caption %q, color %v; got page %d, caption %q, color %v",
			"new one", [3]float64{1, 0, 0}, got.Page, got.Contents, got.Color)
	}
}

// TestSaveHighlights_ShapeAddRoundTrips covers the three shape kinds this
// app can author itself (AddRectShape's Square/Circle, AddLineShape's
// Line — see newAnnotationForShape): each one added in memory, saved,
// and reloaded fresh from disk, confirming both the right pdfcpu
// annotation type got written (Kind survives the round trip) and its own
// geometry/color came back correctly.
func TestSaveHighlights_ShapeAddRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0})
	doc.AddRectShape(1, "Circle", [4]float64{70, 70, 150, 130}, [3]float64{0, 1, 0})
	doc.AddLineShape(1, []float64{20, 30, 80, 90}, [2]string{"None", "ClosedArrow"}, [3]float64{0, 0, 1})

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	if len(doc.Highlights) != 3 {
		t.Fatalf("expected 3 reloaded shapes, got %d: %+v", len(doc.Highlights), doc.Highlights)
	}

	byKind := map[string]*Highlight{}
	for _, h := range doc.Highlights {
		if h.ObjNr <= 0 {
			t.Errorf("expected a real ObjNr for reloaded kind=%s, got %d", h.Kind, h.ObjNr)
		}
		byKind[h.Kind] = h
	}

	square := byKind["Square"]
	if square == nil {
		t.Fatalf("expected a reloaded Square, got none among %+v", doc.Highlights)
	}
	if square.Rect != [4]float64{10, 10, 60, 60} {
		t.Errorf("Square.Rect = %v, want [10 10 60 60]", square.Rect)
	}

	circle := byKind["Circle"]
	if circle == nil {
		t.Fatalf("expected a reloaded Circle, got none among %+v", doc.Highlights)
	}
	if circle.Rect != [4]float64{70, 70, 150, 130} {
		t.Errorf("Circle.Rect = %v, want [70 70 150 130]", circle.Rect)
	}

	line := byKind["Line"]
	if line == nil {
		t.Fatalf("expected a reloaded Line, got none among %+v", doc.Highlights)
	}
	if len(line.Line) != 4 || line.Line[0] != 20 || line.Line[1] != 30 || line.Line[2] != 80 || line.Line[3] != 90 {
		t.Errorf("Line.Line = %v, want [20 30 80 90]", line.Line)
	}
	if line.LineEndStyle != [2]string{"None", "ClosedArrow"} {
		t.Errorf("Line.LineEndStyle = %v, want [None ClosedArrow]", line.LineEndStyle)
	}
	if line.Color != [3]float64{0, 0, 1} {
		t.Errorf("Line.Color = %v, want [0 0 1]", line.Color)
	}
}

// TestSaveHighlights_PolygonAndFreeTextRoundTrips covers the two newest
// TestNewAnnotationForShape_FreeTextAlwaysHasBorderWidth confirms both a
// plain text block and a speech bubble are authored with the same
// defaultShapeBorderWidthPt — deliberately NOT 0 for a plain text block,
// even though that would better match Preview's own borderless style,
// because it was confirmed empirically to have no effect on MuPDF's own
// rendering (it draws a border regardless of /BS) — see
// newAnnotationForShape's own doc comment on why that idea was tried and
// reverted, not just never attempted.
func TestNewAnnotationForShape_FreeTextAlwaysHasBorderWidth(t *testing.T) {
	plain := &Highlight{Kind: "FreeText", Rect: [4]float64{0, 0, 100, 50}, Contents: "hi"}
	ann, ok := newAnnotationForShape(plain).(model.FreeTextAnnotation)
	if !ok {
		t.Fatalf("expected a model.FreeTextAnnotation, got %T", newAnnotationForShape(plain))
	}
	if ann.BorderWidth != defaultShapeBorderWidthPt {
		t.Errorf("plain text block BorderWidth = %v, want %v", ann.BorderWidth, defaultShapeBorderWidthPt)
	}

	tip := [2]float64{-5, -5}
	bubble := &Highlight{Kind: "FreeText", Rect: [4]float64{0, 0, 100, 50}, Contents: "hi", CalloutTip: &tip}
	ann2, ok := newAnnotationForShape(bubble).(model.FreeTextAnnotation)
	if !ok {
		t.Fatalf("expected a model.FreeTextAnnotation, got %T", newAnnotationForShape(bubble))
	}
	if ann2.BorderWidth != defaultShapeBorderWidthPt {
		t.Errorf("speech bubble BorderWidth = %v, want %v", ann2.BorderWidth, defaultShapeBorderWidthPt)
	}
}

// TestSaveHighlights_PolygonAndFreeTextRoundTrips covers the two newest
// authored kinds: a Star/Hexagon (always written as a generic Polygon —
// see AddPolygonShape's own doc comment, PDF has no dedicated subtype for
// either) and a plain text block plus a speech bubble (both FreeText, the
// callout line being the only difference — see AddTextShape). Confirms
// each survives a real save + fresh reload with the right Kind, Rect, and
// (for FreeText) caption text.
func TestSaveHighlights_PolygonAndFreeTextRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	star := starVertices(50, 50, 40, 40)
	doc.AddPolygonShape(1, star, [3]float64{1, 1, 0})
	tip := [2]float64{5, 5}
	doc.AddTextShape(1, [4]float64{10, 10, 110, 60}, nil, "a plain text block", [3]float64{0, 0, 0})
	doc.AddTextShape(1, [4]float64{120, 70, 190, 130}, &tip, "speech bubble text", [3]float64{0, 0, 1})

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	if len(doc.Highlights) != 3 {
		t.Fatalf("expected 3 reloaded shapes, got %d: %+v", len(doc.Highlights), doc.Highlights)
	}

	var polygon *Highlight
	var freeTexts []*Highlight
	for _, h := range doc.Highlights {
		if h.ObjNr <= 0 {
			t.Errorf("expected a real ObjNr for reloaded kind=%s, got %d", h.Kind, h.ObjNr)
		}
		switch h.Kind {
		case "Polygon":
			polygon = h
		case "FreeText":
			freeTexts = append(freeTexts, h)
		}
	}

	if polygon == nil {
		t.Fatalf("expected a reloaded Polygon (the star), got none among %+v", doc.Highlights)
	}
	wantMinX, wantMinY, wantMaxX, wantMaxY := verticesBoundsPt(star)
	want := [4]float64{wantMinX, wantMinY, wantMaxX, wantMaxY}
	for i := range want {
		if math.Abs(polygon.Rect[i]-want[i]) > 0.01 { // PDF's own text representation loses a little precision on round trip
			t.Errorf("Polygon.Rect = %v, want %v (the star's own bounding box)", polygon.Rect, want)
			break
		}
	}

	if len(freeTexts) != 2 {
		t.Fatalf("expected 2 reloaded FreeText annotations, got %d", len(freeTexts))
	}
	gotContents := map[string]bool{}
	for _, ft := range freeTexts {
		gotContents[ft.Contents] = true
	}
	if !gotContents["a plain text block"] || !gotContents["speech bubble text"] {
		t.Errorf("expected both FreeText captions reloaded, got %v", gotContents)
	}
}

// TestSaveHighlights_DeleteRoundTrips confirms DeleteHighlight's queued
// ObjNr actually reaches pdfcpu.RemoveAnnotations and the annotation is
// gone from disk after SaveHighlights.
func TestSaveHighlights_DeleteRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}

	doc := &Document{path: path, Highlights: highlights, cache: newPageCache(pageCacheCapacity)}
	if !doc.DeleteHighlight(highlights[0]) {
		t.Fatalf("DeleteHighlight reported not found")
	}
	if len(doc.Highlights) != 0 {
		t.Fatalf("expected DeleteHighlight to remove it from the in-memory list immediately, got %d left", len(doc.Highlights))
	}

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	reloaded, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights (reloaded): %v", err)
	}
	if len(reloaded) != 0 {
		t.Fatalf("expected the highlight to be gone from disk, got %d", len(reloaded))
	}
}

// TestSaveHighlights_ColorRoundTrips confirms SetHighlightColor's /C
// rewrite (added alongside the color-picker UI) actually reaches disk.
func TestSaveHighlights_ColorRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}

	doc := &Document{path: path, Highlights: highlights, cache: newPageCache(pageCacheCapacity)}
	doc.SetHighlightColor(highlights[0], [3]float64{0, 1, 0})

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	reloaded, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights (reloaded): %v", err)
	}
	if len(reloaded) != 1 {
		t.Fatalf("expected 1 highlight after reload, got %d", len(reloaded))
	}
	if got := reloaded[0].Color; got != [3]float64{0, 1, 0} {
		t.Errorf("expected color %v to round-trip, got %v", [3]float64{0, 1, 0}, got)
	}
}

// TestSaveHighlights_OverwriteClearsRenderCache guards a real bug: a page
// already rendered (and cached) before Save to PDF overwrites d.path kept
// showing its pre-save render — missing whatever the save itself had just
// changed on disk (a healed xref, a new color, ...) — until the tab was
// fully closed and reopened, because nothing was clearing the cache for
// an overwrite the way AddHighlight/DeleteHighlight/SetHighlightColor
// already do for the one page they know they changed. An overwrite can
// change what's on ANY page (OptimizeContext's flatten isn't scoped to
// pages with pending highlight edits), so every cached page must be
// dropped, not just one.
func TestSaveHighlights_OverwriteClearsRenderCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	key := cacheKey(1, 1.0)
	doc.cache.Put(key, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if _, ok := doc.cache.Get(key); !ok {
		t.Fatalf("test setup failed: expected the seeded cache entry to be there")
	}

	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	if _, ok := doc.cache.Get(key); ok {
		t.Errorf("expected SaveHighlights to clear the render cache on an overwrite, but the stale entry is still there")
	}
}

// TestSaveHighlights_SaveAsLeavesRenderCacheAlone confirms the above is
// scoped correctly: a Save-As (outputPath != d.path) is an export, not a
// commit — d.path's own bytes are untouched, so whatever's cached from
// them is still valid and must not be thrown away.
func TestSaveHighlights_SaveAsLeavesRenderCacheAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	key := cacheKey(1, 1.0)
	doc.cache.Put(key, image.NewRGBA(image.Rect(0, 0, 1, 1)))

	outPath := filepath.Join(dir, "copy.pdf")
	if err := doc.SaveHighlights(outPath); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	if _, ok := doc.cache.Get(key); !ok {
		t.Errorf("expected a Save-As to leave the render cache untouched, but the entry is gone")
	}
}

// TestRemoveAnnotationsRepairingIfNeeded_RecoversFromDanglingReference
// guards the real bug found on a real file: pdfcpu.RemoveAnnotations
// validates the FULL object graph reachable from a page's annotations
// before allowing removal — including inside a dict key this app has no
// special knowledge of, exactly like Preview's own AAPL:AKExtras private
// annotation metadata — and hard-fails on the first indirect reference
// that doesn't resolve to anything, with no lenient mode. Simulates that
// exact shape (an opaque key holding a reference to a now-missing
// object) rather than depending on a real AAPL blob, which would need a
// huge fixture to reproduce faithfully.
func TestRemoveAnnotationsRepairingIfNeeded_RecoversFromDanglingReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(50, 50)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	objNr := highlights[0].ObjNr

	ctx, err := api.ReadContextFile(path)
	if err != nil {
		t.Fatalf("ReadContextFile: %v", err)
	}
	dict, err := ctx.XRefTable.DereferenceDict(*types.NewIndirectRef(objNr, 0))
	if err != nil || dict == nil {
		t.Fatalf("dereferencing test annotation: %v", err)
	}
	dict.Update("Test:DanglingRef", *types.NewIndirectRef(999999, 0))

	// Confirm the scenario actually reproduces the bug before trusting the
	// fix: a bare RemoveAnnotations call must fail here.
	if _, err := pdfcpu.RemoveAnnotations(ctx, nil, nil, []int{objNr}, false); err == nil {
		t.Fatalf("expected a bare RemoveAnnotations to fail against a dangling reference, it succeeded")
	}

	if err := removeAnnotationsRepairingIfNeeded(ctx, []int{objNr}); err != nil {
		t.Fatalf("removeAnnotationsRepairingIfNeeded: %v", err)
	}
}

// TestHighlightDictChanges guards the gate SaveHighlights uses to decide
// whether an existing highlight's dict needs touching at all — see
// SaveHighlights' own doc comment on why an untouched dict must stay
// byte-for-byte untouched, not just value-equal after a round trip.
func TestHighlightDictChanges(t *testing.T) {
	base := &Highlight{Kind: "Highlight", Contents: "same", Color: [3]float64{1, 0, 0}}
	base.origContents, base.origColor = base.Contents, base.Color

	if c, col := highlightDictChanges(base); c || col {
		t.Errorf("expected no changes for an untouched highlight, got contentsChanged=%v colorChanged=%v", c, col)
	}

	withNewCaption := *base
	withNewCaption.Contents = "different"
	if c, col := highlightDictChanges(&withNewCaption); !c || col {
		t.Errorf("expected only contentsChanged for a caption edit, got contentsChanged=%v colorChanged=%v", c, col)
	}

	withNewColor := *base
	withNewColor.Color = [3]float64{0, 1, 0}
	if c, col := highlightDictChanges(&withNewColor); c || !col {
		t.Errorf("expected only colorChanged for a color edit, got contentsChanged=%v colorChanged=%v", c, col)
	}

	// Note isn't a hasPaintedColor kind, so a Color/origColor mismatch on
	// one must never report colorChanged=true — its Color is meaningless
	// (Go zero value, not a real /C) in the first place.
	noteWithColorDrift := &Highlight{Kind: "Note", Color: [3]float64{1, 1, 1}}
	if _, col := highlightDictChanges(noteWithColorDrift); col {
		t.Errorf("expected colorChanged=false for Note even with a Color/origColor mismatch, got true")
	}
}

// TestLoadHighlights_SnapshotsOrigContentsAndColor confirms flattenHighlights
// actually populates origContents/origColor to match what was just loaded
// — if it didn't, highlightDictChanges would report every highlight as
// changed on every save, defeating the whole point of the gate above.
func TestLoadHighlights_SnapshotsOrigContentsAndColor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	red := color.SimpleColor{R: 1, G: 0, B: 0}
	ann := model.NewHighlightAnnotation(*rect, 0, "loaded caption", "", "", 0, &red, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 {
		t.Fatalf("expected 1 highlight, got %d", len(highlights))
	}
	h := highlights[0]
	if h.origContents != h.Contents {
		t.Errorf("expected origContents %q to match loaded Contents %q", h.origContents, h.Contents)
	}
	if h.origColor != h.Color {
		t.Errorf("expected origColor %v to match loaded Color %v", h.origColor, h.Color)
	}
}

// TestSaveHighlights_UnderlineColorPaints confirms the fix that lets
// paintHighlights draw Underline/Strikeout/Squiggly too (see
// paintableMarkupKinds): loading a real Underline annotation must now
// populate Color and Quads from its own /QuadPoints and /C, the same way
// Highlight always has, and saving it back must leave that real color
// untouched rather than losing it.
func TestSaveHighlights_UnderlineColorPaints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	blue := color.SimpleColor{R: 0, G: 0, B: 1}
	ann := model.NewUnderlineAnnotation(*rect, 0, "", "", "", 0, &blue, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test underline: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 || highlights[0].Kind != "Underline" {
		t.Fatalf("expected 1 Underline highlight, got %+v", highlights)
	}
	if want := [3]float64{0, 0, 1}; highlights[0].Color != want {
		t.Fatalf("expected Underline's real color %v to be read back, got %v", want, highlights[0].Color)
	}
	if len(highlights[0].Quads) != 1 {
		t.Fatalf("expected Underline's real geometry to be read back, got %d quads", len(highlights[0].Quads))
	}

	doc := &Document{path: path, Highlights: highlights, cache: newPageCache(pageCacheCapacity)}
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	reloaded, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights (reloaded): %v", err)
	}
	if len(reloaded) != 1 {
		t.Fatalf("expected 1 highlight after reload, got %d", len(reloaded))
	}
	if got := reloaded[0].Color; got != [3]float64{0, 0, 1} {
		t.Errorf("expected the real Underline color %v to survive a save, got %v", [3]float64{0, 0, 1}, got)
	}
}

// TestSaveHighlights_DoesNotRecolorNoteKind is what
// TestSaveHighlights_UnderlineColorPaints (above) replaced now that
// Underline is a real, correctly-colored case rather than a "must not get
// blackened" one: Color is only ever populated for a paintableMarkupKinds
// kind (flattenHighlights only calls highlightGeometry for those), so a
// "Note" (Text/Popup)'s in-memory Color sits at the Go zero value,
// [3]float64{} (black) — not that annotation's real /C. SaveHighlights must
// keep gating its /C rewrite on paintableMarkupKinds, or a Note's real
// color would get silently blackened on the very next save.
func TestSaveHighlights_DoesNotRecolorNoteKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(100, 10)
	blue := color.SimpleColor{R: 0, G: 0, B: 1}
	ann := model.NewTextAnnotation(*rect, 0, "", "", "", 0, &blue, "", nil, nil, "", "", 0, 0, 0, false, "")
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test note: %v", err)
	}

	highlights, err := LoadHighlights(path)
	if err != nil {
		t.Fatalf("LoadHighlights: %v", err)
	}
	if len(highlights) != 1 || highlights[0].Kind != "Note" {
		t.Fatalf("expected 1 Note highlight, got %+v", highlights)
	}
	if highlights[0].Color != [3]float64{} {
		t.Fatalf("expected Color to stay at its zero value for Note, got %v", highlights[0].Color)
	}

	doc := &Document{path: path, Highlights: highlights, cache: newPageCache(pageCacheCapacity)}
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	// LoadHighlights itself never reads a Note's /C back (see
	// flattenHighlights), so it can't be used to detect this — read the
	// raw dict directly instead, the same way highlightGeometry does.
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		t.Fatalf("ReadContextFile: %v", err)
	}
	_, col := highlightGeometry(ctx.XRefTable, highlights[0].ObjNr)
	if want := [3]float64{0, 0, 1}; col != want {
		t.Errorf("expected the real Note color %v to survive a save untouched, got %v", want, col)
	}
}

// TestHighlightAt covers click-to-select's hit-testing: page mismatches,
// misses, and overlapping highlights (where the last-added, drawn-on-top
// one must win — see HighlightAt's own doc comment).
func TestHighlightAt(t *testing.T) {
	first := &Highlight{Page: 1, Kind: "Highlight", Quads: [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}}
	second := &Highlight{Page: 1, Kind: "Highlight", Quads: [][8]float64{{5, 10, 15, 10, 5, 0, 15, 0}}}
	otherPage := &Highlight{Page: 2, Kind: "Highlight", Quads: [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}}
	d := &Document{Highlights: []*Highlight{first, second, otherPage}}

	if got := d.HighlightAt(1, 12, 5); got != second {
		t.Errorf("expected a point only inside the second (overlapping) highlight to hit it, got %v", got)
	}
	if got := d.HighlightAt(1, 7, 5); got != second {
		t.Errorf("expected the overlap region to hit the last-added (topmost) highlight, got %v", got)
	}
	if got := d.HighlightAt(1, 1, 5); got != first {
		t.Errorf("expected a point only inside the first highlight to hit it, got %v", got)
	}
	if got := d.HighlightAt(1, 100, 100); got != nil {
		t.Errorf("expected a point outside every quad to miss, got %v", got)
	}
	if got := d.HighlightAt(2, 1, 5); got != otherPage {
		t.Errorf("expected page 2's own highlight to hit on page 2, got %v", got)
	}
	if got := d.HighlightAt(1, 1, 5+100); got != nil {
		// Sanity: a point on the RIGHT page but wrong Y should still miss.
		t.Errorf("expected a point outside the quad's Y range to miss, got %v", got)
	}
}

// TestHighlightAt_GenericRectKind confirms click-select works for a shape
// kind with no per-kind geometry parsing at all (Square, Circle, Polygon,
// PolyLine, Ink, Stamp, FreeText, Caret — see genericRectKinds), falling
// back to its overall Rect the same way Quad-based kinds fall back to
// their bounding box.
func TestHighlightAt_GenericRectKind(t *testing.T) {
	square := &Highlight{Page: 1, Kind: "Square", Rect: [4]float64{10, 10, 50, 50}}
	stamp := &Highlight{Page: 1, Kind: "Stamp", Rect: [4]float64{}} // malformed/unreadable /Rect
	d := &Document{Highlights: []*Highlight{square, stamp}}

	if got := d.HighlightAt(1, 20, 20); got != square {
		t.Errorf("expected a point inside the Square's Rect to hit it, got %v", got)
	}
	if got := d.HighlightAt(1, 100, 100); got != nil {
		t.Errorf("expected a point outside the Square's Rect to miss, got %v", got)
	}
	if got := d.HighlightAt(1, 0, 0); got != nil {
		t.Errorf("expected a zero-value Rect (unreadable /Rect) to never hit, got %v", got)
	}
}

// TestDeleteHighlight_WorksForGenericRectKind confirms Delete doesn't need
// any kind-specific handling: it only ever needs a Highlight's ObjNr, so
// a shape kind this app has no geometry-parsing or paint routine for at
// all (Stamp here) deletes exactly the same way a Highlight does.
func TestDeleteHighlight_WorksForGenericRectKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	rect := types.RectForDim(50, 50)
	ann := model.NewSquareAnnotation(*rect, 0, "", "", "", 0, nil, "", nil, nil, "", "", nil, 0, 0, 0, 0, 0, model.BSSolid, false, 0)
	if err := api.AddAnnotationsFile(path, path, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding Square annotation: %v", err)
	}

	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer doc.Close()

	if len(doc.Highlights) != 1 || doc.Highlights[0].Kind != "Square" {
		t.Fatalf("expected 1 Square highlight after Prepare, got %+v", doc.Highlights)
	}
	if !doc.DeleteHighlight(doc.Highlights[0]) {
		t.Fatalf("DeleteHighlight reported not found")
	}
	if len(doc.Highlights) != 0 {
		t.Errorf("expected the Square highlight gone after delete, got %+v", doc.Highlights)
	}
}

// TestAddHighlight_KeepsHighlightsPageSorted guards a real bug found via
// manual testing: the Highlights panel's list just displays d.Highlights
// in whatever order it's in, and a newly appended highlight always landed
// at the end of that slice regardless of its own page — so drawing one on
// an earlier page (e.g. page 13, after scrolling back from page 77) showed
// up in the list after every higher-numbered page's entry.
func TestAddHighlight_KeepsHighlightsPageSorted(t *testing.T) {
	d := &Document{cache: newPageCache(pageCacheCapacity)}
	quad := [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}

	d.AddHighlight(77, quad, defaultHighlightColor, "")
	d.AddHighlight(56, quad, defaultHighlightColor, "")
	d.AddHighlight(13, quad, defaultHighlightColor, "")
	d.AddHighlight(56, quad, defaultHighlightColor, "second on 56")

	got := make([]int, len(d.Highlights))
	for i, h := range d.Highlights {
		got[i] = h.Page
	}
	want := []int{13, 56, 56, 77}
	if len(got) != len(want) {
		t.Fatalf("got %v pages, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Highlights[%d].Page = %d, want %d (full order: %v)", i, got[i], want[i], got)
		}
	}
	// The two page-56 entries must keep their relative add order (stable
	// sort), not swap places.
	if d.Highlights[1].Contents != "" || d.Highlights[2].Contents != "second on 56" {
		t.Errorf("expected the two page-56 entries to keep add order, got captions %q then %q",
			d.Highlights[1].Contents, d.Highlights[2].Contents)
	}
}
