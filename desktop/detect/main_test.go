package main

import (
	"bufio"
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

func serve(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	dir := t.TempDir()
	srv := httptest.NewServer(handler(dir))
	t.Cleanup(srv.Close)

	return srv, dir
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

	res, err := client.Post(srv.URL+"/results", "application/json",
		strings.NewReader(`{"platform":"../Win32 x","webview":"Chrome-1.2","results":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 1 || strings.Contains(filepath.Base(files[0]), "..") ||
		!strings.HasPrefix(filepath.Base(files[0]), "win32-x-chrome-1.2-") {
		t.Fatalf("files = %v", files)
	}

	bad, err := client.Post(srv.URL+"/results", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Body.Close()

	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad body status %d", bad.StatusCode)
	}
}
