package pdf

import (
	"math"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// newTestView builds a real *view over a fresh minimal PDF, the same
// pattern view_retarget_test.go uses — enough machinery to exercise
// selection/deselection end to end (panel list, Document.selectedHighlight,
// typedKey) without a live GUI.
func newTestView(t *testing.T) *view {
	t.Helper()
	_ = test.NewApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

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

// TestHighlightsPanel_Deselect_ClearsSelectionEverywhere confirms deselect
// clears both the panel's own selected field and Document.selectedHighlight
// (the outline paintHighlights draws from) — the two places a selection
// actually lives.
func TestHighlightsPanel_Deselect_ClearsSelectionEverywhere(t *testing.T) {
	v := newTestView(t)
	h := v.doc.AddHighlight(1, [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}, defaultHighlightColor, "")
	v.highlights.refreshList()
	v.highlights.list.Select(0)

	if v.highlights.selected != h || v.doc.selectedHighlight != h {
		t.Fatalf("setup failed: expected h selected, got panel=%v doc=%v", v.highlights.selected, v.doc.selectedHighlight)
	}

	v.highlights.deselect()

	if v.highlights.selected != nil {
		t.Errorf("expected highlights.selected to be cleared, got %v", v.highlights.selected)
	}
	if v.doc.selectedHighlight != nil {
		t.Errorf("expected doc.selectedHighlight to be cleared, got %v", v.doc.selectedHighlight)
	}
}

// TestHighlightsPanel_Deselect_NoopWithNoSelection guards against a nil
// dereference or spurious repaint when nothing is selected -- Escape and
// an empty-space click both call this unconditionally.
func TestHighlightsPanel_Deselect_NoopWithNoSelection(t *testing.T) {
	v := newTestView(t)
	v.highlights.deselect() // must not panic
	if v.highlights.selected != nil || v.doc.selectedHighlight != nil {
		t.Errorf("expected no selection to remain nil, got panel=%v doc=%v", v.highlights.selected, v.doc.selectedHighlight)
	}
}

// TestHandleHighlightTapped_EmptySpaceDeselects is the real regression
// test for the request that added this: clicking page space with nothing
// under it must clear whatever was previously selected, not leave it
// lingering -- a deliberate "I don't mean to have anything selected"
// gesture, which matters more now that a bare Delete/Backspace acts on
// the current selection.
func TestHandleHighlightTapped_EmptySpaceDeselects(t *testing.T) {
	v := newTestView(t)
	h := v.doc.AddHighlight(1, [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}, defaultHighlightColor, "")
	v.highlights.refreshList()
	v.highlights.list.Select(0)
	if v.highlights.selected != h {
		t.Fatalf("setup failed: expected h selected")
	}

	// Tap far outside the highlight's own quad -- HighlightAt must miss.
	v.handleHighlightTapped(1, fyne.NewPos(9999, 9999), fyne.NewSize(1000, 1000))

	if v.highlights.selected != nil {
		t.Errorf("expected a miss to deselect, got %v still selected", v.highlights.selected)
	}
}

// TestHandleHighlightDrawn_ShapeKinds is the real end-to-end test for the
// shape-drawing UI: for each drawable kind, sets v.drawKind (as the
// toolbar's own Select would) and drives handleHighlightDrawn the same
// way highlightDrawer.DragEnd does — a raw (start, end) pair, direction
// preserved — confirming each kind lands in v.doc.Highlights with the
// right geometry, and that Line specifically preserves drag direction
// (start != a normalized top-left corner).
func TestHandleHighlightDrawn_ShapeKinds(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH)) // 1:1, so widget points == PDF points

	tests := []struct {
		kind string
		want string
	}{
		{"Highlight", "Highlight"},
		{"Square", "Square"},
		{"Circle", "Circle"},
		{"Line", "Line"},
		{"Star", "Polygon"},    // no dedicated PDF subtype -- see AddPolygonShape
		{"Hexagon", "Polygon"}, // same
	}
	for _, tc := range tests {
		before := len(v.doc.Highlights)
		v.drawKind = tc.kind
		// A drag from bottom-right toward top-left (start.Y > end.Y in
		// widget space, i.e. dragging UPWARD on screen) -- deliberately
		// not top-left-to-bottom-right, so a bug that silently assumes
		// drag direction wouldn't slip through unnoticed.
		v.handleHighlightDrawn(1, fyne.NewPos(80, 60), fyne.NewPos(20, 10), widgetSize)

		if len(v.doc.Highlights) != before+1 {
			t.Fatalf("kind=%s: expected 1 new highlight, got %d total (had %d)", tc.kind, len(v.doc.Highlights), before)
		}
		got := v.doc.Highlights[len(v.doc.Highlights)-1]
		if got.Kind != tc.want {
			t.Errorf("kind=%s: got Highlight.Kind = %q, want %q", tc.kind, got.Kind, tc.want)
		}
	}

	// Line must preserve drag direction: start=(80,60) end=(20,10) in
	// widget space converts (via widgetPointToPDF's Y-flip) to a PDF-space
	// line whose FIRST point corresponds to the widget's start, not
	// whichever endpoint happens to be top-left.
	var line *Highlight
	for _, h := range v.doc.Highlights {
		if h.Kind == "Line" {
			line = h
		}
	}
	if line == nil {
		t.Fatalf("expected a Line highlight among %+v", v.doc.Highlights)
	}
	wantX1, wantY1 := widgetPointToPDF(fyne.NewPos(80, 60), widgetSize, pageW, pageH)
	if line.Line[0] != wantX1 || line.Line[1] != wantY1 {
		t.Errorf("Line's own first point = (%v,%v), want (%v,%v) -- drag direction not preserved",
			line.Line[0], line.Line[1], wantX1, wantY1)
	}
}

