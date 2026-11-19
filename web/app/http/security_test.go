package httpapi_test

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// concrete turns the path template of an endpoint into a requestable path.
func concrete(path string) string {
	for _, p := range []string{"{id}", "{job_id}", "{asset_id}", "{name}", "{key}"} {
		path = strings.ReplaceAll(path, p, "x")
	}

	return path
}

// session redeems a launch code over HTTP and returns the Cookie header.
const launcherSecret = "launcher-secret-of-the-test-harness-0123456789abcdef"

// session logs in as the desktop shell does: the launch request proves the launcher secret.
func (h *harness) session(t *testing.T) string {
	t.Helper()

	return h.sessionWith(t, launcherSecret)
}

// sessionWith logs in with a launch request that sends the given launcher secret (none when empty).
func (h *harness) sessionWith(t *testing.T, secret string) string {
	t.Helper()

	issued := h.do(req{method: http.MethodPost, path: "/api/v1/system/launch", headers: launcherHeaders(secret)})
	if issued.status != http.StatusOK {
		t.Fatalf("launch = %d %s", issued.status, issued.body)
	}

	code, _ := issued.json(t)["code"].(string)

	res := h.do(
		req{path: "/auth/launch?code=" + code, noAuth: true, headers: map[string]string{"Sec-Fetch-Site": "none"}},
	)
	if res.status != http.StatusSeeOther {
		t.Fatalf("redeem = %d %s", res.status, res.body)
	}

	cookie := res.header.Get("Set-Cookie")
	name, _, _ := strings.Cut(cookie, ";")

	return name
}

func launcherHeaders(secret string) map[string]string {
	if secret == "" {
		return nil
	}

	return map[string]string{auth.LauncherHeader: secret}
}

func TestLaunchFlowGivesACookieSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	issued := h.post("/api/v1/system/launch", "").json(t)
	code, _ := issued["code"].(string)

	if len(code) != 64 || !strings.HasSuffix(issued["url"].(string), "/auth/launch?code="+code) {
		t.Fatalf("launch = %v", issued)
	}

	res := h.do(req{path: "/auth/launch?code=" + code, noAuth: true})
	if res.status != http.StatusSeeOther || res.header.Get("Location") != "/" {
		t.Fatalf("redeem = %d Location %q", res.status, res.header.Get("Location"))
	}

	cookie := res.header.Get("Set-Cookie")
	for _, attr := range []string{"stuarty_session_", "Path=/", "HttpOnly", "SameSite=Strict", "Max-Age=604800"} {
		if !strings.Contains(cookie, attr) {
			t.Errorf("Set-Cookie %q lacks %s", cookie, attr)
		}
	}

	for _, attr := range []string{"Secure", "Domain"} {
		if strings.Contains(cookie, attr) {
			t.Errorf("Set-Cookie %q carries %s on plain HTTP", cookie, attr)
		}
	}

	if res.header.Get("Cache-Control") != "no-store" || res.header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", res.header)
	}

	// The code is single use.
	if again := h.do(req{path: "/auth/launch?code=" + code, noAuth: true}); again.status != http.StatusUnauthorized {
		t.Errorf("second use = %d, want 401", again.status)
	}

	name, _, _ := strings.Cut(cookie, ";")

	// The cookie reads, and its actor is local:ui.
	read := h.do(req{path: "/api/v1/projects", noAuth: true, headers: map[string]string{"Cookie": name}})
	if read.status != http.StatusOK {
		t.Fatalf("cookie read = %d %s", read.status, read.body)
	}

	// Unsafe: needs Origin or Sec-Fetch-Site, and JSON.
	w := func(hd map[string]string, body string) int {
		hd["Cookie"] = name

		return h.do(
			req{method: http.MethodPost, path: "/api/v1/projects", body: body, noAuth: true, headers: hd},
		).status
	}

	if got := w(map[string]string{}, `{"name":"c"}`); got != http.StatusForbidden {
		t.Errorf("cookie POST without Origin = %d, want 403", got)
	}

	if got := w(
		map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "text/plain"},
		`{"name":"c"}`,
	); got != http.StatusForbidden {
		t.Errorf("cookie POST as text/plain = %d, want 403", got)
	}

	// A browser on http://localhost:P sends Host localhost:P with that Origin.
	local := func(hd map[string]string, body string) int {
		hd["Cookie"] = name

		return h.do(req{
			method: http.MethodPost, path: "/api/v1/projects", body: body, noAuth: true, headers: hd,
			host: "localhost:" + h.port,
		}).status
	}

	if got := local(
		map[string]string{"Origin": "http://localhost:" + h.port},
		`{"name":"c"}`,
	); got != http.StatusCreated {
		t.Errorf("cookie POST with Origin and JSON = %d, want 201", got)
	}

	// A wrong bearer does not fall back to the cookie.
	fallback := h.do(req{
		path: "/api/v1/projects", noAuth: true,
		headers: map[string]string{"Cookie": name, "Authorization": "Bearer wrong"},
	})
	if fallback.status != http.StatusUnauthorized {
		t.Errorf("wrong bearer with a valid cookie = %d, want 401", fallback.status)
	}

	// The browser's actor is local:ui.
	proj := h.newProject("ui")
	edit := h.do(req{
		method: http.MethodPost, path: "/api/v1/projects/" + proj + "/edit", noAuth: true,
		body: `{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual","name":"V"}}]}`,
		host: "localhost:" + h.port, headers: map[string]string{"Cookie": name, "Origin": "http://localhost:" + h.port},
	})
	if edit.status != http.StatusOK || edit.json(t)["actor"] != "local:ui" {
		t.Errorf("cookie edit = %d %s", edit.status, edit.body)
	}
}

