package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

const sseStream = "retry: 1000\ndata: \n\n" +
	"id: 1\nevent: batch.committed\ndata: {\"revision\":1}\n\n" +
	": heartbeat\n\n" +
	"id: 2\nevent: batch.committed\ndata: {\"revision\":2}\n\n" +
	"id: 3\nevent: job.updated\ndata: {\"revision\":3}\n\n"

func TestEventsPrintsNDJSONAndStopsAtCount(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	var got []EventsQuery

	h.res.events = func(_ context.Context, q EventsQuery) (io.ReadCloser, error) {
		got = append(got, q)
		return io.NopCloser(strings.NewReader(sseStream)), nil
	}

	if code := h.run("events", "--project", "prj_7k2m9x4q1b", "--since", "0", "--count", "2"); code != ExitOK {
		t.Fatalf("exit %d: %s%s", code, h.stdout.String(), h.stderr.String())
	}

	want := `{"id":"1","name":"batch.committed","data":{"revision":1}}` + "\n" +
		`{"id":"2","name":"batch.committed","data":{"revision":2}}` + "\n"
	if h.stdout.String() != want {
		t.Errorf("stdout = %q, want %q", h.stdout.String(), want)
	}

	if len(got) != 1 || got[0].ProjectID != "prj_7k2m9x4q1b" || got[0].Since != "0" {
		t.Errorf("queries = %+v", got)
	}
}

func TestEventsResumesAfterTheLastIDWhenTheServerCloses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	var got []EventsQuery

	h.res.events = func(_ context.Context, q EventsQuery) (io.ReadCloser, error) {
		got = append(got, q)

		body := "id: 7\nevent: batch.committed\ndata: {}\n\n"
		if len(got) == 2 {
			body = "id: 8\nevent: batch.committed\ndata: {}\n\n"
		}

		return io.NopCloser(strings.NewReader(body)), nil
	}

	if code := h.run("events", "--project", "prj_7k2m9x4q1b", "--count", "2"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if len(got) != 2 || got[1].LastEventID != "7" {
		t.Errorf("queries = %+v, want a second one resuming after 7", got)
	}
}

func TestEventsContinuesLiveAfterResyncRequired(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	var got []EventsQuery

	h.res.events = func(_ context.Context, q EventsQuery) (io.ReadCloser, error) {
		got = append(got, q)

		body := "id: 7\nevent: batch.committed\ndata: {}\n\n"
		if len(got) == 2 {
			body = "event: resync.required\ndata: {\"reason\":\"gap_too_large\"}\n\n"
		}

		if len(got) == 3 {
			body = "id: 9\nevent: batch.committed\ndata: {}\n\n"
		}

		return io.NopCloser(strings.NewReader(body)), nil
	}

	if code := h.run("events", "--project", "prj_7k2m9x4q1b", "--since", "3", "--count", "2"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr.String())
	}

	if len(got) != 3 || got[1].LastEventID != "7" || got[2].LastEventID != "" || got[2].Since != "" {
		t.Errorf("queries = %+v, want the third one live, with no resume point", got)
	}

	if !strings.Contains(h.stdout.String(), "resync.required") {
		t.Errorf("the resync event was not printed: %q", h.stdout.String())
	}
}

func TestEventsNeedsAServer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if code := h.run("events", "--project", "prj_7k2m9x4q1b", "--json"); code != ExitFailure {
		t.Fatalf("exit %d", code)
	}

	if doc := oneJSONDocument(t, h.stderr.String()+h.stdout.String()); doc["code"] != "E_INVALID_STATE" {
		t.Errorf("error = %v", doc)
	}
}

func TestHTTPEventsSendsTheCredentialAndTheResumePoint(t *testing.T) {
	t.Parallel()

	var req *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req = r
		if r.URL.Path == "/api/v1/projects/prj_nope/events" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":"E_NOT_FOUND","message":"no such project."}`)

			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseStream)
	}))
	t.Cleanup(srv.Close)

	open := httpEvents(srv.URL, "tok", outbound.LocalServer(0))

	body, err := open(t.Context(), EventsQuery{ProjectID: "prj_ok", LastEventID: "5", Since: "2"})
	if err != nil {
		t.Fatal(err)
	}

	_ = body.Close()

	if req.Header.Get("Authorization") != "Bearer tok" || req.Header.Get("Last-Event-ID") != "5" ||
		req.URL.Query().Get("since") != "2" || req.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("request = %v %v", req.Header, req.URL)
	}

	if _, err := open(t.Context(), EventsQuery{ProjectID: "prj_nope"}); err == nil ||
		!strings.Contains(err.Error(), "E_NOT_FOUND") {
		t.Errorf("a 404 must be the server error, got %v", err)
	}
}
