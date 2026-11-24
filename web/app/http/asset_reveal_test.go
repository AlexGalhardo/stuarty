package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/events"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

const revealBody = "the bytes of a stored clip"

// revealHarness serves the API over a service that knows its workspace, so a stored file has a place.
func revealHarness(tb testing.TB) *harness {
	tb.Helper()

	return newHarnessBuilt(tb, events.Options{}, nil, httpapi.RateLimits{},
		func(dir workspace.Dir, o *service.Options) { o.Workspace = string(dir) })
}

// storedAsset puts a ready file in the content store and in a project, as an import that finished would, and returns
// its hash.
func (h *harness) storedAsset(project string) string {
	h.t.Helper()

	sum := sha256.Sum256([]byte(revealBody))
	hexHash := hex.EncodeToString(sum[:])
	hash := "sha256:" + hexHash

	p, err := enginestore.New(string(h.dir)).Path(hexHash)
	if err != nil {
		h.t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		h.t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte(revealBody), 0o600); err != nil {
		h.t.Fatal(err)
	}

	err = h.store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		if _, _, uerr := tx.Assets().Upsert(ctx, service.AssetRow{
			Hash: hash, Kind: "video", SizeBytes: int64(len(revealBody)), OriginalName: "clip.mp4",
			IngestState: service.IngestReady, CreatedMs: 1, UpdatedMs: 1,
		}); uerr != nil {
			return uerr
		}

		return tx.Assets().PutProjectAsset(ctx, service.ProjectAssetRow{
			ProjectID: project, AssetID: "ast_0000000001", AssetHash: hash, State: service.ProjectAssetActive,
			Name: "clip.mp4", ImportedBy: "local:ui", ImportedMs: 1,
		})
	})
	if err != nil {
		h.t.Fatal(err)
	}

	return hash
}

func (h *harness) personHeaders(t *testing.T) map[string]string {
	t.Helper()

	return map[string]string{"Cookie": h.session(t), "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL}
}

// A person in the desktop session gets the path, with a header that keeps it out of every cache; the body is
// `{}` and the hash is in the route, never a path.
func TestAssetRevealRouteAPersonGetsTheCheckedPath(t *testing.T) {
	t.Parallel()

	h := revealHarness(t)
	hash := h.storedAsset(h.newProject("clip"))

	res := h.do(req{
		method: http.MethodPost, path: "/api/v1/assets/" + hash + "/reveal", body: `{}`, noAuth: true,
		headers: h.personHeaders(t),
	})
	if res.status != http.StatusOK {
		t.Fatalf("reveal = %d %s", res.status, res.body)
	}

	if cc := res.header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	got := res.json(t)

	path, _ := got["path"].(string)
	if body, err := os.ReadFile(path); err != nil || string(body) != revealBody {
		t.Errorf("path %q does not lead to the stored file: %v", path, err)
	}

	if got["display"] != path {
		t.Errorf("display = %v, want the same path", got["display"])
	}
}

// The local token (what every agent and the CLI hold), and a session whose launch did not prove the launcher
// secret, are not a person: E_PRIVILEGED_SESSION, no path.
func TestSecurity_T123_TheRouteRefusesTheTokenAndASessionWithoutTheSecret(t *testing.T) {
	t.Parallel()

	h := revealHarness(t)
	hash := h.storedAsset(h.newProject("clip"))
	route := "/api/v1/assets/" + hash + "/reveal"

	cases := map[string]req{
		"the local token": {method: http.MethodPost, path: route, body: `{}`},
		"a session without the secret": {
			method: http.MethodPost, path: route, body: `{}`, noAuth: true,
			headers: map[string]string{
				"Cookie": h.sessionWith(t, ""), "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL,
			},
		},
	}

	for name, r := range cases {
		res := h.do(r)
		if res.status != http.StatusForbidden || res.json(t)["code"] != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s = %d %s, want 403 E_PRIVILEGED_SESSION", name, res.status, res.body)
		}

		if strings.Contains(string(res.body), string(h.dir)) || strings.Contains(string(res.body), "store") {
			t.Errorf("%s: the refusal names a place: %s", name, res.body)
		}
	}

	if res := h.do(
		req{method: http.MethodPost, path: route, body: `{}`, noAuth: true},
	); res.status != http.StatusUnauthorized {
		t.Errorf("no credentials = %d, want 401", res.status)
	}
}

func TestAssetRevealRouteAnswersTheRefusalsOfTheService(t *testing.T) {
	t.Parallel()

	h := revealHarness(t)
	hash := h.storedAsset(h.newProject("clip"))
	person := h.personHeaders(t)

	ask := func(hashOrPath string) resp {
		return h.do(req{
			method: http.MethodPost, path: "/api/v1/assets/" + hashOrPath + "/reveal", body: `{}`, noAuth: true,
			headers: person,
		})
	}

	if res := ask("sha256:" + strings.Repeat("c", 64)); res.status != http.StatusNotFound {
		t.Errorf("unknown hash = %d %s, want 404", res.status, res.body)
	}

	if res := ask("..%2F..%2Fetc%2Fpasswd"); res.status != http.StatusBadRequest && res.status != http.StatusNotFound {
		t.Errorf("a path as the hash = %d %s, want a refusal", res.status, res.body)
	}

	sum := sha256.Sum256([]byte(revealBody))

	p, err := enginestore.New(string(h.dir)).Path(hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}

	res := ask(hash)
	if res.status != http.StatusNotFound || res.json(t)["code"] != "E_FILE_NOT_FOUND" {
		t.Errorf("missing file = %d %s, want 404 E_FILE_NOT_FOUND", res.status, res.body)
	}

	if strings.Contains(string(res.body), string(h.dir)) {
		t.Errorf("the refusal names the workspace: %s", res.body)
	}
}

// No route, to any caller of a network server, names a path on its disk.
func TestSecurity_T123_NoRouteRevealsAPathInNetworkMode(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleAdmin)
	ana := h.session(t, "ana@example.com")
	key := h.makeKey(t, ana, `{"name":"ops","scopes":["read","media","admin"],"expires_in_days":30}`)

	hash := "sha256:" + strings.Repeat("a", 64)
	route := "/api/v1/assets/" + hash + "/reveal"

	for name, res := range map[string]netResp{
		"a session": h.send(t, http.MethodPost, route, `{}`, ana, launcherHeaders(launcherSecret)),
		"a key":     h.asKey(t, key.Secret, http.MethodPost, route, `{}`),
	} {
		if res.status != http.StatusNotFound || res.code() != "E_NOT_FOUND" {
			t.Errorf("%s = %d %s, want 404 E_NOT_FOUND", name, res.status, res.body)
		}

		if strings.Contains(string(res.body), "path") && strings.Contains(string(res.body), "store") {
			t.Errorf("%s: the answer names a place: %s", name, res.body)
		}
	}
}
