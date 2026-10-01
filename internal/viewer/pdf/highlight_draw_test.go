package pdf

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// dragGesture simulates one full click-drag-release gesture (Dragged called
// once, then DragEnd), the same sequence Fyne delivers for a real pointer
// gesture it classifies as a drag rather than a tap — which, as this file's
// own tests below cover, includes a real click with a hair of physical
// jitter, not just a deliberate drag.
func dragGesture(d *highlightDrawer, startX, startY, dx, dy float32) {
	d.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(startX+dx, startY+dy)},
		Dragged:    fyne.Delta{DX: dx, DY: dy},
	})
	d.DragEnd()
}

// newTestHighlightDrawer builds a drawer with its image's MinSize and the
// drawer's own widget Size pinned to the SAME value, so imageLocalPos's
// padding correction (see its own doc comment) is a no-op — keeping these
// gesture-classification tests focused on drag-vs-tap behavior, not
// incidentally exercised by Fyne's own default widget/image sizes
// happening to differ by a pixel or two.
func newTestHighlightDrawer() *highlightDrawer {
	d := newHighlightDrawer(canvas.NewImageFromImage(nil))
	size := fyne.NewSize(400, 400)
	d.image.SetMinSize(size)
	d.Resize(size)
	return d
}

// TestHighlightDrawer_NearStationaryDragFiresTapped is the real regression
// test for a bug found via hands-on testing: Fyne decides whether a given
// click-and-release is delivered as Tapped or as Dragged+DragEnd purely by
// how much the pointer physically moved during the press -- independent of
// Enabled, and independent of anything this widget's own handlers do. A
// real click with a hair of jitter (common on a trackpad, and more likely
// for a careful, precise click on a small target than a quick tap on a big
// one) was getting routed through Dragged/DragEnd, whose own "too small to
// be a deliberate highlight" check just discarded it -- so Tapped's whole
// click-to-select gesture silently never fired at all. Covers both Enabled
// states, since OnTapped must fire independent of Draw Highlight mode.
func TestHighlightDrawer_NearStationaryDragFiresTapped(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var tappedAt fyne.Position
		tapped := false
		drawn := false

		d := newTestHighlightDrawer()
		d.Enabled = enabled
		d.OnTapped = func(pos fyne.Position, _ fyne.Size) { tapped, tappedAt = true, pos }
		d.OnDrawn = func(_, _ fyne.Position, _ fyne.Size) { drawn = true }

		dragGesture(d, 100, 100, minDragPts-1, minDragPts-1)

		if !tapped {
			t.Errorf("Enabled=%v: expected a near-stationary drag to fire OnTapped, it didn't", enabled)
		}
		if drawn {
			t.Errorf("Enabled=%v: expected a near-stationary drag NOT to fire OnDrawn", enabled)
		}
		wantX, wantY := float32(100+minDragPts-1), float32(100+minDragPts-1)
		if tapped && (tappedAt.X != wantX || tappedAt.Y != wantY) {
			t.Errorf("Enabled=%v: OnTapped fired at %v, want (%v,%v)", enabled, tappedAt, wantX, wantY)
		}
	}
}

// TestHighlightDrawer_DeliberateDragFiresOnDrawnOnlyWhenEnabled confirms a
// real, sizeable drag in both dimensions still behaves as before: it draws
// a new highlight when Draw Highlight mode is on, and does nothing at all
// (neither draws nor falls back to a tap -- real, deliberate movement
// happened) when it's off.
func TestHighlightDrawer_DeliberateDragFiresOnDrawnOnlyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		tapped, drawn := false, false

		d := newTestHighlightDrawer()
		d.Enabled = enabled
		d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }
		d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }

		dragGesture(d, 50, 50, 40, 40)

		if drawn != enabled {
			t.Errorf("Enabled=%v: OnDrawn fired=%v, want %v", enabled, drawn, enabled)
		}
		if tapped {
			t.Errorf("Enabled=%v: expected a deliberate drag NOT to fire OnTapped", enabled)
		}
	}
}

