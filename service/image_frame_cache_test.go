package service

import (
	"bytes"
	"context"
	"testing"
)

// The cache of the frames ImageFrame answered (SDD-021 section 6.1): the same content at the same width draws
// nothing, never across users, never past the access the caller still has, and within its memory limit.

// frameDoc makes an image of two layers for the caller and gives its id.
func frameDoc(ctx context.Context, t *testing.T, s *svc) string {
	t.Helper()

	c, err := s.ImageCreate(ctx, ImageCreateRequest{Width: 200, Height: 100, Background: "#102030"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops(
			"layer.add_fill", `{"color": "#335577", "id": "lyr_aaaaaaaaaa"}`,
			"layer.add_shape", `{"shape": "ellipse", "w": 60, "h": 40, "fill": "#FF8800", "id": "lyr_bbbbbbbbbb"}`,
		),
	}); err != nil {
		t.Fatal(err)
	}

	return c.ImageID
}

func frameOf(ctx context.Context, t *testing.T, s *svc, id string, width int) *ImageFrameResult {
	t.Helper()

	f, err := s.ImageFrame(ctx, ImageFrameRequest{ImageID: id, Width: width})
	if err != nil {
		t.Fatal(err)
	}

	return f
}

func TestImageFrameAnswersTheSameRevisionAndWidthFromItsCache(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:person")
	id := frameDoc(ctx, t, s)

	first := frameOf(ctx, t, s, id, 100)

	if st := s.frameCache.stats(); st.Hits != 0 || st.Misses != 1 || st.Frames != 1 {
		t.Fatalf("the first frame: %+v", st)
	}

	drawn := s.imageFrames.Stats()

	again := frameOf(ctx, t, s, id, 100)

	if st := s.frameCache.stats(); st.Hits != 1 || st.Misses != 1 {
		t.Fatalf("the same revision and width again must be a hit: %+v", st)
	}

	if s.imageFrames.Stats() != drawn {
		t.Fatalf("a frame from the cache drew: before %+v, after %+v", drawn, s.imageFrames.Stats())
	}

	if !bytes.Equal(again.Image, first.Image) || again.Revision != first.Revision || again.Width != 100 ||
		again.Height != first.Height {
		t.Fatalf("the answer from the cache is another picture: %+v and %+v", first, again)
	}

	// what a caller does to the answer is not what the next one gets
	again.Image[0] ^= 0xFF

	if third := frameOf(ctx, t, s, id, 100); !bytes.Equal(third.Image, first.Image) {
		t.Fatal("the cache handed out the slice a caller could write to")
	}

	// another width is another frame
	frameOf(ctx, t, s, id, 50)

	if st := s.frameCache.stats(); st.Misses != 2 || st.Frames != 2 {
		t.Fatalf("another width: %+v", st)
	}
}

func TestImageFrameMissesAfterAnEditAndAfterAnUndoToOtherContent(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:person")
	id := frameDoc(ctx, t, s)

	before := frameOf(ctx, t, s, id, 100)

	e, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: id, BaseRevision: before.Revision,
		Operations: ops("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "opacity": 30}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	hits := s.frameCache.stats().Hits

	edited := frameOf(ctx, t, s, id, 100)

	if edited.Revision != e.Revision || s.frameCache.stats().Hits != hits || bytes.Equal(edited.Image, before.Image) {
		t.Fatalf("a frame after an edit came from the cache of the revision before it: %+v", s.frameCache.stats())
	}

	// the undo is a revision of its own with the content of an earlier one, which the cache has: the content is
	// what names a frame, never the number of the revision
	if _, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	undone := frameOf(ctx, t, s, id, 100)

	if undone.Revision <= e.Revision || s.frameCache.stats().Hits != hits+1 ||
		!bytes.Equal(undone.Image, before.Image) {
		t.Fatalf(
			"an undo to content that was drawn must give that picture: rev %d, %+v",
			undone.Revision,
			s.frameCache.stats(),
		)
	}

	// an undo to content nobody drew is a miss
	if _, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: id, BaseRevision: undone.Revision,
		Operations: ops("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "opacity": 60}`),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(ctx, ImageEditRequest{
		ImageID: id, BaseRevision: undone.Revision + 1,
		Operations: ops("layer.set", `{"layer_id": "lyr_bbbbbbbbbb", "opacity": 90}`),
	}); err != nil {
		t.Fatal(err)
	}

	frameOf(ctx, t, s, id, 100)

	hits, misses := s.frameCache.stats().Hits, s.frameCache.stats().Misses

	if _, err := s.ImageUndo(ctx, ImageStepRequest{ImageID: id}); err != nil {
		t.Fatal(err)
	}

	frameOf(ctx, t, s, id, 100)

	if st := s.frameCache.stats(); st.Hits != hits || st.Misses != misses+1 {
		t.Fatalf("an undo to content that was never drawn must draw: %+v", st)
	}
}

func TestImageFrameCacheIsNotSharedAcrossUsers(t *testing.T) {
	s, _ := newImageService(t)
	alice, bob := networkUser("alice"), networkUser("bob")
	mine, theirs := frameDoc(alice, t, s), frameDoc(bob, t, s)

	a := frameOf(alice, t, s, mine, 100)
	frameOf(alice, t, s, mine, 100)

	if st := s.frameCache.stats(); st.Hits != 1 || st.Misses != 1 {
		t.Fatalf("alice: %+v", st)
	}

	// the same content, the same width: bob's first frame is drawn, never taken from alice's
	b := frameOf(bob, t, s, theirs, 100)

	if st := s.frameCache.stats(); st.Hits != 1 || st.Misses != 2 || st.Frames != 2 {
		t.Fatalf("bob took the frame of alice: %+v", st)
	}

	if !bytes.Equal(a.Image, b.Image) {
		t.Fatal("the same document drew two pictures")
	}

	// the key is the owner as well as the content: bob asking for the document of alice is not found at all
	if _, err := s.ImageFrame(bob, ImageFrameRequest{ImageID: mine, Width: 100}); err == nil {
		t.Fatal("bob read the frame of a document of alice")
	}
}

