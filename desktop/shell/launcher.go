package shell

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

// The schedule of the automatic checks (SDD-009 section 10.5, launcher flow item 2).
const (
	CheckInterval = 24 * time.Hour
	RetryInterval = time.Hour
)

// LauncherClient is the part of update.Client the launcher uses.
type LauncherClient interface {
	CheckNow(ctx context.Context) (*update.Offer, error)
	Apply(ctx context.Context, offer *update.Offer) error
}

// LauncherUI is the native side of the launcher. Every question is asked with a callback: the dialog
// is shown and the method returns at once, so no implementation has to block on the person.
type LauncherUI interface {
	// AskConsent shows "Check for updates automatically?" with Yes preselected. answer is called with
	// the choice; it is not called when the dialog is dismissed without one.
	AskConsent(answer func(yes bool))
	// OfferUpdate shows the version and its notes. decide is called with true for "Install and restart".
	OfferUpdate(offer *update.Offer, current string, decide func(install bool))
	// Inform shows the result of a check the person asked for.
	Inform(message string)
	// ShowProgress opens the progress window; SetProgress feeds it; HideProgress closes it.
	ShowProgress(version string)
	SetProgress(done, total int64)
	HideProgress()
	// Undone tells that the update to version was undone because it did not start.
	Undone(version string)
	// Fail shows why an accepted update was not installed.
	Fail(message string)
}

// LauncherConfig is everything the launcher depends on; a test fills it with fakes.
type LauncherConfig struct {
	Settings Settings
	// Save stores the settings; it may be nil when there is nowhere to store them.
	Save func(Settings) error
	// Client is nil when the build has no feed URL and key; BuildErr says why.
	Client   LauncherClient
	BuildErr error
	UI       LauncherUI
	Current  string
	// Language is the catalogue of the dialogs ("en" when empty).
	Language string
	// After is the clock: it returns a channel that fires after d. Default time.After.
	After func(d time.Duration) <-chan time.Time
	// RolledBack is the version of an update that was undone: the schedule never offers it again, only
	// the menu action does. Notify is true until the person was told, once, at a start.
	RolledBack string
	Notify     bool
	// Notified is called after the person was told; it records that.
	Notified func()
}

// Launcher keeps the editor up to date: the first-start consent, the schedule, the offer and the
// install. The person's answer is the only thing that opens a connection (D-039).
type Launcher struct {
	cfg LauncherConfig

	mu         sync.Mutex
	settings   Settings
	stopLoop   context.CancelFunc
	installing bool
	declined   map[string]bool
	status     string
	lastOffer  *update.Offer
	lastErr    string
	progress   int
	wg         sync.WaitGroup
}

// NewLauncher builds a launcher. It starts nothing and opens no connection.
func NewLauncher(cfg LauncherConfig) *Launcher {
	if cfg.After == nil {
		cfg.After = time.After
	}

	status := "idle"
	if cfg.Client == nil {
		status = "unconfigured"
	}

	l := &Launcher{cfg: cfg, settings: cfg.Settings, declined: map[string]bool{}, status: status}
	if cfg.RolledBack != "" {
		l.declined[cfg.RolledBack] = true
	}

	return l
}

// Start runs the first-start question when the person was not asked yet, and the schedule when the
// setting is on. Without a client it does nothing at all, and not asking is what keeps a build that
// cannot update from showing a question that means nothing.
func (l *Launcher) Start(ctx context.Context) {
	if l.cfg.Client == nil {
		return
	}

	if l.cfg.Notify && l.cfg.RolledBack != "" {
		l.cfg.UI.Undone(l.cfg.RolledBack)

		if l.cfg.Notified != nil {
			l.cfg.Notified()
		}
	}

	l.mu.Lock()
	asked, auto := l.settings.Asked(), l.settings.AutoCheck()
	l.mu.Unlock()

	switch {
	case !asked:
		l.cfg.UI.AskConsent(func(yes bool) {
			if err := l.SetAutoCheck(ctx, yes); err != nil {
				l.cfg.UI.Fail(err.Error())
			}
		})
	case auto:
		l.startLoop(ctx)
	}
}

// SetAutoCheck records the answer, saves it and starts or stops the schedule at once. The first check
// of a Yes happens immediately.
func (l *Launcher) SetAutoCheck(ctx context.Context, on bool) error {
	if on && l.cfg.Client == nil {
		return nil
	}

	l.mu.Lock()
	l.settings = l.settings.WithAutoCheck(on)
	next := l.settings
	l.mu.Unlock()

	if on {
		l.startLoop(ctx)
	} else {
		l.stop()
	}

	if l.cfg.Save == nil {
		return nil
	}

	return l.cfg.Save(next)
}

// CanUpdate tells whether this build has an update client. Without one there is nothing to check
// and the automatic check cannot be turned on.
func (l *Launcher) CanUpdate() bool { return l.cfg.Client != nil }

// AutoCheck is the current state of the setting.
func (l *Launcher) AutoCheck() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.settings.AutoCheck()
}

func (l *Launcher) startLoop(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.stopLoop != nil {
		return
	}

	loopCtx, cancel := context.WithCancel(ctx)
	l.stopLoop = cancel

	l.wg.Go(func() {
		for {
			delay := l.scheduledCheck(loopCtx)

			select {
			case <-loopCtx.Done():
				return
			case <-l.cfg.After(delay):
			}
		}
	})
}

func (l *Launcher) stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.stopLoop != nil {
		l.stopLoop()
		l.stopLoop = nil
	}
}

// Wait blocks until the schedule and any install goroutine ended; for tests and for shutdown.
func (l *Launcher) Wait() { l.wg.Wait() }

