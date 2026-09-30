# KrankyBear Reader

A cross-platform desktop document reader/viewer for Windows, Linux, and
macOS, built with Go and the [Fyne](https://fyne.io/) GUI toolkit. Open text,
data, and PDF files in tabs, with the right viewer chosen automatically for
each.

Design philosophy aligns with Fyne: ease of use, solid functionality, steady
bug fixing and performance work.

## Features

- **Tabbed, multi-format viewing** — open several files at once (drag-and-drop
  supports multiple files in one go), each in its own tab:
  - **Text / log files** — read-only, selectable plain text.
  - **JSON, YAML, TOML** — a searchable, collapsible tree, similar to
    [jsonviewer.stack.hu](https://jsonviewer.stack.hu/).
  - **XML** — the same searchable, collapsible tree, with attributes and
    text content shown as their own rows under each element.
  - **Markdown** — rendered (headings, emphasis, lists, links, blockquotes).
  - **CSV / TSV** — a table with a frozen header row and auto-detected
    delimiter.
  - **Images** (PNG, JPEG, GIF, BMP, WebP) — fit-to-window or 100% view.
  - **PDF** — continuous-scroll or page-by-page reading, zoom (fixed levels
    or Fit Width/Page), a Table of Contents panel, Bookmarks you can add,
    rename, and save back into the PDF's standard outline (readable by other
    PDF apps too, and reachable straight from the File menu/tray, not just
    the panel), and a Highlights and Notes panel showing every annotation
    already in the file (e.g. from Preview or Acrobat) — painted directly
    onto the page in its own color, whatever its kind. Draw new
    Highlight/Square/Circle/Line (arrow)/Star/Hexagon/Text block/Speech
    Bubble annotations directly on the page (a toolbar dropdown picks the
    kind, then drag); click one on the page (or pick it from the panel) to
    select it, then Change Color or delete it (Delete/Backspace, or the
    panel's own button); Escape or clicking empty page space deselects.
    Save to PDF overwrites the original or saves a new copy (which the tab
    then follows) — changes made here are real PDF edits other readers see
    too.
  - **Anything else** falls back to a hex + ASCII dump rather than refusing
    to open it or mishandling binary content as text.
- **Find, everywhere** — every viewer has a find bar (plain text or regex):
  CSV filters rows, JSON/YAML/TOML/XML filters the tree, Text/Markdown jump
  between matches, and PDF reports a real "Match X of Y (found on N pages)"
  count and steps through every individual occurrence — including several
  on the same page — drawing an approximate highlight box around the
  current match directly on the page (estimated from the page's own text
  layout, not a pixel-exact lookup, so it can occasionally land a little
  off on an unusual line).
- **Sample Files** (Help menu) — a quick tour of every supported format
  except PDF (most people already have real PDFs on hand to try instead).
- **Recent Files**, drag-and-drop, and command-line file arguments all open
  through the same path — no format-specific special casing to open a file.
- **Startup Behavior** (View menu) — optionally reopen the most recent file,
  or every file left open at last quit, next time the app launches.
- **Show All / Hide All Windows**, including an Alt+H boss key, for hiding
  every open window (main + About/Help/Release Notes/Update) at once.

## Cross-platform support

- **Linux**: GNOME, KDE, XFCE, Cinnamon, MATE, etc. on X11 or Wayland.
- **macOS**: 10.13 (High Sierra) or later.
- **Windows**: Windows 10 or later. Some VMs and locked-down hosts have no
  usable hardware OpenGL, which most Fyne apps otherwise crash or hang on
  with no explanation — KrankyBear Reader automatically probes for it at
  launch and, only if that fails, falls back to a bundled Mesa3D software
  renderer and relaunches itself, with no user action needed. Real hardware
  OpenGL is always preferred when available (it's faster); nothing changes
  on a normal machine with a working GPU. The installer bundles this
  fallback automatically. The **portable (zip) Windows build does not** —
  if you're running the portable version on a machine without hardware
  OpenGL, grab `mesa-fallback.zip` from the same release, extract it into a
  `mesa-fallback` folder next to `KrankyBearReader.exe`, and the same
  automatic fallback applies. Most users on a normal machine will never
  need this file at all. If a host is already known to need the fallback
  (e.g. a golden VM image reused for many identical no-GPU VMs), manually
  moving `opengl32.dll` and `libgallium_wgl.dll` out of `mesa-fallback`
  and into the same folder as the `.exe` itself skips the one-time
  probe-and-relaunch delay on that machine's very first launch too.

## Building & running

Requires Go and a Fyne-capable toolchain (CGo + OpenGL on desktop; PDF
viewing additionally requires a C toolchain for its MuPDF binding — see
CLAUDE.md's cgo notes):

```
go run .
go build -o <app> .
```

Platform helpers: `compile-mac.sh`, `compile-win.sh`, `compile-linux.sh`, and
`package.sh` (`.deb`/`.rpm`, macOS `.pkg`).

## Known limitations

- PDF region bookmarks (an exact scroll position within a page, not just the
  page itself) aren't preserved when saved back into the PDF — they degrade
  to page-level bookmarks on save; the exact position is remembered only for
  the current session.
- Existing highlights/shapes can be selected, deleted, and (for the kinds
  this app can itself draw) recolored, but not yet moved, resized, or
  otherwise edited directly on the page — planned for a future release.
- A few annotation kinds (Stamp, Ink, PolyLine, Caret) can be viewed,
  selected, and deleted, but not drawn by this app — anything already
  saved in a PDF still renders and behaves correctly either way.
- Find in Text/Markdown views moves to each match but can't paint a visible
  highlight box around it (the underlying text widgets have no public API
  for that). PDF find's own highlight box is an estimate based on the
  page's text layout, not an exact character-position lookup, so it can
  occasionally land a little off on an unusual line.

## License

Free for personal, educational and commercial use, under the GNU GPL-3.0.

## Author

Allan Marillier

## Acknowledgments

- Built with [Fyne](https://fyne.io/) — an easy-to-use GUI toolkit for Go.
- PDF rendering via [go-fitz](https://github.com/gen2brain/go-fitz) (MuPDF);
  PDF bookmark/annotation reading and writing via
  [pdfcpu](https://github.com/pdfcpu/pdfcpu).
