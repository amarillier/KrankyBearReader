package pdf

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestView_RetargetTo_SwapsDocumentAndReportsPathChange covers the
// per-tab half of "Save as a new file..." switching a PDF tab to the
// newly-saved file: the actual behavior a user asked for after noticing a
// Save As left the tab still showing the original file's name and content.
// Manager_test.go's TestManager_RetargetTab_UpdatesTitleAndPathBookkeeping
// covers the Manager-side path/title bookkeeping this drives via
// onRetargeted; this covers what happens inside the tab itself.
func TestView_RetargetTo_SwapsDocumentAndReportsPathChange(t *testing.T) {
	_ = test.NewApp()
	dir := t.TempDir()

	origPath := filepath.Join(dir, "orig.pdf")
	writeMinimalPDF(t, origPath)
	newPath := filepath.Join(dir, "new.pdf")
	writeMinimalPDF(t, newPath)

	doc, err := Prepare(origPath)
	if err != nil {
		t.Fatalf("Prepare(orig): %v", err)
	}

	win := test.NewWindow(nil)
	defer win.Close()

	var reportedOld, reportedNew string
	v := &view{
		win:            win,
		doc:            doc,
		currentPage:    1,
		zoomLevel:      -1,
		highlightColor: defaultHighlightColor,
		onRetargeted: func(oldPath, newPath string) {
			reportedOld, reportedNew = oldPath, newPath
		},
	}
	v.build()

	oldDoc := v.doc
	if err := v.retargetTo(newPath); err != nil {
		t.Fatalf("retargetTo: %v", err)
	}
	defer v.doc.Close()

	if v.doc == oldDoc {
		t.Errorf("expected v.doc to be swapped for a new Document")
	}
	if got := v.doc.Path(); got != newPath {
		t.Errorf("expected v.doc.Path() = %q, got %q", newPath, got)
	}
	if reportedOld != origPath || reportedNew != newPath {
		t.Errorf("onRetargeted reported (%q, %q), want (%q, %q)", reportedOld, reportedNew, origPath, newPath)
	}
	if v.highlights.selected != nil {
		t.Errorf("expected highlights panel selection cleared after retarget")
	}
	if v.panel.selected != nil {
		t.Errorf("expected bookmarks panel selection cleared after retarget")
	}
}

// TestHighlightsPanel_SaveChangesTo_RetargetsOnRealSaveAs is the exact
// end-to-end scenario a user reported: delete a highlight, "Save as a new
// file...", and expect the tab to now be editing that new file — not still
// showing the original, unmodified one under a stale title. Drives
// saveChangesTo directly (not through its own dialogs, which this headless
// test has no need to click through) to exercise the real call path a
// Save-As click makes, deletion included.
func TestHighlightsPanel_SaveChangesTo_RetargetsOnRealSaveAs(t *testing.T) {
	_ = test.NewApp()
	dir := t.TempDir()

	origPath := filepath.Join(dir, "orig.pdf")
	writeMinimalPDF(t, origPath)
	rect := types.RectForDim(50, 50)
	quad := types.QuadPoints{*types.NewQuadLiteralForRect(rect)}
	ann := model.NewHighlightAnnotation(*rect, 0, "", "", "", 0, nil, 0, 0, 0, "", nil, nil, "", "", quad)
	if err := api.AddAnnotationsFile(origPath, origPath, []string{"1"}, ann, nil, false); err != nil {
		t.Fatalf("adding test highlight: %v", err)
	}

	doc, err := Prepare(origPath)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight before delete, got %d", len(doc.Highlights))
	}

	win := test.NewWindow(nil)
	defer win.Close()
	v := &view{win: win, doc: doc, currentPage: 1, zoomLevel: -1, highlightColor: defaultHighlightColor}
	v.build()

	if !v.doc.DeleteHighlight(v.doc.Highlights[0]) {
		t.Fatalf("DeleteHighlight reported not found")
	}

	newPath := filepath.Join(dir, "orig-highlighted.pdf")
	v.highlights.saveChangesTo(newPath)
	defer v.doc.Close()

	if v.doc.Path() != newPath {
		t.Fatalf("expected the tab to have retargeted to %q, got %q", newPath, v.doc.Path())
	}
	if len(v.doc.Highlights) != 0 {
		t.Errorf("expected the retargeted Document to reflect the deletion (0 highlights), got %d", len(v.doc.Highlights))
	}

	// The original file must be untouched — still has its 1 highlight,
	// completely unaware anything was ever deleted from a copy of it.
	origHighlights, err := LoadHighlights(origPath)
	if err != nil {
		t.Fatalf("LoadHighlights(origPath): %v", err)
	}
	if len(origHighlights) != 1 {
		t.Errorf("expected the ORIGINAL file to still have its 1 highlight untouched, got %d", len(origHighlights))
	}
}
