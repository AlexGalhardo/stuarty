//go:build race

package export

import (
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
)

// raceEnabled is true when the tests run with the race detector, which multiplies the memory of the
// process and slows it by an order of magnitude: the measurements of NFR-006 and NFR-007 are not taken.
const raceEnabled = true

// The race detector slows the Go compositor that feeds FFmpeg about tenfold, so a 4 s scene on a
// 4-CPU runner outlives the 120 s + 6 x duration export deadline; the tests give it four times that.
func init() {
	exportDeadline = func(d time.Duration) time.Duration { return 4 * ffmpeg.ExportDeadline(d) }
}
