package owner

import "fmt"

// requireTestEnv refuses a test-only address variable that is set outside APP_ENV=test (D-093, SDD-007
// threat T-67): a product binary never sends a token to an address that came from its environment. The
// caller has already seen the variable set, so a non-empty value is what is refused.
func requireTestEnv(getenv func(string) string, name string, refusal error) error {
	if getenv(envAppEnv) == appEnvTesting {
		return nil
	}

	return fmt.Errorf("%w: %s works only with %s=%s; unset it", refusal, name, envAppEnv, appEnvTesting)
}
