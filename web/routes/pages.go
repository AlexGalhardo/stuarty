package routes

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"
	"github.com/AlexGalhardo/stuarty/web/resources"

	vukem "github.com/AlexGalhardo/vukem"
	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/i18n"
	"github.com/AlexGalhardo/vukem/inertia"
	"github.com/AlexGalhardo/vukem/middleware"
	"github.com/AlexGalhardo/vukem/router"
	"github.com/AlexGalhardo/vukem/web"
)

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
		r.Use(app.Inertia.Middleware, inertia.I18n("ui."), sharedProps(d))

		// The home and the sections that have no page of their own yet (dashboard.go).
		dashboardPages(r)

		r.Get("/projects", func(c *web.Ctx) error { return projectsPage(c, d) }).Name("projects.index")
		r.Post("/projects", func(c *web.Ctx) error { return projectCreate(c, d) }).Name("projects.store")

		r.Get("/projects/{project_id}", func(c *web.Ctx) error { return projectPageShow(c, d) }).Name("projects.show")

		r.Get("/editor/{project_id}", func(c *web.Ctx) error { return editorPage(c, d, app) }).Name("editor.show")

		// The page reads the version and the workspace from /api/v1/system itself.
		r.Get("/settings", func(c *web.Ctx) error {
			return inertia.Render(c, "Settings/Index", inertia.Props{})
		}).Name("settings.index")

		// The Connections section (epic 04, SDD-014): the page reads and changes the connections
		// through /api/v1/connections.
		r.Get("/connections", func(c *web.Ctx) error {
			return inertia.Render(c, "Connections/Index", inertia.Props{})
		}).Name("connections.index")

		// The approval queue, the rules and the cost ceilings (epic 05, SDD-015): the page reads and
		// changes them through /api/v1/approvals, /approval-rules and /cost, and follows the queue on
		// /api/v1/approvals/events.
		r.Get("/approvals", func(c *web.Ctx) error {
			return inertia.Render(c, "Approvals/Index", inertia.Props{})
		}).Name("approvals.index")

		// The brand kit is read and saved by the page through /api/v1/brand-kit.
		r.Get("/settings/brand-kit", func(c *web.Ctx) error {
			return inertia.Render(c, "Settings/BrandKit", inertia.Props{})
		}).Name("settings.brand_kit")

		// Network mode only (SDD-012 section 2.1): in local mode there are no users and
		// no keys, so these answer 404. The pages post to /auth/login and /auth/logout
		// and call /keys of the API, which SDD-007 section 4.1 names.
		r.Get(httpapi.LoginPath, func(c *web.Ctx) error {
			if d.Network == nil {
				return errs.ErrNotFound
			}

			return inertia.Render(c, "Auth/Login", inertia.Props{"next": safeNext(c.Query("next"))})
		}).Name("login")

		r.Get("/settings/keys", func(c *web.Ctx) error {
			if d.Network == nil {
				return errs.ErrNotFound
			}

			return inertia.Render(c, "Settings/Keys", inertia.Props{})
		}).Name("settings.keys")

		return nil
	}
}

// appName is the name the layout shows and gives the logo as its accessible name.
const appName = "Stuarty"

// sharedUser is the account that network mode shows in the header; the layout
// draws the keys link and the log out button when it is not null.
type sharedUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// sharedProps shares on every page the name of the app and, in network mode,
// the signed-in user (`auth.user`, null with no session and always in local
// mode). The caller was put in the context by the guard of the API.
func sharedProps(d *httpapi.Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			inertia.Share(ctx, "appName", appName)

			if d.Network != nil && d.Auth != nil {
				var user *sharedUser

				if c, ok := service.CallerFrom(ctx); ok && c.OwnerUserID != "" && c.Browser {
					if u, err := d.Auth.Users.ByID(ctx, c.OwnerUserID); err == nil {
						user = &sharedUser{ID: u.ID, Email: u.Email, Role: u.Role}
					}
				}

				inertia.Share(ctx, "auth", map[string]any{"user": user})
			}

			next.ServeHTTP(w, r)
		})
	}
}

// safeNext is the page the login goes back to: a path of this server, never a
// scheme, a host or a protocol-relative address that would send a person away.
//
// Control characters and backslashes are refused first, since browsers drop or
// rewrite them while resolving (`/<tab>/host` becomes `//host`), and the answer is the
// parsed and re-encoded path, never the raw input. It matches resources/js/lib/safeNext.ts.
func safeNext(next string) string {
	const fallback = "/projects"

	if !strings.HasPrefix(next, "/") || strings.ContainsFunc(next, isControlOrBackslash) {
		return fallback
	}

	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" ||
		!strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") ||
		strings.ContainsFunc(u.Path, isControlOrBackslash) ||
		u.Path == httpapi.LoginPath || strings.HasPrefix(u.Path, httpapi.LoginPath+"/") {
		return fallback
	}

	out := u.RequestURI()
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}

	return out
}

func isControlOrBackslash(r rune) bool { return r < 0x20 || r == 0x7f || r == '\\' }

func projectsPage(c *web.Ctx, d *httpapi.Deps) error {
	list, err := d.Service.ProjectList(c.Context(), service.ProjectListRequest{})
	if err != nil {
		return err
	}

	return inertia.Render(c, "Projects/Index", inertia.Props{"projects": nonNil(list.Projects)})
}

// projectPageShow is the overview of one project as a production (SDD-012 section 2.1): its brief and its items.
func projectPageShow(c *web.Ctx, d *httpapi.Deps) error {
	id := c.Param("project_id")
	if !projectID.MatchString(id) {
		return errs.ErrNotFound
	}

	desc, err := d.Service.ProjectDescribe(c.Context(), service.ProjectDescribeRequest{ProjectID: id})
	if err != nil {
		return err
	}

	brief, err := d.Service.BriefGet(c.Context(), service.BriefGetRequest{ProjectID: id})
	if err != nil {
		return err
	}

	return inertia.Render(c, "Projects/Show", inertia.Props{
		"project":    map[string]any{"id": desc.ProjectID, "name": desc.Name, "state": desc.State},
		"production": desc.Production, "items": nonNil(desc.Items), "brief": brief,
	})
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

	// A trashed project does not open in the editor: the page says where it is and
	// how to bring it back (SDD-012 section 2.1, the trash of the projects page).
	if head.State != "active" {
		return inertia.RenderStatus(c, http.StatusNotFound, "Error", inertia.Props{
			"status": http.StatusNotFound, "message": "This project is in the trash.",
			"hint": "restore it from the trash on the projects page, then open it again",
		})
	}

	locale := i18n.Locale(c.Context())
	if locale == "" {
		locale = app.I18n.Default()
	}

	return c.Render("editor", map[string]any{"Bootstrap": editorBootstrap{
		ProjectID: id, Mode: "local", APIBase: httpapi.BasePath, Locale: locale, Locales: app.I18n.Locales(),
		Messages: app.I18n.Messages(locale, "ui.editor.", "ui.common.", "ui.theme.", "ui.nav."),
		Version:  d.Version,
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

	se = se.ForSurface(service.SurfaceUI)
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
