package main

import "github.com/wailsapp/wails/v3/pkg/application"

// reloadPage reloads the page the window is at. win.Reload() navigates to the
// first address again, the one-time launch URL, which the server refuses the
// second time.
func reloadPage(w application.Window) { w.ExecJS("location.reload()") }

// windowKeyBindings gives the window the browser shortcuts a person expects:
// reload (F5, Ctrl+F5), page zoom (Ctrl+=, Ctrl++, Ctrl+-, Ctrl+0) and full
// screen (F11), print (Ctrl+P). F12 opens the developer tools only when devtools is set, for
// debugging and QA builds (the -devtools flag).
func windowKeyBindings(devtools bool) map[string]func(application.Window) {
	keys := map[string]func(application.Window){
		"F11":       func(w application.Window) { w.ToggleFullscreen() },
		"F5":        reloadPage,
		"ctrl+F5":   reloadPage,
		"ctrl+=":    func(w application.Window) { w.ZoomIn() },
		"ctrl+plus": func(w application.Window) { w.ZoomIn() },
		"ctrl+-":    func(w application.Window) { w.ZoomOut() },
		"ctrl+0":    func(w application.Window) { w.ZoomReset() },
		"ctrl+p":    func(w application.Window) { _ = w.Print() },
	}
	if devtools {
		keys["F12"] = func(w application.Window) { w.OpenDevTools() }
	}
	return keys
}
