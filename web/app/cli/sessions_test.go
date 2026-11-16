package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/cli"
)

// sessionRun is the CLI with no server, over a real workspace with the media
// and the exporter of serve: the commands of sessions A and B of SDD-003 section
// 5.6, one process-sized call at a time.
type sessionRun struct {
	t      *testing.T
	home   string
	files  string
	env    map[string]string
	actor  string
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// sessionFFmpeg finds an FFmpeg and an ffprobe: the pinned build of CI when
// there is one, else the ones on the PATH.
func sessionFFmpeg(t *testing.T) (ffmpeg, ffprobe string) {
	t.Helper()

	if dir := pinnedFFmpegDir(); dir != "" {
		ffmpeg = filepath.Join(dir, exeName("ffmpeg"))
		ffprobe = filepath.Join(dir, exeName("ffprobe"))
	}

	if ffmpeg == "" {
		var err error

		if ffmpeg, err = exec.LookPath("ffmpeg"); err != nil {
			t.Skip("no FFmpeg: sessions A and B import and export real media")
		}

		if ffprobe, err = exec.LookPath("ffprobe"); err != nil {
			t.Skip("no ffprobe: sessions A and B import and export real media")
		}
	}

	// Homebrew's GPL build on the macOS runner has no zscale, which the ingest
	// needs; the pinned LGPL build has it (risk R6).
	out, err := exec.Command(ffmpeg, "-hide_banner", "-filters").Output()
	if err != nil || !strings.Contains(string(out), "zscale") {
		t.Skip("this FFmpeg has no zscale filter: the ingest needs the pinned LGPL build")
	}

	return ffmpeg, ffprobe
}

func newSession(t *testing.T) *sessionRun {
	t.Helper()

	ffmpeg, ffprobe := sessionFFmpeg(t)
	root := t.TempDir()
	s := &sessionRun{t: t, home: filepath.Join(root, "ws"), files: filepath.Join(root, "files"), actor: "claude-code"}

	if err := os.MkdirAll(s.files, 0o700); err != nil {
		t.Fatal(err)
	}

	s.env = map[string]string{
		workspace.EnvHome: s.home, "VUKEM_VIDEO_FFMPEG": ffmpeg, "VUKEM_VIDEO_FFPROBE": ffprobe,
		"VUKEM_VIDEO_MIN_FREE_BYTES": "1",
	}

	return s
}

// clip makes a real 8 second video with sound, the stand-in of IMG_0421.MOV.
func (s *sessionRun) clip() string {
	s.t.Helper()

	ffmpeg := s.env["VUKEM_VIDEO_FFMPEG"]
	path := filepath.Join(s.files, "IMG_0421.mp4")
	args := []string{
		"-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x568:rate=30:duration=8",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=8", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", "-y", path,
	}

	ctx, cancel := context.WithTimeout(s.t.Context(), time.Minute)
	defer cancel()

	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		s.t.Fatalf("ffmpeg: %v\n%s", err, out)
	}

	return path
}

