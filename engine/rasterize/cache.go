package rasterize

import (
	"container/list"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// DefaultCacheBytes is the default memory budget of a Cache.
const DefaultCacheBytes = 64 << 20

// Cache keeps rendered bitmaps in memory, keyed by plan.BitmapSpec.Key: the hash of
// content, style, size and rasterizer version (SDD-004 section 5.4). It evicts
// the least recently used bitmaps when the pixels it holds pass the budget.
// The service stores the same bitmaps on disk under the same key; this cache is
// the in-process layer for the export. It is safe for concurrent use.
type Cache struct {
	fonts  *FontSet
	budget int

	mu    sync.Mutex
	used  int
	order *list.List // front is the most recently used
	items map[string]*list.Element
}

type cacheEntry struct {
	key    string
	bitmap Bitmap
}

// NewCache returns a cache that renders with fonts and holds at most budget
// bytes of pixels (DefaultCacheBytes when budget is not positive).
func NewCache(fonts *FontSet, budget int) *Cache {
	if budget <= 0 {
		budget = DefaultCacheBytes
	}

	return &Cache{fonts: fonts, budget: budget, order: list.New(), items: map[string]*list.Element{}}
}

// Get returns the bitmap of the spec, rendering it on a miss. The returned
// bitmap shares its pixels with the cache: callers must not modify them.
func (c *Cache) Get(spec plan.BitmapSpec) (Bitmap, error) {
	key := spec.Key()

	c.mu.Lock()

	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)

		b := entryOf(el).bitmap
		c.mu.Unlock()

		return b, nil
	}

	c.mu.Unlock()

	b, err := Render(spec, c.fonts)
	if err != nil {
		return Bitmap{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Another goroutine may have rendered the same spec meanwhile: keep one.
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)

		return entryOf(el).bitmap, nil
	}

	c.items[key] = c.order.PushFront(&cacheEntry{key: key, bitmap: b})
	c.used += len(b.Pix)

	for c.used > c.budget && c.order.Len() > 1 {
		last := c.order.Back()
		e := entryOf(last)
		c.order.Remove(last)
		delete(c.items, e.key)
		c.used -= len(e.bitmap.Pix)
	}

	return b, nil
}

// entryOf returns the entry of a list element. The list holds only entries, so
// the assertion cannot fail.
func entryOf(el *list.Element) *cacheEntry {
	e, _ := el.Value.(*cacheEntry)

	return e
}

// Len reports how many bitmaps the cache holds.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.order.Len()
}
