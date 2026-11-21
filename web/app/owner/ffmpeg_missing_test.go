package owner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// TestMain lets a copy of this test binary, named ffmpeg, answer -version.
// Discovery then accepts the binary and fails on the ffprobe that is not next to it.
func TestMain(m *testing.M) {
	if actingAsFFmpeg() {
		return
	}

	os.Exit(m.Run())
}

func actingAsFFmpeg() bool {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	if base != "ffmpeg" {
		return false
	}

	if slices.Contains(os.Args[1:], "-version") {
		fmt.Printf(
			"%s version n9.0.2-fake Copyright (c) the FFmpeg developers\nconfiguration: --enable-version3 --disable-libx264\n",
			base,
		)

		return true
	}

	os.Exit(1)

	return true
}

// installFFmpegWithoutProbe copies this test binary as ffmpeg and leaves ffprobe absent.
func installFFmpegWithoutProbe(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	dst := filepath.Join(dir, name)
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		_ = src.Close()
		t.Fatal(err)
	}

	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	_ = src.Close()

	if copyErr != nil {
		t.Fatal(copyErr)
	}

	if closeErr != nil {
		t.Fatal(closeErr)
	}

	return dst
}

// panicTrap records a recovered job panic. The service logs "job panicked" and
// stores E_INTERNAL; the test must see neither.
type panicTrap struct {
	mu       sync.Mutex
	panicked bool
}

func (h *panicTrap) Enabled(context.Context, slog.Level) bool { return true }

func (h *panicTrap) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "job panicked" {
		h.mu.Lock()
		h.panicked = true
		h.mu.Unlock()
	}

	return nil
}

func (h *panicTrap) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *panicTrap) WithGroup(string) slog.Handler { return h }

func (h *panicTrap) sawPanic() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.panicked
}

// A configured ffmpeg with no ffprobe next to it must not stop the editor, and an
// ingest must answer E_FFMPEG_MISSING instead of panicking into E_INTERNAL (VV-226).
func TestProjectCreateAndIngestWhenFFprobeIsAbsent(t *testing.T) {
	ff := installFFmpegWithoutProbe(t)
	home := filepath.Join(t.TempDir(), "ws")
	clipDir := t.TempDir()

	clip := filepath.Join(clipDir, "clip.mp4")
	if err := os.WriteFile(clip, []byte("not a media file, but long enough to import"), 0o644); err != nil {
		t.Fatal(err)
	}

	logs := &panicTrap{}
	getenv := envOf(map[string]string{
		workspace.EnvHome: home,
		"STUARTY_FFMPEG":  ff,
		// Not a test of the disk guard: it must not depend on the free space of the machine.
		"STUARTY_MIN_FREE_BYTES": "1",
	})

	ow, err := Open(context.Background(), Config{
		Getenv: getenv, Inline: true, Log: slog.New(logs),
	})
	if err != nil {
		t.Fatalf("Open = %v, want the editor to start without ffprobe", err)
	}

	t.Cleanup(func() { _ = ow.Close() })

	ctx := service.WithCaller(context.Background(), service.Caller{Actor: "local:cli", Scopes: service.AllScopes})

	created, err := testproject.CreateWithTimeline(ctx, ow.Service, "spot")
	if se, ok := errors.AsType[*service.Error](err); ok && se.Code == command.CodeInternal {
		t.Fatalf("ProjectCreate = %v, want success without FFmpeg", err)
	}

	if err != nil || created == nil || created.ProjectID == "" {
		t.Fatalf("ProjectCreate = %+v, %v", created, err)
	}

	_, err = ow.Service.JobStart(ctx, service.JobStartRequest{ProjectID: created.ProjectID, Kind: service.JobExport})
	if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != ffmpeg.CodeFFmpegMissing {
		t.Fatalf("export start = %v, want %s", err, ffmpeg.CodeFFmpegMissing)
	}

	res, err := ow.Service.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: created.ProjectID, Files: []service.ImportFile{{Path: clip}},
	})
	if logs.sawPanic() {
		t.Fatal("ingest job panicked")
	}

	got := importError(t, res, err)
	if got.Code != ffmpeg.CodeFFmpegMissing {
		t.Fatalf("ingest code = %s (%s), want %s", got.Code, got.Message, ffmpeg.CodeFFmpegMissing)
	}

	if got.Hint == "" || (!strings.Contains(got.Hint, "STUARTY_FFMPEG") && !strings.Contains(got.Hint, "Install")) {
		t.Fatalf("hint = %q, want the install or configure hint", got.Hint)
	}

	if strings.Contains(got.Message+got.Hint, clipDir) || strings.Contains(got.Message+got.Hint, filepath.Dir(ff)) {
		t.Fatalf("the error names a path: %s / %s", got.Message, got.Hint)
	}
}

func importError(t *testing.T, res *service.MediaImportResult, err error) *service.Error {
	t.Helper()

	if se, ok := errors.AsType[*service.Error](err); ok {
		return se
	}

	if err != nil {
		t.Fatalf("MediaImport = %v, want a service error", err)
	}

	if res == nil || len(res.Assets) != 1 || res.Assets[0].Error == nil {
		t.Fatalf("MediaImport = %+v, want the ingest error on the file", res)
	}

	return res.Assets[0].Error
}
