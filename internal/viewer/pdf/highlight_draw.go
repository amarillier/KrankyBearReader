package pdf

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// highlightDrawer wraps a *canvas.Image so the user can click-drag a
// rectangle directly on the rendered page to create a new highlight.
// go-fitz has no word/line bounding-box API (see ReleaseNotes' Future
// ideas), so unlike Preview/Acrobat's text-snapped selection, this is a
// free rectangle the user positions and sizes by hand — OnDrawn converts it
// to a single-quad PDF highlight once the drag ends (see view.go's
// wireHighlightDrawing).
//
// Always wraps the image, even when Enabled is false: dragging on a plain
// *canvas.Image was already a no-op before this widget existed (Image
// doesn't implement fyne.Draggable), so wrapping it costs nothing while
// highlight-drawing mode is off — Dragged/DragEnd just return immediately,
// same as today's behavior.
type highlightDrawer struct {
	widget.BaseWidget
	image   *canvas.Image
	overlay *canvas.Rectangle

	Enabled bool
	// OnDrawn fires once a drag ends with a non-trivial size, with the
	// rectangle's two corners plus this widget's current Size(), all in
	// this widget's own coordinate space (points, matching image.Size() —
	// whatever MinSize the page is currently displayed at, not the
	// underlying image's raw pixel size) — see widgetRectToQuad.
	OnDrawn func(topLeft, bottomRight fyne.Position, widgetSize fyne.Size)

	// OnTapped fires on a plain click (no drag) — a click-to-select
	// gesture, independent of Enabled/Draw Highlight mode: selecting an
	// existing highlight and drawing a new one are different actions, and
	// Fyne itself already tells the two gestures apart (a real drag beyond
	// a small threshold calls Dragged/DragEnd instead of Tapped, never
	// both, so there's no risk of a draw also firing this). Same
	// coordinate space as OnDrawn.
	OnTapped func(pos fyne.Position, widgetSize fyne.Size)

	dragging   bool
	start, cur fyne.Position
}

func newHighlightDrawer(img *canvas.Image) *highlightDrawer {
	d := &highlightDrawer{image: img}
	d.overlay = canvas.NewRectangle(color.NRGBA{R: 255, G: 220, B: 0, A: 90})
	d.overlay.StrokeColor = color.NRGBA{R: 180, G: 150, B: 0, A: 255}
	d.overlay.StrokeWidth = 1
	// Deliberately never Hide()/Show() the overlay — see Dragged/DragEnd.
	d.ExtendBaseWidget(d)
	return d
}

func (d *highlightDrawer) CreateRenderer() fyne.WidgetRenderer {
	return &highlightDrawerRenderer{d: d}
}

// Dragged tracks a click-drag in progress and live-updates the overlay
// rectangle; DragEnd is what actually creates the highlight. The start
// corner is recovered from the first Dragged call's own Position/Dragged
// pair (Position is where the pointer is now; Dragged is how far it moved
// to get here since the previous event — for the first call in a gesture,
// that "previous" point is the drag's true anchor), the standard Fyne
// idiom for a widget that draws a shape from a drag rather than moving
// something.
//
// The overlay is shown/hidden by resizing it to/from zero, never via
// Hide()/Show(). Found the hard way (manual testing): a Rectangle that
// starts life Hidden, as this one did originally, never gets registered in
// Fyne's internal object->canvas cache (cache.SetCanvasForObject — only
// populated by the render-cache tree walk in
// internal/driver/common/canvas.go, whose per-node setup skips an invisible
// node) until something ELSE marks the canvas dirty for an unrelated
// reason — so calling Show()+Move()+Resize() on it while dragging updated
// its in-memory state correctly but never actually painted anything, since
// repaint(obj)/canvas.Refresh(obj) look that object up in the very cache
// that was never populated. A Rectangle that's always nominally "visible"
// but zero-sized gets registered on the very first real layout pass (the
// page image's own first render), so by the time a drag ever starts, its
// canvas is already known and Resize's own built-in repaint call (see
// fyne's canvas.Rectangle.Resize) reliably reaches the screen.
func (d *highlightDrawer) Dragged(e *fyne.DragEvent) {
	if !d.Enabled {
		return
	}
	if !d.dragging {
		d.dragging = true
		d.start = fyne.NewPos(e.Position.X-e.Dragged.DX, e.Position.Y-e.Dragged.DY)
	}
	d.cur = e.Position
	d.overlay.Move(rectTopLeft(d.start, d.cur))
	d.overlay.Resize(rectSize(d.start, d.cur))
}

