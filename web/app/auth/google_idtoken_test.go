package auth_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/auth/googletest"

	"github.com/AlexGalhardo/vukem/auth/jwt"
)

const (
	testClientID = "1234-client.apps.example.test"
	testNonce    = "nonce-of-this-flow"
)

func idTokenConfig(g *googletest.Server) auth.IDTokenConfig {
	return auth.IDTokenConfig{
		Issuer: g.URL, KeysURL: g.URL + "/certs", ClientID: testClientID, Nonce: testNonce, Client: loopbackClient(),
	}
}

// loopbackClient reaches the fake Google on loopback; the product itself uses the gated client.
func loopbackClient() auth.Doer {
	return &http.Client{Timeout: 5 * time.Second} //nolint:forbidigo // loopback test server
}

func goodGrant() googletest.Grant {
	return googletest.Grant{Subject: "1098765", Email: "Ana@Example.test", Nonce: testNonce}
}

func TestIDTokenAcceptsAGoodToken(t *testing.T) {
	g := googletest.New(t)

	id, err := auth.VerifyIDToken(t.Context(), idTokenConfig(g), g.IDToken(goodGrant(), testClientID))
	if err != nil {
		t.Fatal(err)
	}

	if id.Subject != "1098765" || id.Email != "ana@example.test" {
		t.Errorf("identity = %+v, want the subject and the lower-cased e-mail", id)
	}
}

func TestIDTokenRefusals(t *testing.T) {
	g := googletest.New(t)

	cases := map[string]googletest.Grant{
		"wrong audience":    {Subject: "1", Email: "a@example.test", Nonce: testNonce, Audience: "another-app"},
		"expired":           {Subject: "1", Email: "a@example.test", Nonce: testNonce, Expired: true},
		"bad signature":     {Subject: "1", Email: "a@example.test", Nonce: testNonce, BadSignature: true},
		"wrong nonce":       {Subject: "1", Email: "a@example.test", Nonce: "the-nonce-of-another-flow"},
		"empty nonce":       {Subject: "1", Email: "a@example.test"},
		"wrong issuer":      {Subject: "1", Email: "a@example.test", Nonce: testNonce, Issuer: "https://evil.example"},
		"unverified e-mail": {Subject: "1", Email: "a@example.test", Nonce: testNonce, Unverified: true},
		"no subject":        {Email: "a@example.test", Nonce: testNonce},
		"not a mailbox":     {Subject: "1", Email: "Ana <a@example.test>", Nonce: testNonce},
		"no e-mail":         {Subject: "1", Nonce: testNonce},
	}

	for name, grant := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := auth.VerifyIDToken(t.Context(), idTokenConfig(g), g.IDToken(grant, testClientID))
			if !errors.Is(err, auth.ErrIDToken) {
				t.Fatalf("err = %v, want ErrIDToken", err)
			}
		})
	}
}

func TestIDTokenRefusesOtherAlgorithms(t *testing.T) {
	g := googletest.New(t)

	// HS256 and "none": the two classic downgrades.
	hs, err := jwt.Sign(jwt.HS256(googletest.KeyID, []byte(strings.Repeat("s", 40))), jwt.Claims{
		Issuer: g.URL, Subject: "1", Audience: []string{testClientID},
		Extra: map[string]any{"nonce": testNonce, "email": "a@example.test", "email_verified": true},
	})
	if err != nil {
		t.Fatal(err)
	}

	enc := base64.RawURLEncoding.EncodeToString
	none := enc([]byte(`{"alg":"none","typ":"JWT","kid":"`+googletest.KeyID+`"}`)) + "." +
		enc([]byte(`{"iss":"`+g.URL+`","sub":"1","aud":"`+testClientID+`","exp":4102444800,"nonce":"`+testNonce+
			`","email":"a@example.test","email_verified":true}`)) + "."

	for name, token := range map[string]string{"HS256": hs, "none": none} {
		if _, err := auth.VerifyIDToken(t.Context(), idTokenConfig(g), token); !errors.Is(err, auth.ErrIDToken) {
			t.Errorf("%s: err = %v, want ErrIDToken", name, err)
		}
	}
}

func TestIDTokenRefusesGarbage(t *testing.T) {
	g := googletest.New(t)

	for _, token := range []string{"", "a.b", "a.b.c", strings.Repeat("a", 100000)} {
		if _, err := auth.VerifyIDToken(t.Context(), idTokenConfig(g), token); err == nil {
			t.Errorf("%q: accepted", token[:min(len(token), 10)])
		}
	}
}

func TestIDTokenKeysUnavailableIsNotAJudgement(t *testing.T) {
	g := googletest.New(t)
	cfg := idTokenConfig(g)
	cfg.KeysURL = g.URL + "/missing"

	_, err := auth.VerifyIDToken(t.Context(), cfg, g.IDToken(goodGrant(), testClientID))
	if !errors.Is(err, auth.ErrIDTokenKeys) {
		t.Fatalf("err = %v, want ErrIDTokenKeys", err)
	}
}

func TestIDTokenFetchesTheKeysThroughTheGivenClient(t *testing.T) {
	g := googletest.New(t)
	cfg := idTokenConfig(g)

	var seen []string

	cfg.Client = doerFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.URL.String())

		return loopbackClient().Do(req)
	})

	if _, err := auth.VerifyIDToken(context.Background(), cfg, g.IDToken(goodGrant(), testClientID)); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 1 || seen[0] != g.URL+"/certs" {
		t.Errorf("requests through the gated client = %v, want only the key set", seen)
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
