package pdf

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// compileTextCounter is a small local relative of internal/viewer's
// compileMatcher — this package can't import that one (internal/viewer
// already imports internal/viewer/pdf, so the reverse would cycle) — but
// counts every non-overlapping occurrence in s rather than just reporting
// whether one exists, the data buildFindBar's own document-wide search
// needs (see findState). strings.Count's own non-overlapping convention
// (matching how a browser's or Preview's own find already counts
// "aaa".Count("aa") as 1, not 2) is kept for the plain-text path for the
// same reason; regexp.FindAllStringIndex already returns non-overlapping
// matches for the regex path.
func compileTextCounter(query string, useRegex bool) (func(s string) int, error) {
	if query == "" {
		return func(string) int { return 0 }, nil
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return func(s string) int { return len(re.FindAllStringIndex(s, -1)) }, nil
	}
	lower := strings.ToLower(query)
	return func(s string) int { return strings.Count(strings.ToLower(s), lower) }, nil
}

// findState is one completed document-wide search: matchPages[i] is the
// 1-based page of the i-th match overall, in document order (a page with
// k occurrences appears k times in a row) — simple enough to index
// directly for next/prev stepping and to report "page P" for the current
// match, without needing per-match character positions (blocked by
// go-fitz having no such API at all — see ReleaseNotes' Future ideas for
// the phase this unblocks: an actual on-page highlight box).
type findState struct {
	query      string
	useRegex   bool
	matchPages []int
	// pagesWithMatch is len(unique matchPages) -- reported alongside
	// "Match X of Y" the same way Preview reports "Found on N pages"
	// alongside its own match count.
	pagesWithMatch int
	currentIndex   int // -1 until the first next/prev step after a (re)build
	// lastPage is the page the most recent step() call jumped to — see
	// step's own doc comment for why this, not just currentIndex < 0, is
	// what decides whether to re-anchor near the caller's current page.
	lastPage int
}

// buildFindState scans every page's already-extracted text (Document.PageText)
// once, counting every occurrence via counter — the actual fix for the
// find bar only ever jumping to the nearest matching PAGE, one at a time,
// with no idea how many matches existed anywhere else in the document.
func buildFindState(doc *Document, query string, useRegex bool) (*findState, error) {
	counter, err := compileTextCounter(query, useRegex)
	if err != nil {
		return nil, err
	}
	s := &findState{query: query, useRegex: useRegex, currentIndex: -1}
	for page := 1; page <= doc.PageCount(); page++ {
		text, err := doc.PageText(page)
		if err != nil {
			continue
		}
		c := counter(text)
		if c == 0 {
			continue
		}
		s.pagesWithMatch++
		for i := 0; i < c; i++ {
			s.matchPages = append(s.matchPages, page)
		}
	}
	return s, nil
}

// stale reports whether query/useRegex have changed since s was built —
// buildFindBar rebuilds from scratch when true, rather than stepping a
// search that no longer matches what's in the entry/regex checkbox.
func (s *findState) stale(query string, useRegex bool) bool {
	return s == nil || s.query != query || s.useRegex != useRegex
}

// step advances to the next (dir=1) or previous (dir=-1) match from
// fromPage, wrapping around the document, and returns the 1-based page to
// jump to.
//
// Re-anchors — starts fresh from the nearest match at-or-after fromPage
// (dir=1) or at-or-before it (dir=-1), the same "search from where you
// are" behavior the old page-only find had, rather than blindly moving
// the flat index by dir — whenever fromPage doesn't match lastPage, the
// page the PREVIOUS step call landed on. Not just on the very first step
// after a (re)build (checking currentIndex < 0 alone, an earlier version
// of this method's own behavior): found via real hands-on testing that
// searching the same query again after manually navigating elsewhere
// (e.g. jumping to page 1, or clicking a bookmark) kept advancing the OLD
// sequence from wherever it left off, when the user's own expectation —
// reasonably — was for it to notice they'd moved and start again from
// where they now are. Only actually re-anchors when the caller's current
// page has genuinely diverged from this state's own idea of "where the
// last match left you"; repeatedly clicking next/prev without navigating
// elsewhere in between keeps fromPage == lastPage every time, so normal
// sequential stepping through every occurrence (including several on the
// same page) is unaffected.
func (s *findState) step(fromPage, dir int) int {
	n := len(s.matchPages)
	if s.currentIndex < 0 || fromPage != s.lastPage {
		idx := -1
		if dir > 0 {
			for i, p := range s.matchPages {
				if p >= fromPage {
					idx = i
					break
				}
			}
		} else {
			for i := n - 1; i >= 0; i-- {
				if s.matchPages[i] <= fromPage {
					idx = i
					break
				}
			}
		}
		if idx < 0 {
			idx = 0
			if dir < 0 {
				idx = n - 1
			}
		}
		s.currentIndex = idx
	} else {
		s.currentIndex = ((s.currentIndex+dir)%n + n) % n
	}
	s.lastPage = s.matchPages[s.currentIndex]
	return s.lastPage
}

