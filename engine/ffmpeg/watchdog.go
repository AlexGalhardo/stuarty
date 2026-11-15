package ffmpeg

import (
	"context"
	"errors"
	"os"
	"time"
)

// ErrOutputTooLarge means the output file passed the size cap and the run was killed.
var ErrOutputTooLarge = errors.New("ffmpeg: output over the size cap")

// ReasonCancelTimeout is details.reason of a job whose process could not be killed (SDD-007 6.6):
// the run directory is kept for doctor.
const ReasonCancelTimeout = "cancel_timeout"

// cancelMargin is the time, after CancelGrace, that the wait for a killed run gets before the job is failed.
const cancelMargin = 2 * time.Second

// watchSize polls the output file every interval and calls kill once it is larger than limit bytes.
// A file that does not exist yet is size zero. The overshoot is up to one interval of writing
// (SDD-007 6.5; Linux adds RLIMIT_FSIZE).
func watchSize(ctx context.Context, path string, limit int64, interval time.Duration, kill func()) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if fi, err := os.Stat(path); err == nil && fi.Size() > limit {
				kill()

				return
			}
		}
	}
}