// Close stops the schedule.
func (l *Launcher) Close() { l.stop() }

// scheduledCheck is one automatic check. A network failure is silent and retried sooner than the
// interval; only a newer version the person has not declined opens a dialog. It returns the wait
// until the next check.
func (l *Launcher) scheduledCheck(ctx context.Context) time.Duration {
	offer, err := l.cfg.Client.CheckNow(ctx)

	switch {
	case err == nil:
	case errors.Is(err, update.ErrNotNewer):
		return CheckInterval
	default:
		return RetryInterval
	}

	l.mu.Lock()
	skip := l.declined[offer.Version] || l.installing
	l.mu.Unlock()

	if !skip {
		l.offer(ctx, offer)
	}

	return CheckInterval
}

// CheckNow is the menu action: one check, whatever the setting is, then a dialog with the result.
func (l *Launcher) CheckNow(ctx context.Context) {
	l.wg.Go(func() {
		out := CheckForUpdates(ctx, l.cfg.Language, launcherChecker(l.cfg.Client), l.cfg.BuildErr, l.cfg.Current)
		if out.Offer != nil {
			l.offer(ctx, out.Offer)

			return
		}

		l.cfg.UI.Inform(out.Message)
	})
}

// launcherChecker is a nil interface, not a typed nil, when there is no client.
func launcherChecker(c LauncherClient) UpdateChecker {
	if c == nil {
		return nil
	}

	return c
}

func (l *Launcher) offer(ctx context.Context, offer *update.Offer) {
	l.cfg.UI.OfferUpdate(offer, l.cfg.Current, func(install bool) {
		if !install {
			l.mu.Lock()
			l.declined[offer.Version] = true
			l.mu.Unlock()

			return
		}

		l.install(ctx, offer)
	})
}

// SetProgress records the download progress percentage for the UI bridge (epic 101).
func (l *Launcher) SetProgress(done, total int64) {
	if total <= 0 {
		return
	}
	pct := int(min(max(done*100/total, 0), 100))

	l.mu.Lock()
	l.progress = pct
	if l.installing {
		l.status = "downloading"
	}
	l.mu.Unlock()
}

// State returns the current updater snapshot for window.vukemDesktop (epic 101).
func (l *Launcher) State() UpdateState {
	l.mu.Lock()
	defer l.mu.Unlock()

	st := UpdateState{
		CurrentVersion: l.cfg.Current,
		AutoUpdate:     l.settings.AutoCheck(),
		Status:         l.status,
		Progress:       l.progress,
		Error:          l.lastErr,
	}
	if l.lastOffer != nil {
		st.AvailableVersion = l.lastOffer.Version
		st.ReleaseNotes = strings.TrimSpace(l.lastOffer.Notes)
	}
	return st
}

// CheckState runs a signed feed check and updates the bridge state without popping a native dialog.
func (l *Launcher) CheckState(ctx context.Context) UpdateState {
	if l.cfg.Client == nil {
		l.mu.Lock()
		l.status = "unconfigured"
		l.lastErr = ""
		l.mu.Unlock()
		return l.State()
	}

	l.mu.Lock()
	if l.installing {
		l.mu.Unlock()
		return l.State()
	}
	l.status = "checking"
	l.lastErr = ""
	l.mu.Unlock()

	offer, err := l.cfg.Client.CheckNow(ctx)

	l.mu.Lock()
	switch {
	case err == nil && offer != nil:
		l.lastOffer = offer
		l.status = "available"
		l.lastErr = ""
	case errors.Is(err, update.ErrNotNewer):
		l.lastOffer = nil
		l.status = "up_to_date"
		l.lastErr = ""
	case errors.Is(err, update.ErrNotConfigured):
		l.status = "unconfigured"
		l.lastErr = ""
	case isOfflineError(err):
		l.status = "offline"
		l.lastErr = ""
	default:
		l.status = "error"
		if err != nil {
			l.lastErr = err.Error()
		}
	}
	l.mu.Unlock()

	return l.State()
}

// InstallAvailable downloads, verifies and applies the available update (checking first if none is cached).
func (l *Launcher) InstallAvailable(ctx context.Context) UpdateState {
	if l.cfg.Client == nil {
		return l.State()
	}

	l.mu.Lock()
	offer := l.lastOffer
	busy := l.installing
	l.mu.Unlock()

	if busy {
		return l.State()
	}

	if offer == nil {
		st := l.CheckState(ctx)
		if st.Status != "available" {
			return st
		}
		l.mu.Lock()
		offer = l.lastOffer
		l.mu.Unlock()
	}

	if offer != nil {
		l.install(ctx, offer)
	}

	return l.State()
}

func isOfflineError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "dial tcp")
}

// install downloads, verifies and installs in the background. One install at a time.
func (l *Launcher) install(ctx context.Context, offer *update.Offer) {
	l.mu.Lock()
	if l.installing {
		l.mu.Unlock()

		return
	}

	l.installing = true
	l.status = "downloading"
	l.progress = 0
	l.lastErr = ""
	l.mu.Unlock()

	l.wg.Go(func() {
		l.cfg.UI.ShowProgress(offer.Version)
		err := l.cfg.Client.Apply(ctx, offer)
		l.cfg.UI.HideProgress()

		l.mu.Lock()
		l.installing = false
		if err != nil {
			l.status = "error"
			l.lastErr = err.Error()
		} else {
			l.status = "installing"
			l.progress = 100
		}
		l.mu.Unlock()

		if err != nil {
			l.cfg.UI.Fail(InstallFailedMessage(l.cfg.Language, err))
		}
	})
}
