package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// buildMenu is the native menu: Quit, Reload, About with the version and Open
// workspace folder (ROADMAP P3-DSK-002). The editor menus of SDD-012 section
// 21.3 item 4 (onMenuAction) are a later task; View has the full screen toggle
// that the bridge also offers.
func buildMenu(app *application.App, win *application.WebviewWindow, ws workspace.Dir, up *updater) *application.Menu {
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
	// win.Reload() navigates to the first address again, the one-time launch URL, which
	// the server refuses the second time; the page reloads the address it is at.
	view.Add("Reload").SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) {
		win.ExecJS("location.reload()")
	})
	// F11 is a window key binding (main.go), not a menu accelerator: Windows hides the
	// menu bar in full screen and a menu accelerator would leave no way out.
	view.Add("Toggle full screen\tF11").OnClick(func(*application.Context) { win.ToggleFullscreen() })

	help := menu.AddSubmenu("Help")
	help.Add("Check for updates").OnClick(func(*application.Context) { up.checkNow() })
	help.AddCheckbox("Check for updates at start-up", up.settings.UpdateCheck).
		OnClick(func(ctx *application.Context) { up.setCheckOnStartup(ctx.ClickedMenuItem().Checked()) })
	help.AddSeparator()
	help.Add("About").OnClick(func(*application.Context) {
		app.Dialog.Info().SetTitle("About Vukem Video").SetMessage("Vukem Video " + version).Show()
	})
	return menu
}