// TestHighlightDrawer_ThinSingleAxisDragFiresNeither covers the remaining
// ambiguous case: real movement in only one axis (e.g. a shaky near-
// horizontal drag) is neither a deliberate rectangle nor a stationary tap,
// so nothing should fire.
func TestHighlightDrawer_ThinSingleAxisDragFiresNeither(t *testing.T) {
	tapped, drawn := false, false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }

	dragGesture(d, 50, 50, 40, minDragPts-1)

	if tapped || drawn {
		t.Errorf("expected neither OnTapped nor OnDrawn for a thin single-axis drag, got tapped=%v drawn=%v", tapped, drawn)
	}
}

// TestHighlightDrawer_PlainTapAlwaysFiresRegardlessOfEnabled confirms the
// ordinary, un-jittered Tapped path (Fyne calling Tapped directly, no
// Dragged/DragEnd involved at all) is untouched by any of the above.
func TestHighlightDrawer_PlainTapAlwaysFiresRegardlessOfEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		tapped := false
		d := newTestHighlightDrawer()
		d.Enabled = enabled
		d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }

		d.Tapped(&fyne.PointEvent{Position: fyne.NewPos(10, 10)})

		if !tapped {
			t.Errorf("Enabled=%v: expected a plain Tapped call to fire OnTapped", enabled)
		}
	}
}

// TestHighlightDrawer_TappedCorrectsForImageCenteringPadding is the real
// regression test for a bug found via hands-on testing on an actual PDF:
// clicking exactly on a thin Line annotation's own visible pixels missed
// it entirely, only registering well off to one side. Root cause:
// d.image's FillMode (ImageFillOriginal) centers the image within
// whatever bounds container.Scroll's own layout resizes this widget to —
// which routinely ends up LARGER than the image's own Fit-Width/Fit-Page
// natural size (container.Scroll always resizes content to
// internal.MaxSizes(content.MinSize(), viewportSize), the bigger of the
// two in each dimension) — leaving transparent padding on the sides that
// overflow. Tapped/TappedSecondary/DragEnd must translate a click's
// widget-local position into the image's OWN local space (undoing that
// centering offset) before handing it to OnTapped/OnDrawn, or every
// downstream PDF-space coordinate conversion inherits the same offset.
func TestHighlightDrawer_TappedCorrectsForImageCenteringPadding(t *testing.T) {
	d := newHighlightDrawer(canvas.NewImageFromImage(nil))
	// Image's own natural (Fit-Width) size is 300x200, but the widget
	// (this widget stands in for what container.Scroll's MaxSizes-based
	// stretching produces) got resized to a larger 400x300 -- 50pt of
	// padding on each side horizontally, 50pt vertically, since
	// ImageFillOriginal centers rather than anchoring at the origin.
	d.image.SetMinSize(fyne.NewSize(300, 200))
	d.Resize(fyne.NewSize(400, 300))

	var gotPos fyne.Position
	var gotSize fyne.Size
	d.OnTapped = func(pos fyne.Position, size fyne.Size) { gotPos, gotSize = pos, size }

	// A tap at the widget-local center (200,150) should land at the
	// image's own local center (150,100), not (200,150).
	d.Tapped(&fyne.PointEvent{Position: fyne.NewPos(200, 150)})

	if gotSize != fyne.NewSize(300, 200) {
		t.Errorf("OnTapped got size %v, want the image's own natural size (300,200)", gotSize)
	}
	if gotPos != fyne.NewPos(150, 100) {
		t.Errorf("OnTapped got pos %v, want the padding-corrected (150,100)", gotPos)
	}
}

// TestHighlightDrawer_TappedNoCorrectionWhenSizesMatch confirms the common
// case (no scroll-stretch padding at all, image and widget the same size)
// is a no-op passthrough, not just coincidentally correct in the padded
// case above.
func TestHighlightDrawer_TappedNoCorrectionWhenSizesMatch(t *testing.T) {
	d := newTestHighlightDrawer() // pins image MinSize == widget Size, see its own doc comment

	var gotPos fyne.Position
	d.OnTapped = func(pos fyne.Position, _ fyne.Size) { gotPos = pos }

	d.Tapped(&fyne.PointEvent{Position: fyne.NewPos(37, 42)})

	if gotPos != fyne.NewPos(37, 42) {
		t.Errorf("OnTapped got pos %v, want the untranslated (37,42)", gotPos)
	}
}

