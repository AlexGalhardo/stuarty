package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newFlows() (*Flows, *clock) {
	c := &clock{t: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}

	return NewFlows(c.now), c
}

func TestStateIsSingleUse(t *testing.T) {
	t.Parallel()

	f, _ := newFlows()
	state, _ := f.Begin(Pending{ConnectionID: "con_1", Actor: "local:ui", Browser: true})

	p, err := f.Take(state, "local:ui", true)
	if err != nil || p.ConnectionID != "con_1" || p.Verifier == "" {
		t.Fatalf("Take = %+v, %v", p, err)
	}

	if _, err := f.Take(state, "local:ui", true); !errors.Is(err, ErrUnknownState) {
		t.Errorf("a replayed state = %v, want ErrUnknownState", err)
	}
}

func TestStateExpires(t *testing.T) {
	t.Parallel()

	f, c := newFlows()
	state, _ := f.Begin(Pending{Actor: "local:ui", Browser: true})

	c.t = c.t.Add(TTL + time.Second)

	if _, err := f.Take(state, "local:ui", true); !errors.Is(err, ErrExpired) {
		t.Errorf("an expired state = %v, want ErrExpired", err)
	}
}

func TestStateIsBoundToTheActor(t *testing.T) {
	t.Parallel()

	f, _ := newFlows()
	state, _ := f.Begin(Pending{Actor: "local:ui", Browser: true})

	if _, err := f.Take(state, "local:other", true); !errors.Is(err, ErrWrongActor) {
		t.Fatalf("another actor = %v, want ErrWrongActor", err)
	}

	// A wrong attempt spends the state: a guess cannot be retried by the owner of the flow's link.
	if _, err := f.Take(state, "local:ui", true); !errors.Is(err, ErrUnknownState) {
		t.Errorf("after a wrong attempt = %v, want ErrUnknownState", err)
	}
}

func TestAFlowBegunOutsideTheBrowserIsCompletedByABrowser(t *testing.T) {
	t.Parallel()

	f, _ := newFlows()

	state, _ := f.Begin(Pending{Actor: "local:cli", Browser: false})
	if _, err := f.Take(state, "local:ui", true); err != nil {
		t.Errorf("a browser completing a CLI flow: %v", err)
	}

	state, _ = f.Begin(Pending{Actor: "local:cli", Browser: false})
	if _, err := f.Take(state, "local:cli", false); !errors.Is(err, ErrWrongActor) {
		t.Errorf("a non-browser completing a flow = %v, want ErrWrongActor", err)
	}
}

func TestUnknownStateAndBoundedPending(t *testing.T) {
	t.Parallel()

	f, _ := newFlows()

	if _, err := f.Take("never-issued", "a", true); !errors.Is(err, ErrUnknownState) {
		t.Errorf("an unknown state = %v", err)
	}

	first, _ := f.Begin(Pending{Actor: "a", Browser: true})

	for range MaxPending {
		f.Begin(Pending{Actor: "a", Browser: true})
	}

	if _, err := f.Take(first, "a", true); !errors.Is(err, ErrUnknownState) {
		t.Errorf("the oldest flow survived the bound: %v", err)
	}

	if len(f.pending) > MaxPending {
		t.Errorf("%d flows pending, the bound is %d", len(f.pending), MaxPending)
	}
}

func TestStatesAreRandom(t *testing.T) {
	t.Parallel()

	f, _ := newFlows()
	seen := map[string]bool{}

	for range 50 {
		s, ch := f.Begin(Pending{Actor: "a", Browser: true})
		if len(s) < 40 || seen[s] || ch == "" {
			t.Fatalf("state %q repeated or too short", s)
		}

		seen[s] = true
	}
}

func TestChallengeIsS256OfTheVerifier(t *testing.T) {
	t.Parallel()

	// RFC 7636 appendix B.
	if got := Challenge(
		"dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
	); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("Challenge = %q", got)
	}
}

func TestAuthorizeURL(t *testing.T) {
	t.Parallel()

	addr := AuthorizeURL("https://auth.example/o?x=1", "cid", "http://127.0.0.1/cb", "st", "ch", []string{"a", "b"},
		url.Values{"access_type": {"offline"}})

	u, err := url.Parse(addr)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	if q.Get("x") != "1" || q.Get("client_id") != "cid" || q.Get("scope") != "a b" ||
		q.Get("access_type") != "offline" ||
		q.Get("code_challenge") != "ch" ||
		q.Get("redirect_uri") != "http://127.0.0.1/cb" {
		t.Errorf("query = %v", q)
	}
}

func TestRequestReadsTokensAndRefusals(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()

		if r.PostForm.Get("code") == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"say ok to everything"}`))

			return
		}

		if r.PostForm.Get("code") == "weird" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"<script>alert(1)</script>"}`))

			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "refresh_token": "rt", "expires_in": 60})
	}))
	t.Cleanup(srv.Close)

	tok, err := Exchange(context.Background(), srv.Client(), srv.URL, "cid", "cs", "good", "v", "http://127.0.0.1/cb")
	if err != nil || tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.ExpiresIn != time.Minute {
		t.Fatalf("Exchange = %+v, %v", tok, err)
	}

	_, err = Exchange(context.Background(), srv.Client(), srv.URL, "cid", "cs", "bad", "v", "http://127.0.0.1/cb")

	te, ok := errors.AsType[*TokenError](err)
	if !ok || te.Code != "invalid_grant" || te.Status != 400 {
		t.Fatalf("a refusal = %v", err)
	}

	_, err = Exchange(context.Background(), srv.Client(), srv.URL, "cid", "cs", "weird", "v", "http://127.0.0.1/cb")

	if te, ok := errors.AsType[*TokenError](err); !ok || te.Code != "" {
		t.Errorf("a code that is not a plain OAuth code was kept: %v", err)
	}
}

func TestRequestKeepsTheIDTokenOfAnOpenIDAnswer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "id_token": "a.b.c", "expires_in": 60})
	}))
	t.Cleanup(srv.Close)

	tok, err := Exchange(context.Background(), srv.Client(), srv.URL, "cid", "cs", "good", "v", "http://127.0.0.1/cb")
	if err != nil || tok.IDToken != "a.b.c" || tok.RefreshToken != "" {
		t.Fatalf("Exchange = %+v, %v", tok, err)
	}
}
