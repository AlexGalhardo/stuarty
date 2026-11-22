package outbound

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookClientRefusesAnAddressThatIsNotTheWebhook(t *testing.T) {
	t.Parallel()

	c, err := Webhook("https://hooks.example.com/a/b?token=1", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{
		"https://other.example.com/a/b",
		"http://hooks.example.com/a/b",
		"https://hooks.example.com:8443/a/b",
		"https://user:pw@hooks.example.com/a/b",
		"https://127.0.0.1/a/b",
	} {
		req, _ := http.NewRequest(http.MethodPost, target, strings.NewReader("{}"))

		if resp, err := c.Do(req); err == nil || !errors.Is(err, ErrWebhookURL) {
			if resp != nil {
				_ = resp.Body.Close()
			}

			t.Errorf("%s: err = %v, want ErrWebhookURL", target, err)
		}
	}
}

func TestWebhookClientRefusesAnAddressThatIsNotAllowed(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"http://hooks.example.com/a", "ftp://x/a", "https://u:p@x.example/a", "://", "https:///a"} {
		if _, err := Webhook(raw, time.Second, nil); !errors.Is(err, ErrWebhookURL) {
			t.Errorf("Webhook(%q) = %v", raw, err)
		}
	}

	for _, raw := range []string{"https://hooks.example.com/a", "http://localhost:9/a", "http://127.0.0.1:9/a", "http://[::1]:9/a"} {
		if _, err := Webhook(raw, time.Second, nil); err != nil {
			t.Errorf("Webhook(%q) = %v", raw, err)
		}
	}
}

// T-110: a webhook that is not on this machine reaches no address of this machine or of a private network, and a
// webhook that is on this machine reaches loopback only.
func TestSecurity_T110_WebhookDialRefusesInternalRanges(t *testing.T) {
	t.Parallel()

	internal := []string{
		"127.0.0.1:443", "[::1]:443", "10.0.0.5:443", "172.16.3.4:443", "192.168.1.1:443", "169.254.169.254:80",
		"100.64.0.1:443", "100.100.100.200:80", "0.0.0.0:80", "[fe80::1]:443", "[fd00::1]:443", "[fd00:ec2::254]:80",
		"[::ffff:10.0.0.1]:443", "[::ffff:169.254.169.254]:80", "[64:ff9b::a00:1]:443", "[2002:0a00:0001::]:443",
		"224.0.0.1:443", "not an address",
		// The blocks no public receiver is on, also inside an IPv6 form.
		"0.1.2.3:443", "192.0.0.8:443", "198.18.0.1:443", "198.19.255.1:443", "240.0.0.1:443", "255.255.255.255:443",
		"[fec0::1]:443", "[::ffff:198.18.0.1]:443", "[64:ff9b::c612:1]:443", "[2002:c612:0001::]:443",
	}

	for _, addr := range internal {
		if err := webhookDialCheck(false, addr); !errors.Is(err, ErrNotPublic) {
			t.Errorf("a public webhook dialled %s: %v", addr, err)
		}
	}

	for _, addr := range []string{"93.184.216.34:443", "[2606:2800:220:1:248:1893:25c8:1946]:443"} {
		if err := webhookDialCheck(false, addr); err != nil {
			t.Errorf("a public webhook was refused %s: %v", addr, err)
		}
	}

	for _, addr := range []string{"127.0.0.1:9000", "[::1]:9000", "[::ffff:127.0.0.1]:9000"} {
		if err := webhookDialCheck(true, addr); err != nil {
			t.Errorf("a loopback webhook was refused %s: %v", addr, err)
		}
	}

	for _, addr := range []string{"10.0.0.5:9000", "93.184.216.34:443", "169.254.169.254:80", "[fd00::1]:443"} {
		if err := webhookDialCheck(true, addr); !errors.Is(err, ErrNotLoopback) {
			t.Errorf("a loopback webhook dialled %s: %v", addr, err)
		}
	}

	if !NotPublicAddr(netip.MustParseAddr("10.1.2.3")) || NotPublicAddr(netip.MustParseAddr("93.184.216.34")) {
		t.Error("NotPublicAddr does not follow the ranges")
	}
}

// T-111: a name that was public when the webhook was saved and is internal when it is dialled reaches nothing.
func TestSecurity_T111_WebhookNameThatTurnsInternalReachesNothing(t *testing.T) {
	var reached atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// The name "rebind.example" is dialled at the address of a server on this machine, which is what a DNS answer
	// of 127.0.0.1 for a public name would be. The address of the server is a plain http one, so the scheme rule is
	// met by a loopback webhook in the control case below, and the name is refused as a public one.
	_, port, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")

	for _, internalIP := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254"} {
		c, err := Webhook("https://rebind.example:"+port+"/h", time.Second, &TestOptions{
			Resolve: map[string]string{"rebind.example": internalIP},
		})
		if err != nil {
			t.Fatal(err)
		}

		req, _ := http.NewRequest(http.MethodPost, "https://rebind.example:"+port+"/h", strings.NewReader("{}"))

		resp, err := c.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}

		if !errors.Is(err, ErrNotPublic) {
			t.Errorf("%s: err = %v, want ErrNotPublic", internalIP, err)
		}
	}

	if n := reached.Load(); n != 0 {
		t.Fatalf("the server on this machine was reached %d times", n)
	}

	// Control: the same server is reached when the webhook is on this machine on purpose.
	c, err := Webhook(srv.URL+"/h", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/h", strings.NewReader("{}"))

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("a loopback webhook was not delivered: %v", err)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if reached.Load() != 1 {
		t.Fatalf("the control request was not received")
	}
}

func TestWebhookClientFollowsNoRedirect(t *testing.T) {
	t.Parallel()

	var other atomic.Int32

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { other.Add(1) }))
	defer target.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	c, err := Webhook(srv.URL+"/h", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/h", strings.NewReader("{}"))

	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusFound || other.Load() != 0 {
		t.Fatalf("status %d, redirect target reached %d times", resp.StatusCode, other.Load())
	}
}

func TestWebhookDialsAreSeenByTheRecorder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	var seen []Purpose

	restore := SetDialRecorder(func(p Purpose, _ string) error {
		seen = append(seen, p)

		return ErrNotLoopback
	})
	defer restore()

	c, err := Webhook(srv.URL+"/h", time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/h", strings.NewReader("{}"))

	resp, err := c.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil || len(seen) != 1 || seen[0] != PurposeWebhook {
		t.Fatalf("err = %v, seen = %v", err, seen)
	}
}
