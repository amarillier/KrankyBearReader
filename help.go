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
supported format.

SUPPORTED FORMATS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Text / log files — read-only, selectable, plain text.
• JSON, YAML, TOML — a searchable, collapsible tree (like
  jsonviewer.stack.hu), with Expand All / Collapse All.
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
• Bookmarks: add your own (optionally at your exact scroll position, not
  just the top of the page), then Save to PDF. This writes to the PDF's
  standard outline, so other readers (Preview, Acrobat, ...) see them too —
  they'll just appear under their own "Table of Contents", since the
  page-vs-bookmark distinction is this app's own convention, not a PDF
  standard. Saving with zero entries removes the outline entirely.
• Highlights and Notes: lists highlights, underlines, strikeouts, and notes
  already in the PDF (e.g. made in Preview or Acrobat) — tap to jump to that
  page. Read-only: this app doesn't add or edit highlights, and — a real
  limitation of the underlying renderer, not a missing checkbox — an
  existing highlight won't visually paint on the page here the way it does
  in Preview, only in the list.

FIND:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Every format has a find bar: plain text (case-insensitive) or regular
expressions via the Regex checkbox.
• CSV filters to matching rows. JSON/YAML/TOML filters the tree to matching
  keys/values. Text and Markdown jump to each match in turn (Markdown's jump
  is an approximate scroll position, not exact — the underlying widget has
  no cursor concept the way the plain-text view does).
• PDF search is page-level: it jumps to the next/previous page containing a
  match, not to the exact spot on the page.

WINDOW MANAGEMENT:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Show All Windows / Hide All Windows (View menu and tray) show or hide the
main window plus any open About/Help/Release Notes/Update windows together.
Alt+H is the "boss key" equivalent of Hide All Windows from anywhere in the
app. There's deliberately no matching show-again hotkey — reveal via the
tray or View menu instead.

KEYBOARD SHORTCUTS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• Cmd/Ctrl+O - Open a file
• Cmd/Ctrl+W - Close the current tab
• Alt+H - Hide all windows
• Space, Page Down/Up, Home, End - Page navigation, while a PDF tab is the
  selected one (these don't fire in other tabs)
• Standard system shortcuts also apply (Cmd/Ctrl+Q to quit, etc.)

KNOWN LIMITATIONS:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
• PDF region bookmarks (an exact scroll position, not just a page) aren't
  preserved on Save to PDF today — they're saved as page-level bookmarks;
  the position is remembered only for the current session.
• Highlights/notes are read-only: viewable and listed, not added, edited, or
  painted onto the rendered page.
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
