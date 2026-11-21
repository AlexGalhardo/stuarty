package routes

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
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

		// Edit Image, the image editor (epic 28, image_pages.go).
		imagePages(r)

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

		// The Webhooks page (epic 59, SDD-022): the page reads and changes the outbound webhooks through
		// /api/v1/webhooks.
		r.Get("/webhooks", func(c *web.Ctx) error {
			return inertia.Render(c, "Webhooks/Index", inertia.Props{})
		}).Name("webhooks.index")

		// The redirect address of the sign-in to a social network (epic 18, SDD-020 section 13): the page
		// hands the state and the code to POST /api/v1/social/complete with the session of the browser.
		r.Get("/social/callback", func(c *web.Ctx) error {
			return inertia.Render(c, "Social/Callback", inertia.Props{})
		}).Name("social.callback")

		// The redirect address of the sign-in to an AI provider (epic 12, SDD-016 section 12): the page hands
		// the state, the code and the issued client id to POST /api/v1/providers/signin/complete with the
		// session of the browser.
		r.Get("/auth/callback", func(c *web.Ctx) error {
			return inertia.Render(c, "Providers/SignInCallback", inertia.Props{})
		}).Name("providers.signin.callback")

		// The publishing calendar (epic 20, SDD-020 sections 23 to 31): the page reads and changes the
		// publications through /api/v1/publications and follows them on /api/v1/publications/events.
		r.Get("/publish", func(c *web.Ctx) error {
			return inertia.Render(c, "Publish/Index", inertia.Props{})
		}).Name("dashboard.publish")

		// The approval queue, the rules and the cost ceilings (epic 05, SDD-015): the page reads and
		// changes them through /api/v1/approvals, /approval-rules and /cost, and follows the queue on
		// /api/v1/approvals/events.
		r.Get("/approvals", func(c *web.Ctx) error {
			return inertia.Render(c, "Approvals/Index", inertia.Props{})
		}).Name("approvals.index")

		// The asset library (epic 08, SDD-011 section 14.6): three sections over one library (visual, video and
		// sound, D-121); the pages read and change it through /api/v1/assets.
		r.Get("/visual-assets", func(c *web.Ctx) error {
			return inertia.Render(c, "VisualAssets/Index", inertia.Props{})
		}).Name("dashboard.visual-assets")
		r.Get("/video-assets", func(c *web.Ctx) error {
			return inertia.Render(c, "VideoAssets/Index", inertia.Props{})
		}).Name("dashboard.video-assets")
		r.Get("/sound-assets", func(c *web.Ctx) error {
			return inertia.Render(c, "SoundAssets/Index", inertia.Props{})
		}).Name("dashboard.sound-assets")

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

		// The optional Google sign-in (epic 24, SDD-007 section 4.7, SDD-014 section 12): the page where
		// Google sends the browser back (it needs no session and hands the state and the code to
		// POST /auth/google/complete), and the settings page. Network mode only; 404 in local mode.
		r.Get(auth.GoogleCallbackPath, func(c *web.Ctx) error {
			if d.Network == nil || d.Google == nil {
				return errs.ErrNotFound
			}

			return inertia.Render(c, "Auth/GoogleCallback", inertia.Props{})
		}).Name("login.google.callback")

		r.Get("/settings/google", func(c *web.Ctx) error {
			if d.Network == nil || d.Google == nil {
				return errs.ErrNotFound
			}

			return inertia.Render(c, "Settings/Google", inertia.Props{})
		}).Name("settings.google")

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

// sharedProps shares on every page the name of the app, the privilege of the session and, in network mode,
// the signed-in user (`auth.user`, null with no session and always in local
// mode). The caller was put in the context by the guard of the API.
func sharedProps(d *httpapi.Deps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			inertia.Share(ctx, "appName", appName)

			// Whether this session may do what only a person does (approve, set a price, keep a note, mark a skill
			// as read), so the pages disable those controls and say why instead of refusing after the click
			// (VV-319). The server decides; the page only reads it. The refusal of the call itself is unchanged.
			mode := "local"
			if d.Network != nil {
				mode = "network"
			}

			person, _ := service.CallerFrom(ctx)
			inertia.Share(ctx, "session", map[string]any{"privileged": person.Browser, "mode": mode})

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

	briefing, err := d.Service.BriefingQuestions(c.Context(), service.BriefingQuestionsRequest{ProjectID: id})
	if err != nil {
		return err
	}

	return inertia.Render(c, "Projects/Show", inertia.Props{
		"project": map[string]any{
			"id": desc.ProjectID, "name": desc.Name, "state": desc.State,
			"description": desc.Description, "goal": desc.Goal, "complete": desc.Complete,
		},
		"production": desc.Production, "items": nonNil(desc.Items), "brief": brief, "briefing": briefing,
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

	text := func(m map[string]any, k string) string {
		v, _ := m[k].(string)
		return v
	}

	goal, _ := in["goal"].(map[string]any)

	res, err := d.Service.ProjectCreate(c.Context(), service.ProjectCreateRequest{
		Name: text(in, "name"), Description: text(in, "description"),
		Goal: service.ProjectGoal{
			Specific: text(
				goal,
				"specific",
			),
			Measurable: text(goal, "measurable"),
			Achievable: text(goal, "achievable"),
			Relevant:   text(goal, "relevant"),
			TimeBound:  text(goal, "time_bound"),
			Due:        text(goal, "due"),
		},
	})
	if err != nil {
		return err
	}

	// A new project has no timeline: its page asks for the size when a video is added (epic 56). The page is
	// not an editor, but it is a full visit as well, so that the location is the same kind of answer.
	return inertia.Location(c, "/projects/"+res.ProjectID)
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

	// A project with no timeline has no document to open: its page is where a video is added (epic 56).
	if head.TimelineID == "" {
		return c.Redirect("/projects/" + head.ProjectID)
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
