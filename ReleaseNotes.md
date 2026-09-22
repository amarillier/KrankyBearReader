# KrankyBear Reader Application - Release Notes

## Windows, Linux and MacOS multi-format document reader/viewer

## Future ideas:
- Highlights: a way to annotate one with a brief comment, like a Bookmark or
  TOC entry's title — reading an existing comment (a highlight's own
  /Contents) already works today via the Highlights panel; writing one would
  need pdfcpu low-level dict mutation, since pdfcpu's public API has no
  "update an existing annotation" call, only Add/Remove
- Paint existing PDF underlines, strikeouts, and squiggly marks onto the
  rendered page too, alongside highlights (same technique, just needs each
  shape's own draw routine instead of a filled rectangle)
- Author new PDF highlights/annotations from within the app (currently
  read-only/list-only)
- Preserve PDF region-bookmark exact scroll position on save (currently
  degrades to page-level — pdfcpu's bookmark API has no position field)
- A visible highlight box for Text/Markdown find matches (Fyne's Entry/
  RichText widgets have no public API for painting a selection from code)

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
