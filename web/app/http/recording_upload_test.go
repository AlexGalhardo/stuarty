package httpapi_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The routes of a recording a browser uploads (SDD-025 section 18, threat T-137 of SDD-007).

func sumHex(b string) string {
	h := sha256.Sum256([]byte(b))

	return hex.EncodeToString(h[:])
}

func chunkPath(id, track string, index int, sum string, extra string) string {
	return "/api/v1/recordings/" + id + "/upload/" + track + "/" + strconv.Itoa(index) + "?sha256=" + sum + extra
}

// beginUpload starts a browser recording as a person and returns its id.
func beginUpload(t *testing.T, h *harness, project string) string {
	t.Helper()

	res := h.do(req{
		method: http.MethodPost, path: "/api/v1/recordings/upload", noAuth: true, headers: h.personHeaders(t),
		body: `{"project_id":"` + project + `","container":"webm"}`,
	})
	if res.status != http.StatusOK {
		t.Fatalf("begin = %d %s", res.status, res.body)
	}

	got := res.json(t)
	if got["state"] != "recording" {
		t.Fatalf("begin = %s", res.body)
	}

	id, _ := got["recording_id"].(string)

	return id
}

func TestRecordingUploadRoutesCarryAChunkEndToEnd(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)
	id := beginUpload(t, h, h.newProject("p"))
	person := h.personHeaders(t)

	put := func(index int, body, sum string) resp {
		return h.do(req{
			method: http.MethodPut, path: chunkPath(id, "screen", index, sum, "&elapsed_ms=900"), body: body,
			noAuth: true, headers: person,
		})
	}

	// more than the 1 MiB of the other routes: this one has the limit of a chunk
	first := strings.Repeat("a", 3<<20)

	res := put(0, first, sumHex(first))
	if res.status != http.StatusOK {
		t.Fatalf("chunk 0 = %d %s", res.status, res.body)
	}

	if got := res.json(
		t,
	); got["next_index"] != float64(1) || got["duplicate"] != false ||
		got["bytes"] != float64(len(first)) {
		t.Errorf("chunk 0 answer = %s", res.body)
	}

	// the same chunk again is a success that changes nothing
	if res = put(0, first, sumHex(first)); res.status != http.StatusOK || res.json(t)["duplicate"] != true {
		t.Errorf("a retry = %d %s", res.status, res.body)
	}

	// a gap
	if res = put(2, "x", sumHex("x")); res.status != http.StatusConflict || res.json(t)["code"] != "E_INVALID_STATE" {
		t.Errorf("a gap = %d %s", res.status, res.body)
	}

	// a wrong digest
	if res = put(1, "x", sumHex("y")); res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
		t.Errorf("a wrong digest = %d %s", res.status, res.body)
	}

	// the resume point is in the recording
	got := h.do(req{path: "/api/v1/recordings/" + id, noAuth: true, headers: person})
	if got.status != http.StatusOK || !strings.Contains(string(got.body), `"next_index":1`) {
		t.Errorf("get = %d %s", got.status, got.body)
	}

	// the finish with a count the server does not have
	res = h.do(req{
		method: http.MethodPost, path: "/api/v1/recordings/" + id + "/finish", noAuth: true, headers: person,
		body: `{"tracks":[{"kind":"screen","chunks":9}]}`,
	})
	if res.status != http.StatusConflict || res.json(t)["code"] != "E_INVALID_STATE" {
		t.Errorf("finish with a wrong count = %d %s", res.status, res.body)
	}

	// a stop keeps what was received; this harness has no FFmpeg, so the assembly fails, and says why
	res = h.do(
		req{
			method:  http.MethodPost,
			path:    "/api/v1/recordings/" + id + "/stop",
			noAuth:  true,
			headers: person,
			body:    `{}`,
		},
	)
	if res.status != http.StatusOK {
		t.Errorf("stop = %d %s", res.status, res.body)
	}
}

