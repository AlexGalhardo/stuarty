package main

import (
	"context"
	"os/exec"
	"runtime"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/stuarty/desktop/service"
	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

// bridge answers the requests of window.vukemDesktop (SDD-012 section 13.1
// item 2). The window and the server port exist only after the application is
// created, so they are set later; until then every message is refused.
type bridge struct {
	app  *application.App
	win  atomic.Pointer[application.WebviewWindow]
	port atomic.Int64
	bg   atomic.Pointer[background]
	// mediaRoot is the media folder of the workspace the shell started the server on: the only place a library
	// file may be revealed from (SDD-011 section 14.10). Empty until the workspace is known, and then every
	// reveal is refused.
	mediaRoot atomic.Pointer[string]
	// exportsRoot is the exports folder of the workspace: the only place an exported file may be revealed from.
	exportsRoot atomic.Pointer[string]
	// tele is the teleprompter window of epic 66, which the page opens and closes through the bridge.
	tele teleprompter
}

// backgroundAnswer is what the page gets for the background mode: the settings, and the reason the system
// refused the registration of the start at sign-in when it did.
type backgroundAnswer struct {
	shell.BackgroundSettings
	Error string `json:"error,omitempty"`
}

func (b *bridge) background(win *application.WebviewWindow, req shell.BridgeRequest) {
	bg := b.bg.Load()
	if bg == nil {
		win.ExecJS(shell.ResolveValueScript(req.ID, nil, "the background mode is not ready"))
		return
	}
	var answer backgroundAnswer
	if req.Op == shell.OpSetBackground {
		in := *req.Background
		applied, err := bg.SetSettings(service.Settings{
			KeepRunning: in.KeepRunning, StartAtSignIn: in.StartAtSignIn, Asked: in.Asked,
		})
		answer = backgroundAnswer{
			KeepRunning: applied.KeepRunning, StartAtSignIn: applied.StartAtSignIn, Asked: applied.Asked,
		}
		if err != nil {
			answer.Error = err.Error()
		}
	} else {
		s := bg.Settings()
		answer = backgroundAnswer{KeepRunning: s.KeepRunning, StartAtSignIn: s.StartAtSignIn, Asked: s.Asked}
	}
	win.ExecJS(shell.ResolveValueScript(req.ID, answer, ""))
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
	case shell.OpRevealAsset:
		go b.revealAsset(win, req)
	case shell.OpGetBackground, shell.OpSetBackground:
		go b.background(win, req)
	case shell.OpOpenTeleprompter, shell.OpCloseTeleprompter:
		go b.teleprompter(win, req)
	}
}

// teleprompter opens or closes the teleprompter window. The page names a project; the address is built here, on the
// origin of the server of this shell (T-170), and an id that is not a project opens nothing.
func (b *bridge) teleprompter(win *application.WebviewWindow, req shell.BridgeRequest) {
	msg := ""
	port := int(b.port.Load())
	address, ok := shell.TeleprompterURL(port, req.Project)

	switch {
	case req.Op == shell.OpCloseTeleprompter:
		application.InvokeSync(b.tele.close)
	case ok:
		application.InvokeSync(func() { b.tele.open(b.app, port, address) })
	default:
		msg = "not a project"
	}

	win.ExecJS(shell.ResolveScript(req.ID, nil, msg))
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

// showInFolder opens the file manager on the exported file. The page is not trusted:
// the shell checks by itself that the path is a regular file under the exports folder
// of its own workspace before starting anything, and answers with an empty list or
// the reason it could not.
func (b *bridge) showInFolder(win *application.WebviewWindow, req shell.BridgeRequest) {
	root := b.exportsRoot.Load()
	if root == nil || *root == "" {
		win.ExecJS(shell.ResolveScript(req.ID, nil, "the exports folder of the workspace is not known"))
		return
	}
	msg := ""
	if err := shell.CheckExportRevealUnder(*root, req.Path); err != nil {
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

// revealAsset opens the file manager on a stored library file. The path is the one AssetReveal gave the page, but the
// page is not trusted: the shell checks by itself that it is a regular file under the media folder of its own
// workspace before it starts anything, and answers with an empty list or the sentence the page shows.
func (b *bridge) revealAsset(win *application.WebviewWindow, req shell.BridgeRequest) {
	root := b.mediaRoot.Load()
	if root == nil || *root == "" {
		win.ExecJS(shell.ResolveScript(req.ID, nil, "the media folder of the workspace is not known"))
		return
	}
	r := shell.Revealer{
		GOOS: runtime.GOOS, Root: *root,
		Start: func(name string, args []string) error {
			cmd := exec.CommandContext(context.Background(), name, args...)
			if err := cmd.Start(); err != nil {
				return err
			}
			go func() { _ = cmd.Wait() }()
			return nil
		},
	}
	msg := ""
	if err := r.Reveal(req.Path); err != nil {
		msg = err.Error()
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
