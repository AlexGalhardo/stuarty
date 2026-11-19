package outbound

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRegistry struct {
	mu    sync.Mutex
	dests map[string]Destination
	used  []string
}

func (r *fakeRegistry) Destination(_ context.Context, id string) (Destination, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.dests[id]
	if !ok {
		return Destination{}, ErrNoConnection
	}

	return d, nil
}

func (r *fakeRegistry) Used(_ context.Context, id string, _ time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.used = append(r.used, id)
}

func (r *fakeRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.dests, id)
}

// destination is a fake server that records what it receives.
type destination struct {
	srv  *httptest.Server
	mu   sync.Mutex
	seen []string
}

func newDestination(t *testing.T) *destination {
	t.Helper()

	d := &destination{}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.seen = append(d.seen, r.URL.Path+" "+r.Header.Get("Authorization"))
		d.mu.Unlock()

		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)

			return
		}

		_, _ = io.WriteString(w, "hello")
	}))

	t.Cleanup(d.srv.Close)

	return d
}

func (d *destination) hits() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.seen)
}

func (d *destination) host() string {
	u, _ := url.Parse(d.srv.URL)

	return u.Hostname()
}

// get makes a GET through c, closes the body and returns the status and the body.
func get(c *http.Client, target string) (status int, body string, err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
	if err != nil {
		return 0, "", err
	}

	res, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}

	defer func() { _ = res.Body.Close() }()

	raw, _ := io.ReadAll(res.Body)

	return res.StatusCode, string(raw), nil
}

func TestConnectionReachesItsHostAndRecordsTheUse(t *testing.T) {
	dst := newDestination(t)
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{dst.host()}}}}

	var purposes []Purpose

	restore := SetDialRecorder(func(p Purpose, _ string) error {
		purposes = append(purposes, p)

		return nil
	})
	defer restore()

	_, body, err := get(Connection(reg, "con_a", 5*time.Second, nil), dst.srv.URL+"/ping")
	if err != nil {
		t.Fatal(err)
	}

	if body != "hello" || dst.hits() != 1 {
		t.Fatalf("body %q, hits %d", body, dst.hits())
	}

	if len(reg.used) != 1 || reg.used[0] != "con_a" {
		t.Errorf("Used = %v", reg.used)
	}

	if len(purposes) != 1 || purposes[0] != PurposeConnection {
		t.Errorf("the dial recorder saw %v", purposes)
	}
}

func TestConnectionRefusesAnotherHostBeforeAnySocket(t *testing.T) {
	dst := newDestination(t)
	other := newDestination(t)
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{dst.host()}}}}

	dials := 0
	restore := SetDialRecorder(func(Purpose, string) error { dials++; return nil })

	defer restore()

	client := Connection(reg, "con_a", 5*time.Second, nil)

	// "localhost" reaches the same machine but is not the listed host.
	_, port, _ := hostAndPort(other.srv.URL)

	for _, target := range []string{
		"http://localhost:" + port + "/x",
		"https://example.com/x",
		"https://user:pass@" + dst.host() + "/x",
		"ftp://" + dst.host() + "/x",
		"file:///etc/passwd",
	} {
		_, _, err := get(client, target)
		if !errors.Is(err, ErrDestination) {
			t.Errorf("%s: %v, want ErrDestination", target, err)
		}
	}

	if dials != 0 || dst.hits() != 0 || other.hits() != 0 {
		t.Errorf("a refused request reached the network: dials %d, hits %d and %d", dials, dst.hits(), other.hits())
	}

	if len(reg.used) != 0 {
		t.Errorf("a refused request was recorded as a use: %v", reg.used)
	}
}

func TestConnectionRefusesPlainHTTPToAPublicHost(t *testing.T) {
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{"api.example.com"}}}}

	_, _, err := get(Connection(reg, "con_a", time.Second, nil), "http://api.example.com/x")
	if !errors.Is(err, ErrDestination) {
		t.Fatalf("%v, want ErrDestination", err)
	}
}

func TestConnectionHostMatchIsCaseInsensitive(t *testing.T) {
	u, _ := url.Parse("https://API.Example.com./x")

	if err := checkDestination(u, []string{"api.example.com"}); err != nil {
		t.Fatalf("%v", err)
	}

	if err := checkDestination(u, []string{"example.com"}); !errors.Is(err, ErrDestination) {
		t.Fatalf("a parent domain matched: %v", err)
	}
}