// matchingHitTest returns a HitTestSelected stand-in that always reports a
// hit at the given bounds — good enough for these gesture-classification
// tests, which only care whether OnMoved fires, not the real per-kind
// bounding-box math (covered separately in annotations_test.go's own
// TestHighlightBoundsPt* and view_render.go's handleHighlightMoveHitTest).
func matchingHitTest(tl, br fyne.Position) func(fyne.Position, fyne.Size) (fyne.Position, fyne.Position, bool) {
	return func(fyne.Position, fyne.Size) (fyne.Position, fyne.Position, bool) {
		return tl, br, true
	}
}

// TestHighlightDrawer_MoveDragFiresOnMovedWhenHitTestSelectedMatches covers
// the gap a real drag on the page used to fall into: Draw mode off, a real
// (non-stationary) drag, previously always a no-op (see
// TestHighlightDrawer_DeliberateDragFiresOnDrawnOnlyWhenEnabled's own
// Enabled=false case). Now, if HitTestSelected reports the drag started on
// the selected highlight, it's a move instead — OnMoved fires with the raw
// start/end points, not OnDrawn or OnTapped.
func TestHighlightDrawer_MoveDragFiresOnMovedWhenHitTestSelectedMatches(t *testing.T) {
	var moved, drawn, tapped bool
	var gotStart, gotEnd fyne.Position

	d := newTestHighlightDrawer()
	d.Enabled = false
	d.HitTestSelected = matchingHitTest(fyne.NewPos(40, 40), fyne.NewPos(60, 60))
	d.OnMoved = func(start, end fyne.Position, _ fyne.Size) { moved, gotStart, gotEnd = true, start, end }
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }

	dragGesture(d, 50, 50, 40, 40)

	if !moved {
		t.Fatal("expected a deliberate drag starting on the selected highlight to fire OnMoved")
	}
	if drawn || tapped {
		t.Errorf("expected only OnMoved to fire, got drawn=%v tapped=%v", drawn, tapped)
	}
	if gotStart != fyne.NewPos(50, 50) || gotEnd != fyne.NewPos(90, 90) {
		t.Errorf("OnMoved start/end = %v/%v, want (50,50)/(90,90)", gotStart, gotEnd)
	}
}

// TestHighlightDrawer_MoveDragDoesNothingWhenHitTestSelectedMisses confirms
// the pre-Move behavior survives for a drag that does NOT start on the
// selected highlight: still a no-op, exactly like before HitTestSelected
// existed at all.
func TestHighlightDrawer_MoveDragDoesNothingWhenHitTestSelectedMisses(t *testing.T) {
	var moved, drawn, tapped bool

	d := newTestHighlightDrawer()
	d.Enabled = false
	d.HitTestSelected = func(fyne.Position, fyne.Size) (fyne.Position, fyne.Position, bool) {
		return fyne.Position{}, fyne.Position{}, false
	}
	d.OnMoved = func(fyne.Position, fyne.Position, fyne.Size) { moved = true }
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }

	dragGesture(d, 50, 50, 40, 40)

	if moved || drawn || tapped {
		t.Errorf("expected nothing to fire for a deliberate drag missing the selected highlight, got moved=%v drawn=%v tapped=%v", moved, drawn, tapped)
	}
}

