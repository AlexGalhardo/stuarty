package service

import (
	"strings"
	"testing"
)

func TestHintNamesTheCommandsOfTheSurface(t *testing.T) {
	t.Parallel()

	e := &Error{Code: "E_NOT_FOUND", Message: "no such project.", Hint: "list the projects with project_list"}

	for s, want := range map[Surface]string{
		SurfaceMCP:  "list the projects with project_list",
		SurfaceCLI:  "list the projects with project:list",
		SurfaceHTTP: "list the projects with GET /api/v1/projects",
		SurfaceUI:   "go back to the projects page to see them",
	} {
		got := e.ForSurface(s)
		if got.Hint != want || got.Code != e.Code || got.Message != e.Message {
			t.Errorf("surface %d: %+v, want hint %q", s, got, want)
		}
	}

	if e.Hint != "list the projects with project_list" {
		t.Error("ForSurface changed the original")
	}

	// A client of a server relays the hint the server wrote for HTTP; the canonical form comes back
	// to the tool names so that the stdio MCP and the CLI name their own surface (VV-168).
	for _, h := range []string{
		"list the projects with project_list",
		"list the jobs of the project with job_get",
		`start the transcription with job_start (kind "transcribe") then job_get`,
		"import it again with media_import or create a project with project_create",
	} {
		if got := CanonicalHint(hintFor(SurfaceHTTP, h)); got != h {
			t.Errorf("CanonicalHint(HTTP(%q)) = %q", h, got)
		}
	}

	cli := hintFor(SurfaceCLI, `start the transcription with job_start (kind "transcribe") then job_get`)
	if strings.Contains(cli, "job_") || !strings.Contains(cli, "transcribe") {
		t.Errorf("cli hint = %q", cli)
	}

	got := TextFor(SurfaceHTTP, "still running: call job_get again")
	if !strings.Contains(got, "GET /api/v1/jobs/{id}") {
		t.Errorf("text = %q", got)
	}
}

// A hint that lists names the command, the route or the tool of the surface that answered, and never a Go
// function (VV-310).
func TestListHintsNameTheSurface(t *testing.T) {
	t.Parallel()

	const h = "list the jobs of the project with job_list, or the texts with text_list"

	for s, want := range map[Surface]string{
		SurfaceCLI:  "list the jobs of the project with job:list, or the texts with text:list",
		SurfaceHTTP: "list the jobs of the project with GET /api/v1/projects/{id}/jobs, or the texts with GET /api/v1/texts",
		SurfaceMCP:  "list the jobs of the project with job_get, or the texts with project_item with action text",
	} {
		if got := hintFor(s, h); got != want {
			t.Errorf("surface %d: %q, want %q", s, got, want)
		}
	}

	if got := CanonicalHint(hintFor(SurfaceHTTP, h)); got != h {
		t.Errorf("CanonicalHint(HTTP) = %q, want %q", got, h)
	}

	if e := jobNotFound(
		"job_x",
	); strings.Contains(e.Hint, "TextList") ||
		e.Hint != "list the jobs of the project with job_list" {
		t.Errorf("jobNotFound hint = %q", e.Hint)
	}
}

// A refusal for a person-only operation says where the person does it, not the Approvals page (VV-312).
func TestPrivilegedHintsNameTheRightPage(t *testing.T) {
	t.Parallel()

	for method, want := range map[string]string{
		"SocialConnectComplete": "Connections page",
		"SocialPriceSet":        "Connections page",
		"ProviderPriceSet":      "Connections page",
		"ApprovalDecide":        "Approvals page",
	} {
		if h := privilegedError(method).Hint; !strings.Contains(h, want) {
			t.Errorf("%s: hint %q, want %q", method, h, want)
		}
	}
}
