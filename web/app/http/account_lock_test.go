package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// deadlineWriter records the deadlines a handler sets on the connection.
type deadlineWriter struct {
	http.ResponseWriter
	write []time.Time
}

func (w *deadlineWriter) SetWriteDeadline(d time.Time) error {
	w.write = append(w.write, d)

	return nil
}

func (w *deadlineWriter) SetReadDeadline(time.Time) error { return nil }

// The migration of a large workspace can last longer than the write timeout of the server. The answer holds the
// recovery key, which exists nowhere else, so the deadline is lifted before the work starts.
func TestAccountCreateLiftsTheWriteDeadline(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)

	r := httptest.NewRequest(http.MethodPost, a.srv.URL+"/auth/account/create", strings.NewReader(createBody))
	r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, a.srv.Listener.Addr()))
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set("Cookie", a.sessionWith(t, ""))
	r.Header.Set("Origin", a.srv.URL)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	w := &deadlineWriter{ResponseWriter: rec}

	a.app.Handler().ServeHTTP(w, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}

	if len(w.write) == 0 || !w.write[len(w.write)-1].IsZero() {
		t.Fatalf("the write deadline was not lifted: %v", w.write)
	}
}

// The desktop shell asks for a launch code when it starts, and it starts with the studio locked: refused, the
// window would never open. The code leads to the sign-in page and reads nothing.
func TestAccountLaunchOpensTheSignInWhileLocked(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	a.create()

	launched := a.sessionWith(t, launcherSecret)

	page := a.do(req{path: "/", noAuth: true, headers: map[string]string{
		"Accept": "text/html", "Cookie": launched, "Sec-Fetch-Site": "none",
	}})
	if page.status != http.StatusSeeOther || page.header.Get("Location") != "/account/signin" {
		t.Fatalf("the window of the shell = %d %s", page.status, page.header.Get("Location"))
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", launched); res.status != http.StatusLocked {
		t.Fatalf("a launch session while locked = %d %s", res.status, res.body)
	}

	if res := a.get("/api/v1/projects"); res.status != http.StatusLocked {
		t.Fatalf("the token while locked = %d", res.status)
	}
}

type agentTokens map[string]agenthost.Grant

func (t agentTokens) Lookup(secret string) (agenthost.Grant, bool) {
	g, ok := t[secret]

	return g, ok
}

// A hosted agent has no admin scope. If it could create the account it would hold the password, and a sign-in with
// it is a person: the agent would approve its own publications.
func TestSecurity_T117_AHostedAgentCannotCreateTheAccount(t *testing.T) {
	t.Parallel()

	const agent = "agent-session-token"

	a := newAccountHarnessWith(t, agentTokens{agent: {
		SessionID: "cht_1", ProjectID: "prj_1", Scopes: agenthost.GrantScopes(),
	}})

	res := a.do(req{
		method: http.MethodPost, path: "/auth/account/create", body: createBody, noAuth: true,
		headers: map[string]string{"Authorization": "Bearer " + agent},
	})
	if res.status != http.StatusForbidden {
		t.Fatalf("create by a hosted agent = %d %s", res.status, res.body)
	}

	if a.acc.State() != account.StateNone {
		t.Fatalf("the agent made the account: state %s", a.acc.State())
	}
}

// VV-378: whoever holds the local token could make the first account before the person and sign in as one. Only
// the browser that `stuarty open` or the desktop shell opened may create it.
func TestSecurity_T117_TheLocalTokenAloneCannotCreateTheAccount(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)

	res := a.post("/auth/account/create", createBody)
	if res.status != http.StatusForbidden || res.json(t)["code"] != "E_FORBIDDEN" {
		t.Fatalf("create with the token alone = %d %s", res.status, res.body)
	}

	if a.acc.State() != account.StateNone {
		t.Fatalf("the token made the account: state %s", a.acc.State())
	}

	// The person's own browser still can.
	if res := a.createFrom(a.sessionWith(t, ""), createBody); res.status != http.StatusOK {
		t.Fatalf("create from the opened browser = %d %s", res.status, res.body)
	}
}

// A recovery sets a new password: a session the old password opened does not survive it.
func TestSecurity_T117_ARecoveryEndsTheOtherSessions(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	recovery := a.create()

	stolen, _ := a.signIn(accPassword)

	body, err := json.Marshal(map[string]string{
		"recovery_key": recovery, "password": accOther, "confirmation": accOther,
	})
	if err != nil {
		t.Fatal(err)
	}

	if res := a.browser(http.MethodPost, "/auth/account/recover", string(body), ""); res.status != http.StatusOK {
		t.Fatalf("recover = %d %s", res.status, res.body)
	}

	if res := a.browser(http.MethodGet, "/api/v1/projects", "", stolen); res.status != http.StatusUnauthorized {
		t.Fatalf("a session of the old password after the recovery = %d %s", res.status, res.body)
	}
}

// An open event stream was authorized before the lock, and is asked again only once a minute: it must end with the
// lock, or a browser that logged out would go on receiving.
func TestSecurity_T107_TheLockEndsAnOpenStream(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	a.create()

	cookie, _ := a.signIn(accPassword)

	stream, err := http.NewRequest(http.MethodGet, a.srv.URL+"/api/v1/publications/events", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	stream.Header.Set("Cookie", cookie)
	stream.Header.Set("Accept", "text/event-stream")
	stream.Header.Set("Sec-Fetch-Site", "same-origin")

	res, err := a.srv.Client().Do(stream)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("the stream = %d", res.StatusCode)
	}

	ended := make(chan struct{})

	go func() {
		defer close(ended)

		for sc := bufio.NewScanner(res.Body); sc.Scan(); {
			_ = sc.Text()
		}
	}()

	if out := a.browser(http.MethodPost, "/auth/account/logout", "{}", cookie); out.status != http.StatusOK {
		t.Fatalf("logout = %d %s", out.status, out.body)
	}

	select {
	case <-ended:
	case <-time.After(20 * time.Second):
		t.Fatal("the stream is still open after the lock")
	}
}

// VV-379: a workspace adopted by copy leaves the former folder in clear text outside the workspace. The answer of
// the creation names it, so the page can tell the person; with no such folder it says nothing.
func TestAccountCreateNamesTheFolderAnAdoptionLeftBehind(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	former := t.TempDir()

	record := `{"from":` + strconv.Quote(
		former,
	) + `,"to":"x","method":"copy","backup":"","adopted_at":"2026-11-18T12:30:00Z"}`
	if err := os.WriteFile(a.dir.Path(workspace.AdoptionRecord), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}

	res := a.createFrom(a.sessionWith(t, ""), createBody)
	if res.status != http.StatusOK {
		t.Fatalf("create = %d %s", res.status, res.body)
	}

	if got, _ := res.json(t)["former_folder"].(string); got != former {
		t.Errorf("former_folder = %q, want %q", got, former)
	}

	plain := newAccountHarness(t)
	if res := plain.createFrom(
		plain.sessionWith(t, ""),
		createBody,
	); strings.Contains(
		string(res.body),
		"former_folder",
	) {
		t.Errorf("a workspace that was not adopted by copy names a folder: %s", res.body)
	}
}
