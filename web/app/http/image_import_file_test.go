package httpapi_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// The route that takes an image file as the body of the request (SDD-021 section 10.2, epic 48 task 7). The
// service here has the content store and the path guard and no ingest queue, so a file is stored by hash and its
// ingest stays queued: the answer says the file is pending, with its hash. The ingest and the document are
// tested over the real store in web/app/servicetest and in the browser by the end-to-end specs.

const importFilePath = "/api/v1/images/import/file"

// fileHarness serves the API over a service whose limit of an import is maxImport bytes.
func fileHarness(tb testing.TB, maxImport int64) *harness {
	tb.Helper()

	return newHarnessBuilt(
		tb,
		events.Options{},
		nil,
		httpapi.RateLimits{},
		func(dir workspace.Dir, o *service.Options) {
			guard, err := pathguard.New(pathguard.Config{Workspace: string(dir)})
			if err != nil {
				tb.Fatal(err)
			}

			o.Workspace = string(dir)
			o.Limits = service.Limits{MaxImportBytes: maxImport}
			o.Media = service.Media{
				Content: enginestore.New(string(dir)), Guard: guard,
				FreeBytes: func(string) (uint64, error) { return 1 << 40, nil },
			}
		},
	)
}

// sendFile posts the bytes of a file the way the screen does: the body as it is, the declared type JSON (a
// cookie session needs a preflighted type on an unsafe method), the other members in the query.
func (h *harness) sendFile(body io.Reader, size int64, query url.Values, headers map[string]string) resp {
	h.t.Helper()

	hr, err := http.NewRequest(http.MethodPost, h.srv.URL+importFilePath+"?"+query.Encode(), body)
	if err != nil {
		h.t.Fatal(err)
	}

	hr.ContentLength = size
	hr.Header.Set("Authorization", "Bearer "+canary)
	hr.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		hr.Header.Set(k, v)
	}

	res, err := h.srv.Client().Do(hr)
	if err != nil {
		h.t.Fatal(err)
	}

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		h.t.Fatal(err)
	}

	return resp{status: res.StatusCode, header: res.Header, body: b}
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)

	return "sha256:" + hex.EncodeToString(sum[:])
}

func pending(file string) url.Values {
	return url.Values{"file_name": {file}, "wait_ms": {"1"}}
}

// TestImageUploadStoresTheBodyByHash: the bytes are the body, whatever the declared type is, and the file enters
// the content store under the hash of those bytes; the answer is pending (no ingest queue here) and names it.
func TestImageUploadStoresTheBodyByHash(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<20)
	data := bytes.Repeat([]byte("not json at all, but the bytes of a file "), 40)

	res := h.sendFile(bytes.NewReader(data), int64(len(data)), pending("Holiday Photo.png"), nil)
	if res.status != http.StatusOK {
		t.Fatalf("%d %s", res.status, res.body)
	}

	got := res.json(t)
	if got["state"] != "pending" || got["hash"] != hashOf(data) || got["name"] != "Holiday Photo.png" {
		t.Fatalf("%v", got)
	}

	path, err := enginestore.New(string(h.dir)).Path(strings.TrimPrefix(hashOf(data), "sha256:"))
	if err != nil {
		t.Fatal(err)
	}

	if stored, rerr := os.ReadFile(path); rerr != nil || !bytes.Equal(stored, data) {
		t.Fatalf("the stored file is not the body: %v", rerr)
	}

	// no length declared (chunked): the same bytes, the same hash
	// the declared type does not matter: the bytes are the file
	octet := map[string]string{"Content-Type": "application/octet-stream"}
	again := h.sendFile(io.NopCloser(bytes.NewReader(data)), -1, pending("again.png"), octet)
	if again.status != http.StatusOK || again.json(t)["hash"] != hashOf(data) {
		t.Fatalf("%d %s", again.status, again.body)
	}
}

func TestImageUploadRefusals(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<20)
	data := []byte(strings.Repeat("x", 300))

	for name, c := range map[string]struct {
		query  url.Values
		body   []byte
		status int
		code   string
	}{
		"an empty body":            {pending("a.png"), nil, http.StatusUnprocessableEntity, "E_UNSUPPORTED_MEDIA"},
		"a size for a layer":       {url.Values{"image_id": {"img_zzzzzzzzzz"}, "width": {"10"}, "height": {"10"}}, data, http.StatusBadRequest, "E_VALIDATION"},
		"a width that is not one":  {url.Values{"width": {"wide"}}, data, http.StatusBadRequest, "E_VALIDATION"},
		"a negative base revision": {url.Values{"base_revision": {"-3"}}, data, http.StatusBadRequest, "E_VALIDATION"},
		"an unknown image":         {url.Values{"image_id": {"img_zzzzzzzzzz"}, "base_revision": {"1"}}, data, http.StatusNotFound, "E_NOT_FOUND"},
		"a key with no image":      {url.Values{"idempotency_key": {"upload-key-1"}}, data, http.StatusBadRequest, "E_VALIDATION"},
		"a wait over the limit":    {url.Values{"wait_ms": {"99999"}}, data, http.StatusBadRequest, "E_VALIDATION"},
	} {
		res := h.sendFile(bytes.NewReader(c.body), int64(len(c.body)), c.query, nil)
		if res.status != c.status || !strings.Contains(string(res.body), c.code) {
			t.Errorf("%s: %d %s", name, res.status, res.body)
		}
	}

	// the route needs the edit scope and a credential
	hr, err := http.NewRequest(http.MethodPost, h.srv.URL+importFilePath, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	hr.Header.Set("Content-Type", "application/json")

	res, err := h.srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credential = %d", res.StatusCode)
	}
}

