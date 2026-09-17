package pdf

import (
	"container/list"
	"fmt"
	"image"
	"sync"
)

// pageCache is a real LRU cache of rendered pages, keyed by page+zoom.
// Replaces pdfviewer's original "clear the whole map when full" eviction
// (fine for a single-document app; wasteful once several PDF tabs can be
// open at once and each wants its own bounded cache).
type pageCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List // front = most recently used
}

type cacheEntry struct {
	key string
	img image.Image
}

func newPageCache(capacity int) *pageCache {
	return &pageCache{
		capacity: capacity,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

func cacheKey(page int, zoom float64) string {
	return fmt.Sprintf("%d:%.2f", page, zoom)
}

func (c *pageCache) Get(key string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).img, true
}

func (c *pageCache) Put(key string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).img = img
		c.order.MoveToFront(el)
		return
	}

	el := c.order.PushFront(&cacheEntry{key: key, img: img})
	c.items[key] = el

	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}

// Clear empties the cache (used when it's no longer worth keeping, e.g. the
// document is closing).
func (c *pageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.order.Init()
}