// occurrenceIndexOnPage reports how many earlier occurrences of the
// current match's own page came before it in matchPages — i.e. the
// current match is the (return value)-th occurrence ON THAT PAGE
// specifically, 0-based. buildFindBar's own on-page highlight box needs
// this: Document.SearchMatchRect re-searches just one page's own text, so
// it needs to know which occurrence within that page to land on, not the
// flat document-wide index currentIndex already is.
func (s *findState) occurrenceIndexOnPage() int {
	page := s.matchPages[s.currentIndex]
	count := 0
	for i := 0; i < s.currentIndex; i++ {
		if s.matchPages[i] == page {
			count++
		}
	}
	return count
}

// continuousBuffer is how many extra rows above/below the viewport stay
// rendered in continuous mode — matches pdfviewer's lazyRenderVisible.
const continuousBuffer = 1

func (v *view) updateStatus() {
	if v.doc.Bookmarks.HasBookmarks() {
		v.statusBar.SetText(fmt.Sprintf("%d bookmarks/TOC entries loaded", len(v.doc.Bookmarks.GetBookmarks())))
	} else {
		v.statusBar.SetText("")
	}
}

// renderZoomFor is the zoom actually rendered at: Fit modes render at 100%
// and are rescaled via SetMinSize (no re-render on resize); fixed zoom
// levels render at that exact DPI, so higher zoom is crisp, not upscaled.
func renderZoomFor(zoomLevel float64) float64 {
	if zoomLevel <= 0 {
		return 1.0
	}
	return zoomLevel
}

func (v *view) jumpToPage(page int) {
	v.jumpToPageAndPosition(page, 0)
}

func (v *view) jumpToPageAndPosition(page int, frac float32) {
	if page < 1 {
		page = 1
	}
	if page > v.doc.PageCount() {
		page = v.doc.PageCount()
	}
	v.setCurrentPage(page)
	v.pageEntry.SetText(fmt.Sprintf("%d", page))

	if v.continuous {
		v.imageScroll.Offset = fyne.NewPos(0, float32(page-1)*v.contRowH+frac*v.contRowH)
		v.imageScroll.Refresh()
		v.win.Canvas().Refresh(v.imageScroll)
		v.lazyRenderVisible()
		return
	}

	if err := v.renderCurrentPage(); err != nil {
		dialog.ShowError(err, v.win)
		return
	}
	if frac > 0 {
		v.imageScroll.Offset = fyne.NewPos(0, frac*v.scrollableHeight())
		v.imageScroll.Refresh()
		v.win.Canvas().Refresh(v.imageScroll)
	}
}

func (v *view) nextPage() {
	if v.currentPage < v.doc.PageCount() {
		v.jumpToPage(v.currentPage + 1)
	}
}

func (v *view) previousPage() {
	if v.currentPage > 1 {
		v.jumpToPage(v.currentPage - 1)
	}
}

func (v *view) scrollableHeight() float32 {
	h := v.imageScroll.Content.MinSize().Height - v.imageScroll.Size().Height
	if h < 0 {
		return 0
	}
	return h
}

// scrollFraction reports how far down the current page's content the user
// has scrolled (0..1) — used for "region bookmark" (jump to exact position).
func (v *view) scrollFraction() float32 {
	if v.continuous {
		into := v.imageScroll.Offset.Y - float32(v.currentPage-1)*v.contRowH
		if v.contRowH <= 0 {
			return 0
		}
		f := into / v.contRowH
		if f < 0 {
			f = 0
		}
		return f
	}
	sh := v.scrollableHeight()
	if sh <= 0 {
		return 0
	}
	return v.imageScroll.Offset.Y / sh
}

