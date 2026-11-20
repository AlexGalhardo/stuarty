package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

const (
	schedulerPath    = "/api/v1/system/scheduler"
	schedulerTimeout = 3 * time.Second
)

// SchedulerStatus is the answer of the server about the publication scheduler (SDD-020 section 27).
type SchedulerStatus struct {
	Paused    bool `json:"paused"`
	Scheduled int  `json:"scheduled"`
}

// The actions of POST /api/v1/system/scheduler.
const (
	SchedulerStatusAction = "status"
	SchedulerPause        = "pause"
	SchedulerResume       = "resume"
)

// Scheduler asks the server to read, pause or resume its scheduler. The request goes to the child server
// on loopback only, with the workspace token and the launcher secret that proves this shell started it.
func (s *Server) Scheduler(ctx context.Context, action string) (SchedulerStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, schedulerTimeout)
	defer cancel()

	body, err := json.Marshal(map[string]string{"action": action})
	if err != nil {
		return SchedulerStatus{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL()+schedulerPath, bytes.NewReader(body))
	if err != nil {
		return SchedulerStatus{}, err
	}

	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set(launcherHeader, s.launcher)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: schedulerTimeout} //nolint:forbidigo // loopback only: the child server

	resp, err := client.Do(req)
	if err != nil {
		return SchedulerStatus{}, err
	}

	defer func() { _ = resp.Body.Close() }()

	var out SchedulerStatus
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&out) != nil {
		return SchedulerStatus{}, errors.New("the server refused the scheduler request: " + resp.Status)
	}

	return out, nil
}
