package owner

import (
	"errors"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/config"
)

func env(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }

func TestSocialRegistryHasYouTubeAndNothingElseByDefault(t *testing.T) {
	t.Parallel()

	reg, err := socialRegistry(env(nil), config.ModeLocal)
	if err != nil {
		t.Fatal(err)
	}

	if got := reg.Networks(); !slices.Equal(got, []string{"youtube"}) {
		t.Errorf("networks = %v, want youtube only", got)
	}
}

func TestSocialRegistryTakesTheFakeNetworkOnlyOnLoopbackInLocalMode(t *testing.T) {
	t.Parallel()

	reg, err := socialRegistry(env(map[string]string{EnvTestSocialURL: "http://127.0.0.1:3104"}), config.ModeLocal)
	if err != nil {
		t.Fatal(err)
	}

	if got := reg.Networks(); !slices.Equal(got, []string{"fake", "youtube"}) {
		t.Errorf("networks = %v, want fake and youtube", got)
	}

	for name, tc := range map[string]struct{ url, mode string }{
		"network mode":         {"http://127.0.0.1:3104", config.ModeNetwork},
		"a public host":        {"http://example.com", config.ModeLocal},
		"https":                {"https://127.0.0.1:3104", config.ModeLocal},
		"a path":               {"http://127.0.0.1:3104/x", config.ModeLocal},
		"user information":     {"http://user:pw@127.0.0.1:3104", config.ModeLocal},
		"not an address":       {"::::", config.ModeLocal},
		"a name that resolves": {"http://localtest.me:3104", config.ModeLocal},
	} {
		if _, err := socialRegistry(
			env(map[string]string{EnvTestSocialURL: tc.url}),
			tc.mode,
		); !errors.Is(
			err,
			ErrTestSocial,
		) {
			t.Errorf("%s: %v, want ErrTestSocial", name, err)
		}
	}
}
