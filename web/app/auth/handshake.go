package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

const handshakeMessage = "stuarty-handshake-v1"

// HandshakePath is the route of the proof (SDD-007 section 3.8).
const HandshakePath = "/api/v1/handshake"

var nonceRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidNonce is a nonce of 32 lower-case hex characters.
func ValidNonce(n string) bool { return nonceRe.MatchString(n) }

// Proof is HMAC-SHA256(key = token, message = "stuarty-handshake-v1" + nonce) in hex.
func Proof(token, nonce string) string {
	m := hmac.New(sha256.New, []byte(token))
	m.Write([]byte(handshakeMessage + nonce))

	return hex.EncodeToString(m.Sum(nil))
}

// ErrImpostor means the server did not prove that it knows the token.
var ErrImpostor = errors.New("the server did not prove that it knows the workspace token; the token was not sent")

// Doer is what a client sends requests with: *http.Client, and the loopback
// client of internal/outbound.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Verify asks the server at base for the proof of a fresh nonce and compares
// it in constant time. A client calls it before it sends Authorization; an
// impostor that does not know the token cannot answer (SDD-007 section 3.8, T-30).
func Verify(ctx context.Context, client Doer, base, token string) error {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}

	nonce := hex.EncodeToString(raw[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+HandshakePath+"?nonce="+url.QueryEscape(nonce), http.NoBody)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	var body struct {
		Proof string `json:"proof"`
	}

	if res.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(res.Body, 1<<10)).Decode(&body) != nil ||
		!TokenMatches(Proof(token, nonce), body.Proof) {
		return ErrImpostor
	}

	return nil
}

// Client sends the token only after the server at Base proved that it knows
// it; the proof is asked for before every request that carries the token.
type Client struct {
	Base  string
	Token string
	Next  Doer
}

// Do implements Doer.
func (c *Client) Do(r *http.Request) (*http.Response, error) {
	if r.URL.Path != HandshakePath {
		if err := Verify(r.Context(), c.Next, c.Base, c.Token); err != nil {
			return nil, fmt.Errorf("auth: %w", err)
		}

		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+c.Token)
	}

	return c.Next.Do(r)
}
