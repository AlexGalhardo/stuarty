// Package webviewenv keeps the WebView2 environment variables as the process was started with.
//
// The Wails WebView2 loader overwrites them with empty values in its package initialiser, before main
// runs, so that the browser ignores the registry and the environment (VV-144: reading them in run()
// found nothing, and --remote-debugging-port never reached the browser). Go initialises packages by
// import path among the ones that are ready, so this package, which imports only os and sorts before
// github.com/wailsapp, runs first. The test in desktop/shell/app checks that this still holds.
package webviewenv

import "os"

// ArgsName and DataName are the variables WebView2 documents for extra browser flags and for the
// profile folder.
const (
	ArgsName = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	DataName = "WEBVIEW2_USER_DATA_FOLDER"
)

var startup = map[string]string{
	ArgsName: os.Getenv(ArgsName),
	DataName: os.Getenv(DataName),
}

// Getenv is os.Getenv, except that the two WebView2 variables answer with their value at process start.
func Getenv(key string) string {
	if v, ok := startup[key]; ok {
		return v
	}

	return os.Getenv(key)
}
