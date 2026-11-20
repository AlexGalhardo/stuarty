package owner

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/connections/oauth"
	"github.com/AlexGalhardo/stuarty/service/providers/openai"
	"github.com/AlexGalhardo/stuarty/service/providers/openrouter"
)

// EnvTestSignInURL names a fake authorization server of the end-to-end suite: the address of a server on
// loopback that plays the sign-in of the AI providers (SDD-016 section 12.5). It is for tests only.
const EnvTestSignInURL = "STUARTY_TEST_SIGNIN_URL"

// ErrTestSignIn is the start-up refusal of STUARTY_TEST_SIGNIN_URL outside the test condition.
var ErrTestSignIn = errors.New("owner: test sign-in refused")

// hostIDFile is the file of the workspace that keeps the identifier of this host the provider of a ChatGPT
// sign-in asks for; it is not a secret.
const hostIDFile = "host-id"

// signIns is the set of the sign-ins to an AI provider of this build (SDD-016 section 12): Sign in with
// ChatGPT and OpenRouter, or both against the fake server of the end-to-end suite when
// STUARTY_TEST_SIGNIN_URL is set. Adding a provider with a sign-in is its package and one line here.
func (o *Owner) signIns(getenv func(string) string, now func() time.Time) (*oauth.SignIns, error) {
	hostID, err := oauth.HostID(filepath.Join(string(o.Dir), "providers", hostIDFile))
	if err != nil {
		return nil, err
	}

	chatgpt := openai.SignInConfig{HostID: hostID, Now: now}
	router := openrouter.SignInConfig{}

	if raw := strings.TrimSpace(getenv(EnvTestSignInURL)); raw != "" {
		if o.mode == config.ModeNetwork {
			return nil, fmt.Errorf("%w: %s works only in local mode; unset it", ErrTestSignIn, EnvTestSignInURL)
		}

		u, perr := url.Parse(raw)
		if perr != nil || u.Scheme != "http" || u.Path != "" && u.Path != "/" || u.User != nil ||
			!loopbackHost(u.Hostname()) {
			return nil, fmt.Errorf("%w: %s must be an http address on loopback, such as http://127.0.0.1:3105",
				ErrTestSignIn, EnvTestSignInURL)
		}

		base := strings.TrimRight(raw, "/")

		chatgpt.AuthURL, chatgpt.TokenURL, chatgpt.Endpoint = base+"/api/accounts/authorize",
			base+"/api/accounts/oauth/token", base+"/v1"
		router.AuthURL, router.KeysURL, router.Endpoint = base+"/auth", base+"/api/v1/auth/keys", base+"/api/v1"
	}

	return oauth.NewSignIns(openai.NewSignIn(chatgpt), openrouter.NewSignIn(router)), nil
}
