package oauth_test

import (
	"bytes"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
)

// FuzzParseEnvelope feeds any bytes as the secret of a sign-in connection (SDD-008 section 13.2): no panic, an
// API key is never an envelope, and an envelope written again reads back to the same bytes.
func FuzzParseEnvelope(f *testing.F) {
	pending, _ := oauth.PendingEnvelope("openai", "client", "host").Marshal()
	f.Add(pending)
	f.Add([]byte(`sk-live-1234`))
	f.Add([]byte(`{"format":"stuarty-signin","version":1,"provider":"x","access_token":"\u0000"}`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, secret []byte) {
		env, ok := oauth.ParseEnvelope(secret)
		if !ok {
			return
		}

		if len(secret) == 0 || secret[0] != '{' {
			t.Fatalf("%q is an envelope", secret)
		}

		once, err := env.Marshal()
		if err != nil {
			t.Fatal(err)
		}

		again, ok := oauth.ParseEnvelope(once)
		if !ok {
			t.Fatalf("a written envelope does not parse: %s", once)
		}

		twice, err := again.Marshal()
		if err != nil || !bytes.Equal(once, twice) {
			t.Fatalf("the stored form is not stable:\n%s\n%s", once, twice)
		}
	})
}
