package export

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// A freeze frame (SDD-004 section 3.12.1) is drawn from the one source frame at source_in, decoded once,
// at every frame of the item, and has no sound.
func TestFreezeFrameShowsOneSourceFrameAndIsSilent(t *testing.T) {
	doc := loadScene(t, "two-clips-cut")

	const first = "itm_c1c1c1c1c1"

	it := doc.Items[first]
	it.Freeze, it.AudioMuted = true, true
	doc.Items[first] = it

	moving := loadScene(t, "two-clips-cut")
	// The same frame the freeze shows, from the clip that moves: the first frame of the item.
	want := renderDoc(t, moving, "freeze-source", 1, []int{0})[0]

	frames := []int{0, 1, 2, 17, 31, 59}
	got := renderDoc(t, doc, "freeze", 1, frames)

	for i, f := range frames {
		if !bytes.Equal(got[i], want) {
			t.Errorf("frame %d of the freeze frame is not the source frame at source_in", f)
		}
	}

	if off := it.SourceOffset(59); off != 0 {
		t.Errorf("SourceOffset(59) = %d, a freeze frame always shows offset 0", off)
	}

	// The moving clip is not still: the same frames differ, so the test above proves something.
	other := renderDoc(t, moving, "freeze-moving", 1, []int{59})[0]
	if bytes.Equal(other, want) {
		t.Fatal("the reference clip does not move, the freeze test proves nothing")
	}

	for _, src := range plan.Audio(doc, 0, 120) {
		if src.ItemID == first {
			t.Errorf("the freeze frame has a sound source: %+v", src)
		}
	}
}
