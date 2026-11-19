package owner

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/service/social/youtube"
)

// EnvTestSocialURL names a fake social network of the end-to-end suite: the address of a server on
// loopback that plays the network (SDD-020 section 12). It is for tests only.
const EnvTestSocialURL = "STUARTY_TEST_SOCIAL_URL"

// ErrTestSocial is the start-up refusal of STUARTY_TEST_SOCIAL_URL outside the test condition.
var ErrTestSocial = errors.New("owner: test social network refused")

// socialRegistry is the registry of the social network connectors of the build: YouTube, and the fake
// network of the end-to-end suite when STUARTY_TEST_SOCIAL_URL is set. Adding a network is its package
// and one Register line here (acceptance criterion 5 of epic 18).
func socialRegistry(getenv func(string) string, mode string) (*social.Registry, error) {
	reg := social.NewRegistry()

	if err := reg.Register(youtube.New(youtube.Config{})); err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(getenv(EnvTestSocialURL))
	if raw == "" {
		return reg, nil
	}

	if mode == config.ModeNetwork {
		return nil, fmt.Errorf("%w: %s works only in local mode; unset it", ErrTestSocial, EnvTestSocialURL)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Path != "" && u.Path != "/" || u.User != nil ||
		!loopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("%w: %s must be an http address on loopback, such as http://127.0.0.1:3104",
			ErrTestSocial, EnvTestSocialURL)
	}

	if err := reg.Register(fake.New(strings.TrimRight(raw, "/"))); err != nil {
		return nil, err
	}

	return reg, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}