// TestHandleFreehandDrawn_AddsInkWithEveryPointConverted confirms Ink's
// own view-layer entry point (handleFreehandDrawn, wired to
// highlightDrawer.OnFreehandDrawn) converts every sampled widget-space
// point — not just a start/end pair, unlike every other shape's
// handleHighlightDrawn path — into PDF space, in order, and adds the
// result as a single new Ink-kind Highlight.
func TestHandleFreehandDrawn_AddsInkWithEveryPointConverted(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH)) // 1:1

	points := []fyne.Position{
		fyne.NewPos(20, 20),
		fyne.NewPos(25, 40),
		fyne.NewPos(60, 30),
	}
	v.handleFreehandDrawn(1, points, widgetSize)

	if len(v.doc.Highlights) != 1 {
		t.Fatalf("expected 1 new highlight, got %d", len(v.doc.Highlights))
	}
	got := v.doc.Highlights[0]
	if got.Kind != "Ink" {
		t.Fatalf("got Kind = %q, want Ink", got.Kind)
	}
	if len(got.Vertices) != len(points) {
		t.Fatalf("got %d vertices, want %d", len(got.Vertices), len(points))
	}
	for i, p := range points {
		wantX, wantY := widgetPointToPDF(p, widgetSize, pageW, pageH)
		if got.Vertices[i][0] != wantX || got.Vertices[i][1] != wantY {
			t.Errorf("vertex %d = %v, want (%v,%v)", i, got.Vertices[i], wantX, wantY)
		}
	}
}

// TestHandleFreehandDrawn_PolyLineKindRoutesToAddPolyLineShape confirms
// handleFreehandDrawn picks AddPolyLineShape instead of AddInkShape when
// v.drawKind is "PolyLine" — the one branch distinguishing the two kinds,
// since both otherwise share the exact same freehand capture (see
// AddPolyLineShape's own doc comment).
func TestHandleFreehandDrawn_PolyLineKindRoutesToAddPolyLineShape(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH))

	v.drawKind = "PolyLine"
	v.handleFreehandDrawn(1, []fyne.Position{fyne.NewPos(20, 20), fyne.NewPos(60, 30)}, widgetSize)

	if len(v.doc.Highlights) != 1 {
		t.Fatalf("expected 1 new highlight, got %d", len(v.doc.Highlights))
	}
	if got := v.doc.Highlights[0].Kind; got != "PolyLine" {
		t.Errorf("got Kind = %q, want PolyLine", got)
	}
}

// TestHandleHighlightDrawn_TextKindsDeferToDialog confirms Text/Speech
// Bubble don't add anything to v.doc.Highlights immediately the way
// every other kind does -- creation is deferred to promptForShapeText's
// own dialog confirm (untestable headlessly here, a real modal dialog),
// so a drag with one of these draw kinds selected must leave
// v.doc.Highlights completely unchanged.
func TestHandleHighlightDrawn_TextKindsDeferToDialog(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	widgetSize := fyne.NewSize(float32(pageW), float32(pageH))

	for _, kind := range []string{"Text", "Speech Bubble"} {
		before := len(v.doc.Highlights)
		v.drawKind = kind
		v.handleHighlightDrawn(1, fyne.NewPos(20, 10), fyne.NewPos(80, 60), widgetSize)
		if len(v.doc.Highlights) != before {
			t.Errorf("kind=%s: expected no immediate highlight (creation deferred to the text dialog), got %d new", kind, len(v.doc.Highlights)-before)
		}
	}
}

// TestCalloutTipFor_OffsetsBelowLeftOfBox confirms a speech bubble's
// callout tip lands outside its own box, toward the bottom-left, and
// scales with the box's own (smaller) dimension rather than a fixed
// absolute offset that would look wrong on a tiny or huge box.
func TestCalloutTipFor_OffsetsBelowLeftOfBox(t *testing.T) {
	rect := [4]float64{100, 100, 180, 140} // w=80, h=40 -- smaller dimension is h=40
	tip := calloutTipFor(rect)

	if tip[0] >= rect[0] || tip[1] >= rect[1] {
		t.Errorf("calloutTipFor(%v) = %v, want both coordinates below/left of the box's own min corner (%v,%v)",
			rect, tip, rect[0], rect[1])
	}
	wantOffset := 40.0 * 0.4
	if math.Abs((rect[0]-tip[0])-wantOffset) > geomEpsilon || math.Abs((rect[1]-tip[1])-wantOffset) > geomEpsilon {
		t.Errorf("calloutTipFor(%v) = %v, want offset %v from (%v,%v)", rect, tip, wantOffset, rect[0], rect[1])
	}
}

// TestTypedKey_EscapeDeselects confirms Escape reaches the same deselect
// path as an empty-space click.
func TestTypedKey_EscapeDeselects(t *testing.T) {
	v := newTestView(t)
	h := v.doc.AddHighlight(1, [][8]float64{{0, 10, 10, 10, 0, 0, 10, 0}}, defaultHighlightColor, "")
	v.highlights.refreshList()
	v.highlights.list.Select(0)
	if v.highlights.selected != h {
		t.Fatalf("setup failed: expected h selected")
	}

	v.typedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})

	if v.highlights.selected != nil {
		t.Errorf("expected Escape to deselect, got %v still selected", v.highlights.selected)
	}
}
