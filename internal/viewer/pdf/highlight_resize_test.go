package pdf

import (
	"math"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func rectNear(a, b [4]float64) bool {
	for i := range a {
		if !near(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestResizedBoundsPt_CornersAndEdges(t *testing.T) {
	h := &Highlight{Kind: "Square", Rect: [4]float64{10, 10, 60, 60}}
	cases := []struct {
		handle resizeHandle
		dx, dy float64
		want   [4]float64
	}{
		{handleNE, 10, 20, [4]float64{10, 10, 70, 80}},
		{handleSW, -5, -5, [4]float64{5, 5, 60, 60}},
		{handleE, 15, 99, [4]float64{10, 10, 75, 60}}, // vertical delta ignored by an E edge
		{handleN, 99, 7, [4]float64{10, 10, 60, 67}},
		{handleW, 8, 0, [4]float64{18, 10, 60, 60}},
		// dragging an edge past the opposite one clamps, never flips
		{handleE, -200, 0, [4]float64{10, 10, 10 + minResizeExtentPt, 60}},
		{handleS, 0, 200, [4]float64{10, 60 - minResizeExtentPt, 60, 60}},
	}
	for _, c := range cases {
		got, ok := resizedBoundsPt(h, c.handle, c.dx, c.dy)
		if !ok || !rectNear(got, c.want) {
			t.Errorf("handle %d (%v,%v): got %v ok=%v, want %v", c.handle, c.dx, c.dy, got, ok, c.want)
		}
	}
}

func TestResizeHighlightGeometry_ScalesVerticesQuadsAndRect(t *testing.T) {
	ink := &Highlight{Kind: "Ink", Rect: [4]float64{0, 0, 100, 50}, Vertices: [][2]float64{{0, 0}, {50, 25}, {100, 50}}}
	resizeHighlightGeometry(ink, handleNE, 100, 50) // box becomes 200x100
	if !rectNear(ink.Rect, [4]float64{0, 0, 200, 100}) {
		t.Errorf("Rect = %v", ink.Rect)
	}
	if ink.Vertices[1] != [2]float64{100, 50} || ink.Vertices[2] != [2]float64{200, 100} {
		t.Errorf("Vertices not scaled with the box: %v", ink.Vertices)
	}

	hl := &Highlight{Kind: "Highlight", Quads: [][8]float64{{0, 10, 20, 10, 0, 0, 20, 0}, {0, 30, 40, 30, 0, 20, 40, 20}}}
	resizeHighlightGeometry(hl, handleE, 40, 0) // width 40 -> 80
	if hl.Quads[0][2] != 40 || hl.Quads[1][2] != 80 {
		t.Errorf("quads not stretched horizontally: %v", hl.Quads)
	}
	if hl.Quads[1][1] != 30 {
		t.Errorf("an E-edge drag must not change Y: %v", hl.Quads[1])
	}
}

func TestResizeHighlightGeometry_ZeroWidthStrokeShiftsInsteadOfScaling(t *testing.T) {
	// A perfectly vertical Ink stroke has zero width; scaling from zero is
	// undefined, so a horizontal grip drag should translate it, not NaN it.
	ink := &Highlight{Kind: "Ink", Rect: [4]float64{10, 0, 10, 50}, Vertices: [][2]float64{{10, 0}, {10, 50}}}
	resizeHighlightGeometry(ink, handleE, 5, 0)
	for _, v := range ink.Vertices {
		if math.IsNaN(v[0]) || math.IsInf(v[0], 0) {
			t.Fatalf("non-finite vertex %v", v)
		}
	}
}

func TestResizeHighlightGeometry_LineEndpointsAndCalloutTip(t *testing.T) {
	line := &Highlight{Kind: "Line", Line: []float64{10, 10, 50, 50}}
	resizeHighlightGeometry(line, handleLineEnd, 5, -5)
	if line.Line[0] != 10 || line.Line[1] != 10 || line.Line[2] != 55 || line.Line[3] != 45 {
		t.Errorf("Line = %v", line.Line)
	}
	tip := [2]float64{0, 0}
	ft := &Highlight{Kind: "FreeText", Rect: [4]float64{10, 10, 60, 40}, CalloutTip: &tip}
	resizeHighlightGeometry(ft, handleCalloutTip, -7, -3)
	if *ft.CalloutTip != [2]float64{-7, -3} || ft.Rect != [4]float64{10, 10, 60, 40} {
		t.Errorf("tip=%v rect=%v", *ft.CalloutTip, ft.Rect)
	}
	// Resizing the box leaves the aimed tip where it was.
	resizeHighlightGeometry(ft, handleNE, 20, 20)
	if *ft.CalloutTip != [2]float64{-7, -3} {
		t.Errorf("tip moved with the box: %v", *ft.CalloutTip)
	}
}

func TestHitResizeHandle_AndHandleSets(t *testing.T) {
	sq := &Highlight{Kind: "Square", Rect: [4]float64{10, 10, 60, 60}}
	if n := len(resizeHandlesFor(sq)); n != 8 {
		t.Errorf("Square handles = %d, want 8", n)
	}
	if hnd, ok := hitResizeHandle(sq, 60, 60); !ok || hnd != handleNE {
		t.Errorf("hit at NE corner = %v,%v", hnd, ok)
	}
	if hnd, ok := hitResizeHandle(sq, 35, 61); !ok || hnd != handleN {
		t.Errorf("hit at N edge midpoint = %v,%v", hnd, ok)
	}
	if _, ok := hitResizeHandle(sq, 35, 35); ok {
		t.Error("the interior must not hit a handle (it's a move)")
	}
	line := &Highlight{Kind: "Line", Line: []float64{0, 0, 40, 40}}
	if n := len(resizeHandlesFor(line)); n != 2 {
		t.Errorf("Line handles = %d, want 2", n)
	}
	tip := [2]float64{1, 1}
	bubble := &Highlight{Kind: "FreeText", Rect: [4]float64{10, 10, 60, 40}, CalloutTip: &tip}
	if n := len(resizeHandlesFor(bubble)); n != 9 {
		t.Errorf("speech bubble handles = %d, want 9 (8 + tip)", n)
	}
}

func TestCanResizeHighlight(t *testing.T) {
	if !canResizeHighlight(&Highlight{Kind: "Square", Rect: [4]float64{1, 1, 9, 9}}) {
		t.Error("an unsaved Square should be resizable")
	}
	if canResizeHighlight(&Highlight{Kind: "Square"}) {
		t.Error("a Square with no Rect has nothing to resize")
	}
	// A saved, foreign Polygon (no Vertices) can't be re-authored.
	if canResizeHighlight(&Highlight{Kind: "Polygon", ObjNr: 12, Rect: [4]float64{1, 1, 9, 9}}) {
		t.Error("a Vertices-less saved Polygon must not be resizable")
	}
	if canResizeHighlight(&Highlight{Kind: "Stamp", ObjNr: 12, Rect: [4]float64{1, 1, 9, 9}}) {
		t.Error("Stamp is never resizable")
	}
}

// Resize an already-saved Square, save, reopen: the new Rect is what's on
// disk, there's still exactly one annotation, and MuPDF's real render of
// the saved file paints at the new far corner.
func TestSaveHighlights_ResizeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)
	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddRectShape(1, "Square", [4]float64{20, 20, 60, 60}, [3]float64{1, 0, 0}, 2)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("first save: %v", err)
	}
	h := doc.Highlights[0]
	doc.ResizeHighlight(h, handleNE, 60, 60) // -> [20,20,120,120]
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("second save: %v", err)
	}

	re, err := Prepare(path)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer re.Close()
	if len(re.Highlights) != 1 {
		t.Fatalf("expected exactly 1 highlight after resize+save, got %d", len(re.Highlights))
	}
	if got := re.Highlights[0].Rect; !rectNear(got, [4]float64{20, 20, 120, 120}) {
		t.Errorf("reloaded Rect = %v, want [20 20 120 120]", got)
	}
	img, err := re.RenderPage(1, 1.0)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	_, pageH, _ := re.PageBoundsPt(1)
	scale := baseRenderDPI / 72.0
	// The new right edge at x=120pt, mid-height, must be painted red-ish.
	px, py := int(120*scale), int((pageH-70)*scale)
	found := false
	for dx := -3; dx <= 3 && !found; dx++ {
		r, g, b, _ := img.At(px+dx, py).RGBA()
		found = r>>8 > 150 && g>>8 < 100 && b>>8 < 100
	}
	if !found {
		t.Errorf("expected the resized Square's right edge painted near (%d,%d)", px, py)
	}
}

