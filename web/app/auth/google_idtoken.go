package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem/auth/jwt"
)

// Errors of the verification of an ID token (SDD-014 section 12.2). None carries the token, a claim or
// the text of the identity provider.
var (
	// ErrIDToken is a token that failed any check: signature, algorithm, issuer, audience, expiry,
	// nonce, subject or e-mail.
	ErrIDToken = errors.New("the ID token is not valid")
	// ErrIDTokenKeys is a key set that could not be fetched; the token was not judged.
	ErrIDTokenKeys = errors.New("the keys of the identity provider could not be fetched")
)

// maxSubjectBytes is the longest subject identifier an ID token may carry (OpenID Connect Core 5.1).
const maxSubjectBytes = 255

// Identity is what is kept of a verified ID token: the stable subject and the e-mail.
type Identity struct {
	Subject string
	Email   string
}

// IDTokenConfig says what a token must be to be accepted.
type IDTokenConfig struct {
	// Issuer is the exact value of iss.
	Issuer string
	// KeysURL is where the signing keys are published (the JWKS document).
	KeysURL string
	// ClientID is the audience: the client of the owner's own Google project.
	ClientID string
	// Nonce is the nonce of the flow the token must answer.
	Nonce string
	// Client fetches the keys; it must be the gated client of the connection.
	Client Doer
	Now    func() time.Time
}

// maxKeySetBytes is the most of a key set that is read.
const maxKeySetBytes = 1 << 20

// gatedKeys is the key set of the issuer, fetched once through the gated client of the connection and
// kept for the one verification it was made for. vukem's RemoteJWKS needs an *http.Client of its own,
// which the product may not build (SDD-007 section 10.2); this one asks the gate for every request.
type gatedKeys struct {
	client Doer
	url    string

	fetched bool
	keys    map[string]jwt.Key
	err     error
}

// SharedKeys implements jwt.SharedKeySet: an identity provider signs for all of its clients with the
// same keys, so the verifier insists on an issuer and an audience.
func (*gatedKeys) SharedKeys() bool { return true }

// Key implements jwt.KeySet.
func (g *gatedKeys) Key(ctx context.Context, kid string) (jwt.Key, error) {
	if !g.fetched {
		g.keys, g.err = g.fetch(ctx)
		g.fetched = true
	}

	if g.err != nil {
		return jwt.Key{}, fmt.Errorf("%w: %w", jwt.ErrKeysUnavailable, g.err)
	}

	if k, ok := g.keys[kid]; ok {
		return k, nil
	}

	return jwt.Key{}, errors.New("unknown key id")
}

func (g *gatedKeys) fetch(ctx context.Context) (map[string]jwt.Key, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the key set answered %d", res.StatusCode)
	}

	var set jwt.JWKS
	if err := json.NewDecoder(io.LimitReader(res.Body, maxKeySetBytes)).Decode(&set); err != nil {
		return nil, errors.New("the key set is not JSON")
	}

	keys := map[string]jwt.Key{}

	for i := range set.Keys {
		j := &set.Keys[i]
		if j.Use != "" && j.Use != "sig" {
			continue
		}

		if k, kerr := j.Key(); kerr == nil && k.Alg == jwt.AlgRS256 {
			keys[k.ID] = k
		}
	}

	return keys, nil
}

// VerifyIDToken checks the signature of an ID token against the keys of the issuer and then its
// claims, and returns the subject and the e-mail. The keys are fetched on every call (nothing is
// cached), so a connection that was removed or switched off stops the sign-in at the next request.
// Only RS256 is accepted; an e-mail that Google did not verify is refused.
func VerifyIDToken(ctx context.Context, cfg IDTokenConfig, raw string) (Identity, error) {
	if cfg.Client == nil || cfg.Issuer == "" || cfg.KeysURL == "" || cfg.ClientID == "" || cfg.Nonce == "" {
		return Identity{}, ErrIDToken
	}

	keys := &gatedKeys{client: cfg.Client, url: cfg.KeysURL}

	verifier := jwt.NewVerifier(keys, jwt.VerifyOptions{
		Algorithms: []string{jwt.AlgRS256},
		Issuer:     cfg.Issuer,
		Audience:   cfg.ClientID,
		Nonce:      cfg.Nonce,
		Now:        cfg.Now,
	})

	if verifier.Err() != nil {
		return Identity{}, ErrIDToken
	}

	claims, err := verifier.Verify(ctx, raw)
	if errors.Is(err, jwt.ErrKeysUnavailable) {
		return Identity{}, errors.Join(ErrIDTokenKeys, err)
	}

	if err != nil {
		return Identity{}, ErrIDToken
	}

	return identityOf(claims)
}

func identityOf(c jwt.Claims) (Identity, error) {
	subject := c.Subject
	if subject == "" || len(subject) > maxSubjectBytes || strings.ContainsFunc(subject, isControl) {
		return Identity{}, ErrIDToken
	}

	// Google writes a boolean, and some of its older libraries wrote the string "true".
	verified := false

	switch v := c.Extra["email_verified"].(type) {
	case bool:
		verified = v
	case string:
		verified = v == "true"
	}

	if !verified {
		return Identity{}, ErrIDToken
	}

	rawEmail, _ := c.Extra["email"].(string)

	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return Identity{}, ErrIDToken
	}

	return Identity{Subject: subject, Email: email}, nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
