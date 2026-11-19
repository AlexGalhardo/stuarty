package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TestExportPresetListGivesTheFivePlacements is acceptance criterion 1: ExportPresetList returns the presets
// of the five placements, each with its limits, its source and the date it was read.
func TestExportPresetListGivesTheFivePlacements(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{}, nil)

	res, err := env.s.ExportPresetList(renderCtx(), ExportPresetListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, p := range res.Presets {
		ids = append(ids, p.ID)

		if len(p.Sources) == 0 || p.Sources[0].URL == "" || p.Sources[0].Read == "" || p.Sources[0].Says == "" {
			t.Errorf("%s: no source: %+v", p.ID, p.Sources)
		}

		if p.Container != "mp4" || len(p.Codecs) == 0 || p.Targets.VideoKbps == 0 {
			t.Errorf("%s: %+v", p.ID, p)
		}
	}

	if want := []string{
		"youtube-shorts",
		"instagram-reels",
		"facebook-reels",
		"tiktok",
		"x-video",
	}; !slices.Equal(
		ids,
		want,
	) {
		t.Fatalf("ids %v, want %v", ids, want)
	}

	fb := res.Presets[2]
	if fb.Limits.MaxDurationMS != 90_000 || fb.Limits.MinDurationMS != 3_000 || fb.Limits.AspectRatio != "9:16" ||
		fb.Limits.MinWidth != 540 {
		t.Errorf("facebook limits %+v", fb.Limits)
	}

	if res.Codecs != nil {
		t.Errorf("codecs were not asked for: %+v", res.Codecs)
	}

	if !strings.Contains(res.Text, "facebook-reels") || !strings.Contains(res.Text, "read 2026-11-19") {
		t.Errorf("text:\n%s", res.Text)
	}

	// The list needs the read scope and nothing more.
	if _, err := env.s.ExportPresetList(context.Background(), ExportPresetListRequest{}); err == nil {
		t.Error("a call with no credentials was answered")
	}
}

// TestExportPresetListReportsTheCodecsOfTheMachine: with Codecs the list carries what the exporter says.
func TestExportPresetListReportsTheCodecsOfTheMachine(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{}, nil)
	env.s.exporter = codecReporter{env.s.exporter, []CodecAvailability{
		{Codec: "h264", Available: true, Encoder: "libopenh264"},
		{Codec: "hevc", Reason: "hevc_nvenc failed"},
	}}

	res, err := env.s.ExportPresetList(renderCtx(), ExportPresetListRequest{Codecs: true})
	if err != nil || len(res.Codecs) != 2 || !strings.Contains(res.Text, "codec hevc: not available") {
		t.Fatalf("%v %+v\n%s", err, res.Codecs, res.Text)
	}
}

type codecReporter struct {
	Exporter
	codecs []CodecAvailability
}

func (c codecReporter) Codecs(context.Context) []CodecAvailability { return c.codecs }

// TestExportCheckReportsWhatDoesNotFitBeforeEncoding is acceptance criterion 2: a timeline that is too long for a
// preset is reported with the exact limit, from the document alone: the exporter is never called.
func TestExportCheckReportsWhatDoesNotFitBeforeEncoding(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 100, Limits{}, nil)
	ctx := renderCtx()

	res, err := env.s.ExportCheck(ctx, ExportCheckRequest{ProjectID: batchProject, Preset: "facebook-reels"})
	if err != nil {
		t.Fatal(err)
	}

	if res.OK || len(res.Findings) != 1 || res.Errors != 1 || res.Revision != 7 {
		t.Fatalf("%+v", res)
	}

	f := res.Findings[0]
	if f.Code != "X_DURATION_TOO_LONG" || f.Limit != "90 s" || f.Got != "100 s" || f.Severity != "error" {
		t.Errorf("finding %+v", f)
	}

	if !strings.Contains(res.Text, "X_DURATION_TOO_LONG") || res.DurationMS != 100_000 {
		t.Errorf("%d ms\n%s", res.DurationMS, res.Text)
	}

	// The same timeline fits TikTok; a range fits Facebook.
	if ok, err := env.s.ExportCheck(
		ctx,
		ExportCheckRequest{ProjectID: batchProject, Preset: "tiktok"},
	); err != nil ||
		!ok.OK {
		t.Errorf("tiktok: %v %+v", err, ok)
	}

	short, err := env.s.ExportCheck(ctx, ExportCheckRequest{
		ProjectID: batchProject, Preset: "facebook-reels", Start: "10s", End: "40s",
	})
	if err != nil || !short.OK || short.DurationMS != 30_000 || !strings.Contains(short.Text, "ok: the project fits") {
		t.Errorf("a range of 30 s: %v %+v", err, short)
	}

	if env.calls.Load() != 0 {
		t.Error("the check encoded something")
	}
}

