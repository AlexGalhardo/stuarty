package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
