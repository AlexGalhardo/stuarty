// Package testpublish is the publishing environment of the tests: a fake social network on loopback that
// records every request it gets, the seed of a succeeded export, and (env.go) a service on a workspace that
// a parent test and a child process both open. It is for tests only; it lives under web/ because it opens the
// real store, which service/ may not import (ADR-0001).
package testpublish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

// Constants of the sign-in the fake network plays; tests look for the canaries in what must never leak.
const (
	ClientSecretCanary = "GOCSPX-client-secret-CANARY-77"
	RefreshCanary      = "refresh-token-CANARY-88"
	Redirect           = "http://127.0.0.1:7878/social/callback"
)

// Tester is the part of testing.TB the helpers use, so a program that is not a test can pass its own.
type Tester interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
	TempDir() string
}

// FakeNet is a fake social network that records every request that reaches it: the recording destination of
// the tests of epic 18.
type FakeNet struct {
	Srv *httptest.Server

	Mu       sync.Mutex
	Requests []string
	Forms    []url.Values
	Videos   map[string]string // id to the state it answers
	BadCode  bool
	// VideoURL, when set, is the address of a video the network answers.
	VideoURL string
	// Posted holds the path of the file of every video that was sent.
	Posted []string

	hookMu sync.Mutex
	hook   func(r *http.Request) bool
}

// NewFakeNet starts the network; the test closes it.
func NewFakeNet(t Tester) *FakeNet {
	t.Helper()

	n := &FakeNet{Videos: map[string]string{}}

	n.Srv = httptest.NewServer(http.HandlerFunc(n.serve))

	t.Cleanup(n.Srv.Close)

	return n
}

// SetHook sets a function that runs for every request before the network answers it, with no lock of the
// network held, so it may block: that is how a test keeps a request in flight while it ends the process that
// made it. It answers whether the network goes on to answer the request; false drops it unanswered and
// unrecorded, as a request whose sender died would be. nil removes the hook.
func (n *FakeNet) SetHook(h func(r *http.Request) bool) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()

	n.hook = h
}

func (n *FakeNet) serve(w http.ResponseWriter, r *http.Request) {
	n.hookMu.Lock()
	hook := n.hook
	n.hookMu.Unlock()

	if hook != nil && !hook(r) {
		return
	}

	n.Mu.Lock()
	defer n.Mu.Unlock()

	n.Requests = append(n.Requests, r.Method+" "+r.URL.Path)

	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		n.Forms = append(n.Forms, r.PostForm)

		if n.BadCode {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": RefreshCanary, "expires_in": 3600,
		})
	case r.URL.Path == "/account":
		_, _ = w.Write([]byte(`{"id":"UC-FAKE","title":"Fake channel"}`))
	case r.URL.Path == "/videos" && r.Method == http.MethodPost:
		var sent struct {
			Path string `json:"path"`
		}

		_ = json.NewDecoder(r.Body).Decode(&sent)
		n.Posted = append(n.Posted, sent.Path)

		id := "vid" + string(rune('a'+len(n.Videos)))
		n.Videos[id] = "published"

		address := "https://fake.example/watch/" + id
		if n.VideoURL != "" {
			address = n.VideoURL
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "url": address, "state": "published"})
	case strings.HasPrefix(r.URL.Path, "/videos/"):
		id := strings.TrimPrefix(r.URL.Path, "/videos/")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "state": n.Videos[id]})
	default:
		_, _ = w.Write([]byte("pong"))
	}
}

// Count is how many requests reached the network.
func (n *FakeNet) Count() int {
	n.Mu.Lock()
	defer n.Mu.Unlock()

	return len(n.Requests)
}

// Seen lists the requests, each as "METHOD /path".
func (n *FakeNet) Seen() []string {
	n.Mu.Lock()
	defer n.Mu.Unlock()

	return append([]string(nil), n.Requests...)
}

// Finished is how many posts the network accepted and answered.
func (n *FakeNet) Finished() int {
	n.Mu.Lock()
	defer n.Mu.Unlock()

	return len(n.Videos)
}