func TestLaunchCodeExpiresAndFailuresLock(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	code, _ := h.post("/api/v1/system/launch", "").json(t)["code"].(string)

	h.clock.advance(121 * time.Second)

	if res := h.do(req{path: "/auth/launch?code=" + code, noAuth: true}); res.status != http.StatusUnauthorized {
		t.Errorf("a code after 121 seconds = %d, want 401", res.status)
	}

	for range 4 {
		h.do(req{path: "/auth/launch?code=" + strings.Repeat("0", 64), noAuth: true})
	}

	res := h.do(req{path: "/auth/launch?code=" + strings.Repeat("1", 64), noAuth: true})
	if res.status != http.StatusTooManyRequests || res.header.Get("Retry-After") != "60" ||
		res.json(t)["code"] != "E_RATE_LIMITED" {
		t.Errorf("after five failures = %d Retry-After %q %s", res.status, res.header.Get("Retry-After"), res.body)
	}
}

func TestHandshakeProvesTheTokenAndNeedsNoCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	nonce := strings.Repeat("ab", 16)

	res := h.do(req{path: "/api/v1/handshake?nonce=" + nonce, noAuth: true})
	if res.status != http.StatusOK {
		t.Fatalf("handshake = %d %s", res.status, res.body)
	}

	if got := res.json(t)["proof"]; got == nil || len(got.(string)) != 64 {
		t.Errorf("proof = %v", got)
	}

	for _, bad := range []string{"", "xyz", strings.Repeat("AB", 16), strings.Repeat("ab", 17)} {
		if res := h.do(req{path: "/api/v1/handshake?nonce=" + bad, noAuth: true}); res.status != http.StatusBadRequest {
			t.Errorf("nonce %q = %d, want 400", bad, res.status)
		}
	}
}