// TestHighlightDrawer_MoveNearStationaryFiresTappedNotMoved confirms a tiny
// jittery click that happens to land on the selected highlight is still
// treated as a plain re-select tap, not a (no-op, zero-distance) move —
// matching TestHighlightDrawer_NearStationaryDragFiresTapped's own
// reasoning, just with HitTestSelected now also matching.
func TestHighlightDrawer_MoveNearStationaryFiresTappedNotMoved(t *testing.T) {
	var moved, tapped bool

	d := newTestHighlightDrawer()
	d.Enabled = false
	d.HitTestSelected = matchingHitTest(fyne.NewPos(40, 40), fyne.NewPos(60, 60))
	d.OnMoved = func(fyne.Position, fyne.Position, fyne.Size) { moved = true }
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }

	dragGesture(d, 100, 100, minDragPts-1, minDragPts-1)

	if !tapped {
		t.Error("expected a near-stationary drag on the selected highlight to fire OnTapped")
	}
	if moved {
		t.Error("expected a near-stationary drag NOT to fire OnMoved")
	}
}

// TestHighlightDrawer_DrawModeTakesPriorityOverMove confirms Draw mode
// (Enabled) and Move are mutually exclusive, as designed: with Enabled
// true, HitTestSelected is never even consulted (see Dragged's own
// `!d.Enabled` guard) — a deliberate drag always draws a new shape,
// regardless of whether it happens to start on the currently-selected one.
func TestHighlightDrawer_DrawModeTakesPriorityOverMove(t *testing.T) {
	var moved, drawn bool
	hitTestCalled := false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.HitTestSelected = func(fyne.Position, fyne.Size) (fyne.Position, fyne.Position, bool) {
		hitTestCalled = true
		return fyne.NewPos(40, 40), fyne.NewPos(60, 60), true
	}
	d.OnMoved = func(fyne.Position, fyne.Position, fyne.Size) { moved = true }
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }

	dragGesture(d, 50, 50, 40, 40)

	if hitTestCalled {
		t.Error("expected HitTestSelected not to be consulted while Draw mode (Enabled) is on")
	}
	if !drawn || moved {
		t.Errorf("expected OnDrawn only, got drawn=%v moved=%v", drawn, moved)
	}
}

// TestHighlightDrawer_MoveGhostOverlayTracksDragDelta confirms the live
// ghost overlay shown while moving is the selected highlight's own
// starting bounds translated by however far the drag has moved so far —
// not the draw-a-new-rectangle overlay's own "from start to current
// point" shape, which would be wrong for a move (the ghost must keep the
// shape's own original size, just slide it).
func TestHighlightDrawer_MoveGhostOverlayTracksDragDelta(t *testing.T) {
	d := newTestHighlightDrawer()
	d.Enabled = false
	d.HitTestSelected = matchingHitTest(fyne.NewPos(40, 40), fyne.NewPos(60, 60))

	d.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(50, 50)},
		Dragged:    fyne.Delta{DX: 0, DY: 0},
	})
	d.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(70, 65)},
		Dragged:    fyne.Delta{DX: 20, DY: 15},
	})

	wantPos := fyne.NewPos(60, 55)  // original (40,40) + delta (20,15)
	wantSize := fyne.NewSize(20, 20) // unchanged 20x20 original size
	if d.overlay.Position() != wantPos {
		t.Errorf("ghost overlay position = %v, want %v", d.overlay.Position(), wantPos)
	}
	if d.overlay.Size() != wantSize {
		t.Errorf("ghost overlay size = %v, want %v (must stay the original shape's own size)", d.overlay.Size(), wantSize)
	}

	d.DragEnd()
	if d.overlay.Size() != (fyne.Size{}) {
		t.Errorf("expected the ghost overlay to be zero-sized again after DragEnd, got %v", d.overlay.Size())
	}
}

// freehandGesture simulates a real multi-point freehand drag: an initial
// Dragged call anchoring the start (same idiom dragGesture's own first
// call uses), then one Dragged call per subsequent point in points, then
// DragEnd — the sequence Fyne delivers for a real pen/mouse stroke with
// more than two sampled positions, unlike dragGesture's single-step
// rectangle drag.
func freehandGesture(d *highlightDrawer, start fyne.Position, points ...fyne.Position) {
	d.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: start},
		Dragged:    fyne.Delta{DX: 0, DY: 0},
	})
	prev := start
	for _, p := range points {
		d.Dragged(&fyne.DragEvent{
			PointEvent: fyne.PointEvent{Position: p},
			Dragged:    fyne.Delta{DX: p.X - prev.X, DY: p.Y - prev.Y},
		})
		prev = p
	}
	d.DragEnd()
}

