// Package providertest is the fake provider server of the contract tests of the adapters (NFR-064,
// SDD-016 section 11): it checks each request an adapter makes against what the documentation of the
// API says and answers with a recorded response. It is a test helper: the product never imports it,
// and no test that uses it opens a connection to a real provider.
package providertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// Exchange is one request the adapter must make and the answer it gets.
type Exchange struct {
	Method string
	// Path is the path and the query of the request, as the adapter writes it.
	Path string
	// Header lists headers that must be present with exactly this value.
	Header map[string]string
	// Absent lists headers that must not be present.
	Absent []string
	// WantBody is a fixture with the JSON the adapter must send; empty when the request has no body.
	// It is compared as JSON, so the order of members does not matter.
	WantBody string
	// AnyBody accepts any body, for a test that does not look at the request.
	AnyBody bool
	// Status is the answer status; zero is 200.
	Status int
	// Response is a fixture with the recorded body of the answer.
	Response string
	// ResponseHeader is set on the answer.
	ResponseHeader map[string]string
}

// Recorder is a started fake server.
type Recorder struct {
	Server *httptest.Server

	mu       sync.Mutex
	Requests []Seen
}

// Seen is a request the server received.
type Seen struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// Serve starts a server that expects the exchanges, in order, and fails the test on any request that
// is not the next one or on a missing one at the end. dir is the testdata folder of the adapter.
func Serve(t *testing.T, dir string, exchanges ...Exchange) *Recorder {
	t.Helper()

	r := &Recorder{}
	next := 0

	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)

		r.mu.Lock()
		defer r.mu.Unlock()

		r.Requests = append(
			r.Requests,
			Seen{Method: req.Method, Path: req.URL.RequestURI(), Header: req.Header.Clone(), Body: body},
		)

		if next >= len(exchanges) {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusTeapot)

			return
		}

		x := exchanges[next]
		next++

		check(t, dir, x, req, body)

		for k, v := range x.ResponseHeader {
			w.Header().Set(k, v)
		}

		if x.Response != "" {
			w.Header().Set("Content-Type", "application/json")
		}

		status := x.Status
		if status == 0 {
			status = http.StatusOK
		}

		w.WriteHeader(status)

		if x.Response != "" {
			_, _ = w.Write(Fixture(t, dir, x.Response))
		}
	}))

	t.Cleanup(func() {
		r.Server.Close()

		r.mu.Lock()
		defer r.mu.Unlock()

		if next != len(exchanges) {
			t.Errorf("the adapter made %d requests, the contract has %d", next, len(exchanges))
		}
	})

	return r
}

func check(t *testing.T, dir string, x Exchange, req *http.Request, body []byte) {
	t.Helper()

	if req.Method != x.Method || req.URL.RequestURI() != x.Path {
		t.Errorf("request %s %s, want %s %s", req.Method, req.URL.RequestURI(), x.Method, x.Path)
	}

	for k, v := range x.Header {
		if got := req.Header.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}

	for _, k := range x.Absent {
		if got := req.Header.Get(k); got != "" {
			t.Errorf("header %s must not be sent, got %q", k, got)
		}
	}

	if x.WantBody == "" {
		if len(body) != 0 && !x.AnyBody {
			t.Errorf("the request must have no body, got %q", body)
		}

		return
	}

	var want, got any
	if err := json.Unmarshal(Fixture(t, dir, x.WantBody), &want); err != nil {
		t.Fatalf("fixture %s: %v", x.WantBody, err)
	}

	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("the request body is not JSON: %v", err)

		return
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("request body:\n got %s\nwant %s", bytes.TrimSpace(body), bytes.TrimSpace(Fixture(t, dir, x.WantBody)))
	}
}

// Fixture reads a recorded file of the adapter, with the line ends of a Windows checkout removed.
func Fixture(t *testing.T, dir, name string) []byte {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}

	return []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))
}

// Endpoint is the base URL of the server with a path, the way a connection to a provider names it.
func (r *Recorder) Endpoint(path string) string { return r.Server.URL + path }

// Last returns the last request seen.
func (r *Recorder) Last() Seen {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.Requests[len(r.Requests)-1]
}

// Main is the TestMain of a package that tests an adapter or the registry: it makes the whole run
// refuse any dial that is not to a loopback address, so no test of the layer can reach a real provider
// (NFR-064, SDD-016 section 11). The dial of a connection is shown to the recorder before the socket is made.
func Main(m *testing.M) {
	restore := outbound.SetDialRecorder(func(_ outbound.Purpose, address string) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}

		if ip, perr := netip.ParseAddr(host); perr == nil && ip.IsLoopback() || host == "localhost" {
			return nil
		}

		return fmt.Errorf(
			"providertest: a test dialled %s, and no test of the provider layer may reach the network",
			address,
		)
	})

	code := m.Run()

	restore()
	os.Exit(code)
}
