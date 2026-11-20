package auth_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/auth/googletest"
	"github.com/AlexGalhardo/stuarty/web/app/store"

	"github.com/AlexGalhardo/vukem/auth/hash"
	"github.com/AlexGalhardo/vukem/database"
)

const googleSecret = `{"client_id":"` + testClientID + `","client_secret":"the-client-secret-of-the-owner"}`

// googleHarness is the Google sign-in over a real workspace database, a fake Google and a connection of
// the owner's own project whose secret is in a vault of its own.
type googleHarness struct {
	g     *auth.Google
	users *auth.Users
	fake  *googletest.Server
	conns *connectionstest.Harness
	db    *database.DB
}

func newGoogleHarness(t *testing.T) *googleHarness {
	t.Helper()

	return newGoogleHarnessOf(t, "youtube", googleSecret)
}

// newGoogleHarnessOf is the harness with a connection of the given kind and secret.
func newGoogleHarnessOf(t *testing.T, kind, secret string) *googleHarness {
	t.Helper()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	users := auth.NewUsers(st.DB())
	users.Hasher = hash.Fake()

	fake := googletest.New(t)
	conns := connectionstest.New(t, kind, fake.URL, secret)

	g, err := auth.NewGoogle(auth.GoogleOptions{
		DB: st.DB(), Users: users, Connections: conns.Manager, AppURL: "https://video.example.com",
		Endpoints: auth.TestEndpoints(fake.URL),
	})
	if err != nil {
		t.Fatal(err)
	}

	return &googleHarness{g: g, users: users, fake: fake, conns: conns, db: st.DB()}
}

func (h *googleHarness) rawExec(t *testing.T, q string, args ...any) (sql.Result, error) {
	t.Helper()

	return h.db.ExecContext(t.Context(), q, args...)
}

func countRows(t *testing.T, h *googleHarness, q string) int {
	t.Helper()

	var n int
	if err := h.db.QueryRowContext(t.Context(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}

	return n
}

// dumpTable is every value of a table as text, for a test that looks for what must not be there.
func dumpTable(t *testing.T, h *googleHarness, table string) string {
	t.Helper()

	rows, err := h.db.QueryContext(t.Context(), "SELECT * FROM "+table)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = rows.Close() }()

	cols, _ := rows.Columns()

	var out strings.Builder

	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))

		for i := range vals {
			ptrs[i] = &vals[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}

		fmt.Fprintln(&out, vals...)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return out.String()
}

func (h *googleHarness) enable(t *testing.T) {
	t.Helper()

	h.enableFor(t, nil)
}

