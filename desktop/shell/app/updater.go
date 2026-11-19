package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
	"github.com/AlexGalhardo/stuarty/desktop/update"
)

// updater joins the launcher (desktop/shell) to the native dialogs and menu (SDD-009 section 10.5). The
// update client exists only when the build carries a feed URL and a signing key; without them the
// launcher never asks and every menu action shows the not-configured message instead of failing.
type updater struct {
	app          *application.App
	launcher     *shell.Launcher
	ui           *nativeUI
	settingsPath string
	ctx          context.Context
}

// newUpdater reads the settings and builds the client. A damaged settings file means "not asked yet",
// which opens no connection.
func newUpdater(app *application.App) *updater {
	u := &updater{app: app, ui: &nativeUI{app: app}, ctx: context.Background()}

	var settings shell.Settings

	path, err := shell.SettingsPath()
	if err == nil {
		u.settingsPath = path
		settings, _ = shell.LoadSettings(path)
	}

	staging, err := os.UserCacheDir()
	if err != nil {
		staging = os.TempDir()
	}

	cfg, buildErr := update.FromBuild(version, filepath.Join(staging, "stuarty", "updates"),
		settings.AutoCheck(), shell.InstallerFor(runtime.GOOS, app.Quit))

	lc := shell.LauncherConfig{Settings: settings, UI: u.ui, Current: version, Language: shell.SystemLanguage()}

	if dir, err := shell.UpdateStateDir(); err == nil {
		if failed, ok := shell.LoadFailed(dir); ok {
			lc.RolledBack, lc.Notify = failed.Version, !failed.Notified
			lc.Notified = func() { _ = shell.MarkFailedNotified(dir) }
		}
	}

	if buildErr == nil {
		cfg.Progress = func(done, total int64) { u.ui.SetProgress(done, total) }

		var client *update.Client

		if client, buildErr = update.New(cfg); buildErr == nil {
			lc.Client = client
		}
	}

	lc.BuildErr = buildErr

	if u.settingsPath != "" {
		lc.Save = func(s shell.Settings) error { return s.Save(u.settingsPath) }
	}

	u.launcher = shell.NewLauncher(lc)

	return u
}

// start is called once the event loop runs: it asks the first-start question or starts the schedule.
func (u *updater) start() { go u.launcher.Start(u.ctx) }

// checkNow is the menu action: one check, then a dialog with the result.
func (u *updater) checkNow() { u.launcher.CheckNow(u.ctx) }

// setAutoCheck stores the setting update.check from the menu; it takes effect at once.
func (u *updater) setAutoCheck(on bool) {
	go func() {
		if err := u.launcher.SetAutoCheck(u.ctx, on); err != nil {
			u.ui.Fail(tr(shell.TextCannotSave) + err.Error())
		}
	}()
}

// nativeUI is the shell.LauncherUI on Wails dialogs. Dialogs are shown on the main thread.
type nativeUI struct {
	app      *application.App
	progress *application.WebviewWindow
}

func (n *nativeUI) AskConsent(answer func(yes bool)) {
	application.InvokeSync(func() {
		d := n.app.Dialog.Question().SetTitle(tr(shell.TextConsentTitle)).SetMessage(tr(shell.TextConsentBody))
		yes := d.AddButton(tr(shell.TextYes))
		yes.OnClick(func() { answer(true) })
		no := d.AddButton(tr(shell.TextNo))
		no.OnClick(func() { answer(false) })
		d.SetDefaultButton(yes)
		d.Show()
	})
}

func (n *nativeUI) OfferUpdate(offer *update.Offer, current string, decide func(install bool)) {
	application.InvokeSync(func() {
		d := n.app.Dialog.Question().SetTitle(tr(shell.TextCheckUpdates)).
			SetMessage(shell.OfferMessage(shell.SystemLanguage(), offer, current))
		install := d.AddButton(tr(shell.TextInstall))
		install.OnClick(func() { decide(true) })
		later := d.AddButton(tr(shell.TextNotNow))
		later.OnClick(func() { decide(false) })
		d.SetDefaultButton(install)
		d.SetCancelButton(later)
		d.Show()
	})
}

func (n *nativeUI) Inform(message string) {
	application.InvokeSync(func() {
		n.app.Dialog.Info().SetTitle(tr(shell.TextCheckUpdates)).SetMessage(message).Show()
	})
}

func (n *nativeUI) Undone(version string) {
	application.InvokeSync(func() {
		n.app.Dialog.Info().SetTitle("Stuarty").SetMessage(tr(shell.TextUndone) + version).Show()
	})
}

func (n *nativeUI) Fail(message string) {
	application.InvokeSync(func() {
		n.app.Dialog.Error().SetTitle(tr(shell.TextCheckUpdates)).SetMessage(message).Show()
	})
}

// progressPage is the whole page of the progress window; the version is MAJOR.MINOR.PATCH, so it needs
// no escaping.
const progressPage = `<!doctype html><meta charset="utf-8"><body style="margin:0;padding:24px;` +
	`font:14px system-ui,sans-serif;background:#0b0b0b;color:#eee"><div>%s</div>` +
	`<progress id="p" max="100" value="0" style="width:100%%;margin-top:16px"></progress>` +
	`<div id="n" style="margin-top:8px">0%%</div>`

func (n *nativeUI) ShowProgress(version string) {
	application.InvokeSync(func() {
		n.progress = n.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title:            tr(shell.TextDownloading) + version,
			Width:            420,
			Height:           150,
			DisableResize:    true,
			BackgroundColour: application.NewRGB(0x0b, 0x0b, 0x0b),
			HTML:             fmt.Sprintf(progressPage, tr(shell.TextDownloading)+version),
		})
		n.progress.Center()
	})
}

func (n *nativeUI) SetProgress(done, total int64) {
	if total <= 0 {
		return
	}

	percent := done * 100 / total

	application.InvokeAsync(func() {
		if n.progress == nil {
			return
		}

		n.progress.ExecJS(fmt.Sprintf(
			"document.getElementById('p').value=%d;document.getElementById('n').textContent='%d%%'", percent, percent))
	})
}

func (n *nativeUI) HideProgress() {
	application.InvokeSync(func() {
		if n.progress != nil {
			n.progress.Close()
			n.progress = nil
		}
	})
}
