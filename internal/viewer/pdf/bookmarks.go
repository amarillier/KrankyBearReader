package pdf

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// bookmarkTitlePrefix marks an outline entry as a user "bookmark" (vs the document's own
// table-of-contents) when written into the PDF. A single BMP glyph keeps it portable: pdfcpu
// stores titles as UTF-16BE so it round-trips cleanly, and it renders in other readers too.
// On load, entries with this prefix are shown as bookmarks (the prefix is stripped for
// display); entries without it are TOC. Ported from pdfviewer's bookmarks.go.
const bookmarkTitlePrefix = "⚑ "

// Bookmark represents a PDF bookmark/outline entry.
type Bookmark struct {
	Title       string
	PageNo      int
	YOffset     float32 // Vertical scroll position as a fraction (0..1) for region bookmarks
	Children    []*Bookmark
	Level       int
	Bold        bool
	Italic      bool
	IsUserAdded bool // true = added in this app (a "bookmark"); false = the PDF's own outline (TOC)
}

// BookmarkManager handles PDF bookmark operations for one open Document.
type BookmarkManager struct {
	pdfPath   string
	bookmarks []*Bookmark
}

// NewBookmarkManager creates a new bookmark manager.
func NewBookmarkManager() *BookmarkManager {
	return &BookmarkManager{bookmarks: make([]*Bookmark, 0)}
}

// LoadBookmarks loads bookmarks/TOC from a PDF file. Not an error if the PDF
// simply has none.
func (bm *BookmarkManager) LoadBookmarks(pdfPath string) error {
	bm.pdfPath = pdfPath
	bm.bookmarks = make([]*Bookmark, 0)

	ctx, err := api.ReadContextFile(pdfPath)
	if err != nil {
		return fmt.Errorf("failed to open PDF: %w", err)
	}

	bookmarkList, err := pdfcpu.Bookmarks(ctx)
	if err != nil {
		log.Printf("[WARN] no bookmarks found or error reading bookmarks: %v", err)
		return nil
	}

	bm.bookmarks = convertPdfcpuBookmarks(bookmarkList)
	populateRegionOffsets(ctx.XRefTable, bm.bookmarks)
	return nil
}

// populateRegionOffsets fills in YOffset for every bookmark (recursively)
// whose own named destination is a real /XYZ array rather than pdfcpu's
// always-whole-page-/Fit default — see writeRegionDestinations' own doc
// comment for why pdfcpu's Bookmark/AddBookmarks never writes one itself,
// and regionYOffsetForKey for how the position is recovered. A bookmark
// with no such destination (every page bookmark, every TOC entry from this
// app, and any entry whose destination this app never touched) is left at
// its zero-value YOffset, same as before this existed.
func populateRegionOffsets(xRefTable *model.XRefTable, bookmarks []*Bookmark) {
	for _, b := range bookmarks {
		key := b.Title
		if b.IsUserAdded {
			key = bookmarkTitlePrefix + key
		}
		b.YOffset = regionYOffsetForKey(xRefTable, key, b.PageNo)
		if len(b.Children) > 0 {
			populateRegionOffsets(xRefTable, b.Children)
		}
	}
}