// do runs one command with --json; it returns the exit code and the one
// document on standard output (the result or the error).
func (s *sessionRun) do(stdin string, args ...string) (int, map[string]any) {
	s.t.Helper()

	getenv := func(k string) string { return s.env[k] }

	s.stdout.Reset()
	s.stderr.Reset()

	rt := &cli.Runtime{
		Stdin: strings.NewReader(stdin), Stdout: &s.stdout, Stderr: &s.stderr,
		Getenv: getenv, Getwd: os.Getwd, Resolver: cli.LocalResolver{Getenv: getenv},
	}

	code := rt.Execute(s.t.Context(), args[0], append(args[1:], "--json", "--actor", s.actor))

	if s.stderr.Len() != 0 {
		s.t.Fatalf("%v: standard error must be empty with --json: %q", args, s.stderr.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(s.stdout.Bytes(), &doc); err != nil || bytes.Count(s.stdout.Bytes(), []byte("\n")) != 1 {
		s.t.Fatalf("%v: standard output is not one JSON document: %v\n%s", args, err, s.stdout.String())
	}

	return code, doc
}

func (s *sessionRun) ok(stdin string, args ...string) map[string]any {
	s.t.Helper()

	code, doc := s.do(stdin, args...)
	if code != cli.ExitOK {
		s.t.Fatalf("%v: exit %d: %v", args, code, doc)
	}

	return doc
}

func str(t *testing.T, doc map[string]any, key string) string {
	t.Helper()

	v, _ := doc[key].(string)
	if v == "" {
		t.Fatalf("%q is missing in %v", key, doc)
	}

	return v
}

// teaser plays the first half of session A: the import, the tracks and the clip,
// the cut and the caption. It returns the project, at revision 4.
func (s *sessionRun) teaser() (project string) {
	s.t.Helper()

	created := s.ok("", "project:create", "--name", "Launch teaser")
	project = str(s.t, created, "project_id")

	imported := s.ok("", "media:import", s.clip(), "--project", project)
	assets, _ := imported["assets"].([]any)

	if imported["failed"] != float64(0) || len(assets) != 1 {
		s.t.Fatalf("media:import: %v", imported)
	}

	asset, _ := assets[0].(map[string]any)
	if asset["state"] != "ready" {
		s.t.Fatalf("the asset is not ready: %v", asset)
	}

	id := str(s.t, asset, "asset_id")

	s.stdout.Reset()

	tracks := s.ok(`[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_f6g7h8j9k0"}},
 {"op":"track.add","args":{"kind":"visual","name":"Captions","id":"trk_a1b2c3d4e5"}},
 {"op":"clip.add","args":{"track_id":"trk_f6g7h8j9k0","asset_id":"`+id+`","start":"0s","source_in":"2s","duration":"5s","id":"itm_m1n2p3q4r5"}}]`,
		"edit", "--project", project, "--ops", "-")
	if tracks["revision"] != float64(2) || tracks["actor"] != "local:claude-code" {
		s.t.Fatalf("tracks and clip: %v", tracks)
	}

	cut := s.ok(`[{"op":"item.split","args":{"item_id":"itm_m1n2p3q4r5","at":"2s","new_id":"itm_c3d4e5f6g7"}},
 {"op":"item.split","args":{"item_id":"itm_c3d4e5f6g7","at":"3s","new_id":"itm_d4e5f6g7h8"}},
 {"op":"item.delete","args":{"item_ids":["itm_c3d4e5f6g7"]}},
 {"op":"item.move","args":{"item_id":"itm_d4e5f6g7h8","start":"2s"}}]`,
		"edit", "--project", project, "--base", "2", "--ops", "-")
	if cut["revision"] != float64(3) || cut["base_revision"] != float64(2) {
		s.t.Fatalf("the cut: %v", cut)
	}

	caption := s.ok(`[{"op":"caption.add","args":{"track_id":"trk_a1b2c3d4e5","start":"0.4s",
  "linked_item":"itm_m1n2p3q4r5","id":"itm_s6t7v8w9x0","words":[{"text":"Meet","start":"0s","duration":"0.3s"},{"text":"the","start":"0.3s","duration":"0.2s"},{"text":"new","start":"0.5s","duration":"0.4s"},{"text":"editor.","start":"0.9s","duration":"0.7s"}]}}]`,
		"edit", "--project", project, "--base", "3", "--ops", "-")
	if caption["revision"] != float64(4) {
		s.t.Fatalf("the caption: %v", caption)
	}

	return project
}

// TestSessionAImportCutCaptionExport is session A of SDD-003 section 5.6: the
// Prototype scenario from the import to a verified export.
func TestSessionAImportCutCaptionExport(t *testing.T) {
	s := newSession(t)
	project := s.teaser()

	linted := s.ok("", "lint", "--project", project)
	if linted["errors"] != float64(0) || linted["revision"] != float64(4) {
		t.Fatalf("lint: %v", linted)
	}

	shown := s.ok("", "project:show", "--project", project)
	if text, _ := shown["text"].(string); !strings.Contains(text, "tracks=2 items=3 assets=1") {
		t.Fatalf("project:show: %v", shown["text"])
	}

	viewed := s.ok("", "view", "--project", project, "--at", "1s")
	if viewed["mime"] != "image/jpeg" || viewed["mode"] != "frame" {
		t.Fatalf("view: %v", viewed)
	}

	if _, err := os.Stat(str(t, viewed, "path")); err != nil {
		t.Fatalf("view saved no image: %v", err)
	}

	exported := s.ok("", "export", "--project", project, "--wait")
	if exported["state"] != "succeeded" || exported["kind"] != "export" {
		t.Fatalf("export: %v", exported)
	}

	result, _ := exported["result"].(map[string]any)
	file, _ := result["export"].(map[string]any)

	if file["revision"] != float64(4) {
		t.Fatalf("the export is of revision %v, want 4", file["revision"])
	}

	if _, err := os.Stat(str(t, file, "path")); err != nil {
		t.Fatalf("the export file is missing: %v", err)
	}
}

// TestSessionBBlockedExport is session B: a conflict (exit 2), the dry run, the
// commit with an idempotency key and its replay, and a permanent failure
// (exit 1) from the lint gate.
func TestSessionBBlockedExport(t *testing.T) {
	s := newSession(t)
	project := s.teaser()

	// move is the caption move of the session, planned against a base revision.
	move := func(base, start string, flags ...string) []string {
		args := []string{"edit", "--project", project, "--base", base}

		return append(append(args, flags...), "item.move", "item_id=itm_s6t7v8w9x0", "start="+start)
	}

	s.actor = "ui-user"

	if moved := s.ok("", move("4", "0.5s")...); moved["revision"] != float64(5) {
		t.Fatalf("the other actor: %v", moved)
	}

	s.actor = "claude-code"

	code, conflict := s.do("", move("4", "0.6s")...)
	if code != cli.ExitTransient || conflict["code"] != command.CodeRevisionConflict || conflict["retryable"] != true {
		t.Fatalf("a stale base: exit %d, %v", code, conflict)
	}

	dry := s.ok("", move("5", "0.6s", "--dry-run")...)
	if dry["applied"] != false || dry["dry_run"] != true || dry["revision"] != float64(6) {
		t.Fatalf("dry run: %v", dry)
	}

	commit := move("5", "0.6s", "--key", "move-caption-0001")

	if first := s.ok("", commit...); first["revision"] != float64(6) || first["replayed"] != false {
		t.Fatalf("the commit: %v", first)
	}

	if again := s.ok("", commit...); again["revision"] != float64(6) || again["replayed"] != true {
		t.Fatalf("the retry must be a replay: %v", again)
	}

	empty := s.ok("", "project:create", "--name", "Empty")

	code, refused := s.do("", "export", "--project", str(t, empty, "project_id"), "--wait")
	if code != cli.ExitFailure || refused["code"] != service.CodeLintBlocking || refused["retryable"] != false {
		t.Fatalf("an export of an empty project: exit %d, %v", code, refused)
	}
}