// TestExportCheckRefusesWhatItCannotCheck: an unknown preset or codec, a bad range, a missing scope and an
// unknown project.
func TestExportCheckRefusesWhatItCannotCheck(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 10, Limits{}, nil)
	ctx := renderCtx()

	cases := []struct {
		name string
		req  ExportCheckRequest
		path string
	}{
		{"unknown preset", ExportCheckRequest{ProjectID: batchProject, Preset: "myspace"}, "args.preset"},
		{"no preset", ExportCheckRequest{ProjectID: batchProject}, "args.preset"},
		{
			"unknown codec",
			ExportCheckRequest{ProjectID: batchProject, Preset: "tiktok", Codec: "libx265"},
			"args.codec",
		},
		{"range outside", ExportCheckRequest{ProjectID: batchProject, Preset: "tiktok", End: "1000s"}, "args"},
	}

	for _, c := range cases {
		_, err := env.s.ExportCheck(ctx, c.req)

		var se *Error
		if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != c.path {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	if _, err := env.s.ExportCheck(ctx, ExportCheckRequest{ProjectID: "prj_0000000000", Preset: "tiktok"}); err == nil {
		t.Error("an unknown project was checked")
	}

	none := WithCaller(context.Background(), Caller{Actor: "local:ui", Scopes: []string{ScopeEdit}})
	if _, err := env.s.ExportCheck(none, ExportCheckRequest{ProjectID: batchProject, Preset: "tiktok"}); err == nil {
		t.Error("a caller without the read scope checked a project")
	}
}

func startExport(env *batchEnv, args string) (*Job, error) {
	return env.s.JobStart(renderCtx(), JobStartRequest{
		ProjectID: batchProject, Kind: JobExport, Args: json.RawMessage(args), DryRun: true,
	})
}

// TestExportStartWithAPreset: an export with a preset is checked at the start with the same function, and a
// project that does not fit is E_VALIDATION with the findings, before a job exists.
func TestExportStartWithAPreset(t *testing.T) {
	t.Parallel()

	env := newBatchEnv(t, 100, Limits{}, nil)

	if _, err := startExport(env, `{"preset":"tiktok","codec":"hevc"}`); err != nil {
		t.Fatalf("a project that fits: %v", err)
	}

	_, err := startExport(env, `{"preset":"facebook-reels"}`)

	var se *Error
	if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != "args.preset" ||
		se.Details["reason"] != "preset_mismatch" || se.Details["preset"] != "facebook-reels" ||
		!strings.Contains(se.Message, "90 s") || se.Hint == "" {
		t.Fatalf("a project that does not fit: %+v", err)
	}

	if _, err := startExport(env, `{"preset":"facebook-reels","start":"0s","end":"60s"}`); err != nil {
		t.Errorf("a range that fits: %v", err)
	}

	for name, args := range map[string]string{
		"unknown preset": `{"preset":"myspace"}`,
		"preset on gif":  `{"preset":"tiktok","format":"gif"}`,
		"codec on wav":   `{"codec":"av1","format":"aac"}`,
		"unknown codec":  `{"codec":"libx265"}`,
		"preset not str": `{"preset":3}`,
	} {
		if _, err := startExport(env, args); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
