package connections

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// SDD-007 T-41 (ADR-0032, RF-206): a request through a connection reaches the hosts of that
// connection and no other, follows no redirect to another host, carries the credential nowhere else,
// and stops the moment the connection is removed. Nothing is dialled for a refused request.
func TestSecurity_T41_ConnectionRefusesOtherHostAndRemoved(t *testing.T) {
	// The destination answers a redirect to "localhost", the same machine under a name the
	// connection does not list: following it would send the credential to another host.
	var (
		mu       sync.Mutex
		received []string
	)

	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Get("Authorization"))
		mu.Unlock()
	}))
	defer other.Close()

	otherURL, _ := url.Parse(other.URL)

	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://localhost:"+otherURL.Port()+"/steal", http.StatusFound)

			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer dst.Close()

	m, st, v := newManager(t)
	srv := &server{srv: dst}
	connect(t, m, st, v, srv, "tok-CANARY")

	dials := 0
	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error { dials++; return nil })

	defer restore()

	do := func(c Doer, target string) (int, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
		if err != nil {
			return 0, err
		}

		req.Header.Set("Authorization", "Bearer tok-CANARY")

		res, err := c.Do(req)
		if err != nil {
			return 0, err
		}

		_ = res.Body.Close()

		return res.StatusCode, nil
	}

	var client Doer

	if err := m.With(
		context.Background(),
		"con_a",
		func(c Credential) error { client = c.Client; return nil },
	); err != nil {
		t.Fatal(err)
	}

	// The host of the connection works.
	if status, err := do(client, dst.URL+"/ok"); err != nil || status != http.StatusOK {
		t.Fatalf("the listed host: %d, %v", status, err)
	}

	dialsBefore := dials

	// Another host, another scheme, userinfo: refused, nothing dialled.
	for _, target := range []string{
		"http://localhost:" + otherURL.Port() + "/x",
		"https://evil.example.com/x",
		"https://169.254.169.254/latest/meta-data/",
		"http://127.0.0.2:" + otherURL.Port() + "/x",
		"ftp://" + otherURL.Hostname() + "/x",
		"http://user:pass@" + otherURL.Host + "/x",
	} {
		if _, err := do(
			client,
			target,
		); !errors.Is(err, outbound.ErrDestination) &&
			!errors.Is(err, outbound.ErrNotPublic) {
			t.Errorf("%s: %v, want a refusal of the gate", target, err)
		}
	}

	if dials != dialsBefore {
		t.Errorf("a refused request opened %d connection(s)", dials-dialsBefore)
	}

	// A redirect to another host is returned, not followed.
	if status, err := do(client, dst.URL+"/redirect"); err != nil || status != http.StatusFound {
		t.Fatalf("redirect: %d, %v", status, err)
	}

	mu.Lock()
	if len(received) != 0 {
		t.Errorf("the other host received %v", received)
	}
	mu.Unlock()

	// Removing the connection stops traffic at once, for a client already handed out.
	if err := st.Delete(context.Background(), "con_a"); err != nil {
		t.Fatal(err)
	}

	dialsBefore = dials

	if _, err := do(client, dst.URL+"/ok"); !errors.Is(err, outbound.ErrNoConnection) {
		t.Errorf("after removal: %v, want ErrNoConnection", err)
	}

	if dials != dialsBefore {
		t.Error("a request through a removed connection opened a connection")
	}
}
