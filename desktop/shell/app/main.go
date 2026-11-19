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
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
	"github.com/AlexGalhardo/stuarty/desktop/shell/webviewenv"
	"github.com/AlexGalhardo/stuarty/service/workspace"
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
		fmt.Fprintln(os.Stderr, "stuarty-desktop:", err)
		os.Exit(1)
	}
}

func run() error {
	binary := flag.String("binary", defaultBinary(), "path of the stuarty server binary")
	devtools := flag.Bool("devtools", false, "bind F12 to the developer tools (debugging and QA builds)")
	watch := flag.String(shell.WatchFlag[2:], "", "internal: run the watchdog of an update (marker file)")
	flag.Parse()

	// The watchdog is the previous version started by the new one's installer; it has no window.
	if *watch != "" {
		return shell.RunWatchdog(context.Background(), *watch)
	}

	// An update whose watchdog died leaves the previous version in a backup: put it back before anything else.
	if dir, err := shell.UpdateStateDir(); err == nil {
		_, _ = shell.RecoverPending(dir, version, time.Now)
	}

	br := &bridge{}
	ws, wsErr := workspace.Resolve(os.Getenv)
	app := application.New(application.Options{
		Name:              "Stuarty",
		Description:       "Stuarty desktop",
		RawMessageHandler: br.handle,
		Mac:               application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		Windows: application.WindowsOptions{
			WebviewUserDataPath:   shell.WebviewDataPath(webviewenv.Getenv, string(ws)),
			AdditionalBrowserArgs: shell.WebviewArgs(webviewenv.Getenv),
		},
	})

	if err := wsErr; err != nil {
		return failStartup(app, err)
	}
	srv, err := shell.Start(context.Background(), shell.Config{Binary: *binary, Workspace: ws})
	if err != nil {
		return failStartup(app, err)
	}
	// Stop runs on quit, on a window close and on a normal return.
	defer srv.Stop()
	app.OnShutdown(srv.Stop)

	// The first load is the one-time launch address: the server answers it with
	// the session cookie, so the window is signed in without seeing the token.
	home := srv.URL() + "/projects"
	launch, err := srv.LaunchURL(context.Background())
	if err != nil {
		return failStartup(app, err)
	}
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Stuarty",
		Width:            defaultWidth,
		Height:           defaultHeight,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		BackgroundColour: application.NewRGB(0x0b, 0x0b, 0x0b),
		EnableFileDrop:   true,
		KeyBindings:      windowKeyBindings(*devtools),
		URL:              launch,
	})
	guardNavigation(win, srv.Port, home)
	br.app = app
	br.port.Store(int64(srv.Port))
	br.win.Store(win)
	br.install(win)
	// macOS shows the application menu; Windows and Linux show a menu per window,
	// and without win.SetMenu the window has no menu bar.
	up := newUpdater(app)
	app.OnShutdown(up.launcher.Close)
	menu := buildMenu(app, win, ws, up)
	app.Menu.SetApplicationMenu(menu)
	win.SetMenu(menu)
	// A dialog shown before the loop runs is dropped (VV-006), so the launcher starts with the loop.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go confirmStart()
		up.start()
	})

	// The child ending on its own is a crash: tell the person, then leave.
	go func() {
		<-srv.Done()
		application.InvokeSync(func() {
			app.Dialog.Error().SetTitle("Stuarty").SetMessage(tr(shell.TextServerStopped)).Show()
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
	title := tr(shell.TextCannotStart)
	// A dialog shown before the loop runs is dropped (Wails: "MessageDialog.Show was called before the
	// application started"), so the person saw nothing (VV-006). Run the loop just to show it, then leave.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		app.Dialog.Error().SetTitle(title).SetMessage(msg).Show()
		app.Quit()
	})
	_ = app.Run()
	return err
}

// defaultBinary is the server next to the shell executable.
func defaultBinary() string {
	name := "stuarty"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	return filepath.Join(filepath.Dir(exe), name)
}

// confirmStart tells the watchdog of an update that this version started (SDD-009 section 10.5 item 7).
func confirmStart() {
	if dir, err := shell.UpdateStateDir(); err == nil {
		_ = shell.ConfirmStart(dir, version)
	}
}
