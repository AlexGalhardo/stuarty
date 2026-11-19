package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/web/app/events"

	vukem "github.com/AlexGalhardo/vukem"
)

// manifest stands in for the output of `vite build`, which the Go tests do not run.
const manifest = `{
	"resources/js/app.tsx": {"file": "assets/app.js", "src": "resources/js/app.tsx", "isEntry": true, "css": ["assets/app.css"]},
	"resources/js/editor.tsx": {"file": "assets/editor.js", "src": "resources/js/editor.tsx", "isEntry": true},
	"resources/css/app.css": {"file": "assets/app.css", "src": "resources/css/app.css", "isEntry": true}
}`

func pagesHarness(t *testing.T) *harness {
	t.Helper()

	built := fstest.MapFS{"build/.vite/manifest.json": {Data: []byte(manifest)}}

	return newHarnessWith(t, events.Options{}, vukem.WithVite(built))
}

// cookie logs in the way the browser does: a launch code redeemed at /auth/launch.
func cookie(h *harness) string {
	h.t.Helper()

	code := h.do(req{method: http.MethodPost, path: "/api/v1/system/launch", headers: launcherHeaders(launcherSecret)})
	if code.status != http.StatusOK {
		h.t.Fatalf("launch = %d %s", code.status, code.body)
	}

	u, err := url.Parse(code.json(h.t)["url"].(string))
	if err != nil {
		h.t.Fatal(err)
	}

	res := h.do(req{path: "/auth/launch?" + u.RawQuery, noAuth: true})
	if res.status != http.StatusSeeOther {
		h.t.Fatalf("redeem = %d %s", res.status, res.body)
	}

	for _, c := range (&http.Response{Header: res.header}).Cookies() {
		return c.Name + "=" + c.Value
	}

	h.t.Fatal("no session cookie")

	return ""
}

func TestPages(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)
	page := func(path string) resp {
		return h.do(req{path: path, noAuth: true, headers: map[string]string{"Cookie": ck}})
	}

	robots := h.do(req{path: "/robots.txt", noAuth: true})
	if robots.status != http.StatusOK || !strings.Contains(string(robots.body), "User-agent") {
		t.Errorf("robots.txt = %d %s", robots.status, robots.body)
	}

	// The application opens on the dashboard (RF-310), and each section without
	// a page of its own answers with its empty state, never an error.
	home := page("/")
	if home.status != http.StatusOK || !strings.Contains(string(home.body), `Dashboard\/Home`) {
		t.Errorf("/ = %d, want the dashboard home", home.status)
	}

	for _, key := range []string{"edit-video", "write", "publish"} {
		res := page("/" + key)
		if res.status != http.StatusOK || !strings.Contains(string(res.body), `Dashboard\/Section`) ||
			!strings.Contains(string(res.body), `"section":"`+key+`"`) {
			t.Errorf("/%s = %d, want its empty state", key, res.status)
		}
	}

	// The Connections section has its own page (epic 04).
	if res := page(
		"/connections",
	); res.status != http.StatusOK ||
		!strings.Contains(string(res.body), `Connections\/Index`) {
		t.Errorf("/connections = %d, want the connections page", res.status)
	}

	// The redirect address of the sign-in to a social network is a page (epic 18).
	if res := page("/social/callback"); res.status != http.StatusOK ||
		!strings.Contains(string(res.body), `Social\/Callback`) {
		t.Errorf("/social/callback = %d, want the callback page", res.status)
	}

	// The approval queue has its own page (epic 05).
	if res := page("/approvals"); res.status != http.StatusOK ||
		!strings.Contains(string(res.body), `Approvals\/Index`) {
		t.Errorf("/approvals = %d, want the approvals page", res.status)
	}

	if res := h.do(req{path: "/projects", noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("/projects without a session = %d", res.status)
	}

	id := h.newProject("Launch video")

	list := page("/projects")
	if list.status != http.StatusOK || !strings.Contains(string(list.body), `data-page="app"`) ||
		!strings.Contains(string(list.body), `Projects\/Index`) || !strings.Contains(string(list.body), id) {
		t.Errorf("/projects = %d", list.status)
	}

	ed := page("/editor/" + id)
	if ed.status != http.StatusOK || !strings.Contains(string(ed.body), `id="vv-bootstrap"`) ||
		!strings.Contains(
			string(ed.body),
			`"project_id":"`+id+`"`,
		) || !strings.Contains(string(ed.body), `"api_base":"/api/v1"`) ||
		!strings.Contains(
			string(ed.body),
			`"mode":"local"`,
		) || !strings.Contains(string(ed.body), "assets/editor.js") ||
		!strings.Contains(string(ed.body), `id="editor"`) {
		t.Errorf("/editor = %d\n%s", ed.status, ed.body)
	}

	if strings.Contains(string(ed.body), canary) {
		t.Error("the editor page carries the local token")
	}

	for _, p := range []string{"/editor/prj_0000000000", "/editor/not-an-id"} {
		if res := page(p); res.status != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, res.status)
		}
	}

	create := h.do(req{
		method: http.MethodPost, path: "/projects", body: `{"name":"From the form","preset":"16:9"}`, noAuth: true,
		headers: map[string]string{"Cookie": ck, "X-Inertia": "true", "Origin": h.srv.URL},
	})
	if create.status != http.StatusConflict ||
		!strings.HasPrefix(create.header.Get("X-Inertia-Location"), "/editor/prj_") {
		t.Errorf("create = %d, location %q\n%s", create.status, create.header.Get("X-Inertia-Location"), create.body)
	}
}

