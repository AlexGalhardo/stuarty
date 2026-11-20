package openai

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func token(t *testing.T, claims map[string]any) string {
	t.Helper()

	enc := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}

		return base64.RawURLEncoding.EncodeToString(raw)
	}

	return enc(map[string]string{"alg": "RS256"}) + "." + enc(claims) + ".signature"
}

func TestCheckIDToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	good := func() map[string]any {
		return map[string]any{
			"iss": "https://auth.openai.com", "aud": "oaiapp_x", "exp": now.Add(time.Hour).Unix(), "nonce": "n1",
			"email": "person@example.test",
		}
	}

	cases := map[string]struct {
		edit func(map[string]any)
		ok   bool
	}{
		"as issued":            {func(map[string]any) {}, true},
		"audience is a list":   {func(c map[string]any) { c["aud"] = []string{"other", "oaiapp_x"} }, true},
		"another client":       {func(c map[string]any) { c["aud"] = "oaiapp_other" }, false},
		"a list without it":    {func(c map[string]any) { c["aud"] = []string{"other"} }, false},
		"another nonce":        {func(c map[string]any) { c["nonce"] = "n2" }, false},
		"no nonce":             {func(c map[string]any) { delete(c, "nonce") }, false},
		"expired":              {func(c map[string]any) { c["exp"] = now.Add(-time.Second).Unix() }, false},
		"no expiry":            {func(c map[string]any) { delete(c, "exp") }, false},
		"another issuer":       {func(c map[string]any) { c["iss"] = "https://evil.example" }, false},
		"no issuer":            {func(c map[string]any) { delete(c, "iss") }, false},
		"audience of a number": {func(c map[string]any) { c["aud"] = 7 }, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := good()
			tc.edit(c)

			claims, ok := checkIDToken(token(t, c), "oaiapp_x", "n1", "auth.openai.com", now)
			if ok != tc.ok {
				t.Fatalf("checkIDToken = %v, want %v", ok, tc.ok)
			}

			if ok && claims.Email != "person@example.test" {
				t.Errorf("email %q", claims.Email)
			}
		})
	}

	for _, raw := range []string{"", "a.b", "a.!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".c"} {
		if _, ok := checkIDToken(raw, "oaiapp_x", "n1", "auth.openai.com", now); ok {
			t.Errorf("%q was taken for an identity token", raw)
		}
	}
}
