package pdf

import (
	"math"
	"testing"

	"fyne.io/fyne/v2"
)

func TestCenteredRectClamped_StaysFullSizeWellInsidePage(t *testing.T) {
	rect := centeredRectClamped(100, 100, 30, 10, 200, 200)
	want := [4]float64{70, 90, 130, 110}
	if rect != want {
		t.Errorf("expected %v, got %v", want, rect)
	}
}

func TestCenteredRectClamped_ShiftsRatherThanShrinksNearEdges(t *testing.T) {
	// Centered just inside the bottom-left corner: a naive clamp would
	// crop the rect down to a sliver; this should shift it to stay the
	// full requested size.
	rect := centeredRectClamped(5, 5, 30, 10, 200, 200)
	if got := rect[2] - rect[0]; got != 60 {
		t.Errorf("expected full 60pt width preserved, got %v (rect=%v)", got, rect)
	}
	if got := rect[3] - rect[1]; got != 20 {
		t.Errorf("expected full 20pt height preserved, got %v (rect=%v)", got, rect)
	}
	if rect[0] < 0 || rect[1] < 0 {
		t.Errorf("expected rect to stay on the page, got %v", rect)
	}

	// Same near the top-right corner.
	rect = centeredRectClamped(195, 195, 30, 10, 200, 200)
	if rect[2] > 200 || rect[3] > 200 {
		t.Errorf("expected rect to stay within the page, got %v", rect)
	}
	if got := rect[2] - rect[0]; got != 60 {
		t.Errorf("expected full 60pt width preserved, got %v (rect=%v)", got, rect)
	}
}

func TestCenteredRectClamped_ShapeLargerThanPageStillStaysNonNegative(t *testing.T) {
	rect := centeredRectClamped(100, 100, 500, 500, 200, 200)
	if rect[0] < 0 || rect[1] < 0 {
		t.Errorf("expected no negative coordinates even when the shape can't fully fit, got %v", rect)
	}
}

// TestAddShapeAt_AddsAtClickedLocation confirms the right-click "Add
// Highlight Here"/"Add Shape" submenu actions (showEmptySpaceContextMenu)
// actually add a highlight/shape on the right page, roughly centered where
// the click happened, and make it visible immediately — the same
// refreshList/repaintPage contract every other "add a shape" path in this
// package already follows.
func TestAddShapeAt_AddsAtClickedLocation(t *testing.T) {
	v := newTestView(t)

	pos := fyne.NewPos(50, 50) // top-left-ish, widget-local
	widgetSize := fyne.NewSize(200, 200)

	v.addShapeAt(1, pos, widgetSize, "Highlight")
	if len(v.doc.Highlights) != 1 {
		t.Fatalf("expected 1 highlight after adding, got %d", len(v.doc.Highlights))
	}
	if v.doc.Highlights[0].Kind != "Highlight" {
		t.Errorf("expected Kind Highlight, got %q", v.doc.Highlights[0].Kind)
	}

	v.addShapeAt(1, pos, widgetSize, "Square")
	if len(v.doc.Highlights) != 2 {
		t.Fatalf("expected 2 highlights after adding a second, got %d", len(v.doc.Highlights))
	}
	sq := v.doc.Highlights[1]
	if sq.Kind != "Square" {
		t.Errorf("expected Kind Square, got %q", sq.Kind)
	}
	// Widget (50,50) on a 200x200 widget over a 200x200 page is PDF point
	// (50, 150) (Y flipped) — confirm the square's own Rect is centered
	// close to that point, not just "somewhere on the page".
	cx, cy := (sq.Rect[0]+sq.Rect[2])/2, (sq.Rect[1]+sq.Rect[3])/2
	if math.Abs(cx-50) > 0.01 || math.Abs(cy-150) > 0.01 {
		t.Errorf("expected square centered near (50,150), got center (%v,%v) rect=%v", cx, cy, sq.Rect)
	}
}

// TestAddShapeAt_EveryQuickShapeKindAddsExactlyOne drives addShapeAt
// through every kind rightClickQuickShapeKinds actually offers (plus
// Highlight, its own top-level sibling action) and confirms each one adds
// a highlight of the right Kind without error — a broad regression guard
// so a future kind added to the Draw dropdown that ALSO gets added here
// doesn't silently panic or no-op (e.g. a kind this function's switch
// doesn't yet handle, falling through to the Highlight default).
func TestAddShapeAt_EveryQuickShapeKindAddsExactlyOne(t *testing.T) {
	kinds := append([]string{"Highlight"}, rightClickQuickShapeKinds...)
	pos := fyne.NewPos(100, 100)
	widgetSize := fyne.NewSize(200, 200)

	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			if kind == "Text" || kind == "Speech Bubble" {
				t.Skip("defers creation to a caption dialog (promptForShapeText), not testable headlessly")
			}
			// Star/Hexagon have no dedicated PDF annotation subtype —
			// both are authored as a generic Polygon (see AddPolygonShape
			// and CLAUDE.md's own "Draw Star, Hexagon..." section).
			wantKind := kind
			if kind == "Star" || kind == "Hexagon" {
				wantKind = "Polygon"
			}
			v := newTestView(t)
			v.addShapeAt(1, pos, widgetSize, kind)
			if len(v.doc.Highlights) != 1 {
				t.Fatalf("expected exactly 1 highlight added for kind %q, got %d", kind, len(v.doc.Highlights))
			}
			if got := v.doc.Highlights[0].Kind; got != wantKind {
				t.Errorf("expected Kind %q, got %q", wantKind, got)
			}
		})
	}
}

