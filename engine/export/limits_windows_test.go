//go:build windows

package export

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"

	"github.com/AlexGalhardo/vukem/contrib/engine"
)

// A child killed by its Job Object limit surfaces from contrib/engine as ErrRun; the export turns it into E_EXPORT_FAILED.
func TestWindowsJobObjectLimitEndsTheExportFailed(t *testing.T) {
	err := wrapRun(fmt.Errorf("ffmpeg: %w", engine.ErrRun), ReasonEncoder, "The encoder failed.", "")

	ffErr, ok := errors.AsType[*ffmpeg.Error](err)
	if !ok || ffErr.Code != CodeExportFailed {
		t.Fatalf("wrapRun = %v, want an *ffmpeg.Error with code %s", err, CodeExportFailed)
	}
}
