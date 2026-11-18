package update_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/desktop/update"
	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

const (
	running  = "0.14.0"
	rfc3339  = time.RFC3339
	platform = "windows-amd64"
	fileName = "vukem-video_0.15.0_windows_amd64_setup.exe"
)

// clock is the fixed "now" of every test, so the freshness window is deterministic.
var clock = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// None of these tests runs in parallel: the dial recorder of internal/outbound is global.

func testKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()

	seed, err := os.ReadFile(filepath.Join("testdata", "test-key.seed"))
	if err != nil {
		t.Fatal(err)
	}

	raw, err := hex.DecodeString(strings.TrimSpace(string(seed)))
	if err != nil {
		t.Fatal(err)
	}

	return ed25519.NewKeyFromSeed(raw)
}

func TestTestKeyFixtureIsConsistent(t *testing.T) {
	pub, err := os.ReadFile(filepath.Join("testdata", "test-key.pub"))
	if err != nil {
		t.Fatal(err)
	}

	got := hex.EncodeToString(testKey(t).Public().(ed25519.PublicKey))
	if got != strings.TrimSpace(string(pub)) {
		t.Fatalf("test-key.pub is %s, the seed gives %s", strings.TrimSpace(string(pub)), got)
	}
}

type installerFake struct {
	calls   []update.Artifact
	content [][]byte
	err     error
}

func (f *installerFake) Install(_ context.Context, a update.Artifact) error {
	f.calls = append(f.calls, a)

	b, err := os.ReadFile(a.Path)
	if err != nil {
		return err
	}

	f.content = append(f.content, b)

	return f.err
}

// scenario describes what the local feed serves. The zero value is a valid update.
type scenario struct {
	feedVersion    string              // default 0.15.0
	feedKey        ed25519.PrivateKey  // key that signs the feed, default the test key
	artifactKey    ed25519.PrivateKey  // key that signs the artifact, default the test key
	sigVersion     string              // version in the signed artifact message, default feedVersion
	sigPlatform    string              // platform in the signed artifact message, default platform
	declaredSHA    []byte              // bytes whose digest the feed declares, default the artifact
	declaredSize   int64               // default len(artifact)
	served         []byte              // bytes the server sends, default the artifact
	truncateAt     int                 // when > 0 the server promises the full length and sends this many bytes
	artifactHost   string              // host for the artifact URL, default the server
	feedRedirectTo string              // when set, the feed answers a redirect there
	publishedAt    string              // default clock-1h; "-" leaves the field out of the signed payload
	expires        string              // default clock+30d; "-" leaves the field out of the signed payload
	notes          string              // optional release notes in the signed payload
	mutateFeed     func([]byte) []byte // rewrites the served feed after it was signed
}

type env struct {
	srv       *httptest.Server
	cfg       update.Config
	installer *installerFake
	feedHits  atomic.Int32
	fileHits  atomic.Int32
	artifact  []byte
}

func newEnv(t *testing.T, sc scenario) *env {
	t.Helper()

	e := &env{installer: &installerFake{}, artifact: bytes.Repeat([]byte("vukem-video installer bytes\n"), 200)}
	key := testKey(t)

	applyDefaults(&sc, e, key)

	mux := http.NewServeMux()
	mux.HandleFunc("/feed.json", func(w http.ResponseWriter, r *http.Request) {
		e.feedHits.Add(1)

		if sc.feedRedirectTo != "" {
			http.Redirect(w, r, sc.feedRedirectTo, http.StatusFound)

			return
		}

		body := e.feedJSON(t, sc)
		if sc.mutateFeed != nil {
			body = sc.mutateFeed(body)
		}

		_, _ = w.Write(body)
	})
	mux.HandleFunc("/"+fileName, func(w http.ResponseWriter, _ *http.Request) {
		e.fileHits.Add(1)
		serve(w, sc)
	})

	e.srv = httptest.NewTLSServer(mux)
	t.Cleanup(e.srv.Close)

	pool := x509.NewCertPool()
	pool.AddCert(e.srv.Certificate())

	e.cfg = update.Config{
		FeedURL:        e.srv.URL + "/feed.json",
		PublicKey:      key.Public().(ed25519.PublicKey),
		CurrentVersion: running,
		Platform:       platform,
		StagingDir:     filepath.Join(t.TempDir(), "staging"),
		AutoCheck:      true,
		Installer:      e.installer,
		Now:            func() time.Time { return clock },
		Test:           &outbound.TestOptions{RootCAs: pool},
	}

	return e
}

