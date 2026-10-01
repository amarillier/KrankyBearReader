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
// Always wraps the image, even when Enabled is false: a plain *canvas.Image
// doesn't implement fyne.Draggable at all, so wrapping it costs nothing
// while highlight-drawing mode is off. Dragged/DragEnd still run in that
// state (see their own doc comments) — for click-to-select's sake, not
// drawing's: Fyne decides Tapped vs. Dragged+DragEnd purely by how much the
// pointer physically moved, independent of Enabled, so DragEnd needs to
// recognize a near-stationary "drag" and treat it as the tap it was, even
// with drawing mode off.
type highlightDrawer struct {
	widget.BaseWidget
	image   *canvas.Image
	overlay *canvas.Rectangle

	Enabled bool
	// OnDrawn fires once a drag ends with a non-trivial size, with the
	// gesture's own RAW start and end points (not normalized into
	// top-left/bottom-right — direction is preserved, e.g. a Line/arrow
	// shape needs to know which end the user actually dragged FROM versus
	// TO) plus the image's own natural size, both already corrected for
	// any centering offset (see imageLocalPos) — the caller normalizes
	// into a rectangle itself for any shape kind that doesn't care about
	// direction (widgetRectToQuad's own callers do this via
	// rectTopLeft/rectBottomRight).
	OnDrawn func(start, end fyne.Position, widgetSize fyne.Size)

	// OnTapped fires on a plain click (no drag) — a click-to-select
	// gesture, independent of Enabled/Draw Highlight mode: selecting an
	// existing highlight and drawing a new one are different actions, and
	// Fyne itself already tells the two gestures apart (a real drag beyond
	// a small threshold calls Dragged/DragEnd instead of Tapped, never
	// both, so there's no risk of a draw also firing this). Same
	// coordinate space as OnDrawn.
	OnTapped func(pos fyne.Position, widgetSize fyne.Size)

	// OnTappedSecondary fires on a right-click (or long-press) — same
	// click-to-select gesture as OnTapped (pos/widgetSize are the same
	// coordinate space), plus absolutePos (canvas-space, from the
	// triggering *fyne.PointEvent's own AbsolutePosition) for positioning a
	// context menu right at the cursor.
	OnTappedSecondary func(pos fyne.Position, widgetSize fyne.Size, absolutePos fyne.Position)

	// HitTestSelected, if set, is consulted at the START of a drag — but
	// only while Enabled is false, since moving an existing shape and
	// drawing a new one are mutually exclusive modes — to decide whether
	// the drag repositions the currently-selected highlight instead of
	// doing nothing (the prior behavior for any non-trivial drag while
	// Draw mode was off: DragEnd's own "too small for a rectangle, too big
	// for a tap" fallthrough). pos is in this widget's own image-local
	// space (the same corrected space OnTapped/OnDrawn already receive).
	// Returns the hit highlight's own current bounding box, ALSO in
	// image-local space, for the live ghost overlay while dragging
	// (Dragged converts it to this widget's outer space itself — see
	// outerPos) — and whether pos actually landed on it at all.
	HitTestSelected func(pos fyne.Position, widgetSize fyne.Size) (tl, br fyne.Position, ok bool)

	// OnMoved fires once a move-drag (see HitTestSelected/moving) ends
	// with a non-trivial size, with the gesture's own start/end points and
	// the image's own natural size — same shape and coordinate-correction
	// as OnDrawn, so the caller computes a PDF-space delta the same way it
	// already computes PDF-space geometry there.
	OnMoved func(start, end fyne.Position, widgetSize fyne.Size)

	// FreehandMode, while Enabled is also true, switches Dragged/DragEnd
	// from "drag out a rectangle" (OnDrawn, start+end only) to "trace a
	// freehand path" (OnFreehandDrawn, every sampled point along the
	// drag) — set by the view alongside Enabled whenever the selected
	// Draw kind is "Ink" (see setDrawKind). The live overlay still only
	// ever shows the drag's own bounding-box rectangle, same as every
	// other kind — see OnFreehandDrawn's own doc comment for why a
	// kind-accurate live preview isn't needed here either.
	FreehandMode bool

	// OnFreehandDrawn fires once a freehand drag ends with a non-trivial
	// size, with every point Dragged sampled along the gesture (already
	// corrected for image-centering padding, the same space OnDrawn's own
	// start/end arrive in) plus the image's own natural size. Unlike
	// OnDrawn, a single-axis drag is accepted (a deliberate straight
	// vertical or horizontal pen stroke is a perfectly normal thing to
	// draw, unlike a single-axis rectangle, which can't form a sensible
	// shape at all).
	OnFreehandDrawn func(points []fyne.Position, widgetSize fyne.Size)

	dragging   bool
	moving     bool
	start, cur fyne.Position
	moveOrigTL fyne.Position
	moveOrigBR fyne.Position
	path       []fyne.Position
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
//
// Always tracks the gesture's own start/current position, regardless of
// Enabled — DragEnd needs that even while Draw Highlight mode is off, to
// tell a near-stationary "drag" (Fyne's own jitter tolerance, not this
// app's) apart from one Fyne never delivered at all. The live-preview
// overlay serves two different purposes depending on mode, decided once
// at the start of the gesture: while Enabled (Draw mode on), it's the
// rectangle-being-drawn, as it always was; while off, HitTestSelected (if
// set) gets one chance to turn this into a MOVE of the currently-selected
// highlight instead — a real, sizeable drag while Draw mode was off
// otherwise still does nothing, same as before this existed. While
// moving, the SAME overlay rectangle is reused as a ghost of the
// selected highlight's own bounding box, translated live by however far
// the drag has moved so far — cheap, real-time feedback without needing
// to re-render the actual page on every pointer-move event; the real
// geometry only changes once, in DragEnd, via OnMoved.
func (d *highlightDrawer) Dragged(e *fyne.DragEvent) {
	if !d.dragging {
		d.dragging = true
		d.start = fyne.NewPos(e.Position.X-e.Dragged.DX, e.Position.Y-e.Dragged.DY)
		d.moving = false
		d.path = nil
		if !d.Enabled && d.HitTestSelected != nil {
			localStart, size := d.imageLocalPos(d.start)
			if tl, br, ok := d.HitTestSelected(localStart, size); ok {
				d.moving = true
				d.moveOrigTL = d.outerPos(tl)
				d.moveOrigBR = d.outerPos(br)
			}
		}
		if d.Enabled && d.FreehandMode {
			d.path = append(d.path, d.start)
		}
	}
	d.cur = e.Position
	if d.Enabled && d.FreehandMode {
		// Skip a duplicate of the last recorded point — the very first
		// call's own e.Position often exactly equals d.start (whenever
		// Fyne's first delivered event for this gesture carried zero
		// initial delta), which would otherwise record the anchor twice.
		if len(d.path) == 0 || d.path[len(d.path)-1] != d.cur {
			d.path = append(d.path, d.cur)
		}
	}
	switch {
	case d.Enabled:
		d.overlay.Move(rectTopLeft(d.start, d.cur))
		d.overlay.Resize(rectSize(d.start, d.cur))
	case d.moving:
		dx, dy := d.cur.X-d.start.X, d.cur.Y-d.start.Y
		tl := fyne.NewPos(d.moveOrigTL.X+dx, d.moveOrigTL.Y+dy)
		br := fyne.NewPos(d.moveOrigBR.X+dx, d.moveOrigBR.Y+dy)
		d.overlay.Move(tl)
		d.overlay.Resize(fyne.NewSize(br.X-tl.X, br.Y-tl.Y))
	}
}

// minDragPts guards against an accidental click-with-tiny-jitter being
// treated as a deliberate highlight — and, since DragEnd below, is also
// the threshold for treating that same tiny jitter as the tap it actually
// was.
const minDragPts = 4

// DragEnd finalizes the drag: fires OnMoved if the drag began a move (see
// Dragged/HitTestSelected) and isn't near-stationary; otherwise fires
// OnFreehandDrawn if FreehandMode is on (and Enabled); otherwise fires
// OnDrawn if it's big enough in BOTH dimensions to be a deliberate
// highlight rectangle (and Enabled); fires OnTapped instead if it's small
// in BOTH dimensions (see below); does nothing for a real, sizeable
// movement in only one axis while drawing a rectangle (neither a
// rectangle nor a tap) — a move or a freehand stroke, unlike drawing a
// rectangle, accepts a single-axis delta just fine, since a translation
// or a straight pen stroke is well-defined regardless of which axes
// actually moved. Fyne calls DragEnd at the end of every drag gesture,
// even one this widget never started reacting to (Dragged never ran) —
// the dragging guard makes that a no-op.
//
// The near-stationary -> OnTapped fallback is not optional polish: Fyne
// decides whether a given click-and-release is a Tapped or a
// Dragged+DragEnd gesture purely by how much the pointer physically moved
// during the press — a decision this widget has no say in and that
// happens BEFORE Tapped/Dragged is even called, regardless of Enabled or
// what either handler does. A real click with a hair of jitter (common on
// a trackpad -- and, non-obviously, MORE likely for a careful, deliberate
// click on a small/precise target than a quick tap on a big one) gets
// routed through Dragged/DragEnd instead of Tapped. Before this fix,
// DragEnd's own "too small to be deliberate" check just returned without
// doing anything else -- meaning Tapped's whole click-to-select gesture
// silently never fired for that click at all, no error, no visual
// feedback, nothing. Found via real user testing, not speculatively: a
// thin Line annotation's on-page click-to-select was wildly unreliable --
// "click all over ... can't get it to select" -- succeeding only on an
// occasional, perfectly jitter-free click, while the Highlights panel's
// own list-based selection (an unrelated gesture path) worked every time.
func (d *highlightDrawer) DragEnd() {
	if !d.dragging {
		return
	}
	d.dragging = false
	wasMoving := d.moving
	d.moving = false
	tl, br := rectTopLeft(d.start, d.cur), rectBottomRight(d.start, d.cur)
	d.overlay.Resize(fyne.NewSize(0, 0))

	dx, dy := br.X-tl.X, br.Y-tl.Y
	if dx < minDragPts && dy < minDragPts {
		if d.OnTapped != nil {
			pos, size := d.imageLocalPos(d.cur)
			d.OnTapped(pos, size)
		}
		return
	}
	if wasMoving {
		if d.OnMoved != nil {
			// Raw d.start/d.cur, same as OnDrawn below — the caller
			// computes its own PDF-space delta from the two points.
			startPos, size := d.imageLocalPos(d.start)
			endPos, _ := d.imageLocalPos(d.cur)
			d.OnMoved(startPos, endPos, size)
		}
		return
	}
	if d.Enabled && d.FreehandMode {
		if d.OnFreehandDrawn != nil {
			points := make([]fyne.Position, len(d.path))
			var size fyne.Size
			for i, p := range d.path {
				points[i], size = d.imageLocalPos(p)
			}
			d.OnFreehandDrawn(points, size)
		}
		return
	}
	if dx < minDragPts || dy < minDragPts || !d.Enabled {
		return
	}
	if d.OnDrawn != nil {
		// Raw d.start/d.cur, NOT the normalized tl/br above (those exist
		// only for the magnitude check just above) — see OnDrawn's own
		// doc comment for why direction matters to some callers.
		startPos, size := d.imageLocalPos(d.start)
		endPos, _ := d.imageLocalPos(d.cur)
		d.OnDrawn(startPos, endPos, size)
	}
}

// imageLocalPos converts pos (this widget's own local coordinate space,
// i.e. relative to d's top-left corner — the same space Fyne delivers
// every pointer event in) into the underlying image's own local
// coordinate space, and returns the image's own natural size alongside it
// — the correct pair to feed widgetPointToPDF/widgetRectToQuad, INSTEAD
// of pos/d.Size() directly.
//
// Necessary because d's own Size() and the image's own displayed size can
// genuinely differ: image.FillMode is ImageFillOriginal, which — per its
// own doc comment, behaving like ImageFillContain — centers the image
// within whatever bounds it's given, with transparent padding on the
// sides that overflow, whenever those bounds exceed the image's own
// natural size (tracked via SetMinSize, by applyFitWidth/applyFitPage).
// And a container.Scroll's own renderer (internal/widget/scroller.go)
// unconditionally resizes its Content to
// internal.MaxSizes(content.MinSize(), viewportSize) — the LARGER of the
// two in each dimension — so d ends up padded (and its own Size() bigger
// than the image's natural size) any time the scroll viewport exceeds the
// current Fit-Width/Fit-Page image size in either dimension, which is
// routine (e.g. any page whose fitted height is less than the window's
// own height). Confirmed via real hands-on testing, not just reading
// Fyne's source: clicking exactly on a thin Line annotation's own visible
// pixels missed it entirely, only landing correctly well off to the side
// — exactly what an uncorrected centering offset produces, worse for
// anything not near the image's own center.
func (d *highlightDrawer) imageLocalPos(pos fyne.Position) (fyne.Position, fyne.Size) {
	imgSize := d.image.MinSize()
	widgetSize := d.Size()
	padX := (widgetSize.Width - imgSize.Width) / 2
	padY := (widgetSize.Height - imgSize.Height) / 2
	return fyne.NewPos(pos.X-padX, pos.Y-padY), imgSize
}

// outerPos is imageLocalPos's inverse for a position only — converts an
// image-local point back into this widget's own OUTER coordinate space,
// the same space Dragged/DragEnd's own d.start/d.cur and the live overlay
// rectangle already use. Needed only for the move-drag ghost overlay
// (HitTestSelected's own returned bounds arrive in image-local space, same
// as every other callback's own pos argument, but the overlay itself is
// positioned in outer space — see Dragged's own doc comment).
func (d *highlightDrawer) outerPos(pos fyne.Position) fyne.Position {
	imgSize := d.image.MinSize()
	widgetSize := d.Size()
	padX := (widgetSize.Width - imgSize.Width) / 2
	padY := (widgetSize.Height - imgSize.Height) / 2
	return fyne.NewPos(pos.X+padX, pos.Y+padY)
}

// Tapped fires OnTapped — see its own doc comment for why this never
// conflicts with a drag gesture.
func (d *highlightDrawer) Tapped(e *fyne.PointEvent) {
	if d.OnTapped != nil {
		pos, size := d.imageLocalPos(e.Position)
		d.OnTapped(pos, size)
	}
}

// TappedSecondary fires OnTappedSecondary, implementing fyne.SecondaryTappable
// so a right-click (or long-press) reaches this widget at all.
func (d *highlightDrawer) TappedSecondary(e *fyne.PointEvent) {
	if d.OnTappedSecondary != nil {
		pos, size := d.imageLocalPos(e.Position)
		d.OnTappedSecondary(pos, size, e.AbsolutePosition)
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

// pdfRectToWidget converts a PDF-space rectangle (origin bottom-left) into
// a widget-local rectangle (points, origin top-left, sized to widgetSize)
// — the exact inverse of widgetRectToQuad's own scale-and-flip-Y. Used by
// the Move gesture (view_render.go's handleHighlightMoveHitTest) to show
// the selected highlight's own bounding box as a ghost outline while
// dragging.
func pdfRectToWidget(rect [4]float64, widgetSize fyne.Size, pageW, pageH float64) (topLeft, bottomRight fyne.Position) {
	sx := float64(widgetSize.Width) / pageW
	sy := float64(widgetSize.Height) / pageH
	minX, minY, maxX, maxY := rect[0], rect[1], rect[2], rect[3]
	topLeft = fyne.NewPos(float32(minX*sx), float32((pageH-maxY)*sy))
	bottomRight = fyne.NewPos(float32(maxX*sx), float32((pageH-minY)*sy))
	return topLeft, bottomRight
}
