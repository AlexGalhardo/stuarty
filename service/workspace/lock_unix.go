//go:build !windows

package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type lockImpl struct{ f *os.File }

func tryLock(path string) (lockImpl, error) {
	// os.OpenFile sets O_CLOEXEC, so a child process never inherits the lock.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return lockImpl{}, fmt.Errorf("workspace: open lock file: %w", err)
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return lockImpl{}, ErrLocked
		}
		return lockImpl{}, fmt.Errorf("workspace: flock: %w", err)
	}
	return lockImpl{f: f}, nil
}

func (l lockImpl) release() error { return l.f.Close() }
