// Package bootstrap wires the application together. There is no hidden
// registration: everything the app uses is listed here.
package bootstrap

import (
	vukem "github.com/AlexGalhardo/vukem"

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
func New(opts ...vukem.Option) (*vukem.App, error) {
	base := []vukem.Option{
		vukem.WithName("Vukem Video"),
		vukem.WithEnvDefaults(defaultEnv),
		vukem.WithProviders(Providers()...),
		vukem.WithMigrations(migrations.All()),
		vukem.WithSeeders(seeders.All()...),
		vukem.WithListeners(listeners.All()...),
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