// regionYOffsetForKey recovers the scroll-position fraction (0..1, top to
// bottom) a region bookmark's own named destination was given by
// writeRegionDestinations, by reading back the real /XYZ array's own "top"
// coordinate and the target page's actual MediaBox height. Returns 0 (page
// bookmark, "jump to top") for anything that isn't exactly this shape — no
// destination at all, a /Fit or other non-/XYZ destination, a malformed
// array, or a page whose MediaBox can't be read — the same degrade-to-
// page-bookmark fallback this feature had before a position could be
// written at all.
//
// Does not account for the target page's own /Rotate: the position is
// computed against the page's raw, pre-rotation MediaBox, which matches
// how writeRegionDestinations writes it, but a rotated page's own rendered
// (and so scrolled) height differs from that raw MediaBox height by more
// than a simple fraction — a known, narrow gap, not yet hit on a real
// rotated test file.
func regionYOffsetForKey(xRefTable *model.XRefTable, key string, pageNo int) float32 {
	arr, err := xRefTable.DereferenceDestArray(key)
	if err != nil || len(arr) < 4 {
		return 0
	}
	name, ok := arr[1].(types.Name)
	if !ok || name.Value() != "XYZ" {
		return 0
	}
	top, err := xRefTable.DereferenceNumber(arr[3])
	if err != nil {
		return 0
	}
	_, _, pAttrs, err := xRefTable.PageDict(pageNo, false)
	if err != nil || pAttrs == nil || pAttrs.MediaBox == nil {
		return 0
	}
	h := pAttrs.MediaBox.Height()
	if h <= 0 {
		return 0
	}
	frac := 1 - (top-pAttrs.MediaBox.LL.Y)/h
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return float32(frac)
}

func convertPdfcpuBookmarks(pdfcpuBookmarks []pdfcpu.Bookmark) []*Bookmark {
	return convertPdfcpuBookmarksRecursive(pdfcpuBookmarks, 1)
}

func convertPdfcpuBookmarksRecursive(pdfcpuBookmarks []pdfcpu.Bookmark, level int) []*Bookmark {
	bookmarks := make([]*Bookmark, 0, len(pdfcpuBookmarks))

	for _, pb := range pdfcpuBookmarks {
		title := pb.Title
		userAdded := false
		if strings.HasPrefix(title, bookmarkTitlePrefix) {
			userAdded = true
			title = strings.TrimPrefix(title, bookmarkTitlePrefix)
		}

		bookmark := &Bookmark{
			Title:       title,
			PageNo:      pb.PageFrom,
			Level:       level,
			Bold:        pb.Bold,
			Italic:      pb.Italic,
			Children:    make([]*Bookmark, 0),
			IsUserAdded: userAdded,
		}

		if len(pb.Kids) > 0 {
			bookmark.Children = convertPdfcpuBookmarksRecursive(pb.Kids, level+1)
		}

		bookmarks = append(bookmarks, bookmark)
	}

	return bookmarks
}

// GetBookmarks returns all top-level bookmarks/TOC entries.
func (bm *BookmarkManager) GetBookmarks() []*Bookmark {
	return bm.bookmarks
}

// AddBookmark adds a new entry at the top level with optional region
// position. userAdded=true marks it as a user Bookmark; false makes it a
// Table-of-Contents entry. Both are written to the standard PDF outline on
// save.
func (bm *BookmarkManager) AddBookmark(title string, pageNo int, yOffset float32, userAdded bool) *Bookmark {
	bookmark := &Bookmark{
		Title:       title,
		PageNo:      pageNo,
		YOffset:     yOffset,
		Level:       1,
		Children:    make([]*Bookmark, 0),
		IsUserAdded: userAdded,
	}
	bm.bookmarks = append(bm.bookmarks, bookmark)
	return bookmark
}

// RenameBookmark updates a bookmark/TOC entry's title in place, leaving its
// page, position, and children untouched — the "just fix the text" edit that
// doesn't require deleting and re-adding the entry. newTitle must be
// non-empty (blank titles aren't accepted by AddBookmark's dialog either).
func (bm *BookmarkManager) RenameBookmark(bookmark *Bookmark, newTitle string) error {
	if strings.TrimSpace(newTitle) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	bookmark.Title = newTitle
	return nil
}

// DeleteBookmark removes a bookmark (searched recursively).
func (bm *BookmarkManager) DeleteBookmark(bookmark *Bookmark) bool {
	return deleteBookmarkRecursive(&bm.bookmarks, bookmark)
}