// Every router route has a row in the table, every Service method has a scope,
// and the table agrees with service.Scopes (SDD-007 section 5.1 item 2, T-05).
func TestSecurity_T05_EveryOperationHasScope(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	rows := map[string]httpapi.Endpoint{}
	for _, ep := range httpapi.Endpoints {
		key := ep.Method + " " + ep.FullPath()
		if _, dup := rows[key]; dup {
			t.Errorf("duplicate row %s", key)
		}

		rows[key] = ep
	}

	// The browser pages are not API operations: they authenticate like the API
	// (authenticate runs first) and call the Service, which checks the scope.
	builtin := []string{
		"GET /up",
		"GET /{$}",
		"GET /projects",
		"POST /projects",
		"GET /projects/{project_id}",
		"GET /editor/{project_id}",
		"GET /settings",
		"GET /settings/brand-kit",
		"GET /login",
		"GET /settings/keys",
		// The empty-state pages of the dashboard sections (web/routes/dashboard.go).
		"GET /edit-video", "GET /edit-image", "GET /write", "GET /visual-assets", "GET /video-assets",
		"GET /publish", "GET /skills", "GET /prompts", "GET /memory", "GET /connections",
	}

	for _, r := range h.vapp.Router.Routes() {
		key := r.Method + " " + r.Path
		if _, ok := rows[key]; !ok && !slices.Contains(builtin, key) {
			t.Errorf("the route %s has no row in the scope table; a route without one is not served", key)
		}
	}

	served := map[string]bool{}
	for _, r := range h.vapp.Router.Routes() {
		served[r.Method+" "+r.Path] = true
	}

	for key := range rows {
		if !served[key] {
			t.Errorf("the row %s has no route", key)
		}
	}

	methods := reflect.TypeFor[service.Service]()
	for method := range methods.Methods() {
		name := method.Name

		scope, ok := service.Scopes[name]
		if !ok {
			t.Errorf("Service.%s has no scope", name)
		}

		found := false

		for _, ep := range httpapi.Endpoints {
			if ep.Service == name {
				found = true

				if ep.Scope != scope {
					t.Errorf("%s needs %q in the service and %q in the route %s", name, scope, ep.Scope, ep.ID)
				}
			}
		}

		if !found {
			t.Errorf("Service.%s has no route", name)
		}
	}

	for name := range service.Scopes {
		if _, ok := methods.MethodByName(name); !ok {
			t.Errorf("service.Scopes names %s, which is not a method of Service", name)
		}
	}

	// Every operation of the Prototype in SDD-000 section 1 has a row.
	for _, id := range []string{
		"projects.list", "projects.create", "projects.get", "projects.edit", "commands.list", "commands.get",
		"projects.history", "projects.undo", "projects.redo", "media.import", "media.inspect", "media.list",
		"projects.frame", "projects.sheet", "projects.lint", "jobs.start", "jobs.get", "jobs.list", "jobs.file",
		"events.stream", "projects.plan", "media.proxy", "media.thumbnail", "media.strip", "media.peaks", "media.lut", "media.chunks", "bitmaps.get",
	} {
		if !slices.ContainsFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id }) {
			t.Errorf("the Prototype operation %s has no row", id)
		}
	}

	// Every operation of ALPHA in SDD-000 section 1 has a row, with the scope of the table.
	for id, scope := range map[string]string{
		"projects.trash": service.ScopeEdit, "projects.restore": service.ScopeEdit,
		"media.trash": service.ScopeMedia, "media.restore": service.ScopeMedia,
		"jobs.cancel":    service.ScopeRender,
		"templates.list": service.ScopeRead, "templates.save": service.ScopeEdit,
		"templates.instantiate": service.ScopeEdit, "templates.export": service.ScopeRead,
		"templates.file": service.ScopeRead, "templates.import": service.ScopeEdit,
		"system.get": service.ScopeRead, "projects.snap_points": service.ScopeRead, "fonts.list": service.ScopeRead,
		"jobs.file": service.ScopeRead, "audit.list": service.ScopeAdmin, "trash.empty": service.ScopeAdmin,
		"system.tools.install": service.ScopeAdmin, "system.tools": service.ScopeRead,
	} {
		i := slices.IndexFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id })
		if i < 0 {
			t.Errorf("the ALPHA operation %s has no row", id)
			continue
		}

		if got := httpapi.Endpoints[i].Scope; got != scope {
			t.Errorf("%s needs %q, SDD-000 section 1 says %q", id, got, scope)
		}
	}

	for _, id := range []string{"keys.create", "keys.list", "keys.revoke"} {
		if !slices.ContainsFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id }) {
			t.Errorf("the key route %s has no row", id)
		}
	}

	// Every operation of ALPHA in SDD-000 section 1 has a row, with the scope of the table.
	for id, scope := range map[string]string{
		"projects.trash": service.ScopeEdit, "projects.restore": service.ScopeEdit,
		"media.trash": service.ScopeMedia, "media.restore": service.ScopeMedia,
		"jobs.cancel":    service.ScopeRender,
		"templates.list": service.ScopeRead, "templates.save": service.ScopeEdit,
		"templates.instantiate": service.ScopeEdit, "templates.export": service.ScopeRead,
		"templates.file": service.ScopeRead, "templates.import": service.ScopeEdit,
		"system.get": service.ScopeRead, "projects.snap_points": service.ScopeRead, "fonts.list": service.ScopeRead,
		"audit.list": service.ScopeAdmin, "trash.empty": service.ScopeAdmin,
		"system.tools.install": service.ScopeAdmin, "system.tools": service.ScopeRead,
	} {
		i := slices.IndexFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id })
		if i < 0 {
			t.Errorf("the ALPHA operation %s has no row", id)
			continue
		}

		if got := httpapi.Endpoints[i].Scope; got != scope {
			t.Errorf("%s needs %q, SDD-000 section 1 says %q", id, got, scope)
		}
	}

	for _, id := range []string{"keys.create", "keys.list", "keys.revoke"} {
		if !slices.ContainsFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id }) {
			t.Errorf("the key route %s has no row", id)
		}
	}

	for _, c := range command.Default().List() {
		if c.Scope != command.ScopeEdit && c.Scope != command.ScopeMedia {
			t.Errorf("command %s has scope %q", c.Name, c.Scope)
		}
	}
}

