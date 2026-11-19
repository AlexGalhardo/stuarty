package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// exchange is one recorded request and response pair of the YouTube API (NFR-064): the request is what
// the connector must send, the response what Google answered when it was recorded.
type exchange struct {
	Name    string `json:"name"`
	Request struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Query   map[string]string `json:"query"`
		Form    map[string]string `json:"form"`
		Headers map[string]string `json:"headers"`
		JSON    map[string]any    `json:"json"`
	} `json:"request"`
	Response struct {
		Status  int               `json:"status"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	} `json:"response"`
	Times int `json:"times"`
	used  int
}

func loadExchanges(t *testing.T, file string, names ...string) []*exchange {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}

	var doc struct {
		Exchanges []*exchange `json:"exchanges"`
	}

	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	if len(names) == 0 {
		return doc.Exchanges
	}

	var out []*exchange

	for _, n := range names {
		found := false

		for _, e := range doc.Exchanges {
			if e.Name == n {
				out = append(out, e)
				found = true
			}
		}

		if !found {
			t.Fatalf("no recorded pair %q in %s", n, file)
		}
	}

	return out
}

// replay serves recorded pairs and nothing else: a request no pair matches fails the test.
type replay struct {
	t   *testing.T
	srv *httptest.Server

	mu  sync.Mutex
	ex  []*exchange
	log []string
}

func newReplay(t *testing.T, sets ...[]*exchange) *replay {
	t.Helper()

	r := &replay{t: t}

	for _, s := range sets {
		r.ex = append(r.ex, s...)
	}

	r.srv = httptest.NewServer(http.HandlerFunc(r.handle))
	t.Cleanup(r.srv.Close)

	return r
}

func (r *replay) handle(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	_ = req.ParseForm()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.log = append(r.log, req.Method+" "+req.URL.Path)

	for _, e := range r.ex {
		times := max(e.Times, 1)
		if e.used >= times || !r.matches(e, req, body) {
			continue
		}

		e.used++

		for k, v := range e.Response.Headers {
			w.Header().Set(k, strings.ReplaceAll(v, "{{base}}", r.srv.URL))
		}

		w.WriteHeader(e.Response.Status)

		if len(e.Response.Body) > 0 && string(e.Response.Body) != `""` {
			_, _ = w.Write(e.Response.Body)
		}

		return
	}

	r.t.Errorf("no recorded pair for %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
	w.WriteHeader(599)
}

func (r *replay) matches(e *exchange, req *http.Request, body []byte) bool {
	if e.Request.Method != req.Method || e.Request.Path != req.URL.Path {
		return false
	}

	for k, v := range e.Request.Query {
		if req.URL.Query().Get(k) != v {
			return false
		}
	}

	for k, v := range e.Request.Form {
		if req.PostForm.Get(k) != v {
			return false
		}
	}

	for k, v := range e.Request.Headers {
		if req.Header.Get(k) != v {
			return false
		}
	}

	if len(e.Request.JSON) > 0 {
		var doc any
		if json.Unmarshal(body, &doc) != nil {
			return false
		}

		for path, want := range e.Request.JSON {
			got := doc

			for part := range strings.SplitSeq(path, ".") {
				m, ok := got.(map[string]any)
				if !ok {
					return false
				}

				got = m[part]
			}

			if !reflect.DeepEqual(got, want) {
				return false
			}
		}
	}

	return true
}

func (r *replay) count(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0

	for _, l := range r.log {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}

	return n
}

func (r *replay) connector() *Connector {
	return New(Config{
		APIBase: r.srv.URL, UploadBase: r.srv.URL + "/upload", TokenURL: r.srv.URL + "/token",
		Polls: 3, Resumes: 3, Wait: func(context.Context, time.Duration) {},
	})
}

func (r *replay) cred() connections.Credential {
	return connections.Credential{
		Secret: []byte(`{"client_id":"cid","client_secret":"csecret","refresh_token":"rt-1"}`), Client: r.srv.Client(),
	}
}

func video(t *testing.T) (path string, data []byte) {
	t.Helper()

	data = make([]byte, 2000)
	for i := range data {
		data[i] = byte(i % 251)
	}

	path = filepath.Join(t.TempDir(), "out.mp4")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path, data
}

func thumb(t *testing.T) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), "thumb.png")
	if err := os.WriteFile(p, []byte("\x89PNG-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	return p
}

func post() publish.Post {
	return publish.Post{
		Output: publish.File{
			Bytes:  2000,
			Format: "mp4",
		},
		Title:        "My video",
		Description:  "About it",
		Tags:         []string{"a", "b"},
		MadeForKids:  new(false),
		AIDisclosure: true,
	}
}

func upload(t *testing.T, path string, sessions social.Sessions) social.Upload {
	t.Helper()

	return social.Upload{Post: post(), Path: path, Key: "key-1", Sessions: sessions}
}

// Acceptance criterion 1 of epic 18, as far as the connector goes: the upload ends published with the id
// and the address, every request is one of the recorded pairs, and the post goes up private by default.
func TestUploadPublishesFromRecordedPairs(t *testing.T) {
	t.Parallel()

	r := newReplay(t, loadExchanges(t, "auth.json"), loadExchanges(t, "upload.json"))
	path, _ := video(t)
	u := upload(t, path, &social.MemorySessions{})
	u.ThumbnailPath = thumb(t)

	got, err := r.connector().Publish(context.Background(), r.cred(), u)
	if err != nil {
		t.Fatal(err)
	}

	if got.RemoteID != "vid123" || got.URL != "https://www.youtube.com/watch?v=vid123" ||
		got.State != publish.StatePublished ||
		got.ThumbnailError != "" {
		t.Fatalf("published = %+v", got)
	}

	if _, ok, _ := u.Sessions.Load("key-1"); ok {
		t.Error("the session is still kept after the upload ended")
	}

	for _, ex := range loadExchanges(t, "upload.json") {
		_ = ex
	}

	if r.count("PUT /upload/session/abc123") != 1 {
		t.Errorf("the file was sent %d times, want 1: %v", r.count("PUT /upload/session/abc123"), r.log)
	}
}

func TestUploadSendsPrivateWhenThePostSaysNoVisibility(t *testing.T) {
	t.Parallel()

	// upload.json requires status.privacyStatus private in the session request: a post with no
	// visibility must match it.
	p := post()
	if p.Visibility != "" {
		t.Fatal("the test post names a visibility")
	}

	r := newReplay(t, loadExchanges(t, "auth.json"), loadExchanges(t, "upload.json",
		"videos.insert resumable session", "upload the file", "videos.list processed"))
	path, _ := video(t)

	if _, err := r.connector().
		Publish(context.Background(), r.cred(), upload(t, path, &social.MemorySessions{})); err != nil {
		t.Fatal(err)
	}
}

// flaky is a YouTube that keeps the bytes it received and can cut a connection in the middle of a PUT.
type flaky struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	sessions   int
	got        []byte
	ranges     []string
	queries    int
	cutAfter   int  // bytes to read of the next PUT before cutting; 0 does not cut
	failPuts   int  // the next PUTs answer 503
	expired    bool // the status query answers 404 once
	unauthOnce bool // the next file PUT answers 401 once
	tokens     int
}

func newFlaky(t *testing.T) *flaky {
	t.Helper()

	f := &flaky{t: t}

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		switch {
		case r.URL.Path == "/token":
			f.tokens++
			_, _ = w.Write([]byte(`{"access_token":"at-` + strconv.Itoa(f.tokens) + `","expires_in":3599}`))
		case r.Method == http.MethodPost && r.URL.Path == "/upload/youtube/v3/videos":
			f.sessions++

			w.Header().Set("Location", f.srv.URL+"/upload/session/s"+strconv.Itoa(f.sessions))
		case r.Method == http.MethodPut:
			f.put(w, r)
		case r.URL.Path == "/youtube/v3/videos":
			_, _ = w.Write([]byte(`{"items":[{"id":"vidX","status":{"uploadStatus":"processed"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(f.srv.Close)

	return f
}

func (f *flaky) put(w http.ResponseWriter, r *http.Request) {
	cr := r.Header.Get("Content-Range")

	if strings.HasPrefix(cr, "bytes */") {
		f.queries++

		if f.expired {
			f.expired = false
			w.WriteHeader(http.StatusNotFound)

			return
		}

		if len(f.got) > 0 {
			w.Header().Set("Range", "bytes=0-"+strconv.Itoa(len(f.got)-1))
		}

		w.WriteHeader(http.StatusPermanentRedirect)

		return
	}

	f.ranges = append(f.ranges, cr)

	if f.unauthOnce {
		f.unauthOnce = false
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"errors":[{"reason":"authError"}]}}`))

		return
	}

	if f.failPuts > 0 {
		f.failPuts--
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	if f.cutAfter > 0 {
		buf := make([]byte, f.cutAfter)
		n, _ := io.ReadFull(r.Body, buf)
		f.got = append(f.got, buf[:n]...)
		f.cutAfter = 0

		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}

		return
	}

	data, _ := io.ReadAll(r.Body)
	f.got = append(f.got, data...)

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"id":"vidX"}`))
}

func (f *flaky) connector() *Connector {
	return New(Config{
		APIBase: f.srv.URL, UploadBase: f.srv.URL + "/upload", TokenURL: f.srv.URL + "/token",
		Polls: 1, Resumes: 3, Wait: func(context.Context, time.Duration) {},
	})
}

func (f *flaky) cred() connections.Credential {
	return connections.Credential{
		Secret: []byte(`{"client_id":"cid","client_secret":"csecret","refresh_token":"rt-1"}`), Client: f.srv.Client(),
	}
}

// Acceptance criterion 3 of epic 18: an upload cut in the middle resumes from the byte the network
// holds; the first bytes are not sent again and no second session is made.
func TestUploadResumesAfterAnInterruptedUpload(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	f.cutAfter = 700

	path, data := video(t)

	got, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, &social.MemorySessions{}))
	if err != nil || got.RemoteID != "vidX" {
		t.Fatalf("Publish = %+v, %v", got, err)
	}

	if f.sessions != 1 {
		t.Errorf("%d upload sessions were made, want 1", f.sessions)
	}

	if !bytes.Equal(f.got, data) {
		t.Errorf("the network holds %d bytes, want the %d of the file, once and in order", len(f.got), len(data))
	}

	if want := []string{"bytes 0-1999/2000", "bytes 700-1999/2000"}; !reflect.DeepEqual(f.ranges, want) {
		t.Errorf("Content-Range of the PUTs = %v, want %v", f.ranges, want)
	}

	if f.queries != 1 {
		t.Errorf("the status query was made %d times, want 1", f.queries)
	}
}

