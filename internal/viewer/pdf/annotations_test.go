package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
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

// TestSaveHighlights_DoesNotRecolorNonHighlightKinds guards a real bug
// caught while adding the color picker: Color is only ever populated for
// Kind == "Highlight" (flattenHighlights only calls highlightGeometry for
// model.AnnHighLight), so an Underline/Strikeout/Squiggly/Note's in-memory
// Color sits at the Go zero value, [3]float64{} (black) — not that
// annotation's real /C. An earlier version of SaveHighlights rewrote /C
// for every ObjNr>0 highlight unconditionally, which would have silently
// blackened every such annotation's real color on the very next save.
func TestSaveHighlights_DoesNotRecolorNonHighlightKinds(t *testing.T) {
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
	if highlights[0].Color != [3]float64{} {
		t.Fatalf("expected Color to stay at its zero value for a non-Highlight kind, got %v", highlights[0].Color)
	}

	doc := &Document{path: path, Highlights: highlights}
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("SaveHighlights: %v", err)
	}

	// LoadHighlights itself never reads a non-Highlight's /C back (see
	// flattenHighlights), so it can't be used to detect this — read the
	// raw dict directly instead, the same way highlightGeometry does.
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		t.Fatalf("ReadContextFile: %v", err)
	}
	_, col := highlightGeometry(ctx.XRefTable, highlights[0].ObjNr)
	if want := [3]float64{0, 0, 1}; col != want {
		t.Errorf("expected the real Underline color %v to survive a save untouched, got %v", want, col)
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
