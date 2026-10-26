package pdf

import (
	"fmt"
	"image"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// thumbnailWidthPx is the width pages are rendered at for the panel.
const thumbnailWidthPx = 120

// thumbnailPumpDelay spaces out thumbnail renders so a screenful of rows
// doesn't block the UI in one go: each step renders ONE page on the main
// goroutine (MuPDF's context isn't safe to share with a worker, and
// rebuildDoc can swap d.doc under one), then yields.
const thumbnailPumpDelay = 2 * time.Millisecond

// thumbnailPanel is the page-thumbnail side panel: a scrolling list of
// small page previews with annotations painted in, each labelled with its
// page number and annotation count, with an "annotated pages only" filter
// for finding highlights and shapes quickly. Clicking a row jumps to it.
type thumbnailPanel struct {
	v         *view
	container *fyne.Container
	list      *widget.List
	onlyAnnot *widget.Check

	rows      []int // page number (1-based) shown in each list row
	pending   []int // pages awaiting a render, most recently requested last
	pumping   bool
	syncing   bool // true while we move the list selection ourselves, so OnSelected doesn't jump
	rowHeight float32
}

func newThumbnailPanel(v *view) *thumbnailPanel {
	tp := &thumbnailPanel{v: v}
	tp.onlyAnnot = widget.NewCheck("Annotated pages only", func(bool) { tp.rebuildRows(true) })

	tp.list = widget.NewList(
		func() int { return len(tp.rows) },
		func() fyne.CanvasObject {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(thumbnailWidthPx, tp.thumbHeight()))
			return container.NewVBox(img, widget.NewLabel(""))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(tp.rows) {
				return
			}
			page := tp.rows[id]
			box := obj.(*fyne.Container)
			img := box.Objects[0].(*canvas.Image)
			label := box.Objects[1].(*widget.Label)

			text := fmt.Sprintf("Page %d", page)
			if n := tp.v.doc.highlightCountOnPage(page); n > 0 {
				text += fmt.Sprintf("  ·  %d annotation", n)
				if n != 1 {
					text += "s"
				}
			}
			label.SetText(text)

			if t := tp.cached(page); t != nil {
				img.Image = t
			} else {
				img.Image = nil
				tp.request(page)
			}
			img.Refresh()
		},
	)
	tp.list.OnSelected = func(id widget.ListItemID) {
		if tp.syncing || id < 0 || id >= len(tp.rows) {
			return
		}
		tp.v.jumpToPage(tp.rows[id])
	}

	tp.container = container.NewBorder(tp.onlyAnnot, nil, nil, nil, tp.list)
	tp.rebuildRows(true)
	return tp
}

// thumbHeight is the row image height, from page 1's aspect ratio (capped
// so one very tall page can't make every row huge); other pages are
// letterboxed into the same box by ImageFillContain.
func (tp *thumbnailPanel) thumbHeight() float32 {
	if tp.rowHeight > 0 {
		return tp.rowHeight
	}
	h := float32(thumbnailWidthPx * 1.3)
	if w, ph, err := tp.v.doc.PageBoundsPt(1); err == nil && w > 0 {
		h = float32(thumbnailWidthPx * ph / w)
	}
	tp.rowHeight = min(max(h, 60), 240)
	return tp.rowHeight
}

// cached returns page's thumbnail if it's already rendered, without
// rendering anything.
func (tp *thumbnailPanel) cached(page int) image.Image {
	d := tp.v.doc
	if d.thumbs == nil {
		return nil
	}
	img, _ := d.thumbs.Get(cacheKey(page, float64(thumbnailWidthPx)))
	return img
}

// request queues page for rendering and makes sure the pump is running.
// Most recently requested pages render first, so scrolling shows what's on
// screen now before rows the user already scrolled past.
func (tp *thumbnailPanel) request(page int) {
	for _, p := range tp.pending {
		if p == page {
			return
		}
	}
	tp.pending = append(tp.pending, page)
	tp.schedulePump()
}

func (tp *thumbnailPanel) schedulePump() {
	if tp.pumping || len(tp.pending) == 0 {
		return
	}
	tp.pumping = true
	time.AfterFunc(thumbnailPumpDelay, func() { fyne.Do(tp.pumpStep) })
}

// pumpStep renders one pending thumbnail and refreshes its row.
func (tp *thumbnailPanel) pumpStep() {
	tp.pumping = false
	for len(tp.pending) > 0 {
		page := tp.pending[len(tp.pending)-1]
		tp.pending = tp.pending[:len(tp.pending)-1]
		if tp.cached(page) != nil {
			continue
		}
		if _, err := tp.v.doc.RenderThumbnail(page, thumbnailWidthPx); err != nil {
			continue // leave this row blank rather than retrying forever
		}
		for i, p := range tp.rows {
			if p == page {
				tp.list.RefreshItem(i)
				break
			}
		}
		break
	}
	tp.schedulePump()
}

// rebuildRows recomputes which pages are listed (all, or only annotated
// ones) and refreshes the list, keeping the current page selected.
func (tp *thumbnailPanel) rebuildRows(scroll bool) {
	d := tp.v.doc
	tp.rows = tp.rows[:0]
	for p := 1; p <= d.PageCount(); p++ {
		if tp.onlyAnnot.Checked && d.highlightCountOnPage(p) == 0 {
			continue
		}
		tp.rows = append(tp.rows, p)
	}
	tp.list.Refresh()
	tp.syncSelection(tp.v.currentPage, scroll)
}

// refresh re-requests every visible row after annotations changed (the
// Document has already dropped its stale thumbnails), and re-filters.
func (tp *thumbnailPanel) refresh() {
	tp.pending = tp.pending[:0]
	tp.rebuildRows(false) // don't yank the list back while the user is scrolling it
}

// reset is refresh for an entirely different Document (Save As retarget).
func (tp *thumbnailPanel) reset() {
	tp.rowHeight = 0
	tp.pending = tp.pending[:0]
	tp.rebuildRows(true)
}

// syncSelection highlights page's row (if listed) and scrolls it into
// view when scroll is set, without triggering OnSelected's jump.
func (tp *thumbnailPanel) syncSelection(page int, scroll bool) {
	tp.syncing = true
	defer func() { tp.syncing = false }()
	for i, p := range tp.rows {
		if p == page {
			tp.list.Select(i)
			if scroll {
				tp.list.ScrollTo(i)
			}
			return
		}
	}
	tp.list.UnselectAll()
}
