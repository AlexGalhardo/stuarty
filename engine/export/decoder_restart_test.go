package export

import (
	"testing"
)

// A decoder that is restarted over and over (every backward seek restarts it) must not share its
// channels with the goroutines of the process it replaced: under the race detector the old pipe closer
// read the fields that start wrote again.
func TestDecoderRestartsWithoutARace(t *testing.T) {
	setup(t)

	doc := loadScene(t, "clip-caption")
	end := endFrame(doc)
	var frames []int
	for i := range 12 {
		frames = append(frames, min(end-1, 4+i%3), 0)
	}

	renderDoc(t, doc, "restarts", 0, frames)
}
