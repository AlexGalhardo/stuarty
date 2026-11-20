package owner

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// The sign-ins of a build (SDD-016 section 12): both providers, at their own hosts, and a host identifier
// kept in the workspace.
func TestSignInsHaveBothProvidersAtTheirOwnHosts(t *testing.T) {
	t.Parallel()

	o := &Owner{Dir: workspace.Dir(t.TempDir()), mode: config.ModeLocal}

	set, err := o.signIns(env(nil), nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := set.IDs(); !slices.Equal(got, []string{"openai", "openrouter"}) {
		t.Fatalf("sign-ins = %v, want openai and openrouter", got)
	}

	chatgpt, _ := set.Get("openai")
	if got := chatgpt.Describe().Hosts; !slices.Equal(got, []string{"api.openai.com", "auth.openai.com"}) {
		t.Errorf("Sign in with ChatGPT reaches %v", got)
	}

	if err := chatgpt.ValidateRedirect("http://127.0.0.1:7878/auth/callback"); err != nil {
		t.Errorf("a local server refuses Sign in with ChatGPT at 127.0.0.1: %v", err)
	}

	if _, err := os.Stat(filepath.Join(string(o.Dir), "providers", hostIDFile)); err != nil {
		t.Errorf("the host identifier is not kept in the workspace: %v", err)
	}
}

// D-094 and SDD-016 section 12.3: a server in network mode refuses Sign in with ChatGPT whatever the
// address the browser is at, 127.0.0.1 through a tunnel included; OpenRouter is still offered.
func TestNetworkModeRefusesSignInWithChatGPT(t *testing.T) {
	t.Parallel()

	o := &Owner{Dir: workspace.Dir(t.TempDir()), mode: config.ModeNetwork}

	set, err := o.signIns(env(nil), nil)
	if err != nil {
		t.Fatal(err)
	}

	chatgpt, _ := set.Get("openai")
	if err := chatgpt.ValidateRedirect("http://127.0.0.1:7878/auth/callback"); !errors.Is(err, oauth.ErrLocalOnly) {
		t.Errorf("network mode, Sign in with ChatGPT at 127.0.0.1 = %v, want ErrLocalOnly", err)
	}

	router, _ := set.Get("openrouter")
	if err := router.ValidateRedirect("https://stuarty.example.com/auth/callback"); err != nil {
		t.Errorf("network mode refuses OpenRouter at its https address: %v", err)
	}
}

func TestSignInsTakeTheFakeServerOnlyOnLoopbackInLocalMode(t *testing.T) {
	t.Parallel()

	o := &Owner{Dir: workspace.Dir(t.TempDir()), mode: config.ModeLocal}

	set, err := o.signIns(env(map[string]string{EnvTestSignInURL: "http://127.0.0.1:3105"}), nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"openai", "openrouter"} {
		x, _ := set.Get(id)
		if got := x.Describe().Hosts; !slices.Equal(got, []string{"127.0.0.1"}) {
			t.Errorf("%s reaches %v under the test address", id, got)
		}
	}

	for name, tc := range map[string]struct{ url, mode string }{
		"network mode":     {"http://127.0.0.1:3105", config.ModeNetwork},
		"a public host":    {"http://example.com", config.ModeLocal},
		"https":            {"https://127.0.0.1:3105", config.ModeLocal},
		"a path":           {"http://127.0.0.1:3105/x", config.ModeLocal},
		"user information": {"http://user:pw@127.0.0.1:3105", config.ModeLocal},
		"not an address":   {"::::", config.ModeLocal},
	} {
		o := &Owner{Dir: workspace.Dir(t.TempDir()), mode: tc.mode}

		if _, err := o.signIns(env(map[string]string{EnvTestSignInURL: tc.url}), nil); !errors.Is(err, ErrTestSignIn) {
			t.Errorf("%s: %v, want ErrTestSignIn", name, err)
		}
	}
}
