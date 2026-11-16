// Package bootstrap wires the application together. There is no hidden
// registration: everything the app uses is listed here.
package bootstrap

import (
	"context"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/inertia"

	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/app/listeners"
	"github.com/AlexGalhardo/vukem-video/web/database/migrations"
	"github.com/AlexGalhardo/vukem-video/web/database/seeders"
	"github.com/AlexGalhardo/vukem-video/web/resources"
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
	var app *vukem.App

	base := []vukem.Option{
		vukem.WithName("Vukem Video"),
		vukem.WithEnvDefaults(defaultEnv),
		vukem.WithProviders(Providers()...),
		vukem.WithMigrations(migrations.All()),
		vukem.WithSeeders(seeders.All()...),
		vukem.WithListeners(listeners.All()...),
		// The pages middleware of routes.Web reads the language, with or without the API.
		vukem.WithTranslations(resources.Lang()),
	}

	if api != nil {
		// First, so that the guard of the API is the outermost layer.
		base = append(base,
			vukem.WithOpenAPI(httpapi.Info()),
			vukem.WithRoutes(httpapi.Routes(api)),
			vukem.WithViews(resources.Views()),
			vukem.WithVite(resources.Public()),
			vukem.WithInertia(errorPages(func() *vukem.App { return app })),
		)
	}

	for _, fn := range routes.All() {
		base = append(base, vukem.WithRoutes(fn))
	}

	if api != nil {
		// After Web, whose default middleware the pages use.
		base = append(base, vukem.WithRoutes(routes.Pages(api)))
	}

	var err error

	app, err = vukem.New(append(base, opts...)...)
	if err != nil {
		return nil, err
	}

	app.AddCommand(Commands()...)

	return app, nil
}

// errorPages makes a page that does not exist answer the Inertia error page with
// the layout instead of the bare status line (ErrorFullPages). A route that does
// not match never runs the route middleware that shares the catalogue, so the
// config shares the one of the default locale for the error page; a matched page
// shares its own and wins.
func errorPages(app func() *vukem.App) inertia.Config {
	lazy := func(value func(*vukem.App) any) func(context.Context) (any, error) {
		return func(context.Context) (any, error) { return value(app()), nil }
	}

	return inertia.Config{
		ErrorFullPages: true,
		Shared: inertia.Props{
			"locale":  lazy(func(a *vukem.App) any { return a.I18n.Default() }),
			"locales": lazy(func(a *vukem.App) any { return a.I18n.Locales() }),
			"messages": lazy(func(a *vukem.App) any {
				return a.I18n.Messages(a.I18n.Default(), "ui.")
			}),
		},
	}
}
