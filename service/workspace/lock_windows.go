package workspace

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// lockOffset is one byte beyond the end of the empty lock file, so no reader
// of the file ever touches the locked region (SDD-005 section 6.2).
const lockOffset = 0x7fffff00

type lockImpl struct{ h windows.Handle }

func tryLock(path string) (lockImpl, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return lockImpl{}, fmt.Errorf("workspace: %w", err)
	}
	// Share read and write so scanners and backup tools can open the file; the
	// handle is not inheritable (nil security attributes).
	h, err := windows.CreateFile(name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return lockImpl{}, fmt.Errorf("workspace: open lock file: %w", err)
	}

	ol := &windows.Overlapped{Offset: lockOffset}
	err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		_ = windows.CloseHandle(h)
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return lockImpl{}, ErrLocked
		}
		return lockImpl{}, fmt.Errorf("workspace: LockFileEx: %w", err)
	}
	return lockImpl{h: h}, nil
}

func (l lockImpl) release() error {
	ol := &windows.Overlapped{Offset: lockOffset}
	unlockErr := windows.UnlockFileEx(l.h, 0, 1, 0, ol)
	closeErr := windows.CloseHandle(l.h)
	return errors.Join(unlockErr, closeErr)
}