// renderCurrentPage renders the current page in paginated mode.
func (v *view) renderCurrentPage() error {
	renderZoom := renderZoomFor(v.zoomLevel)
	img, err := v.doc.RenderPage(v.currentPage, renderZoom)
	if err != nil {
		return err
	}

	v.currentImage.Image = img
	v.currentImage.Resource = nil
	bounds := img.Bounds()

	switch {
	case v.zoomLevel == -1: // Fit Width
		v.applyFitWidth(bounds.Dx(), bounds.Dy())
	case v.zoomLevel == -2: // Fit Page
		v.applyFitPage(bounds.Dx(), bounds.Dy())
	default:
		v.currentImage.SetMinSize(fyne.NewSize(float32(bounds.Dx())*displayScale, float32(bounds.Dy())*displayScale))
	}
	v.currentImage.Refresh()
	v.imageScroll.Offset = fyne.NewPos(0, 0)
	v.imageScroll.Refresh()
	return nil
}

// repaintPage forces page (1-based) to re-render on the next paint.
// AddHighlight/DeleteHighlight already clear the whole render cache, but the
// already-rendered *image.Image sitting in v.currentImage/v.pageImages is
// still there and won't repaint on its own — renderCurrentPage's cache Get
// would just miss and re-render (cheap), but continuous mode's
// lazyRenderVisible only fills in a page image when it's nil, so that one
// needs an explicit nudge.
func (v *view) repaintPage(page int) {
	if v.continuous {
		if i := page - 1; i >= 0 && i < len(v.pageImages) {
			v.pageImages[i].Image = nil
		}
		v.lazyRenderVisible()
		return
	}
	if page != v.currentPage {
		return
	}
	if err := v.renderCurrentPage(); err != nil {
		dialog.ShowError(err, v.win)
	}
}

// refreshAllPages forces every page — not just the current one — to
// re-render on the next paint, unlike repaintPage's single-page targeting.
// Used by retargetTo after swapping in an entirely different Document: any
// page could show different content now, not only whichever one happened
// to be on screen at the time.
func (v *view) refreshAllPages() {
	if v.continuous {
		for _, img := range v.pageImages {
			if img != nil {
				img.Image = nil
			}
		}
		v.lazyRenderVisible()
		return
	}
	if err := v.renderCurrentPage(); err != nil {
		dialog.ShowError(err, v.win)
	}
}

// setDrawKind sets which shape kind (if any) dragging on the page creates
// — "" turns drawing off entirely, dragging does nothing (same as before
// this feature existed, and the same as clicking anywhere while it's on
// still does for click-to-select — see highlightDrawer.Dragged/DragEnd).
// Applies to every page's drawer at once, single-page and continuous
// alike, so switching modes mid-scroll doesn't leave some pages still
// draggable while others aren't.
func (v *view) setDrawKind(kind string) {
	v.drawKind = kind
	enabled := kind != ""
	if v.currentDrawer != nil {
		v.currentDrawer.Enabled = enabled
	}
	for _, d := range v.pageDrawers {
		if d != nil {
			d.Enabled = enabled
		}
	}
}

// defaultArrowEndStyle is what a freshly-drawn "Line" shape gets: no
// arrowhead at the drag's start, a closed arrowhead at its end — matching
// the natural "point at something" gesture (drag FROM the thing you're
// pointing away from TO the thing you're pointing at). An array literal
// can't be a Go const, hence var.
var defaultArrowEndStyle = [2]string{"None", "ClosedArrow"}