// SDD-007 T-137, criterion 5 of epic 63: the local token that every agent holds is refused by every upload route
// with one stable code, before the project, the recording or the disk is looked at.
func TestSecurity_T137_AnAgentTokenGetsPrivilegedSessionOnEveryUploadRoute(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)
	project := h.newProject("p")
	id := beginUpload(t, h, project)

	sum := sumHex("x")

	for name, r := range map[string]req{
		"begin": {
			method: http.MethodPost, path: "/api/v1/recordings/upload",
			body: `{"project_id":"` + project + `","container":"webm"}`,
		},
		"chunk":               {method: http.MethodPut, path: chunkPath(id, "screen", 0, sum, ""), body: "x"},
		"chunk of another id": {method: http.MethodPut, path: chunkPath("rec_9999999999", "camera", 7, sum, ""), body: "x"},
		"finish":              {method: http.MethodPost, path: "/api/v1/recordings/" + id + "/finish", body: `{}`},
		"finish of a bad id":  {method: http.MethodPost, path: "/api/v1/recordings/nope/finish", body: `{"tracks":[{"kind":"x","chunks":-1}]}`},
	} {
		res := h.do(r)
		if res.status != http.StatusForbidden || res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s with the local token = %d %s, want 403 E_PRIVILEGED_SESSION", name, res.status, res.body)
		}
	}

	// a session that did not prove the launcher secret is no person either
	weak := map[string]string{"Cookie": h.sessionWith(t, ""), "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL}

	res := h.do(req{
		method: http.MethodPut, path: chunkPath(id, "screen", 0, sum, ""), body: "x", noAuth: true, headers: weak,
	})
	if res.status != http.StatusForbidden || res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
		t.Errorf("a session without the secret = %d %s", res.status, res.body)
	}

	// and nothing was stored for any of them
	if m, _ := filepath.Glob(filepath.Join(string(h.dir), "recordings", id, "parts", "screen", "*")); len(m) != 0 {
		t.Errorf("a refused caller stored %v", m)
	}
}

// A chunk route has the limit of a chunk, and no other route is raised: a JSON body over 1 MiB is still refused.
func TestSecurity_T137_TheChunkRouteHasItsOwnBodyLimit(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)
	project := h.newProject("p")
	id := beginUpload(t, h, project)
	person := h.personHeaders(t)

	over := strings.Repeat("z", 16<<20+1)

	res := h.do(req{
		method: http.MethodPut, path: chunkPath(id, "screen", 0, sumHex(over), ""), body: over, noAuth: true,
		headers: person,
	})
	if res.status != http.StatusRequestEntityTooLarge {
		t.Errorf("a chunk over 16 MiB = %d, want 413", res.status)
	}

	if got := res.json(t); got["code"] != "E_PAYLOAD_TOO_LARGE" {
		t.Errorf("code = %v, want E_PAYLOAD_TOO_LARGE: %s", got["code"], res.body)
	}

	if m, _ := filepath.Glob(filepath.Join(string(h.dir), "recordings", id, "parts", "screen", "*")); len(m) != 0 {
		t.Errorf("an over-limit chunk left %v", m)
	}

	// exactly the limit is taken
	exact := strings.Repeat("e", 16<<20)

	if res = h.do(req{
		method: http.MethodPut, path: chunkPath(id, "screen", 0, sumHex(exact), ""), body: exact, noAuth: true,
		headers: person,
	}); res.status != http.StatusOK {
		t.Errorf("a chunk of 16 MiB = %d %s", res.status, res.body[:min(len(res.body), 200)])
	}

	// the other routes keep the limit of the router
	big := `{"project_id":"` + project + `","container":"webm","label":"` + strings.Repeat("l", 2<<20) + `"}`

	if res = h.do(req{
		method: http.MethodPost, path: "/api/v1/recordings/upload", body: big, noAuth: true, headers: person,
	}); res.status != http.StatusRequestEntityTooLarge {
		t.Errorf("a JSON body of 2 MiB = %d, want 413", res.status)
	}
}

