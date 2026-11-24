package testpublish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexGalhardo/stuarty/service/social/youtube"
)

// FakeYouTube is a YouTube on loopback that speaks what the connector speaks: the token endpoint, the channel
// list, the resumable upload session (SDD-020 section 7.2: the session is opened by one request, the file goes
// by a PUT with a Content-Range, and an empty PUT asks how much of it the network holds) and the state of a
// video. It keeps the bytes of every session, so a test can say how much arrived, how many sessions were
// opened and how many videos were finished.
type FakeYouTube struct {
	Srv *httptest.Server

	mu       sync.Mutex
	sessions []*ytSession
	videos   int
	stall    *ytStall
	puts     []Put
	quit     chan struct{}
}

// Put is one PUT of file bytes the network received for a session.
type Put struct {
	Session int
	// Start is the offset the PUT began at, as its Content-Range said.
	Start int64
	// Bytes is how many bytes of it were read before it ended or was dropped.
	Bytes int64
}

type ytSession struct {
	size     int64
	received []byte
	done     bool
	videoID  string
}

type ytStall struct {
	at      int64
	reached chan struct{}
}

// NewFakeYouTube starts the network; the test closes it.
func NewFakeYouTube(t Tester) *FakeYouTube {
	t.Helper()

	y := &FakeYouTube{quit: make(chan struct{})}
	y.Srv = httptest.NewServer(http.HandlerFunc(y.serve))

	// A stalled request is not read to its end, so the server cannot see its sender go away: the test ending
	// is what lets it return.
	t.Cleanup(func() {
		close(y.quit)
		y.Srv.Close()
	})

	return y
}

// Config is the configuration of the YouTube connector that talks to this network.
func (y *FakeYouTube) Config() youtube.Config { return YouTubeConfig(y.Srv.URL) }

// YouTubeConfig is the configuration of the YouTube connector for a fake network at base, which a child
// process builds from the address the parent gave it.
func YouTubeConfig(base string) youtube.Config {
	return youtube.Config{
		APIBase: base, UploadBase: base + "/upload", AuthURL: base + "/authorize", TokenURL: base + "/token",
		WatchURL: base + "/watch?v=", Polls: 2, PollEvery: time.Millisecond,
	}
}

// StallAt arms the network to stop reading the file of a session once it holds that many bytes: it keeps the
// bytes, drops the request unanswered, and closes the returned channel. It fires once.
func (y *FakeYouTube) StallAt(bytes int64) <-chan struct{} {
	y.mu.Lock()
	defer y.mu.Unlock()

	y.stall = &ytStall{at: bytes, reached: make(chan struct{})}

	return y.stall.reached
}

// Sessions is how many upload sessions were opened.
func (y *FakeYouTube) Sessions() int {
	y.mu.Lock()
	defer y.mu.Unlock()

	return len(y.sessions)
}

// Finished is how many videos the network completed.
func (y *FakeYouTube) Finished() int {
	y.mu.Lock()
	defer y.mu.Unlock()

	return y.videos
}

// Received is how many bytes a session holds.
func (y *FakeYouTube) Received(session int) int64 {
	y.mu.Lock()
	defer y.mu.Unlock()

	if session >= len(y.sessions) {
		return 0
	}

	return int64(len(y.sessions[session].received))
}

// Digest is the SHA-256 of the bytes a session holds.
func (y *FakeYouTube) Digest(session int) string {
	y.mu.Lock()
	defer y.mu.Unlock()

	if session >= len(y.sessions) {
		return ""
	}

	sum := sha256.Sum256(y.sessions[session].received)

	return hex.EncodeToString(sum[:])
}

// Puts lists the PUTs that carried file bytes, in order.
func (y *FakeYouTube) Puts() []Put {
	y.mu.Lock()
	defer y.mu.Unlock()

	return append([]Put(nil), y.puts...)
}