// A resized-then-recolored Ink stroke keeps its (scaled) Vertices through
// the save, via the same ObjNr carry-forward Move relies on.
func TestSaveHighlights_ResizeInkKeepsScaledVertices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)
	doc := &Document{path: path, cache: newPageCache(pageCacheCapacity)}
	doc.AddInkShape(1, [][2]float64{{10, 10}, {30, 60}, {60, 20}, {90, 80}}, [3]float64{1, 0, 0}, 1)
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	doc.ResizeHighlight(doc.Highlights[0], handleE, 80, 0) // 80 wide -> 160
	if err := doc.SaveHighlights(path); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	got := doc.Highlights[0].Vertices
	if len(got) != 4 || !near(got[3][0], 170) || !near(got[1][0], 50) {
		t.Errorf("Vertices after resize+save = %v, want x scaled 2x about the left edge", got)
	}
}

func TestHandleResize_ViewLevelEndToEnd(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatalf("PageBoundsPt: %v", err)
	}
	size := fyne.NewSize(float32(pageW), float32(pageH)) // 1 point per unit
	h := v.doc.AddRectShape(1, "Square", [4]float64{10, 10, 60, 60}, [3]float64{1, 0, 0}, 1)
	v.doc.SetSelectedHighlight(h)

	ne := fyne.NewPos(60, float32(pageH)-60)
	hnd, ok := v.handleResizeHitTest(1, ne, size)
	if !ok || hnd != handleNE {
		t.Fatalf("hit test at the NE corner = %v,%v", hnd, ok)
	}
	if _, ok := v.handleResizeHitTest(1, fyne.NewPos(35, float32(pageH)-35), size); ok {
		t.Error("the interior must not register as a handle")
	}
	end := fyne.NewPos(ne.X+25, ne.Y-30)
	tl, br := v.handleResizeGhost(1, hnd, ne, end, size)
	if tl != fyne.NewPos(10, float32(pageH)-90) || br != fyne.NewPos(85, float32(pageH)-10) {
		t.Errorf("ghost = %v/%v", tl, br)
	}
	v.handleResized(1, hnd, ne, end, size)
	if !rectNear(h.Rect, [4]float64{10, 10, 85, 90}) {
		t.Errorf("Rect after handleResized = %v", h.Rect)
	}
}