func applyDefaults(sc *scenario, e *env, key ed25519.PrivateKey) {
	if sc.feedVersion == "" {
		sc.feedVersion = "0.15.0"
	}

	if sc.feedKey == nil {
		sc.feedKey = key
	}

	if sc.artifactKey == nil {
		sc.artifactKey = key
	}

	if sc.sigVersion == "" {
		sc.sigVersion = sc.feedVersion
	}

	if sc.sigPlatform == "" {
		sc.sigPlatform = platform
	}

	if sc.declaredSHA == nil {
		sc.declaredSHA = e.artifact
	}

	if sc.declaredSize == 0 {
		sc.declaredSize = int64(len(e.artifact))
	}

	if sc.served == nil {
		sc.served = e.artifact
	}
}

func (e *env) feedJSON(t *testing.T, sc scenario) []byte {
	t.Helper()

	sum := sha256.Sum256(sc.declaredSHA)
	digest := hex.EncodeToString(sum[:])
	host := e.srv.URL

	if sc.artifactHost != "" {
		host = sc.artifactHost
	}

	fields := map[string]any{
		"schema":  1,
		"version": sc.feedVersion,
		"notes":   sc.notes,
		"platforms": map[string]any{
			platform: map[string]any{
				"url":    host + "/" + fileName,
				"sha256": digest,
				"size":   sc.declaredSize,
				"signature": base64.StdEncoding.EncodeToString(
					ed25519.Sign(sc.artifactKey, update.ArtifactMessage(sc.sigVersion, sc.sigPlatform, digest)),
				),
			},
		},
	}

	if sc.publishedAt != "-" {
		fields["published_at"] = orDefault(sc.publishedAt, clock.Add(-time.Hour).Format(rfc3339))
	}

	if sc.expires != "-" {
		fields["expires"] = orDefault(sc.expires, clock.Add(30*24*time.Hour).Format(rfc3339))
	}

	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}

	out, err := json.Marshal(map[string]string{
		"payload":   base64.StdEncoding.EncodeToString(body),
		"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(sc.feedKey, update.FeedMessage(body))),
	})
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}

	return v
}

func serve(w http.ResponseWriter, sc scenario) {
	if sc.truncateAt <= 0 {
		_, _ = w.Write(sc.served)

		return
	}

	// Promise the whole body, send part of it and drop the connection.
	w.Header().Set("Content-Length", "99999999")
	_, _ = w.Write(sc.served[:sc.truncateAt])

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	if hj, ok := w.(http.Hijacker); ok {
		if conn, _, err := hj.Hijack(); err == nil {
			_ = conn.Close()
		}
	}
}

