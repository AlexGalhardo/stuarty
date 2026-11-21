package httpapi_test

import (
	"bufio"
	"bytes"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
)

// hostRow is one line of testdata/hosts.txt with the port filled in.
type hostRow struct {
	kind   string
	value  string
	status int
}

func hostRows(t *testing.T, kind, port string) []hostRow {
	t.Helper()

	data, err := os.ReadFile("testdata/hosts.txt")
	if err != nil {
		t.Fatal(err)
	}

	var rows []hostRow

	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		f := strings.Split(line, "\t")
		if len(f) != 3 {
			t.Fatalf("hosts.txt: bad line %q", line)
		}

		status, err := strconv.Atoi(f[2])
		if err != nil {
			t.Fatalf("hosts.txt: bad status in %q", line)
		}

		if f[0] != kind {
			continue
		}

		v := strings.ReplaceAll(f[1], "P", port)
		if v == "empty" {
			v = ""
		}

		rows = append(rows, hostRow{kind: f[0], value: v, status: status})
	}

	if len(rows) == 0 {
		t.Fatalf("hosts.txt has no %s rows", kind)
	}

	return rows
}

func TestSecurity_T02_HostHeaderRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	p := h.port

	for _, row := range hostRows(t, "host", p) {
		res := h.do(req{path: "/api/v1/projects", host: row.value})
		if res.status != row.status {
			t.Errorf("Host %q = %d %s; want %d", row.value, res.status, res.body, row.status)
		}

		if row.status != http.StatusForbidden {
			continue
		}

		if res.json(t)["code"] != "E_FORBIDDEN" || res.json(t)["message"] != "Request refused." {
			t.Errorf("Host %q = %s; want E_FORBIDDEN Request refused.", row.value, res.body)
		}

		if _, hasHint := res.json(t)["hint"]; hasHint {
			t.Errorf("Host %q: the refusal carries a hint", row.value)
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

	headers := func(row hostRow) map[string]string {
		if row.kind == "origin" {
			return map[string]string{"Origin": row.value}
		}

		return map[string]string{"Sec-Fetch-Site": row.value}
	}

	var rows []hostRow

	rows = append(rows, hostRows(t, "origin", p)...)
	rows = append(rows, hostRows(t, "fetch-site", p)...)

	for _, row := range rows {
		if row.status != http.StatusForbidden {
			continue
		}

		for _, ep := range httpapi.Endpoints {
			res := h.do(req{method: ep.Method, path: concrete(ep.FullPath()), headers: headers(row)})
			if res.status != http.StatusForbidden {
				t.Errorf("%s %s with %v = %d, want 403", ep.Method, ep.FullPath(), headers(row), res.status)
			}
		}
	}

	for _, row := range rows {
		if row.status != http.StatusOK {
			continue
		}

		if res := h.do(req{path: "/api/v1/projects", headers: headers(row)}); res.status != http.StatusOK {
			t.Errorf("%v = %d, want 200", headers(row), res.status)
		}
	}
}

func TestSecurity_T01_SameSiteDifferentPortRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	// A page on another port of localhost is same-site: only the exact Origin keeps it out.
	res := h.do(req{
		method: http.MethodPost, path: "/api/v1/projects", body: projectJSON("x"),
		headers: map[string]string{"Origin": "http://localhost:" + otherPort(h.port), "Sec-Fetch-Site": "same-site"},
	})
	if res.status != http.StatusForbidden {
		t.Errorf("same-site request from another port = %d, want 403", res.status)
	}

	cookie := h.session(t)

	res = h.do(req{
		method: http.MethodPost, path: "/api/v1/projects", body: projectJSON("x"), noAuth: true,
		headers: map[string]string{"Cookie": cookie, "Origin": "http://localhost:" + otherPort(h.port)},
	})
	if res.status != http.StatusForbidden {
		t.Errorf("cookie request from another port = %d, want 403", res.status)
	}
}

