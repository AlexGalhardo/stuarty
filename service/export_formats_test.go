package service

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/otio"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestExportFormatsAreValidated(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{"": "mp4", "mp4": "mp4", "gif": "gif", "wav": "wav", "aac": "aac"} {
		got, err := checkExportFormat(name)
		if err != nil || got != want {
			t.Errorf("checkExportFormat(%q) = %q, %v; want %q", name, got, err, want)
		}
	}

	for _, name := range []string{"GIF", "mov", "webm", "bundle", "m4a", " mp4"} {
		_, err := checkExportFormat(name)
		if err == nil || err.Code != command.CodeValidation || err.Path != "args.format" {
			t.Errorf("checkExportFormat(%q) = %v; want E_VALIDATION at args.format", name, err)
		}
	}
}

func TestExportFormatsFilesAndContentTypes(t *testing.T) {
	t.Parallel()

	for format, want := range map[string][2]string{
		"":    {"job_x.mp4", "video/mp4"},
		"mp4": {"job_x.mp4", "video/mp4"},
		"gif": {"job_x.gif", "image/gif"},
		"wav": {"job_x.wav", "audio/wav"},
		"aac": {"job_x.m4a", "audio/mp4"},
	} {
		f, _, ok := exportFormatOf(format)
		if !ok || exportFileName("job_x", format) != want[0] || f.contentType != want[1] {
			t.Errorf(
				"%q: file %q, type %q, ok %v; want %v",
				format,
				exportFileName("job_x", format),
				f.contentType,
				ok,
				want,
			)
		}
	}
}

func TestExportFormatsLimitsAreELimit(t *testing.T) {
	t.Parallel()

	ok := timeline.Settings{Width: 4096, Height: 4096, FPS: timeline.Rational{Num: 120, Den: 1}}
	if err := checkExportSettings(&ok); err != nil {
		t.Fatalf("the ceilings themselves are allowed: %v", err)
	}

	cases := map[string]timeline.Settings{
		"width":  {Width: 4098, Height: 1920, FPS: timeline.Rational{Num: 30, Den: 1}},
		"height": {Width: 1080, Height: 4098, FPS: timeline.Rational{Num: 30, Den: 1}},
		"fps":    {Width: 1080, Height: 1920, FPS: timeline.Rational{Num: 121, Den: 1}},
	}

	for what, st := range cases {
		err := checkExportSettings(&st)
		if err == nil || err.Code != command.CodeLimit || err.Details["limit"] != what {
			t.Errorf("%s: err = %v; want E_LIMIT naming %s", what, err, what)
		}
	}

	// 120000/1001 is below 120 fps.
	fine := timeline.Settings{Width: 1080, Height: 1920, FPS: timeline.Rational{Num: 120000, Den: 1001}}
	if err := checkExportSettings(&fine); err != nil {
		t.Errorf("119.88 fps: %v", err)
	}
}

func TestOtioMediaIsMatchedByNameNeverOpened(t *testing.T) {
	t.Parallel()

	doc := &timeline.Document{Assets: map[string]timeline.Asset{
		"ast_bbbbbbbbbb": {ID: "ast_bbbbbbbbbb", Name: "clip.mp4"},
		"ast_aaaaaaaaaa": {ID: "ast_aaaaaaaaaa", Name: "clip.mp4"},
		"ast_cccccccccc": {ID: "ast_cccccccccc", Name: "other.wav"},
	}}

	assets, unresolved, matched := matchOtioMedia(doc, []otio.Media{
		{Ref: "clip.mp4", Status: otio.Candidate},
		{Ref: `C:\footage\other.wav`, Status: otio.Candidate},
		{Ref: "missing.mov", Status: otio.Candidate},
		{Ref: "https://example.com/a.mp4", Status: otio.Unresolved, Reason: "a URL"},
	})

	if assets["clip.mp4"] != "ast_aaaaaaaaaa" || assets[`C:\footage\other.wav`] != "ast_cccccccccc" ||
		len(assets) != 2 {
		t.Errorf("assets = %v", assets)
	}

	if len(matched) != 2 || matched[0].asset != "ast_aaaaaaaaaa" || matched[1].asset != "ast_cccccccccc" {
		t.Errorf("matched = %+v", matched)
	}

	if len(unresolved) != 2 || unresolved[0].Ref != "missing.mov" || unresolved[0].Reason == "" ||
		unresolved[1].Ref != "https://example.com/a.mp4" {
		t.Errorf("unresolved = %+v", unresolved)
	}
}

func TestOtioFileNameIsSafe(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Launch teaser": "Launch_teaser.otio",
		"../../etc/x":   "etc_x.otio",
		"":              "project.otio",
		"///":           "project.otio",
		"a\r\nb":        "a__b.otio",
	} {
		if got := otioFileName(in); got != want {
			t.Errorf("otioFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
