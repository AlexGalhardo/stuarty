package auth_test

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections/connectionstest"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
)

func TestGoogleSignInPolicyIsOffByDefault(t *testing.T) {
	h := newGoogleHarness(t)

	s, err := h.g.Settings(t.Context())
	if err != nil || s.Enabled || s.ConnectionID != "" || len(s.AllowedEmails) != 0 {
		t.Fatalf("Settings = %+v, %v, want everything off and empty", s, err)
	}

	if on, err := h.g.Enabled(t.Context()); err != nil || on {
		t.Errorf("Enabled = %v, %v", on, err)
	}

	if _, err := h.g.BeginSignIn(t.Context(), "cookie"); !errors.Is(err, auth.ErrGoogleOff) {
		t.Errorf("BeginSignIn while off: err = %v, want ErrGoogleOff", err)
	}
}

func TestGoogleSignInPolicyValidatesWhatTheOwnerSets(t *testing.T) {
	h := newGoogleHarness(t)

	tooMany := make([]string, auth.MaxAllowedEmails+1)
	for i := range tooMany {
		tooMany[i] = "user" + strconv.Itoa(i) + "@example.test"
	}

	cases := map[string]struct {
		in   auth.GoogleSettings
		path string
	}{
		"on with no connection": {auth.GoogleSettings{Enabled: true}, "connection_id"},
		"on with an unknown id": {
			auth.GoogleSettings{Enabled: true, ConnectionID: "con_nothingnothing"},
			"connection_id",
		},
		"an entry that is not a mail": {
			auth.GoogleSettings{ConnectionID: connectionstest.ID, AllowedEmails: []string{"not an address"}},
			"allowed_emails",
		},
		"a name with the address": {
			auth.GoogleSettings{ConnectionID: connectionstest.ID, AllowedEmails: []string{"Ana <a@example.test>"}},
			"allowed_emails",
		},
		"too many addresses": {
			auth.GoogleSettings{ConnectionID: connectionstest.ID, AllowedEmails: tooMany},
			"allowed_emails",
		},
	}

	for name, c := range cases {
		_, err := h.g.SetSettings(t.Context(), c.in)

		se, ok := errors.AsType[*auth.SettingsError](err)
		if !ok || se.Path != c.path {
			t.Errorf("%s: err = %v, want a SettingsError at %s", name, err, c.path)
		}
	}

	// Nothing the refusals tried was kept.
	if s, _ := h.g.Settings(t.Context()); s.Enabled || s.ConnectionID != "" {
		t.Errorf("a refused change was stored: %+v", s)
	}
}

func TestGoogleSignInPolicyNeedsTheConnectionOfAGoogleProject(t *testing.T) {
	// A connection of another kind, and one whose secret is no client, are both refused.
	for name, c := range map[string]struct{ kind, secret string }{
		"another kind":                  {"test", googleSecret},
		"a secret that is not a client": {"youtube", `{"refresh_token":"only-a-token"}`},
		"a secret that is not JSON":     {"youtube", `plain-token`},
	} {
		h := newGoogleHarnessOf(t, c.kind, c.secret)

		_, err := h.g.SetSettings(t.Context(), auth.GoogleSettings{Enabled: true, ConnectionID: connectionstest.ID})
		if se, ok := errors.AsType[*auth.SettingsError](err); !ok || se.Path != "connection_id" {
			t.Errorf("%s: err = %v, want a SettingsError at connection_id", name, err)
		}
	}
}

func TestGoogleSignInPolicyKeepsTheListNormalised(t *testing.T) {
	h := newGoogleHarness(t)

	got, err := h.g.SetSettings(t.Context(), auth.GoogleSettings{
		Enabled: true, ConnectionID: connectionstest.ID,
		AllowedEmails: []string{" Ana@Gmail.test ", "ana@gmail.test", "bob@gmail.test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"ana@gmail.test", "bob@gmail.test"}
	if !slices.Equal(got.AllowedEmails, want) {
		t.Errorf("AllowedEmails = %v, want %v", got.AllowedEmails, want)
	}

	if read, _ := h.g.Settings(t.Context()); !read.Enabled || !slices.Equal(read.AllowedEmails, want) ||
		read.ConnectionID != connectionstest.ID {
		t.Errorf("Settings after the change = %+v", read)
	}

	// Clearing the list lifts the restriction.
	if _, err := h.g.SetSettings(
		t.Context(),
		auth.GoogleSettings{Enabled: true, ConnectionID: connectionstest.ID},
	); err != nil {
		t.Fatal(err)
	}

	if read, _ := h.g.Settings(t.Context()); len(read.AllowedEmails) != 0 {
		t.Errorf("the list after clearing = %v", read.AllowedEmails)
	}
}

func TestGoogleSignInPolicyRestrictsSignInAndLinkingToTheList(t *testing.T) {
	h := newGoogleHarness(t)
	h.enableFor(t, []string{"ana@gmail.test"})
	ana := h.user(t, "ana@example.test")
	bob := h.user(t, "bob@example.test")

	if err := h.link(t, bob, "sub-bob", "bob@gmail.test"); !errors.Is(err, auth.ErrNotAllowed) {
		t.Fatalf("linking an address that is not on the list: err = %v, want ErrNotAllowed", err)
	}

	if _, ok, _ := h.g.Identity(t.Context(), bob.ID); ok {
		t.Fatal("an address that is not on the list was linked")
	}

	if err := h.link(t, ana, "sub-ana", "Ana@Gmail.test"); err != nil {
		t.Fatalf("an address on the list: %v", err)
	}

	if _, err := h.signIn(t, "sub-ana", "ana@gmail.test"); err != nil {
		t.Fatalf("sign-in of an address on the list: %v", err)
	}

	// The owner narrows the list afterwards: Ana, already linked, is out.
	h.enableFor(t, []string{"someone.else@gmail.test"})

	if _, err := h.signIn(t, "sub-ana", "ana@gmail.test"); !errors.Is(err, auth.ErrNotAllowed) {
		t.Fatalf("sign-in after the list changed: err = %v, want ErrNotAllowed", err)
	}
}

func TestGoogleSignInPolicyTurnedOffStopsFlowsAlreadyBegun(t *testing.T) {
	h := newGoogleHarness(t)
	h.enable(t)
	ana := h.user(t, "ana@example.test")

	raw, err := h.g.BeginLink(t.Context(), ana.ID)
	if err != nil {
		t.Fatal(err)
	}

	req := h.answer(flowOf(t, raw), "sub-ana", "ana@example.test")
	req.SessionUserID = ana.ID

	// The owner turns it off while the person is at the consent screen.
	if _, err := h.g.SetSettings(t.Context(), auth.GoogleSettings{ConnectionID: connectionstest.ID}); err != nil {
		t.Fatal(err)
	}

	before := h.fake.TotalHits()

	if _, err := h.g.Complete(t.Context(), req); !errors.Is(err, auth.ErrGoogleOff) {
		t.Fatalf("Complete after the owner turned it off: err = %v, want ErrGoogleOff", err)
	}

	if h.fake.TotalHits() != before {
		t.Error("a sign-in that was turned off still reached Google")
	}

	if _, ok, _ := h.g.Identity(t.Context(), ana.ID); ok {
		t.Error("an identity was linked after the owner turned the feature off")
	}
}
