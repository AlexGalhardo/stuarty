package httpapi_test

import (
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
)

// concrete turns the path template of an endpoint into a requestable path.
func concrete(path string) string {
	for _, p := range []string{"{id}", "{job_id}", "{asset_id}", "{name}", "{key}"} {
		path = strings.ReplaceAll(path, p, "x")
	}

	return path
}

func TestSecurity_T02_HostHeaderRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	p := h.port

	for _, host := range []string{
		"evil.example:" + p, "127.0.0.1", "127.1:" + p, "localhost.:" + p, "foo.localhost:" + p,
		"[::ffff:127.0.0.1]:" + p, "127.0.0.1:1", "2130706433:" + p, "0x7f.1:" + p, "0.0.0.0:" + p,
		"user@127.0.0.1:" + p, "127.0.0.1:" + p + "/x", "localhost", "[::1]",
	} {
		res := h.do(req{path: "/api/v1/projects", host: host})
		if res.status != http.StatusForbidden || res.json(t)["code"] != "E_FORBIDDEN" ||
			res.json(t)["message"] != "Request refused." {
			t.Errorf("Host %q = %d %s; want 403 E_FORBIDDEN Request refused.", host, res.status, res.body)
		}

		if _, hasHint := res.json(t)["hint"]; hasHint {
			t.Errorf("Host %q: the refusal carries a hint", host)
		}
	}

	for _, host := range []string{"127.0.0.1:" + p, "localhost:" + p, "LOCALHOST:" + p, "[::1]:" + p} {
		if res := h.do(req{path: "/api/v1/projects", host: host}); res.status != http.StatusOK {
			t.Errorf("Host %q = %d, want 200", host, res.status)
		}
	}

	// The launch route is checked too, before the code is looked at.
	if res := h.do(
		req{path: "/auth/launch?code=x", host: "evil.example:" + p, noAuth: true},
	); res.status != http.StatusForbidden {
		t.Errorf("launch with a bad Host = %d", res.status)
	}
}

func TestSecurity_T01_CrossOriginRequestsRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	p := h.port

	bad := []map[string]string{
		{"Origin": "https://evil.example"},
		{"Origin": "null"},
		{"Origin": "http://localhost:3000"},
		{"Origin": "http://127.0.0.1:1"},
		{"Origin": "http://LOCALHOST:" + p},
		{"Origin": "https://localhost:" + p},
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Sec-Fetch-Site": ""},
	}

	for _, ep := range httpapi.Endpoints {
		for _, hd := range bad {
			method := ep.Method

			res := h.do(req{method: method, path: concrete(ep.FullPath()), headers: hd})
			if res.status != http.StatusForbidden {
				t.Errorf("%s %s with %v = %d, want 403", method, ep.FullPath(), hd, res.status)
			}
		}
	}

	for _, ok := range []map[string]string{
		{"Origin": "http://127.0.0.1:" + p},
		{"Origin": "http://localhost:" + p},
		{"Origin": "http://[::1]:" + p},
		{"Sec-Fetch-Site": "same-origin"},
		{"Sec-Fetch-Site": "none"},
	} {
		if res := h.do(req{path: "/api/v1/projects", headers: ok}); res.status != http.StatusOK {
			t.Errorf("%v = %d, want 200", ok, res.status)
		}
	}
}

func TestSecurity_T01_SameSiteDifferentPortRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	// A page on another port of localhost is same-site: only the exact Origin keeps it out.
	res := h.do(req{
		method: http.MethodPost, path: "/api/v1/projects", body: `{"name":"x"}`,
		headers: map[string]string{"Origin": "http://localhost:" + otherPort(h.port), "Sec-Fetch-Site": "same-site"},
	})
	if res.status != http.StatusForbidden {
		t.Errorf("same-site request from another port = %d, want 403", res.status)
	}

	cookie := h.session(t)

	res = h.do(req{
		method: http.MethodPost, path: "/api/v1/projects", body: `{"name":"x"}`, noAuth: true,
		headers: map[string]string{"Cookie": cookie, "Origin": "http://localhost:" + otherPort(h.port)},
	})
	if res.status != http.StatusForbidden {
		t.Errorf("cookie request from another port = %d, want 403", res.status)
	}
}

