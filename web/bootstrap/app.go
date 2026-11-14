// Package bootstrap wires the application together. There is no hidden
// registration: everything the app uses is listed here.
package bootstrap

import (
	vukem "github.com/AlexGalhardo/vukem"

	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/listeners"
	"github.com/AlexGalhardo/vukem-video/web/database/migrations"
	"github.com/AlexGalhardo/vukem-video/web/database/seeders"
	"github.com/AlexGalhardo/vukem-video/web/routes"
)

// defaultEnv is where the server listens until the workspace configuration
// (SDD-000 section 10, VUKEM_VIDEO_ADDR) replaces it. vukem reads HOST and PORT.
var defaultEnv = map[string]string{
	"HOST": "127.0.0.1",
	"PORT": "7878",
}

// New builds the application; cmd/app and the tests call it.
//
// The editor keeps its own jobs table (ADR-0018), so the vukem queue is not
// enabled here: WithJobsFunc is deliberately absent.
func New(opts ...vukem.Option) (*vukem.App, error) { return NewWith(nil, opts...) }

// NewWith is New with the HTTP API of SDD-003 section 6 installed over the
// given dependencies; nil installs none (the commands that never serve).
func NewWith(api *httpapi.Deps, opts ...vukem.Option) (*vukem.App, error) {
	base := []vukem.Option{
		vukem.WithName("Vukem Video"),
		vukem.WithEnvDefaults(defaultEnv),
		vukem.WithProviders(Providers()...),
		vukem.WithMigrations(migrations.All()),
		vukem.WithSeeders(seeders.All()...),
		vukem.WithListeners(listeners.All()...),
	}

	if api != nil {
		// First, so that the guard of the API is the outermost layer.
		base = append(base, vukem.WithOpenAPI(httpapi.Info()), vukem.WithRoutes(httpapi.Routes(api)))
	}

	for _, fn := range routes.All() {
		base = append(base, vukem.WithRoutes(fn))
	}

	app, err := vukem.New(append(base, opts...)...)
	if err != nil {
		return nil, err
	}

	app.AddCommand(Commands()...)

	return app, nil
}
