package main

import (
	"context"
	_ "embed"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/AlexGalhardo/stuarty/desktop/service"
	"github.com/AlexGalhardo/stuarty/desktop/shell"
)

//go:embed tray.ico
var trayICO []byte

//go:embed tray.png
var trayPNG []byte

// askTimeout bounds how long closing the window waits for the server to count the scheduled posts.
const askTimeout = 1500 * time.Millisecond

// background carries out the background mode of SDD-012 section 21.3 item 7. The decisions are the pure
// functions of desktop/shell (DecideClose, QuitPrompt, StartHidden); this type only calls the framework.
type background struct {
	app *application.App
	win *application.WebviewWindow
	srv *shell.Server
	// dir is the folder of background.json.
	dir string
	// reg is the registration of the start at sign-in.
	reg service.Registrar

	// quitting is set before every quit of the application, so closing a window then never hides it.
	quitting atomic.Bool

	mu        sync.Mutex
	settings  service.Settings
	scheduled int
	known     bool
	paused    bool
	tray      *application.SystemTray
	pause     *application.MenuItem
}

func newBackground(app *application.App, srv *shell.Server, dir string, s service.Settings) *background {
	return &background{app: app, srv: srv, dir: dir, settings: s, reg: newRegistrar(app)}
}

// Reconcile makes the settings agree with the registration of the system at the start: a person who removed
// the entry by hand has start at sign-in off here too.
func (b *background) Reconcile() {
	b.mu.Lock()
	b.settings = service.Reconcile(b.settings, b.reg)
	s := b.settings
	b.mu.Unlock()

	_ = service.Save(b.dir, s)
}

// SetSettings carries out the choice of the person: it registers or removes the start at sign-in, stores
// the settings, and returns what holds now. When the system refuses the registration the option is off and
// the error says why.
func (b *background) SetSettings(s service.Settings) (service.Settings, error) {
	applied, err := service.Apply(s, b.reg)

	b.mu.Lock()
	b.settings = applied
	b.mu.Unlock()

	if saveErr := service.Save(b.dir, applied); saveErr != nil && err == nil {
		err = saveErr
	}

	return applied, err
}

// Settings is the current choice of the person.
func (b *background) Settings() service.Settings {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.settings
}

// refresh asks the server for the scheduled posts and the pause, and remembers the answer; on a failure
// the last answer stays.
func (b *background) refresh(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	st, err := b.srv.Scheduler(ctx, shell.SchedulerStatusAction)
	if err != nil {
		return
	}

	b.mu.Lock()
	b.scheduled, b.known, b.paused = st.Scheduled, true, st.Paused
	b.mu.Unlock()
}

func (b *background) state() shell.BackgroundState {
	b.mu.Lock()
	defer b.mu.Unlock()

	return shell.BackgroundState{
		KeepRunning: b.settings.KeepRunning, Scheduled: b.scheduled, Known: b.known, Paused: b.paused,
		Quitting: b.quitting.Load(),
	}
}

// install hooks the close of the window.
func (b *background) install(win *application.WebviewWindow) {
	b.win = win
	win.RegisterHook(events.Common.WindowClosing, b.onClosing)
}

func (b *background) onClosing(e *application.WindowEvent) {
	if b.quitting.Load() || !b.Settings().KeepRunning {
		return
	}

	b.refresh(askTimeout)

	if shell.DecideClose(b.state()) != shell.CloseHide {
		return
	}

	e.Cancel()
	b.win.Hide()
	b.ensureTray()
}

// showWindow brings the window back, from the tray or from a second launch.
func (b *background) showWindow() {
	b.win.Show()
	b.win.Restore()
	b.win.Focus()
	b.hideTray()
}

// quit ends the application; nothing hides a window from now on.
func (b *background) quit() {
	b.quitting.Store(true)
	b.app.Quit()
}

func (b *background) ensureTray() {
	// The framework is never called with the lock held: a call to the main thread that waits for it while the
	// main thread waits for the lock would never end. The tray is made only from the main thread.
	b.mu.Lock()
	t, paused := b.tray, b.paused
	b.mu.Unlock()

	if t != nil {
		t.Show()

		return
	}

	icon := trayICO
	if runtime.GOOS != "windows" {
		icon = trayPNG
	}

	menu := b.app.NewMenu()
	menu.Add(tr(shell.TextTrayOpen)).OnClick(func(*application.Context) { b.showWindow() })
	pause := menu.AddCheckbox(tr(shell.TextTrayPause), paused)
	pause.OnClick(func(ctx *application.Context) { go b.setPaused(ctx.ClickedMenuItem().Checked()) })
	menu.AddSeparator()
	menu.Add(tr(shell.TextQuit)).OnClick(func(*application.Context) { go b.trayQuit() })

	tray := b.app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip(tr(shell.TextTrayTooltip))
	tray.SetMenu(menu)
	tray.OnClick(func() { b.showWindow() })

	b.mu.Lock()
	b.tray, b.pause = tray, pause
	b.mu.Unlock()
}

func (b *background) hideTray() {
	b.mu.Lock()
	t := b.tray
	b.mu.Unlock()

	if t != nil {
		t.Hide()
	}
}

// setPaused pauses or resumes publishing from the tray menu; when the server refuses, the check mark goes
// back to what the server holds and the person is told.
func (b *background) setPaused(pause bool) {
	action := shell.SchedulerResume
	if pause {
		action = shell.SchedulerPause
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st, err := b.srv.Scheduler(ctx, action)

	b.mu.Lock()
	if err == nil {
		b.paused, b.scheduled, b.known = st.Paused, st.Scheduled, true
	}
	held, item := b.paused, b.pause
	b.mu.Unlock()

	application.InvokeSync(func() {
		if item != nil {
			item.SetChecked(held)
		}

		if err != nil {
			b.app.Dialog.Error().SetTitle("Stuarty").SetMessage(tr(shell.TextCannotPause) + err.Error()).Show()
		}
	})
}

// trayQuit is Quit from the tray: it says how many scheduled posts will wait, and quits unless the person
// chooses to keep running.
func (b *background) trayQuit() {
	b.refresh(askTimeout)

	ask, n := shell.QuitPrompt(b.state().Scheduled, b.state().Known)
	if !ask {
		b.quit()

		return
	}

	application.InvokeSync(func() {
		d := b.app.Dialog.Question().SetTitle(tr(shell.TextQuitTitle)).
			SetMessage(shell.QuitWaitingMessage(shell.SystemLanguage(), n))
		quit := d.AddButton(tr(shell.TextQuitConfirm))
		quit.OnClick(func() { go b.quit() })
		keep := d.AddButton(tr(shell.TextKeepRunning))
		d.SetDefaultButton(keep)
		d.SetCancelButton(keep)
		d.Show()
	})
}