// DeleteByType removes all top-level entries of the given kind (userAdded=true → bookmarks,
// false → TOC) and returns how many were removed.
func (bm *BookmarkManager) DeleteByType(userAdded bool) int {
	kept := make([]*Bookmark, 0, len(bm.bookmarks))
	removed := 0
	for _, b := range bm.bookmarks {
		if b.IsUserAdded == userAdded {
			removed++
			continue
		}
		kept = append(kept, b)
	}
	bm.bookmarks = kept
	return removed
}

func deleteBookmarkRecursive(bookmarks *[]*Bookmark, target *Bookmark) bool {
	for i, b := range *bookmarks {
		if b == target {
			*bookmarks = append((*bookmarks)[:i], (*bookmarks)[i+1:]...)
			return true
		}
		if deleteBookmarkRecursive(&b.Children, target) {
			return true
		}
	}
	return false
}

// HasBookmarks returns true if there are any bookmarks/TOC entries.
func (bm *BookmarkManager) HasBookmarks() bool {
	return len(bm.bookmarks) > 0
}

// SaveBookmarks writes the current entries into the PDF's outline at
// outputPath. With zero entries it strips the outline entirely.
//
// Reads d.pdfPath into a *model.Context itself (rather than the simpler
// api.AddBookmarksFile/RemoveBookmarksFile one-shot helpers this used
// before) so that, after pdfcpu.AddBookmarks builds the outline tree, this
// can still patch in a real position for every region bookmark via
// writeRegionDestinations — pdfcpu's own API has no way to ask for
// anything but a whole-page destination up front. Mirrors the same
// read-ctx/mutate/write-atomically shape SaveHighlights already uses for
// its own pdfcpu API gaps.
func (bm *BookmarkManager) SaveBookmarks(outputPath string) error {
	if bm.pdfPath == "" {
		return fmt.Errorf("no PDF loaded")
	}

	ctx, err := api.ReadContextFile(bm.pdfPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", bm.pdfPath, err)
	}

	if len(bm.bookmarks) == 0 {
		removed, err := pdfcpu.RemoveBookmarks(ctx)
		if err != nil {
			return fmt.Errorf("failed to clear bookmarks: %w", err)
		}
		if !removed {
			if outputPath != "" && outputPath != bm.pdfPath {
				return copyFile(bm.pdfPath, outputPath)
			}
			return nil
		}
		return writeContextAtomically(ctx, outputPath)
	}

	pdfcpuBookmarks := bm.convertToPdfcpuFormat()
	if err := pdfcpu.AddBookmarks(ctx, pdfcpuBookmarks, true); err != nil {
		return fmt.Errorf("failed to save bookmarks: %w", err)
	}

	writeRegionDestinations(ctx, bm.bookmarks)

	return writeContextAtomically(ctx, outputPath)
}

// writeRegionDestinations overwrites each region bookmark's own named
// destination — just created by pdfcpu.AddBookmarks as a whole-page /Fit
// array (bmDict, pdfcpu's own bookmark.go, always writes exactly
// [pageRef /Fit], with no way to ask for anything else) — with a real
// /XYZ array carrying that bookmark's exact scroll position. pdfcpu's own
// Bookmark/AddBookmarks has no concept of a position within a page at
// all, so this is the same raw-dict/raw-object mutation escape hatch
// annotations.go's highlightGeometry-adjacent code already uses for
// similar pdfcpu API gaps (see CLAUDE.md) — just on an array object
// instead of a dict.
//
// Looks each bookmark's destination up by the exact key pdfcpu's own
// bmDict registered it under in ctx.Names["Dests"] (the title string
// actually written to the PDF, bookmarkTitlePrefix included for a
// user-added entry) rather than trying to walk the outline tree pdfcpu
// just built — pdfcpu's own createOutlineItemDictDepth doesn't hand back
// per-item object numbers, but every title it writes is already a unique
// key in the Dests name tree (pdfcpu itself errors out on AddBookmarks if
// two entries ever collide, independent of this feature), which is all a
// name-tree lookup needs.
func writeRegionDestinations(ctx *model.Context, bookmarks []*Bookmark) {
	for _, b := range bookmarks {
		if b.YOffset > 0 {
			key := b.Title
			if b.IsUserAdded {
				key = bookmarkTitlePrefix + key
			}
			if err := setRegionDestination(ctx, key, b.PageNo, b.YOffset); err != nil {
				log.Printf("[WARN] bookmark %q: writing region position: %v", b.Title, err)
			}
		}
		if len(b.Children) > 0 {
			writeRegionDestinations(ctx, b.Children)
		}
	}
}

