package shell

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/vukem-video/desktop/update"
)

// ErrInstallUnsupported is returned by the installer of a system that cannot update itself yet.
var ErrInstallUnsupported = errors.New("installing updates is not supported on this system yet; " +
	"download the new version from " + ReleasePageURL)

// InstallerFor is the update.Installer of an operating system (a runtime.GOOS value). On Windows it runs
// the downloaded Inno Setup installer and then calls quit, so the installer can replace the files; on
// macOS and Linux it refuses with ErrInstallUnsupported.
func InstallerFor(goos string, quit func()) update.Installer {
	if goos == "windows" {
		return setupInstaller{run: runSetup, quit: quit}
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
