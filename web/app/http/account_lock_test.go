package httpapi_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/agenthost"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
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

	r := httptest.NewRequest(http.MethodPost, "/auth/account/create", strings.NewReader(createBody))
	r.Header.Set("Authorization", "Bearer "+canary)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(auth.ClientHeader, "test")

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

// A recovery sets a new password: a session the old password opened does not survive it.
func TestSecurity_T116_ARecoveryEndsTheOtherSessions(t *testing.T) {
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
func TestSecurity_T116_TheLockEndsAnOpenStream(t *testing.T) {
	t.Parallel()

	a := newAccountHarness(t)
	a.create()

	cookie, _ := a.signIn(accPassword)

	stream, err := http.NewRequest(http.MethodGet, a.srv.URL+"/api/v1/publications/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	stream.Header.Set("Cookie", cookie)
	stream.Header.Set("Accept", "text/event-stream")
	stream.Header.Set("Sec-Fetch-Site", "same-origin")

	res, err := http.DefaultClient.Do(stream)
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