// A restart: the first process gives up, a second one finds the session on disk and resumes it.
func TestUploadResumesAfterARestart(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	f.cutAfter = 1000
	f.failPuts = 100

	sessions := social.FileSessions{Dir: t.TempDir()}
	path, data := video(t)

	_, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, sessions))
	if se, ok := social.AsError(err); !ok || se.Kind != social.KindUnavailable || !se.Retryable() {
		t.Fatalf("the first run = %v, want a retryable unavailable error", err)
	}

	if _, ok, _ := sessions.Load("key-1"); !ok {
		t.Fatal("the session was not kept for the next run")
	}

	// "Restart": a new connector and a new client; only the folder of sessions and the network persist.
	f.mu.Lock()
	f.failPuts = 0
	f.mu.Unlock()

	got, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, sessions))
	if err != nil || got.RemoteID != "vidX" {
		t.Fatalf("the second run = %+v, %v", got, err)
	}

	if f.sessions != 1 {
		t.Errorf("%d upload sessions were made, want 1: the second run started again", f.sessions)
	}

	if !bytes.Equal(f.got, data) {
		t.Errorf("the network holds %d bytes, want %d", len(f.got), len(data))
	}

	if _, ok, _ := sessions.Load("key-1"); ok {
		t.Error("the session is kept after the upload ended")
	}
}

