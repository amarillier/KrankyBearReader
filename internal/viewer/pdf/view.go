package pdf

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// PanelMode selects what the side panel shows.
type PanelMode int

const (
	PanelNone PanelMode = iota
	PanelTOC
	PanelBookmarks
	PanelHighlights
)

// displayScale converts a rendered-at-baseRenderDPI pixel size down to an
// on-screen point size (Fyne's coordinate space is ~72 DPI). Matches
// pdfviewer's own math exactly: a page rendered at 100% zoom (baseRenderDPI)
// should display at its natural size, not at baseRenderDPI's raw pixel count.
const displayScale = 72.0 / baseRenderDPI

var zoomOptions = []struct {
	label string
	level float64 // -1 = Fit Width, -2 = Fit Page
}{
	{"50%", 0.5},
	{"75%", 0.75},
	{"100%", 1.0},
	{"125%", 1.25},
	{"150%", 1.5},
	{"200%", 2.0},
	{"300%", 3.0},
	{"Fit Width", -1},
	{"Fit Page", -2},
}

// ViewHandle is what NewView hands back to the tab that hosts it: the
// content to display, an optional Close to release the Document's resources
// when the tab closes, and an optional TypedKey for page-navigation
// shortcuts — registered by the caller only while this tab is the selected
// one (a JSON tab shouldn't page-turn on Space; see manager.go).
type ViewHandle struct {
	Content  fyne.CanvasObject
	Close    func()
	TypedKey func(*fyne.KeyEvent)
}

// view holds all per-tab PDF viewing state. Exactly one is created per open
// PDF tab (via NewView), mirroring pdfviewer's PDFViewer but scoped to one
// tab's content instead of the whole window.
type view struct {
	win fyne.Window
	doc *Document

	currentPage int
	zoomLevel   float64 // -1 Fit Width, -2 Fit Page, else a literal zoom factor
	continuous  bool
	panelMode   PanelMode

	currentImage *canvas.Image
	imageScroll  *container.Scroll
	viewportSize fyne.Size

	pagesBox           *fyne.Container
	pagesL             *pagesLayout
	pageImages         []*canvas.Image
	contRowW, contRowH float32

	pageEntry      *widget.Entry
	totalLabel     *widget.Label
	zoomSelect     *widget.Select
	panelSelectRef *widget.Select
	statusBar      *widget.Label

	panel       *bookmarkPanel
	highlights  *highlightsPanel
	split       *container.Split
	mainContent *fyne.Container // Stack: split (panel+content) or content alone
	pdfContent  fyne.CanvasObject
}

// NewView builds the tab content for one open PDF Document.
func NewView(win fyne.Window, doc *Document) ViewHandle {
	v := &view{
		win:         win,
		doc:         doc,
		currentPage: 1,
		zoomLevel:   -1, // default to Fit Width, a sensible first look at any page size
	}

	content := v.build()
	return ViewHandle{
		Content:  content,
		Close:    func() { _ = doc.Close() },
		TypedKey: v.typedKey,
	}
}

func (v *view) build() fyne.CanvasObject {
	// Built before the toolbar: widget.Select.SetSelected (used below to set
	// the toolbar's initial zoom/panel choices) fires its OnChanged handler
	// synchronously, which calls into renderCurrentPage/setPanelMode — those
	// touch imageScroll/currentImage/statusBar, so those must already exist.
	// Confirmed via manual testing: building the toolbar first crashed with
	// a nil-pointer dereference the instant the zoom Select's initial value
	// was set.
	v.currentImage = &canvas.Image{FillMode: canvas.ImageFillOriginal}
	v.imageScroll = container.NewScroll(v.currentImage)
	v.imageScroll.OnScrolled = func(_ fyne.Position) {
		if v.continuous {
			v.lazyRenderVisible()
			v.updateCurrentPageFromScroll()
		}
	}
	v.statusBar = widget.NewLabel("")

	toolbar := v.buildToolbar()
	findRow := v.buildFindBar()
	topBar := container.NewVBox(toolbar, findRow)

	viewport := container.New(&resizeReportingLayout{onResize: v.onViewportResize}, v.imageScroll)
	v.pdfContent = container.NewBorder(topBar, v.statusBar, nil, nil, viewport)

	v.panel = newBookmarkPanel(v)
	v.highlights = newHighlightsPanel(v)
	v.split = container.NewHSplit(v.panel.container, v.pdfContent)
	v.split.SetOffset(0.22)

	v.mainContent = container.NewStack(v.pdfContent)
	v.panel.rebuildTreeData()

	if err := v.renderCurrentPage(); err != nil {
		v.statusBar.SetText(fmt.Sprintf("Failed to render page: %v", err))
	}
	v.updateStatus()

	return v.mainContent
}

