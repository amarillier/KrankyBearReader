package pdf

import (
	"image"
	"image/color"
	"math"
)

// resizeHandle identifies one draggable grip on the selected shape.
type resizeHandle int

const (
	handleNone resizeHandle = iota
	handleNW
	handleN
	handleNE
	handleE
	handleSE
	handleS
	handleSW
	handleW
	handleLineStart // a Line's first endpoint (/L[0:2])
	handleLineEnd   // a Line's second endpoint (/L[2:4])
	handleCalloutTip
)

const (
	// resizeHandleSizePt is a painted handle's side length in PDF points.
	resizeHandleSizePt = 7.0
	// resizeHandleHitPt is the half-extent around a handle's center that
	// counts as grabbing it — a little larger than the painted square.
	resizeHandleHitPt = 6.0
	// minResizeExtentPt stops a drag collapsing a shape to nothing.
	minResizeExtentPt = 4.0
)

type handleSpot struct {
	id   resizeHandle
	x, y float64 // PDF user-space points
}

// canResizeHighlight reports whether h can be resized: it must have
// geometry this app can scale (every kind it draws itself), and — for an
// already-saved one — be rebuildable by newAnnotationForShape, the same
// gate canMoveHighlight applies, since a resize persists through the same
// delete-and-reauthor path (a bare geometry rewrite can't change what a
// baked /AP paints).
func canResizeHighlight(h *Highlight) bool {
	if !canMoveHighlight(h) {
		return false
	}
	switch {
	case h.Kind == "Line":
		return len(h.Line) == 4
	case paintableMarkupKinds[h.Kind]:
		return len(h.Quads) > 0
	case authoredGenericKinds[h.Kind]:
		return h.Rect != [4]float64{}
	}
	return false
}

// resizeHandlesFor lists h's grips: a Line gets its two endpoints, every
// other kind the 8 bounding-box corners/edge midpoints, plus the callout
// tip for a speech bubble that has one in memory.
func resizeHandlesFor(h *Highlight) []handleSpot {
	var spots []handleSpot
	if h.Kind == "Line" && len(h.Line) == 4 {
		return []handleSpot{
			{handleLineStart, h.Line[0], h.Line[1]},
			{handleLineEnd, h.Line[2], h.Line[3]},
		}
	}
	if h.Kind == "FreeText" && h.CalloutTip != nil {
		spots = append(spots, handleSpot{handleCalloutTip, h.CalloutTip[0], h.CalloutTip[1]})
	}
	b, ok := highlightBoundsPt(h)
	if !ok {
		return spots
	}
	mx, my := (b[0]+b[2])/2, (b[1]+b[3])/2
	return append(spots,
		handleSpot{handleNW, b[0], b[3]}, handleSpot{handleNE, b[2], b[3]},
		handleSpot{handleSE, b[2], b[1]}, handleSpot{handleSW, b[0], b[1]},
		handleSpot{handleN, mx, b[3]}, handleSpot{handleS, mx, b[1]},
		handleSpot{handleE, b[2], my}, handleSpot{handleW, b[0], my},
	)
}

// hitResizeHandle returns the handle of h nearest to PDF point (x, y)
// within resizeHandleHitPt, or false. Earlier spots win ties (tip and
// corners before edges), so a tiny shape's overlapping handles stay
// predictable.
func hitResizeHandle(h *Highlight, x, y float64) (resizeHandle, bool) {
	best, bestDist := handleNone, math.MaxFloat64
	for _, s := range resizeHandlesFor(h) {
		dx, dy := math.Abs(s.x-x), math.Abs(s.y-y)
		if dx > resizeHandleHitPt || dy > resizeHandleHitPt {
			continue
		}
		if dist := dx*dx + dy*dy; dist < bestDist {
			best, bestDist = s.id, dist
		}
	}
	return best, best != handleNone
}

