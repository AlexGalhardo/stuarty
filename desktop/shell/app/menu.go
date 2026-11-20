package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// editorItems adds the items of the group `name` of shell.EditorMenus to menu.
// A click tells the page the action id, which runs the same action as the
// control of the page (SDD-012 section 21.3 item 4). No item registers an
// accelerator: Mod+Z, Mod+Shift+Z and Mod+A are only shown, because the key
// reaches the webview, where the editor decides between a field's own undo or
// select all and the project's.
func editorItems(menu *application.Menu, win *application.WebviewWindow, name string) {
	for _, group := range shell.EditorMenus {
		if group.Name != name {
			continue
		}
		for _, item := range group.Items {
			if item.Action == "" {
				menu.AddSeparator()
				continue
			}
			label := tr(item.Text)
			if item.Hint != "" {
				label += "\t" + shell.DisplayHint(item.Hint, runtime.GOOS)
			}
			action := item.Action
			menu.Add(label).OnClick(func(*application.Context) {
				if script, ok := shell.MenuActionScript(action); ok {
					win.ExecJS(script)
				}
			})
		}
	}
}

// buildMenu is the native menu: the File, Edit, Clip, Timeline, View and Help
// menus of SDD-012 section 6.4, whose editor items call the page, and the shell's
// own items (Open workspace folder, Quit, Reload, full screen, updates, About with
// the version).
func buildMenu(
	app *application.App, win *application.WebviewWindow, ws workspace.Dir, up *updater, quit func(),
) *application.Menu {
	menu := app.NewMenu()

	file := menu.AddSubmenu(tr(shell.TextFile))
	editorItems(file, win, "file")
	file.AddSeparator()
	file.Add(tr(shell.TextOpenWorkspace)).OnClick(func(*application.Context) {
		if err := app.Browser.OpenFile(string(ws)); err != nil {
			app.Dialog.Error().SetTitle("Stuarty").SetMessage(tr(shell.TextCannotOpen) + err.Error()).Show()
		}
	})
	file.AddSeparator()
	file.Add(tr(shell.TextQuit)).SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { quit() })

	editorItems(menu.AddSubmenu(tr(shell.TextMenuEdit)), win, "edit")
	editorItems(menu.AddSubmenu(tr(shell.TextMenuClip)), win, "clip")
	editorItems(menu.AddSubmenu(tr(shell.TextMenuTimeline)), win, "timeline")

	view := menu.AddSubmenu(tr(shell.TextView))
	editorItems(view, win, "view")
	view.AddSeparator()
	// win.Reload() navigates to the first address again, the one-time launch URL, which
	// the server refuses the second time; the page reloads the address it is at.
	view.Add(tr(shell.TextReload)).SetAccelerator("CmdOrCtrl+R").OnClick(func(*application.Context) { reloadPage(win) })
	// F11 is a window key binding (main.go), not a menu accelerator: Windows hides the
	// menu bar in full screen and a menu accelerator would leave no way out.
	view.Add(tr(shell.TextFullScreen) + "\tF11").OnClick(func(*application.Context) { win.ToggleFullscreen() })

	help := menu.AddSubmenu(tr(shell.TextHelp))
	editorItems(help, win, "help")
	help.AddSeparator()
	help.Add(tr(shell.TextCheckUpdates)).OnClick(func(*application.Context) { up.checkNow() })
	auto := help.AddCheckbox(tr(shell.TextCheckAtStart), up.launcher.AutoCheck())
	auto.OnClick(func(ctx *application.Context) { up.setAutoCheck(ctx.ClickedMenuItem().Checked()) })
	// A build without a feed has nothing to check: the item says so by being disabled (VV-073).
	auto.SetEnabled(up.launcher.CanUpdate())
	help.AddSeparator()
	help.Add(tr(shell.TextAbout)).OnClick(func(*application.Context) {
		app.Dialog.Info().SetTitle(tr(shell.TextAboutTitle)).SetMessage("Stuarty " + version).Show()
	})
	return menu
}