// Drawer gesture: a drag that starts on a grip resizes (fires OnResized,
// never OnMoved), and Draw mode still wins over a resize.
func TestHighlightDrawer_DragOnHandleResizes(t *testing.T) {
	d := newTestHighlightDrawer()
	var gotHandle resizeHandle
	moved, resized := false, false
	d.HitTestHandle = func(pos fyne.Position, _ fyne.Size) (resizeHandle, bool) {
		return handleSE, pos.X >= 90
	}
	d.HitTestSelected = func(fyne.Position, fyne.Size) (fyne.Position, fyne.Position, bool) {
		return fyne.NewPos(0, 0), fyne.NewPos(100, 100), true
	}
	d.ResizeGhost = func(_ resizeHandle, _, cur fyne.Position, _ fyne.Size) (fyne.Position, fyne.Position) {
		return fyne.NewPos(0, 0), cur
	}
	d.OnResized = func(hnd resizeHandle, _, _ fyne.Position, _ fyne.Size) { gotHandle, resized = hnd, true }
	d.OnMoved = func(_, _ fyne.Position, _ fyne.Size) { moved = true }

	dragGesture(d, 95, 95, 30, 30)
	if !resized || moved || gotHandle != handleSE {
		t.Errorf("grip drag: resized=%v moved=%v handle=%v", resized, moved, gotHandle)
	}

	resized, moved = false, false
	dragGesture(d, 50, 50, 30, 30) // starts in the interior, not on a grip
	if resized || !moved {
		t.Errorf("interior drag: resized=%v moved=%v, want a move only", resized, moved)
	}

	resized, moved = false, false
	d.Enabled = true
	dragGesture(d, 95, 95, 30, 30)
	if resized || moved {
		t.Errorf("Draw mode must suppress resize/move: resized=%v moved=%v", resized, moved)
	}
}
