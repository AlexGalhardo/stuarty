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

// DefaultImageCacheBytes is the memory the pictures and the prepared layers of an ImageCache may hold: eight
// pictures of a 4,000 by 4,000 pixel document (SDD-021 section 6.1). The 30 prepared layers of the reference
// document are about 260 MiB and the pictures an edit needs three of them, so the 384 MiB it had left one layer
// to prepare again at every edit.
const DefaultImageCacheBytes = 512 << 20

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

	// last is the latest render of each document shape: what the next edit starts from (SDD-021 section 6.3).
	last     map[lastKey]*lastFrame
	windows  int
	sessions map[lastKey]*windowSession
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

	return &ImageCache{
		limit: limit, order: list.New(), entries: map[cacheKey]*list.Element{}, last: map[lastKey]*lastFrame{},
		sessions: map[lastKey]*windowSession{},
	}
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

	// Windows is the renders that drew only the window of an edit over the picture before (section 6.3).
	Windows int
}

// Stats reports the state of the cache.
func (c *ImageCache) Stats() ImageCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return ImageCacheStats{
		Pictures: len(c.entries) - c.prepared, Bytes: c.used - int64(c.preparedBytes), Hits: c.hits, Misses: c.misses,
		PreparedLayers: c.prepared, PreparedBytes: int64(c.preparedBytes),
		PreparedHits: c.preparedHits, PreparedMisses: c.preparedMisses, Windows: c.windows,
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

// keptPictures is how many pictures of the canvas stay ahead of the prepared layers when the cache is full: the
// picture below the top layer and the whole picture, which an edit of the top layer and a new layer start from.
const keptPictures = 2

// evict drops entries until the cache is within its limit. The caller holds the lock.
func (c *ImageCache) evict() {
	for c.used > c.limit && c.order.Len() > 0 {
		last := c.victim()
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

// victim is the entry to drop: the least recently used picture of the canvas past the newest keptPictures, and
// otherwise the least recently used entry. A picture costs one draw over the prepared layers, and a prepared
// layer costs its looks again. An edit of the top layer touches no prepared layer below it, so by age alone the
// pictures of those edits would push out the very layers the next edit needs.
func (c *ImageCache) victim() *list.Element {
	var older *list.Element

	pictures := 0

	for el := c.order.Front(); el != nil; el = el.Next() {
		if e, ok := el.Value.(*cacheEntry); ok && e.img != nil {
			pictures++

			if pictures > keptPictures {
				older = el
			}
		}
	}

	if older != nil {
		return older
	}

	return c.order.Back()
}

// getPrepared gives the prepared layer kept under the key of the scope. The block is read only.
func (c *ImageCache) getPrepared(scope string, digest [sha256.Size]byte) (renderhost.Prepared, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if p, ok := c.findPrepared(scope, digest); ok {
		c.preparedHits++

		return p, true
	}

	c.preparedMisses++

	return renderhost.Prepared{}, false
}

// peekPrepared is getPrepared for a second look at a key that was already counted as a miss.
func (c *ImageCache) peekPrepared(scope string, digest [sha256.Size]byte) (renderhost.Prepared, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.findPrepared(scope, digest)
}

// findPrepared finds a prepared layer and marks it as recently used. The caller holds the lock.
func (c *ImageCache) findPrepared(scope string, digest [sha256.Size]byte) (renderhost.Prepared, bool) {
	if el, ok := c.entries[cacheKey{scope, digest}]; ok {
		if e, ok := el.Value.(*cacheEntry); ok && e.img == nil {
			c.order.MoveToFront(el)

			return e.pic, true
		}
	}

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
	// layer is the digest of each plan layer alone, and shape the digest of the canvas and of the item ids in
	// order: two renders of one shape differ only in the content of some layers (SDD-021 section 6.3).
	layer [][sha256.Size]byte
	shape [sha256.Size]byte
}

func newLayerChain(fp plan.FramePlan, live []plan.Layer) *layerChain {
	ch := &layerChain{
		digest: make([][sha256.Size]byte, len(live)+1), valid: make([]bool, len(live)+1),
		layer: make([][sha256.Size]byte, len(live)),
	}

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "stuarty image cache v1 %d %d %s %t", fp.Width, fp.Height, fp.Background, fp.Transparent)
	h.Sum(ch.digest[0][:0])

	position := layerPositions(live)

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

		ch.layer[i] = sha256.Sum256(body)
	}

	shape := sha256.New()
	_, _ = shape.Write(ch.digest[0][:])

	for i := range live {
		_, _ = fmt.Fprintf(shape, "|%d:%s", len(live[i].ItemID), live[i].ItemID)
	}

	shape.Sum(ch.shape[:0])

	for k := range ch.valid {
		ch.valid[k] = (k == 0 || ch.digest[k] != [sha256.Size]byte{}) && cutIsClean(live, position, k)
	}

	return ch
}

// layerPositions is the place of each layer in the plan, by item id.
func layerPositions(live []plan.Layer) map[string]int {
	position := make(map[string]int, len(live))
	for i := range live {
		position[live[i].ItemID] = i
	}

	return position
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

// lastKey names the latest render of a document shape: the owner, and the canvas and layer ids in order.
type lastKey struct {
	scope string
	shape [sha256.Size]byte
}

// lastFrame is what the cache remembers of the latest render of a shape: the digest of its whole picture (kept
// in the cache like any picture), the digest of each layer and the pixels each layer can touch. An edit of some
// layers is then a window of that picture drawn again (image_window.go).
type lastFrame struct {
	whole  [sha256.Size]byte
	layer  [][sha256.Size]byte
	bounds []layerBounds
}

// maxLastFrames bounds the shapes remembered; past it the table starts again, which costs one full render.
const maxLastFrames = 64

// lastFrameOf gives the latest render of the shape and its picture, when the picture is still in the cache. The
// picture is read only.
func (c *ImageCache) lastFrameOf(scope string, shape [sha256.Size]byte) (*lastFrame, *image.RGBA) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lf, ok := c.last[lastKey{scope, shape}]
	if !ok {
		return nil, nil
	}

	if el, ok := c.entries[cacheKey{scope, lf.whole}]; ok {
		if e, ok := el.Value.(*cacheEntry); ok && e.img != nil {
			c.order.MoveToFront(el)

			return lf, e.img
		}
	}

	return nil, nil
}

// setLastFrame remembers the render that just finished.
func (c *ImageCache) setLastFrame(scope string, shape [sha256.Size]byte, lf *lastFrame) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.last) >= maxLastFrames {
		clear(c.last)
	}

	c.last[lastKey{scope, shape}] = lf
}

// countWindow notes a render that drew the window of an edit.
func (c *ImageCache) countWindow() {
	c.mu.Lock()
	c.windows++
	c.mu.Unlock()
}