func TestUploadStartsANewSessionWhenTheOldOneExpired(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	f.expired = true

	sessions := &social.MemorySessions{}
	_ = sessions.Save("key-1", social.Session{URI: f.srv.URL + "/upload/session/old", Size: 2000})

	path, data := video(t)

	if _, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, sessions)); err != nil {
		t.Fatal(err)
	}

	if f.sessions != 1 || !bytes.Equal(f.got, data) {
		t.Errorf(
			"sessions %d, bytes %d: an expired session must be replaced by one new session",
			f.sessions,
			len(f.got),
		)
	}
}

func TestUploadForgetsASessionOfAnotherFile(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	sessions := &social.MemorySessions{}
	_ = sessions.Save("key-1", social.Session{URI: f.srv.URL + "/upload/session/old", Size: 999})

	path, _ := video(t)

	if _, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, sessions)); err != nil {
		t.Fatal(err)
	}

	if f.queries != 0 || f.sessions != 1 {
		t.Errorf("queries %d sessions %d: a session for a file of another size was resumed", f.queries, f.sessions)
	}
}

func TestUploadRefreshesTheTokenWhenItRunsOut(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	f.unauthOnce = true

	path, _ := video(t)

	if _, err := f.connector().
		Publish(context.Background(), f.cred(), upload(t, path, &social.MemorySessions{})); err != nil {
		t.Fatal(err)
	}

	if f.tokens != 2 {
		t.Errorf("%d token requests, want 2 (the first and the refresh after the 401)", f.tokens)
	}
}