// resizedBoundsPt returns the bounding box h would have after dragging
// handle by (dx, dy) PDF points (dy positive = up) — used for both the
// live ghost outline and the real resize. Edges are clamped to
// minResizeExtentPt rather than flipping past the opposite edge.
func resizedBoundsPt(h *Highlight, handle resizeHandle, dx, dy float64) ([4]float64, bool) {
	switch handle {
	case handleLineStart, handleLineEnd:
		if len(h.Line) != 4 {
			return [4]float64{}, false
		}
		l := append([]float64(nil), h.Line...)
		if handle == handleLineStart {
			l[0] += dx
			l[1] += dy
		} else {
			l[2] += dx
			l[3] += dy
		}
		return [4]float64{min(l[0], l[2]), min(l[1], l[3]), max(l[0], l[2]), max(l[1], l[3])}, true
	case handleCalloutTip:
		b, ok := highlightBoundsPt(h)
		if !ok || h.CalloutTip == nil {
			return [4]float64{}, false
		}
		tx, ty := h.CalloutTip[0]+dx, h.CalloutTip[1]+dy
		return [4]float64{min(b[0], tx), min(b[1], ty), max(b[2], tx), max(b[3], ty)}, true
	}
	b, ok := highlightBoundsPt(h)
	if !ok {
		return [4]float64{}, false
	}
	west := handle == handleNW || handle == handleW || handle == handleSW
	east := handle == handleNE || handle == handleE || handle == handleSE
	north := handle == handleNW || handle == handleN || handle == handleNE
	south := handle == handleSW || handle == handleS || handle == handleSE
	if west {
		b[0] = min(b[0]+dx, b[2]-minResizeExtentPt)
	}
	if east {
		b[2] = max(b[2]+dx, b[0]+minResizeExtentPt)
	}
	if north {
		b[3] = max(b[3]+dy, b[1]+minResizeExtentPt)
	}
	if south {
		b[1] = min(b[1]+dy, b[3]-minResizeExtentPt)
	}
	return b, true
}

// resizeHighlightGeometry applies a handle drag to h's geometry. A Line
// endpoint or the callout tip just moves that point; any box handle maps
// every coordinate (Quads, Vertices, Rect) from the old bounding box into
// the new one, so multi-quad highlights and freehand strokes scale as a
// whole. The callout tip stays put when its box is resized — the user
// aimed it.
func resizeHighlightGeometry(h *Highlight, handle resizeHandle, dx, dy float64) {
	switch handle {
	case handleLineStart:
		h.Line[0] += dx
		h.Line[1] += dy
		return
	case handleLineEnd:
		h.Line[2] += dx
		h.Line[3] += dy
		return
	case handleCalloutTip:
		h.CalloutTip[0] += dx
		h.CalloutTip[1] += dy
		return
	}
	ob, ok := highlightBoundsPt(h)
	if !ok {
		return
	}
	nb, ok := resizedBoundsPt(h, handle, dx, dy)
	if !ok {
		return
	}
	mapX := scaleMapper(ob[0], ob[2], nb[0], nb[2])
	mapY := scaleMapper(ob[1], ob[3], nb[1], nb[3])
	for i := range h.Quads {
		for j := 0; j < 8; j += 2 {
			h.Quads[i][j] = mapX(h.Quads[i][j])
			h.Quads[i][j+1] = mapY(h.Quads[i][j+1])
		}
	}
	for i := range h.Vertices {
		h.Vertices[i][0] = mapX(h.Vertices[i][0])
		h.Vertices[i][1] = mapY(h.Vertices[i][1])
	}
	if h.Rect != [4]float64{} {
		h.Rect = [4]float64{mapX(h.Rect[0]), mapY(h.Rect[1]), mapX(h.Rect[2]), mapY(h.Rect[3])}
	}
}

// scaleMapper maps a coordinate linearly from [oldLo, oldHi] to
// [newLo, newHi]; a zero-length old span (e.g. a perfectly vertical
// stroke's width) can't be scaled, so it just shifts.
func scaleMapper(oldLo, oldHi, newLo, newHi float64) func(float64) float64 {
	span := oldHi - oldLo
	if span <= 0 {
		return func(v float64) float64 { return v - oldLo + newLo }
	}
	k := (newHi - newLo) / span
	return func(v float64) float64 { return newLo + (v-oldLo)*k }
}

// ResizeHighlight resizes h by dragging handle (dxPt, dyPt) PDF points —
// MoveHighlight's sibling, persisting through the same geometryMoved flag
// and rebuildDoc-for-saved-highlight path. Callers have already checked
// canResizeHighlight.
func (d *Document) ResizeHighlight(h *Highlight, handle resizeHandle, dxPt, dyPt float64) {
	resizeHighlightGeometry(h, handle, dxPt, dyPt)
	h.geometryMoved = true
	if h.ObjNr > 0 {
		_ = d.rebuildDoc()
	}
	d.invalidateRender()
}

// drawResizeHandles paints h's grips onto the page image: white squares
// with the selection color's outline, sized from resizeHandleSizePt so
// they scale with zoom.
func drawResizeHandles(img *image.RGBA, h *Highlight, pageHeightPt, scale float64) {
	half := max(int(resizeHandleSizePt*scale/2), 3)
	for _, s := range resizeHandlesFor(h) {
		cx, cy := int(s.x*scale), int((pageHeightPt-s.y)*scale)
		r := image.Rect(cx-half, cy-half, cx+half, cy+half)
		drawTranslucentRect(img, r, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		drawRectOutline(img, r, selectionOutlineColor, 2)
	}
}
