package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

const (
	previewReady   = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	previewPending = "5d41402abc4b2a76b9719d911017c59200000000000000000000000000000000"
)

// previewService answers the keys of the test itself and gives every other one to the real service, which hands out
// no key until a plan window does: the answer for those is 404.
type previewService struct {
	service.Service

	project string

	mu   sync.Mutex
	asks []service.FileRef
}

func (p *previewService) OpenFile(ctx context.Context, ref service.FileRef) (*service.File, error) {
	if ref.Kind != "audio_preview" || ref.ProjectID != p.project {
		return p.Service.OpenFile(ctx, ref)
	}

	p.mu.Lock()
	p.asks = append(p.asks, ref)
	p.mu.Unlock()

	switch ref.ID {
	case previewPending:
		return &service.File{Pending: true}, nil
	case previewReady:
		f := &service.File{
			Path: "audio-previews/" + ref.ID + ".wav", ContentType: "audio/wav", ETag: `"ap-` + ref.ID + `"`,
			ModTime: time.Unix(0, 0).UTC(), Immutable: true, MaxAge: 31536000,
		}

		if ref.Variant != "" {
			f.Header = map[string]string{"X-Stuarty-Window": ref.Variant + "-119"}
		}

		return f, nil
	default:
		return p.Service.OpenFile(ctx, ref)
	}
}

var wavBody = append([]byte("RIFF....WAVEfmt "), bytes.Repeat([]byte{'d'}, 4000)...)

func previewHarness(t *testing.T) (*harness, *previewService, string) {
	t.Helper()

	var ps *previewService

	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service {
		ps = &previewService{Service: s}

		return ps
	})

	id := h.newProject("preview")
	ps.mu.Lock()
	ps.project = id
	ps.mu.Unlock()

	dir := filepath.Join(string(h.dir), "audio-previews")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, previewReady+".wav"), wavBody, 0o600); err != nil {
		t.Fatal(err)
	}

	return h, ps, "/api/v1/projects/" + id + "/audio-previews/"
}

// The processed sound of an item is served with Range and long caching, answers 202 while it is rendered, and is 404
// for a key the project's plan did not hand out (SDD-003 section 6.5).
func TestAudioPreviewRoute(t *testing.T) {
	t.Parallel()

	h, ps, base := previewHarness(t)

	whole := h.get(base + previewReady)
	if whole.status != http.StatusOK || !bytes.Equal(whole.body, wavBody) {
		t.Fatalf("whole = %d, %d bytes", whole.status, len(whole.body))
	}

	for name, want := range map[string]string{
		"Content-Type": "audio/wav", "Etag": `"ap-` + previewReady + `"`, "Accept-Ranges": "bytes",
		"Cache-Control": "private, max-age=31536000, immutable",
	} {
		if got := whole.header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	ranged := h.do(req{path: base + previewReady, headers: map[string]string{"Range": "bytes=0-3"}})
	if ranged.status != http.StatusPartialContent || string(ranged.body) != "RIFF" ||
		ranged.header.Get("Content-Range") != "bytes 0-3/"+strconv.Itoa(len(wavBody)) {
		t.Errorf("range = %d %q %q", ranged.status, ranged.body, ranged.header.Get("Content-Range"))
	}

	if bad := h.do(req{path: base + previewReady, headers: map[string]string{"Range": "bytes=999999-"}}); bad.status !=
		http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("an unsatisfiable range = %d, want 416", bad.status)
	}

	window := h.get(base + previewReady + "?from=60")
	if window.status != http.StatusOK || window.header.Get("X-Stuarty-Window") != "60-119" {
		t.Errorf("window = %d %q", window.status, window.header.Get("X-Stuarty-Window"))
	}

	ps.mu.Lock()
	last := ps.asks[len(ps.asks)-1]
	ps.mu.Unlock()

	if last.Kind != "audio_preview" || last.Variant != "60" || last.ID != previewReady {
		t.Errorf("the service was asked for %+v", last)
	}

	pending := h.get(base + previewPending)
	answer := strings.TrimSpace(string(pending.body))
	if pending.status != http.StatusAccepted || pending.header.Get("Retry-After") != "1" ||
		pending.header.Get("Cache-Control") != "no-store" || answer != `{"state":"processing"}` {
		t.Errorf("pending = %d %v %s", pending.status, pending.header, pending.body)
	}

	// A key that no plan window handed out: 404 whatever it looks like.
	for _, key := range []string{strings.Repeat("a", 64), "short", strings.Repeat("A", 64), "..%2F..%2Fdb.sqlite", "%2e%2e"} {
		res := h.get(base + key)
		if res.status != http.StatusNotFound {
			t.Errorf("key %q = %d %s, want 404", key, res.status, res.body)
		}
	}

	// Another project does not see it, and a caller with no token is refused.
	if res := h.get("/api/v1/projects/prj_zzzzzzzzzz/audio-previews/" + previewReady); res.status == http.StatusOK {
		t.Errorf("another project was served the file: %d", res.status)
	}

	if res := h.do(req{path: base + previewReady, noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", res.status)
	}
}
