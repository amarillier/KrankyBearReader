# KrankyBear Reader Application - Release Notes

## Windows, Linux and MacOS multi-format document reader/viewer

## Future ideas:
- Real PDF text search, phased (today's find bar — view.go's buildFindBar —
  is page-level only: it jumps to the nearest page whose extracted text
  matches, one hit at a time, with no total count):
  - Phase 1 — document-wide match count and true next/prev-occurrence
    stepping (not just next/prev matching page): scan every page's
    already-available PageText once per query, count every occurrence
    (regexp.FindAllStringIndex, or an index-scan for plain-text mode) into
    a flat ordered (page, occurrence) list, show "Found N matches on M
    pages" (like Preview's "Found on 16 pages"), and have the arrows step
    through that flat list, wrapping around, jumping pages as needed even
    for multiple hits on the same page. Fully achievable now, no new
    dependencies.
  - Phase 2 — a visible highlight box on the matched word on the page
    itself, like Preview/Acrobat's search. Blocked by the exact same gap
    as the next item below (real text-snapped highlight drawing):
    go-fitz's text extraction has no character/word coordinates, so
    there's nothing to draw a box around. The same go-fitz cgo-binding
    extension would unlock both features at once — worth doing together,
    not twice.
- Real text-snapped highlight drawing, like Preview/Acrobat's "select text
  → highlight" — today's "Draw Highlight" (see Version 0.5.0) is a free
  hand-drawn rectangle, not snapped to text, because go-fitz has no word/
  line bounding-box API to select against. Would need either extending
  go-fitz's own cgo bindings to walk MuPDF's stext char boxes (real
  engineering, and has to cover both the cgo and purego backends), or
  accepting line-level-only granularity some other way. Only
  handleHighlightDrawn's geometry math would need to change for this, not
  the panel/save plumbing (AddHighlight/SaveHighlights don't care how a
  quad was produced).
- Paint existing PDF underlines, strikeouts, and squiggly marks onto the
  rendered page too, alongside highlights (same technique, just needs each
  shape's own draw routine instead of a filled rectangle)
- Maybe a bigger effort - similar to Mac Preview ability to show pdf page thumbnails to make finding test, highlights, shapes etc easier?
- Preserve PDF region-bookmark exact scroll position on save (currently
  degrades to page-level — pdfcpu's bookmark API has no position field)
- A visible highlight box for Text/Markdown find matches (Fyne's Entry/
  RichText widgets have no public API for painting a selection from code)


## Version 0.4.0 - September 25, 2026

### 🐛 FIXED

- About window's HardHat "ahead of latest release" badge could keep showing
  for up to a day after actually publishing a matching release — it now
  runs its own fresh check each time the window opens instead of trusting
  the once-a-day launch check's same-day cache, which can predate a release
  published later that same day

### ✨ NEW

- PDF: the Highlights panel can now caption any highlight — one made in
  another PDF app (Preview, Acrobat, ...) or drawn in KrankyBear Reader
  itself — with a short comment via Edit Caption, shown in the panel list
  instead of a bare "p.NN Highlight"
- PDF: a new "Draw Highlight" toolbar toggle — click-drag a rectangle
  directly on the page to create a brand-new highlight, in both
  single-page and Continuous Scroll modes. Not text-snapped like Preview/
  Acrobat's select-to-highlight (see Future ideas) — a free rectangle you
  position and size by hand
- PDF: Delete Selected in the Highlights panel removes a highlight —
  drawn in-app or made elsewhere — from the page
- PDF: Change Color in the Highlights panel, and a color swatch next to
  "Draw Highlight" for the next one you draw — Fyne's own full-RGB color
  picker, not just a handful of presets
- PDF: click a highlight on the page to select its row in the Highlights
  panel (and vice versa — selecting a row outlines that highlight on the
  page in blue), so it's always clear which list entry is which highlight
- PDF: Save to PDF in the Highlights panel now writes all of the above —
  captions, colors, newly-drawn highlights, and deletions — together in
  one save
- PDF: the Highlights panel's rows now size themselves to fit their own
  caption — a long one used to silently overlay the row below it instead
  of wrapping into a taller row
- PDF: reopening a file (Recent Files, drag-drop, command line, or
  Startup Behavior's own auto-reopen) now lands back on whichever page you
  were last reading in it, instead of always starting at page 1

## Version 0.3.0 - September 22, 2026

### ✨ NEW

- PDF: existing Highlight annotations (made in Preview, Acrobat, ...) now
  paint directly onto the rendered page — one translucent rectangle per
  highlighted line, in the highlight's own color when the PDF specifies one,
  yellow otherwise — not just listed in the Highlights and Notes panel
- PDF: Edit Title on a TOC entry or Bookmark — rename it in place without
  deleting and re-adding it; page and position are left untouched
- PDF: the panel dropdown's closed-state label now reads "Show Panel" while
  hidden and "Hide Panel" once a panel is open, instead of always saying
  "Hide Panel"
- PDF: "Save as a new file" now opens a real file-save dialog to choose the
  exact destination name and folder, instead of always saving next to the
  original with "-bookmarked" appended
- PDF: "Save to PDF..." is now in the File menu (and tray menu) too, so
  saving bookmarks/TOC changes doesn't require opening the panel first
- Startup Behavior (View menu): optionally reopen the most recently opened
  file, or every file that was still open at last quit, the next time the
  app launches — off by default. A file passed on the command line or
  dropped at launch always takes precedence over this

## Version 0.2.0 - September 18, 2026

### ✨ NEW

- XML viewer: the same searchable, collapsible tree as JSON/YAML/TOML, with
  attributes (@name) and element text (#text) shown as their own rows
- PDF: the TOC/Bookmarks/Highlights panel dropdown moved from the far right
  of the toolbar to the far left, right above where the panel itself opens —
  quicker to switch between them, with no side-panel width used up while
  it's hidden

### 🐛 FIXED

- PDF: turning on Continuous Scroll while reading any page but the first no
  longer jumps back to page 1 — it now stays on the page you were reading

## Version 0.1.0 - September 17, 2026
### ✨ NEW Cross-platform multi-format document reader/viewer

- Windows, Linux and macOS desktop app
- Tabbed viewer for Text, JSON/YAML/TOML (searchable collapsible tree, like
  https://jsonviewer.stack.hu/), Markdown, CSV/TSV, and images — anything
  else falls back to a hex/ASCII dump rather than refusing to open
- Real PDF viewing: continuous-scroll and page-by-page reading, zoom (fixed
  levels or Fit Width/Page), a Table of Contents panel, Bookmarks you can
  add and save into the PDF's standard outline (readable by other PDF
  viewers too), and a Highlights and Notes panel listing annotations already
  in the file
- Find in every viewer (plain text or regex): row-filtering in CSV,
  tree-filtering in JSON/YAML/TOML, jump-to-match in Text/Markdown, and
  jump-to-page in PDF
- Open several files at once — drag multiple files onto the window
  simultaneously, or pass several as command-line arguments — each opens in
  its own tab
- Sample Files (Help menu) — one example file per supported format
- Recent Files, drag-and-drop, and command-line arguments all open through
  the same code path
- Show All / Hide All Windows (View menu, tray, and an Alt+H boss key) for
  the main window plus any open About/Help/Release Notes/Update windows
