package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/auth"
)

const (
	netHost    = "video.example.com"
	netOrigin  = "https://" + netHost
	cookieName = "__Host-vukem_video_session"
	goodPass   = "correct horse battery"
)

// netResp is what a test reads of a response of the network harness.
type netResp struct {
	status int
	header http.Header
	body   []byte
	cookie *http.Cookie
}

func (r netResp) message() string {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	msg, _ := m["message"].(string)

	return msg
}

func (r netResp) code() string {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	code, _ := m["code"].(string)

	return code
}

// send makes a request as a browser at APP_URL would, with the session cookie
// when value is not empty.
func (h *networkHarness) send(t *testing.T, method, path, body, cookie string, headers map[string]string) netResp {
	t.Helper()

	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}

	hr, err := http.NewRequest(method, h.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = netHost
	hr.Header.Set("Origin", netOrigin)

	if body != "" || method != http.MethodGet {
		hr.Header.Set("Content-Type", "application/json")
	}

	if cookie != "" {
		hr.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}

	for k, v := range headers {
		hr.Header.Set(k, v)
	}

	client := *h.srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)
	out := netResp{status: res.StatusCode, header: res.Header, body: raw}

	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			out.cookie = c
		}
	}

	return out
}

func (h *networkHarness) user(t *testing.T, email, role string) auth.User {
	t.Helper()

	u, err := h.auth.Users.Create(t.Context(), email, goodPass, role)
	if err != nil {
		t.Fatal(err)
	}

	return u
}

func (h *networkHarness) login(t *testing.T, email, password string) netResp {
	t.Helper()

	return h.send(t, http.MethodPost, "/auth/login", `{"email":"`+email+`","password":"`+password+`"}`, "", nil)
}

func (h *networkHarness) session(t *testing.T, email string) string {
	t.Helper()

	res := h.login(t, email, goodPass)
	if res.status != http.StatusOK || res.cookie == nil {
		t.Fatalf("login = %d %s", res.status, res.body)
	}

	return res.cookie.Value
}

// SDD-007 section 15 check 12, T-32: the cookie has the __Host- prefix, Secure,
// HttpOnly and SameSite=Lax, and the session id changes at login.
func TestSecurity_T32_SessionCookieAttributes(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleAdmin)

	first := h.login(t, "ana@example.com", goodPass)
	if first.status != http.StatusOK || first.cookie == nil {
		t.Fatalf("login = %d %s", first.status, first.body)
	}

	c := first.cookie
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("cookie = %+v, want Secure, HttpOnly, SameSite=Lax, Path=/, no Domain", c)
	}

	if !strings.HasPrefix(c.Name, "__Host-") {
		t.Errorf("cookie name = %q, want the __Host- prefix", c.Name)
	}

	if !strings.Contains(string(first.body), `"role":"admin"`) || strings.Contains(string(first.body), goodPass) {
		t.Errorf("login answer = %s", first.body)
	}

	// Logging in again with the cookie of the first login rotates the id and
	// ends the old session.
	second := h.send(t, http.MethodPost, "/auth/login",
		`{"email":"ana@example.com","password":"`+goodPass+`"}`, c.Value, nil)
	if second.cookie == nil || second.cookie.Value == c.Value {
		t.Fatalf("the session id did not change at login: %+v", second.cookie)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", c.Value, nil); res.status != http.StatusUnauthorized {
		t.Errorf("the old session id = %d, want 401", res.status)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", second.cookie.Value, nil); res.status != http.StatusOK {
		t.Errorf("the new session = %d %s, want 200", res.status, res.body)
	}
}

