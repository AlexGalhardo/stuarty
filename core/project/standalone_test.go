package project_test

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/project"
)

func TestCheckStandaloneTitle(t *testing.T) {
	if err := project.CheckStandaloneTitle("Quick reel edit"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := project.CheckStandaloneTitle(""); err == nil || err.Path != "name" || err.Code != "E_VALIDATION" {
		t.Fatalf("expected validation error on name, got %+v", err)
	}

	if err := project.CheckStandaloneTitle("   "); err == nil || err.Path != "name" {
		t.Fatalf("expected error for whitespace-only name, got %+v", err)
	}

	if err := project.CheckStandaloneTitle("bad\u202ename"); err == nil || err.Path != "name" {
		t.Fatalf("expected error for bidi control character, got %+v", err)
	}
}

func TestStandaloneNameFromFile(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "demo-clip.mp4", want: "demo-clip"},
		{in: `C:\clips\my_video.mov`, want: "my_video"},
		{in: "/tmp/nested/launch.teaser.webm", want: "launch.teaser"},
		{in: ".mp4", want: ".mp4"},
		{in: "   ", want: project.DefaultStandaloneName},
		{in: "", want: project.DefaultStandaloneName},
		{in: strings.Repeat("a", 150) + ".mp4", want: strings.Repeat("a", project.MaxTitle)},
	}

	for _, tc := range tests {
		if got := project.StandaloneNameFromFile(tc.in); got != tc.want {
			t.Errorf("StandaloneNameFromFile(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStandaloneDimensions(t *testing.T) {
	if w, h, ok := project.StandaloneDimensions(1920, 1080); !ok || w != 1920 || h != 1080 {
		t.Fatalf("got (%d,%d,%v), want (1920,1080,true)", w, h, ok)
	}

	if w, h, ok := project.StandaloneDimensions(1081, 1919); !ok || w != 1080 || h != 1918 {
		t.Fatalf("expected odd dims rounded down to even, got (%d,%d,%v)", w, h, ok)
	}

	if w, h, ok := project.StandaloneDimensions(8000, 10); !ok || w != 4096 || h != 16 {
		t.Fatalf("expected clamped dims, got (%d,%d,%v)", w, h, ok)
	}

	if _, _, ok := project.StandaloneDimensions(0, 1080); ok {
		t.Fatalf("expected ok=false for zero width")
	}
}

func TestCheckPromote(t *testing.T) {
	valid := project.Context{
		Name:        "Launch Video",
		Description: strings.Repeat("A full production description for the standalone video edit. ", 2),
		Goal: project.Goal{
			Specific:   "Publish a concise product launch short video.",
			Measurable: "Reach five thousand views in the first week.",
			Achievable: "All footage is already edited in the standalone timeline.",
			Relevant:   "Drives creator onboarding for the new release.",
			TimeBound:  "Export and schedule within three days.",
			Due:        "2026-12-15",
		},
	}

	opts := project.CheckOptions{Today: "2026-11-27", CheckDue: true}

	if errs := project.CheckPromote(true, valid, opts); len(errs) != 0 {
		t.Fatalf("expected valid promote, got %+v", errs)
	}

	if errs := project.CheckPromote(false, valid, opts); len(errs) != 1 || errs[0].Code != "E_INVALID_STATE" {
		t.Fatalf("expected E_INVALID_STATE for non-standalone project, got %+v", errs)
	}

	bad := valid
	bad.Name = "No"
	if errs := project.CheckPromote(true, bad, opts); len(errs) == 0 || errs[0].Code != "E_VALIDATION" {
		t.Fatalf("expected E_VALIDATION for invalid context, got %+v", errs)
	}
}
