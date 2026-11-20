package owner

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	"github.com/AlexGalhardo/stuarty/web/app/mcp"

	"github.com/AlexGalhardo/vukem/database"
)

// EnvTestGoogleURL names a fake Google of the end-to-end suite: the address of a server on loopback that
// plays the identity provider of the optional Google sign-in (SDD-014 section 12.1). It is for tests only.
const EnvTestGoogleURL = "STUARTY_TEST_GOOGLE_URL"

// ErrTestGoogle is the start-up refusal of STUARTY_TEST_GOOGLE_URL when it is not an address on loopback.
var ErrTestGoogle = errors.New("owner: test Google refused")

// googleEndpoints are the addresses of the identity provider: Google's, or the fake of a test.
func googleEndpoints(getenv func(string) string) (auth.Endpoints, error) {
	raw := strings.TrimSpace(getenv(EnvTestGoogleURL))
	if raw == "" {
		return auth.GoogleEndpoints(), nil
	}

	if err := requireTestEnv(getenv, EnvTestGoogleURL, ErrTestGoogle); err != nil {
		return auth.Endpoints{}, err
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Path != "" && u.Path != "/" || u.User != nil || u.RawQuery != "" ||
		!loopbackHost(u.Hostname()) {
		return auth.Endpoints{}, fmt.Errorf("%w: %s must be an http address on loopback, such as http://127.0.0.1:3104",
			ErrTestGoogle, EnvTestGoogleURL)
	}

	return auth.TestEndpoints(raw), nil
}

// googleSignIn builds the optional Google sign-in of network mode over the connections. It is built always
// in network mode and is off until the owner turns it on; with it off the server opens no connection to
// Google (SDD-007 section 10.1 path 8).
func googleSignIn(
	getenv func(string) string, appURL string, db database.Executor, netAuth *auth.Network,
	conns *connections.Manager, now func() time.Time,
) (*auth.Google, error) {
	ep, err := googleEndpoints(getenv)
	if err != nil {
		return nil, err
	}

	return auth.NewGoogle(auth.GoogleOptions{
		DB: db, Users: netAuth.Users, Connections: conns, AppURL: appURL, Endpoints: ep, Now: now,
	})
}

// attachHandlers adds to the dependencies of the HTTP adapter the two that need the service and the
// connections: the MCP handler, and in network mode the Google sign-in.
func (o *Owner) attachHandlers(cfg Config, db database.Executor, conns *connections.Manager) error {
	var err error

	if o.netAuth != nil {
		if o.deps.Google, err = googleSignIn(
			cfg.Getenv,
			cfg.Resolved.AppURL,
			db,
			o.netAuth,
			conns,
			cfg.Now,
		); err != nil {
			return err
		}
	}

	o.deps.MCP, err = mcp.Handler(o.Service, mcp.Options{Version: o.version})

	return err
}