// handleHighlightDrawn is every highlightDrawer.OnDrawn's target, for both
// single-page and continuous-scroll page widgets: converts the drag's
// start/end points into the right geometry for v.drawKind and adds it as
// a new in-memory highlight, then makes it visible immediately — in the
// Highlights panel's list and painted on the page (see paintHighlights'
// own needsHandPaint gate, which is what makes a not-yet-saved shape of
// any of these kinds show up at all before Save to PDF runs).
//
// start/end are the drag's own raw endpoints, not normalized into
// top-left/bottom-right — Line needs that (an arrow points from start
// toward end, not toward whichever corner happens to be lower-right);
// Square/Circle/Highlight don't care about direction, so they derive
// their own min/max bounds from the same two points.
func (v *view) handleHighlightDrawn(page int, start, end fyne.Position, widgetSize fyne.Size) {
	pageW, pageH, err := v.doc.PageBoundsPt(page)
	if err != nil {
		return
	}
	switch v.drawKind {
	case "Line":
		x1, y1 := widgetPointToPDF(start, widgetSize, pageW, pageH)
		x2, y2 := widgetPointToPDF(end, widgetSize, pageW, pageH)
		v.doc.AddLineShape(page, []float64{x1, y1, x2, y2}, defaultArrowEndStyle, v.highlightColor)
	case "Square", "Circle":
		x1, y1 := widgetPointToPDF(start, widgetSize, pageW, pageH)
		x2, y2 := widgetPointToPDF(end, widgetSize, pageW, pageH)
		rect := [4]float64{min(x1, x2), min(y1, y2), max(x1, x2), max(y1, y2)}
		v.doc.AddRectShape(page, v.drawKind, rect, v.highlightColor)
	case "Star", "Hexagon":
		x1, y1 := widgetPointToPDF(start, widgetSize, pageW, pageH)
		x2, y2 := widgetPointToPDF(end, widgetSize, pageW, pageH)
		cx, cy := (x1+x2)/2, (y1+y2)/2
		rx, ry := math.Abs(x2-x1)/2, math.Abs(y2-y1)/2
		vertices := hexagonVertices(cx, cy, rx, ry)
		if v.drawKind == "Star" {
			vertices = starVertices(cx, cy, rx, ry)
		}
		v.doc.AddPolygonShape(page, vertices, v.highlightColor)
	case "Text", "Speech Bubble":
		// Unlike every other kind, this one needs a caption before
		// there's anything to add at all — promptForShapeText creates
		// the shape (and does its own refreshList/repaintPage) only on
		// confirm, so a cancelled dialog leaves no phantom empty
		// annotation behind. Must return here, not fall through to the
		// unconditional refresh/repaint below.
		x1, y1 := widgetPointToPDF(start, widgetSize, pageW, pageH)
		x2, y2 := widgetPointToPDF(end, widgetSize, pageW, pageH)
		rect := [4]float64{min(x1, x2), min(y1, y2), max(x1, x2), max(y1, y2)}
		v.promptForShapeText(page, rect, v.drawKind == "Speech Bubble")
		return
	default: // "Highlight", and the fallback for any unrecognized drawKind
		quad := widgetRectToQuad(rectTopLeft(start, end), rectBottomRight(start, end), widgetSize, pageW, pageH)
		v.doc.AddHighlight(page, [][8]float64{quad}, v.highlightColor, "")
	}
	v.highlights.refreshList()
	v.repaintPage(page)
}

// starVertexCount/starInnerRadiusRatio shape the 5-pointed star every
// "Draw: Star" drag produces: 10 vertices alternating an outer point and
// an inner one, closing back to the first — innerRatio 0.4 is a plain
// aesthetic choice (not a precise mathematical constant) that reads as a
// recognizable 5-point star at typical drawn sizes, not a geometric
// requirement.
const (
	starPointCount       = 5
	starInnerRadiusRatio = 0.4
	hexagonVertexCount   = 6
)

// hexagonVertices/starVertices generate a regular hexagon/5-pointed star
// inscribed in the ellipse centered at (cx, cy) with radii rx, ry (PDF
// user-space points — same non-uniform-stretch allowance a drag-to-fit
// rectangle already gives Square/Circle, rather than forcing a perfect
// regular polygon regardless of how the user actually dragged). Both
// start at angle -90° (straight up) so the shape's own "top" lands where
// a user dragging top-to-bottom would expect it, then walk evenly-spaced
// angles around the ellipse — Polygon's own /Vertices has no notion of
// "which vertex is the top," so this is purely about matching visual
// expectation, not a PDF requirement. There's no dedicated PDF annotation
// subtype for either shape; both are authored as a generic Polygon (see
// AddPolygonShape) — MuPDF has no idea, or need to know, that this
// Polygon started life as a hexagon rather than any other six-vertex
// shape someone else might have drawn.
func hexagonVertices(cx, cy, rx, ry float64) [][2]float64 {
	v := make([][2]float64, hexagonVertexCount)
	for i := range v {
		theta := -math.Pi/2 + float64(i)*2*math.Pi/float64(hexagonVertexCount)
		v[i] = [2]float64{cx + rx*math.Cos(theta), cy + ry*math.Sin(theta)}
	}
	return v
}

