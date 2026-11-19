package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// InstanceID is the unique id of the single-instance lock of a workspace: stable for one workspace
// folder and different for two, so two workspaces run side by side and one never twice. The folder is
// cleaned and, on Windows and macOS where the file system ignores case, lowered, so two spellings of
// one folder are one instance.
func InstanceID(workspace string) string {
	p := filepath.Clean(workspace)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}

	sum := sha256.Sum256([]byte(p))

	return "stuarty-desktop-" + hex.EncodeToString(sum[:8])
}

// SecondLaunch is what the first instance does when a second one is started and exits.
type SecondLaunch int

const (
	// SecondFocus shows the window (it may be hidden) and brings it to the front.
	SecondFocus SecondLaunch = iota
	// SecondIgnore does nothing: the second launch was the start at sign-in.
	SecondIgnore
)

// DecideSecondLaunch reads the arguments of the second launch.
func DecideSecondLaunch(args []string) SecondLaunch {
	if slices.Contains(args, BackgroundFlag) {
		return SecondIgnore
	}

	return SecondFocus
}
