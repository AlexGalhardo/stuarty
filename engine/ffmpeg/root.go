package ffmpeg

import (
	"errors"
	"os"
	"strconv"
)

// EnvAllowRoot is the variable that lets the runner start children as root. The service validates the
// same name at boot (SDD-007 4.5); the engine does not import the service, so the name is repeated here.
const EnvAllowRoot = "STUARTY_INSECURE_ALLOW_ROOT"

// ErrRefusedRoot means the process runs as uid 0 and STUARTY_INSECURE_ALLOW_ROOT is not true, so no
// child was started (SDD-007 T-16).
var ErrRefusedRoot = errors.New("ffmpeg: refusing to start a child as root")

// geteuid is the seam of the root check: tests replace it to simulate root on any system. Windows has no
// uid 0 (os.Geteuid returns -1), so the check never refuses there.
var geteuid = os.Geteuid

func checkNotRoot() error {
	if geteuid() != 0 {
		return nil
	}

	if allow, _ := strconv.ParseBool(os.Getenv(EnvAllowRoot)); allow {
		return nil
	}

	return &Error{
		Code:    CodeInternal,
		Reason:  "refused_root",
		Message: "refusing to start FFmpeg as root",
		Hint:    "run as an unprivileged user, or set " + EnvAllowRoot + "=true if you accept the risk",
		Err:     ErrRefusedRoot,
	}
}
