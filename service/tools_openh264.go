package service

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
)

// openH264Manifest is the manifest that pins the Cisco library: the one of the install step, or the
// embedded one when the step was built without it.
func (t *Tools) openH264Manifest() ffmpeg.OpenH264Manifest {
	if len(t.OpenH264.Entries) > 0 {
		return t.OpenH264
	}

	m, err := ffmpeg.DefaultOpenH264Manifest()
	if err != nil {
		return ffmpeg.OpenH264Manifest{}
	}

	return m
}

// noEncoder is E_NO_ENCODER for the library: the reason and the tool, the same keys the engine gives.
func noEncoder(reason, message, hint string) *Error {
	return newError(ffmpeg.CodeNoEncoder, "%s", message).withHint("%s", hint).
		withDetail("reason", reason).withDetail("tool", ffmpeg.OpenH264Tool)
}

// installOpenH264 is the openh264 branch of ToolsInstall (SDD-007 section 6.8): it announces the
// download of the library the own FFmpeg build loads and starts it in the background through
// ffmpeg.InstallOpenH264, which verifies it before anything is kept. A refusal is E_NO_ENCODER with
// details.reason. The library is read when the server starts, so a running server needs a restart.
func (s *svc) installOpenH264(ctx context.Context, c Caller) (*ToolsInstallResult, error) {
	goos, goarch := s.tools.platform()
	manifest := s.tools.openH264Manifest()

	entry, ok := manifest.For(goos, goarch)
	if !ok {
		return nil, noEncoder(ffmpeg.ReasonOpenH264Platform,
			fmt.Sprintf("there is no OpenH264 library for %s/%s.", goos, goarch),
			"use the hardware or operating system encoder of this machine")
	}

	if !entry.Published {
		return nil, noEncoder(ffmpeg.ReasonNotPublished, "the OpenH264 library for this platform is not published yet.",
			"use the hardware or operating system encoder of this machine")
	}

	u, perr := url.Parse(entry.URL)
	if perr != nil || u.Scheme != "https" || !slices.Contains(manifest.Hosts, u.Hostname()) {
		return nil, internalError()
	}

	res := &ToolsInstallResult{
		State: "running", Files: []ToolFile{{Name: entry.File, Bytes: entry.Bytes, Host: u.Hostname()}},
		DownloadBytes: entry.Bytes,
	}
	res.Text = fmt.Sprintf("installing %s %s: 1 file, %s from %s; restart the server to load it",
		ffmpeg.OpenH264Tool, entry.Version, sizesText(entry.Bytes), u.Hostname())

	if err := s.beginInstall(entry.Bytes); err != nil {
		return nil, err
	}

	bg := context.WithoutCancel(ctx)

	go func() {
		_, err := ffmpeg.InstallOpenH264(bg, ffmpeg.InstallOpenH264Options{
			ToolsDir: s.tools.Dir, Manifest: manifest, Client: s.tools.Client, GOOS: goos, GOARCH: goarch,
			Progress: func(done, _ int64) {
				s.toolsState.mu.Lock()
				s.toolsState.done = done
				s.toolsState.mu.Unlock()
			},
		})

		s.endInstall(fromEngine(err))
	}()

	s.auditInstall(ctx, c.Actor, ffmpeg.OpenH264Tool)

	return res, nil
}

// beginInstall claims the one install that may run at a time.
func (s *svc) beginInstall(total int64) *Error {
	s.toolsState.mu.Lock()
	defer s.toolsState.mu.Unlock()

	if s.toolsState.running {
		return newError(CodeQueueFull, "an install is already running.").
			withHint("read its progress with GET /api/v1/system/tools")
	}

	s.toolsState.running, s.toolsState.done, s.toolsState.total, s.toolsState.err = true, 0, total, nil

	return nil
}

// endInstall releases the claim and keeps the error of a failed install for ToolsStatus.
func (s *svc) endInstall(err *Error) {
	s.toolsState.mu.Lock()
	defer s.toolsState.mu.Unlock()

	s.toolsState.running = false

	if err != nil {
		s.toolsState.err = err
	}
}
