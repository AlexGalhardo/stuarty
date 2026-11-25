package shell

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestCaptureStatusRequestCarriesTheToken(t *testing.T) {
	var auth, path, method string

	s := schedulerServer(t, func(w http.ResponseWriter, r *http.Request) {
		auth, path, method = r.Header.Get("Authorization"), r.URL.Path, r.Method
		_, _ = w.Write([]byte(`{"active":true,"since_ms":1234,"helpers":1}`))
	})

	st, err := s.CaptureStatus(context.Background())
	if err != nil || !st.Active || st.SinceMS != 1234 || st.Helpers != 1 {
		t.Fatalf("status = %+v, %v", st, err)
	}

	if auth != "Bearer tok" || path != capturePath || method != http.MethodGet {
		t.Errorf("request = %q %q %q", auth, path, method)
	}
}

func TestCaptureStatusRefusalIsAnError(t *testing.T) {
	s := schedulerServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })

	if _, err := s.CaptureStatus(context.Background()); err == nil {
		t.Fatal("a 404 was taken as an answer")
	}
}

type fakeAsk struct {
	status CaptureStatus
	err    error
}

func (f *fakeAsk) ask(context.Context) (CaptureStatus, error) { return f.status, f.err }

// T-167, the shell side: the indicator follows the helper count of the host, goes on and off only on a change, and
// has no input from a window.
func TestSecurity_T167_TheIndicatorFollowsTheServerAnswerAndCallsShowOnlyOnAChange(t *testing.T) {
	f := &fakeAsk{}
	now := time.Unix(1000, 0)

	var shown []bool

	ind := &CaptureIndicator{
		Ask:  f.ask,
		Show: func(on bool) { shown = append(shown, on) },
		Now:  func() time.Time { return now },
	}

	ind.Poll(context.Background())
	ind.Poll(context.Background())

	if ind.On() || len(shown) != 0 {
		t.Fatalf("idle: on %v, shown %v", ind.On(), shown)
	}

	f.status = CaptureStatus{Active: true, Helpers: 1, SinceMS: 5}

	ind.Poll(context.Background())
	ind.Poll(context.Background())

	if !ind.On() || len(shown) != 1 || !shown[0] {
		t.Fatalf("a helper is alive: on %v, shown %v", ind.On(), shown)
	}

	f.status = CaptureStatus{}

	ind.Poll(context.Background())

	if ind.On() || len(shown) != 2 || shown[1] {
		t.Fatalf("the helper exited: on %v, shown %v", ind.On(), shown)
	}
}

// A server that stops answering keeps a capture that is on visible for the grace, then the helper is gone with it.
func TestSecurity_T167_ASilentServerKeepsTheIndicatorOnForTheGraceOnly(t *testing.T) {
	f := &fakeAsk{status: CaptureStatus{Active: true, Helpers: 1}}
	now := time.Unix(1000, 0)

	var shown []bool

	ind := &CaptureIndicator{
		Ask: f.ask, Show: func(on bool) { shown = append(shown, on) }, Now: func() time.Time { return now },
		Grace: 5 * time.Second,
	}

	ind.Poll(context.Background())

	f.err = errors.New("no answer")

	ind.Poll(context.Background()) // the first silence

	now = now.Add(4 * time.Second)

	ind.Poll(context.Background())

	if !ind.On() {
		t.Fatal("the indicator went off before the grace was over")
	}

	now = now.Add(2 * time.Second)

	ind.Poll(context.Background())

	if ind.On() || len(shown) != 2 || shown[1] {
		t.Fatalf("after the grace: on %v, shown %v", ind.On(), shown)
	}

	// the server is back and says a helper is alive: on again
	f.err = nil

	ind.Poll(context.Background())

	if !ind.On() || len(shown) != 3 {
		t.Fatalf("the server answered again: on %v, shown %v", ind.On(), shown)
	}

	// a silent server never turns an indicator on
	quiet := &CaptureIndicator{
		Ask:  func(context.Context) (CaptureStatus, error) { return CaptureStatus{}, errors.New("x") },
		Show: func(bool) { t.Error("Show was called for a server that never answered") },
		Now:  func() time.Time { return now },
	}
	for range 3 {
		now = now.Add(10 * time.Second)
		quiet.Poll(context.Background())
	}
}

func TestCaptureIndicatorRunStopsWithItsContext(t *testing.T) {
	f := &fakeAsk{status: CaptureStatus{Active: true}}
	on := make(chan bool, 4)

	ind := &CaptureIndicator{Ask: f.ask, Show: func(v bool) { on <- v }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		ind.Run(ctx, 5*time.Millisecond)
		close(done)
	}()

	select {
	case v := <-on:
		if !v {
			t.Fatal("shown off")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never shown")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}
