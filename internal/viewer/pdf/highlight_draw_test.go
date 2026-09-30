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
