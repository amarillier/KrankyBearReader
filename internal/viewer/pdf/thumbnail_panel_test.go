package pdf

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

// writeThreePagePDF is writeMinimalPDF with three blank 200x200 pages.
func writeThreePagePDF(t *testing.T, path string) {
	t.Helper()
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, body := range objs {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objs)+1, xrefOffset)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newThreePageView(t *testing.T) *view {
	t.Helper()
	_ = test.NewApp()
	path := filepath.Join(t.TempDir(), "three.pdf")
	writeThreePagePDF(t, path)
	doc, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	v := &view{win: win, doc: doc, currentPage: 1, zoomLevel: -1, highlightColor: defaultHighlightColor}
	v.build()
	t.Cleanup(func() { _ = v.doc.Close() })
	return v
}

func TestRenderThumbnail_SizeCachingAndInvalidation(t *testing.T) {
	v := newThreePageView(t)
	d := v.doc
	img, err := d.RenderThumbnail(2, thumbnailWidthPx)
	if err != nil {
		t.Fatalf("RenderThumbnail: %v", err)
	}
	if w := img.Bounds().Dx(); w < thumbnailWidthPx-3 || w > thumbnailWidthPx+3 {
		t.Errorf("thumbnail width = %d, want about %d", w, thumbnailWidthPx)
	}
	again, _ := d.RenderThumbnail(2, thumbnailWidthPx)
	if again != img {
		t.Error("expected the second request to be served from the thumbnail cache")
	}
	before := countNonWhitePixelsForTest(img)

	// A real annotation change must drop the cached thumbnail, and the
	// re-render must show it.
	d.AddRectShape(2, "Square", [4]float64{20, 20, 180, 180}, [3]float64{1, 0, 0}, 4)
	fresh, _ := d.RenderThumbnail(2, thumbnailWidthPx)
	if fresh == img {
		t.Fatal("expected AddRectShape to invalidate the cached thumbnail")
	}
	if after := countNonWhitePixelsForTest(fresh); after <= before {
		t.Errorf("thumbnail should show the new annotation: before=%d after=%d", before, after)
	}
	if _, err := d.RenderThumbnail(9, thumbnailWidthPx); err == nil {
		t.Error("expected an error for an out-of-range page")
	}
}

// Selecting a shape adds an outline + handles to the full page render but
// must NOT touch a thumbnail (fixed-pixel overlays would swamp it) — and
// must not needlessly invalidate the thumbnail cache either.
func TestRenderThumbnail_IgnoresSelectionOverlays(t *testing.T) {
	v := newThreePageView(t)
	d := v.doc
	h := d.AddRectShape(1, "Square", [4]float64{20, 20, 180, 180}, [3]float64{1, 0, 0}, 2)
	plain, _ := d.RenderThumbnail(1, thumbnailWidthPx)
	d.SetSelectedHighlight(h)
	after, _ := d.RenderThumbnail(1, thumbnailWidthPx)
	if after != plain {
		t.Error("selecting a shape must not invalidate thumbnails")
	}
	// Force a fresh render with the selection active and compare pixels.
	d.thumbs.Clear()
	withSel, _ := d.RenderThumbnail(1, thumbnailWidthPx)
	if countNonWhitePixelsForTest(withSel) != countNonWhitePixelsForTest(plain) {
		t.Error("thumbnail drew selection overlays")
	}
}

func TestThumbnailPanel_RowsFilterAndAnnotationCounts(t *testing.T) {
	v := newThreePageView(t)
	tp := v.thumbnails
	if len(tp.rows) != 3 {
		t.Fatalf("expected 3 rows for a 3-page PDF, got %v", tp.rows)
	}
	v.doc.AddRectShape(2, "Square", [4]float64{20, 20, 80, 80}, [3]float64{1, 0, 0}, 2)
	v.doc.AddRectShape(2, "Circle", [4]float64{90, 90, 150, 150}, [3]float64{0, 0, 1}, 2)
	if n := v.doc.highlightCountOnPage(2); n != 2 {
		t.Errorf("highlightCountOnPage(2) = %d, want 2", n)
	}
	tp.onlyAnnot.SetChecked(true)
	if len(tp.rows) != 1 || tp.rows[0] != 2 {
		t.Errorf("annotated-only rows = %v, want [2]", tp.rows)
	}
	tp.onlyAnnot.SetChecked(false)
	if len(tp.rows) != 3 {
		t.Errorf("unfiltered rows = %v, want all 3", tp.rows)
	}
}

func TestThumbnailPanel_PumpRendersPendingAndSelectionFollowsPage(t *testing.T) {
	v := newThreePageView(t)
	tp := v.thumbnails
	tp.pumping = true // suppress the real timer; drive pumpStep by hand
	tp.request(3)
	tp.request(1)
	tp.request(1) // duplicate ignored
	if len(tp.pending) != 2 {
		t.Fatalf("pending = %v, want 2 distinct pages", tp.pending)
	}
	tp.pumping = false
	tp.pumpStep() // most recently requested first
	if tp.cached(1) == nil {
		t.Error("expected page 1 rendered first (most recent request)")
	}
	tp.pumping = false
	tp.pumpStep()
	if tp.cached(3) == nil {
		t.Error("expected page 3 rendered by the second step")
	}

	// Navigating while the panel is showing moves the selected row without
	// bouncing a jump back.
	v.setPanelMode(PanelThumbnails)
	v.setCurrentPage(3)
	if got := v.currentPage; got != 3 {
		t.Errorf("syncing the thumbnail selection changed currentPage to %d", got)
	}
}

func TestPanelSelectOptions_IncludesThumbnails(t *testing.T) {
	opts := panelSelectOptions(false)
	if opts[len(opts)-1] != "Page Thumbnails" {
		t.Errorf("options = %v, want Page Thumbnails last", opts)
	}
}
