package shell

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	capturePath    = "/api/v1/system/capture"
	captureTimeout = 2 * time.Second
	// CapturePollInterval is how often the shell asks the server whether a capture helper is alive.
	CapturePollInterval = time.Second
	// CaptureGrace is how long the indicator stays on after the server stops answering: the helper dies with its
	// server, so a silent server is a capture that ended, but one missed answer must not hide a capture that is on.
	CaptureGrace = 5 * time.Second
)

// CaptureStatus is the answer of the server about the capture helper (SDD-025 section 19.4). It is the host's
// count of live helper processes, not the state of a recording.
type CaptureStatus struct {
	Active  bool  `json:"active"`
	SinceMS int64 `json:"since_ms"`
	Helpers int   `json:"helpers"`
}

// CaptureStatus asks the child server whether a capture helper is alive. The request goes to loopback only, with
// the workspace token.
func (s *Server) CaptureStatus(ctx context.Context) (CaptureStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, captureTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL()+capturePath, http.NoBody)
	if err != nil {
		return CaptureStatus{}, err
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: captureTimeout} //nolint:forbidigo // loopback only: the child server

	resp, err := client.Do(req)
	if err != nil {
		return CaptureStatus{}, err
	}

	defer func() { _ = resp.Body.Close() }()

	var out CaptureStatus
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&out) != nil {
		return CaptureStatus{}, errors.New("the server refused the capture status request: " + resp.Status)
	}

	return out, nil
}

// CaptureIndicator decides whether the system indicator of a capture is shown. It is driven by the answers of the
// server about its helper process and by nothing the window or the page does, so closing the window, hiding it to
// the tray or leaving the page cannot turn it off (ADR-0051 item 4, SDD-025 section 19.4).
type CaptureIndicator struct {
	// Ask reads the status; Show draws the indicator on or off, and is called only when the state changes.
	Ask  func(ctx context.Context) (CaptureStatus, error)
	Show func(on bool)
	// Now is the clock; nil is time.Now. Grace is how long a silent server keeps the indicator on; zero is
	// CaptureGrace.
	Now   func() time.Time
	Grace time.Duration

	mu        sync.Mutex
	on        bool
	silentFor time.Time // when the server stopped answering; zero while it answers
}

// On reports whether the indicator is shown now.
func (c *CaptureIndicator) On() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.on
}

// Poll asks once and moves the indicator: on while the host says a helper is alive; off when it says none is; and,
// when the server does not answer, unchanged for the grace and then off.
func (c *CaptureIndicator) Poll(ctx context.Context) {
	st, err := c.Ask(ctx)

	now := time.Now
	if c.Now != nil {
		now = c.Now
	}

	grace := c.Grace
	if grace == 0 {
		grace = CaptureGrace
	}

	c.mu.Lock()

	want := c.on

	switch {
	case err == nil:
		c.silentFor = time.Time{}
		want = st.Active
	case c.silentFor.IsZero():
		c.silentFor = now()
	case now().Sub(c.silentFor) >= grace:
		want = false
	}

	changed := want != c.on
	c.on = want
	c.mu.Unlock()

	if changed {
		c.Show(want)
	}
}

// Run polls every interval until ctx ends. It is the only thing that drives the indicator.
func (c *CaptureIndicator) Run(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = CapturePollInterval
	}

	tick := time.NewTicker(every)
	defer tick.Stop()

	for {
		c.Poll(ctx)

		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
