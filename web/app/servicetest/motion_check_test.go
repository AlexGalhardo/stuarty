package servicetest

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/engine/ingest/mediagen"
	"github.com/AlexGalhardo/stuarty/service"
)

// motionGen makes the fixtures of the motion tests with the pinned FFmpeg.
func (m *mediaEnv) motionGen() mediagen.Gen {
	m.t.Helper()

	inst := pinnedFFmpeg(m.t)

	return mediagen.Gen{
		Dir: m.files,
		Run: func(ctx context.Context, dir string, args ...string) (string, error) {
			res, err := inst.FFmpeg.Run(ctx, dir, args...)

			return res.Stderr, err
		},
	}
}

// motionFit is a project of 160x90 at 30 fps with two visual tracks and 2 s of text on the lower one.
func (m *mediaEnv) motionFit() (project, upper, lower string) {
	m.t.Helper()

	project = m.newProject("motion check")
	res := m.mustEdit(ui, project, 0,
		op("settings.set", `{"width":160,"height":90}`),
		op("track.add", `{"kind":"visual","name":"Lower"}`),
		op("track.add", `{"kind":"visual","name":"Upper"}`),
	)
	lower, upper = res.Created[0], res.Created[1]

	m.mustEdit(ui, project, 1,
		op("text.add", `{"track_id":"`+lower+`","start":"0s","duration":"2s","text":"base"}`))

	return project, upper, lower
}

func mismatchOf(res *service.MotionCheckResult, field string) (expected, found string, ok bool) {
	for _, mm := range res.Mismatches {
		if mm.Field == field {
			return mm.Expected, mm.Found, true
		}
	}

	return "", "", false
}

// SDD-011 section 15.3: the files a render by another tool may come back as are imported like any media and
// checked against the brief; each mismatch has the value expected and the value found.
func TestMotionCheckReportsEachMismatchWithTheValueExpected(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})
	ctx := m.as(ui)
	gen := m.motionGen()
	project, upper, lower := m.motionFit()

	imp := func(name string, in mediagen.AlphaInput, w, h, fps int, secs float64) string {
		t.Helper()

		p, err := gen.AlphaEdge(context.Background(), in, name, w, h, fps, secs, 3)
		if err != nil {
			t.Fatal(err)
		}

		res := m.importOne(project, p)
		if res.Failed != 0 || res.Assets[0].State != "ready" {
			t.Fatalf("import of %s = %+v", name, res)
		}

		return res.Assets[0].AssetID
	}

	check := func(asset, track string) *service.MotionCheckResult {
		t.Helper()

		res, err := m.svc.MotionCheck(ctx, service.MotionCheckRequest{
			ProjectID: project, AssetID: asset, Until: "1s", TrackID: track,
		})
		if err != nil {
			t.Fatal(err)
		}

		return res
	}

	// A file that fits: the size, the rate and the length of the brief, with alpha, for a clip over another track.
	fits := imp("fits.mov", mediagen.AlphaProRes, 160, 90, 30, 1)
	if res := check(fits, upper); !res.OK || len(res.Mismatches) != 0 || !res.NeedsAlpha {
		t.Fatalf("a file that fits: %+v", res)
	}

	// The wrong rate and the wrong length: 48 frames at 24 fps last 60 frames at 30 fps.
	slow := imp("slow.mov", mediagen.AlphaProRes, 160, 90, 24, 2)
	res := check(slow, upper)

	if e, f, ok := mismatchOf(res, "frame_rate"); !ok || e != "30/1" || f != "24/1" {
		t.Errorf("frame rate: %q %q %v", e, f, ok)
	}

	if e, f, ok := mismatchOf(res, "duration"); !ok || e != "30f" || f != "60f" {
		t.Errorf("duration: %q %q %v", e, f, ok)
	}

	if res.OK || len(res.Mismatches) != 2 {
		t.Errorf("only the rate and the length are wrong: %+v", res.Mismatches)
	}

	// The wrong size.
	big := imp("big.mov", mediagen.AlphaProRes, 320, 180, 30, 1)
	if e, f, ok := mismatchOf(check(big, upper), "size"); !ok || e != "160x90" || f != "320x180" {
		t.Errorf("size: %q %q %v", e, f, ok)
	}

	// No alpha where the clip goes over another track, and none needed on the lowest track.
	plain, err := gen.Clip(context.Background(), "plain.mp4", 160, 90, 30, 1)
	if err != nil {
		t.Fatal(err)
	}

	opaque := m.importOne(project, plain).Assets[0].AssetID
	if e, f, ok := mismatchOf(check(opaque, upper), "alpha"); !ok || e != "alpha" || f != "no alpha" {
		t.Errorf("alpha: %q %q %v", e, f, ok)
	}

	if res := check(opaque, lower); !res.OK || res.NeedsAlpha {
		t.Errorf("an opaque clip on the lowest track: %+v", res)
	}

	// The text names the mismatch and the asset name is delimited as untrusted.
	if !strings.Contains(check(slow, upper).Text, "frame_rate: expected 30/1, found 24/1") ||
		!strings.Contains(res.Text, `<untrusted_media_text source="asset_name"`) {
		t.Errorf("text:\n%s", res.Text)
	}
}

func TestMotionCheckRefusals(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	ctx := m.as(ui)
	project, _, _ := m.motionFit()

	for name, c := range map[string]struct {
		req  service.MotionCheckRequest
		code string
	}{
		"not an asset id":     {service.MotionCheckRequest{ProjectID: project, AssetID: "logo"}, "E_VALIDATION"},
		"no asset id":         {service.MotionCheckRequest{ProjectID: project}, "E_VALIDATION"},
		"an asset not in it":  {service.MotionCheckRequest{ProjectID: project, AssetID: "ast_zzzzzzzzzz"}, "E_NOT_FOUND"},
		"an unknown project":  {service.MotionCheckRequest{ProjectID: "prj_unknown001", AssetID: "ast_zzzzzzzzzz"}, "E_NOT_FOUND"},
		"a reversed range":    {service.MotionCheckRequest{ProjectID: project, AssetID: "ast_zzzzzzzzzz", From: "2s", Until: "1s"}, "E_INVALID_TIME"},
		"an unknown track id": {service.MotionCheckRequest{ProjectID: project, AssetID: "ast_zzzzzzzzzz", Until: "1s", TrackID: "trk_zzzzzzzzzz"}, "E_NOT_FOUND"},
		"a range of too long": {service.MotionCheckRequest{ProjectID: project, AssetID: "ast_zzzzzzzzzz", Until: "121s"}, "E_LIMIT_EXCEEDED"},
		"a time that is not":  {service.MotionCheckRequest{ProjectID: project, AssetID: "ast_zzzzzzzzzz", From: "soon"}, "E_INVALID_TIME"},
	} {
		if _, err := m.svc.MotionCheck(ctx, c.req); code(err) != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
}
