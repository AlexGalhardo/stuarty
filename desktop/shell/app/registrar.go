package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/stuarty/desktop/service"
)

// autostartID names the registration of the shell on macOS (launch agent) and Linux (autostart entry).
const autostartID = "stuarty-desktop"

// newRegistrar is the registration of the start at sign-in: the Run key of the user on Windows (written by
// desktop/service, which the tests exercise on an in-memory key), and the autostart of the framework
// elsewhere, where epic 43 verifies it.
func newRegistrar(app *application.App) service.Registrar {
	if r := service.SystemRegistrar(); r != nil {
		return r
	}

	return wailsRegistrar{app: app}
}

// wailsRegistrar registers the shell with the autostart of the framework, for the signed-in user only.
type wailsRegistrar struct{ app *application.App }

func (w wailsRegistrar) Enable(args []string) error {
	return w.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: autostartID, Arguments: args})
}

func (w wailsRegistrar) Disable() error { return w.app.Autostart.Disable() }

func (w wailsRegistrar) Enabled() (bool, error) { return w.app.Autostart.IsEnabled() }