func (e *env) client(t *testing.T) *update.Client {
	t.Helper()

	c, err := update.New(e.cfg)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func (e *env) stagedFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(e.cfg.StagingDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, d := range entries {
		names = append(names, d.Name())
	}

	return names
}

// checkAndApply runs the whole flow and returns the first error.
func (e *env) checkAndApply(t *testing.T) error {
	t.Helper()

	c := e.client(t)

	offer, err := c.CheckNow(t.Context())
	if err != nil {
		return err
	}

	return c.Apply(t.Context(), offer)
}

func TestUpdate_ValidUpdate(t *testing.T) {
	e := newEnv(t, scenario{})

	if err := e.checkAndApply(t); err != nil {
		t.Fatalf("a valid update failed: %v", err)
	}

	if len(e.installer.calls) != 1 {
		t.Fatalf("the installer was called %d times, want 1", len(e.installer.calls))
	}

	if got := e.installer.calls[0].Version; got != "0.15.0" {
		t.Errorf("the installer got version %q, want 0.15.0", got)
	}

	if !bytes.Equal(e.installer.content[0], e.artifact) {
		t.Error("the installer got a file that is not the artifact")
	}

	if filepath.Base(e.installer.calls[0].Path) != fileName {
		t.Errorf("the installer path is %q", e.installer.calls[0].Path)
	}
}

func TestUpdate_WrongDigest(t *testing.T) {
	e := newEnv(t, scenario{declaredSHA: []byte("some other file")})

	err := e.checkAndApply(t)
	if !errors.Is(err, update.ErrDigestMismatch) {
		t.Fatalf("got %v, want ErrDigestMismatch", err)
	}

	assertNeverInstalled(t, e)
}

func TestUpdate_WrongSignature(t *testing.T) {
	_, other, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	e := newEnv(t, scenario{artifactKey: other})

	err = e.checkAndApply(t)
	if !errors.Is(err, update.ErrBadArtifactSignature) {
		t.Fatalf("got %v, want ErrBadArtifactSignature", err)
	}

	assertNeverInstalled(t, e)
}

func TestUpdate_FeedFreshness(t *testing.T) {
	day := 24 * time.Hour
	at := func(d time.Duration) string { return clock.Add(d).Format(rfc3339) }

	cases := []struct {
		name string
		sc   scenario
		want error // nil means the update is offered
	}{
		{"fresh feed", scenario{}, nil},
		{"expired feed", scenario{expires: at(-time.Minute)}, update.ErrStaleFeed},
		{"expires exactly now", scenario{expires: at(0)}, update.ErrStaleFeed},
		{
			"replay of an old signed feed",
			scenario{publishedAt: at(-200 * day), expires: at(-20 * day)},
			update.ErrStaleFeed,
		},
		{"no published_at", scenario{publishedAt: "-"}, update.ErrBadFeed},
		{"no expires", scenario{expires: "-"}, update.ErrBadFeed},
		{"legacy feed without both", scenario{publishedAt: "-", expires: "-"}, update.ErrBadFeed},
		{"unparseable expires", scenario{expires: "next week"}, update.ErrBadFeed},
		{
			"expires before published_at",
			scenario{publishedAt: at(-time.Hour), expires: at(-2 * time.Hour)},
			update.ErrBadFeed,
		},
		{
			"window beyond the maximum",
			scenario{publishedAt: at(-time.Hour), expires: at(2000 * day)},
			update.ErrBadFeed,
		},
		{"published in the future", scenario{publishedAt: at(time.Hour), expires: at(30 * day)}, update.ErrBadFeed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, c.sc)
			offer, err := e.client(t).CheckNow(context.Background())

			if c.want == nil {
				if err != nil || offer == nil {
					t.Fatalf("a fresh feed was refused: %v", err)
				}

				return
			}

			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}

			assertNeverInstalled(t, e)
		})
	}
}

func TestUpdate_OlderVersion(t *testing.T) {
	for _, v := range []string{"0.13.9", "0.14.0"} {
		e := newEnv(t, scenario{feedVersion: v})

		_, err := e.client(t).CheckNow(t.Context())
		if !errors.Is(err, update.ErrNotNewer) {
			t.Fatalf("feed %s, running %s: got %v, want ErrNotNewer", v, running, err)
		}

		if e.fileHits.Load() != 0 {
			t.Errorf("feed %s: the artifact was requested", v)
		}

		assertNeverInstalled(t, e)
	}
}

func TestUpdate_InterruptedDownload(t *testing.T) {
	e := newEnv(t, scenario{truncateAt: 1000})

	err := e.checkAndApply(t)
	if !errors.Is(err, update.ErrIncompleteDownload) {
		t.Fatalf("got %v, want ErrIncompleteDownload", err)
	}

	assertNeverInstalled(t, e)
}

func TestUpdate_FailedInstallLeavesTheRunningVersionUsable(t *testing.T) {
	e := newEnv(t, scenario{})
	e.installer.err = errors.New("the installer exited with code 5")

	err := e.checkAndApply(t)
	if !errors.Is(err, update.ErrInstallFailed) || !strings.Contains(err.Error(), "code 5") {
		t.Fatalf("got %v, want ErrInstallFailed carrying the reason", err)
	}

	if files := e.stagedFiles(t); len(files) != 0 {
		t.Errorf("the staging directory still holds %v", files)
	}
}

func TestUpdate_FeedSignedByAnotherKeyIsRefused(t *testing.T) {
	_, other, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	e := newEnv(t, scenario{feedKey: other})

	_, err = e.client(t).CheckNow(t.Context())
	if !errors.Is(err, update.ErrBadFeedSignature) {
		t.Fatalf("got %v, want ErrBadFeedSignature", err)
	}

	if e.fileHits.Load() != 0 {
		t.Error("the artifact was requested after a bad feed signature")
	}
}