func TestConnectionStopsAtOnceWhenRemoved(t *testing.T) {
	dst := newDestination(t)
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{dst.host()}}}}
	client := Connection(reg, "con_a", 5*time.Second, nil)

	if _, _, err := get(client, dst.srv.URL+"/one"); err != nil {
		t.Fatal(err)
	}

	reg.remove("con_a")

	if _, _, err := get(client, dst.srv.URL+"/two"); !errors.Is(err, ErrNoConnection) {
		t.Fatalf("after removal: %v, want ErrNoConnection", err)
	}

	if dst.hits() != 1 {
		t.Errorf("the destination saw %d requests, want 1", dst.hits())
	}
}

func TestConnectionFollowsNoRedirect(t *testing.T) {
	dst := newDestination(t)
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{dst.host()}}}}

	status, _, err := get(Connection(reg, "con_a", 5*time.Second, nil), dst.srv.URL+"/redirect")
	if err != nil {
		t.Fatal(err)
	}

	if status != http.StatusFound || dst.hits() != 1 {
		t.Errorf("status %d, hits %d: the redirect was followed", status, dst.hits())
	}
}

func TestConnectionRefusesAMetadataAddressAtDial(t *testing.T) {
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{"169.254.169.254"}}}}

	dials := 0
	restore := SetDialRecorder(func(Purpose, string) error { dials++; return nil })

	defer restore()

	_, _, err := get(Connection(reg, "con_a", 2*time.Second, nil), "https://169.254.169.254/latest/meta-data/")
	if !errors.Is(err, ErrNotPublic) {
		t.Fatalf("%v, want ErrNotPublic", err)
	}

	if dials != 0 {
		t.Errorf("the recorder saw %d dials of a refused address", dials)
	}
}

func TestConnectionDialRecorderCanStopTheAttempt(t *testing.T) {
	dst := newDestination(t)
	reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{dst.host()}}}}

	stop := errors.New("stop")
	restore := SetDialRecorder(func(Purpose, string) error { return stop })

	defer restore()

	if _, _, err := get(Connection(reg, "con_a", 2*time.Second, nil), dst.srv.URL); !errors.Is(err, stop) {
		t.Fatalf("%v, want the recorder error", err)
	}

	if dst.hits() != 0 {
		t.Error("the destination was reached")
	}
}

// hostAndPort is the host and the port of a URL.
func hostAndPort(raw string) (host, port string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}

	return u.Hostname(), u.Port(), nil
}

// TestConnectionRefusesNamedCloudMetadataAddressesAtDial: the metadata services that sit in ranges
// the gate allows on purpose (the CGNAT block and the unique-local block) are refused by address.
func TestConnectionRefusesNamedCloudMetadataAddressesAtDial(t *testing.T) {
	for _, host := range []string{"100.100.100.200", "fd00:ec2::254", "::ffff:100.100.100.200"} {
		t.Run(host, func(t *testing.T) {
			reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{host}}}}

			dials := 0
			restore := SetDialRecorder(func(Purpose, string) error { dials++; return nil })

			defer restore()

			_, _, err := get(Connection(reg, "con_a", 2*time.Second, nil), "https://["+host+"]/latest/meta-data/")
			if !errors.Is(err, ErrNotPublic) {
				t.Fatalf("%v, want ErrNotPublic", err)
			}

			if dials != 0 {
				t.Errorf("the recorder saw %d dials of a refused address", dials)
			}
		})
	}
}

// TestConnectionStillAllowsCGNATAndUniqueLocalNeighbours: only the named addresses are refused.
func TestConnectionStillAllowsCGNATAndUniqueLocalNeighbours(t *testing.T) {
	for _, host := range []string{"100.100.100.201", "fd00:ec2::255", "10.0.0.1"} {
		reg := &fakeRegistry{dests: map[string]Destination{"con_a": {ID: "con_a", Hosts: []string{host}}}}
		stop := errors.New("stop")
		restore := SetDialRecorder(func(Purpose, string) error { return stop })

		authority := host
		if strings.Contains(host, ":") {
			authority = "[" + host + "]"
		}

		_, _, err := get(Connection(reg, "con_a", 2*time.Second, nil), "https://"+authority+"/")

		restore()

		if !errors.Is(err, stop) {
			t.Errorf("%s: %v, want the gate to let the dial reach the recorder", host, err)
		}
	}
}
