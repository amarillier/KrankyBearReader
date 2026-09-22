package pdf

import "testing"

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