func starVertices(cx, cy, rx, ry float64) [][2]float64 {
	n := starPointCount * 2
	v := make([][2]float64, n)
	for i := range v {
		theta := -math.Pi/2 + float64(i)*math.Pi/float64(starPointCount)
		r := 1.0
		if i%2 == 1 {
			r = starInnerRadiusRatio
		}
		v[i] = [2]float64{cx + rx*r*math.Cos(theta), cy + ry*r*math.Sin(theta)}
	}
	return v
}

// promptForShapeText shows a text-entry dialog for a freshly-drawn Text
// or Speech Bubble shape's caption, mirroring bookmarkPanel's own "Add
// Bookmark" title-entry dialog (dialog.NewCustomConfirm + focus the
// entry). Creates the shape only on confirm with non-empty text —
// cancelling, or confirming blank, leaves no phantom empty annotation
// behind, matching AddHighlight-family methods' own "nothing reaches
// d.Highlights until there's something real to add" convention.
func (v *view) promptForShapeText(page int, rect [4]float64, speechBubble bool) {
	entry := widget.NewMultiLineEntry()
	entry.SetPlaceHolder("Text...")
	title := "Add Text Block"
	if speechBubble {
		title = "Add Speech Bubble"
	}
	d := dialog.NewCustomConfirm(title, "Add", "Cancel", entry, func(ok bool) {
		if !ok || entry.Text == "" {
			return
		}
		var tip *[2]float64
		if speechBubble {
			t := calloutTipFor(rect)
			tip = &t
		}
		v.doc.AddTextShape(page, rect, tip, entry.Text, v.highlightColor)
		v.highlights.refreshList()
		v.repaintPage(page)
	}, v.win)
	d.Resize(fyne.NewSize(360, 220))
	d.Show()
	v.win.Canvas().Focus(entry)
}

// calloutTipFor picks a speech bubble's callout tip (the tail's far end)
// a fixed offset below-left of its box, proportional to the box's own
// (smaller) dimension so the tail looks reasonable whether the box is
// small or large. Simple, predictable placement for a first version —
// letting the user aim the tail precisely is a shape-EDIT feature (see
// ReleaseNotes' Future ideas), not something initial drawing needs to
// solve.
func calloutTipFor(rect [4]float64) [2]float64 {
	w, h := rect[2]-rect[0], rect[3]-rect[1]
	offset := math.Min(w, h) * 0.4
	if offset <= 0 {
		offset = 10
	}
	return [2]float64{rect[0] - offset, rect[1] - offset}
}

// handleHighlightTapped is every highlightDrawer.OnTapped's target: hit-
// tests the tap against this page's highlights (Document.HighlightAt) and,
// on a hit, selects it in the Highlights panel's list — the reverse
// direction of selecting a list row and seeing it outlined on the page.
// Reuses the list's own Select (which fires OnSelected, see
// highlightsPanel.newHighlightsPanel) rather than duplicating its
// jump/outline/repaint logic here. A tap that hits nothing deselects
// instead of leaving whatever was selected lingering — clicking empty
// page space is a deliberate "I don't mean to have anything selected"
// gesture, the same as it would be in most editors, and matters more now
// that a bare Delete/Backspace keypress acts on the current selection.
func (v *view) handleHighlightTapped(page int, pos fyne.Position, widgetSize fyne.Size) {
	pageW, pageH, err := v.doc.PageBoundsPt(page)
	if err != nil {
		return
	}
	x, y := widgetPointToPDF(pos, widgetSize, pageW, pageH)
	h := v.doc.HighlightAt(page, x, y)
	if h == nil {
		v.highlights.deselect()
		return
	}
	for i, candidate := range v.doc.Highlights {
		if candidate == h {
			v.highlights.list.Select(i)
			return
		}
	}
}

// handleHighlightSecondaryTapped is every highlightDrawer.OnTappedSecondary's
// target: right-click (or long-press) a highlight/shape directly on the
// page to select it — the exact same hit test and selection as a plain
// left click (handleHighlightTapped) — and immediately show a small
// context menu at the cursor, so deleting one doesn't require first
// selecting it here, then reaching over to the Highlights panel's own
// Delete button. Only offers Delete for now, matching the request that
// motivated this; Change Color already has its own dedicated swatch/button
// and doesn't need a second way to reach it.
func (v *view) handleHighlightSecondaryTapped(page int, pos fyne.Position, widgetSize fyne.Size, absolutePos fyne.Position) {
	pageW, pageH, err := v.doc.PageBoundsPt(page)
	if err != nil {
		return
	}
	x, y := widgetPointToPDF(pos, widgetSize, pageW, pageH)
	h := v.doc.HighlightAt(page, x, y)
	if h == nil {
		return
	}
	for i, candidate := range v.doc.Highlights {
		if candidate == h {
			v.highlights.list.Select(i)
			break
		}
	}

	menu := fyne.NewMenu("", fyne.NewMenuItem("Delete", v.highlights.deleteSelected))
	widget.ShowPopUpMenuAtPosition(menu, v.win.Canvas(), absolutePos)
}