// TestHighlightDrawer_FreehandDragFiresOnFreehandDrawnWithAllPoints
// confirms a real multi-point freehand stroke hands back every sampled
// point, in order — not just start/end like OnDrawn/OnMoved — and that
// OnDrawn/OnTapped don't also fire.
func TestHighlightDrawer_FreehandDragFiresOnFreehandDrawnWithAllPoints(t *testing.T) {
	var gotPoints []fyne.Position
	drawn, tapped := false, false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.FreehandMode = true
	d.OnFreehandDrawn = func(points []fyne.Position, _ fyne.Size) { gotPoints = points }
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }

	start := fyne.NewPos(50, 50)
	mid := fyne.NewPos(55, 60)
	end := fyne.NewPos(70, 55)
	freehandGesture(d, start, mid, end)

	if drawn || tapped {
		t.Errorf("expected only OnFreehandDrawn to fire, got drawn=%v tapped=%v", drawn, tapped)
	}
	want := []fyne.Position{start, mid, end}
	if len(gotPoints) != len(want) {
		t.Fatalf("got %d points, want %d: %v", len(gotPoints), len(want), gotPoints)
	}
	for i := range want {
		if gotPoints[i] != want[i] {
			t.Errorf("point %d = %v, want %v", i, gotPoints[i], want[i])
		}
	}
}

// TestHighlightDrawer_FreehandSingleAxisDragStillFires confirms a
// deliberate single-axis stroke (e.g. a straight vertical pen line) fires
// OnFreehandDrawn — unlike OnDrawn's own rectangle requirement
// (TestHighlightDrawer_ThinSingleAxisDragFiresNeither), a single-axis
// freehand stroke is a perfectly ordinary thing to draw.
func TestHighlightDrawer_FreehandSingleAxisDragStillFires(t *testing.T) {
	fired := false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.FreehandMode = true
	d.OnFreehandDrawn = func([]fyne.Position, fyne.Size) { fired = true }

	freehandGesture(d, fyne.NewPos(50, 50), fyne.NewPos(50, 50+minDragPts+10))

	if !fired {
		t.Error("expected a deliberate single-axis freehand stroke to fire OnFreehandDrawn")
	}
}

// TestHighlightDrawer_FreehandNearStationaryFiresTappedNotFreehand
// confirms the usual near-stationary-drag-is-really-a-tap rule still
// applies in FreehandMode.
func TestHighlightDrawer_FreehandNearStationaryFiresTappedNotFreehand(t *testing.T) {
	tapped, fired := false, false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.FreehandMode = true
	d.OnTapped = func(fyne.Position, fyne.Size) { tapped = true }
	d.OnFreehandDrawn = func([]fyne.Position, fyne.Size) { fired = true }

	dragGesture(d, 100, 100, minDragPts-1, minDragPts-1)

	if !tapped {
		t.Error("expected a near-stationary drag to fire OnTapped")
	}
	if fired {
		t.Error("expected a near-stationary drag NOT to fire OnFreehandDrawn")
	}
}

// TestHighlightDrawer_NonFreehandModeStillDrawsRectangle is a direct
// regression guard: with FreehandMode left at its zero value (false), a
// deliberate two-axis drag must still behave exactly as it always did —
// OnDrawn, not OnFreehandDrawn.
func TestHighlightDrawer_NonFreehandModeStillDrawsRectangle(t *testing.T) {
	drawn, fired := false, false

	d := newTestHighlightDrawer()
	d.Enabled = true
	d.OnDrawn = func(fyne.Position, fyne.Position, fyne.Size) { drawn = true }
	d.OnFreehandDrawn = func([]fyne.Position, fyne.Size) { fired = true }

	dragGesture(d, 50, 50, 40, 40)

	if !drawn || fired {
		t.Errorf("expected OnDrawn only when FreehandMode is off, got drawn=%v fired=%v", drawn, fired)
	}
}
