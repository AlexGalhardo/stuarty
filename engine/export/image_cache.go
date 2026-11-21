package export

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/renderhost"
)

// DefaultImageCacheBytes is the memory the pictures of an ImageCache may hold: six pictures of a 4,000 by 4,000
// pixel document (SDD-021 section 6.1).
const DefaultImageCacheBytes = 384 << 20

// ImageCache keeps pictures that RenderImage composed, so that the render of an edited document starts from the
// picture of the layers that did not change instead of drawing them again (SDD-021 section 6.1). A picture is
// keyed by the owner (CacheScope, a user in network mode) and by a digest of everything that made it: the
// canvas and the content of each plan layer from the bottom up. The cache is bounded by the bytes of its
// pictures and drops the least recently used first. It is safe for concurrent use; the pictures it holds are
// never handed out for writing.
type ImageCache struct {
	mu      sync.Mutex
	limit   int64
	used    int64
	order   *list.List // front is the most recently used
	entries map[cacheKey]*list.Element
	hits    int
	misses  int

	prepared, preparedBytes int
	preparedHits            int
	preparedMisses          int
}

type cacheKey struct {
	scope  string
	digest [sha256.Size]byte
}

type cacheEntry struct {
	key cacheKey
	img *image.RGBA         // a picture of the canvas, or
	pic renderhost.Prepared // a prepared layer (SDD-021 section 6.2)
}

func (e *cacheEntry) size() int64 {
	if e.img != nil {
		return int64(len(e.img.Pix))
	}

	return int64(e.pic.Bytes())
}

// NewImageCache makes a cache that holds at most limit bytes of pixels; a limit of zero or less is the default.
func NewImageCache(limit int64) *ImageCache {
	if limit <= 0 {
		limit = DefaultImageCacheBytes
	}

	return &ImageCache{limit: limit, order: list.New(), entries: map[cacheKey]*list.Element{}}
}

// ImageCacheStats is what a cache holds and how often a render found its start in it. Pictures, Bytes, Hits
// and Misses are the pictures of the canvas; the prepared layers (SDD-021 section 6.2) are counted apart and
// share the limit.
type ImageCacheStats struct {
	Pictures int
	Bytes    int64
	Hits     int
	Misses   int

	PreparedLayers int
	PreparedBytes  int64
	PreparedHits   int
	PreparedMisses int
}

// Stats reports the state of the cache.
func (c *ImageCache) Stats() ImageCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ImageCacheStats{
		Pictures: len(c.entries) - c.prepared, Bytes: c.used - int64(c.preparedBytes), Hits: c.hits, Misses: c.misses,
		PreparedLayers: c.prepared, PreparedBytes: int64(c.preparedBytes),
		PreparedHits: c.preparedHits, PreparedMisses: c.preparedMisses,
	}
}

// nearest gives the picture of the most layers that the cache holds for this chain, and how many layers it has
// drawn; 0 and nil when none. The picture is read only.
func (c *ImageCache) nearest(scope string, ch *layerChain) (int, *image.RGBA) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k := len(ch.digest) - 1; k >= 1; k-- {
		if !ch.valid[k] {
			continue
		}

		el, ok := c.entries[cacheKey{scope, ch.digest[k]}]
		if !ok {
			continue
		}

		if e, ok := el.Value.(*cacheEntry); ok {
			c.order.MoveToFront(el)
			c.hits++

			return k, e.img
		}
	}

	c.misses++

	return 0, nil
}

// put keeps a picture, which the caller no longer writes. A picture larger than the whole cache is not kept.
func (c *ImageCache) put(scope string, digest [sha256.Size]byte, img *image.RGBA) {
	size := int64(len(img.Pix))

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey{scope, digest}
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)

		return
	}

	if size > c.limit {
		return
	}

	c.entries[key] = c.order.PushFront(&cacheEntry{key: key, img: img})
	c.used += size

	c.evict()
}

// evict drops the least recently used entries until the cache is within its limit. The caller holds the lock.
func (c *ImageCache) evict() {
	for c.used > c.limit && c.order.Len() > 0 {
		last := c.order.Back()
		c.order.Remove(last)

		if e, ok := last.Value.(*cacheEntry); ok {
			delete(c.entries, e.key)
			c.used -= e.size()

			if e.img == nil {
				c.prepared--
				c.preparedBytes -= e.pic.Bytes()
			}
		}
	}
}

// getPrepared gives the prepared layer kept under the key of the scope. The block is read only.
func (c *ImageCache) getPrepared(scope string, digest [sha256.Size]byte) (renderhost.Prepared, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[cacheKey{scope, digest}]; ok {
		if e, ok := el.Value.(*cacheEntry); ok && e.img == nil {
			c.order.MoveToFront(el)
			c.preparedHits++

			return e.pic, true
		}
	}

	c.preparedMisses++

	return renderhost.Prepared{}, false
}

// putPrepared keeps a prepared layer. One larger than the whole cache is not kept.
func (c *ImageCache) putPrepared(scope string, digest [sha256.Size]byte, p renderhost.Prepared) {
	size := int64(p.Bytes())

	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey{scope, digest}
	if el, ok := c.entries[key]; ok {
		c.order.MoveToFront(el)

		return
	}

	if size > c.limit {
		return
	}

	c.entries[key] = c.order.PushFront(&cacheEntry{key: key, pic: p})
	c.used += size
	c.prepared++
	c.preparedBytes += p.Bytes()

	c.evict()
}

// layerChain is the digest of the first k layers of a plan, for every k, and whether the first k layers can be
// drawn on their own: no layer below the cut is clipped to a layer above it, and none above it to one below it
// (a clip needs its source in the same draw).
type layerChain struct {
	digest [][sha256.Size]byte
	valid  []bool
}

func newLayerChain(fp plan.FramePlan, live []plan.Layer) *layerChain {
	ch := &layerChain{digest: make([][sha256.Size]byte, len(live)+1), valid: make([]bool, len(live)+1)}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "stuarty image cache v1 %d %d %s %t", fp.Width, fp.Height, fp.Background, fp.Transparent)
	h.Sum(ch.digest[0][:0])

	position := make(map[string]int, len(live))
	for i := range live {
		position[live[i].ItemID] = i
	}

	for i := range live {
		body, err := json.Marshal(&live[i])
		if err != nil {
			// A layer that cannot be written cannot be told apart from another: nothing past it is cached.
			break
		}

		h := sha256.New()
		_, _ = h.Write(ch.digest[i][:])
		_, _ = h.Write(body)
		h.Sum(ch.digest[i+1][:0])
	}

	for k := range ch.valid {
		ch.valid[k] = (k == 0 || ch.digest[k] != [sha256.Size]byte{}) && cutIsClean(live, position, k)
	}

	return ch
}

// cutIsClean tells that no clip of a layer crosses the cut after the first k layers.
func cutIsClean(live []plan.Layer, position map[string]int, k int) bool {
	for i := range live {
		c := live[i].Clip
		if c == nil {
			continue
		}

		src, ok := position[c.Source]
		if !ok {
			continue // its source is not drawn at all: the layer is dropped, with or without a cut
		}

		if (i < k) != (src < k) {
			return false
		}
	}

	return true
}

func cloneRGBA(img *image.RGBA) *image.RGBA {
	out := *img
	out.Pix = append([]byte(nil), img.Pix...)

	return &out
}
