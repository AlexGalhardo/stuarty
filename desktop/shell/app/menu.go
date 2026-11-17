package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// buildMenu is the native menu: Quit, Reload, About with the version and Open
// workspace folder (ROADMAP P3-DSK-002). The editor menus of SDD-012 section
// 21.3 item 4 (onMenuAction) are a later task; View has the full screen toggle
// that the bridge also offers.
func buildMenu(app *application.App, win *application.WebviewWindow, ws workspace.Dir, up *updater) *application.Menu {
	menu := app.NewMenu()

	file := menu.AddSubmenu(tr(shell.TextFile))
	file.Add(tr(shell.TextOpenWorkspace)).OnClick(func(*application.Context) {
		if err := app.Browser.OpenFile(string(ws)); err != nil {
			app.Dialog.Error().SetTitle("Vukem Video").SetMessage(tr(shell.TextCannotOpen) + err.Error()).Show()
		}
	})
	file.AddSeparator()
	file.Add(tr(shell.TextQuit)).SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { app.Quit() })

	view := menu.AddSubmenu(tr(shell.TextView))
	// win.Reload() navigates to the first address again, the one-time launch URL, which
	// the server refuses the second time; the page reloads the address it is at.
	view.Add(tr(shell.TextReload)).SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { reloadPage(win) })
	// F11 is a window key binding (main.go), not a menu accelerator: Windows hides the
	// menu bar in full screen and a menu accelerator would leave no way out.
	view.Add(tr(shell.TextFullScreen) + "\tF11").OnClick(func(*application.Context) { win.ToggleFullscreen() })

	help := menu.AddSubmenu(tr(shell.TextHelp))
	help.Add(tr(shell.TextCheckUpdates)).OnClick(func(*application.Context) { up.checkNow() })
	auto := help.AddCheckbox(tr(shell.TextCheckAtStart), up.launcher.AutoCheck())
	auto.OnClick(func(ctx *application.Context) { up.setAutoCheck(ctx.ClickedMenuItem().Checked()) })
	// A build without a feed has nothing to check: the item says so by being disabled (VV-073).
	auto.SetEnabled(up.launcher.CanUpdate())
	help.AddSeparator()
	help.Add(tr(shell.TextAbout)).OnClick(func(*application.Context) {
		app.Dialog.Info().SetTitle(tr(shell.TextAboutTitle)).SetMessage("Vukem Video " + version).Show()
	})
	return menu
}
