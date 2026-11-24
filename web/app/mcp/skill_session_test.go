package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
)

// skillSpy is a Service that answers SkillGet itself and remembers the caller of each read, which is
// what the recording of a skill (SDD-017 section 8.1) is decided on.
type skillSpy struct {
	service.Service

	mu      sync.Mutex
	callers []service.Caller
	views   []bool
}

func (s *skillSpy) SkillGet(ctx context.Context, req service.LibraryRef) (*service.SkillGetResult, error) {
	c, _ := service.CallerFrom(ctx)

	s.mu.Lock()
	s.callers = append(s.callers, c)
	s.views = append(s.views, req.AgentView)
	s.mu.Unlock()

	return &service.SkillGetResult{Text: "skill " + req.Name, ContentHash: "sha256:x"}, nil
}

// sessionGuard stands in for the guard of the API with a session token: the header names the session the
// token is bound to, and the Caller it puts in the context carries that id, as auth.AgentCaller does.
func sessionGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := service.Caller{
			Actor: "local:chat-test", Scopes: service.AllScopes, ChatSessionID: r.Header.Get("X-Test-Session"),
		}

		next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), c)))
	})
}

type sessionTransport struct {
	session string
	next    http.RoundTripper
}

func (s sessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())

	if s.session != "" {
		r.Header.Set("X-Test-Session", s.session)
	}

	return s.next.RoundTrip(r)
}

// Task 3 of epic 76: the tool library hands the caller of the request to SkillGet whole, so the session of the
// token that read the skill reaches the service, and a caller with none reaches it with none.
func TestLibraryGetSkillCarriesTheSessionOfTheToken(t *testing.T) {
	t.Parallel()

	spy := &skillSpy{Service: newService(t)}

	h, err := Handler(spy, Options{})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(sessionGuard(h))
	defer srv.Close()

	args := map[string]any{"action": "get", "kind": "skill", "name": "captions"}

	for _, session := range []string{"cht_0000000007", ""} {
		own := *srv.Client()
		own.Transport = sessionTransport{session: session, next: srv.Client().Transport}

		client := sdk.NewClient(&sdk.Implementation{Name: "http-client", Version: "1"}, nil)

		cs, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: &own}, nil)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = cs.Close() })

		ok(t, call(t, cs, "library", args))
	}

	spy.mu.Lock()
	defer spy.mu.Unlock()

	if len(spy.callers) != 2 {
		t.Fatalf("SkillGet was called %d times, want 2", len(spy.callers))
	}

	if spy.callers[0].ChatSessionID != "cht_0000000007" || spy.callers[1].ChatSessionID != "" {
		t.Errorf("sessions = %q and %q", spy.callers[0].ChatSessionID, spy.callers[1].ChatSessionID)
	}

	if !spy.views[0] || !spy.views[1] {
		t.Error("the tool library must read as an agent view")
	}
}