// enableFor turns the sign-in on, restricted to the addresses (none is no restriction).
func (h *googleHarness) enableFor(t *testing.T, allowed []string) {
	t.Helper()

	_, err := h.g.SetSettings(t.Context(), auth.GoogleSettings{
		Enabled: true, ConnectionID: connectionstest.ID, AllowedEmails: allowed,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (h *googleHarness) user(t *testing.T, email string) auth.User {
	t.Helper()

	return h.userAs(t, email, service.RoleMember)
}

func (h *googleHarness) userAs(t *testing.T, email, role string) auth.User {
	t.Helper()

	u, err := h.users.Create(t.Context(), email, "correct horse battery", role)
	if err != nil {
		t.Fatal(err)
	}

	return u
}

// flow is a begun flow: the state the browser carries and the nonce Google must put in the token.
type flow struct{ state, nonce string }

func flowOf(t *testing.T, authorizeURL string) flow {
	t.Helper()

	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	if q.Get("state") == "" || q.Get("nonce") == "" {
		t.Fatalf("authorize address %q has no state or nonce", authorizeURL)
	}

	return flow{state: q.Get("state"), nonce: q.Get("nonce")}
}

// answer is Google's redirect for the flow, for a person whose Google account is sub and email.
func (h *googleHarness) answer(f flow, sub, email string) auth.CompleteRequest {
	code := h.fake.Grant(googletest.Grant{Subject: sub, Email: email, Nonce: f.nonce})

	return auth.CompleteRequest{State: f.state, Code: code}
}

// signIn runs a whole sign-in of a browser with no session, for a person whose Google account is sub and
// email, and returns what Complete answered.
func (h *googleHarness) signIn(t *testing.T, sub, email string) (auth.Completed, error) {
	t.Helper()

	binding, err := auth.NewBinding()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := h.g.BeginSignIn(t.Context(), binding)
	if err != nil {
		t.Fatal(err)
	}

	req := h.answer(flowOf(t, raw), sub, email)
	req.Binding = binding

	return h.g.Complete(t.Context(), req)
}

// link runs a whole linking for user and returns the error of Complete.
func (h *googleHarness) link(t *testing.T, user auth.User, sub, email string) error {
	t.Helper()

	raw, err := h.g.BeginLink(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}

	req := h.answer(flowOf(t, raw), sub, email)
	req.SessionUserID = user.ID

	_, err = h.g.Complete(t.Context(), req)

	return err
}

func TestLinkIdentityStoresTheSubjectAndTheEmailOnly(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	if err := h.link(t, ana, "sub-ana", "Ana.Google@Example.test"); err != nil {
		t.Fatal(err)
	}

	li, ok, err := h.g.Identity(t.Context(), ana.ID)
	if err != nil || !ok || li.Email != "ana.google@example.test" || li.LinkedMS == 0 {
		t.Fatalf("Identity = %+v, %v, %v", li, ok, err)
	}

	forms := h.fake.Forms()
	if len(forms) != 1 || forms[0].Get("client_secret") == "" || forms[0].Get("code_verifier") == "" {
		t.Fatalf("token requests = %v, want one with the client secret and the PKCE verifier", forms)
	}

	// The tokens are held for the length of the call: none is in a table the sign-in writes.
	for _, table := range []string{"user_identities", "auth_settings"} {
		if dump := dumpTable(
			t,
			h,
			table,
		); strings.Contains(dump, "fake-access-token") ||
			strings.Contains(dump, "eyJ") ||
			strings.Contains(dump, "client-secret") {
			t.Errorf("%s holds a token or a secret: %s", table, dump)
		}
	}
}

func TestLinkIdentityRefusesASecondIdentityAndAnIdentityOfAnotherUser(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")
	bob := h.user(t, "bob@example.test")

	if err := h.link(t, ana, "sub-ana", "ana@example.test"); err != nil {
		t.Fatal(err)
	}

	if err := h.link(t, ana, "sub-other", "ana2@example.test"); !errors.Is(err, auth.ErrAlreadyLinked) {
		t.Errorf("a second identity: err = %v, want ErrAlreadyLinked", err)
	}

	if err := h.link(t, bob, "sub-ana", "ana@example.test"); !errors.Is(err, auth.ErrIdentityTaken) {
		t.Errorf("an identity of another user: err = %v, want ErrIdentityTaken", err)
	}

	if _, ok, _ := h.g.Identity(t.Context(), bob.ID); ok {
		t.Error("bob holds an identity he was refused")
	}
}

func TestLinkIdentityIsNotByEmail(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")
	bob := h.user(t, "bob@example.test")

	// Bob links a Google account whose e-mail is Ana's: it becomes Bob's, by its subject, and Ana has none.
	if err := h.link(t, bob, "sub-bob", "ana@example.test"); err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := h.g.Identity(t.Context(), ana.ID); ok {
		t.Error("an identity with Ana's e-mail was attached to Ana")
	}

	li, ok, _ := h.g.Identity(t.Context(), bob.ID)
	if !ok || li.Email != "ana@example.test" {
		t.Errorf("bob identity = %+v, %v", li, ok)
	}
}

func TestLinkIdentityIsBoundToTheSession(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")
	bob := h.user(t, "bob@example.test")

	raw, err := h.g.BeginLink(t.Context(), ana.ID)
	if err != nil {
		t.Fatal(err)
	}

	f := flowOf(t, raw)

	// Bob (or a page that makes Bob complete it) finishes Ana's flow: refused, and the state is spent.
	req := h.answer(f, "sub-bob", "bob@example.test")
	req.SessionUserID = bob.ID

	if _, err := h.g.Complete(t.Context(), req); !errors.Is(err, auth.ErrFlow) {
		t.Fatalf("another session: err = %v, want ErrFlow", err)
	}

	req = h.answer(f, "sub-ana", "ana@example.test")
	req.SessionUserID = ana.ID

	if _, err := h.g.Complete(t.Context(), req); !errors.Is(err, auth.ErrFlow) {
		t.Fatalf("the state after a refusal: err = %v, want ErrFlow (spent)", err)
	}

	for _, u := range []auth.User{ana, bob} {
		if _, ok, _ := h.g.Identity(t.Context(), u.ID); ok {
			t.Errorf("%s was linked by a flow that was refused", u.Email)
		}
	}

	// No session at all.
	raw, _ = h.g.BeginLink(t.Context(), ana.ID)

	req = h.answer(flowOf(t, raw), "sub-ana", "ana@example.test")
	if _, err := h.g.Complete(t.Context(), req); !errors.Is(err, auth.ErrFlow) {
		t.Fatalf("no session: err = %v, want ErrFlow", err)
	}
}

func TestLinkIdentityStateIsSingleUseAndOfTheRightPurpose(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	raw, _ := h.g.BeginLink(t.Context(), ana.ID)
	f := flowOf(t, raw)

	// A link state presented as a sign-in state is another purpose.
	swapped := h.answer(
		flow{state: "l." + strings.TrimPrefix(f.state, "k."), nonce: f.nonce},
		"sub-ana",
		"ana@example.test",
	)
	swapped.Binding = "any-cookie"

	if _, err := h.g.Complete(t.Context(), swapped); !errors.Is(err, auth.ErrFlow) {
		t.Fatalf("a link state as a sign-in state: err = %v, want ErrFlow", err)
	}

	// The swap spent the state: shown with its own prefix by its own user, it is refused now.
	again := h.answer(f, "sub-ana", "ana@example.test")
	again.SessionUserID = ana.ID

	if _, err := h.g.Complete(t.Context(), again); !errors.Is(err, auth.ErrFlow) {
		t.Fatalf("the link state after a swap: err = %v, want ErrFlow", err)
	}

	for _, bad := range []string{"", "k.", "l.", "x.abc", "abc"} {
		if _, err := h.g.Complete(
			t.Context(),
			auth.CompleteRequest{State: bad, Code: "c"},
		); !errors.Is(
			err,
			auth.ErrFlow,
		) {
			t.Errorf("state %q: err = %v, want ErrFlow", bad, err)
		}
	}
}

func TestLinkIdentityUnlinkLeavesTheUserFreeToLinkAgain(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	if err := h.g.Unlink(t.Context(), ana.ID); !errors.Is(err, auth.ErrNoIdentity) {
		t.Fatalf("unlink with none: err = %v, want ErrNoIdentity", err)
	}

	if err := h.link(t, ana, "sub-ana", "ana@example.test"); err != nil {
		t.Fatal(err)
	}

	if err := h.g.Unlink(t.Context(), ana.ID); err != nil {
		t.Fatal(err)
	}

	if _, ok, _ := h.g.Identity(t.Context(), ana.ID); ok {
		t.Fatal("the identity is still there after unlink")
	}

	if err := h.link(t, ana, "sub-ana2", "ana@example.test"); err != nil {
		t.Fatalf("link again after unlink: %v", err)
	}
}

func TestLinkIdentityNeedsGoogleSignInOn(t *testing.T) {
	h := newGoogleHarness(t)
	ana := h.user(t, "ana@example.test")

	if _, err := h.g.BeginLink(t.Context(), ana.ID); !errors.Is(err, auth.ErrGoogleOff) {
		t.Fatalf("off by default: err = %v, want ErrGoogleOff", err)
	}

	if h.fake.TotalHits() != 0 {
		t.Errorf("a switched-off sign-in reached Google %d times", h.fake.TotalHits())
	}
}

func TestLinkIdentityDeletesWithTheUser(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	if err := h.link(t, ana, "sub-ana", "ana@example.test"); err != nil {
		t.Fatal(err)
	}

	if n := countRows(t, h, "SELECT COUNT(*) FROM user_identities"); n != 1 {
		t.Fatalf("identities = %d, want 1", n)
	}

	if _, err := h.rawExec(t, `DELETE FROM users WHERE id = ?`, ana.ID); err != nil {
		t.Fatal(err)
	}

	if n := countRows(t, h, "SELECT COUNT(*) FROM user_identities"); n != 0 {
		t.Errorf("identities after the user was deleted = %d, want 0", n)
	}
}

// Anyone may begin a sign-in without a credential, and the flows that wait are capped; a flood of them must
// not push out the linking a signed-in user began with their password.
func TestLinkIdentitySurvivesAFloodOfSignIns(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	raw, err := h.g.BeginLink(t.Context(), ana.ID)
	if err != nil {
		t.Fatal(err)
	}

	for range oauth.MaxPending + 5 {
		binding, berr := auth.NewBinding()
		if berr != nil {
			t.Fatal(berr)
		}

		if _, berr := h.g.BeginSignIn(t.Context(), binding); berr != nil {
			t.Fatal(berr)
		}
	}

	req := h.answer(flowOf(t, raw), "sub-ana", "ana@example.test")
	req.SessionUserID = ana.ID

	if _, err := h.g.Complete(t.Context(), req); err != nil {
		t.Fatalf("linking after a flood of sign-ins: %v", err)
	}
}
