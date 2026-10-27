package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var helpWindow fyne.Window
var helpManagedWindow *managedWindow

// showHelp displays comprehensive help documentation
// Reusable pattern from KrankyBearClock - customize these for your app:
//   - appName: Your application name
//   - resourceKrankyBearReaderPng: Your embedded icon resource
//   - helpText: Your application's help content (see below for structure)
//   - GitHub and License URLs
//
// Help text structure recommendation:
//   - Use section headers with visual separators (━━━)
//   - Group related features together
//   - Include tips, tricks, and known limitations
//   - Add keyboard shortcuts
//   - Provide links to external resources
func showHelp(a fyne.App) {
	if helpWindow != nil && helpWindow.Content().Visible() {
		helpManagedWindow.open = true
		helpWindow.Show()
		helpWindow.RequestFocus()
		return
	}

	helpWindow = a.NewWindow(appName + " - Help")
	helpWindow.SetIcon(resourceKrankyBearReaderPng)

	helpText := `` + appName + ` - Help

OVERVIEW:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
A tabbed, multi-format document reader. Open a file via File → Open..., drag
one (or several at once) onto the window, pass paths on the command line, or
pick one from Recent Files — each opens in its own tab, with the right viewer
chosen automatically. Try Help → Sample Files for a quick tour of every
supported format except PDF (you almost certainly already have a real PDF
on hand to try that viewer with instead).

SUPPORTED FORMATS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Text / log files — read-only, selectable, plain text.
• JSON, YAML, TOML — a searchable, collapsible tree (like
  jsonviewer.stack.hu), with Expand All / Collapse All.
• XML — the same searchable, collapsible tree, with attributes and text
  content shown as their own rows (@name, #text) under each element.
• Markdown — rendered (headings, bold/italic, lists, links, blockquotes).
• CSV / TSV — a table with a frozen header row; the delimiter is
  auto-detected.
• Images (PNG, JPEG, GIF, BMP, WebP) — Fit to Window or View at 100%.
• PDF — see below.
• Anything else (or a file that turns out not to be safely renderable as
  text) falls back to a hex + ASCII dump rather than refusing to open it.

PDF VIEWING:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Continuous Scroll or page-by-page (toggle in the toolbar); zoom to a fixed
  level or Fit Width / Fit Page.
• Table of Contents: the PDF's own outline, if it has one.
• Page Thumbnails: small previews of every page with your annotations shown;
  click one to jump there. "Annotated pages only" lists just the pages that
  have highlights or shapes.
• Bookmarks: add your own (optionally at your exact scroll position, not
  just the top of the page), then Save to PDF — also reachable from the File
  menu (and tray) without opening the panel first. Saving as a new file
  opens a normal save dialog to pick the exact name and folder; overwriting
  writes straight back to the original. This writes to the PDF's standard
  outline, so other readers (Preview, Acrobat, ...) see them too — they'll
  just appear under their own "Table of Contents", since the
  page-vs-bookmark distinction is this app's own convention, not a PDF
  standard. Saving with zero entries removes the outline entirely. Edit
  Title renames a selected TOC entry or Bookmark in place (its page/position
  are left as-is); to change where one points, delete and re-add it instead.
• Highlights and Notes: lists every annotation already in the PDF (e.g.
  made in Preview or Acrobat) — tap to jump to that page. Every kind paints
  directly onto the page, in its own color where it has one, whatever its
  kind (Highlight, Underline, Strikeout, Squiggly, Line/arrow, Square,
  Circle, Polygon, FreeText, Stamp, Ink, and more).
• Draw: pick a kind from the toolbar's "Draw: ..." dropdown (Highlight,
  Underline, Strikeout, Squiggly, Square, Circle, Line, Star, Hexagon,
  Text, Boxed Text, Speech Bubble, Ink, PolyLine, Polygon, Connect Points), pick a line weight from the
  dropdown next to it (Square/Circle/Line/Polygon/Ink/PolyLine only —
  meaningless for the others), then drag on the page to add it
  (Highlight/Underline/Strikeout/Squiggly snap to the text you drag across,
  line by line; a drag over a figure or blank area stays a plain rectangle). Text and
  Speech Bubble prompt for the caption right after the drag; Line draws as
  an arrow from where you start dragging to where you release; Ink and
  PolyLine both trace your actual freehand drag as a single pen stroke,
  not a rectangle (your stroke is traced live as you drag; one that ends
  near where it began snaps shut). Polygon is the same freehand drag but
  always closes on release into a real closed shape. Connect Points is
  click-to-place instead of a drag: click each vertex (a live line follows
  your pointer), then click the last vertex again or press Enter to finish
  a PolyLine, or click the first vertex to close it into a Polygon; Escape
  cancels. A Text block drawn too small to read is enlarged to a minimum
  size so it can't end up invisible.
• Select and edit: click any highlight/shape directly on the page, or pick
  it from the panel's own list, to select it — shown with a blue outline.
  "Change Color" recolors it (for the kinds this app can draw itself);
  "Change Line Weight" adjusts its stroke width (Square/Circle/Line/
  Polygon/Ink/PolyLine only); "Change Text Size" sets a Text block or
  Speech Bubble's font size (10-24pt); "Toggle Border" turns a text block's box
  outline on or off (Text is borderless, Boxed Text has an outline); Delete/Backspace (or the panel's "Delete
  Selected" button) removes it; Escape, or clicking empty page space,
  deselects without changing anything. With Draw mode off, drag a selected
  shape (one this app can draw itself) directly on the page to reposition
  it, or drag one of the small square handles around it to resize (a
  Line's endpoints, a Speech Bubble's tail, and a bounding-box grip on
  everything else).
• Save to PDF: writes your changes back — overwrite the original, or "Save
  as a new file..." (the tab then follows the new file). Real edits to the
  PDF's own annotations, so other readers (Preview, Acrobat, ...) see them
  too.

FIND:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Every format has a find bar: plain text (case-insensitive) or regular
expressions via the Regex checkbox.
• CSV filters to matching rows. JSON/YAML/TOML/XML filters the tree to
  matching keys/values (or tags/attributes/text for XML). Text and Markdown
  jump to each match in turn (Markdown's jump is an approximate scroll
  position, not exact — the underlying widget has no cursor concept the way
  the plain-text view does).
• PDF search reports a real "Match X of Y (found on N pages)" count and
  steps through every individual occurrence, including several on the same
  page, drawing an exact highlight box directly on the page around the
  current match (one box per line if it wraps).

WINDOW MANAGEMENT:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Show All Windows / Hide All Windows (View menu and tray) show or hide the
main window plus any open About/Help/Release Notes/Update windows together.
Alt+H is the "boss key" equivalent of Hide All Windows from anywhere in the
app. There's deliberately no matching show-again hotkey — reveal via the
tray or View menu instead.

STARTUP BEHAVIOR:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
View → Startup Behavior... controls what reopens automatically on launch:
nothing (the default), just the most recently opened file, or every file
that was still open when the app last quit. A file passed on the command
line or dropped onto the app icon at launch always takes precedence over
this setting.

KEYBOARD SHORTCUTS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Cmd/Ctrl+O - Open a file
• Cmd/Ctrl+W - Close the current tab
• Alt+H - Hide all windows
• Space, Page Down/Up, Home, End - Page navigation, while a PDF tab is the
  selected one (these don't fire in other tabs)
• Delete/Backspace - Delete the currently selected PDF highlight/shape
  (PDF tab only; does nothing with no selection)
• Escape - Deselect the currently selected PDF highlight/shape (PDF tab
  only)
• Standard system shortcuts also apply (Cmd/Ctrl+Q to quit, etc.)

KNOWN LIMITATIONS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Existing highlights/shapes (the kinds this app can draw) can be selected,
  recolored, moved, resized, and deleted; ones it can't fully rebuild
  (e.g. a Polygon from another app) can be deleted but not moved or
  resized.
• Stamp and Caret annotations can be viewed, selected, and deleted, but not
  drawn by this app — anything already saved in the file still renders and
  behaves correctly either way.
• Text/Markdown find moves to each match but can't paint a visible
  highlight box around it — a limitation of the underlying text widgets,
  not a missing feature.

MORE INFORMATION:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
For documentation, bug reports, or feature requests:
📦 GitHub: https://github.com/amarillier/KrankyBearReader
📄 License: https://github.com/amarillier/KrankyBearReader/blob/main/LICENSE
📝 Release Notes: Help → Release Notes

FREE SOFTWARE - Use anywhere, anytime, any purpose!
No registration, no tracking, no phone-home (except manual update checks).
`

	helpLabel := widget.NewLabel(helpText)
	helpLabel.Wrapping = fyne.TextWrapWord

	// Links - update URLs for your project
	githubURL, _ := url.Parse("https://github.com/amarillier/KrankyBearReader")
	githubLink := widget.NewHyperlink("Visit GitHub Repository", githubURL)
	githubLink.Alignment = fyne.TextAlignCenter

	licenseURL, _ := url.Parse("https://github.com/amarillier/KrankyBearReader/blob/main/LICENSE")
	licenseLink := widget.NewHyperlink("View License", licenseURL)
	licenseLink.Alignment = fyne.TextAlignCenter

	// Create scrollable area with minimum size for better readability
	scrollContent := container.NewScroll(helpLabel)
	scrollContent.SetMinSize(fyne.NewSize(750, 550))

	// Layout with better proportions
	header := container.NewVBox(
		widget.NewLabelWithStyle(appName+" - Help", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
	)

	footer := container.NewVBox(
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(githubLink, licenseLink)),
	)

	content := container.NewBorder(header, footer, nil, nil, scrollContent)

	helpWindow.SetContent(container.NewPadded(content))
	helpWindow.Resize(fyne.NewSize(850, 700))

	helpWindow.SetCloseIntercept(func() {
		helpManagedWindow.open = false
		helpWindow.Hide()
	})
	registerBossKeyHideAll(helpWindow)

	helpManagedWindow = registerManagedWindow(helpWindow)
	helpManagedWindow.open = true
	helpWindow.Show()
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
