package outbound_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

func TestLocalServerReachesLoopbackOnly(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)
	defer srv.Close()

	client := outbound.LocalServer(2 * time.Second)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("loopback: %v", err)
	}

	_ = res.Body.Close()

	// A non-loopback literal is refused before any packet leaves (TEST-NET-1, never routed).
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, "http://192.0.2.1:80/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	if res, err = client.Do(req); err == nil {
		_ = res.Body.Close()
		t.Fatal("a non-loopback address was dialed")
	} else if !errors.Is(err, outbound.ErrNotLoopback) {
		t.Errorf("err = %v, want ErrNotLoopback", err)
	}
}

func TestRemoteServerFollowsNoRedirect(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the redirect was followed")
	}))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	res, err := outbound.RemoteServer(2 * time.Second).Do(req)
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want the redirect itself", res.StatusCode)
	}
}