// setRegionDestination replaces the array object a bookmark's named
// destination (in ctx.Names["Dests"], added by pdfcpu's own bmDict) points
// to with a real /XYZ destination: [page /XYZ null top null] — left and
// zoom null ("leave as the viewer's current value", a real PDF null per
// spec, not merely absent) since this app only ever controls vertical
// scroll position, never horizontal offset or zoom level. top is computed
// from yOffset (0..1, top to bottom) against the target page's own raw
// MediaBox height — see regionYOffsetForKey's doc comment for the
// (deliberately accepted, not yet hit in practice) rotated-page caveat
// that comes with using the raw MediaBox rather than a rotation-aware
// size.
func setRegionDestination(ctx *model.Context, key string, pageNo int, yOffset float32) error {
	dNames := ctx.Names["Dests"]
	if dNames == nil {
		return fmt.Errorf("no destinations name tree")
	}
	obj, ok := dNames.Value(key)
	if !ok {
		return fmt.Errorf("no named destination for %q", key)
	}
	ref, ok := obj.(types.IndirectRef)
	if !ok {
		return fmt.Errorf("named destination for %q is not an indirect reference", key)
	}
	entry, ok := ctx.XRefTable.FindTableEntryLight(ref.ObjectNumber.Value())
	if !ok {
		return fmt.Errorf("named destination object %d not found", ref.ObjectNumber.Value())
	}

	pageIndRef, err := ctx.XRefTable.PageDictIndRef(pageNo)
	if err != nil {
		return fmt.Errorf("page %d: %w", pageNo, err)
	}

	_, _, pAttrs, err := ctx.XRefTable.PageDict(pageNo, false)
	if err != nil {
		return fmt.Errorf("page %d: %w", pageNo, err)
	}
	if pAttrs == nil || pAttrs.MediaBox == nil {
		return fmt.Errorf("page %d: no media box", pageNo)
	}

	top := pAttrs.MediaBox.LL.Y + pAttrs.MediaBox.Height()*float64(1-yOffset)
	entry.Object = types.Array{*pageIndRef, types.Name("XYZ"), nil, types.Float(top), nil}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func (bm *BookmarkManager) convertToPdfcpuFormat() []pdfcpu.Bookmark {
	return convertBookmarksRecursive(bm.bookmarks)
}

func convertBookmarksRecursive(bookmarks []*Bookmark) []pdfcpu.Bookmark {
	// pdfcpu requires sibling bookmarks in non-decreasing page order.
	// Bookmarks are stored in add-order, so sort a copy by page before
	// writing; stable sort keeps add-order among same-page entries.
	sorted := make([]*Bookmark, len(bookmarks))
	copy(sorted, bookmarks)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PageNo < sorted[j].PageNo })

	result := make([]pdfcpu.Bookmark, 0, len(sorted))
	for _, b := range sorted {
		title := strings.TrimPrefix(b.Title, bookmarkTitlePrefix)
		if b.IsUserAdded {
			title = bookmarkTitlePrefix + title
		}

		pb := pdfcpu.Bookmark{
			Title:    title,
			PageFrom: b.PageNo,
			Bold:     b.Bold,
			Italic:   b.Italic,
		}
		if len(b.Children) > 0 {
			pb.Kids = convertBookmarksRecursive(b.Children)
		}
		result = append(result, pb)
	}
	return result
}
