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
- Draw Underline/Strikeout/Squiggly ourselves, the same way "Draw Highlight"
  (highlight_draw.go, wired into view.go's toolbar) already lets a user add a
  brand-new Highlight annotation by dragging a rectangle. Painting one, once
  drawn, is no longer the hard part — see 0.5.0's "real MuPDF annotation
  rendering" note below: any annotation actually saved into the PDF now
  renders correctly with zero extra painting code, whatever its kind. What's
  still missing is authoring: a UI to pick which of the four kinds a
  freshly-drawn rectangle should become, plus a
  model.New{Underline,StrikeOut,Squiggly}Annotation branch in
  SaveHighlights' new-annotation path alongside the existing
  NewHighlightAnnotation one. Until saved, an in-progress draw still needs
  paintHighlights' existing hand-rolled preview (unchanged, still Kind-aware)
  to show live.
- Square/Circle/Ink/Polygon/PolyLine/Stamp/FreeText/Caret — every other
  annotation kind PDF defines — no longer needs its own geometry parsing and
  draw routine the way this list previously assumed (see 0.5.0's note below):
  MuPDF renders any of them correctly, generically, the moment they're
  actually saved into the file. 0.5.0 also added listing, click-select
  (page and Highlights panel both), and delete for all of these, via a
  generic `/Rect`-based bounding box (enough to select/outline/delete one,
  not enough to hand-paint or recolor it — MuPDF's own render already
  covers the former). Still missing, for a future version:
  - **Move**: repositioning would mean rewriting each kind's own geometry
    (`/Rect` plus whatever else that kind stores — `/L` for Line,
    `/QuadPoints` for markup, `/Vertices` for Ink/Polygon) and, for
    anything with a baked `/AP` appearance stream, regenerating that
    stream too — a bare geometry update alone won't move what's actually
    painted, the same class of gap the 0.5.0 color-change fix (below) had
    to work around for recoloring.
  - **Draw new ones / edit existing ones** (e.g. Preview's drag-the-midpoint
    arrow-curving trick): authoring UI plus a `SaveHighlights` branch per
    kind, and matching whatever non-standard representation an app like
    Preview uses for an "edited" shape, reverse-engineered from a real
    file the way the Line/arrow geometry work above already was once.
- Real text-snapped highlight drawing, like Preview/Acrobat's "select text
  → highlight" — today's "Draw Highlight" (see Version 0.4.0) is a free
  hand-drawn rectangle, not snapped to text, because go-fitz has no word/
  line bounding-box API to select against. Would need either extending
  go-fitz's own cgo bindings to walk MuPDF's stext char boxes (real
  engineering, and has to cover both the cgo and purego backends), or
  accepting line-level-only granularity some other way. Only
  handleHighlightDrawn's geometry math would need to change for this, not
  the panel/save plumbing (AddHighlight/SaveHighlights don't care how a
  quad was produced).
- Maybe a bigger effort - similar to Mac Preview ability to show pdf page thumbnails to make finding test, highlights, shapes etc easier?
- Preserve PDF region-bookmark exact scroll position on save (currently
  degrades to page-level — pdfcpu's bookmark API has no position field)
- A visible highlight box for Text/Markdown find matches (Fyne's Entry/
  RichText widgets have no public API for painting a selection from code)


## Version 0.5.0 - September 29, 2026

### ✨ NEW

- Underline, Strikeout, and Squiggly annotations that another app (Preview,
  Acrobat, ...) already put in a PDF now paint on the page, alongside
  Highlight — a solid colored line/mark at each one's own real position and
  color, not the flat rectangle tint Highlight uses. They're also
  click-selectable on the page and recolorable from the Highlights panel's
  "Change Color" button now, the same as Highlight already was.
- Line annotations (an arrow, or a plain straight line) that another app
  already drew now paint too — the shaft plus an arrowhead at either end,
  reading each end's real style (Open/Closed/None) from /LE. Found and
  worked around a real-world data quirk while adding this: a genuine
  Preview-authored arrow had one /L endpoint sitting ~145pt outside its own
  /Rect and off the page entirely (almost certainly stale from an earlier
  edit that resized /Rect without updating /L to match) — this app now
  repairs a bad coordinate by mirroring it from the annotation's own
  still-good endpoint (not by clamping to /Rect's far edge, which put the
  arrow noticeably further up and at a steeper angle than Preview's own
  rendering of the same arrow) so it lands close to where it really
  belongs, confirmed by rendering the real file and comparing side-by-side
  against Preview.
- Real MuPDF annotation rendering: every annotation already saved in a PDF
  now paints pixel-accurately, whatever its kind — Stamp (a freeform
  shape/speech-bubble drawing tool, e.g. Preview's "shapes") included, which
  has no fixed geometry a hand-rolled draw routine could ever reconstruct,
  only an arbitrary vector-art appearance stream. This app now depends on
  github.com/amarillier/go-fitz (a small fork of go-fitz, wired in via
  go.mod's replace directive) instead of upstream directly: upstream's
  Image/ImageDPI only ever call MuPDF's fz_run_page_contents (content
  stream only, see CLAUDE.md's "Painting PDF highlights" section), and nothing
  in go-fitz exposes MuPDF's own fz_run_page (content + every annotation's
  real appearance) as a public method at all. The fork adds one new method,
  ImageWithAnnotsDPI, that calls it. This app's own hand-rolled painting
  (Highlight/Underline/Strikeout/Squiggly/Line, above and in earlier
  versions) still runs, but now only for an annotation that doesn't exist in
  the file yet (freshly drawn, not yet saved) — MuPDF already paints
  everything already on disk correctly, so hand-painting those too would
  double them up.
- Every other annotation kind PDF defines — Square, Circle, Polygon,
  PolyLine, Ink, Stamp, FreeText, Caret — now shows up in the Highlights
  panel, is click-selectable on the page, and deletes, the same as
  Highlight/Underline/Strikeout/Squiggly/Line already did. These carry no
  per-kind geometry (Stamp especially has none at all to reconstruct — see
  the real MuPDF rendering note above), so selection and hit-testing fall
  back to each one's overall `/Rect` — enough to select, outline, and
  delete one, not enough to hand-paint or recolor it, which MuPDF's own
  rendering already covers anyway.
- Deleting a highlight/shape no longer requires selecting it in the
  Highlights panel and then reaching for its "Delete Selected" button:
  right-click (or long-press) any highlight/shape directly on the page for
  a quick "Delete" menu right at the cursor, or press Delete/Backspace
  after selecting one (by page click, right-click, or the panel's own
  list) to delete it immediately. Both still show the same confirmation
  dialog the panel's button always has.

### 🐛 FIXED

- Save to PDF could leave a file's cross-reference table subtly malformed
  after repeated rounds of saving (both this app's and another app's, e.g.
  Preview's, layered on top of each other) — invisible to this app before
  0.5.0, since nothing here ever read annotations back out of a file's own
  structure until real MuPDF annotation rendering (above) started doing
  exactly that. Once it did, the symptom became real and visible: a Line
  annotation's arrowhead silently reverted to a plain straight line, and a
  Highlight stopped painting at all. The first fix tried here — running
  pdfcpu's own xref-table optimizer before every write, to flatten
  everything into one clean revision — did fix that, but turned out to
  cause a worse problem of its own: an object referenced *only* from
  inside an annotation's opaque, app-private data (e.g. Preview's own
  `AAPL:AKExtras` metadata) looks unreferenced to that optimizer, so it
  quietly got pruned — eventually making Save to PDF fail outright with
  "missing xref table entry" the next time this app tried to delete any
  annotation on that page. No longer runs that optimizer at all; instead,
  a failed annotation removal now triggers a targeted repair (scan the
  file's own object graph for a dangling reference and patch it in place)
  and one retry, only when actually needed. A cosmetic "needs repair"
  message MuPDF handles silently on its own is a far smaller problem than
  a hard, blocking save failure.
- Save to PDF could make Preview stop treating one of its own annotations
  (a shape/speech-bubble drawn with Preview's own tools) as a live,
  editable object — it would still show up, and other apps' PDF renderers
  wouldn't see anything wrong at all, but Preview itself would only let
  you move or resize it, with no further editing (an arrow's arrowhead
  reverting to a plain uneditable line was the case that surfaced this).
  Root cause: SaveHighlights was rewriting every already-saved highlight's
  `/Contents` and `/C` on every save, unconditionally, even ones nothing
  had actually changed about. Preview attaches its own private editing
  metadata to a shape it authored (visible in the raw PDF as
  `AAPL:AKAnnotationObject`/`AAPL:AKExtras`); rewriting an annotation's
  dict for no reason forces pdfcpu to fully re-emit it rather than copy
  its bytes through unchanged, and that was apparently enough to break
  Preview's own trust in that metadata, even though the rewritten values
  were identical to what was already there. Fix: SaveHighlights now only
  touches a highlight's `/Contents` or `/C` when it's actually diverged
  from what was loaded. Confirmed a no-op save now leaves an untouched
  annotation's full dict — private metadata included — byte-for-byte
  identical to before the save, which it did not before this fix.
- Deleting an already-saved highlight removed it from the Highlights
  panel immediately but left its paint visible on the page until Save to
  PDF and a reopen — confusing, easy to read as "the delete didn't work."
  Now disappears immediately, no save required: the page renders from a
  real scratch copy of the file with the deletion actually applied,
  rather than a hand-painted patch over the region — the first version of
  this fix used exactly that kind of patch, and it visibly broke when a
  deleted annotation's own bounding box overlapped a different, still-live
  one (two arrows crossing near the same text, in the case that surfaced
  it) — the patch had no way to know that overlapping region should still
  show the other annotation, so it blanked part of it too, Stamp
  annotations included. A real render has no such blind spot.
- Save to PDF (overwriting the original file) could leave an already-open
  tab still showing stale, pre-save page renders — e.g. after Preview added
  something new since this app last rendered it, or after removing a
  highlight that turned out to need the repair fix above — until the tab
  was fully closed and reopened. The page cache was never being told the
  underlying file had just changed. Now cleared on every overwrite (a
  Save-As, which leaves the original file untouched, correctly leaves it
  alone).
- "Save as a new file..." (Highlights panel) now switches the tab to
  editing the newly-saved file — its own title updates to the new
  filename, its Highlights/Bookmarks panels and page all reflect what was
  actually just written to disk — instead of leaving the tab pointed at
  the original file while its own bytes have quietly diverged from what
  the tab shows (e.g. deleted shapes still appearing to be there, since
  they were only ever removed from a saved copy, not the file the tab was
  still open on). The original file is left exactly as it was either way;
  this only changes which file the tab keeps editing afterward — matching
  how Save As behaves in most editors.
- Opening a PDF that had already been through several rounds of saving
  (this app's own, interleaved with another app's) could show a highlight
  with no color at all, or a Line annotation's arrowhead reverted to a
  stale straight-line revision from before it had been curved in Preview —
  both on a file MuPDF printed a "repairing" warning for on open, and both
  gone after one clean read-and-rewrite round-trip through pdfcpu. Every
  PDF open (and every highlight edit) now always renders from a
  freshly-normalized scratch copy rather than the file directly, so this
  whole bug class can't come up — no detection needed, and the original
  file's own timestamp is never touched by it (the normalized copy is a
  temp file, read-only against the real one).
- Changing an already-saved highlight's color (Change Color in the
  Highlights panel) now shows the new color on the page immediately, no
  Save required — matching the "delete shows immediately" fix above. A
  first attempt at this just rewrote the highlight's `/C` in the
  normalized scratch copy, and it did nothing visually: a real
  Preview/Acrobat-authored annotation already carries its own baked
  appearance stream (`/AP`), and MuPDF's real annotation rendering always
  paints from that stream when present, regardless of `/C` — confirmed by
  dumping actual rendered pixels before and after the rewrite and finding
  them byte-identical. The real fix instead deletes the recolored
  annotation from the scratch copy entirely (the same safe mechanism the
  delete fix above uses) and lets this app's own hand-painting draw it in
  the new color — safe against a different, overlapping annotation for
  the same reason the delete fix is.


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