// minDragPts guards against an accidental click-with-tiny-jitter being
// treated as a deliberate highlight.
const minDragPts = 4

// DragEnd finalizes the drag, firing OnDrawn if it's big enough to be
// deliberate. Fyne calls DragEnd at the end of every drag gesture, even one
// this widget never started reacting to (mode was off, or Dragged never
// ran) — the dragging guard makes that a no-op.
func (d *highlightDrawer) DragEnd() {
	if !d.dragging {
		return
	}
	d.dragging = false
	tl, br := rectTopLeft(d.start, d.cur), rectBottomRight(d.start, d.cur)
	d.overlay.Resize(fyne.NewSize(0, 0))

	if br.X-tl.X < minDragPts || br.Y-tl.Y < minDragPts {
		return
	}
	if d.OnDrawn != nil {
		d.OnDrawn(tl, br, d.Size())
	}
}

// Tapped fires OnTapped — see its own doc comment for why this never
// conflicts with a drag gesture.
func (d *highlightDrawer) Tapped(e *fyne.PointEvent) {
	if d.OnTapped != nil {
		d.OnTapped(e.Position, d.Size())
	}
}

func rectTopLeft(a, b fyne.Position) fyne.Position {
	return fyne.NewPos(min(a.X, b.X), min(a.Y, b.Y))
}

func rectBottomRight(a, b fyne.Position) fyne.Position {
	return fyne.NewPos(max(a.X, b.X), max(a.Y, b.Y))
}

func rectSize(a, b fyne.Position) fyne.Size {
	tl, br := rectTopLeft(a, b), rectBottomRight(a, b)
	return fyne.NewSize(br.X-tl.X, br.Y-tl.Y)
}

// highlightDrawerRenderer just stacks the image and the (usually zero-
// sized, see Dragged/DragEnd) live-drag overlay rectangle, filling whatever
// size the widget is given — the same "resize child to fill" contract
// v.currentImage always had directly, now one layer removed.
type highlightDrawerRenderer struct {
	d *highlightDrawer
}

func (r *highlightDrawerRenderer) Layout(size fyne.Size) {
	r.d.image.Resize(size)
	r.d.image.Move(fyne.NewPos(0, 0))
}

func (r *highlightDrawerRenderer) MinSize() fyne.Size { return r.d.image.MinSize() }
func (r *highlightDrawerRenderer) Refresh()           {}
func (r *highlightDrawerRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.d.image, r.d.overlay}
}
func (r *highlightDrawerRenderer) Destroy() {}

// widgetRectToQuad converts a rectangle drawn in a highlightDrawer's own
// widget-local coordinate space (points, origin top-left, sized to
// widgetSize — see highlightDrawer.OnDrawn) into a single PDF /QuadPoints
// quad in user-space points (origin bottom-left), given the page's real
// size in points (pageW, pageH — see Document.PageBoundsPt). Purely a
// ratio between the two sizes, so it works regardless of the actual render
// DPI: however the image is currently scaled to fit widgetSize, this maps
// back through exactly that same scaling.
func widgetRectToQuad(topLeft, bottomRight fyne.Position, widgetSize fyne.Size, pageW, pageH float64) [8]float64 {
	sx := pageW / float64(widgetSize.Width)
	sy := pageH / float64(widgetSize.Height)

	llx, urx := float64(topLeft.X)*sx, float64(bottomRight.X)*sx
	// Flip Y: widget space is top-left origin, PDF space is bottom-left.
	ury := pageH - float64(topLeft.Y)*sy
	lly := pageH - float64(bottomRight.Y)*sy

	return [8]float64{llx, ury, urx, ury, llx, lly, urx, lly}
}

// widgetPointToPDF is widgetRectToQuad's single-point counterpart, used to
// hit-test a tap (see highlightDrawer.OnTapped) against Document.HighlightAt.
func widgetPointToPDF(pos fyne.Position, widgetSize fyne.Size, pageW, pageH float64) (x, y float64) {
	sx := pageW / float64(widgetSize.Width)
	sy := pageH / float64(widgetSize.Height)
	x = float64(pos.X) * sx
	y = pageH - float64(pos.Y)*sy
	return x, y
}
