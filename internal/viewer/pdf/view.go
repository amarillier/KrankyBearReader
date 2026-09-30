package pdf

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// panelSelectOptions returns the panel-switcher dropdown's option list, with
// its closed-state entry (index 0) worded for what selecting it would do:
// "Show Panel" while nothing is open, "Hide Panel" while one already is.
// Both values fall into setPanelMode's "default" case (PanelNone) — the
// caller doesn't need to know or care which one is current.
func panelSelectOptions(panelOpen bool) []string {
	closedLabel := "Show Panel"
	if panelOpen {
		closedLabel = "Hide Panel"
	}
	return []string{closedLabel, "Table of Contents", "Bookmarks", "Highlights and Notes"}
}

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
// when the tab closes, an optional TypedKey for page-navigation shortcuts —
// registered by the caller only while this tab is the selected one (a JSON
// tab shouldn't page-turn on Space; see manager.go) — and SaveDialog, which
// opens the same "Save to PDF" dialog as the Bookmarks/TOC panel's own Save
// button, for a File-menu "Save to PDF..." item to call without needing the
// panel open first.
type ViewHandle struct {
	Content    fyne.CanvasObject
	Close      func()
	TypedKey   func(*fyne.KeyEvent)
	SaveDialog func()
}

// view holds all per-tab PDF viewing state. Exactly one is created per open
// PDF tab (via NewView), mirroring pdfviewer's PDFViewer but scoped to one
// tab's content instead of the whole window.
type view struct {
	win fyne.Window
	doc *Document

	currentPage    int
	onPageChanged  func(int)                     // reports every currentPage change, for cross-launch page persistence — see setCurrentPage
	onRetargeted   func(oldPath, newPath string) // reports a successful retargetTo, for the tab-owning code's own path bookkeeping — see retargetTo
	zoomLevel      float64                       // -1 Fit Width, -2 Fit Page, else a literal zoom factor
	continuous     bool
	panelMode      PanelMode
	drawKind       string     // "" = off, else "Highlight"/"Square"/"Circle"/"Line" — see setDrawKind/handleHighlightDrawn
	highlightColor [3]float64 // color the next drawn shape uses — see colorSwatchBtn
	colorSwatchBtn *colorSwatch

	currentImage  *canvas.Image
	currentDrawer *highlightDrawer
	imageScroll   *container.Scroll
	viewportSize  fyne.Size

	pagesBox           *fyne.Container
	pagesL             *pagesLayout
	pageImages         []*canvas.Image
	pageDrawers        []*highlightDrawer
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

// NewView builds the tab content for one open PDF Document. initialPage
// (1-based; values < 2 are a no-op) jumps there once the view is built —
// clamped the same way jumpToPage always clamps, so a stale saved page from
// a since-shortened file doesn't misbehave. onPageChanged, if non-nil, is
// called with the new current page every time it changes (explicit
// navigation or continuous-scroll tracking alike), letting the caller
// persist "where was I" without this package needing to know that's what
// it's for. onRetargeted, if non-nil, is called after a genuine "Save as a
// new file..." switches this tab to editing the newly-saved file instead of
// the original (see retargetTo) — letting the caller update its own
// path-keyed bookkeeping (tab title, open-files map, recent files) without
// this package needing to know any of that exists.
func NewView(win fyne.Window, doc *Document, initialPage int, onPageChanged func(int), onRetargeted func(oldPath, newPath string)) ViewHandle {
	v := &view{
		win:            win,
		doc:            doc,
		currentPage:    1,
		onPageChanged:  onPageChanged,
		onRetargeted:   onRetargeted,
		zoomLevel:      -1, // default to Fit Width, a sensible first look at any page size
		highlightColor: defaultHighlightColor,
	}

	content := v.build()
	if initialPage > 1 {
		v.jumpToPage(initialPage)
	}

	return ViewHandle{
		Content: content,
		// v.doc, not the doc parameter directly: retargetTo can swap v.doc
		// out for a different *Document mid-session (see its own doc
		// comment), and this must close whichever one is CURRENT when the
		// tab actually closes, not whichever one was passed in here —
		// closing the original after a retarget would double-close an
		// already-closed Document and leak the replacement's own MuPDF
		// handle and scratch temp file.
		Close:      func() { _ = v.doc.Close() },
		TypedKey:   v.typedKey,
		SaveDialog: func() { v.panel.showSaveDialog() },
	}
}

// retargetTo swaps this tab's Document for a freshly-opened one at newPath.
// Used after a genuine "Save as a new file..." (never after an overwrite,
// which keeps editing the same Document/path throughout) — mirrors how
// Save As behaves in most editors (a saved-as file becomes the one you're
// now editing), rather than leaving the tab pointed at the original file
// while its own on-disk bytes have diverged from what's showing. The
// original file is untouched either way (SaveHighlights/SaveBookmarks never
// write to it when outputPath differs from the Document's own path) — this
// only decides which file THIS TAB keeps editing afterward.
//
// Closes the old Document (releasing its MuPDF handle and any scratch
// normalized-copy temp file — see Document.Close) before opening the new
// one: once Save has actually written newPath, the old Document's own
// in-memory state has nothing left to do. This does mean any OTHER kind of
// pending, not-yet-saved edit the old Document was carrying — e.g. a
// pending Bookmarks change, if this was called from a Highlights-only save,
// or vice versa — is discarded along with it, since it was never part of
// what got written to newPath either. Deliberately not wired into
// bookmarkPanel's own independent Save As for exactly this reason: saving
// bookmarks alone never applies pending highlight edits, so retargeting
// there would silently discard them with no way to still save them
// afterward — a real risk this app's two independent save paths (bookmarks
// vs. highlights) create that a single unified save wouldn't.
//
// Best-effort: if re-opening newPath fails (surprising right after
// successfully writing it, but not impossible — a permissions or
// network-drive hiccup), the old Document and tab are left exactly as they
// were, still fully working — only the "this tab now follows the new file"
// convenience is skipped, not the save that already succeeded.
func (v *view) retargetTo(newPath string) error {
	newDoc, err := Prepare(newPath)
	if err != nil {
		return err
	}
	oldPath := v.doc.Path()
	_ = v.doc.Close()
	v.doc = newDoc

	v.highlights.selected = nil
	v.doc.SetSelectedHighlight(nil)
	v.highlights.refreshList()

	v.panel.selected = nil
	v.panel.tree.UnselectAll()
	v.panel.rebuildTreeData()

	v.totalLabel.SetText(fmt.Sprintf("/ %d", v.doc.PageCount()))
	if v.currentPage > v.doc.PageCount() {
		v.currentPage = v.doc.PageCount()
	}
	if v.currentPage < 1 {
		v.currentPage = 1
	}
	v.refreshAllPages()

	if v.onRetargeted != nil {
		v.onRetargeted(oldPath, newPath)
	}
	return nil
}

// setCurrentPage updates currentPage and reports the change via
// onPageChanged — the one choke point every page-changing code path
// (jumpToPageAndPosition, continuous-scroll's updateCurrentPageFromScroll)
// goes through, so page persistence has a single place to hook rather than
// needing a call at every navigation site.
func (v *view) setCurrentPage(page int) {
	v.currentPage = page
	if v.onPageChanged != nil {
		v.onPageChanged(page)
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
	v.currentDrawer = newHighlightDrawer(v.currentImage)
	v.currentDrawer.OnDrawn = func(start, end fyne.Position, size fyne.Size) {
		v.handleHighlightDrawn(v.currentPage, start, end, size)
	}
	v.currentDrawer.OnTapped = func(pos fyne.Position, size fyne.Size) {
		v.handleHighlightTapped(v.currentPage, pos, size)
	}
	v.currentDrawer.OnTappedSecondary = func(pos fyne.Position, size fyne.Size, absPos fyne.Position) {
		v.handleHighlightSecondaryTapped(v.currentPage, pos, size, absPos)
	}
	v.imageScroll = container.NewScroll(v.currentDrawer)
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

	// Draw: click-drag directly on the page to create a new shape of
	// whichever kind is selected here (see handleHighlightDrawn) — a
	// dropdown rather than one checkbox per kind, matching the same
	// "don't reserve space for every option all the time" reasoning as
	// panelSelect below. Highlight/Square/Circle/Star/Hexagon are a plain
	// rectangle drag (go-fitz has no text bounding-box API for a real
	// text-snapped selection like Preview/Acrobat's Highlight tool, so
	// this is a free rectangle instead; Star/Hexagon generate their own
	// vertices parametrically from that same rectangle — see
	// starVertices/hexagonVertices, since PDF has no dedicated subtype
	// for either, both are authored as a generic Polygon); Line draws an
	// arrow from the drag's start toward its end (see
	// defaultArrowEndStyle); Text/Speech Bubble prompt for the caption
	// text before creating anything (see promptForShapeText). "Off" by
	// default so a plain click-drag (which did nothing before Draw
	// Highlight existed) still does nothing unless the user deliberately
	// picks a shape.
	//
	// Every option is prefixed "Draw: " — found via real hands-on testing
	// that a bare "Off" (this control's own closed-state label, same as
	// every other option) reads as just an unlabeled toolbar item sitting
	// next to Continuous Scroll, not obviously a mode selector at all, so
	// it went unnoticed until the user went looking for it specifically.
	// Unlike panelSelect below (whose closed-state label alone flips
	// between two self-describing strings), every option here already
	// needs its own distinct label, so the fix is prefixing all of them
	// uniformly rather than special-casing just the closed state.
	drawOptions := []string{
		"Draw: Off", "Draw: Highlight", "Draw: Square", "Draw: Circle", "Draw: Line",
		"Draw: Star", "Draw: Hexagon", "Draw: Text", "Draw: Speech Bubble",
	}
	drawSelect := widget.NewSelect(drawOptions, func(s string) {
		kind := strings.TrimPrefix(s, "Draw: ")
		if kind == "Off" {
			v.setDrawKind("")
		} else {
			v.setDrawKind(kind)
		}
	})
	drawSelect.Selected = "Draw: Off"
	drawSelect.Refresh()

	// The swatch shows the color the next drawn highlight will use; tapping
	// it opens Fyne's own color picker (Advanced mode — full RGB, not just
	// a handful of presets) seeded with the current choice.
	v.colorSwatchBtn = newColorSwatch(rgbToFyneColor(v.highlightColor))
	v.colorSwatchBtn.OnTapped = func() {
		showHighlightColorPicker(v.win, v.highlightColor, func(rgb [3]float64) {
			v.highlightColor = rgb
			v.colorSwatchBtn.SetColor(rgbToFyneColor(rgb))
		})
	}

	// A single dropdown rather than a persistent row of buttons: picking the
	// closed option is what actually closes the split (mainContent.Objects
	// swaps back to pdfContent alone in setPanelMode), so no side-panel width
	// is reserved at all while it's hidden — unlike a permanently-visible
	// button rail, which costs that width whether or not a panel is open.
	// Placed at the far left of the toolbar (not its original spot at the far
	// right) so it sits right above where the panel actually opens, keeping
	// the "quick to reach" win without the "always reserves space" cost.
	//
	// The closed-state option's own label flips between "Show Panel" and
	// "Hide Panel" (see setPanelMode) so it always describes what clicking it
	// does next, rather than statically reading "Hide Panel" even while
	// nothing is showing to hide. Both labels map to PanelNone here (neither
	// names a real panel), so the switch below doesn't need to know which one
	// is currently displayed.
	panelSelect := widget.NewSelect(panelSelectOptions(false), func(s string) {
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
	panelSelect.Selected = "Show Panel"
	panelSelect.Refresh()
	v.panelSelectRef = panelSelect

	nav := container.NewHBox(panelSelect, firstBtn, prevBtn, v.pageEntry, v.totalLabel, nextBtn, lastBtn)
	right := container.NewHBox(v.zoomSelect, continuousCheck, drawSelect, v.colorSwatchBtn)
	return container.NewBorder(nil, nil, nav, right)
}

// buildFindBar is a small local find bar (this package can't reuse
// internal/viewer's shared one — that package already imports this one, so
// the reverse would be an import cycle). Document-wide: scans every page's
// already-extracted text once per query (findState), reporting a real
// match count and "found on N pages" (like Preview), and next/prev steps
// through every individual occurrence — including several on the same
// page — not just the nearest matching page. Also draws an approximate
// on-page highlight box for the current match (Document.SearchMatchRect,
// via MuPDF's HTML export line positions — see CLAUDE.md for why this is
// an estimate, not a pixel-perfect box like Preview's, and why that's an
// accepted tradeoff): when SearchMatchRect can't locate it (e.g. a match
// spanning a line-wrap boundary the HTML export can't see), the status
// label stays the source of truth for "which match", same as before this
// existed.
func (v *view) buildFindBar() fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Find in document...")
	regexCheck := widget.NewCheck("Regex", nil)
	status := widget.NewLabel("")

	var state *findState

	find := func(dir int) {
		query := entry.Text
		if query == "" {
			status.SetText("")
			state = nil
			v.doc.ClearSearchHighlight()
			v.repaintPage(v.currentPage)
			return
		}
		if state.stale(query, regexCheck.Checked) {
			s, err := buildFindState(v.doc, query, regexCheck.Checked)
			if err != nil {
				status.SetText(err.Error())
				state = nil
				v.doc.ClearSearchHighlight()
				v.repaintPage(v.currentPage)
				return
			}
			state = s
		}
		if len(state.matchPages) == 0 {
			status.SetText("No matches")
			v.doc.ClearSearchHighlight()
			v.repaintPage(v.currentPage)
			return
		}
		page := state.step(v.currentPage, dir)
		if rect, ok := v.doc.SearchMatchRect(page, query, regexCheck.Checked, state.occurrenceIndexOnPage()); ok {
			v.doc.SetSearchHighlight(page, rect)
		} else {
			v.doc.ClearSearchHighlight()
		}
		v.jumpToPage(page)
		v.repaintPage(page)
		pageWord := "pages"
		if state.pagesWithMatch == 1 {
			pageWord = "page"
		}
		status.SetText(fmt.Sprintf("Match %d of %d (found on %d %s)",
			state.currentIndex+1, len(state.matchPages), state.pagesWithMatch, pageWord))
	}

	entry.OnSubmitted = func(string) { find(1) }
	prevBtn := widget.NewButton("<", func() { find(-1) })
	nextBtn := widget.NewButton(">", func() { find(1) })

	return container.NewBorder(nil, nil, nil, container.NewHBox(regexCheck, prevBtn, nextBtn, status), entry)
}
