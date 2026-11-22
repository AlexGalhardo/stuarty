package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// VV-369: the names "." and ".." are dot segments, which a URL path normalises with a redirect. A server that
// answers the name as a command name must still be reached, so the answer is E_UNKNOWN_COMMAND and not a
// redirect the client reads as a broken server.
func TestCommandHelpWithADotNameReachesTheServer(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/commands/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"E_UNKNOWN_COMMAND","message":"no such command","retryable":false}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New(srv.URL+"/", "tok", srv.Client())

	for _, name := range []string{".", "..", "%2e", "a/b", "../x"} {
		_, err := c.CommandHelp(asCLI("dots"), service.CommandHelpRequest{Name: name})
		if err == nil {
			t.Fatalf("%q: no error", name)
		}

		if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != command.CodeUnknownCommand {
			t.Errorf("%q: %v", name, err)
		}
	}
}

// An id reaches the route it was sent to, whole, whatever it holds: a dot segment, a slash, a percent sign, a
// question mark or a hash never moves the request to another route or cuts the id (VV-369).
func TestAnIDReachesItsRouteWhole(t *testing.T) {
	t.Parallel()

	var (
		mu  sync.Mutex
		got []string
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.PathValue("id"))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"E_NOT_FOUND","message":"no such job","retryable":false}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := New(srv.URL+"/", "tok", srv.Client())

	ids := []string{".", "..", "a/b", "../../keys", "100%", "%2e", "%2E%2E", "a?b=c", "a#b", "a b", "job_plain1"}
	for _, id := range ids {
		_, err := c.JobGet(asCLI("ids"), service.JobGetRequest{JobID: id})

		if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != command.CodeNotFound {
			t.Errorf("%q: %v", id, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if !slices.Equal(got, ids) {
		t.Errorf("the route saw %q, want %q", got, ids)
	}
}
