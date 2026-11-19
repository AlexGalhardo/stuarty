package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

// ErrInstallUnsupported is returned by the installer of a system that cannot update itself yet.
var ErrInstallUnsupported = errors.New("installing updates is not supported on this system yet; " +
	"download the new version from " + ReleasePageURL)

// InstallerFor is the update.Installer of an operating system (a runtime.GOOS value). On Windows it runs
// the downloaded Inno Setup installer and then calls quit, so the installer can replace the files; on
// macOS and Linux it replaces the bundle or the AppImage in place with a backup (swap.go); any other
// system refuses with ErrInstallUnsupported.
func InstallerFor(goos string, quit func()) update.Installer {
	switch goos {
	case "windows":
		return setupInstaller{run: runSetup, quit: quit}
	case "darwin", "linux":
		return swapInstaller{
			goos:   goos,
			quit:   quit,
			locate: locateRunning,
			start:  startWatchdog,
			state:  UpdateStateDir,
			now:    time.Now,
		}
	}

	return unsupportedInstaller{}
}

type setupInstaller struct {
	run  func(path string) error
	quit func()
}

// Install starts the setup program and quits only when it started; the setup outlives this process.
func (i setupInstaller) Install(_ context.Context, a update.Artifact) error {
	if err := i.run(a.Path); err != nil {
		return err
	}

	i.quit()

	return nil
}

type unsupportedInstaller struct{}

func (unsupportedInstaller) Install(context.Context, update.Artifact) error {
	return ErrInstallUnsupported
}

// swapInstaller replaces the running application and hands the restart to the watchdog.
type swapInstaller struct {
	goos   string
	quit   func()
	locate func(goos string) (Target, error)
	// start runs the backup program as the watchdog; it must not wait for it.
	start func(backupExec, pendingFile string) error
	state func() (string, error)
	now   func() time.Time
}

// artifactSuffix is the file type of the artifact of each system.
var artifactSuffix = map[string]string{"darwin": ".zip", "linux": ".AppImage"}

// Install swaps the application, starts the watchdog and quits only when both worked. Any error leaves
// the running version in use (and the files as they were).
func (i swapInstaller) Install(_ context.Context, a update.Artifact) error {
	if !strings.HasSuffix(a.Path, artifactSuffix[i.goos]) {
		return fmt.Errorf("%w: the %s update is not a %s file", ErrInstallUnsupported, i.goos, artifactSuffix[i.goos])
	}

	target, err := i.locate(i.goos)
	if err != nil {
		return err
	}

	state, err := i.state()
	if err != nil {
		return err
	}

	p, err := Swap(SwapRequest{StateDir: state, Target: target, Artifact: a.Path, Version: a.Version, Now: i.now})
	if err != nil {
		return err
	}

	if err := i.start(p.BackupExec, pendingPath(state)); err != nil {
		if undo := rollbackFiles(p); undo != nil {
			return fmt.Errorf("start the watchdog: %w (and restoring failed: %w)", err, undo)
		}

		_ = os.Remove(pendingPath(state))

		return fmt.Errorf("start the watchdog: %w", err)
	}

	i.quit()

	return nil
}

func locateRunning(goos string) (Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return Target{}, fmt.Errorf("find the running program: %w", err)
	}

	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	return LocateTarget(goos, exe, os.Getenv)
}

// WatchFlag is the command line flag of the watchdog: the backup program run as
// `stuarty-desktop --update-watch <pending.json>`.
const WatchFlag = "--update-watch"

func startWatchdog(backupExec, pendingFile string) error {
	_, err := startDetached(backupExec, WatchFlag, pendingFile)

	return err
}

// RunWatchdog is the body of the watchdog process: pendingFile is the marker the installer wrote.
func RunWatchdog(ctx context.Context, pendingFile string) error {
	return Watch(ctx, WatchConfig{
		StateDir: filepath.Dir(pendingFile),
		Start:    func(execPath string) (<-chan struct{}, error) { return startDetached(execPath) },
	})
}

// bundleOf is the .app directory that contains execPath, or empty.
func bundleOf(execPath string) string {
	for dir := filepath.Dir(execPath); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if strings.HasSuffix(dir, ".app") {
			return dir
		}
	}

	return ""
}
