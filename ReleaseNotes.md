# KrankyBear Reader Application - Release Notes

## Windows, Linux and MacOS multi-format document reader/viewer

## Future ideas:
- Preview-style arrow curving (dragging an arrow's midpoint): Preview stores an
  edited arrow in a non-standard way, so this is not currently feasible.
  Deferred, and maybe never.
- Stamp / Caret annotations: Preview's shapes (star, speech bubble, loupe) are
  Stamps made of arbitrary vector art, so this app can't author or move them.
  Not currently feasible. Deferred, and maybe never.


## Version 0.9.0 - October 27, 2026

### ✨ NEW

- Draw: Connect Points — a click-to-place PolyLine tool. Click each vertex
  (a live line follows the pointer), then click the last vertex again or
  press Enter to finish an open PolyLine, or click the first vertex to close
  it into a Polygon. Escape cancels; changing the Draw kind or page also
  drops an unfinished shape. The existing freehand PolyLine is unchanged.
- A Text block drawn too small to hold any text (easy to do with a borderless
  one, which has no outline to find) is now enlarged to a minimum size at
  creation, and grown to fit its text, instead of being invisible.

## Version 0.8.1 - October 27, 2026

### ✨ NEW

- Text-snapped markup: Highlight, Underline, Strikeout and Squiggly now snap
  to the text under your drag, like selecting text in Preview or Acrobat —
  drag from one word to another and every character in between is marked,
  one band per line, even across several lines. A drag that doesn't start
  and end on or near text (over a figure, or blank margin) still draws a
  plain rectangle as before. Multi-line markup now saves in full (only the
  first line used to be written).
- Exact find highlight: the PDF search box now sits precisely on the matched
  text, one box per line for a match that wraps, instead of the earlier
  font-size estimate (which could land a little off, or in the wrong column).
  The old estimate remains only as a fallback for a page with no text layer.

- "Draw: Polygon": trace a freehand outline and it closes itself when you
  let go (a faint line shows where it will close while you draw), then is
  thinned to a sensible number of corners and saved as a real PDF Polygon.
- Ink and PolyLine strokes that end close to where they began now snap shut
  onto the start point, so an almost-closed loop really is closed.
- Text blocks can now be borderless or boxed. "Draw: Text" is borderless, like
  Preview's own "Text" tool (just the text, in the chosen color); the new
  "Draw: Boxed Text" adds an outline (both are also in the right-click Add
  Shape menu). The Highlights panel's new "Toggle Border" switches a selected
  text block either way, including ones already saved. A Speech Bubble keeps
  its outline.
- Changing a Text block or Speech Bubble's size now grows its box downward
  if the bigger text wouldn't fit (best effort; you can still resize it).

### 🐞 FIXED

- The live pen trace for Ink/PolyLine/Polygon now uses your chosen color and
  line weight (scaled to the page's on-screen size), so it looks like the
  finished shape instead of a thin orange line that thickened on release.
- A PolyLine/Ink/Polygon stroke finished exactly on its own start point
  could vanish entirely: the drag looked like a click (start and end the
  same spot) and was discarded. Whether it was a click is now judged from
  the whole stroke's extent.
- The live pen trace while drawing Ink/PolyLine didn't appear in the real
  window (it only showed up in headless tests) — the drawing surface never
  asked for a repaint as the stroke grew.

- Selecting an existing shape (on the page or in the Highlights panel) now
  switches the "Draw:" dropdown back to Off, so dragging its handles or body
  resizes/moves it instead of starting a new shape.
- Changing the text size (or anything else) on a Text block from another app
  no longer turns its text yellow: the text color is now read from the
  annotation's own text style (RGB, gray or CMYK, or its default-style
  string), falling back to black rather than highlighter yellow. For a
  Text block from Preview this now reads the color and size from its
  appearance (the red, ~13.5pt text you actually see) — its stored
  text-style fields are stale placeholders.
- Changing a Text block's size no longer shows an empty box until you resize
  it: when the new size is too big for the box, the first line of text is
  still drawn (overflowing, as other PDF viewers do) instead of nothing.

## Version 0.8.0 - October 26, 2026

### ✨ NEW

- Adjustable text size for Text block and Speech Bubble annotations: the
  Highlights panel's new "Change Text Size" button offers 10/12/14/18/24pt
  (a closed list, like Line Weight). The size is written into the PDF
  itself, so other viewers show it too, and the in-app preview now uses a
  scalable font that follows both the chosen size and the zoom level.

- Drawing Ink or PolyLine now shows your actual pen stroke live as you
  drag, instead of the bounding-box rectangle every other shape uses.

- Resize an already-placed shape: select it and drag the small square
  handles that appear — 8 around the bounding box for most shapes
  (Highlight/Underline/Strikeout/Squiggly, Square, Circle, Star/Hexagon,
  Text, Speech Bubble, Ink, PolyLine; freehand strokes and multi-line
  highlights scale as a whole), the two endpoints for a Line/arrow, and an
  extra handle on a Speech Bubble's tail so you can aim it precisely. A
  live outline shows the new size while you drag. Works for shapes from
  other apps too, wherever Move already did; a shape this app can't fully
  rebuild shows no handles.

- Page thumbnails panel (the panel dropdown's new "Page Thumbnails"): a
  scrolling strip of small page previews with your highlights and shapes
  painted in, each labelled with its page number and annotation count. Click
  one to jump there; the current page stays highlighted as you navigate. Tick
  "Annotated pages only" to list just the pages that have annotations — a
  quick way to find your highlights and shapes in a long document. Previews
  render a few at a time in the background of the UI so large documents stay
  responsive, and refresh when you add, change or delete an annotation.

### 🐞 FIXED

- Text blocks and Speech Bubbles saved by this app rendered as a solid
  filled box in the annotation's own color, hiding their text (the text was
  the same color as the fill). The color is now carried by the text/border
  alone, so the text is visible after saving. Already-saved ones from older
  versions load as before.

## Version 0.7.0 - October 01, 2026

### ✨ NEW

- The toolbar's highlight color and line weight pickers now remember your
  last choice across app launches, instead of always resetting to yellow
  and 1pt — pick a color/weight once and every new shape you draw, in this
  session or the next, starts from it.
- Right-click empty space on a page for a quick "Add Bookmark Here"/"Add
  TOC Entry Here"/"Add Highlight Here"/"Add Shape" menu, right where you
  clicked — faster than the separate paths these already had (the
  Bookmarks/TOC panel's own "Add" button always uses the current page and
  scroll position, not wherever you clicked; adding a highlight or shape
  needed the toolbar's "Draw: ..." dropdown set first, then a drag). "Add
  Shape" opens a submenu with every other placeable kind the toolbar's own
  "Draw: ..." dropdown offers (Underline, Strikeout, Squiggly, Square,
  Circle, Line, Star, Hexagon, Text, Speech Bubble — Ink/PolyLine aren't
  offered here since they're freehand strokes with no sensible default
  shape for a single click to place). Every option uses your current
  color/line-weight pickers and a sensible default size — a quick way to
  drop one in without switching the toolbar into Draw mode first.
- A region bookmark's exact scroll position now survives a real save and
  reload — not just for as long as the tab stays open, which is as far as
  0.6.0 got. Previously the position only ever lived in memory
  (`Bookmark.YOffset`), so closing and reopening the file (or relaunching
  the app) silently degraded a region bookmark to a plain page one, the
  moment it had to be re-read from the PDF's own bytes. Now written as a
  real `/XYZ` destination (page, left, top, zoom — PDF's own standard
  exact-position outline destination, not a custom extension), so other
  PDF viewers that understand `/XYZ` destinations (which is most of them)
  will also jump to the right spot, not just this app.
- Draw new Underline, Strikeout, and Squiggly annotations directly on the
  page, the same "Draw: ..." dropdown and click-drag gesture as every
  other shape. Previews live before Save to PDF (the same hand-paint
  routines 0.5.0 already added for reading one of these back from another
  app's PDF), and reads back correctly in Preview/Acrobat/any other real
  PDF viewer once saved.
- Move an already-placed highlight/shape: with Draw mode off, drag a
  selected shape (one this app can draw itself — Highlight/Underline/
  Strikeout/Squiggly/Square/Circle/Line/Polygon/FreeText/Ink/PolyLine, the
  last two added later this same version) directly on the page to
  reposition it, the same click-and-drag as everywhere else in this app.
  Works whether it was just drawn in this session or loaded from another
  app's PDF, and whether or not it's been saved yet; a shape this app
  can't fully reconstruct (e.g. a Polygon from another app with no
  recorded vertices) can't be dragged at all, rather than silently losing
  its real shape on the next save. Resizing isn't supported yet — see
  Future ideas.
- Draw Ink: a new "Draw: Ink" option traces your actual freehand drag as
  a single pen stroke, not a rectangle — the first "Draw" kind that isn't
  a drag-out-a-shape gesture. Previews live before Save to PDF, reads
  back correctly in Preview/Acrobat/any other real PDF viewer once saved,
  and — like every shape this app can draw itself — can be recolored and
  moved afterward too.
- Draw PolyLine: a new "Draw: PolyLine" option, right alongside Ink and
  using the exact same freehand-drag capture — saved under PDF's own
  distinct `/PolyLine` subtype instead of `/Ink`. Not (yet) a real
  click-a-vertex-at-a-time tool — see Future ideas.
- Line weight: pick a stroke/border width from a new dropdown next to the
  color swatch before drawing a Square, Circle, Line, Polygon, Ink, or
  PolyLine, and change it afterward via the Highlights panel's new
  "Change Line Weight" button — the same before/after pairing Change
  Color already had. Persists through Save to PDF and reopening, for a
  shape drawn in this app or loaded from another one.

### 🐛 FIXED

- Recoloring (or moving) an Ink/PolyLine stroke or a Star/Hexagon/speech
  bubble shape right after Save to PDF — even in the same session, no
  reopen needed — turned it into a plain rectangle instead. Any save at
  all was silently discarding the real shape data for every one of these,
  because this app has no way to read that data back out of the PDF it
  just wrote (a PDF limitation, not something this app can fix), and
  Save to PDF was fully re-reading the file afterward rather than keeping
  what it already had in memory. Save to PDF now keeps that data across
  a save instead of discarding and trying to re-read it.
- Recoloring one of this app's own shapes (Square/Circle/Polygon/
  FreeText/Ink), saving, and reopening the file could show the shape's
  color as black in the Highlights panel even though the saved file's own
  color was correct and the page rendered correctly — the in-memory
  record of a shape's own color was never actually being read back on
  reload for any of these kinds, only for Highlight/Underline/Strikeout/
  Squiggly/Line.
- Change Color on a highlight/shape originally made by another app
  (Preview, Acrobat, ...) looked like it worked — the page updated
  immediately — but reopening the file afterward (even back in this same
  app) silently reverted to the original color. Save to PDF was only ever
  rewriting the annotation's `/C` entry, but a real PDF viewer always
  prefers an annotation's own baked appearance over `/C` when one exists,
  and every annotation another app creates has one; this app's own
  freshly-drawn shapes never did, which is why this never showed up
  testing with those. Save to PDF now replaces a recolored annotation
  outright with a freshly-authored one in the new color, the same
  approach already used for making a just-drawn shape's own color show up
  immediately.
- Changing Line Weight on an already-saved Ink or PolyLine stroke made it
  visually disappear and show as a plain rectangle instead, until Save to
  PDF, close, and reopen — even though nothing was actually lost (the
  real shape was always there once reopened). This app has no way to
  read an existing Ink/PolyLine's exact path back out of a saved PDF, so
  recoloring/reweighting one of these always had to delete-and-replace it
  for the live preview to update, and had nothing but a plain box to put
  back with. Changing Line Weight (or Color) on one of these now simply
  leaves the existing shape's old-but-correctly-shaped appearance on
  screen — with the real new weight/color still saved for real once you
  Save to PDF — and shows a brief, auto-closing notice that the visible
  update is pending until then, rather than replacing it with a
  misleading box.

## Version 0.6.0 - September 30, 2026

### ✨ NEW

- Deselect a highlight/shape without deleting or changing it: press
  Escape, or click anywhere on the page with nothing under it. A small
  safety net now that a bare Delete/Backspace acts on whatever's
  currently selected — and it'll matter more once shapes can be moved or
  edited directly, where an accidental drag on a selection you no longer
  meant to have live would otherwise be the risk.
- Draw new Square, Circle, and Line (arrow) shapes directly on the page,
  the same click-drag gesture "Draw Highlight" already used — pick the
  kind from the toolbar's "Draw: ..." dropdown (replacing the old single
  "Draw Highlight" checkbox), drag on the page, done. Line draws as an
  arrow from where you start dragging to where you release. All three
  preview live before Save to PDF, same as Highlight already did, and
  read back correctly in Preview/Acrobat/any other real PDF viewer once
  saved.