func (v *view) buildToolbar() fyne.CanvasObject {
	firstBtn := widget.NewButton("|<", func() { v.jumpToPage(1) })
	prevBtn := widget.NewButton("<", v.previousPage)
	nextBtn := widget.NewButton(">", v.nextPage)
	lastBtn := widget.NewButton(">|", func() { v.jumpToPage(v.doc.PageCount()) })

	v.pageEntry = widget.NewEntry()
	v.pageEntry.SetText("1")
	v.pageEntry.OnSubmitted = func(s string) {
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			v.jumpToPage(n)
		}
	}
	v.totalLabel = widget.NewLabel(fmt.Sprintf("/ %d", v.doc.PageCount()))

	zoomLabels := make([]string, len(zoomOptions))
	for i, z := range zoomOptions {
		zoomLabels[i] = z.label
	}
	v.zoomSelect = widget.NewSelect(zoomLabels, v.handleZoomChange)
	// Set the initial value directly rather than via SetSelected: SetSelected
	// fires OnChanged synchronously, which calls renderCurrentPage — fine
	// once the view is fully built, but buildToolbar runs before the panel/
	// split are constructed (see build()), so triggering it here would nil-
	// deref. Matches the "set .Selected + Refresh, not SetSelected" idiom
	// already used by pdfviewer's own syncModeSelect for the same reason.
	v.zoomSelect.Selected = "Fit Width"
	v.zoomSelect.Refresh()

	continuousCheck := widget.NewCheck("Continuous Scroll", func(on bool) {
		v.setContinuous(on)
	})

	panelSelect := widget.NewSelect([]string{"Hide Panel", "Table of Contents", "Bookmarks", "Highlights and Notes"}, func(s string) {
		switch s {
		case "Table of Contents":
			v.setPanelMode(PanelTOC)
		case "Bookmarks":
			v.setPanelMode(PanelBookmarks)
		case "Highlights and Notes":
			v.setPanelMode(PanelHighlights)
		default:
			v.setPanelMode(PanelNone)
		}
	})
	panelSelect.Selected = "Hide Panel"
	panelSelect.Refresh()
	v.panelSelectRef = panelSelect

	nav := container.NewHBox(firstBtn, prevBtn, v.pageEntry, v.totalLabel, nextBtn, lastBtn)
	right := container.NewHBox(v.zoomSelect, continuousCheck, panelSelect)
	return container.NewBorder(nil, nil, nav, right)
}

// buildFindBar is a small local find bar (this package can't reuse
// internal/viewer's shared one — that package already imports this one, so
// the reverse would be an import cycle). Search is page-level: go-fitz's
// Text() has no character-position API, so find-next/prev jumps to the
// nearest page (forward/backward from the current one, wrapping around)
// whose extracted text matches, not to an exact position within the page.
func (v *view) buildFindBar() fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Find in document (page-level)...")
	regexCheck := widget.NewCheck("Regex", nil)
	status := widget.NewLabel("")

	find := func(dir int) {
		query := entry.Text
		if query == "" {
			return
		}
		matches, err := compileTextMatcher(query, regexCheck.Checked)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		n := v.doc.PageCount()
		for i := 1; i <= n; i++ {
			page := ((v.currentPage-1+dir*i)%n+n)%n + 1
			text, err := v.doc.PageText(page)
			if err != nil {
				continue
			}
			if matches(text) {
				v.jumpToPage(page)
				status.SetText(fmt.Sprintf("found on page %d", page))
				return
			}
		}
		status.SetText("no matches")
	}

	entry.OnSubmitted = func(string) { find(1) }
	prevBtn := widget.NewButton("<", func() { find(-1) })
	nextBtn := widget.NewButton(">", func() { find(1) })

	return container.NewBorder(nil, nil, nil, container.NewHBox(regexCheck, prevBtn, nextBtn, status), entry)
}
