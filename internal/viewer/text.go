package viewer

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// maxRenderedTextBytes caps how much content NewTextView/NewMarkdownView will
// actually hand to Fyne's text widgets, independent of isProbablyText's
// binary-content check. Even genuinely valid text can be slow to lay out at
// large sizes (word-wrapping millions of characters is expensive regardless
// of whether the shaper handles it correctly), so this is a defense against
// that cost specifically, not against wrong classification.
const maxRenderedTextBytes = 2 * 1024 * 1024 // 2MB

// truncateForDisplay caps content to maxRenderedTextBytes, prefixing a notice
// when it does — mirrors NewHexView's "showing first N of M bytes" pattern.
func truncateForDisplay(content string) string {
	if len(content) <= maxRenderedTextBytes {
		return content
	}
	return fmt.Sprintf("(showing the first %d of %d bytes)\n\n%s",
		maxRenderedTextBytes, len(content), content[:maxRenderedTextBytes])
}

// readOnlyEntry is a widget.Entry that displays and lets the user select/copy
// text but blocks editing. A plain Entry with Disable() called also blocks
// keyboard-driven selection, and widget.Label/RichText don't support mouse
// text-selection at all — this is the smallest widget that gets both.
type readOnlyEntry struct {
	widget.Entry
}

func newReadOnlyEntry(content string, monospace bool) *readOnlyEntry {
	e := &readOnlyEntry{}
	e.MultiLine = true
	e.TextStyle = fyne.TextStyle{Monospace: monospace}
	if monospace {
		// Monospace content here is always pre-formatted, fixed-width rows
		// (currently just the hex dump view) that must NOT be reflowed —
		// word-wrapping would break the column alignment the hex/ASCII
		// layout depends on. It also turns out to matter for performance,
		// not just correctness: confirmed via manual testing, word-wrap
		// layout of a large monospace hex dump (tens of KB of content) was
		// the actual source of a real, severe (tens-of-seconds) slowdown
		// opening certain files — not an infinite hang, but bad enough to
		// look like one. Turning wrapping off for this content removes that
		// reflow cost entirely; the widget scrolls horizontally instead.
		e.Wrapping = fyne.TextWrapOff
	} else {
		e.Wrapping = fyne.TextWrapWord
	}
	e.ExtendBaseWidget(e)
	e.SetText(content)
	return e
}

// TypedRune blocks character insertion; selection/copy via mouse or keyboard
// navigation (handled by TypedKey below and by Entry's own shortcut/mouse
// handling) still work.
func (e *readOnlyEntry) TypedRune(_ rune) {}

// TypedKey passes through navigation/selection keys but blocks the ones that
// would mutate the (unsaved, display-only) text buffer.
func (e *readOnlyEntry) TypedKey(ev *fyne.KeyEvent) {
	switch ev.Name {
	case fyne.KeyBackspace, fyne.KeyDelete, fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
		return
	}
	e.Entry.TypedKey(ev)
}

// NewTextView renders content as read-only, selectable plain text, with a
// find bar. Find moves the cursor to each match (widget.Entry has no public
// API to paint a highlighted selection — CursorRow/CursorColumn are the only
// exported position controls) and requests focus so the caret is visible;
// for a very long file you may still need to scroll a little; Entry's
// scroll-into-view logic is only wired to its own keyboard handling, not to
// cursor changes made from outside the widget.
func NewTextView(win fyne.Window, content string) fyne.CanvasObject {
	text := truncateForDisplay(content)
	entry := newReadOnlyEntry(text, false)

	var offsets []int
	current := -1

	jump := func() {
		if current < 0 || current >= len(offsets) {
			return
		}
		row, col := rowColForRuneOffset(text, offsets[current])
		entry.CursorRow = row
		entry.CursorColumn = col
		entry.Refresh()
		win.Canvas().Focus(entry)
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

	return container.NewBorder(bar.content, nil, nil, nil, entry)
}