- Draw Star, Hexagon, a plain text block, and a speech bubble too — same
  "Draw: ..." dropdown, same drag gesture. Star/Hexagon are a parametric
  shape fit to the drag's own rectangle (PDF has no dedicated subtype for
  either, so both are authored as a generic Polygon, same as any other
  closed-shape annotation). Text block and Speech Bubble prompt for the
  caption text right after the drag, and preview live — including the
  actual wrapped text, not just an empty box — before Save to PDF; Speech
  Bubble additionally gets a callout tail pointing away from the box
  (aiming it precisely, or editing an already-drawn shape's text, is a
  future shape-edit feature, not this one).
- "Change Color" now works on Square, Circle, Star/Hexagon, and text
  block/speech bubble shapes — previously refused with "not supported"
  for all four, the same as it correctly still does for a kind this app
  has no way to paint at all (Stamp, Ink, ...). Works whether the shape
  was just drawn in this session or loaded from another app's PDF. Also
  deselects the shape right after changing its color, so the new color
  is immediately visible instead of possibly being masked by the
  selection outline drawn on top of it.
- Real document-wide search: the find bar now reports "Match X of Y
  (found on N pages)" and next/prev steps through every individual
  occurrence — including several on the same page — instead of only ever
  jumping to the nearest page with *any* match. Also draws an
  approximate highlight box around the current match directly on the
  page (orange, distinct from both the blue selection outline and any
  real annotation color) — estimated from MuPDF's own HTML-export line
  positions plus a character-count/font-size width estimate, since
  go-fitz's text extraction has no real character-coordinate API; not
  pixel-perfect like Preview's own box, but lands on the right word on
  the right line (see Future ideas for what a truly exact box would
  still need).

