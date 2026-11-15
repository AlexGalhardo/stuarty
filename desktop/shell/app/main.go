// Command app is the desktop shell (ADR-0023): one native window on the local
// server that it starts, with a native menu. It is a module of its own so the
// server binary and the root go.mod never pull Wails or its webview toolkits.
// Everything that does not need a window lives in desktop/shell.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

const (
	// Size of SDD-012 section 21.3 items 1 and 3: the minimum inner size and the
	// dark background of the editor before the page paints.
	minWidth, minHeight         = 1280, 640
	defaultWidth, defaultHeight = 1600, 900
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "vukem-video-desktop:", err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("binary", defaultBinary(), "path of the vukem-video server binary")
	flag.Parse()

	app := application.New(application.Options{
		Name:        "Vukem Video",
		Description: "Vukem Video desktop",
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})

	ws, err := workspace.Resolve(os.Getenv)
	if err != nil {
		return failStartup(app, err)
	}
	srv, err := shell.Start(context.Background(), shell.Config{Binary: *binary, Workspace: ws})
	if err != nil {
		return failStartup(app, err)
	}
	// Stop runs on quit, on a window close and on a normal return.
	defer srv.Stop()
	app.OnShutdown(srv.Stop)

	home := srv.URL() + "/projects"
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Vukem Video",
		Width:            defaultWidth,
		Height:           defaultHeight,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		BackgroundColour: application.NewRGB(0x0b, 0x0b, 0x0b),
		URL:              home,
	})
	guardNavigation(win, srv.Port, home)
	app.Menu.SetApplicationMenu(buildMenu(app, win, ws))

	// The child ending on its own is a crash: tell the person, then leave.
	go func() {
		<-srv.Done()
		application.InvokeSync(func() {
			app.Dialog.Error().SetTitle("Vukem Video").SetMessage("The local server stopped unexpectedly.").Show()
			app.Quit()
		})
	}()

	return app.Run()
}

// guardNavigation sends the window back home when a navigation ends outside
// the allowed origin. Wails v3.1.0 offers no veto before the navigation.
func guardNavigation(win *application.WebviewWindow, port int, home string) {
	script := shell.GuardScript(port, home)
	guard := func(*application.WindowEvent) { win.ExecJS(script) }
	win.RegisterHook(events.Windows.WebViewNavigationCompleted, guard)
	win.RegisterHook(events.Mac.WebViewDidFinishNavigation, guard)
	win.RegisterHook(events.Linux.WindowLoadFinished, guard)
}

// failStartup shows the message of a startup failure in a native dialog.
func failStartup(app *application.App, err error) error {
	msg := err.Error()
	if se, ok := errors.AsType[*shell.StartupError](err); ok {
		msg = se.Message
	}
	app.Dialog.Error().SetTitle("Vukem Video cannot start").SetMessage(msg).Show()
	return err
}

// defaultBinary is the server next to the shell executable.
func defaultBinary() string {
	name := "vukem-video"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	return filepath.Join(filepath.Dir(exe), name)
}
