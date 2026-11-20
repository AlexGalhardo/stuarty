package owner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/config"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// TestSecurity_T67_TestAddressesAreHonouredOnlyUnderTheTestEnvironment: the variables that point a connector,
// a sign-in or the Google sign-in at an address of the environment take effect only with APP_ENV=test; set
// under any other environment the server refuses to start and names the variable, so a product binary never
// sends a token to an address it was handed.
func TestSecurity_T67_TestAddressesAreHonouredOnlyUnderTheTestEnvironment(t *testing.T) {
	t.Parallel()

	const addr = "http://127.0.0.1:3104"

	type probe func(getenv func(string) string) error

	probes := map[string]struct {
		name    string
		refusal error
		run     probe
	}{
		"social": {EnvTestSocialURL, ErrTestSocial, func(g func(string) string) error {
			_, err := socialRegistry(g, config.ModeLocal)

			return err
		}},
		"sign-in": {EnvTestSignInURL, ErrTestSignIn, func(g func(string) string) error {
			o := &Owner{Dir: workspace.Dir(t.TempDir()), mode: config.ModeLocal}
			_, err := o.signIns(g, nil)

			return err
		}},
		"google": {EnvTestGoogleURL, ErrTestGoogle, func(g func(string) string) error {
			_, err := googleEndpoints(g)

			return err
		}},
	}

	for label, p := range probes {
		for appEnv, wantRefused := range map[string]bool{
			"": true, "production": true, "development": true, "Test": true, "test": false,
		} {
			vars := map[string]string{p.name: addr}
			if appEnv != "" {
				vars[envAppEnv] = appEnv
			}

			err := p.run(env(vars))

			switch {
			case wantRefused && !errors.Is(err, p.refusal):
				t.Errorf("%s with APP_ENV=%q: %v, want the refusal", label, appEnv, err)
			case wantRefused && !strings.Contains(err.Error(), p.name):
				t.Errorf("%s with APP_ENV=%q: %v does not name %s", label, appEnv, err, p.name)
			case !wantRefused && err != nil:
				t.Errorf("%s with APP_ENV=test: %v", label, err)
			}
		}

		if err := p.run(env(nil)); err != nil {
			t.Errorf("%s with the variable unset: %v", label, err)
		}
	}

	openRefusesBeforeTheWorkspace(t, addr)
}

// openRefusesBeforeTheWorkspace is the start itself: each variable outside APP_ENV=test stops Open in local
// mode before the workspace folder exists, the Google one too although local mode never reads it, and the
// message names the variable and not its value.
func openRefusesBeforeTheWorkspace(t *testing.T, addr string) {
	t.Helper()

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	for _, v := range testAddresses {
		ws := filepath.Join(t.TempDir(), "ws")
		vars := map[string]string{workspace.EnvHome: ws, v.name: addr}

		o, err := Open(ctx, Config{Getenv: env(vars), Inline: true})
		if o != nil {
			_ = o.Close()
		}

		if !errors.Is(err, v.refusal) || !strings.Contains(err.Error(), v.name) || strings.Contains(err.Error(), addr) {
			t.Errorf("Open with %s and no APP_ENV: %v, want the refusal naming the variable only", v.name, err)
		}

		if _, serr := os.Stat(ws); !errors.Is(serr, fs.ErrNotExist) {
			t.Errorf("Open refused %s after it made the workspace: %v", v.name, serr)
		}
	}

	vars := map[string]string{
		workspace.EnvHome: filepath.Join(t.TempDir(), "ws"), EnvTestGoogleURL: addr, envAppEnv: appEnvTesting,
	}

	o, err := Open(ctx, Config{Getenv: env(vars), Inline: true})
	if err != nil {
		t.Fatalf("Open with %s under APP_ENV=test: %v", EnvTestGoogleURL, err)
	}

	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
}
