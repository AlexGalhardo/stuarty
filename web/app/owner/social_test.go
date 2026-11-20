package owner

import (
	"errors"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/config"
)

func env(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }

func TestSocialRegistryHasTheNetworksAtTheirOwnHostsByDefault(t *testing.T) {
	t.Parallel()

	reg, err := socialRegistry(env(nil), config.ModeLocal)
	if err != nil {
		t.Fatal(err)
	}

	if got := reg.Networks(); !slices.Equal(got, []string{"facebook", "instagram", "tiktok", "x", "youtube"}) {
		t.Errorf("networks = %v, want facebook, instagram, tiktok, x and youtube", got)
	}

	for n, want := range map[string][]string{
		"facebook":  {"graph.facebook.com", "rupload.facebook.com"},
		"instagram": {"graph.facebook.com", "rupload.facebook.com"},
		"x":         {"api.x.com"},
		"tiktok":    {"open.tiktokapis.com", "open-upload.tiktokapis.com"},
	} {
		c, _ := reg.Get(n)
		if got := c.Describe().Hosts; !slices.Equal(got, want) {
			t.Errorf("%s reaches %v, want %v", n, got, want)
		}
	}
}

func TestSocialRegistryTakesTheFakeNetworkOnlyOnLoopbackInLocalMode(t *testing.T) {
	t.Parallel()

	reg, err := socialRegistry(env(map[string]string{EnvTestSocialURL: "http://127.0.0.1:3104"}), config.ModeLocal)
	if err != nil {
		t.Fatal(err)
	}

	if got := reg.Networks(); !slices.Equal(got, []string{"facebook", "fake", "instagram", "tiktok", "x", "youtube"}) {
		t.Errorf("networks = %v, want the fake network besides the five", got)
	}

	// The networks that have a fake server speak to the loopback server alone, never to the network, while the
	// suite runs.
	for _, n := range []string{"facebook", "instagram", "x", "tiktok"} {
		c, _ := reg.Get(n)
		if got := c.Describe().Hosts; !slices.Equal(got, []string{"127.0.0.1"}) {
			t.Errorf("%s reaches %v under the test address", n, got)
		}
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
