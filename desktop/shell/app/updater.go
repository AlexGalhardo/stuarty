package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
	"github.com/AlexGalhardo/vukem-video/desktop/update"
)

// updater joins the update client (desktop/update) to the native menu (SDD-009 section 10.5). The client
// exists only when the build carries a feed URL and a signing key; without them every menu action shows
// the not-configured message instead of failing.
type updater struct {
	app          *application.App
	client       *update.Client
	buildErr     error
	settings     shell.Settings
	settingsPath string
}

// newUpdater reads the settings and builds the client. A damaged settings file leaves the check off.
func newUpdater(app *application.App) *updater {
	u := &updater{app: app}

	path, err := shell.SettingsPath()
	if err == nil {
		u.settingsPath = path
		u.settings, _ = shell.LoadSettings(path)
	}

	staging, err := os.UserCacheDir()
	if err != nil {
		staging = os.TempDir()
	}

	cfg, err := update.FromBuild(version, filepath.Join(staging, "vukem-video", "updates"),
		u.settings.UpdateCheck, shell.InstallerFor(runtime.GOOS, app.Quit))
	if err == nil {
		u.client, err = update.New(cfg)
	}

	u.buildErr = err

	return u
}

// checker is nil, not a typed nil, when the build has no client.
func (u *updater) checker() shell.UpdateChecker {
	if u.client == nil {
		return nil
	}

	return u.client
}

// checkNow is the menu action: one check, then a dialog with the result.
func (u *updater) checkNow() {
	go func() {
		out := shell.CheckForUpdates(context.Background(), u.checker(), u.buildErr, version)
		application.InvokeSync(func() { u.show(out) })
	}()
}

// checkOnStartup makes the one check of the setting update.check. With the setting off, or without a
// client, it opens no connection. A failure at start-up is silent: the person did not ask for it.
func (u *updater) checkOnStartup() {
	if u.client == nil || !u.settings.UpdateCheck {
		return
	}

	go func() {
		offer, err := u.client.CheckOnStartup(context.Background())
		if err != nil {
			return
		}

		out := shell.CheckForUpdates(context.Background(), offerOnly{offer}, nil, version)
		application.InvokeSync(func() { u.show(out) })
	}()
}

// offerOnly replays the offer that CheckOnStartup already verified, so the same dialog is built.
type offerOnly struct{ offer *update.Offer }

func (o offerOnly) CheckNow(context.Context) (*update.Offer, error) { return o.offer, nil }

func (u *updater) show(out shell.UpdateOutcome) {
	if out.Offer == nil {
		u.app.Dialog.Info().SetTitle(tr(shell.TextCheckUpdates)).SetMessage(out.Message).Show()

		return
	}

	d := u.app.Dialog.Question().SetTitle(tr(shell.TextCheckUpdates)).SetMessage(out.Message)
	install := d.AddButton(tr(shell.TextInstall))
	install.OnClick(func() { u.apply(out.Offer) })
	later := d.AddButton(tr(shell.TextNotNow))
	d.SetDefaultButton(install)
	d.SetCancelButton(later)
	d.Show()
}

// apply downloads, verifies and installs the offer. On success the installer quits the application.
func (u *updater) apply(offer *update.Offer) {
	go func() {
		if err := u.client.Apply(context.Background(), offer); err != nil {
			application.InvokeSync(func() {
				u.app.Dialog.Error().SetTitle(tr(shell.TextCheckUpdates)).
					SetMessage(shell.InstallFailedMessage(err)).Show()
			})
		}
	}()
}

// setCheckOnStartup stores the setting update.check; it takes effect at the next launch.
func (u *updater) setCheckOnStartup(on bool) {
	u.settings.UpdateCheck = on

	if u.settingsPath == "" {
		return
	}

	if err := u.settings.Save(u.settingsPath); err != nil {
		application.InvokeSync(func() {
			u.app.Dialog.Error().SetTitle("Vukem Video").SetMessage(tr(shell.TextCannotSave) + err.Error()).Show()
		})
	}
}