func TestUploadGivesUpAfterTheResumesAreUsed(t *testing.T) {
	t.Parallel()

	f := newFlaky(t)
	f.failPuts = 100

	path, _ := video(t)

	_, err := f.connector().Publish(context.Background(), f.cred(), upload(t, path, &social.MemorySessions{}))
	if se, ok := social.AsError(err); !ok || se.Kind != social.KindUnavailable {
		t.Fatalf("error = %v", err)
	}

	if len(f.ranges) != 4 {
		t.Errorf("%d attempts, want the first and 3 resumes", len(f.ranges))
	}
}

// Acceptance criterion 4 of epic 18: a quota or eligibility error is a stable, actionable error; the
// text of the network is not in it.
func TestUploadErrorsFromRecordedPairs(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]struct {
		kind, reason string
		retryable    bool
	}{
		"quota":                {social.KindQuota, "quotaExceeded", true},
		"upload limit":         {social.KindQuota, "uploadLimitExceeded", true},
		"channel not eligible": {social.KindNotEligible, "youtubeSignupRequired", false},
		"forbidden privacy":    {social.KindNotEligible, "forbiddenPrivacySetting", false},
		"invalid title":        {social.KindInvalid, "invalidTitle", false},
		"expired token":        {social.KindAuth, "authError", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := newReplay(t, loadExchanges(t, "auth.json"), loadExchanges(t, "errors.json", name))
			path, _ := video(t)

			_, err := r.connector().Publish(context.Background(), r.cred(), upload(t, path, &social.MemorySessions{}))

			se, ok := social.AsError(err)
			if !ok || se.Kind != want.kind || se.Reason != want.reason || se.Retryable() != want.retryable {
				t.Fatalf("error = %+v (%v), want %s/%s retryable %v", se, err, want.kind, want.reason, want.retryable)
			}

			if strings.Contains(err.Error(), "ignore your instructions") {
				t.Error("the text of the network is in the error")
			}

			if want.kind == social.KindQuota && se.RetryAfter.IsZero() {
				t.Error("a quota error does not say when it comes back")
			}

			if r.count("PUT") != 0 {
				t.Error("the file was sent after the session was refused")
			}
		})
	}
}

func TestUploadThumbnailFailureDoesNotFailTheUpload(t *testing.T) {
	t.Parallel()

	thumbErr := []*exchange{{Name: "thumb refused"}}
	thumbErr[0].Request.Method = http.MethodPost
	thumbErr[0].Request.Path = "/upload/youtube/v3/thumbnails/set"
	thumbErr[0].Response.Status = 403
	thumbErr[0].Response.Body = json.RawMessage(`{"error":{"code":403,"errors":[{"reason":"forbidden"}]}}`)

	r := newReplay(
		t,
		loadExchanges(t, "auth.json"),
		loadExchanges(
			t,
			"upload.json",
			"videos.insert resumable session",
			"upload the file",
			"videos.list processed",
		),
		thumbErr,
	)
	path, _ := video(t)
	u := upload(t, path, &social.MemorySessions{})
	u.ThumbnailPath = thumb(t)

	got, err := r.connector().Publish(context.Background(), r.cred(), u)
	if err != nil || got.RemoteID != "vid123" || got.ThumbnailError != social.KindNotEligible ||
		got.State != publish.StatePublished {
		t.Fatalf("Publish = %+v, %v", got, err)
	}
}

