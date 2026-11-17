package shell

import (
	"path/filepath"
	"strings"
)

// WebviewArgsEnv and WebviewDataEnv are the variables WebView2 documents for extra browser flags and
// for the profile folder. The Wails shell builds the browser environment itself, and that replaces what
// the variables say, so the shell reads them and hands the values on (VV-076).
const (
	WebviewArgsEnv = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	WebviewDataEnv = "WEBVIEW2_USER_DATA_FOLDER"
)

// WebviewArgs is the list of extra browser flags the person asked for, for example
// --remote-debugging-port=9223 to automate the window.
func WebviewArgs(getenv func(string) string) []string {
	return strings.Fields(getenv(WebviewArgsEnv))
}

// WebviewDataPath is the browser profile folder: the one the person named, otherwise a folder inside the
// workspace, so two workspaces do not share a profile (and a browser process) through the user's
// application data folder.
func WebviewDataPath(getenv func(string) string, workspace string) string {
	if dir := getenv(WebviewDataEnv); dir != "" {
		return dir
	}

	if workspace == "" {
		return ""
	}

	return filepath.Join(workspace, "webview")
}
