package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestUploadChecksTheFileItNames(t *testing.T) {
	root := t.TempDir()
	h := (&network{mediaRoot: root}).mux()
	video := filepath.Join(root, "out.mp4")

	if got := post(t, h, "/videos", `{"title":"t","path":`+quote(video)+`}`); got.Code != http.StatusBadRequest {
		t.Errorf("a missing file was accepted: %d", got.Code)
	}

	if err := os.WriteFile(video, []byte("not empty"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := post(t, h, "/videos", `{"title":"t","path":`+quote(video)+`}`); got.Code != http.StatusOK {
		t.Fatalf("the exported file was refused: %d %s", got.Code, got.Body)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/_uploads", http.NoBody))

	if !strings.Contains(rec.Body.String(), `"bytes":9`) {
		t.Errorf("the upload was not recorded with its size: %s", rec.Body)
	}
}

func TestUploadRefusesAFileOutsideTheMediaRoot(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "out.mp4")
	if err := os.WriteFile(outside, []byte("not empty"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := (&network{mediaRoot: t.TempDir()}).mux()

	if got := post(t, h, "/videos", `{"title":"t","path":`+quote(outside)+`}`); got.Code != http.StatusBadRequest {
		t.Errorf("a file outside the media root was accepted: %d %s", got.Code, got.Body)
	}
}

func TestAuthorizeSendsBackOnlyToLoopback(t *testing.T) {
	h := (&network{}).mux()

	for _, uri := range []string{"http://evil.example/cb", "https://127.0.0.1/cb", "%zz", ""} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/authorize?redirect_uri="+url.QueryEscape(uri), http.NoBody))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("redirect_uri %q got %d, want 400", uri, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/authorize?redirect_uri="+url.QueryEscape("http://127.0.0.1:9/cb"), http.NoBody))

	if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther {
		t.Errorf("a loopback redirect_uri got %d", rec.Code)
	}
}

func TestTokenNeedsTheClientSecret(t *testing.T) {
	h := (&network{}).mux()
	form := url.Values{"client_secret": {"wrong"}}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("a wrong client secret got a token: %d", rec.Code)
	}
}

func TestListenLoopbackRefusesOtherAddresses(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "192.0.2.1:0", "example.com:0", "nonsense"} {
		if l, err := listenLoopback(addr); err == nil {
			_ = l.Close()

			t.Errorf("%s was accepted", addr)
		}
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }
