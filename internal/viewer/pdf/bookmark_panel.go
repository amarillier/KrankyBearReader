package pdf

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// bookmarkPanel is the TOC/Bookmarks side panel for one PDF tab. Ported from
// pdfviewer's BookmarkPanel, adapted to reference this package's per-tab
// *view instead of the whole-window PDFViewer.
type bookmarkPanel struct {
	v         *view
	container *fyne.Container
	tree      *widget.Tree
	treeData  map[string]*Bookmark
	rootIDs   []string
	addBtn    *widget.Button
	selected  *Bookmark
}

func newBookmarkPanel(v *view) *bookmarkPanel {
	bp := &bookmarkPanel{
		v:        v,
		treeData: make(map[string]*Bookmark),
		rootIDs:  make([]string, 0),
	}
	bp.buildUI()
	return bp
}

func (bp *bookmarkPanel) buildUI() {
	bp.tree = widget.NewTree(bp.childUIDs, bp.isBranch, bp.createTemplate, bp.updateItem)
	bp.tree.OnSelected = bp.onSelected

	bp.addBtn = widget.NewButton("Add Bookmark", func() {
		bp.showAddDialog(bp.v.panelMode == PanelTOC)
	})

	editBtn := widget.NewButton("Edit Title", bp.editSelected)

	deleteBtn := widget.NewButton("Delete Selected", bp.deleteSelected)
	deleteBtn.Importance = widget.WarningImportance

	deleteAllBtn := widget.NewButton("Delete All", bp.deleteAll)
	deleteAllBtn.Importance = widget.DangerImportance

	saveBtn := widget.NewButton("Save to PDF", bp.showSaveDialog)

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		_ = bp.v.doc.Bookmarks.LoadBookmarks(bp.v.doc.Path())
		bp.rebuildTreeData()
		bp.tree.Refresh()
		bp.v.updateStatus()
	})
	refreshBtn.Importance = widget.LowImportance

	toolbar := container.NewBorder(nil, nil, nil, refreshBtn,
		container.NewVBox(bp.addBtn, editBtn, deleteBtn, deleteAllBtn, saveBtn))

	bp.container = container.NewBorder(toolbar, nil, nil, nil, container.NewScroll(bp.tree))
}

func (bp *bookmarkPanel) updateAddButtonLabel() {
	if bp.v.panelMode == PanelTOC {
		bp.addBtn.SetText("Add TOC Entry")
	} else {
		bp.addBtn.SetText("Add Bookmark")
	}
}

func (bp *bookmarkPanel) childUIDs(uid string) []string {
	if uid == "" {
		return bp.rootIDs
	}
	bookmark, exists := bp.treeData[uid]
	if !exists || len(bookmark.Children) == 0 {
		return []string{}
	}
	ids := make([]string, len(bookmark.Children))
	for i, child := range bookmark.Children {
		id := fmt.Sprintf("%p", child)
		bp.treeData[id] = child
		ids[i] = id
	}
	return ids
}

func (bp *bookmarkPanel) isBranch(uid string) bool {
	if uid == "" {
		return true
	}
	bookmark, exists := bp.treeData[uid]
	return exists && len(bookmark.Children) > 0
}

func (bp *bookmarkPanel) createTemplate(bool) fyne.CanvasObject {
	return widget.NewLabel("Template")
}

func (bp *bookmarkPanel) updateItem(uid string, _ bool, obj fyne.CanvasObject) {
	label := obj.(*widget.Label)
	bookmark, exists := bp.treeData[uid]
	if !exists {
		label.SetText("Unknown")
		return
	}
	switch {
	case !bookmark.IsUserAdded:
		label.SetText(fmt.Sprintf("\U0001F4D6 %s (p.%d)", bookmark.Title, bookmark.PageNo)) // 📖
	case bookmark.YOffset > 0:
		label.SetText(fmt.Sprintf("\U0001F4CD %s (p.%d)", bookmark.Title, bookmark.PageNo)) // 📍
	default:
		label.SetText(fmt.Sprintf("\U0001F516 %s (p.%d)", bookmark.Title, bookmark.PageNo)) // 🔖
	}
}

