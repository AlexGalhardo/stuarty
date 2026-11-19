package servicetest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

// fakeEngineSource is a motion engine of the test: a small program with the shape of the command line of a real
// one (render, a size, a rate, a length, a colour, an output file). It runs on the agent's side, never inside the
// product, and draws a solid colour with a transparent left half through the pinned FFmpeg. SDD-011 section 15: the
// product runs no engine; the test plays the agent that does.
const fakeEngineSource = `package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "render" {
		fmt.Fprintln(os.Stderr, "usage: fakeengine render -width w -height h -fps n -frames n -color c [-alpha] -o file")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	width, height := fs.Int("width", 0, ""), fs.Int("height", 0, "")
	fps, frames := fs.Int("fps", 30, ""), fs.Int("frames", 0, "")
	colour := fs.String("color", "red", "")
	alpha := fs.Bool("alpha", false, "")
	out := fs.String("o", "", "")
	_ = fs.Parse(os.Args[2:])

	vf, codec, container := "format=yuv420p", []string{"-c:v", "mpeg4", "-q:v", "4"}, "mp4"
	if *alpha {
		vf = "format=yuva444p,geq=lum='lum(X,Y)':cb='cb(X,Y)':cr='cr(X,Y)':a='if(gte(X,W/2),255,0)',format=yuva444p10le"
		codec, container = []string{"-c:v", "prores_ks", "-profile:v", "4444", "-pix_fmt", "yuva444p10le"}, "mov"
	}
	args := []string{
		"-hide_banner", "-v", "error", "-y", "-f", "lavfi",
		"-i", fmt.Sprintf("color=c=%s:s=%dx%d:r=%d", *colour, *width, *height, *fps),
		"-frames:v", strconv.Itoa(*frames), "-vf", vf,
	}
	args = append(append(args, codec...), "-f", container, *out)
	if b, err := exec.Command(os.Getenv("FAKE_ENGINE_FFMPEG"), args...).CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, string(b))
		os.Exit(1)
	}
	fmt.Println("rendered", *out)
}
`

// buildFakeEngine compiles the engine of the test in a folder of its own, outside the repository, and returns the
// program and a function that runs it.
func buildFakeEngine(t *testing.T) func(args ...string) {
	t.Helper()

	ffdir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")
	if ffdir == "" {
		t.Skip("set STUARTY_TEST_FFMPEG_DIR: the fake engine renders with it") // verify:env-skip no pinned FFmpeg
	}

	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fakeengine\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(fakeEngineSource), 0o600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "fakeengine"+exe)

	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	build.Dir = dir

	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the fake engine: %v\n%s", err, out)
	}

	return func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), bin, append([]string{"render"}, args...)...)
		cmd.Env = append(os.Environ(), "FAKE_ENGINE_FFMPEG="+filepath.Join(ffdir, "ffmpeg"+exe))

		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fakeengine render %v: %v\n%s", args, err, out)
		}
	}
}

// docOf reads the document of a project.
func (m *mediaEnv) docOf(project string) *timeline.Document {
	m.t.Helper()

	doc, err := timeline.Decode(m.document(project))
	if err != nil {
		m.t.Fatal(err)
	}

	return doc
}