func otherPort(p string) string {
	if p == "3000" {
		return "3001"
	}

	return "3000"
}

// No response carries Access-Control-*, in any status, and OPTIONS is 405.
func TestSecurity_T01_NoCORSHeaders(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, r := range []req{
		{path: "/api/v1/projects"},
		{path: "/api/v1/projects", noAuth: true},
		{path: "/api/v1/nothing"},
		{path: "/up", noAuth: true},
		{path: "/api/v1/projects", headers: map[string]string{"Origin": "https://evil.example"}},
		{method: http.MethodOptions, path: "/api/v1/projects"},
		{method: http.MethodOptions, path: "/api/v1/projects", noAuth: true},
		{
			method: http.MethodOptions, path: "/nothing", noAuth: true,
			headers: map[string]string{"Access-Control-Request-Method": "POST", "Origin": "https://evil.example"},
		},
	} {
		res := h.do(r)

		for name := range res.header {
			if strings.HasPrefix(strings.ToLower(name), "access-control-") {
				t.Errorf("%s %s carries %s", r.method, r.path, name)
			}
		}

		if r.method == http.MethodOptions && res.status != http.StatusMethodNotAllowed &&
			res.status != http.StatusForbidden {
			t.Errorf("OPTIONS %s = %d, want 405", r.path, res.status)
		}
	}

	if res := h.do(
		req{method: http.MethodOptions, path: "/api/v1/projects"},
	); res.status != http.StatusMethodNotAllowed {
		t.Errorf("OPTIONS with credentials = %d, want 405", res.status)
	}
}

func TestSecurity_T04_CredentialInQueryRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, name := range []string{"token", "key", "access_token", "api_key"} {
		res := h.do(req{path: "/api/v1/projects?" + name + "=" + canary, noAuth: true})
		if res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
			t.Errorf("?%s= = %d %s; want 400 E_VALIDATION", name, res.status, res.body)
		}

		if strings.Contains(string(res.body), canary) {
			t.Errorf("?%s= echoed the credential", name)
		}
	}

	if logs := h.app.Logs(); strings.Contains(logs, canary) {
		t.Error("the credential of a query string reached the log")
	}
}

// The canary token appears in no log, no body and no header, whatever the outcome.
func TestSecurity_T04_SecretsNeverAppear(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, r := range []req{
		{path: "/api/v1/projects"},
		{path: "/api/v1/projects", headers: map[string]string{"Authorization": "Bearer " + canary + "x"}, noAuth: true},
		{path: "/api/v1/projects?token=" + canary, noAuth: true},
		{method: http.MethodPost, path: "/api/v1/projects", body: `{"name":`},
		{path: "/api/v1/projects", host: "evil.example"},
		{path: "/openapi.json"},
	} {
		res := h.do(r)
		if strings.Contains(string(res.body), canary) {
			t.Errorf("%s: the token is in the body", r.path)
		}

		for name, vs := range res.header {
			if strings.Contains(strings.Join(vs, ","), canary) {
				t.Errorf("%s: the token is in the header %s", r.path, name)
			}
		}
	}

	if logs := h.app.Logs(); strings.Contains(logs, canary) {
		t.Errorf("the token is in the log:\n%s", logs)
	}
}

// session redeems a launch code over HTTP and returns the Cookie header.
func (h *harness) session(t *testing.T) string {
	t.Helper()

	issued := h.post("/api/v1/system/launch", "")
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
	for _, attr := range []string{"vukem_video_session=", "Path=/", "HttpOnly", "SameSite=Strict", "Max-Age=604800"} {
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
		"GET /up", "GET /{$}", "GET /projects", "POST /projects", "GET /editor/{project_id}", "POST /locale",
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
		"events.stream", "projects.plan", "media.proxy", "media.thumbnail", "media.strip", "media.peaks", "bitmaps.get",
	} {
		if !slices.ContainsFunc(httpapi.Endpoints, func(e httpapi.Endpoint) bool { return e.ID == id }) {
			t.Errorf("the Prototype operation %s has no row", id)
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