// TestImageFrameCacheRefusesAnAssetTheCallerLost: a hit still asks for the file of every image the document
// draws, so what the caller may no longer use is refused although a picture of it is cached.
func TestImageFrameCacheRefusesAnAssetTheCallerLost(t *testing.T) {
	s, _ := newImageService(t)
	holdAsset(s, "alice")

	alice := networkUser("alice")

	c, err := s.ImageCreate(alice, ImageCreateRequest{Width: 64, Height: 48})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ImageEdit(alice, ImageEditRequest{
		ImageID: c.ImageID, BaseRevision: 1,
		Operations: ops("layer.add_image", `{"asset": "`+imgHash+`", "id": "lyr_aaaaaaaaaa"}`),
	}); err != nil {
		t.Fatal(err)
	}

	frameOf(alice, t, s, c.ImageID, 32)
	frameOf(alice, t, s, c.ImageID, 32)

	if s.frameCache.stats().Hits != 1 {
		t.Fatalf("the frame was not cached: %+v", s.frameCache.stats())
	}

	// the project that held the file is gone
	s.store.(*memStore).assets.projects["prj_alice"] = nil

	if f, err := s.ImageFrame(alice, ImageFrameRequest{ImageID: c.ImageID, Width: 32}); err == nil {
		t.Fatalf("a cached frame of a file the caller lost was served: %+v", f)
	}
}

func TestImageFrameCacheIsBoundedAndDropsTheLeastRecentlyUsed(t *testing.T) {
	c := newImageFrameCache(100)
	key := func(n int) frameKey { return frameKey{digest: "d", width: n} }
	body := func(n int) []byte { return bytes.Repeat([]byte{byte(n)}, 40) }

	c.put(key(1), body(1))
	c.put(key(2), body(2))

	if _, ok := c.get(key(1)); !ok { // 1 is now the most recent
		t.Fatal("a frame inside the limit was not kept")
	}

	c.put(key(3), body(3)) // 120 bytes: the least recent, 2, goes

	if _, ok := c.get(key(2)); ok {
		t.Fatal("the least recently used frame stayed")
	}

	for _, n := range []int{1, 3} {
		if got, ok := c.get(key(n)); !ok || !bytes.Equal(got, body(n)) {
			t.Fatalf("frame %d is gone or changed", n)
		}
	}

	if st := c.stats(); st.Bytes != 80 || st.Frames != 2 {
		t.Fatalf("after the eviction: %+v", st)
	}

	// a frame larger than the whole limit is not kept, and does not push the others out
	c.put(key(4), bytes.Repeat([]byte{4}, 101))

	if _, ok := c.get(key(4)); ok {
		t.Fatal("a frame larger than the limit was kept")
	}

	if st := c.stats(); st.Bytes != 80 || st.Frames != 2 {
		t.Fatalf("a frame over the limit changed what is held: %+v", st)
	}

	// the same key again replaces the frame and the count of bytes follows
	c.put(key(1), bytes.Repeat([]byte{9}, 10))

	if st := c.stats(); st.Bytes != 50 || st.Frames != 2 {
		t.Fatalf("a frame replaced: %+v", st)
	}
}

// TestImageFrameCacheStaysWithinItsLimitInTheService: with a limit smaller than a frame nothing is kept, and the
// answer is still the picture.
func TestImageFrameCacheStaysWithinItsLimitInTheService(t *testing.T) {
	s, _ := newImageService(t)
	s.frameCache = newImageFrameCache(16)
	ctx := asActor("local:person")
	id := frameDoc(ctx, t, s)

	a := frameOf(ctx, t, s, id, 100)
	b := frameOf(ctx, t, s, id, 100)

	if st := s.frameCache.stats(); st.Hits != 0 || st.Bytes != 0 || st.Frames != 0 {
		t.Fatalf("a frame larger than the limit was kept: %+v", st)
	}

	if !bytes.Equal(a.Image, b.Image) {
		t.Fatal("two frames of the same revision differ")
	}
}

// The frames are content made readable by the data key: the lock forgets them, and a frame drawn before the lock is
// not kept after it (SDD-007 T-134).
func TestImageFrameCacheIsForgottenWhenTheStudioLocks(t *testing.T) {
	s, _ := newImageService(t)
	ctx := asActor("local:person")
	id := frameDoc(ctx, t, s)

	frameOf(ctx, t, s, id, 100)

	if st := s.frameCache.stats(); st.Frames != 1 {
		t.Fatalf("the frame was not kept: %+v", st)
	}

	s.dropContent()

	if st := s.frameCache.stats(); st.Frames != 0 || st.Bytes != 0 {
		t.Fatalf("a frame outlived the lock: %+v", st)
	}

	before := s.frameCache.now()
	s.dropContent()
	s.frameCache.putIn(before, frameKey{digest: "d", width: 1}, []byte{1})

	if st := s.frameCache.stats(); st.Frames != 0 {
		t.Fatalf("a frame drawn before the lock was kept after it: %+v", st)
	}
}
