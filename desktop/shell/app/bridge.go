package main

import (
	"context"
	"os/exec"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/vukem-video/desktop/shell"
)

// bridge answers the requests of window.vukemDesktop (SDD-012 section 13.1
// item 2). The window and the server port exist only after the application is
// created, so they are set later; until then every message is refused.
type bridge struct {
	app  *application.App
	win  atomic.Pointer[application.WebviewWindow]
	port atomic.Int64
}

// handle is the application's raw message handler. A message counts only when
// it comes from the main frame of the local server origin.
func (b *bridge) handle(_ application.Window, message string, origin *application.OriginInfo) {
	win := b.win.Load()
	if win == nil || origin == nil || !shell.Allowed(origin.Origin, int(b.port.Load())) {
		return
	}
	req, ok := shell.ParseBridgeMessage(message)
	if !ok {
		return
	}
	switch req.Op {
	case shell.OpPickFiles:
		go b.pickFiles(win, req.ID)
	case shell.OpPickTemplate:
		go b.pickTemplate(win, req.ID)
	case shell.OpShowInFolder:
		go b.showInFolder(win, req)
	}
}

// pickFiles shows the native dialog and gives the page the chosen absolute
// paths; the shell never reads the files.
func (b *bridge) pickFiles(win *application.WebviewWindow, id int64) {
	dialog := b.app.Dialog.OpenFile().
		AttachToWindow(win).
		SetTitle("Choose files").
		CanChooseFiles(true).
		CanChooseDirectories(false)
	for _, f := range shell.MediaFilters() {
		dialog.AddFilter(f.Name, f.Pattern)
	}
	paths, err := dialog.PromptForMultipleSelection()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	win.ExecJS(shell.ResolveScript(id, paths, msg))
}

// pickTemplate shows the native dialog for one template file and gives the page
// its absolute path (an empty list when the dialog is cancelled).
func (b *bridge) pickTemplate(win *application.WebviewWindow, id int64) {
	dialog := b.app.Dialog.OpenFile().
		AttachToWindow(win).
		SetTitle("Choose a template file").
		CanChooseFiles(true).
		CanChooseDirectories(false)
	for _, f := range shell.TemplateFilters() {
		dialog.AddFilter(f.Name, f.Pattern)
	}
	path, err := dialog.PromptForSingleSelection()
	msg := ""
	var paths []string
	if err != nil {
		msg = err.Error()
	} else if path != "" {
		paths = []string{path}
	}
	win.ExecJS(shell.ResolveScript(id, paths, msg))
}

// showInFolder opens the file manager on the exported file. The page names the
// path; the shell opens it only when it is an existing file, and answers with an
// empty list or the reason it could not.
func (b *bridge) showInFolder(win *application.WebviewWindow, req shell.BridgeRequest) {
	msg := ""
	if err := shell.CheckRevealPath(req.Path); err != nil {
		msg = err.Error()
	} else {
		name, args := shell.RevealCommand(runtime.GOOS, req.Path)
		// Explorer exits with 1 even when it succeeded, so only a failure to start counts.
		cmd := exec.CommandContext(context.Background(), name, args...)
		if err := cmd.Start(); err != nil {
			msg = err.Error()
		} else {
			go func() { _ = cmd.Wait() }()
		}
	}
	win.ExecJS(shell.ResolveScript(req.ID, nil, msg))
}

// install runs the bridge script after every navigation, once the webview has
// put window._wails.invoke on the page. Wails holds back every ExecJS until the
// page says "wails:runtime:ready", which only its own JavaScript runtime does and
// the editor does not load, so the shell says it itself; this also releases the
// navigation guard script, which waited for the same message.
func (b *bridge) install(win *application.WebviewWindow) {
	run := func(*application.WindowEvent) {
		win.HandleMessage("wails:runtime:ready")
		win.ExecJS(shell.BridgeScript)
	}
	win.RegisterHook(events.Windows.WebViewNavigationCompleted, run)
	win.RegisterHook(events.Mac.WebViewDidFinishNavigation, run)
	win.RegisterHook(events.Linux.WindowLoadFinished, run)
}