// TestSecurity_T92_UploadOverTheLimitIsRefusedAndStoresNothing: a file over the limit, declared or streamed
// without a length, is E_LIMIT_EXCEEDED, and nothing of it is left in the content store.
func TestSecurity_T92_UploadOverTheLimitIsRefusedAndStoresNothing(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 100)
	data := bytes.Repeat([]byte("y"), 5000)

	declared := h.sendFile(bytes.NewReader(data), int64(len(data)), pending("big.png"), nil)
	if declared.status != http.StatusUnprocessableEntity ||
		!strings.Contains(string(declared.body), "E_LIMIT_EXCEEDED") {
		t.Fatalf("declared: %d %s", declared.status, declared.body)
	}

	streamed := h.sendFile(io.NopCloser(bytes.NewReader(data)), -1, pending("big.png"), nil)
	if streamed.status != http.StatusUnprocessableEntity ||
		!strings.Contains(string(streamed.body), "E_LIMIT_EXCEEDED") {
		t.Fatalf("streamed: %d %s", streamed.status, streamed.body)
	}

	// no object, and no partial copy in .incoming
	var left []string

	_ = filepath.WalkDir(filepath.Join(string(h.dir), "store"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			left = append(left, p)
		}

		return nil
	})

	if len(left) != 0 {
		t.Fatalf("a refused file left %v", left)
	}

	// the limit is the one of the service, not the 1 MiB of the router: a file between them is read
	big := fileHarness(t, 8<<20)
	body := bytes.Repeat([]byte("z"), 2<<20)

	if res := big.sendFile(
		bytes.NewReader(body),
		int64(len(body)),
		pending("two.png"),
		nil,
	); res.status != http.StatusOK ||
		res.json(t)["hash"] != hashOf(body) {
		t.Fatalf("a 2 MiB file: %d %.200s", res.status, res.body)
	}
}

// TestSecurity_T94_UploadFromAnotherOriginIsRefused: a page of another origin cannot post a file with the
// session of the person (SDD-007 section 3.6), and a cookie session that sends a type a page can send without a
// preflight is refused as well.
func TestSecurity_T94_UploadFromAnotherOriginIsRefused(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<20)
	cookie := h.session(t)
	data := []byte(strings.Repeat("q", 300))

	post := func(headers map[string]string) resp {
		hr, err := http.NewRequest(http.MethodPost, h.srv.URL+importFilePath+"?wait_ms=1", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		hr.Header.Set("Cookie", cookie)

		for k, v := range headers {
			hr.Header.Set(k, v)
		}

		res, err := h.srv.Client().Do(hr)
		if err != nil {
			t.Fatal(err)
		}

		defer res.Body.Close()

		b, _ := io.ReadAll(res.Body)

		return resp{status: res.StatusCode, header: res.Header, body: b}
	}

	same := map[string]string{"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json"}

	if res := post(same); res.status != http.StatusOK {
		t.Fatalf("the same origin with a preflighted type = %d %s", res.status, res.body)
	}

	for name, headers := range map[string]map[string]string{
		"another origin":  {"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"},
		"another site":    {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-site", "Content-Type": "application/json"},
		"a form type":     {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "multipart/form-data; boundary=x"},
		"a plain type":    {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "text/plain"},
		"no type":         {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin"},
		"no origin given": {"Content-Type": "application/json"},
	} {
		if res := post(headers); res.status != http.StatusForbidden {
			t.Errorf("%s: %d %s", name, res.status, res.body)
		}
	}
}

// TestSecurity_T95_UploadFileNameIsALabelNeverAPath: a name that is a path, or that carries control and
// bidirectional characters, markup or a very long text, is cut to a base name with those characters replaced; the
// file is stored under its hash and nowhere else.
func TestSecurity_T95_UploadFileNameIsALabelNeverAPath(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<20)
	data := []byte(strings.Repeat("n", 300))

	for _, name := range []string{
		"../../../etc/passwd.png",
		`C:\Windows\System32\evil.png`,
		"evil\u202egpj.exe",
		"a\x00b\x1b[31m.png",
		"<img src=x onerror=alert(1)>.png",
		strings.Repeat("long", 200) + ".png",
	} {
		res := h.sendFile(bytes.NewReader(data), int64(len(data)), pending(name), nil)
		if res.status != http.StatusOK {
			t.Fatalf("%q: %d %s", name, res.status, res.body)
		}

		got, _ := res.json(t)["name"].(string)

		if strings.ContainsAny(got, "/\\\x00\x1b\u202e") || len([]rune(got)) > 200 || got == "" {
			t.Errorf("%q became %q", name, got)
		}
	}

	// every file is in the store by its hash: one object, whatever the names were
	var objects int

	_ = filepath.WalkDir(filepath.Join(string(h.dir), "store"), func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			objects++
		}

		return nil
	})

	if objects != 1 {
		t.Errorf("%d objects in the store, want 1", objects)
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(string(h.dir)), "etc")); err == nil {
		t.Error("a path in a name made a folder next to the workspace")
	}
}
