package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// buildMenu is the native menu: Quit, Reload, About with the version and Open
// workspace folder (ROADMAP P3-DSK-002). The editor menus of SDD-012 section
// 21.3 item 4 come with the UI bridge (P3-DSK-003).
func buildMenu(app *application.App, win *application.WebviewWindow, ws workspace.Dir) *application.Menu {
	menu := app.NewMenu()

	file := menu.AddSubmenu("File")
	file.Add("Open workspace folder").OnClick(func(*application.Context) {
		if err := app.Browser.OpenFile(string(ws)); err != nil {
			app.Dialog.Error().SetTitle("Vukem Video").SetMessage("Cannot open the folder: " + err.Error()).Show()
		}
	})
	file.AddSeparator()
	file.Add("Quit").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { app.Quit() })

	view := menu.AddSubmenu("View")
	view.Add("Reload").SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { win.Reload() })

	help := menu.AddSubmenu("Help")
	help.Add("About").OnClick(func(*application.Context) {
		app.Dialog.Info().SetTitle("About Vukem Video").SetMessage("Vukem Video " + version).Show()
	})
	return menu
}