func (y *FakeYouTube) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1", "refresh_token": RefreshCanary, "expires_in": 3600,
		})
	case r.URL.Path == "/youtube/v3/channels":
		_, _ = w.Write([]byte(`{"items":[{"id":"UC-FAKE","snippet":{"title":"Fake channel"}}]}`))
	case r.URL.Path == "/upload/youtube/v3/videos" && r.Method == http.MethodPost:
		y.open(w, r)
	case strings.HasPrefix(r.URL.Path, "/upload/session/") && r.Method == http.MethodPut:
		y.put(w, r)
	case r.URL.Path == "/youtube/v3/videos":
		_, _ = w.Write([]byte(`{"items":[{"status":{"uploadStatus":"processed"},` +
			`"processingDetails":{"processingStatus":"succeeded"}}]}`))
	default:
		http.NotFound(w, r)
	}
}

func (y *FakeYouTube) open(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)

	size, _ := strconv.ParseInt(r.Header.Get("X-Upload-Content-Length"), 10, 64)

	y.mu.Lock()
	y.sessions = append(y.sessions, &ytSession{size: size})
	n := len(y.sessions) - 1
	y.mu.Unlock()

	w.Header().Set("Location", fmt.Sprintf("%s/upload/session/%d", y.Srv.URL, n))
	w.WriteHeader(http.StatusOK)
}

// put answers the empty PUT that asks for the offset and the PUT that carries the rest of the file.
func (y *FakeYouTube) put(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/upload/session/"))

	y.mu.Lock()

	if err != nil || n < 0 || n >= len(y.sessions) {
		y.mu.Unlock()
		http.NotFound(w, r)

		return
	}

	s := y.sessions[n]
	have := int64(len(s.received))
	done, videoID := s.done, s.videoID
	y.mu.Unlock()

	rng := r.Header.Get("Content-Range")

	if done {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": videoID})

		return
	}

	if strings.HasPrefix(rng, "bytes */") {
		answerOffset(w, have)

		return
	}

	start, ok := rangeStart(rng)
	if !ok || start != have {
		// A resume that does not continue where the network stopped: the contract of the protocol is broken.
		http.Error(w, fmt.Sprintf("the PUT starts at %d, the session holds %d", start, have), http.StatusBadRequest)

		return
	}

	read := y.readFile(r, n, s, start)
	if !read {
		return
	}

	y.mu.Lock()

	if int64(len(s.received)) < s.size {
		have = int64(len(s.received))
		y.mu.Unlock()
		answerOffset(w, have)

		return
	}

	y.videos++
	s.done, s.videoID = true, fmt.Sprintf("yt%d", y.videos)
	id := s.videoID
	y.mu.Unlock()

	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// readFile reads the body of a PUT into the session. It returns false when the request was dropped (the
// stall fired, or the sender went away).
func (y *FakeYouTube) readFile(r *http.Request, n int, s *ytSession, start int64) bool {
	var got int64

	buf := make([]byte, 16<<10)

	record := func() {
		y.mu.Lock()
		y.puts = append(y.puts, Put{Session: n, Start: start, Bytes: got})
		y.mu.Unlock()
	}

	for {
		want := int64(len(buf))

		y.mu.Lock()
		stall := y.stall

		if stall != nil {
			if room := stall.at - int64(len(s.received)); room < want {
				want = room
			}
		}

		y.mu.Unlock()

		if want <= 0 {
			// The network holds what the test asked for: it keeps it and never answers.
			y.mu.Lock()

			if y.stall == stall && stall != nil {
				y.stall = nil
				close(stall.reached)
			}

			y.mu.Unlock()
			record()
			select {
			case <-r.Context().Done():
			case <-y.quit:
			}

			return false
		}

		c, err := r.Body.Read(buf[:want])

		if c > 0 {
			y.mu.Lock()
			s.received = append(s.received, buf[:c]...)
			y.mu.Unlock()

			got += int64(c)
		}

		if err == io.EOF {
			record()

			return true
		}

		if err != nil {
			record()

			return false
		}
	}
}

func answerOffset(w http.ResponseWriter, have int64) {
	if have > 0 {
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", have-1))
	}

	w.WriteHeader(http.StatusPermanentRedirect)
}

// rangeStart reads the first offset of "bytes 100-199/1000".
func rangeStart(h string) (int64, bool) {
	rest, ok := strings.CutPrefix(h, "bytes ")
	if !ok {
		return 0, false
	}

	first, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}

	n, err := strconv.ParseInt(first, 10, 64)

	return n, err == nil
}