// rebuildTreeData reconstructs the tree from the Document's bookmarks,
// filtered by the panel's current mode (TOC shows the document's own
// outline, Bookmarks shows user-added entries).
func (bp *bookmarkPanel) rebuildTreeData() {
	bp.treeData = make(map[string]*Bookmark)
	bp.rootIDs = make([]string, 0)

	bookmarks := bp.v.doc.Bookmarks.GetBookmarks()
	sorted := make([]*Bookmark, len(bookmarks))
	copy(sorted, bookmarks)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PageNo < sorted[j].PageNo })

	mode := bp.v.panelMode
	for _, bookmark := range sorted {
		switch mode {
		case PanelTOC:
			if bookmark.IsUserAdded {
				continue
			}
		case PanelBookmarks:
			if !bookmark.IsUserAdded {
				continue
			}
		}
		id := fmt.Sprintf("%p", bookmark)
		bp.treeData[id] = bookmark
		bp.rootIDs = append(bp.rootIDs, id)
	}
	bp.updateAddButtonLabel()
}

func (bp *bookmarkPanel) onSelected(uid string) {
	bookmark, exists := bp.treeData[uid]
	if !exists {
		return
	}
	bp.selected = bookmark
	bp.v.jumpToPageAndPosition(bookmark.PageNo, bookmark.YOffset)
}

// editSelected renames the selected entry's title in place — page and
// position are untouched, unlike delete-and-recreate via showAddDialog.
// Works for both TOC entries and Bookmarks; the dialog itself doesn't need a
// mode-specific label the way showAddDialog's does.
func (bp *bookmarkPanel) editSelected() {
	bookmark := bp.selected
	if bookmark == nil {
		dialog.ShowInformation("No Selection", "Please select an entry to edit", bp.v.win)
		return
	}

	titleEntry := widget.NewEntry()
	titleEntry.SetText(bookmark.Title)
	form := widget.NewForm(widget.NewFormItem("Title:", titleEntry))

	d := dialog.NewCustomConfirm("Edit Title", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		if err := bp.v.doc.Bookmarks.RenameBookmark(bookmark, titleEntry.Text); err != nil {
			dialog.ShowError(err, bp.v.win)
			return
		}
		bp.rebuildTreeData()
		bp.tree.Refresh()
	}, bp.v.win)

	d.Resize(fyne.NewSize(400, 150))
	d.Show()
	bp.v.win.Canvas().Focus(titleEntry)
}

func (bp *bookmarkPanel) showAddDialog(toTOC bool) {
	kind, dlgTitle := "Bookmark", "Add Bookmark"
	if toTOC {
		kind, dlgTitle = "TOC entry", "Add to Table of Contents"
	}

	titleEntry := widget.NewEntry()
	titleEntry.SetPlaceHolder(kind + " title")

	pageEntry := widget.NewEntry()
	pageEntry.SetText(strconv.Itoa(bp.v.currentPage))
	pageEntry.SetPlaceHolder("Page number")

	form := container.NewVBox(widget.NewForm(
		widget.NewFormItem("Title:", titleEntry),
		widget.NewFormItem("Page:", pageEntry),
	))

	currentFrac := bp.v.scrollFraction()
	var typeRadio *widget.RadioGroup
	if !toTOC {
		typeRadio = widget.NewRadioGroup([]string{
			"Page bookmark (jump to top of page)",
			"Region bookmark (jump to current position)",
		}, nil)
		if currentFrac > 0.01 {
			typeRadio.SetSelected("Region bookmark (jump to current position)")
		} else {
			typeRadio.SetSelected("Page bookmark (jump to top of page)")
		}
		form.Add(typeRadio)
	}

	d := dialog.NewCustomConfirm(dlgTitle, "Add", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		title := titleEntry.Text
		if title == "" {
			dialog.ShowError(fmt.Errorf("title cannot be empty"), bp.v.win)
			return
		}
		pageNo, err := strconv.Atoi(pageEntry.Text)
		if err != nil || pageNo < 1 || pageNo > bp.v.doc.PageCount() {
			dialog.ShowError(fmt.Errorf("invalid page number"), bp.v.win)
			return
		}
		var yOffset float32
		if typeRadio != nil && typeRadio.Selected == "Region bookmark (jump to current position)" {
			yOffset = currentFrac
		}

		bp.v.doc.Bookmarks.AddBookmark(title, pageNo, yOffset, !toTOC)
		if toTOC {
			bp.v.setPanelMode(PanelTOC)
		} else {
			bp.v.setPanelMode(PanelBookmarks)
		}
		bp.showTransientInfo("Success", fmt.Sprintf("%s '%s' added", kind, title))
	}, bp.v.win)

	d.Resize(fyne.NewSize(460, 250))
	d.Show()
	bp.v.win.Canvas().Focus(titleEntry)
}