### 🐛 FIXED

- Searching the same text again after manually navigating elsewhere (e.g.
  jumping to page 1, or clicking a bookmark) kept stepping through the
  old match sequence from wherever it had left off, instead of noticing
  you'd moved and starting again from where you now are.
- Clicking directly on a highlight/shape on the page (as opposed to
  selecting it from the Highlights panel's list) was unreliable for a
  precise target like a thin straight line — a real click with a hair of
  physical jitter (routine on a trackpad, and more likely for a careful,
  precise click on a small target than a quick tap on a big one) could
  silently do nothing at all, with no error and no visual feedback,
  because of how Fyne itself decides whether a click-and-release counts as
  a tap or a drag. Selecting via the Highlights panel's own list was
  never affected — only clicking on the page.
- Even after the above fix, clicking exactly on a shape could still miss
  and register somewhere else on the page instead — worse at some window
  sizes than others. The page image can end up centered within a larger
  area than its own natural size (a normal side-effect of how the
  scrollable page view lays out its content), and clicks weren't
  accounting for that centering, so they were measured against the wrong
  frame of reference. Every click, drag, and right-click on the page is
  now corrected for this.
- Clicking a region bookmark (one added at an exact scroll position, not
  just the top of a page) jumped to the right page but never actually
  scrolled down to that position — it silently landed at the page top
  every time, even though the exact position was being remembered
  correctly the whole time. The page just wasn't being told to repaint
  after the scroll position changed underneath it.

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
