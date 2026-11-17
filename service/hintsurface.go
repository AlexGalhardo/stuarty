package service

import (
	"regexp"
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
			`job_start|job_get|job_cancel|font_import)\b`)

	cliNames = map[string]string{
		"project_list": "project:list", "project_create": "project:create", "project_get": "project:show",
		"project_trash": "project:trash", "command_help": "command:help", "media_import": "media:import",
		"media_inspect": "media:inspect", "job_start": "export", "job_get": "job:show", "job_cancel": "job:cancel",
		"font_import": "font:import",
	}

	httpNames = map[string]string{
		"project_list": "GET /api/v1/projects", "project_create": "POST /api/v1/projects",
		"project_get": "GET /api/v1/projects/{id}", "project_trash": "POST /api/v1/projects/{id}/trash",
		"command_help": "GET /api/v1/commands", "media_import": "POST /api/v1/projects/{id}/media",
		"media_inspect": "GET /api/v1/projects/{id}/media", "job_start": "POST /api/v1/projects/{id}/jobs",
		"job_get": "GET /api/v1/jobs/{id}", "job_cancel": "POST /api/v1/jobs/{id}/cancel",
		"font_import": "POST /api/v1/fonts",
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
	default:
		return hint
	}
}

// ForSurface returns a copy of the error whose hint names the commands of the surface.
func (e *Error) ForSurface(s Surface) *Error {
	out := *e
	out.Hint = hintFor(s, e.Hint)

	return &out
}

// TextFor is hintFor for the text of a result, which also tells the caller what to call next.
func TextFor(s Surface, text string) string { return hintFor(s, text) }
