package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// client talks to the loopback test server only.
var client = &http.Client{Timeout: 10 * time.Second} //nolint:forbidigo // loopback test server

func serveIn(t *testing.T, dir string) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(nil)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	srv.Config.Handler = handler(dir, port)
	srv.Start()
	t.Cleanup(srv.Close)

	return srv
}

func serve(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	dir := t.TempDir()

	return serveIn(t, dir), dir
}

// post sends a results request the way the page does unless a header is changed.
type reply struct {
	StatusCode int
	Text       string
}

func post(t *testing.T, srv *httptest.Server, body string, tweak func(*http.Request)) reply {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/results", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", srv.URL)

	if tweak != nil {
		tweak(req)
	}

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	text, _ := io.ReadAll(res.Body)

	return reply{StatusCode: res.StatusCode, Text: strings.TrimSpace(string(text))}
}

const goodBody = `{"platform":"Win32","webview":"Chrome-1","results":[]}`

func files(t *testing.T, dir string) []string {
	t.Helper()

	found, _ := filepath.Glob(filepath.Join(dir, "*.json"))

	return found
}

func TestPageHasNoInlineCodeAndTheDefaultPolicy(t *testing.T) {
	srv, _ := serve(t)

	res, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	policy := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(policy, "script-src 'self'") || strings.Contains(policy, "unsafe-") {
		t.Fatalf("policy = %q", policy)
	}

	body, _ := os.ReadFile("index.html")
	for _, bad := range []string{"<script>", "<style>", "style=\"", "onclick="} {
		if strings.Contains(string(body), bad) {
			t.Errorf("index.html has inline code: %s", bad)
		}
	}
}

func TestFixtureAnswersRange(t *testing.T) {
	srv, _ := serve(t)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/fixtures/proxy.mp4", http.NoBody)
	req.Header.Set("Range", "bytes=0-99")

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent || res.ContentLength != 100 {
		t.Fatalf("status %d, length %d", res.StatusCode, res.ContentLength)
	}
}

func readEvents(t *testing.T, url, lastID string) []string {
	t.Helper()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/events", http.NoBody)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}

	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var ids []string

	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		if id, ok := strings.CutPrefix(scanner.Text(), "id: "); ok {
			ids = append(ids, id)
		}
	}

	return ids
}

func TestEventsAreCutAfterFortyAndResumeAfterLastEventID(t *testing.T) {
	srv, _ := serve(t)

	first := readEvents(t, srv.URL, "")
	if len(first) != 40 || first[0] != "1" || first[39] != "40" {
		t.Fatalf("first connection: %d events, %v", len(first), first)
	}

	rest := readEvents(t, srv.URL, "40")
	if len(rest) != 60 || rest[0] != "41" || rest[59] != "100" {
		t.Fatalf("resumed connection: %d events", len(rest))
	}
}

func TestResultsAreWrittenUnderASafeName(t *testing.T) {
	srv, dir := serve(t)

	res := post(t, srv, `{"platform":"../Win32 x","webview":"Chrome-1.2","results":[]}`, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}

	found := files(t, dir)
	if len(found) != 1 || strings.Contains(filepath.Base(found[0]), "..") ||
		!strings.HasPrefix(filepath.Base(found[0]), "win32-x-chrome-1.2-") {
		t.Fatalf("files = %v", found)
	}
}

func TestResultsAcceptSecFetchSiteWithoutOrigin(t *testing.T) {
	srv, dir := serve(t)

	res := post(t, srv, goodBody, func(r *http.Request) {
		r.Header.Del("Origin")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	})
	if res.StatusCode != http.StatusOK || len(files(t, dir)) != 1 {
		t.Fatalf("status %d, files %v", res.StatusCode, files(t, dir))
	}
}

func TestResultsRefusals(t *testing.T) {
	cases := []struct {
		name   string
		tweak  func(*http.Request)
		status int
	}{
		{
			"another origin",
			func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") },
			http.StatusForbidden,
		},
		{"another origin that claims same-origin", func(r *http.Request) {
			r.Header.Set("Origin", "http://evil.example")
			r.Header.Set("Sec-Fetch-Site", "same-origin")
		}, http.StatusForbidden},
		{"a cross-site fetch with no origin", func(r *http.Request) {
			r.Header.Del("Origin")
			r.Header.Set("Sec-Fetch-Site", "cross-site")
		}, http.StatusForbidden},
		{"neither origin nor fetch metadata", func(r *http.Request) { r.Header.Del("Origin") }, http.StatusForbidden},
		{
			"text/plain",
			func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
			http.StatusUnsupportedMediaType,
		},
		{
			"a form",
			func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") },
			http.StatusUnsupportedMediaType,
		},
		{"no content type", func(r *http.Request) { r.Header.Del("Content-Type") }, http.StatusUnsupportedMediaType},
		{"another host", func(r *http.Request) { r.Host = "evil.example:7979" }, http.StatusForbidden},
		{"the right name, the wrong port", func(r *http.Request) { r.Host = "localhost:1" }, http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := serve(t)

			res := post(t, srv, goodBody, tc.tweak)
			if res.StatusCode != tc.status {
				t.Fatalf("status %d, want %d", res.StatusCode, tc.status)
			}

			if len(files(t, dir)) != 0 {
				t.Fatalf("a refused request wrote %v", files(t, dir))
			}
		})
	}
}

func TestEveryRouteChecksTheHost(t *testing.T) {
	srv, _ := serve(t)

	for _, path := range []string{"/", "/events", "/fixtures/proxy.mp4"} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, http.NoBody)
		req.Host = "rebind.example"

		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = res.Body.Close()

		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s with another Host = %d", path, res.StatusCode)
		}
	}
}

func TestTheLocalhostNameIsAccepted(t *testing.T) {
	srv, dir := serve(t)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	res := post(t, srv, goodBody, func(r *http.Request) {
		r.Host = "localhost:" + port
		r.Header.Set("Origin", "http://localhost:"+port)
	})
	if res.StatusCode != http.StatusOK || len(files(t, dir)) != 1 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestResultsNeverOverwriteAFile(t *testing.T) {
	srv, dir := serve(t)

	first := post(t, srv, goodBody, nil)
	second := post(t, srv, `{"platform":"Win32","webview":"Chrome-1","results":[1]}`, nil)

	if first.StatusCode != http.StatusOK || second.StatusCode != http.StatusOK {
		t.Fatalf("statuses %d %d", first.StatusCode, second.StatusCode)
	}

	found := files(t, dir)
	if len(found) != 2 {
		t.Fatalf("files = %v", found)
	}

	var suffixed string

	for _, f := range found {
		if strings.HasSuffix(f, "-2.json") {
			suffixed = f
		}
	}

	if suffixed == "" {
		t.Fatalf("no -2 file in %v", found)
	}

	if saved, _ := os.ReadFile(suffixed); !strings.Contains(string(saved), `"results":[1]`) {
		t.Fatalf("the second body is not in %s", suffixed)
	}
}

func TestErrorsAreGeneric(t *testing.T) {
	srv, _ := serve(t)

	bad := post(t, srv, "not json", nil)
	if bad.StatusCode != http.StatusBadRequest || bad.Text != "bad request" {
		t.Fatalf("bad body: %d %q", bad.StatusCode, bad.Text)
	}

	// A results folder that cannot be made: its path is a file.
	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	broken := post(t, serveIn(t, filepath.Join(file, "sub")), goodBody, nil)
	if broken.StatusCode != http.StatusInternalServerError || broken.Text != "internal error" {
		t.Fatalf("server error: %d %q", broken.StatusCode, broken.Text)
	}
}
