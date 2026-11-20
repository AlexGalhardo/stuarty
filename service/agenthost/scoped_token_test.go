package agenthost

import (
	"testing"
	"time"
)

func TestScopedTokenIsIssuedBoundAndNeverAdmin(t *testing.T) {
	t.Parallel()

	tokens := NewTokens(nil)

	for name, g := range map[string]Grant{
		"no project": {SessionID: "cht_0000000001", Scopes: []string{"read"}},
		"no session": {ProjectID: "prj_0000000001", Scopes: []string{"read"}},
		"admin":      {SessionID: "cht_0000000001", ProjectID: "prj_0000000001", Scopes: []string{"read", "admin"}},
	} {
		if _, _, err := tokens.Issue(g, time.Time{}); err == nil {
			t.Errorf("%s: a token was issued", name)
		}
	}

	if tokens.Count() != 0 {
		t.Errorf("%d tokens are live", tokens.Count())
	}
}

func TestScopedTokenSecretIsNotStoredAndHasTheShapeOfAToken(t *testing.T) {
	t.Parallel()

	tokens := NewTokens(nil)
	g := Grant{SessionID: "cht_0000000001", ProjectID: "prj_0000000001", Scopes: GrantScopes()}

	a, _, err := tokens.Issue(g, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	b, _, err := tokens.Issue(g, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if a == b || len(a) != len(TokenPrefix)+43 || a[:4] != TokenPrefix {
		t.Errorf("tokens %q and %q", a, b)
	}

	// The last character of the near miss is always changed: a fixed replacement would give back b itself,
	// a valid token, whenever b already ends with it.
	swapped := byte('A')
	if b[len(b)-1] == swapped {
		swapped = 'B'
	}

	// Near misses, other shapes and the empty string are not found.
	for _, bad := range []string{"", a[:len(a)-1], a + "x", "vvk_" + a[4:], "Bearer " + a, a[:4] + b[4:len(b)-1] + string(swapped)} {
		if _, ok := tokens.Lookup(bad); ok {
			t.Errorf("%q was accepted", bad)
		}
	}

	// A copy of the grant cannot widen the scopes held by the registry.
	got, _ := tokens.Lookup(a)
	got.Scopes[0] = "admin"

	again, _ := tokens.Lookup(a)
	if again.Scopes[0] != "read" {
		t.Error("a caller changed the scopes in the registry")
	}
}

func TestScopedTokenDiesWithItsSessionAndWhenItExpires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	tokens := NewTokens(func() time.Time { return now })
	g := Grant{SessionID: "cht_0000000001", ProjectID: "prj_0000000001", Scopes: GrantScopes()}

	secret, revoke, err := tokens.Issue(g, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := tokens.Lookup(secret); !ok {
		t.Fatal("a live token is not found")
	}

	revoke()

	if _, ok := tokens.Lookup(secret); ok {
		t.Error("a revoked token is accepted")
	}

	// It expires on its own.
	secret, _, err = tokens.Issue(g, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(time.Hour)

	if _, ok := tokens.Lookup(secret); ok {
		t.Error("an expired token is accepted")
	}

	if tokens.Count() != 0 {
		t.Errorf("%d tokens are still held", tokens.Count())
	}
}
