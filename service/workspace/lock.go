package workspace

import "fmt"

// Lock is the exclusive ownership of a workspace.
type Lock struct {
	d        Dir
	released bool
	impl     lockImpl
}

// TryLock takes the exclusive lock on <workspace>/lock without waiting.
// It returns ErrLocked when another process, or another Lock in this one,
// holds it. The operating system releases it if the process dies.
func (d Dir) TryLock() (*Lock, error) {
	impl, err := tryLock(d.Path("lock"))
	if err != nil {
		return nil, err
	}
	return &Lock{d: d, impl: impl}, nil
}

// Release gives the lock up. It is safe to call twice.
func (l *Lock) Release() error {
	if l == nil || l.released {
		return nil
	}
	l.released = true
	if err := l.impl.release(); err != nil {
		return fmt.Errorf("workspace: release lock: %w", err)
	}
	return nil
}
