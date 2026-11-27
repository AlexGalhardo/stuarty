//go:build !windows

package ffmpeg

import "os/exec"

// HideWindow is a no-op outside Windows.
func HideWindow(*exec.Cmd) {}
