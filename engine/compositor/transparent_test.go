package compositor

import (
	"bytes"
	"image"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// An empty transparent plan leaves every pixel at alpha 0. The same plan with the flag clear is the
// opaque background, which is what a project without the flag has always drawn.
func TestTransparentFillLeavesTheFrameClear(t *testing.T) {
	t.Parallel()

	c := &Compositor{}
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fp := plan.FramePlan{Width: 4, Height: 4, Background: "#000000", Transparent: true}
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dst.Pix, make([]byte, len(dst.Pix))) {
		t.Fatalf("transparent fill %v", dst.Pix[:4])
	}

	fp.Transparent = false
	fp.Background = "#202020"
	if err := c.Compose(dst, fp, 1); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{0x20, 0x20, 0x20, 255}, 16)
	if !bytes.Equal(dst.Pix, want) {
		t.Fatalf("opaque fill %v", dst.Pix[:4])
	}
}
