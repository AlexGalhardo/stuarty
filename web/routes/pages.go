package routes

import (
	"errors"
	"net/http"
	"regexp"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/i18n"
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/middleware"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
	"github.com/AlexGalhardo/vukem-video/web/resources"
)

// phase is the build's feature gate that the editor reads from its bootstrap
// (SDD-012 section 10).
var phase = string(command.Build)

var projectID = regexp.MustCompile(`^prj_[0-9a-hjkmnp-tv-z]{10}$`)

// editorBootstrap is the object that GET /editor/{project_id} writes into the
// page (SDD-012 section 2.2); resources/js/editor/boot/bootstrap.ts reads it.
// The session cookie set by /auth/launch authenticates the editor's calls, so
// no credential travels in the page (SDD-007 section 3.7).
type editorBootstrap struct {
	ProjectID string         `json:"project_id"`
	Mode      string         `json:"mode"`
	APIBase   string         `json:"api_base"`
	Locale    string         `json:"locale"`
	Locales   []string       `json:"locales"`
	Messages  map[string]any `json:"messages"`
	User      *struct{}      `json:"user"`
	Phase     string         `json:"phase"`
	Version   string         `json:"version"`
}

// Pages installs the browser routes of SDD-012 section 2: the Inertia pages,
// the editor, the language switcher and the static files. Each handler calls the
// Service like the HTTP API does and draws what it answers (ADR-0008). It runs
// after Web, whose default middleware it relies on, and after httpapi.Routes,
// whose guard and authentication cover these routes (SDD-007 section 5.1).
func Pages(d *httpapi.Deps) func(r *router.Router, app *vukem.App) error {
	return func(r *router.Router, app *vukem.App) error {
		// The static files carry no credential and nothing private: the build of
		// the editor, the fonts it bundles and robots.txt.
		r.UseServer(middleware.StaticFS(resources.Public(), middleware.StaticConfig{Prefix: "/"}))
		r.Use(app.Inertia.Middleware, inertia.I18n("ui."))

		r.Get("/{$}", func(c *web.Ctx) error { return c.Redirect("/projects") }).Name("home")

		r.Get("/projects", func(c *web.Ctx) error { return projectsPage(c, d) }).Name("projects.index")
		r.Post("/projects", func(c *web.Ctx) error { return projectCreate(c, d) }).Name("projects.store")

		r.Get("/editor/{project_id}", func(c *web.Ctx) error { return editorPage(c, d, app) }).Name("editor.show")

		// The page reads the version and the workspace from /api/v1/system itself.
		r.Get("/settings", func(c *web.Ctx) error {
			return inertia.Render(c, "Settings/Index", inertia.Props{})
		}).Name("settings.index")

		// The brand kit is read and saved by the page through /api/v1/brand-kit.
		r.Get("/settings/brand-kit", func(c *web.Ctx) error {
			return inertia.Render(c, "Settings/BrandKit", inertia.Props{})
		}).Name("settings.brand_kit")

		// The choice is kept in the lang cookie, which i18n.Middleware reads on
		// every request; Input accepts the JSON body the client sends.
		r.Post("/locale", func(c *web.Ctx) error {
			in, err := c.Input()
			if err != nil {
				return err
			}

			tag, _ := in["locale"].(string)

			l, ok := app.I18n.Match(tag)
			if !ok {
				return errs.ErrNotFound
			}

			// Secure only over TLS: loopback is plain HTTP (SDD-007 section 3.5).
			c.SetCookie(&http.Cookie{ //nolint:gosec // see above
				Name: "lang", Value: l, Path: "/", MaxAge: 365 * 24 * 3600, HttpOnly: true,
				Secure: c.Request().TLS != nil, SameSite: http.SameSiteLaxMode,
			})

			return c.Back("/projects")
		}).Name("locale.update")

		return nil
	}
}

func projectsPage(c *web.Ctx, d *httpapi.Deps) error {
	list, err := d.Service.ProjectList(c.Context(), service.ProjectListRequest{})
	if err != nil {
		return err
	}

	return inertia.Render(c, "Projects/Index", inertia.Props{"projects": nonNil(list.Projects)})
}

// projectCreate is the form of the list page. The app has no sessions, so a
// refused name cannot travel as a flashed validation error: the service error
// goes to the Inertia error page.
func projectCreate(c *web.Ctx, d *httpapi.Deps) error {
	in, err := c.Input()
	if err != nil {
		return err
	}

	name, _ := in["name"].(string)
	preset, _ := in["preset"].(string)

	res, err := d.Service.ProjectCreate(c.Context(), service.ProjectCreateRequest{Name: name, Preset: preset})
	if err != nil {
		return err
	}

	// The editor is not an Inertia page: a full visit, not an XHR navigation.
	return inertia.Location(c, "/editor/"+res.ProjectID)
}

func editorPage(c *web.Ctx, d *httpapi.Deps, app *vukem.App) error {
	id := c.Param("project_id")
	if !projectID.MatchString(id) {
		return errs.ErrNotFound
	}

	head, err := d.Service.ProjectGet(c.Context(), service.ProjectGetRequest{ProjectID: id})
	if err != nil {
		return editorFailure(c, err)
	}

	// A trashed project opens read-only from ALPHA; in the Prototype it is gone.
	if head.State != "active" {
		return errs.ErrNotFound
	}

	locale := i18n.Locale(c.Context())
	if locale == "" {
		locale = app.I18n.Default()
	}

	return c.Render("editor", map[string]any{"Bootstrap": editorBootstrap{
		ProjectID: id, Mode: "local", APIBase: httpapi.BasePath, Locale: locale, Locales: app.I18n.Locales(),
		Messages: app.I18n.Messages(locale, "ui.editor.", "ui.common.", "ui.theme.", "ui.nav."),
		Phase:    phase, Version: d.Version,
	}})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}

	return s
}

// editorFailure answers a project that the editor cannot open with the Inertia error page, which shows
// the code, the message and the hint of the service (SDD-012 section 4.4), instead of the framework's
// bare status line. Any other error is returned to the framework.
func editorFailure(c *web.Ctx, err error) error {
	se, ok := errors.AsType[*service.Error](err)
	if !ok {
		return err
	}

	status := se.HTTPStatus()
	// RenderStatus refuses 409, which the Inertia protocol keeps for version and location replies.
	if status == http.StatusConflict {
		status = http.StatusUnprocessableEntity
	}

	code := se.Code
	if specific, ok := se.Details["code"].(string); ok {
		code = specific
	}

	props := inertia.Props{"status": status, "message": se.Message, "code": code}
	if se.Hint != "" {
		props["hint"] = se.Hint
	}

	return inertia.RenderStatus(c, status, "Error", props)
}