// A scripted agent hands a clip to an engine and brings it back (epic 33, SDD-011 section 15): it asks for the
// brief, renders to it on its own side, imports the file with its origin, checks it, places it, renders again,
// imports under the same render key, is offered the replacement and takes it as one undoable batch that keeps the
// keyframes of the clip. Nothing here runs an engine in the product: the test is the agent.
func TestRenderKeyScenarioWithAFakeEngine(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})
	render := buildFakeEngine(t)
	gen := m.motionGen()
	ctx, bg := m.as(ui), context.Background()
	out := t.TempDir()

	project := m.newProject("handoff")
	made := m.mustEdit(ui, project, 0,
		op("settings.set", `{"width":160,"height":90}`),
		op("track.add", `{"kind":"visual","name":"Lower"}`),
		op("track.add", `{"kind":"visual","name":"Upper"}`),
	)
	lower, upper := made.Created[0], made.Created[1]

	// The footage under the animation: 3 s at 160x90 on the lower track.
	footage, err := gen.Clip(bg, "footage.mp4", 160, 90, 30, 3)
	if err != nil {
		t.Fatal(err)
	}

	base := m.importOne(project, footage).Assets[0]
	m.mustEdit(ui, project, m.head(project).Revision, op("clip.add", fmt.Sprintf(
		`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"3s"}`, lower, base.AssetID)))

	// 1. The brief for 1 s of the upper track.
	brief, err := m.svc.MotionBrief(ctx, service.MotionBriefRequest{
		ProjectID: project, From: "1s", Until: "2s", TrackID: upper,
	})
	if err != nil || brief.Width != 160 || brief.Height != 90 || brief.DurationFrames != 30 || !brief.NeedsAlpha {
		t.Fatalf("brief = %+v, %v", brief, err)
	}

	renderTo := func(name, colour string, frames int, alpha bool, w, h int) string {
		t.Helper()

		args := []string{
			"-width", strconv.Itoa(w), "-height", strconv.Itoa(h), "-fps", strconv.Itoa(brief.FrameRate.Num),
			"-frames", strconv.Itoa(frames), "-color", colour, "-o", filepath.Join(out, name),
		}
		if alpha {
			args = append(args, "-alpha")
		}

		render(args...)

		return filepath.Join(out, name)
	}

	importAs := func(path, key string) service.ImportedAsset {
		t.Helper()

		res, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
			ProjectID: project, WaitMS: 20000, Files: []service.ImportFile{{Path: path}},
			Origin: &asset.Origin{
				Kind: asset.RenderedExternally, Source: "fakeengine project title-card", Engine: "fakeengine",
				EngineVersion: "1.0.0", RenderKey: key,
			},
		})
		if err != nil || res.Failed != 0 || res.Assets[0].State != "ready" {
			t.Fatalf("import of %s = %+v, %v", path, res, err)
		}

		return res.Assets[0]
	}

	// 2. The first render: the brief's size, rate and length, with alpha. It is imported with its origin and checked.
	first := importAs(
		renderTo("title-v1.mov", "red", brief.DurationFrames, true, brief.Width, brief.Height),
		"title-card",
	)
	if len(first.Replaces) != 0 {
		t.Errorf("the first render replaces nothing: %+v", first.Replaces)
	}

	check, err := m.svc.MotionCheck(ctx, service.MotionCheckRequest{
		ProjectID: project, AssetID: first.AssetID, From: "1s", Until: "2s", TrackID: upper,
	})
	if err != nil || !check.OK {
		t.Fatalf("the render fits the brief: %+v, %v", check, err)
	}

	// The library keeps the origin of the file.
	if got, err := m.svc.AssetGet(ctx, service.AssetRef{Hash: first.Hash}); err != nil ||
		got.Origin.Kind != asset.RenderedExternally || got.Origin.RenderKey != "title-card" ||
		got.Origin.Engine != "fakeengine" || got.Origin.EngineVersion != "1.0.0" {
		t.Errorf("origin = %+v, %v", got, err)
	}

	// A render at another size is reported with the size expected, and nothing is stretched.
	wrong := importAs(renderTo("title-wrong.mov", "green", brief.DurationFrames, true, 320, 180), "title-wrong")

	badCheck, err := m.svc.MotionCheck(ctx, service.MotionCheckRequest{
		ProjectID: project, AssetID: wrong.AssetID, From: "1s", Until: "2s", TrackID: upper,
	})
	if e, f, ok := mismatchOf(badCheck, "size"); err != nil || badCheck.OK || !ok || e != "160x90" || f != "320x180" {
		t.Errorf("a wrong size: %+v, %v", badCheck, err)
	}

	// 3. It is placed twice, and the first clip is given keyframes by the person.
	placed := m.mustEdit(ui, project, m.head(project).Revision,
		op("clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"1s","duration":"1s"}`, upper, first.AssetID)),
		op("clip.add", fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"2s","duration":"1s"}`, upper, first.AssetID)))
	clipID, againID := placed.Created[0], placed.Created[1]
	both := []string{clipID, againID}
	slices.Sort(both)

	m.mustEdit(ui, project, m.head(project).Revision,
		op("keyframe.set", fmt.Sprintf(`{"item_id":%q,"property":"opacity","at":"1s","value":1}`, clipID)),
		op("keyframe.set", fmt.Sprintf(`{"item_id":%q,"property":"opacity","at":"1.5s","value":0.4}`, clipID)))

	before, beforeAgain := m.docOf(project).Items[clipID], m.docOf(project).Items[againID]
	if len(before.Keyframes["opacity"]) < 2 {
		t.Fatalf("the clip has no keyframes: %+v", before.Keyframes)
	}

	// 4. The animation is rendered again and imported under the same key: the import offers the replacement.
	second := importAs(
		renderTo("title-v2.mov", "blue", brief.DurationFrames, true, brief.Width, brief.Height),
		"title-card",
	)
	if len(second.Replaces) != 1 || second.Replaces[0].AssetID != first.AssetID ||
		!reflect.DeepEqual(second.Replaces[0].ItemIDs, both) {
		t.Fatalf("the offer = %+v", second.Replaces)
	}

	// A render too short for the clip refuses the whole batch and names what fits; nothing changes.
	short := importAs(renderTo("title-short.mov", "white", 15, true, brief.Width, brief.Height), "title-card")
	rev := m.head(project).Revision

	_, err = m.svc.MotionReplace(ctx, service.MotionReplaceRequest{ProjectID: project, AssetID: short.AssetID})
	if se := serviceError(t, err); se.Code != "E_OUT_OF_SOURCE" || m.head(project).Revision != rev {
		t.Errorf("a render too short: %v (revision %d, was %d)", err, m.head(project).Revision, rev)
	}

	// A dry run says what would change and changes nothing.
	dry, err := m.svc.MotionReplace(ctx, service.MotionReplaceRequest{
		ProjectID: project, AssetID: second.AssetID, DryRun: true,
	})
	if err != nil || dry.Applied || !reflect.DeepEqual(dry.Replaced, both) ||
		m.head(project).Revision != rev {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}

	// 5. The replacement: every clip of the key in one batch, one revision, the keyframes and everything else kept.
	done, err := m.svc.MotionReplace(ctx, service.MotionReplaceRequest{ProjectID: project, AssetID: second.AssetID})
	if err != nil || !done.Applied || done.Revision != rev+1 || done.RenderKey != "title-card" ||
		!reflect.DeepEqual(done.Replaced, both) || len(done.Effective) != 2 {
		t.Fatalf("replacement = %+v, %v", done, err)
	}

	after := m.docOf(project)
	for id, want := range map[string]timeline.Item{clipID: before, againID: beforeAgain} {
		if got := after.Items[id]; got.Asset != second.AssetID {
			t.Fatalf("clip %s reads %s, want %s", id, got.Asset, second.AssetID)
		} else {
			got.Asset = want.Asset
			if !reflect.DeepEqual(got, want) {
				t.Errorf("clip %s changed beyond its asset:\n got %+v\nwant %+v", id, got, want)
			}
		}
	}

	if _, kept := after.Assets[first.AssetID]; !kept {
		t.Error("the earlier render left the document")
	}

	// Asking again finds nothing to replace; the clips already read the new render.
	_, err = m.svc.MotionReplace(ctx, service.MotionReplaceRequest{ProjectID: project, AssetID: second.AssetID})
	if code(err) != "E_NOT_FOUND" {
		t.Errorf("a second replacement: %v", err)
	}

	// 6. One history entry, and one undo restores both clips.
	hist, err := m.svc.HistoryList(ctx, service.HistoryListRequest{ProjectID: project})
	if err != nil || len(hist.Entries) == 0 ||
		!reflect.DeepEqual(hist.Entries[0].Ops, []string{"clip.replace_media", "clip.replace_media"}) ||
		hist.Entries[0].Revision != done.Revision {
		t.Fatalf("the history = %+v, %v", hist, err)
	}

	if _, err := m.svc.Undo(ctx, service.UndoRequest{ProjectID: project}); err != nil {
		t.Fatal(err)
	}

	undone := m.docOf(project)
	if got := undone.Items[clipID]; !reflect.DeepEqual(got, before) {
		t.Errorf("one undo did not restore the clip:\n got %+v\nwant %+v", got, before)
	}

	if got := undone.Items[againID]; !reflect.DeepEqual(got, beforeAgain) {
		t.Errorf("one undo did not restore the second clip:\n got %+v\nwant %+v", got, beforeAgain)
	}
}