// The container on the user's own machine uses the local-mode cookie name over
// plain HTTP (SDD-007 section 4.3 item 4).
func TestSessionCookieOverLoopbackHTTP(t *testing.T) {
	res := resolved()
	res.AppURL, res.LoopbackHTTP = "http://localhost:7878", true
	h := newNetworkHarness(t, res, false)
	h.user(t, "ana@example.com", service.RoleMember)

	hr, err := http.NewRequest(http.MethodPost, h.srv.URL+"/auth/login",
		strings.NewReader(`{"email":"ana@example.com","password":"`+goodPass+`"}`))
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = "localhost:7878"
	hr.Header.Set("Content-Type", "application/json")

	got, err := h.srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	defer got.Body.Close()

	var c *http.Cookie

	for _, ck := range got.Cookies() {
		c = ck
	}

	if c == nil || c.Name != "vukem_video_session" || c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = %+v, want the local-mode name, HttpOnly, Lax and not Secure", c)
	}
}

// An idle session of 8 hours and 1 minute is refused; one of 7 hours 59 minutes
// is kept and renewed (SDD-007 section 4.1 item 4).
func TestSessionIdleTimeout(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	idle := h.session(t, "ana@example.com")
	h.clk.advance(8*time.Hour + time.Minute)

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", idle, nil); res.status != http.StatusUnauthorized {
		t.Errorf("an idle session of 8h01m = %d, want 401", res.status)
	}

	live := h.session(t, "ana@example.com")
	h.clk.advance(7*time.Hour + 59*time.Minute)

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", live, nil); res.status != http.StatusOK {
		t.Errorf("a session idle for 7h59m = %d, want 200", res.status)
	}
}

func TestSessionAbsoluteLifetime(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	id := h.session(t, "ana@example.com")
	start := h.clk.now()

	// In use every 6 hours, so it never goes idle.
	for h.clk.now().Sub(start)+6*time.Hour < 7*24*time.Hour {
		h.clk.advance(6 * time.Hour)

		if res := h.send(t, http.MethodGet, "/api/v1/projects", "", id, nil); res.status != http.StatusOK {
			t.Fatalf("a session in use after %s = %d, want 200", h.clk.now().Sub(start), res.status)
		}
	}

	h.clk.advance(7*24*time.Hour + time.Minute - h.clk.now().Sub(start))

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", id, nil); res.status != http.StatusUnauthorized {
		t.Errorf("a session older than 7 days = %d, want 401", res.status)
	}
}

func TestLoginWrongPasswordAndUnknownEmailAreTheSameAnswer(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	wrong := h.login(t, "ana@example.com", "not the password")
	unknown := h.login(t, "nobody@example.com", goodPass)

	for _, r := range []netResp{wrong, unknown} {
		if r.status != http.StatusUnauthorized || r.code() != service.CodeUnauthorized || r.cookie != nil {
			t.Errorf("answer = %d %s cookie=%v", r.status, r.body, r.cookie)
		}
	}

	if wrong.message() != unknown.message() {
		t.Errorf("the two answers differ: %q and %q", wrong.message(), unknown.message())
	}
}

// Repeated attempts are throttled by vukem's login keys (SDD-007 section 4.1
// item 1): per email and address, per email, per address.
func TestSecurity_T32_LoginThrottled(t *testing.T) {
	h := newNetworkHarnessLimits(t, resolved(), true, nil)
	h.user(t, "ana@example.com", service.RoleMember)

	for i := range 5 {
		if res := h.login(t, "ana@example.com", "wrong password here"); res.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, res.status)
		}
	}

	res := h.login(t, "ana@example.com", goodPass)
	if res.status != http.StatusTooManyRequests || res.code() != service.CodeRateLimited ||
		res.header.Get("Retry-After") == "" {
		t.Errorf("the 6th attempt = %d %s, Retry-After %q", res.status, res.body, res.header.Get("Retry-After"))
	}

	if res.cookie != nil {
		t.Error("a throttled attempt got a cookie")
	}
}