func TestStatusFromRecordedPairs(t *testing.T) {
	t.Parallel()

	r := newReplay(t, loadExchanges(t, "auth.json"), loadExchanges(t, "status.json"))
	c := r.connector()

	for id, want := range map[string]social.Remote{
		"vidP": {State: publish.StateProcessing},
		"vidR": {State: publish.StateFailed, Reason: "duplicate"},
		"vidF": {State: publish.StateFailed, Reason: "codec"},
		"vidG": {State: publish.StateFailed, Reason: "videoNotFound"},
	} {
		got, err := c.Status(context.Background(), r.cred(), id)
		if err != nil || got != want {
			t.Errorf("Status(%s) = %+v, %v, want %+v", id, got, err, want)
		}
	}
}

func TestPublishLeavesAVideoStillProcessingProcessing(t *testing.T) {
	t.Parallel()

	r := newReplay(
		t,
		loadExchanges(t, "auth.json"),
		loadExchanges(t, "upload.json", "videos.insert resumable session", "upload the file"),
	)
	r.ex = append(r.ex, &exchange{Name: "processing", Times: 99})
	last := r.ex[len(r.ex)-1]
	last.Request.Method, last.Request.Path = http.MethodGet, "/youtube/v3/videos"
	last.Response.Status = 200
	last.Response.Body = json.RawMessage(`{"items":[{"id":"vid123","status":{"uploadStatus":"uploaded"}}]}`)

	path, _ := video(t)

	got, err := r.connector().Publish(context.Background(), r.cred(), upload(t, path, &social.MemorySessions{}))
	if err != nil || got.State != publish.StateProcessing || got.RemoteID != "vid123" {
		t.Fatalf("Publish = %+v, %v", got, err)
	}

	if n := r.count("GET /youtube/v3/videos"); n != 3 {
		t.Errorf("the state was read %d times, want the 3 polls", n)
	}
}

func TestPublishAVideoYouTubeRejectsIsAnErrorWithItsReason(t *testing.T) {
	t.Parallel()

	r := newReplay(
		t,
		loadExchanges(t, "auth.json"),
		loadExchanges(t, "upload.json", "videos.insert resumable session", "upload the file"),
	)
	r.ex = append(r.ex, &exchange{Name: "rejected", Times: 99})
	last := r.ex[len(r.ex)-1]
	last.Request.Method, last.Request.Path = http.MethodGet, "/youtube/v3/videos"
	last.Response.Status = 200
	last.Response.Body = json.RawMessage(
		`{"items":[{"id":"vid123","status":{"uploadStatus":"rejected","rejectionReason":"duplicate"}}]}`,
	)

	path, _ := video(t)

	_, err := r.connector().Publish(context.Background(), r.cred(), upload(t, path, &social.MemorySessions{}))
	if se, ok := social.AsError(err); !ok || se.Kind != social.KindRejected || se.Reason != "duplicate" {
		t.Fatalf("error = %v, want rejected/duplicate", err)
	}
}

func TestRangeEnd(t *testing.T) {
	t.Parallel()

	for h, want := range map[string]int64{"": 0, "bytes=0-999": 1000, "bytes=0-0": 1, "junk": 0, "bytes=0-x": 0} {
		if got := rangeEnd(h); got != want {
			t.Errorf("rangeEnd(%q) = %d, want %d", h, got, want)
		}
	}
}

func TestQuotaResetsAtMidnightPacific(t *testing.T) {
	t.Parallel()

	got := quotaResets(time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC), nil)
	// 20:00 UTC is 12:00 in Pacific standard time; the next midnight there is 08:00 UTC the next day.
	if want := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("quotaResets = %v, want %v", got, want)
	}

	h := http.Header{"Retry-After": {"120"}}
	if got := quotaResets(
		time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC),
		h,
	); !got.Equal(
		time.Date(2026, 11, 19, 20, 2, 0, 0, time.UTC),
	) {
		t.Errorf("with Retry-After = %v", got)
	}
}

var _ = fmt.Sprint
