package pdf

import (
	"math"
	"path/filepath"
	"testing"
)

func TestRenameBookmark_UpdatesTitleOnly(t *testing.T) {
	bm := NewBookmarkManager()
	b := bm.AddBookmark("Old Title", 5, 0.25, true)

	if err := bm.RenameBookmark(b, "New Title"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Title != "New Title" {
		t.Errorf("Title = %q, want %q", b.Title, "New Title")
	}
	if b.PageNo != 5 || b.YOffset != 0.25 {
		t.Errorf("page/position changed unexpectedly: PageNo=%d YOffset=%v", b.PageNo, b.YOffset)
	}
}

func TestRenameBookmark_RejectsBlankTitle(t *testing.T) {
	bm := NewBookmarkManager()
	b := bm.AddBookmark("Keep Me", 1, 0, false)

	if err := bm.RenameBookmark(b, "   "); err == nil {
		t.Fatal("expected an error for a blank title")
	}
	if b.Title != "Keep Me" {
		t.Errorf("Title changed despite rejected rename: %q", b.Title)
	}
}

// TestSaveBookmarks_RegionPositionRoundTrips builds a real single-page PDF,
// adds a region bookmark (nonzero YOffset) and a plain page bookmark
// (YOffset 0) via the same AddBookmark path the UI uses, saves to the same
// file, then re-loads it with a fresh BookmarkManager — proving the real
// /XYZ destination writeRegionDestinations/setRegionDestination write
// actually reaches disk and regionYOffsetForKey recovers the same fraction
// back, not just that the save doesn't error. The 200x200 MediaBox from
// writeMinimalPDF makes the expected top coordinate easy to hand-verify:
// yOffset 0.4 on a 200pt-tall page should land /XYZ's top at 120pt.
func TestSaveBookmarks_RegionPositionRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	writeMinimalPDF(t, path)

	bm := NewBookmarkManager()
	if err := bm.LoadBookmarks(path); err != nil {
		t.Fatalf("initial LoadBookmarks: %v", err)
	}

	const wantFrac = 0.4
	bm.AddBookmark("Region Mark", 1, wantFrac, true)
	bm.AddBookmark("Page Mark", 1, 0, true)

	if err := bm.SaveBookmarks(path); err != nil {
		t.Fatalf("SaveBookmarks: %v", err)
	}

	reloaded := NewBookmarkManager()
	if err := reloaded.LoadBookmarks(path); err != nil {
		t.Fatalf("reloaded LoadBookmarks: %v", err)
	}

	got := reloaded.GetBookmarks()
	if len(got) != 2 {
		t.Fatalf("expected 2 bookmarks after reload, got %d", len(got))
	}

	byTitle := map[string]*Bookmark{}
	for _, b := range got {
		byTitle[b.Title] = b
	}

	region, ok := byTitle["Region Mark"]
	if !ok {
		t.Fatalf("Region Mark not found after reload: %+v", got)
	}
	if diff := math.Abs(float64(region.YOffset) - wantFrac); diff > 0.01 {
		t.Errorf("Region Mark YOffset = %v, want ~%v", region.YOffset, wantFrac)
	}

	page, ok := byTitle["Page Mark"]
	if !ok {
		t.Fatalf("Page Mark not found after reload: %+v", got)
	}
	if page.YOffset != 0 {
		t.Errorf("Page Mark YOffset = %v, want 0 (plain page bookmark must not gain a position)", page.YOffset)
	}
}