func TestLoginThrottledPerAddress(t *testing.T) {
	h := newNetworkHarnessLimits(t, resolved(), true, nil)

	var last netResp

	for i := range 31 {
		last = h.login(t, "user"+strings.Repeat("x", i)+"@example.com", "wrong password here")
	}

	if last.status != http.StatusTooManyRequests {
		t.Errorf("the 31st attempt from one address = %d, want 429", last.status)
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	id := h.session(t, "ana@example.com")

	if res := h.send(t, http.MethodPost, "/auth/logout", `{}`, id, nil); res.status != http.StatusOK {
		t.Fatalf("logout = %d %s", res.status, res.body)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/projects", "", id, nil); res.status != http.StatusUnauthorized {
		t.Errorf("after logout = %d, want 401", res.status)
	}
}

// A password change ends every session of the user (SDD-007 section 4.1 item 4).
func TestPasswordChangeRevokesSessions(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	id := h.session(t, "ana@example.com")
	other := h.session(t, "ana@example.com")

	if _, err := h.auth.ChangePassword(t.Context(), "ana@example.com", "a brand new password"); err != nil {
		t.Fatal(err)
	}

	for _, s := range []string{id, other} {
		if res := h.send(t, http.MethodGet, "/api/v1/projects", "", s, nil); res.status != http.StatusUnauthorized {
			t.Errorf("a session of the old password = %d, want 401", res.status)
		}
	}

	if res := h.login(t, "ana@example.com", goodPass); res.status != http.StatusUnauthorized {
		t.Errorf("the old password = %d, want 401", res.status)
	}

	if res := h.login(t, "ana@example.com", "a brand new password"); res.status != http.StatusOK {
		t.Errorf("the new password = %d, want 200", res.status)
	}
}

// admin has the five scopes, member the four that work on own projects, and a
// user sees only own projects (SDD-007 section 4.1 item 3).
func TestRolesAndOwnership(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "admin@example.com", service.RoleAdmin)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "ben@example.com", service.RoleMember)

	admin := h.session(t, "admin@example.com")
	ana := h.session(t, "ana@example.com")
	ben := h.session(t, "ben@example.com")

	if res := h.send(t, http.MethodGet, "/api/v1/audit", "", ana, nil); res.status != http.StatusForbidden {
		t.Errorf("a member reading the audit = %d, want 403", res.status)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/audit", "", admin, nil); res.status != http.StatusOK {
		t.Errorf("an admin reading the audit = %d %s, want 200", res.status, res.body)
	}

	created := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"mine"}`, ana, nil)
	if created.status != http.StatusCreated {
		t.Fatalf("create = %d %s", created.status, created.body)
	}

	var p struct {
		ProjectID string `json:"project_id"`
	}

	_ = json.Unmarshal(created.body, &p)

	if res := h.send(
		t,
		http.MethodGet,
		"/api/v1/projects/"+p.ProjectID,
		"",
		ben,
		nil,
	); res.status != http.StatusNotFound {
		t.Errorf("another user's project = %d, want 404", res.status)
	}

	if res := h.send(
		t,
		http.MethodGet,
		"/api/v1/projects/"+p.ProjectID,
		"",
		admin,
		nil,
	); res.status != http.StatusNotFound {
		t.Errorf("an admin gains no access to a member's project = %d, want 404", res.status)
	}

	if res := h.send(t, http.MethodGet, "/api/v1/projects/"+p.ProjectID, "", ana, nil); res.status != http.StatusOK {
		t.Errorf("the owner = %d, want 200", res.status)
	}
}

// A request that only the cookie authenticates must show where it comes from
// (SDD-007 section 3.6 step 6).
func TestSessionCookieNeedsOriginForStateChanges(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)

	id := h.session(t, "ana@example.com")

	res := h.send(t, http.MethodPost, "/api/v1/projects", `{"name":"x"}`, id, map[string]string{"Origin": ""})
	if res.status != http.StatusForbidden {
		t.Errorf("a state change with no Origin = %d, want 403", res.status)
	}
}

func TestLocalModeHasNoLoginRoutes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, p := range []string{"/auth/login", "/auth/logout"} {
		if res := h.post(p, `{"email":"a@example.com","password":"`+goodPass+`"}`); res.status != http.StatusNotFound {
			t.Errorf("%s in local mode = %d, want 404", p, res.status)
		}
	}
}