// The cookie of a person is not enough for a page of another origin: the checks of every route (SDD-007 section
// 3.6) apply to the chunk route, which a cross-site form cannot reach.
func TestSecurity_T137_AnUploadFromAnotherOriginIsRefused(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)
	id := beginUpload(t, h, h.newProject("p"))
	cookie := h.session(t)
	path := chunkPath(id, "screen", 0, sumHex("x"), "")

	send := func(headers map[string]string) int {
		hr, err := http.NewRequest(http.MethodPut, h.srv.URL+path, strings.NewReader("x"))
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

		_ = res.Body.Close()

		return res.StatusCode
	}

	if got := send(map[string]string{
		"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "application/json",
	}); got != http.StatusOK {
		t.Fatalf("the same origin = %d", got)
	}

	for name, headers := range map[string]map[string]string{
		"another origin": {"Origin": "http://evil.example", "Sec-Fetch-Site": "cross-site", "Content-Type": "application/json"},
		"a form type":    {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "multipart/form-data; boundary=x"},
		"a plain type":   {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin", "Content-Type": "text/plain"},
		"no type":        {"Origin": h.srv.URL, "Sec-Fetch-Site": "same-origin"},
	} {
		if got := send(headers); got != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", name, got)
		}
	}
}

// What the caller names never becomes a path: a track, an index or an id that is not what the route says is
// refused, and the folder of the recording holds only the generated names.
func TestSecurity_T137_NoNameOfTheCallerBuildsAPath(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)
	id := beginUpload(t, h, h.newProject("p"))
	person := h.personHeaders(t)

	for name, p := range map[string]string{
		"a track that climbs":   chunkPath(id, "..%2F..%2Fx", 0, sumHex("x"), ""),
		"a track with a slash":  chunkPath(id, "screen%2Fx", 0, sumHex("x"), ""),
		"a sound track":         chunkPath(id, "microphone", 0, sumHex("x"), ""),
		"an index that climbs":  "/api/v1/recordings/" + id + "/upload/screen/..%2Fx?sha256=" + sumHex("x"),
		"a negative index":      chunkPath(id, "screen", -1, sumHex("x"), ""),
		"an id that is a path":  chunkPath("..%2F..%2Fx", "screen", 0, sumHex("x"), ""),
		"an index out of range": chunkPath(id, "screen", 100000, sumHex("x"), ""),
	} {
		res := h.do(req{method: http.MethodPut, path: p, body: "x", noAuth: true, headers: person})
		if res.status < 400 || res.status >= 500 {
			t.Errorf("%s = %d %s, want a refusal", name, res.status, res.body)
		}
	}

	entries := map[string]bool{}

	_ = filepath.WalkDir(filepath.Join(string(h.dir), "recordings"), func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			entries[path] = true
		}

		return nil
	})

	if len(entries) != 0 {
		t.Errorf("refused chunks wrote %v", entries)
	}
}

// SDD-007 section 3.6, T-137: a browser recording needs the screen, the camera and the microphone, which the default
// of the framework turns off for every page. The policy names the page itself and no other origin, on every kind of
// response, and the location stays off.
func TestSecurity_T137_OnlyThePageItselfMayUseTheScreenTheCameraAndTheMicrophone(t *testing.T) {
	t.Parallel()

	h := fileHarness(t, 1<<30)

	want := map[string]string{
		"camera": "(self)", "microphone": "(self)", "display-capture": "(self)", "geolocation": "()",
	}

	for _, path := range []string{"/up", "/api/v1/projects", "/"} {
		res := h.do(req{path: path})

		policy := res.header.Get("Permissions-Policy")
		if policy == "" {
			t.Errorf("%s has no Permissions-Policy", path)

			continue
		}

		if strings.Contains(policy, "*") || strings.Contains(policy, "http") {
			t.Errorf("%s: the policy names an origin: %q", path, policy)
		}

		got := map[string]string{}

		for directive := range strings.SplitSeq(policy, ",") {
			name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
			got[name] = value
		}

		for name, value := range want {
			if got[name] != value {
				t.Errorf("%s: %s = %q in %q, want %q", path, name, got[name], policy, value)
			}
		}
	}
}
