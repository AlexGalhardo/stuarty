package service

import (
	"container/list"
	"sync"
)

// defaultImageFrameBytes is the memory the encoded frames of the editor may hold (SDD-021 section 6.1).
const defaultImageFrameBytes = 64 << 20

// frameKey names a frame by who asked, what was drawn and how wide: the owner (the user in network mode, empty in
// local mode), the canonical SHA-256 of the document of the revision (`doc_sha256`, which names its content and,
// through the asset hashes in it, the content of every file it draws) and the width asked for (0 is the canvas).
type frameKey struct {
	owner  string
	digest string
	width  int
}

type frameEntry struct {
	key  frameKey
	body []byte
}

// imageFrameCache keeps the PNG that ImageFrame answered, so that asking again for the same revision at the same
// width draws and encodes nothing (SDD-021 section 6.1). It holds bytes in memory only, never writes to disk, drops
// the least recently used frame first, keeps no frame larger than its limit and is never read across owners.
type imageFrameCache struct {
	mu      sync.Mutex
	limit   int64
	used    int64
	order   *list.List // front is the most recently used
	entries map[frameKey]*list.Element
	hits    int
	misses  int
	// epoch moves at every reset, so a frame drawn before the studio locked is not kept after it (SDD-007 T-134).
	epoch uint64
}

func newImageFrameCache(limit int64) *imageFrameCache {
	if limit <= 0 {
		limit = defaultImageFrameBytes
	}

	return &imageFrameCache{limit: limit, order: list.New(), entries: map[frameKey]*list.Element{}}
}

// get is the PNG kept for the key. The slice is shared: nobody writes to it.
func (c *imageFrameCache) get(k frameKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[k]
	if !ok {
		c.misses++

		return nil, false
	}

	c.hits++
	c.order.MoveToFront(el)

	frame, _ := el.Value.(*frameEntry)

	return frame.body, true
}

// now is the epoch a render starts in; putIn keeps its frame only while the cache is still in that epoch.
func (c *imageFrameCache) now() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.epoch
}

// reset forgets every frame. It runs when the studio locks (dropContent).
func (c *imageFrameCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.epoch++
	c.used = 0
	c.order.Init()
	c.entries = map[frameKey]*list.Element{}
}

// putIn keeps a frame drawn in the given epoch, unless the cache was reset since.
func (c *imageFrameCache) putIn(epoch uint64, k frameKey, body []byte) {
	c.mu.Lock()
	stale := epoch != c.epoch
	c.mu.Unlock()

	if stale {
		return
	}

	c.put(k, body)
}

// put keeps a frame, then drops the least recently used ones until the cache is within its limit.
func (c *imageFrameCache) put(k frameKey, body []byte) {
	size := int64(len(body))
	if size > c.limit {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[k]; ok {
		old, _ := el.Value.(*frameEntry)
		c.used += size - int64(len(old.body))
		old.body = body
		c.order.MoveToFront(el)
	} else {
		c.entries[k] = c.order.PushFront(&frameEntry{key: k, body: body})
		c.used += size
	}

	for c.used > c.limit {
		last := c.order.Back()
		if last == nil {
			break
		}

		old, _ := last.Value.(*frameEntry)
		c.used -= int64(len(old.body))
		delete(c.entries, old.key)
		c.order.Remove(last)
	}
}

// imageFrameStats is what the cache holds and how often it answered.
type imageFrameStats struct {
	Frames int
	Bytes  int64
	Hits   int
	Misses int
}

func (c *imageFrameCache) stats() imageFrameStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return imageFrameStats{Frames: len(c.entries), Bytes: c.used, Hits: c.hits, Misses: c.misses}
}