// POST /mcp is behind the guard and the authentication, and the Caller of the
// bearer token reaches the tool handlers; GET and DELETE answer 405.
func TestMCPEndpointIsGuardedAndAuthenticated(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	const call = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project_list","arguments":{}}}`

	headers := map[string]string{"Accept": "application/json, text/event-stream"}

	res := h.do(req{method: http.MethodPost, path: "/mcp", body: call, headers: headers})
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"result"`) ||
		strings.Contains(string(res.body), "E_UNAUTHORIZED") {
		t.Fatalf("an authenticated call = %d %s", res.status, res.body)
	}

	if res := h.do(
		req{method: http.MethodPost, path: "/mcp", body: call, headers: headers, noAuth: true},
	); res.status != http.StatusUnauthorized {
		t.Errorf("no credential = %d, want 401", res.status)
	}

	if res := h.do(
		req{method: http.MethodPost, path: "/mcp", body: call, headers: headers, host: "evil.example:" + h.port},
	); res.status < 400 {
		t.Errorf("a foreign Host = %d, want a refusal", res.status)
	}

	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		if res := h.do(req{method: m, path: "/mcp"}); res.status != http.StatusMethodNotAllowed {
			t.Errorf("%s /mcp = %d, want 405", m, res.status)
		}
	}
}

// T-28: the local bearer token cannot empty the trash, a browser session can,
// and the operation is on no agent surface.
func TestSecurity_T28_TrashEmptyNeedsHumanSession(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("trash")

	if res := h.post("/api/v1/projects/"+id+"/trash", ""); res.status != http.StatusOK {
		t.Fatalf("trash = %d %s", res.status, res.body)
	}

	if res := h.post("/api/v1/trash/empty", "{}"); res.status != http.StatusForbidden ||
		res.json(t)["code"] != "E_FORBIDDEN" {
		t.Errorf("the local token = %d %s, want 403 E_FORBIDDEN", res.status, res.body)
	}

	if res := h.get("/api/v1/projects/" + id); res.status != http.StatusOK {
		t.Fatalf("a refused call deleted the project: %d", res.status)
	}

	cookie := h.session(t)

	res := h.do(req{
		method: http.MethodPost, path: "/api/v1/trash/empty", body: "{}", noAuth: true,
		headers: map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL},
	})
	if res.status != http.StatusOK || res.json(t)["projects"] != float64(1) {
		t.Errorf("a browser session = %d %s", res.status, res.body)
	}

	if res := h.get("/api/v1/projects/" + id); res.status != http.StatusNotFound {
		t.Errorf("the emptied project = %d, want 404", res.status)
	}
}

// T-28 again, from the other side (SDD-007 section 5.3): a launch code asked for with the bearer
// token alone (`stuarty open`, an agent) or with a wrong launcher secret redeems into a
// session that works as the editor but has none of the browser-only privileges.
func TestSecurity_T28_BearerLaunchCodeGivesNoBrowserPrivileges(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for name, secret := range map[string]string{
		"no secret":    "",
		"wrong secret": launcherSecret + "x",
		"shorter":      launcherSecret[:8],
		"blank":        " ",
	} {
		id := h.newProject(name)
		if res := h.post("/api/v1/projects/"+id+"/trash", ""); res.status != http.StatusOK {
			t.Fatalf("%s: trash = %d %s", name, res.status, res.body)
		}

		cookie := h.sessionWith(t, secret)
		headers := map[string]string{"Cookie": cookie, "Sec-Fetch-Site": "same-origin", "Origin": h.srv.URL}

		// The session is a working editor session.
		read := h.do(req{path: "/api/v1/projects/" + id, noAuth: true, headers: headers})
		if read.status != http.StatusOK {
			t.Errorf("%s: read with the session = %d %s", name, read.status, read.body)
		}

		for _, r := range []req{
			{method: http.MethodPost, path: "/api/v1/trash/empty", body: "{}"},
			{method: http.MethodPost, path: "/api/v1/system/tools/install", body: `{"name":"whisper","model":"tiny"}`},
		} {
			r.noAuth, r.headers = true, headers
			if res := h.do(r); res.status != http.StatusForbidden || res.json(t)["code"] != "E_FORBIDDEN" {
				t.Errorf("%s: %s = %d %s, want 403 E_FORBIDDEN", name, r.path, res.status, res.body)
			}
		}

		if res := h.get("/api/v1/projects/" + id); res.status != http.StatusOK {
			t.Errorf("%s: the project did not survive: %d", name, res.status)
		}
	}
}

// The install step is a person's click: the local token and a key without
// admin are refused before any download starts, and a route nobody listed is 404
// (SDD-000 section 1, SDD-007 section 6.8, T-26). Listing what is installed is a read (VV-080).
func TestInstallRouteRefusesTheLocalToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, r := range []req{
		{method: http.MethodPost, path: "/api/v1/system/tools/install", body: `{"name":"whisper","model":"tiny"}`},
	} {
		if res := h.do(r); res.status != http.StatusForbidden || res.json(t)["code"] != "E_FORBIDDEN" {
			t.Errorf("%s %s with the local token = %d %s, want 403 E_FORBIDDEN", r.method, r.path, res.status, res.body)
		}
	}

	if res := h.get("/api/v1/system/tools"); res.status != http.StatusOK {
		t.Errorf("GET /api/v1/system/tools with the local token = %d %s, want 200", res.status, res.body)
	}

	if res := h.do(req{method: http.MethodPost, path: "/api/v1/system/tools/install", noAuth: true}); res.status !=
		http.StatusUnauthorized {
		t.Errorf("no credential = %d, want 401", res.status)
	}

	if res := h.get("/api/v1/unlisted"); res.status != http.StatusNotFound {
		t.Errorf("an unlisted route = %d, want 404", res.status)
	}
}

// VV-170: two local servers on one host must not share a cookie name, or opening one signs the other out.
func TestLocalSessionCookieNamesThePortOfTheServer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	cookie := h.session(t)
	name, id, _ := strings.Cut(cookie, "=")

	if want := "stuarty_session_" + h.port; name != want {
		t.Fatalf("cookie name = %q, want %q", name, want)
	}

	if res := h.do(
		req{path: "/api/v1/projects", noAuth: true, headers: map[string]string{"Cookie": name + "=" + id}},
	); res.status != http.StatusOK {
		t.Errorf("the session cookie of this server = %d", res.status)
	}

	bare := "stuarty_session=" + id
	if res := h.do(
		req{path: "/api/v1/projects", noAuth: true, headers: map[string]string{"Cookie": bare}},
	); res.status != http.StatusUnauthorized {
		t.Errorf("a cookie without the port = %d, want 401", res.status)
	}
}
