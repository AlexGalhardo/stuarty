// Package socialtest replays recorded request and response pairs of a network, for the tests of the
// connectors of X and TikTok (SDD-020 part 4). It is test support: nothing in the product imports it, and it
// opens no connection but the one to its own server on loopback.
package socialtest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections"
)

// Exchange is one recorded pair of a request and its response.
type Exchange struct {
	Name    string `json:"name"`
	Request struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	} `json:"request"`
	Response struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers,omitempty"`
		Body    json.RawMessage   `json:"body"`
	} `json:"response"`
}

// Request is what the server received.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Form   url.Values
	Header http.Header
	Body   []byte
}

// Server answers each request with the first recorded exchange still waiting for its method and path.
type Server struct {
	URL string
	// Client is what a test puts in the credential: it reaches this server and no other.
	Client connections.Doer

	mu       sync.Mutex
	srv      *httptest.Server
	queue    []*Exchange
	replace  map[string]string
	Requests []Request
}

// Load reads the exchanges of testdata/<file> and returns the ones named, in that order. A name may repeat.
func Load(t *testing.T, file string, names ...string) []*Exchange {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Exchanges []*Exchange `json:"exchanges"`
	}

	if jerr := json.Unmarshal(raw, &doc); jerr != nil {
		t.Fatal(jerr)
	}

	out := make([]*Exchange, 0, len(names))

	for _, n := range names {
		found := false

		for _, e := range doc.Exchanges {
			if e.Name == n {
				c := *e
				out = append(out, &c)
				found = true

				break
			}
		}

		if !found {
			t.Fatalf("no recorded pair %q in %s", n, file)
		}
	}

	return out
}

// New starts a server that replays the named exchanges of testdata/<file>. Every occurrence of a key of
// hosts in a response body is replaced by the address of the server, so that an upload address the network
// gave points back to it.
func New(t *testing.T, file string, hosts []string, names ...string) *Server {
	t.Helper()

	s := &Server{queue: Load(t, file, names...), replace: map[string]string{}}

	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	s.URL, s.Client = s.srv.URL, s.srv.Client()

	for _, h := range hosts {
		s.replace[h] = s.URL
	}

	t.Cleanup(s.srv.Close)

	return s
}

// Queue adds exchanges to wait for, in the middle of a test.
func (s *Server) Queue(t *testing.T, file string, names ...string) {
	t.Helper()

	more := Load(t, file, names...)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.queue = append(s.queue, more...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	var form url.Values

	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		form, _ = url.ParseQuery(string(body))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Requests = append(s.Requests, Request{
		Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Form: form, Header: r.Header.Clone(), Body: body,
	})

	for i, e := range s.queue {
		if e.Request.Method != r.Method || e.Request.Path != r.URL.Path {
			continue
		}

		s.queue = append(s.queue[:i], s.queue[i+1:]...)

		out := []byte(e.Response.Body)
		for from, to := range s.replace {
			out = bytes.ReplaceAll(out, []byte(from), []byte(to))
		}

		for k, v := range e.Response.Headers {
			w.Header().Set(k, v)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(e.Response.Status)

		if string(out) != "null" && len(out) > 0 {
			_, _ = w.Write(out)
		}

		return
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"no recorded pair"}`))
}

// Left is the number of exchanges that were queued and not asked for.
func (s *Server) Left() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.queue)
}

// Calls lists the requests received as "METHOD path", in order.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.Requests))
	for i, r := range s.Requests {
		out[i] = r.Method + " " + r.Path
	}

	return out
}

// Seen is every request received, copied.
func (s *Server) Seen() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Request(nil), s.Requests...)
}

// NoSecretInAddress fails the test when one of the secrets appears in the address or the query of a request:
// a token travels in a header or a body, never in an address.
func (s *Server) NoSecretInAddress(t *testing.T, secrets ...string) {
	t.Helper()

	for _, r := range s.Seen() {
		addr := r.Path + "?" + r.Query.Encode()

		for _, sec := range secrets {
			if strings.Contains(addr, sec) {
				t.Errorf("a secret is in the address of %s %s", r.Method, addr)
			}
		}
	}
}
