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

	cli := hintFor(SurfaceCLI, `start the transcription with job_start (kind "transcribe") then job_get`)
	if strings.Contains(cli, "job_") || !strings.Contains(cli, "transcribe") {
		t.Errorf("cli hint = %q", cli)
	}

	got := TextFor(SurfaceHTTP, "still running: call job_get again")
	if !strings.Contains(got, "GET /api/v1/jobs/{id}") {
		t.Errorf("text = %q", got)
	}
}
