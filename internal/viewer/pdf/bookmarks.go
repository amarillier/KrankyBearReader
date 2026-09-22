package pdf

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

	conf := model.NewDefaultConfiguration()

	f, err := os.Open(pdfPath)
	if err != nil {
		return fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	bookmarkList, err := api.Bookmarks(f, conf)
	if err != nil {
		log.Printf("[WARN] no bookmarks found or error reading bookmarks: %v", err)
		return nil
	}

	bm.bookmarks = convertPdfcpuBookmarks(bookmarkList)
	return nil
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
func (bm *BookmarkManager) SaveBookmarks(outputPath string) error {
	if bm.pdfPath == "" {
		return fmt.Errorf("no PDF loaded")
	}

	conf := model.NewDefaultConfiguration()

	if len(bm.bookmarks) == 0 {
		err := api.RemoveBookmarksFile(bm.pdfPath, outputPath, conf)
		if errors.Is(err, api.ErrNoOutlines) {
			if outputPath != "" && outputPath != bm.pdfPath {
				return copyFile(bm.pdfPath, outputPath)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to clear bookmarks: %w", err)
		}
		return nil
	}

	pdfcpuBookmarks := bm.convertToPdfcpuFormat()
	if err := api.AddBookmarksFile(bm.pdfPath, outputPath, pdfcpuBookmarks, true, conf); err != nil {
		return fmt.Errorf("failed to save bookmarks: %w", err)
	}
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