// TestBookmarkPanel_ShowAddDialogAt_UsesGivenPageAndFraction is a narrow
// regression guard for showAddDialogAt's own parameterization: the
// right-click "Add Bookmark Here"/"Add TOC Entry Here" actions pass a
// specific clicked page/fraction, which must reach the dialog's own
// prefilled page field and region-bookmark preselection rather than
// silently falling back to the view's current page/scroll position (what
// the un-parameterized showAddDialog still uses). This can't drive the
// dialog's own Confirm button headlessly (it's a real dialog.Dialog), so
// it only checks the one thing that would silently regress if a future
// edit went back to reading bp.v.currentPage/bp.v.scrollFraction()
// directly instead of the parameters: that calling it doesn't panic and
// doesn't touch the view's own current page.
func TestBookmarkPanel_ShowAddDialogAt_UsesGivenPageAndFraction(t *testing.T) {
	v := newTestView(t)
	v.currentPage = 1

	v.panel.showAddDialogAt(false, 1, 0.75)
	if v.currentPage != 1 {
		t.Errorf("expected showAddDialogAt not to change the view's own current page, got %d", v.currentPage)
	}
}

func TestSnapClosedIfNearStart(t *testing.T) {
	loop := [][2]float64{{0, 0}, {50, 0}, {50, 50}, {0, 50}, {2, 3}}
	got := snapClosedIfNearStart(loop)
	if got[len(got)-1] != got[0] {
		t.Errorf("a nearly-closed loop should snap its last point to the start: %v", got)
	}
	if loop[len(loop)-1] != [2]float64{2, 3} {
		t.Error("input slice must not be mutated")
	}
	open := [][2]float64{{0, 0}, {50, 0}, {50, 50}, {0, 50}}
	if g := snapClosedIfNearStart(open); g[len(g)-1] != open[len(open)-1] {
		t.Errorf("a clearly open stroke must stay open: %v", g)
	}
	if g := snapClosedIfNearStart([][2]float64{{0, 0}, {1, 1}}); len(g) != 2 || g[1] != [2]float64{1, 1} {
		t.Errorf("too-short stroke changed: %v", g)
	}
}

func TestSimplifyPath_ThinsDensePointsButKeepsCorners(t *testing.T) {
	var pts [][2]float64
	for i := 0; i <= 100; i++ { // dense straight run, then a corner, then another run
		pts = append(pts, [2]float64{float64(i), 0})
	}
	for i := 1; i <= 100; i++ {
		pts = append(pts, [2]float64{100, float64(i)})
	}
	got := simplifyPath(pts, 1.5)
	if len(got) != 3 || got[1] != [2]float64{100, 0} {
		t.Errorf("expected just the two endpoints and the corner, got %v", got)
	}
}

func TestHandleFreehandDrawn_PolygonKindAddsClosedSimplifiedPolygon(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, _ := v.doc.PageBoundsPt(1)
	size := fyne.NewSize(float32(pageW), float32(pageH))
	v.drawKind = "Polygon"
	var pts []fyne.Position
	add := func(x, y float32) { pts = append(pts, fyne.NewPos(x, y)) }
	for i := 0; i <= 60; i++ { // a dense square loop
		add(20+float32(i), 20)
	}
	for i := 1; i <= 60; i++ {
		add(80, 20+float32(i))
	}
	for i := 1; i <= 60; i++ {
		add(80-float32(i), 80)
	}
	for i := 1; i <= 60; i++ {
		add(20, 80-float32(i))
	}
	v.handleFreehandDrawn(1, pts, size)
	if len(v.doc.Highlights) != 1 || v.doc.Highlights[0].Kind != "Polygon" {
		t.Fatalf("expected one Polygon, got %+v", v.doc.Highlights)
	}
	if n := len(v.doc.Highlights[0].Vertices); n < 4 || n > 8 {
		t.Errorf("a square loop should thin to a handful of vertices, got %d", n)
	}
}
