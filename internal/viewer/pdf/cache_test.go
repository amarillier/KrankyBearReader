package pdf

import (
	"image"
	"testing"
)

func TestPageCache_EvictsLeastRecentlyUsed(t *testing.T) {
	c := newPageCache(2)
	img := func() image.Image { return image.NewRGBA(image.Rect(0, 0, 1, 1)) }

	c.Put("a", img())
	c.Put("b", img())
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected 'a' to still be cached")
	}
	// "a" is now most-recently-used; adding "c" should evict "b", not "a".
	c.Put("c", img())

	if _, ok := c.Get("b"); ok {
		t.Error("expected 'b' to have been evicted as least-recently-used")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("expected 'a' to remain cached")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("expected 'c' to be cached")
	}
}

func TestPageCache_PutExistingKeyRefreshesRecency(t *testing.T) {
	c := newPageCache(2)
	img := func() image.Image { return image.NewRGBA(image.Rect(0, 0, 1, 1)) }

	c.Put("a", img())
	c.Put("b", img())
	c.Put("a", img()) // re-insert "a"; should now be most recently used
	c.Put("c", img()) // should evict "b", not "a"

	if _, ok := c.Get("b"); ok {
		t.Error("expected 'b' to have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("expected 'a' to remain cached after being refreshed")
	}
}

func TestCacheKey_DistinctForDifferentPagesOrZoom(t *testing.T) {
	if cacheKey(1, 1.0) == cacheKey(2, 1.0) {
		t.Error("expected different pages to produce different keys")
	}
	if cacheKey(1, 1.0) == cacheKey(1, 1.5) {
		t.Error("expected different zoom levels to produce different keys")
	}
}
