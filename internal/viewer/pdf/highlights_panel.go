package pdf

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// highlightsPanel is the read-only "Highlights and Notes" side panel,
// matching Preview.app's own sidebar (per the user's screenshot): every
// highlight/underline/strikeout/squiggly/note annotation already in the
// PDF, listed by page with an excerpt, tap to jump to that page. Unlike
// bookmarkPanel there's nothing to add/delete/save — this reads what other
// apps already wrote (see Document.Highlights / LoadHighlights), it doesn't
// author new annotations.
type highlightsPanel struct {
	v         *view
	container *fyne.Container
	list      *widget.List
}

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
			label.SetText(fmt.Sprintf("p.%d  %s\n%s", h.Page, h.Kind, excerpt))
			label.Wrapping = fyne.TextWrapWord
		},
	)
	hp.list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(v.doc.Highlights) {
			v.jumpToPage(v.doc.Highlights[id].Page)
		}
	}

	var content fyne.CanvasObject = hp.list
	if len(v.doc.Highlights) == 0 {
		content = container.NewCenter(widget.NewLabel("No highlights or notes in this PDF"))
	}
	hp.container = container.NewBorder(nil, nil, nil, nil, content)
	return hp
}
