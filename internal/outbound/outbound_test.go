package outbound_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
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

// The install client speaks HTTPS to listed hosts only, whatever the URL.
func TestHelperDownloadRefusesWhatIsNotListed(t *testing.T) {
	t.Parallel()

	client := outbound.HelperDownload([]string{"github.com"}, 2*time.Second)

	for _, target := range []string{
		"http://github.com/x",          // not HTTPS
		"https://evil.example/x",       // not on the list
		"https://127.0.0.1:1/x",        // an address, not a listed name
		"https://github.com.evil.io/x", // a lookalike
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
			t.Errorf("%s was fetched", target)
		} else if !errors.Is(err, outbound.ErrHostNotAllowed) {
			t.Errorf("%s: %v, want ErrHostNotAllowed", target, err)
		}
	}
}

func TestDesktopUpdateReachesItsFeedHostOverHTTPSOnly(t *testing.T) {
	t.Parallel()

	if _, err := outbound.DesktopUpdate(
		"http://127.0.0.1:1/feed.json",
		time.Second,
		nil,
	); !errors.Is(
		err,
		outbound.ErrScheme,
	) {
		t.Errorf("an http feed URL: got %v, want ErrScheme", err)
	}

	client, err := outbound.DesktopUpdate("https://feed.example.test/feed.json", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://other.example.test/x", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	if res, err := client.Do(
		req,
	); !errors.Is(
		err,
		outbound.ErrHost,
	) {
		if res != nil {
			_ = res.Body.Close()
		}

		t.Errorf("another host: got %v, want ErrHost", err)
	}
}
