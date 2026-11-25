package capturehost

import (
	"sync"
	"time"
)

// Activity is what the indicator says: whether a helper that was asked to capture is alive (SDD-025 section 19.4).
type Activity struct {
	// Active is true from the moment a helper is asked to capture until its process has exited.
	Active bool `json:"active"`
	// Helpers is how many such processes are alive.
	Helpers int `json:"helpers"`
	// SinceMS is when the indicator last went from off to on, in Unix milliseconds; 0 while it is off.
	SinceMS int64 `json:"since_ms"`
}

// Indicator counts the helper processes that were asked to capture and have not exited. It is driven by the host
// alone: it reads no recording row, no window and no page, so none of them can hide it (ADR-0051 item 4, T-167).
// The zero value is ready; a nil *Indicator ignores every call and reports nothing.
type Indicator struct {
	mu      sync.Mutex
	n       int
	since   time.Time
	changed chan struct{}
	// Now is the clock; nil is time.Now. It is for tests.
	Now func() time.Time
}

// Enter records a helper that was asked to capture.
func (i *Indicator) Enter() {
	if i == nil {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.n == 0 {
		now := time.Now
		if i.Now != nil {
			now = i.Now
		}

		i.since = now()
	}

	i.n++
	i.signal()
}

// Leave records the exit of a helper that Enter counted.
func (i *Indicator) Leave() {
	if i == nil {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.n > 0 {
		i.n--
	}

	if i.n == 0 {
		i.since = time.Time{}
	}

	i.signal()
}

// signal wakes every waiter on Changed; the caller holds the lock.
func (i *Indicator) signal() {
	if i.changed != nil {
		close(i.changed)
		i.changed = nil
	}
}

// State is what the indicator says now.
func (i *Indicator) State() Activity {
	if i == nil {
		return Activity{}
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	a := Activity{Active: i.n > 0, Helpers: i.n}
	if a.Active {
		a.SinceMS = i.since.UnixMilli()
	}

	return a
}

// Changed returns a channel that is closed at the next change of the count.
func (i *Indicator) Changed() <-chan struct{} {
	if i == nil {
		return nil
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.changed == nil {
		i.changed = make(chan struct{})
	}

	return i.changed
}
