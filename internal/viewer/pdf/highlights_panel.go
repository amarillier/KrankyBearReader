package pdf

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

// highlightsPanel is the "Highlights and Notes" side panel, matching
// Preview.app's own sidebar (per the user's screenshot): every
// highlight/underline/strikeout/squiggly/note annotation already in the
// PDF, listed by page with an excerpt, tap to jump to that page.
//
// Edit Caption/Delete/Save to PDF mirror bookmarkPanel's Edit Title/Delete/
// Save to PDF: a caption lets a highlight someone else's PDF app already
// made get a descriptive label instead of a bare "p.NN Highlight"; new
// highlights themselves are drawn directly on the page (see
// highlight_draw.go's drag-a-rectangle UI, wired into view.go's toolbar),
// not added from this panel — there's nothing page/position-specific to ask
// for in a dialog the way there is for a Bookmark. Always shows its
// toolbar, even with zero highlights loaded, the same reason
// bookmarkPanel's "Add Bookmark" is always available: drawing the very
// first highlight on an unannotated PDF must land somewhere.
type highlightsPanel struct {
	v         *view
	container *fyne.Container
	list      *widget.List
	selected  *Highlight
}

// highlightRowPadding is added on top of a row's own measured MinSize
// height (see newHighlightsPanel's UpdateItem callback) — a little
// breathing room below the wrapped text, matching the visual weight a
// fixed-height row used to get for free from the list's default padding.
const highlightRowPadding float32 = 8

func newHighlightsPanel(v *view) *highlightsPanel {
	hp := &highlightsPanel{v: v}
	hp.list = widget.NewList(
		func() int { return len(v.doc.Highlights) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Template")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id < 0 || id >= len(v.doc.Highlights) {
				return
			}
			h := v.doc.Highlights[id]
			excerpt := h.Contents
			const maxExcerpt = 80
			if len(excerpt) > maxExcerpt {
				excerpt = excerpt[:maxExcerpt] + "…"
			}
			// Wrapping must be set BEFORE SetText, not after: SetText
			// triggers its own internal Refresh (widget.Label's source),
			// so setting Wrapping afterward missed that refresh and used
			// whichever wrap mode this reused row widget happened to
			// already be in from a previous, possibly shorter, item —
			// part of why a long caption could overlay the row below it
			// instead of wrapping into a taller one.
			label.Wrapping = fyne.TextWrapWord
			label.SetText(fmt.Sprintf("p.%d  %s\n%s", h.Page, h.Kind, excerpt))

			// The other part: widget.List gives every row the same fixed
			// height by default, sized for the template's own two lines.
			// SetItemHeight makes each row exactly as tall as its own
			// wrapped content needs instead, so a long caption grows its
			// own row rather than bleeding into the next one. MinSize is
			// accurate here because SetText's own Refresh (just above)
			// already re-measured wrapping at this label's current
			// width — the width the list itself assigned before this
			// callback ever ran; only the height changes.
			hp.list.SetItemHeight(id, label.MinSize().Height+highlightRowPadding)
		},
	)
	hp.list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(v.doc.Highlights) {
			prev := hp.selected
			hp.selected = v.doc.Highlights[id]
			v.doc.SetSelectedHighlight(hp.selected)
			if prev != nil && prev.Page != hp.selected.Page {
				v.repaintPage(prev.Page)
			}
			v.jumpToPage(hp.selected.Page)
			v.repaintPage(hp.selected.Page)
		}
	}

	editBtn := widget.NewButton("Edit Caption", hp.editSelected)
	colorBtn := widget.NewButton("Change Color", hp.changeSelectedColor)

	deleteBtn := widget.NewButton("Delete Selected", hp.deleteSelected)
	deleteBtn.Importance = widget.WarningImportance

	saveBtn := widget.NewButton("Save to PDF", hp.showSaveDialog)

	// A vertical stack, not a horizontal row: an HBox's MinSize is the SUM
	// of its children's widths, which for 4 buttons side by side pinned the
	// whole panel — and the page view it steals width from — far wider
	// than the user could narrow it back down to. bookmarkPanel's own
	// toolbar (Add Bookmark/Edit Title/Delete Selected/Delete All/Save to
	// PDF) is a VBox for exactly this reason: stacked, the panel's MinSize
	// is only its single widest button, matching how TOC/Bookmarks already
	// narrow down freely.
	toolbar := container.NewVBox(editBtn, colorBtn, deleteBtn, saveBtn)

	hp.container = container.NewBorder(toolbar, nil, nil, nil, hp.list)
	return hp
}

// refreshList picks up highlights added/removed since the panel was built —
// called after highlight_draw.go's drag UI adds one, and after deleteSelected.
func (hp *highlightsPanel) refreshList() {
	hp.list.Refresh()
}

