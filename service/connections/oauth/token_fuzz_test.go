package oauth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// answer is a token endpoint that gives one status and one body, whatever it is asked.
type answer struct {
	status int
	body   []byte
}

func (a answer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: a.status, Body: io.NopCloser(bytes.NewReader(a.body))}, nil
}

// FuzzTokenResponse feeds any status and body as the answer of a token endpoint, the untrusted half of an OAuth
// callback (SDD-008 section 13.2): no panic; a success always carries an access token and never a negative
// lifetime; a refusal is a TokenError whose code is a short snake_case word or empty, so the text of the
// endpoint never reaches a message.
func FuzzTokenResponse(f *testing.F) {
	f.Add(200, []byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"scope":"a b"}`))
	f.Add(200, []byte(`{"access_token":"at","expires_in":9227000000}`))
	f.Add(200, []byte(`{"access_token":"at","expires_in":-5}`))
	f.Add(400, []byte(`{"error":"invalid_grant"}`))
	f.Add(400, []byte(`{"error":"<script>alert(1)</script>"}`))
	f.Add(200, []byte(`{}`))
	f.Add(500, []byte(`not json`))

	f.Fuzz(func(t *testing.T, status int, body []byte) {
		tok, err := Request(context.Background(), answer{status, body}, "https://token.example/", url.Values{})
		if err == nil {
			if tok.AccessToken == "" || tok.ExpiresIn < 0 {
				t.Fatalf("%d %q: a success with no access token or a negative lifetime (%v)", status, body, tok.ExpiresIn)
			}

			return
		}

		var te *TokenError
		if !errors.As(err, &te) {
			t.Fatalf("%d %q: %v is not a TokenError", status, body, err)
		}

		if te.Code != "" && !codePattern.MatchString(te.Code) {
			t.Fatalf("%d %q: the code %q of the endpoint reached the error", status, body, te.Code)
		}
	})
}

// A lifetime longer than a Duration holds is no lifetime, not one that overflowed: 18446744074 seconds would
// wrap to a quarter of a second and make every use of the connection refresh its token.
func TestTokenLifetimeThatOverflowsIsUnknown(t *testing.T) {
	t.Parallel()

	for _, secs := range []string{"9227000000", "18446744074", "9223372036854775807", "-5"} {
		tok, err := Request(context.Background(), answer{200, []byte(`{"access_token":"at","expires_in":` + secs + `}`)},
			"https://token.example/", url.Values{})
		if err != nil {
			t.Fatal(err)
		}

		if tok.ExpiresIn != 0 {
			t.Errorf("expires_in %s: lifetime %v, want 0 (unknown)", secs, tok.ExpiresIn)
		}
	}

	tok, err := Request(context.Background(), answer{200, []byte(`{"access_token":"at","expires_in":3600}`)},
		"https://token.example/", url.Values{})
	if err != nil || tok.ExpiresIn != time.Hour {
		t.Errorf("expires_in 3600: %v, %v", tok.ExpiresIn, err)
	}
}
