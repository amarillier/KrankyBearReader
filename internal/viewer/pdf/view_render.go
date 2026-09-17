package pdf

import (
	"fmt"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// compileTextMatcher is a small local copy of internal/viewer's
// compileMatcher — this package can't import that one (internal/viewer
// already imports internal/viewer/pdf, so the reverse would cycle), and the
// logic is small enough that duplicating it here is simpler than a new
// shared package just for this.
func compileTextMatcher(query string, useRegex bool) (func(s string) bool, error) {
	if query == "" {
		return func(string) bool { return false }, nil
	}
	if useRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	lower := strings.ToLower(query)
	return func(s string) bool { return strings.Contains(strings.ToLower(s), lower) }, nil
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
	v.currentPage = page
	v.pageEntry.SetText(fmt.Sprintf("%d", page))

	if v.continuous {
		v.imageScroll.Offset = fyne.NewPos(0, float32(page-1)*v.contRowH+frac*v.contRowH)
		v.imageScroll.Refresh()
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
		v.imageScroll.Content = v.currentImage
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
// (and later frees) only the ones near the viewport.
func (v *view) buildContinuousPages() {
	n := v.doc.PageCount()
	if n < 1 {
		return
	}
	v.computeRowSize()

	v.pagesL = &pagesLayout{rowW: v.contRowW, rowH: v.contRowH}
	v.pageImages = make([]*canvas.Image, n)
	objs := make([]fyne.CanvasObject, n)
	for i := range v.pageImages {
		img := canvas.NewImageFromImage(nil)
		img.FillMode = canvas.ImageFillStretch
		v.pageImages[i] = img
		objs[i] = img
	}
	v.pagesBox = container.New(v.pagesL, objs...)
	v.imageScroll.Content = v.pagesBox
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
		v.currentPage = page
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
	switch mode {
	case PanelTOC:
		v.panelSelectRef.Selected = "Table of Contents"
	case PanelBookmarks:
		v.panelSelectRef.Selected = "Bookmarks"
	case PanelHighlights:
		v.panelSelectRef.Selected = "Highlights and Notes"
	default:
		v.panelSelectRef.Selected = "Hide Panel"
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
	}
}