// editSelected captions the selected highlight in place — geometry, color,
// and every other highlight are untouched — mirroring bookmarkPanel's
// editSelected/RenameBookmark. Blank is accepted (clears the caption).
func (hp *highlightsPanel) editSelected() {
	h := hp.selected
	if h == nil {
		dialog.ShowInformation("No Selection", "Please select a highlight to caption", hp.v.win)
		return
	}

	captionEntry := widget.NewEntry()
	captionEntry.MultiLine = true
	captionEntry.SetText(h.Contents)
	form := widget.NewForm(widget.NewFormItem("Caption:", captionEntry))

	d := dialog.NewCustomConfirm("Edit Caption", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		hp.v.doc.SetHighlightCaption(h, captionEntry.Text)
		hp.refreshList()
	}, hp.v.win)

	d.Resize(fyne.NewSize(420, 240))
	d.Show()
	hp.v.win.Canvas().Focus(captionEntry)
}

// changeSelectedColor recolors the selected highlight and repaints it
// immediately (Document.SetHighlightColor also clears the render cache).
// Only Kind == "Highlight" actually paints on the page or writes a /C on
// save (see SaveHighlights) — Underline/Strikeout/Squiggly/Note aren't
// painted by this app at all yet (see ReleaseNotes' Future ideas), so
// recoloring one would have no visible effect and is refused up front
// rather than silently doing nothing.
func (hp *highlightsPanel) changeSelectedColor() {
	h := hp.selected
	if h == nil {
		dialog.ShowInformation("No Selection", "Please select a highlight to recolor", hp.v.win)
		return
	}
	if h.Kind != "Highlight" {
		dialog.ShowInformation("Not supported",
			fmt.Sprintf("%s annotations aren't painted on the page yet, so there's no color to change.", h.Kind),
			hp.v.win)
		return
	}
	showHighlightColorPicker(hp.v.win, h.Color, func(rgb [3]float64) {
		hp.v.doc.SetHighlightColor(h, rgb)
		hp.v.repaintPage(h.Page)
	})
}

// deleteSelected removes the selected highlight from the page immediately
// (Document.DeleteHighlight also clears the render cache, so it disappears
// on the very next repaint) — mirroring bookmarkPanel.deleteSelected. Not
// written back into the PDF file itself until Save to PDF runs.
func (hp *highlightsPanel) deleteSelected() {
	h := hp.selected
	if h == nil {
		dialog.ShowInformation("No Selection", "Please select a highlight to delete", hp.v.win)
		return
	}
	dialog.ShowConfirm("Delete", fmt.Sprintf("Delete this %s?", strings.ToLower(h.Kind)), func(ok bool) {
		if !ok {
			return
		}
		if !hp.v.doc.DeleteHighlight(h) {
			dialog.ShowError(fmt.Errorf("failed to delete highlight"), hp.v.win)
			return
		}
		hp.selected = nil
		hp.v.doc.SetSelectedHighlight(nil)
		hp.list.UnselectAll()
		hp.refreshList()
		hp.v.repaintPage(h.Page)
	}, hp.v.win)
}

// showSaveDialog offers overwriting the original file or saving a new copy,
// mirroring bookmarkPanel.showSaveDialog.
func (hp *highlightsPanel) showSaveDialog() {
	const optNew = "Save as a new file..."
	const optOverwrite = "Overwrite the original file"
	choice := widget.NewRadioGroup([]string{optNew, optOverwrite}, nil)
	choice.SetSelected(optNew)

	content := container.NewVBox(widget.NewLabel("Save highlight changes to:"), choice)

	d := dialog.NewCustomConfirm("Save to PDF", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		if choice.Selected == optOverwrite {
			hp.saveChangesTo(hp.v.doc.Path())
			return
		}
		hp.showSaveAsDialog()
	}, hp.v.win)

	d.Resize(fyne.NewSize(460, 220))
	d.Show()
}

// showSaveAsDialog lets the user pick the exact destination file and
// folder, mirroring bookmarkPanel.showSaveAsDialog.
func (hp *highlightsPanel) showSaveAsDialog() {
	origPath := hp.v.doc.Path()
	dir := filepath.Dir(origPath)
	base := strings.TrimSuffix(filepath.Base(origPath), filepath.Ext(origPath))

	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, hp.v.win)
			return
		}
		if w == nil {
			return // user cancelled
		}
		path := w.URI().Path()
		w.Close()
		hp.saveChangesTo(path)
	}, hp.v.win)

	fd.SetFileName(base + "-highlighted.pdf")
	if lister, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
		fd.SetLocation(lister)
	}
	fd.Show()
}

func (hp *highlightsPanel) saveChangesTo(outputPath string) {
	if err := hp.v.doc.SaveHighlights(outputPath); err != nil {
		dialog.ShowError(fmt.Errorf("failed to save: %w", err), hp.v.win)
		return
	}
	hp.refreshList()
	hp.showTransientInfo("Success", fmt.Sprintf("Saved to:\n%s", outputPath))
}

func (hp *highlightsPanel) showTransientInfo(title, msg string) {
	d := dialog.NewInformation(title, msg, hp.v.win)
	d.Show()
	time.AfterFunc(2*time.Second, func() {
		fyne.Do(d.Hide)
	})
}