func (v *view) applyFitWidth(imgW, imgH int) {
	vpW := v.viewportSize.Width - 4
	if vpW <= 0 || imgW <= 0 {
		return
	}
	scale := vpW / float32(imgW)
	v.currentImage.SetMinSize(fyne.NewSize(vpW, scale*float32(imgH)))
}

func (v *view) applyFitPage(imgW, imgH int) {
	vpW := v.viewportSize.Width - 4
	vpH := v.viewportSize.Height - 4
	if vpW <= 0 || vpH <= 0 || imgW <= 0 || imgH <= 0 {
		return
	}
	scaleW := vpW / float32(imgW)
	scaleH := vpH / float32(imgH)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	v.currentImage.SetMinSize(fyne.NewSize(scale*float32(imgW), scale*float32(imgH)))
}

// onViewportResize keeps Fit Width/Fit Page correct as the window resizes.
// Fixed zoom levels are already rendered at their final pixel size, so
// there's nothing to re-fit — matches pdfviewer's onViewportResize.
func (v *view) onViewportResize(size fyne.Size) {
	v.viewportSize = size
	if v.continuous {
		v.relayoutContinuous()
		return
	}
	if v.zoomLevel > 0 || v.currentImage.Image == nil {
		return
	}
	b := v.currentImage.Image.Bounds()
	if v.zoomLevel == -1 {
		v.applyFitWidth(b.Dx(), b.Dy())
	} else {
		v.applyFitPage(b.Dx(), b.Dy())
	}
	v.currentImage.Refresh()
}

func (v *view) handleZoomChange(label string) {
	for _, z := range zoomOptions {
		if z.label == label {
			v.zoomLevel = z.level
			break
		}
	}
	if v.continuous {
		v.relayoutContinuous()
		return
	}
	if err := v.renderCurrentPage(); err != nil {
		dialog.ShowError(err, v.win)
	}
}

func (v *view) setContinuous(on bool) {
	v.continuous = on
	if on {
		v.buildContinuousPages()
	} else {
		v.imageScroll.Content = v.currentDrawer
		v.imageScroll.Refresh()
		if err := v.renderCurrentPage(); err != nil {
			dialog.ShowError(err, v.win)
		}
	}
}

// computeRowSize estimates a uniform per-page display size from page 1
// alone (assumes roughly uniform page dimensions across the document, same
// simplification pdfviewer's computeRowSize makes) so the continuous layout
// can be sized before every page has been rendered.
func (v *view) computeRowSize() {
	renderZoom := renderZoomFor(v.zoomLevel)
	img, err := v.doc.RenderPage(1, renderZoom)
	if err != nil {
		return
	}
	b := img.Bounds()

	switch {
	case v.zoomLevel == -1: // Fit Width
		vpW := v.viewportSize.Width - 4
		if vpW <= 0 {
			vpW = 600
		}
		scale := vpW / float32(b.Dx())
		v.contRowW = vpW
		v.contRowH = scale * float32(b.Dy())
	case v.zoomLevel == -2: // Fit Page
		vpW := v.viewportSize.Width - 4
		if vpW <= 0 {
			vpW = 600
		}
		scale := vpW / float32(b.Dx())
		v.contRowW = vpW
		v.contRowH = scale * float32(b.Dy())
	default:
		v.contRowW = float32(b.Dx()) * displayScale
		v.contRowH = float32(b.Dy()) * displayScale
	}
}