func TestUpdate_NotConfiguredBuildRefuses(t *testing.T) {
	if _, err := update.FromBuild(
		running,
		t.TempDir(),
		true,
		&installerFake{},
	); !errors.Is(
		err,
		update.ErrNotConfigured,
	) {
		t.Errorf("FromBuild with no key: got %v, want ErrNotConfigured", err)
	}

	cfg := newEnv(t, scenario{}).cfg
	cfg.FeedURL = update.FeedURL

	if _, err := update.New(cfg); !errors.Is(err, update.ErrNotConfigured) {
		t.Errorf("New with the placeholder URL: got %v, want ErrNotConfigured", err)
	}
}

func TestUpdate_RunningDevelopmentBuildDoesNotCheck(t *testing.T) {
	cfg := newEnv(t, scenario{}).cfg
	cfg.CurrentVersion = "dev"

	if _, err := update.New(cfg); err == nil {
		t.Fatal("a development build must not check for updates")
	}
}

func assertNeverInstalled(t *testing.T, e *env) {
	t.Helper()

	if n := len(e.installer.calls); n != 0 {
		t.Fatalf("the installer was reached %d time(s), want 0", n)
	}

	if files := e.stagedFiles(t); len(files) != 0 {
		t.Errorf("the staging directory still holds %v", files)
	}
}

func TestSecurity_T35_TamperedUpdateNeverReachesTheInstaller(t *testing.T) {
	_, other, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		sc   scenario
		want error
	}{
		{"digest of another file", scenario{declaredSHA: []byte("another file")}, update.ErrDigestMismatch},
		{"signature of another key", scenario{artifactKey: other}, update.ErrBadArtifactSignature},
		{"signature made for another platform", scenario{sigPlatform: "linux-amd64"}, update.ErrBadArtifactSignature},
		{"signature made for an older version", scenario{sigVersion: "0.14.5"}, update.ErrBadArtifactSignature},
		{"download cut short", scenario{truncateAt: 500}, update.ErrIncompleteDownload},
		{
			"download longer than declared",
			scenario{served: bytes.Repeat([]byte("x"), 6000), declaredSize: 100},
			update.ErrIncompleteDownload,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.sc)

			if err := e.checkAndApply(t); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}

			assertNeverInstalled(t, e)
		})
	}
}

func TestSecurity_T35_UpdateChannelIsClosedWithTheSettingOff(t *testing.T) {
	e := newEnv(t, scenario{})
	e.cfg.AutoCheck = false

	var dials atomic.Int32

	restore := outbound.SetDialRecorder(func(outbound.Purpose, string) error {
		dials.Add(1)

		return nil
	})
	defer restore()

	c := e.client(t)

	if _, err := c.CheckOnStartup(t.Context()); !errors.Is(err, update.ErrDisabled) {
		t.Fatalf("start-up check with the setting off: got %v, want ErrDisabled", err)
	}

	if dials.Load() != 0 || e.feedHits.Load() != 0 {
		t.Fatalf("the setting is off yet %d dial(s) and %d feed request(s) happened", dials.Load(), e.feedHits.Load())
	}

	// The person asking is the other trigger: it opens exactly the feed connection.
	if _, err := c.CheckNow(t.Context()); err != nil {
		t.Fatal(err)
	}

	if dials.Load() != 1 || e.feedHits.Load() != 1 {
		t.Errorf(
			"a requested check made %d dial(s) and %d feed request(s), want 1 and 1",
			dials.Load(),
			e.feedHits.Load(),
		)
	}
}

func TestSecurity_T35_UpdateReachesTheFeedHostOnly(t *testing.T) {
	t.Run("artifact on another host", func(t *testing.T) {
		e := newEnv(t, scenario{artifactHost: "https://127.0.0.2:1"})

		var otherDials atomic.Int32

		restore := outbound.SetDialRecorder(func(_ outbound.Purpose, address string) error {
			if strings.HasPrefix(address, "127.0.0.2:") {
				otherDials.Add(1)
			}

			return nil
		})
		defer restore()

		err := e.checkAndApply(t)
		if !errors.Is(err, outbound.ErrHost) {
			t.Fatalf("got %v, want outbound.ErrHost", err)
		}

		if otherDials.Load() != 0 {
			t.Error("the client dialled a host that is not the feed host")
		}

		assertNeverInstalled(t, e)
	})

	t.Run("redirect is not followed", func(t *testing.T) {
		target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("the redirect target was reached")
		}))
		defer target.Close()

		e := newEnv(t, scenario{feedRedirectTo: target.URL + "/feed.json"})

		if _, err := e.client(t).CheckNow(t.Context()); err == nil {
			t.Fatal("a redirected feed was accepted")
		}
	})

	t.Run("plain http feed URL", func(t *testing.T) {
		cfg := newEnv(t, scenario{}).cfg
		cfg.FeedURL = "http://127.0.0.1:1/feed.json"

		if _, err := update.New(cfg); !errors.Is(err, outbound.ErrScheme) {
			t.Fatalf("got %v, want outbound.ErrScheme", err)
		}
	})
}

