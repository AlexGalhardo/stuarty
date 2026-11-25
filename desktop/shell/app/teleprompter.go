package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

// The teleprompter window (SDD-025 section 20.3 and 20.4, epic 66): the page /projects/<id>/teleprompter of the
// same web UI in a window of its own, always on top, with content protection on, which on Windows is
// SetWindowDisplayAffinity(WDA_EXCLUDEFROMCAPTURE) and on macOS a sharing type of none, so a screen capture does not
// hold it. The page names only the project; the address is built here (T-170).
const (
	teleprompterWidth  = 520
	teleprompterHeight = 720
)

// teleprompter holds the one window, so asking again for the same or another project shows it again instead of
// opening a second one.
type teleprompter struct {
	mu  sync.Mutex
	win *application.WebviewWindow
}

// open shows the window at the address, making it if there is none. It runs on the main thread.
func (t *teleprompter) open(app *application.App, port int, address string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.win != nil {
		t.win.SetURL(address)
		t.win.Show()
		t.win.Focus()

		return
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                    tr(shell.TextTeleprompter),
		Width:                    teleprompterWidth,
		Height:                   teleprompterHeight,
		MinWidth:                 320,
		MinHeight:                320,
		AlwaysOnTop:              true,
		ContentProtectionEnabled: true,
		BackgroundColour:         application.NewRGB(0x0b, 0x0b, 0x0b),
		URL:                      address,
	})
	win.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		t.mu.Lock()
		t.win = nil
		t.mu.Unlock()
	})

	// Wails holds every ExecJS until the page says "wails:runtime:ready", which the web UI never does (bridge.install
	// says it for the main window). Without this the navigation guard of this window would wait forever and never run.
	ready := func(*application.WindowEvent) { win.HandleMessage("wails:runtime:ready") }
	win.RegisterHook(events.Windows.WebViewNavigationCompleted, ready)
	win.RegisterHook(events.Mac.WebViewDidFinishNavigation, ready)
	win.RegisterHook(events.Linux.WindowLoadFinished, ready)
	guardNavigation(win, port, address)

	t.win = win
}

// close closes the window when there is one.
func (t *teleprompter) close() {
	t.mu.Lock()
	win := t.win
	t.win = nil
	t.mu.Unlock()

	if win != nil {
		win.Close()
	}
}