// buildContinuousPages sets up the virtualized continuous-scroll layout:
// every page gets an empty slot up front, and lazyRenderVisible fills in
// (and later frees) only the ones near the viewport. Scrolls straight to
// v.currentPage rather than leaving the fresh scroll container at its
// zero-value offset (top of page 1) — confirmed via manual testing that
// without this, switching Continuous Scroll on while reading any page but
// the first silently jumped back to page 1.
//
// The pagesBox.Resize(MinSize) call below is load-bearing, not cosmetic.
// Fyne's *container.Scroll.Refresh() always calls refreshBars(), which calls
// updateOffset(0, 0) — and updateOffset's very first check is `if
// s.Content.Size() fits within the viewport, force Offset to (0,0)`
// (internal/widget/scroller.go). A brand-new pagesBox has never been through
// a layout pass, so Content.Size() reads as the zero value, which always
// "fits" — so Refresh() silently stomps whatever Offset we just set back to
// (0,0) before anything is ever painted, regardless of what page we wanted.
// Resizing pagesBox to its real MinSize first makes Content.Size() already
// reflect the full multi-page height, so updateOffset takes its normal
// clamp-in-bounds path instead of its reset-to-zero one, and our offset
// survives the Refresh() call that follows.
func (v *view) buildContinuousPages() {
	n := v.doc.PageCount()
	if n < 1 {
		return
	}
	v.computeRowSize()

	v.pagesL = &pagesLayout{rowW: v.contRowW, rowH: v.contRowH}
	v.pageImages = make([]*canvas.Image, n)
	v.pageDrawers = make([]*highlightDrawer, n)
	objs := make([]fyne.CanvasObject, n)
	for i := range v.pageImages {
		img := canvas.NewImageFromImage(nil)
		img.FillMode = canvas.ImageFillStretch
		v.pageImages[i] = img

		page := i + 1
		drawer := newHighlightDrawer(img)
		drawer.Enabled = v.drawKind != ""
		drawer.OnDrawn = func(start, end fyne.Position, size fyne.Size) {
			v.handleHighlightDrawn(page, start, end, size)
		}
		drawer.OnTapped = func(pos fyne.Position, size fyne.Size) {
			v.handleHighlightTapped(page, pos, size)
		}
		drawer.OnTappedSecondary = func(pos fyne.Position, size fyne.Size, absPos fyne.Position) {
			v.handleHighlightSecondaryTapped(page, pos, size, absPos)
		}
		v.pageDrawers[i] = drawer
		objs[i] = drawer
	}
	v.pagesBox = container.New(v.pagesL, objs...)
	v.pagesBox.Resize(v.pagesBox.MinSize())
	v.imageScroll.Content = v.pagesBox
	v.imageScroll.Offset = fyne.NewPos(0, float32(v.currentPage-1)*v.contRowH)
	v.imageScroll.Refresh()
	v.lazyRenderVisible()
}

func (v *view) relayoutContinuous() {
	if !v.continuous || v.pagesL == nil {
		return
	}
	v.computeRowSize()
	v.pagesL.rowW = v.contRowW
	v.pagesL.rowH = v.contRowH
	v.pagesBox.Refresh()
	v.lazyRenderVisible()
}

// lazyRenderVisible renders only the pages within continuousBuffer rows of
// the viewport, and frees (sets nil) any page image outside that window —
// bounds memory for large PDFs instead of keeping every page rendered
// forever. Ported from pdfviewer's lazyRenderVisible.
func (v *view) lazyRenderVisible() {
	if v.contRowH <= 0 || len(v.pageImages) == 0 {
		return
	}
	viewportH := v.imageScroll.Size().Height
	offset := v.imageScroll.Offset.Y

	lo := int(offset/v.contRowH) - continuousBuffer
	hi := int((offset+viewportH)/v.contRowH) + continuousBuffer
	if lo < 0 {
		lo = 0
	}
	if hi >= len(v.pageImages) {
		hi = len(v.pageImages) - 1
	}

	renderZoom := renderZoomFor(v.zoomLevel)
	for i, im := range v.pageImages {
		if i < lo || i > hi {
			if im.Image != nil {
				im.Image = nil
				im.Refresh()
			}
			continue
		}
		if im.Image == nil {
			img, err := v.doc.RenderPage(i+1, renderZoom)
			if err != nil {
				continue
			}
			im.Image = img
			im.Refresh()
		}
	}
}

func (v *view) updateCurrentPageFromScroll() {
	if v.contRowH <= 0 {
		return
	}
	// 30%-into-viewport heuristic: whichever page occupies that point is
	// "current" for the page-number field and Add Bookmark's default page.
	pos := v.imageScroll.Offset.Y + v.imageScroll.Size().Height*0.3
	page := int(pos/v.contRowH) + 1
	if page < 1 {
		page = 1
	}
	if page > v.doc.PageCount() {
		page = v.doc.PageCount()
	}
	if page != v.currentPage {
		v.setCurrentPage(page)
		v.pageEntry.SetText(fmt.Sprintf("%d", page))
	}
}

