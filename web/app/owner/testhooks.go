package owner

import (
	"fmt"
	"strings"
)

// testAddresses are the test-only variables that point a connector or a sign-in at an address of the
// environment (SDD-007 threat T-67), each with its start-up refusal.
var testAddresses = []struct {
	name    string
	refusal error
}{
	{EnvTestSocialURL, ErrTestSocial},
	{EnvTestSignInURL, ErrTestSignIn},
	{EnvTestGoogleURL, ErrTestGoogle},
}

// refuseTestAddresses is the start-up check of T-67, run before the workspace is taken so that a refused
// start leaves nothing behind. It covers every mode, also where the build would not read the variable (the
// Google one in local mode), because the specification promises the refusal whatever the mode.
func refuseTestAddresses(getenv func(string) string) error {
	for _, v := range testAddresses {
		if strings.TrimSpace(getenv(v.name)) == "" {
			continue
		}

		if err := requireTestEnv(getenv, v.name, v.refusal); err != nil {
			return err
		}
	}

	return nil
}

// requireTestEnv refuses a test-only address variable that is set outside APP_ENV=test (D-093, SDD-007
// threat T-67): a product binary never sends a token to an address that came from its environment. The
// caller has already seen the variable set, so a non-empty value is what is refused.
func requireTestEnv(getenv func(string) string, name string, refusal error) error {
	if getenv(envAppEnv) == appEnvTesting {
		return nil
	}

	return fmt.Errorf("%w: %s works only with %s=%s; unset it", refusal, name, envAppEnv, appEnvTesting)
}