func TestOfferCarriesTheNotesOfTheSignedFeed(t *testing.T) {
	e := newEnv(t, scenario{notes: "Faster export. Fixes a crash."})

	offer, err := e.client(t).CheckNow(context.Background())
	if err != nil || offer.Notes != "Faster export. Fixes a crash." {
		t.Fatalf("offer %+v, err %v", offer, err)
	}
}

func TestOfferWithoutNotesHasNone(t *testing.T) {
	offer, err := newEnv(t, scenario{}).client(t).CheckNow(context.Background())
	if err != nil || offer.Notes != "" {
		t.Fatalf("offer %+v, err %v", offer, err)
	}
}

func TestNotesAreCutOnACharacterBoundary(t *testing.T) {
	long := strings.Repeat("é", 3000) // 6000 bytes
	e := newEnv(t, scenario{notes: long})

	offer, err := e.client(t).CheckNow(context.Background())
	if err != nil || len(offer.Notes) > 4096 || !utf8.ValidString(offer.Notes) || len(offer.Notes) < 4000 {
		t.Fatalf("notes of %d bytes, err %v", len(offer.Notes), err)
	}
}

func TestApplyReportsProgressUpToTheDeclaredSize(t *testing.T) {
	e := newEnv(t, scenario{})

	var last, total int64

	calls := 0
	e.cfg.Progress = func(done, size int64) { last, total = done, size; calls++ }

	c := e.client(t)

	offer, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Apply(context.Background(), offer); err != nil {
		t.Fatal(err)
	}

	if last != total || total != int64(len(e.artifact)) || calls < 1 || calls > 100 {
		t.Errorf("last %d total %d calls %d", last, total, calls)
	}
}

// The attacker holds the feed host but not the key: the payload is rewritten after it was signed, to
// offer a far newer version at the attacker's own URL.
func TestSecurity_T35_ForgedFeedPayloadIsNeverReadOrDownloaded(t *testing.T) {
	forge := func(raw []byte) []byte {
		var env map[string]string
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}

		payload, err := base64.StdEncoding.DecodeString(env["payload"])
		if err != nil {
			t.Fatal(err)
		}

		payload = bytes.Replace(payload, []byte(`"version":"0.15.0"`), []byte(`"version":"9.9.9"`), 1)
		env["payload"] = base64.StdEncoding.EncodeToString(payload)

		out, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}

		return out
	}

	e := newEnv(t, scenario{mutateFeed: forge})

	_, err := e.client(t).CheckNow(t.Context())
	if !errors.Is(err, update.ErrBadFeedSignature) {
		t.Fatalf("got %v, want ErrBadFeedSignature", err)
	}

	if e.fileHits.Load() != 0 {
		t.Error("a forged feed must never lead to a download")
	}

	assertNeverInstalled(t, e)
}

// A validly signed older release replayed as the newest, and the same version again, are not updates.
func TestSecurity_T35_ReplayedSignedReleaseIsNotAnUpdate(t *testing.T) {
	for _, v := range []string{"0.13.0", "0.14.0"} {
		e := newEnv(t, scenario{feedVersion: v})
		e.cfg.CurrentVersion = running

		c := e.client(t)

		if _, err := c.CheckNow(t.Context()); !errors.Is(err, update.ErrNotNewer) {
			t.Fatalf("replayed %s: got %v, want ErrNotNewer", v, err)
		}

		if e.fileHits.Load() != 0 {
			t.Errorf("replayed %s: the artifact was requested", v)
		}

		assertNeverInstalled(t, e)
	}
}
