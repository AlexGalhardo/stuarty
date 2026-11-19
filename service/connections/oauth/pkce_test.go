package oauth_test

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
)

// The S256 challenge is the one of the example of RFC 7636 appendix B, so a provider that checks it with
// any other implementation agrees.
func TestChallengeIsTheS256ExampleOfRFC7636(t *testing.T) {
	t.Parallel()

	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)

	if got := oauth.Challenge(verifier); got != challenge {
		t.Fatalf("Challenge(%q) = %q, want %q", verifier, got, challenge)
	}
}

// Every flow gets its own verifier, long enough for RFC 7636 (43 to 128 characters of the unreserved set),
// and its own state.
func TestFlowsMakeLongDistinctVerifiersAndStates(t *testing.T) {
	t.Parallel()

	flows := oauth.NewFlows(nil)
	seen := map[string]bool{}

	for range 20 {
		state, challenge := flows.Begin(oauth.Pending{Actor: "local:ui", Browser: true})

		p, err := flows.Take(state, "local:ui", true)
		if err != nil {
			t.Fatal(err)
		}

		if n := len(p.Verifier); n < 43 || n > 128 || oauth.Challenge(p.Verifier) != challenge {
			t.Fatalf("verifier of %d characters, challenge %q", n, challenge)
		}

		for _, v := range []string{state, p.Verifier} {
			if len(v) < 40 || seen[v] {
				t.Fatalf("%q is short or was issued before", v)
			}

			seen[v] = true
		}
	}
}
