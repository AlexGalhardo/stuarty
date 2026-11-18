package gioapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/workspace"
	"github.com/AlexGalhardo/vukem-video/web/app/agentdocs"
	"github.com/AlexGalhardo/vukem-video/web/app/owner"
)

// Session is one local workspace opened in this process. The desktop window is
// the owner (ADR-0008): it does not start a second server, and every edit goes
// through service.Service.
type Session struct {
	mu         sync.Mutex
	own        *owner.Owner
	runner     *ffmpeg.Runner
	artifacts  string
	ffmpegPath string
}

// Open locks the workspace and builds the service. An empty dir uses the
// default workspace (VUKEM_VIDEO_HOME, else the user default). The desktop
// editor always runs in local mode: it has no login screen.
func Open(dir string) (*Session, error) {
	own, err := owner.Open(context.Background(), owner.Config{
		Getenv:  getenv(dir),
		Inline:  true,
		Version: agentdocs.Version,
	})
	if err != nil {
		return nil, err
	}

	return &Session{own: own}, nil
}

func getenv(dir string) func(string) string {
	return func(k string) string {
		switch k {
		case workspace.EnvHome:
			if dir != "" {
				return dir
			}
		case "VUKEM_VIDEO_MODE":
			return "local"
		}

		return os.Getenv(k)
	}
}

// Close releases the workspace lock.
func (s *Session) Close() error {
	if s == nil || s.own == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Close()
}

// Engine is the FFmpeg runner, the artifacts directory and the ffmpeg binary.
// The desktop preview uses them to keep a decoder open across frames. The
// result is cached for the life of the session.
func (s *Session) Engine(ctx context.Context) (*ffmpeg.Runner, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runner != nil {
		return s.runner, s.artifacts, s.ffmpegPath, nil
	}

	if s.own == nil {
		return nil, "", "", errors.New("the workspace is not open")
	}

	dir := string(s.own.Dir)
	in, err := ffmpeg.Discover(ctx, ffmpeg.Config{})
	if err != nil {
		return nil, "", "", err
	}

	runner, err := ffmpeg.NewRunner(in, dir)
	if err != nil {
		return nil, "", "", err
	}

	artifacts := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		return nil, "", "", err
	}

	s.runner, s.artifacts, s.ffmpegPath = runner, artifacts, in.FFmpeg.Path()

	return s.runner, s.artifacts, s.ffmpegPath, nil
}

func (s *Session) call(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	scopes := append([]string(nil), service.AllScopes...)

	return service.WithCaller(ctx, service.Caller{Actor: "local:desktop", Scopes: scopes})
}

// Projects lists active projects.
func (s *Session) Projects(ctx context.Context) ([]service.ProjectSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.own.Service.ProjectList(s.call(ctx), service.ProjectListRequest{Limit: 50})
	if err != nil {
		return nil, err
	}

	return res.Projects, nil
}

// Create makes an empty project. preset is 16:9, 9:16, 1:1 or 4:5.
func (s *Session) Create(ctx context.Context, name, preset string) (*service.ProjectCreateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Service.ProjectCreate(s.call(ctx), service.ProjectCreateRequest{Name: name, Preset: preset, FPS: "30"})
}

// Project reads the document of one project.
func (s *Session) Project(ctx context.Context, id string) (*service.ProjectGetResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Service.ProjectGet(s.call(ctx), service.ProjectGetRequest{ProjectID: id, Detail: "document"})
}

// Media lists the assets of a project, including ones still ingesting.
func (s *Session) Media(ctx context.Context, id string) ([]service.MediaEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.own.Service.MediaList(s.call(ctx), service.MediaListRequest{
		ProjectID: id, Limit: 100,
	})
	if err != nil {
		return nil, err
	}

	return res.Assets, nil
}

// Edit applies one batch. The revision is the one the window last read.
func (s *Session) Edit(
	ctx context.Context,
	id string,
	rev int64,
	ops ...command.Operation,
) (*service.EditResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, err := idempotencyKey()
	if err != nil {
		return nil, err
	}

	return s.own.Service.Edit(s.call(ctx), service.EditRequest{
		ProjectID: id, BaseRevision: rev, Operations: ops, IdempotencyKey: key,
	})
}

// Undo reverts the caller's latest step.
func (s *Session) Undo(ctx context.Context, id string) (*service.UndoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Service.Undo(s.call(ctx), service.UndoRequest{ProjectID: id})
}

// Redo reapplies the caller's latest undone step.
func (s *Session) Redo(ctx context.Context, id string) (*service.UndoResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Service.Redo(s.call(ctx), service.UndoRequest{ProjectID: id})
}

// Import copies files into the project and waits up to 20s for ingest.
func (s *Session) Import(ctx context.Context, id string, paths []string) (*service.MediaImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	files := make([]service.ImportFile, len(paths))
	for i, p := range paths {
		files[i] = service.ImportFile{Path: p}
	}

	return s.own.Service.MediaImport(s.call(ctx), service.MediaImportRequest{
		ProjectID: id, Files: files, WaitMS: 20000,
	})
}

// Frame renders one JPEG of the current revision through the export path.
func (s *Session) Frame(ctx context.Context, id string, frame int) (img []byte, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("preview is unavailable: %v", rec)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.own.Service.ViewFrame(s.call(ctx), service.ViewFrameRequest{
		ProjectID: id, At: framesArg(frame), Width: 640,
	})
	if err != nil {
		return nil, err
	}

	return res.Image, nil
}

// Export starts an export job. format is mp4, gif, wav, aac or png.
func (s *Session) Export(ctx context.Context, id, format, at string) (*service.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	args := service.ExportArgs{Format: format}
	if format == "png" {
		args.At = at
	}

	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}

	return s.own.Service.JobStart(s.call(ctx), service.JobStartRequest{
		ProjectID: id, Kind: service.JobExport, Args: raw,
	})
}

// Job reads one job.
func (s *Session) Job(ctx context.Context, id string) (*service.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.own.Service.JobGet(s.call(ctx), service.JobGetRequest{JobID: id})
}

func operation(name string, args any) (command.Operation, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return command.Operation{}, err
	}

	return command.Operation{Op: name, Args: raw}, nil
}

func idempotencyKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return hex.EncodeToString(b[:]), nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}

	if se, ok := errors.AsType[*service.Error](err); ok {
		if se.Hint != "" {
			return se.Message + " " + se.Hint
		}

		return se.Message
	}

	return err.Error()
}
