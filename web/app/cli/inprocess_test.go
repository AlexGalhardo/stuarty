package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

// exeName is the file name of a tool on this operating system.
func exeName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}

	return tool
}

// pinnedFFmpegDir is the directory of the pinned FFmpeg build, or "" when there
// is none.
func pinnedFFmpegDir() string {
	exe := exeName("ffmpeg")

	dir := os.Getenv("STUARTY_TEST_FFMPEG_DIR")

	if dir == "" {
		for _, base := range []string{os.Getenv("RUNNER_TEMP"), os.Getenv("TEMP"), os.Getenv("TMPDIR")} {
			if m, _ := filepath.Glob(filepath.Join(base, "ffmpeg", "*", "*", "bin", exe)); base != "" && len(m) > 0 {
				dir = filepath.Dir(m[len(m)-1])

				break
			}
		}
	}

	return dir
}

func pinnedFFmpegPath(t *testing.T) string {
	t.Helper()

	dir := pinnedFFmpegDir()
	if dir == "" {
		t.Skip( // verify:env-skip no pinned FFmpeg on this machine
			"no pinned FFmpeg: run scripts/install-ffmpeg.sh and set STUARTY_TEST_FFMPEG_DIR",
		)
	}

	return filepath.Join(dir, exeName("ffmpeg"))
}

// With no server the CLI owns the workspace and exports inside the call: the
// in-process Service has the media, the exporter and the queue of serve.
func TestInProcessConnectionCanExport(t *testing.T) {
	ff := pinnedFFmpegPath(t)
	home := filepath.Join(t.TempDir(), "ws")

	conn, err := cli.LocalResolver{Getenv: func(k string) string {
		switch k {
		case workspace.EnvHome:
			return home
		case "STUARTY_FFMPEG":
			return ff
		case "STUARTY_MIN_FREE_BYTES":
			return "1" // not a test of the disk guard: it must not depend on the free space of the machine
		}

		return ""
	}}.Resolve(context.Background(), cli.Target{Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = conn.Close() }()

	if !conn.InProcess {
		t.Fatal("expected the in-process connection")
	}

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "cli", Scopes: service.AllScopes})

	p, err := conn.Service.ProjectCreate(ctx, service.ProjectCreateRequest{Name: "export"})
	if err != nil {
		t.Fatal(err)
	}

	op := func(name, args string) command.Operation { return command.Operation{Op: name, Args: []byte(args)} }

	tr, err := conn.Service.Edit(ctx, service.EditRequest{
		ProjectID: p.ProjectID, Operations: []command.Operation{op("track.add", `{"kind":"visual","name":"V"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = conn.Service.Edit(ctx, service.EditRequest{
		ProjectID: p.ProjectID, BaseRevision: tr.Revision,
		Operations: []command.Operation{
			op("text.add", `{"track_id":"`+tr.Created[0]+`","start":"0s","duration":"1s","text":"hi"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}

	job, err := conn.Service.JobStart(
		ctx,
		service.JobStartRequest{ProjectID: p.ProjectID, Kind: "export", WaitMS: 1000},
	)
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
		if job.Error != nil {
			t.Logf("error details: %+v", job.Error.Details) // the failed checks of a verification
		}

		t.Fatalf("job = %+v", job)
	}
}