// The consent screen of a social network sends the browser back by a cross-site navigation, which carries
// no SameSite=Strict session cookie: the sign-in page is drawn all the same, with nothing of the workspace
// in it, and the call it makes, POST /api/v1/social/complete, still needs the session. Every other page
// and the API still answer 401 without one (SDD-020 section 6).
func TestSignInPageIsDrawnWithoutTheSessionCookie(t *testing.T) {
	t.Parallel()

	h := pagesHarness(t)

	navigation := map[string]string{
		"Accept":         "text/html",
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
	}

	res := h.do(req{path: "/social/callback?state=s&code=c", noAuth: true, headers: navigation})
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `Social\/Callback`) {
		t.Fatalf("the sign-in page without a session = %d, want the page", res.status)
	}

	if strings.Contains(string(res.body), `"auth"`) || strings.Contains(string(res.body), "prj_") {
		t.Errorf("the sign-in page carries data of the workspace: %.400s", res.body)
	}

	for name, r := range map[string]req{
		"another page": {path: "/connections", noAuth: true, headers: map[string]string{"Accept": "text/html"}},
		"the completion": {
			method: http.MethodPost, path: "/api/v1/social/complete", body: `{"state":"s","code":"c"}`,
			noAuth: true, headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
		},
	} {
		if got := h.do(r); got.status != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", name, got.status)
		}
	}

	// The exemption is the GET of the page: the path takes no other method.
	if got := h.do(req{
		method: http.MethodPost, path: "/social/callback", body: `{}`, noAuth: true,
		headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
	}); got.status != http.StatusUnauthorized && got.status != http.StatusMethodNotAllowed {
		t.Errorf("a POST of the page without a session = %d, want 401 or 405", got.status)
	}
}

// A project that the editor cannot open answers with the Inertia error page carrying the code, the
// message and the hint of the service, never a bare "500 Internal Server Error".
func TestEditorPageShowsWhyAProjectCannotOpen(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)
	id := h.newProject("From the future")

	_, err := h.store.DB().ExecContext(
		context.Background(),
		"UPDATE timelines SET document = replace(document, ?, ?) WHERE id = ?",
		fmt.Sprintf(`"schema_version":%d`, timeline.CurrentSchemaVersion),
		fmt.Sprintf(`"schema_version":%d`, timeline.CurrentSchemaVersion+1),
		id,
	)
	if err != nil {
		t.Fatal(err)
	}

	res := h.do(req{path: "/editor/" + id, noAuth: true, headers: map[string]string{"Cookie": ck}})

	body := string(res.body)
	if res.status == http.StatusInternalServerError || res.status < 400 {
		t.Errorf("status = %d, want a client error", res.status)
	}

	for _, want := range []string{`data-page="app"`, `Error`, "V_SCHEMA_VERSION", "update stuarty"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q\n%s", want, body)
		}
	}
}

// The language is a preference of each browser (SDD-012 section 16.3): the
// Stuarty-Lang header decides, the shared lang cookie is neither read nor
// written, and the old POST /locale is gone.
func TestPagesLanguageIsPerBrowser(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)
	page := func(headers map[string]string, query string) resp {
		headers["Cookie"] = ck + "; " + headers["Cookie"]

		return h.do(req{path: "/projects" + query, noAuth: true, headers: headers})
	}

	if got := page(map[string]string{"Stuarty-Lang": "pt-BR"}, "").header.Get("Content-Language"); got != "pt-BR" {
		t.Errorf("the language header = %q, want pt-BR", got)
	}

	if got := page(map[string]string{"Cookie": "lang=pt-BR"}, "").header.Get("Content-Language"); got == "pt-BR" {
		t.Errorf("the lang cookie of another browser was read: %q", got)
	}

	if got := page(map[string]string{"Accept-Language": "pt-BR"}, "").header.Get("Content-Language"); got != "pt-BR" {
		t.Errorf("Accept-Language = %q, want pt-BR as the fallback", got)
	}

	if got := page(map[string]string{}, "?lang=pt-BR").header.Get("Content-Language"); got != "pt-BR" {
		t.Errorf("?lang= = %q, want pt-BR", got)
	}

	res := h.do(req{
		method: http.MethodPost, path: "/locale", body: `{"locale":"pt-BR"}`, noAuth: true,
		headers: map[string]string{"Cookie": ck, "Content-Type": "application/json"},
	})
	if res.status != http.StatusNotFound && res.status != http.StatusMethodNotAllowed {
		t.Errorf("POST /locale = %d, want it gone", res.status)
	}

	if len((&http.Response{Header: res.header}).Cookies()) != 0 {
		t.Error("a response set a cookie for the language")
	}
}
