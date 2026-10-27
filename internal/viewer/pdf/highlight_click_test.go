package pdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func tapAt(d *highlightDrawer, x, y float32) {
	d.Tapped(&fyne.PointEvent{Position: fyne.NewPos(x, y)})
}

type clickResult struct {
	points []fyne.Position
	closed bool
	n      int
}

func newClickDrawer(res *clickResult) *highlightDrawer {
	d := newTestHighlightDrawer()
	d.Enabled = true
	d.ClickMode = true
	d.OnClickPolyline = func(p []fyne.Position, _ fyne.Size, closed bool) {
		res.points, res.closed = p, closed
		res.n++
	}
	return d
}

func TestClickMode_TapLastVertexFinishesOpenLine(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	tapAt(d, 50, 50)
	tapAt(d, 150, 60)
	tapAt(d, 200, 160)
	if res.n != 0 || len(d.clickPts) != 3 {
		t.Fatalf("expected 3 pending vertices and no result yet, got n=%d pts=%d", res.n, len(d.clickPts))
	}
	tapAt(d, 202, 158) // within snap of the last vertex
	if res.n != 1 || res.closed || len(res.points) != 3 {
		t.Fatalf("got n=%d closed=%v points=%v", res.n, res.closed, res.points)
	}
	if d.HasClickPending() {
		t.Error("state should clear after finishing")
	}
}

func TestClickMode_TapFirstVertexClosesShape(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	tapAt(d, 50, 50)
	tapAt(d, 150, 50)
	tapAt(d, 100, 150)
	tapAt(d, 52, 52)
	if res.n != 1 || !res.closed || len(res.points) != 3 {
		t.Fatalf("got n=%d closed=%v points=%v", res.n, res.closed, res.points)
	}
}

func TestClickMode_TapFirstWithTwoVerticesDoesNothing(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	tapAt(d, 50, 50)
	tapAt(d, 150, 50)
	tapAt(d, 51, 51)
	if res.n != 0 || len(d.clickPts) != 2 {
		t.Fatalf("n=%d pts=%d, want 0 and 2", res.n, len(d.clickPts))
	}
}

func TestClickMode_FinishAndCancel(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	tapAt(d, 50, 50)
	if d.FinishClick(false) || res.n != 0 {
		t.Error("a single vertex must not finish")
	}
	tapAt(d, 50, 50)
	tapAt(d, 100, 100)
	d.CancelClick()
	if d.HasClickPending() || res.n != 0 {
		t.Error("cancel should drop the shape without firing")
	}
	tapAt(d, 10, 10)
	tapAt(d, 90, 10)
	if !d.FinishClick(false) || res.n != 1 || len(res.points) != 2 {
		t.Errorf("Enter-style finish failed: n=%d points=%v", res.n, res.points)
	}
}

func TestClickMode_JitteryClickViaDragStillAddsVertexAndDragDoesNot(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	drawn := false
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }
	dragGesture(d, 100, 100, minDragPts-1, minDragPts-1)
	if len(d.clickPts) != 1 {
		t.Fatalf("jittery click should add a vertex, got %d", len(d.clickPts))
	}
	dragGesture(d, 200, 200, 60, 60)
	if drawn || len(d.clickPts) != 1 {
		t.Errorf("a real drag must do nothing in click mode: drawn=%v pts=%d", drawn, len(d.clickPts))
	}
}

func TestClickMode_OffLeavesTapToOnTapped(t *testing.T) {
	tapped := false
	d := newTestHighlightDrawer()
	d.Enabled = true
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }
	tapAt(d, 10, 10)
	if !tapped || d.HasClickPending() {
		t.Errorf("tapped=%v pending=%v", tapped, d.HasClickPending())
	}
}

func TestClickMode_RubberBandFollowsPointerAndObjectsListed(t *testing.T) {
	var res clickResult
	d := newClickDrawer(&res)
	d.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(5, 5)}})
	if d.rubber != nil {
		t.Error("no rubber band before the first vertex")
	}
	tapAt(d, 50, 50)
	tapAt(d, 100, 50)
	d.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(120, 90)}})
	if d.rubber == nil || d.rubber.Position1 != fyne.NewPos(100, 50) || d.rubber.Position2 != fyne.NewPos(120, 90) {
		t.Fatalf("rubber band wrong: %+v", d.rubber)
	}
	// 1 segment + rubber + 2 markers + image + overlay
	if got := len((&highlightDrawerRenderer{d: d}).Objects()); got != 6 {
		t.Errorf("renderer objects = %d, want 6", got)
	}
}

func TestHandleClickPolyline_OpenIsPolyLineClosedIsPolygon(t *testing.T) {
	v := newTestView(t)
	pageW, pageH, err := v.doc.PageBoundsPt(1)
	if err != nil {
		t.Fatal(err)
	}
	size := fyne.NewSize(float32(pageW), float32(pageH))
	pts := []fyne.Position{fyne.NewPos(20, 20), fyne.NewPos(80, 30), fyne.NewPos(50, 90)}

	v.handleClickPolyline(1, pts, size, false)
	v.handleClickPolyline(1, pts, size, true)
	if len(v.doc.Highlights) != 2 {
		t.Fatalf("got %d highlights", len(v.doc.Highlights))
	}
	if k := v.doc.Highlights[0].Kind; k != "PolyLine" {
		t.Errorf("open shape Kind = %q", k)
	}
	if k := v.doc.Highlights[1].Kind; k != "Polygon" {
		t.Errorf("closed shape Kind = %q", k)
	}
	for i, p := range pts {
		wx, wy := widgetPointToPDF(p, size, pageW, pageH)
		if got := v.doc.Highlights[0].Vertices[i]; got[0] != wx || got[1] != wy {
			t.Errorf("vertex %d = %v, want (%v,%v)", i, got, wx, wy)
		}
	}
}

func TestView_EscapeCancelsClickShapeBeforeDeselecting_EnterFinishes(t *testing.T) {
	v := newTestView(t)
	v.setDrawKind(connectPointsKind)
	d := v.currentDrawer
	tapAt(d, 20, 20)
	tapAt(d, 120, 20)
	if !v.hasClickPending() {
		t.Fatal("expected a pending shape")
	}
	v.typedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if v.hasClickPending() {
		t.Error("Escape should cancel the pending shape")
	}
	tapAt(d, 20, 20)
	tapAt(d, 120, 20)
	n := len(v.doc.Highlights)
	v.typedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if v.hasClickPending() || len(v.doc.Highlights) != n+1 {
		t.Errorf("Enter should finish: pending=%v highlights %d->%d", v.hasClickPending(), n, len(v.doc.Highlights))
	}
	if k := v.doc.Highlights[len(v.doc.Highlights)-1].Kind; k != "PolyLine" {
		t.Errorf("Kind = %q", k)
	}
}

func TestView_ChangingDrawKindOrPageDropsPendingShape(t *testing.T) {
	v := newTestView(t)
	v.setDrawKind(connectPointsKind)
	tapAt(v.currentDrawer, 20, 20)
	v.setDrawKind("Square")
	if v.hasClickPending() {
		t.Error("changing kind should drop the pending shape")
	}
}
