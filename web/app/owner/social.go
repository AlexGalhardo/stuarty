package owner

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/facebook"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/service/social/instagram"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
	"github.com/AlexGalhardo/stuarty/service/social/youtube"
)

// reach is everything the service reaches out through: the connections over the vault, the AI providers
// over them, and the social network connectors.
type reach struct {
	conns     *connections.Manager
	provs     *providers.Registry
	socials   *social.Registry
	signInSet *oauth.SignIns
}

// outbound builds the destinations of the service (the connections and the AI providers) and the registry
// of the social networks.
func (o *Owner) outbound(getenv func(string) string, st service.Store, now func() time.Time) (reach, error) {
	conns, provs, err := o.destinations(getenv, st, now)
	if err != nil {
		return reach{}, err
	}

	socials, err := socialRegistry(getenv, o.mode)
	if err != nil {
		return reach{}, err
	}

	signInSet, err := o.signIns(getenv, now)
	if err != nil {
		return reach{}, err
	}

	return reach{conns: conns, provs: provs, socials: socials, signInSet: signInSet}, nil
}

// EnvTestSocialURL names a fake social network of the end-to-end suite: the address of a server on
// loopback that plays the network (SDD-020 section 12). It is for tests only.
const EnvTestSocialURL = "STUARTY_TEST_SOCIAL_URL"

// ErrTestSocial is the start-up refusal of STUARTY_TEST_SOCIAL_URL outside the test condition.
var ErrTestSocial = errors.New("owner: test social network refused")

// socialRegistry is the registry of the social network connectors of the build: YouTube, Instagram and
// Facebook, and the fake network of the end-to-end suite when STUARTY_TEST_SOCIAL_URL is set (then Instagram
// and Facebook also speak to that server alone, so the suite runs a whole Meta flow with no Meta account).
// Adding a network is its package and one Register line here (acceptance criteria 5 of epics 18 and 19).
func socialRegistry(getenv func(string) string, mode string) (*social.Registry, error) {
	reg := social.NewRegistry()

	testBase, err := testSocialBase(getenv, mode)
	if err != nil {
		return nil, err
	}

	metaCfg := meta.Config{}
	if testBase != "" {
		metaCfg = meta.AtBase(testBase)
	}

	for _, c := range []social.Connector{
		youtube.New(youtube.Config{}), instagram.New(metaCfg), facebook.New(metaCfg),
	} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	if testBase != "" {
		if err := reg.Register(fake.New(testBase)); err != nil {
			return nil, err
		}
	}

	return reg, nil
}

// testSocialBase is the address of the fake network of the suite, or empty when there is none: loopback, http
// and local mode only.
func testSocialBase(getenv func(string) string, mode string) (string, error) {
	raw := strings.TrimSpace(getenv(EnvTestSocialURL))
	if raw == "" {
		return "", nil
	}

	if err := requireTestEnv(getenv, EnvTestSocialURL, ErrTestSocial); err != nil {
		return "", err
	}

	if mode == config.ModeNetwork {
		return "", fmt.Errorf("%w: %s works only in local mode; unset it", ErrTestSocial, EnvTestSocialURL)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Path != "" && u.Path != "/" || u.User != nil ||
		!loopbackHost(u.Hostname()) {
		return "", fmt.Errorf("%w: %s must be an http address on loopback, such as http://127.0.0.1:3104",
			ErrTestSocial, EnvTestSocialURL)
	}

	return strings.TrimRight(raw, "/"), nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}
