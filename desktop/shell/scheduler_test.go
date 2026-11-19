package shell

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func schedulerServer(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	port, _ := strconv.Atoi(u.Port())

	return &Server{Addr: u.Host, Port: port, Token: "tok", launcher: "secret"}
}

func TestSchedulerRequestCarriesTheTokenAndTheLauncherSecret(t *testing.T) {
	var got struct {
		auth, launcher, action, path string
	}

	s := schedulerServer(t, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Action string `json:"action"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		got.auth, got.launcher, got.action, got.path = r.Header.Get("Authorization"),
			r.Header.Get(launcherHeader), in.Action, r.URL.Path
		_, _ = w.Write([]byte(`{"paused":true,"scheduled":4}`))
	})

	st, err := s.Scheduler(context.Background(), SchedulerPause)
	if err != nil {
		t.Fatal(err)
	}

	if !st.Paused || st.Scheduled != 4 {
		t.Errorf("status = %+v", st)
	}

	if got.auth != "Bearer tok" || got.launcher != "secret" || got.action != "pause" || got.path != schedulerPath {
		t.Errorf("request = %+v", got)
	}
}

func TestSchedulerRefusalIsAnError(t *testing.T) {
	s := schedulerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if _, err := s.Scheduler(context.Background(), SchedulerResume); err == nil {
		t.Fatal("a 403 was taken as an answer")
	}
}