func (bp *bookmarkPanel) showSaveDialog() {
	empty := !bp.v.doc.Bookmarks.HasBookmarks()

	const optNew = "Save as a new file..."
	const optOverwrite = "Overwrite the original file"
	choice := widget.NewRadioGroup([]string{optNew, optOverwrite}, nil)
	choice.SetSelected(optNew)

	headline := "Save bookmarks & TOC to:"
	if empty {
		headline = "No entries — this will REMOVE all bookmarks/TOC from the file. Save to:"
	}
	content := container.NewVBox(widget.NewLabel(headline), choice)

	d := dialog.NewCustomConfirm("Save to PDF", "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		if choice.Selected == optOverwrite {
			bp.saveBookmarksTo(bp.v.doc.Path())
			return
		}
		bp.showSaveAsDialog()
	}, bp.v.win)

	d.Resize(fyne.NewSize(460, 220))
	d.Show()
}

// showSaveAsDialog lets the user pick the exact destination file and folder
// for a new bookmarked copy, rather than always suffixing "-bookmarked" onto
// the original name in its own folder — pre-filled with that as a starting
// point, but changeable.
func (bp *bookmarkPanel) showSaveAsDialog() {
	origPath := bp.v.doc.Path()
	dir := filepath.Dir(origPath)
	base := strings.TrimSuffix(filepath.Base(origPath), filepath.Ext(origPath))

	fd := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, bp.v.win)
			return
		}
		if w == nil {
			return // user cancelled
		}
		path := w.URI().Path()
		w.Close()
		bp.saveBookmarksTo(path)
	}, bp.v.win)

	fd.SetFileName(base + "-bookmarked.pdf")
	if lister, err := storage.ListerForURI(storage.NewFileURI(dir)); err == nil {
		fd.SetLocation(lister)
	}
	fd.Show()
}

func (bp *bookmarkPanel) saveBookmarksTo(outputPath string) {
	empty := !bp.v.doc.Bookmarks.HasBookmarks()
	if err := bp.v.doc.Bookmarks.SaveBookmarks(outputPath); err != nil {
		dialog.ShowError(fmt.Errorf("failed to save: %w", err), bp.v.win)
		return
	}
	verb := "Saved to"
	if empty {
		verb = "Removed all entries; saved to"
	}
	bp.showTransientInfo("Success", fmt.Sprintf("%s:\n%s", verb, outputPath))
}

func (bp *bookmarkPanel) showTransientInfo(title, msg string) {
	d := dialog.NewInformation(title, msg, bp.v.win)
	d.Show()
	time.AfterFunc(2*time.Second, func() {
		fyne.Do(d.Hide)
	})
}

func (bp *bookmarkPanel) deleteSelected() {
	bookmark := bp.selected
	if bookmark == nil {
		dialog.ShowInformation("No Selection", "Please select an entry to delete", bp.v.win)
		return
	}
	dialog.ShowConfirm("Delete", fmt.Sprintf("Delete '%s'?", bookmark.Title), func(ok bool) {
		if !ok {
			return
		}
		if bp.v.doc.Bookmarks.DeleteBookmark(bookmark) {
			bp.selected = nil
			bp.tree.UnselectAll()
			bp.rebuildTreeData()
			bp.tree.Refresh()
			bp.showTransientInfo("Deleted", fmt.Sprintf("Deleted '%s'", bookmark.Title))
		} else {
			dialog.ShowError(fmt.Errorf("failed to delete entry"), bp.v.win)
		}
	}, bp.v.win)
}

func (bp *bookmarkPanel) deleteAll() {
	userAdded := bp.v.panelMode != PanelTOC
	what := "bookmarks"
	if !userAdded {
		what = "table-of-contents entries"
	}
	dialog.ShowConfirm("Delete All", fmt.Sprintf("Delete ALL %s?", what), func(ok bool) {
		if !ok {
			return
		}
		n := bp.v.doc.Bookmarks.DeleteByType(userAdded)
		bp.selected = nil
		bp.tree.UnselectAll()
		bp.rebuildTreeData()
		bp.tree.Refresh()
		bp.showTransientInfo("Deleted", fmt.Sprintf("Deleted %d %s", n, what))
	}, bp.v.win)
}
