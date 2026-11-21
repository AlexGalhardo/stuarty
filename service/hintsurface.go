package service

import (
	"regexp"
	"slices"
	"strings"
)

// Surface is the way a caller reached the product. The hints of an error are written with the names
// of the MCP tools, the surface the agents read most; every other surface gets the same hint with the
// names of its own commands (VV-101). Only the text of the hint changes: the code, the message and the
// details are the same on every surface.
type Surface int

// The surfaces of the product.
const (
	SurfaceMCP Surface = iota
	SurfaceCLI
	SurfaceHTTP
	SurfaceUI
)

var (
	toolName = regexp.MustCompile(
		`\b(project_list|project_create|project_get|project_trash|command_help|media_import|media_inspect|` +
			`job_start|job_get|job_list|job_cancel|text_list|font_import)\b`)

	cliNames = map[string]string{
		"project_list": "project:list", "project_create": "project:create", "project_get": "project:show",
		"project_trash": "project:trash", "command_help": "command:help", "media_import": "media:import",
		"media_inspect": "media:inspect", "job_start": "export", "job_get": "job:show", "job_cancel": "job:cancel",
		"font_import": "font:import", "job_list": "job:list", "text_list": "text:list",
	}

	httpNames = map[string]string{
		"project_list": "GET /api/v1/projects", "project_create": "POST /api/v1/projects",
		"project_get": "GET /api/v1/projects/{id}", "project_trash": "POST /api/v1/projects/{id}/trash",
		"command_help": "GET /api/v1/commands", "media_import": "POST /api/v1/projects/{id}/media",
		"media_inspect": "GET /api/v1/projects/{id}/media", "job_start": "POST /api/v1/projects/{id}/jobs",
		"job_get": "GET /api/v1/jobs/{id}", "job_cancel": "POST /api/v1/jobs/{id}/cancel",
		"font_import": "POST /api/v1/fonts", "job_list": "GET /api/v1/projects/{id}/jobs",
		"text_list": "GET /api/v1/texts",
	}
)

// hintFor rewrites the tool names of a hint for the surface; the MCP surface keeps them.
func hintFor(s Surface, hint string) string {
	switch s {
	case SurfaceCLI:
		hint = strings.ReplaceAll(hint, `job_start (kind "transcribe")`, "transcribe")

		return toolName.ReplaceAllStringFunc(hint, func(n string) string { return cliNames[n] })
	case SurfaceHTTP:
		return toolName.ReplaceAllStringFunc(hint, func(n string) string { return httpNames[n] })
	case SurfaceUI:
		hint = strings.ReplaceAll(hint, "list the projects with project_list",
			"go back to the projects page to see them")

		return toolName.ReplaceAllStringFunc(hint, func(n string) string { return strings.ReplaceAll(n, "_", " ") })
	case SurfaceMCP:
		// The tools that list have no name of their own: job_get lists with a project and project_item lists the texts.
		hint = strings.ReplaceAll(hint, "job_list", "job_get")

		return strings.ReplaceAll(hint, "text_list", "project_item with action text")
	default:
		return hint
	}
}

var toolOfRoute = func() map[string]string {
	m := make(map[string]string, len(httpNames))
	for tool, route := range httpNames {
		m[route] = tool
	}

	return m
}()

// httpRoutes matches the routes of httpNames, the longest first so that a route is not cut at its prefix.
var httpRoutes = func() *regexp.Regexp {
	routes := make([]string, 0, len(toolOfRoute))
	for r := range toolOfRoute {
		routes = append(routes, r)
	}

	slices.SortFunc(routes, func(a, b string) int { return len(b) - len(a) })

	for i, r := range routes {
		routes[i] = regexp.QuoteMeta(r)
	}

	return regexp.MustCompile(strings.Join(routes, "|"))
}()

// CanonicalHint turns the HTTP form of a hint, as an HTTP server wrote it, back to the tool names. A
// client that relays the error of a server (the stdio MCP and the CLI over a running server) applies
// it before ForSurface, so the hint names the surface that answered and not the one that carried it.
func CanonicalHint(hint string) string {
	var b strings.Builder

	last := 0

	for _, m := range httpRoutes.FindAllStringIndex(hint, -1) {
		// A longer route that the table does not know ("/media/{asset_id}") stays as written.
		if m[1] < len(hint) && hint[m[1]] == '/' {
			continue
		}

		b.WriteString(hint[last:m[0]])
		b.WriteString(toolOfRoute[hint[m[0]:m[1]]])

		last = m[1]
	}

	b.WriteString(hint[last:])

	return b.String()
}

// ForSurface returns a copy of the error whose hint names the commands of the surface.
func (e *Error) ForSurface(s Surface) *Error {
	out := *e
	out.Hint = hintFor(s, e.Hint)

	return &out
}

// TextFor is hintFor for the text of a result, which also tells the caller what to call next.
func TextFor(s Surface, text string) string { return hintFor(s, text) }