// setPanelMode swaps the side panel in or out and refreshes its contents
// for the new mode — the same "swap .Objects between panel+content and
// content-only" pattern this app already uses elsewhere (Show All/Hide All
// windows, DocTabs), ported from pdfviewer's applyPanelMode/PanelMode.
func (v *view) setPanelMode(mode PanelMode) {
	v.panelMode = mode
	if mode == PanelNone {
		v.mainContent.Objects = []fyne.CanvasObject{v.pdfContent}
	} else {
		if mode == PanelHighlights {
			v.split.Leading = v.highlights.container
		} else {
			v.split.Leading = v.panel.container
		}
		v.split.Refresh()
		v.mainContent.Objects = []fyne.CanvasObject{v.split}
	}
	v.mainContent.Refresh()

	// Sync the dropdown's displayed value directly rather than via
	// SetSelected: confirmed via manual testing that SetSelected fires
	// OnChanged unconditionally in this Fyne version, even when reselecting
	// the value it already holds (widget/select.go's updateSelected has no
	// same-value guard) — since this method IS that OnChanged handler for
	// panelSelect, calling SetSelected here recurses into itself forever.
	// Same fix, same reasoning as buildToolbar's initial zoom/panel value.
	// SetOptions itself doesn't touch .Selected or fire OnChanged, so it's
	// safe to call before setting .Selected below.
	v.panelSelectRef.SetOptions(panelSelectOptions(mode != PanelNone))
	switch mode {
	case PanelTOC:
		v.panelSelectRef.Selected = "Table of Contents"
	case PanelBookmarks:
		v.panelSelectRef.Selected = "Bookmarks"
	case PanelHighlights:
		v.panelSelectRef.Selected = "Highlights and Notes"
	default:
		v.panelSelectRef.Selected = "Show Panel"
	}
	v.panelSelectRef.Refresh()
	if mode == PanelTOC || mode == PanelBookmarks {
		v.panel.rebuildTreeData()
		v.panel.tree.Refresh()
	}
}

// typedKey is the PDF tab's page-navigation shortcut handler. Registered by
// the caller (see manager.go) only while this tab is the currently selected
// one, so other formats' tabs are unaffected. Guards focused text entries
// (the page-number box, or a bookmark title field in a dialog) the same way
// pdfviewer's setupShortcuts does: only Page Up/Down still act as page nav
// while a text field has focus, so arrows/Home/End can move its cursor.
func (v *view) typedKey(ev *fyne.KeyEvent) {
	if _, focused := v.win.Canvas().Focused().(*widget.Entry); focused {
		switch ev.Name {
		case fyne.KeyPageDown:
			v.nextPage()
		case fyne.KeyPageUp:
			v.previousPage()
		}
		return
	}

	switch ev.Name {
	case fyne.KeyPageDown, fyne.KeySpace, fyne.KeyRight, fyne.KeyDown:
		v.nextPage()
	case fyne.KeyPageUp, fyne.KeyLeft, fyne.KeyUp:
		v.previousPage()
	case fyne.KeyHome:
		v.jumpToPage(1)
	case fyne.KeyEnd:
		v.jumpToPage(v.doc.PageCount())
	case fyne.KeyDelete, fyne.KeyBackspace:
		// Deletes whichever highlight is currently selected — via the
		// Highlights panel's own list, a page click, or a right-click's
		// select-then-menu (see handleHighlightSecondaryTapped) — all three
		// keep highlights.selected in sync through the same list.Select
		// call. Silently does nothing with no selection, rather than
		// popping deleteSelected's own "No Selection" dialog for an
		// unrelated stray Delete/Backspace keypress.
		if v.highlights.selected != nil {
			v.highlights.deleteSelected()
		}
	case fyne.KeyEscape:
		// Deselects whatever's currently selected — a deliberate,
		// explicit way to back out of a selection before it causes an
		// accidental delete (or, once shape move/edit exists, an
		// accidental move/edit), on top of delete's own confirm dialog.
		// A no-op with nothing selected, same as deleteSelected's own
		// guard above.
		v.highlights.deselect()
	}
}
