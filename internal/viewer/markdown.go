package viewer

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewMarkdownView renders content as Markdown (via widget.NewRichTextFromMarkdown,
// the same call already proven in releasenotes.go) wrapped in an explicit
// scroll container — RichText does not scroll on its own, it relies on
// being hosted inside one (releasenotes.go already does this; this view
// previously didn't, a real gap for long documents independent of find),
// plus a find bar.
func NewMarkdownView(win fyne.Window, content string) fyne.CanvasObject {
	text := truncateForDisplay(content)
	rt := widget.NewRichTextFromMarkdown(text)
	rt.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(rt)

	var offsets []int
	current := -1

	// RichText, unlike Entry, has no cursor/position concept at all, so an
	// exact jump isn't possible without deeper Fyne internals. This scrolls
	// to the match's approximate position instead: its fraction of the way
	// through the source text, applied to the scrollable height. Close
	// enough to find the right neighborhood, not a precise jump.
	jump := func() {
		if current < 0 || current >= len(offsets) || len(text) == 0 {
			return
		}
		frac := float32(offsets[current]) / float32(len([]rune(text)))
		scrollable := rt.MinSize().Height - scroll.Size().Height
		if scrollable < 0 {
			scrollable = 0
		}
		scroll.Offset = fyne.NewPos(0, frac*scrollable)
		scroll.Refresh()
	}

	var bar *findBar
	bar = newFindBar(
		func(query string, useRegex bool) {
			var err error
			offsets, err = findAllOffsets(text, query, useRegex)
			if err != nil {
				bar.SetStatus(err.Error())
				return
			}
			current = -1
			switch {
			case query == "":
				bar.SetStatus("")
			case len(offsets) == 0:
				bar.SetStatus("no matches")
			default:
				bar.SetStatus(fmt.Sprintf("%d match(es)", len(offsets)))
			}
		},
		func() { // next
			if len(offsets) == 0 {
				return
			}
			current = (current + 1) % len(offsets)
			jump()
		},
		func() { // prev
			if len(offsets) == 0 {
				return
			}
			current--
			if current < 0 {
				current = len(offsets) - 1
			}
			jump()
		},
	)

	return container.NewBorder(bar.content, nil, nil, nil, scroll)
}