// The redirect from a consent screen is a cross-site navigation, and the sign-in page is the one place
// it may land (SDD-007 section 3.6 item 4): a GET that loads a document, of that path and no other.
func TestSecurity_T01_SignInRedirectIsTheOnlyCrossSiteNavigationAllowed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	navigate := map[string]string{
		"Sec-Fetch-Site": "cross-site",
		"Sec-Fetch-Mode": "navigate",
		"Sec-Fetch-Dest": "document",
	}

	if res := h.do(req{path: httpapi.SignInCallbackPath, headers: navigate}); res.status == http.StatusForbidden {
		t.Errorf("the cross-site navigation to the sign-in page = %d, want it let in", res.status)
	}

	for name, r := range map[string]req{
		"another page":              {path: "/connections", headers: navigate},
		"the API":                   {path: "/api/v1/projects", headers: navigate},
		"the callback by a fetch":   {path: httpapi.SignInCallbackPath, headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Dest": "empty"}},
		"the callback in a frame":   {path: httpapi.SignInCallbackPath, headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"}},
		"the callback by a POST":    {method: http.MethodPost, path: httpapi.SignInCallbackPath, body: "{}", headers: navigate},
		"the API complete by POST":  {method: http.MethodPost, path: "/api/v1/social/complete", body: `{"state":"x"}`, headers: navigate},
		"a callback with an Origin": {path: httpapi.SignInCallbackPath, headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document", "Origin": "http://evil.example"}},
	} {
		if res := h.do(r); res.status != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", name, res.status)
		}
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

// The local token, a launch code and a session id appear in no log, no body and
// no header of any route, whatever the outcome (SDD-007 section 15 item 7). The
// bundle and the environment half of the check are in
// TestSecurity_T04_BundleAndEnvironmentHoldNoSecret (web/app/servicetest).
func TestSecurity_T04_SecretsNeverAppear(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	code, _ := h.post("/api/v1/system/launch", "").json(t)["code"].(string)
	cookie := h.session(t)
	cookieName, sessionID, _ := strings.Cut(cookie, "=")

	secrets := map[string]string{"local token": canary, "launch code": code, "session id": sessionID}

	for name, s := range secrets {
		if s == "" {
			t.Fatalf("no %s to look for", name)
		}
	}

	wrong := strings.Repeat("0", 8) + canary[8:]
	creds := []map[string]string{
		{"Authorization": "Bearer " + canary},
		{"Authorization": "Bearer " + wrong},
		{"Cookie": cookie},
		{"Cookie": cookieName + "=" + sessionID + "x"},
		{},
	}

	var seen []resp

	for _, ep := range httpapi.Endpoints {
		// The approval stream never ends for a caller with a session: it is read with a deadline by
		// TestApprovalsStreamSendsThePendingAndThenTheDecisions, which checks its frames for the token; the
		// publication stream is the same kind of stream.
		if ep.ID == "approvals.events" || ep.ID == "publications.events" {
			continue
		}

		for _, bad := range []string{"", `{"name":`, `{"unknown":1}`} {
			for _, hd := range creds {
				seen = append(seen, h.do(req{
					method: ep.Method, path: concrete(ep.FullPath()) + "?cursor=" + canary[:12], body: bad,
					noAuth: true, headers: hd,
				}))
			}
		}
	}

	seen = append(seen,
		h.do(req{path: "/api/v1/projects?token=" + canary, noAuth: true}),
		h.do(req{path: "/api/v1/projects?api_key=" + code, noAuth: true}),
		h.do(req{path: "/auth/launch?code=" + code, noAuth: true}),
		h.do(req{path: "/api/v1/projects", host: "evil.example"}),
		h.do(req{path: "/openapi.json"}),
	)

	for i, res := range seen {
		for name, s := range secrets {
			if strings.Contains(string(res.body), s) {
				t.Errorf("response %d: the %s is in the body", i, name)
			}

			for hn, vs := range res.header {
				if hn == "Set-Cookie" {
					continue // the one place the session id is meant to be, and it is a cookie
				}

				if strings.Contains(strings.Join(vs, ","), s) {
					t.Errorf("response %d: the %s is in the header %s", i, name, hn)
				}
			}
		}
	}

	logs := h.app.Logs()
	for name, s := range secrets {
		if strings.Contains(logs, s) {
			t.Errorf("the %s is in the log", name)
		}
	}
}

func TestSystemLaunchRefusesABodyThatIsNotJSON(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	if got := h.post("/api/v1/system/launch", "not json"); got.status != http.StatusBadRequest {
		t.Fatalf("non-JSON body = %d %s, want 400", got.status, got.body)
	}

	for _, body := range []string{"", "{}"} {
		if got := h.post("/api/v1/system/launch", body); got.status != http.StatusOK {
			t.Fatalf("body %q = %d %s, want 200", body, got.status, got.body)
		}
	}
}
