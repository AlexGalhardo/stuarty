package main

import (
	"context"
	_ "embed"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

//go:embed recording.ico
var recordingICO []byte

//go:embed recording.png
var recordingPNG []byte

// recordingIndicator is the system indicator of a capture (ADR-0051 item 4, SDD-025 section 19.4): a red tray icon
// with a tooltip, drawn while the server says a capture helper is alive. It is a tray of its own, not the one of
// the background mode, so it exists with the window open, hidden or closed, and it is driven by the host through
// shell.CaptureIndicator and by nothing the window or the page does.
type recordingIndicator struct {
	app *application.App

	mu   sync.Mutex
	tray *application.SystemTray
}

// start asks the server once a second until ctx ends.
func (r *recordingIndicator) start(ctx context.Context, srv *shell.Server) {
	ind := &shell.CaptureIndicator{Ask: srv.CaptureStatus, Show: r.show}
	go ind.Run(ctx, shell.CapturePollInterval)
}

// show draws the indicator on or off. The framework is called from the main thread only.
func (r *recordingIndicator) show(on bool) {
	application.InvokeSync(func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if !on {
			if r.tray != nil {
				r.tray.Hide()
			}

			return
		}

		if r.tray != nil {
			r.tray.Show()

			return
		}

		icon := recordingICO
		if runtime.GOOS != "windows" {
			icon = recordingPNG
		}

		tray := r.app.SystemTray.New()
		tray.SetIcon(icon)
		tray.SetTooltip(tr(shell.TextTrayRecording))
		r.tray = tray
	})
}
