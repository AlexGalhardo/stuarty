package service

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/aihost"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
)

// FacePaths are the three files of the face set, each verified against tools/installed.json just now.
type FacePaths struct {
	Helper, Runtime, Model string
}

// faceEntries are the entries of the face set for this platform.
func (t *Tools) faceEntries() ([]transcribe.ManifestEntry, bool) {
	goos, goarch := t.platform()

	return t.Face.Set(goos, goarch)
}

// FaceStack returns the verified paths of the helper, the runtime library and the model, or ok false when
// the set is not installed or a file no longer has its recorded digest (SDD-007 section 6.11 item 2). It
// never downloads anything.
func (t *Tools) FaceStack() (p FacePaths, ok bool) {
	if t == nil {
		return FacePaths{}, false
	}

	entries, found := t.faceEntries()
	if !found {
		return FacePaths{}, false
	}

	for i := range entries {
		e := &entries[i]

		path, verified := VerifiedTool(t.Dir, e.Name, e.Kind, e.Model)
		if !verified {
			return FacePaths{}, false
		}

		switch e.Name {
		case aihost.ToolHelper:
			p.Helper = path
		case aihost.ToolRuntime:
			p.Runtime = path
		case aihost.ToolModel:
			p.Model = path
		}
	}

	return p, true
}

// ToolOffer is an install set the person may choose, with what it costs and its licences, so a prompt
// shows them before anything is downloaded (SDD-024 section 3 item 2).
type ToolOffer struct {
	Name          string     `json:"name"`
	Files         []ToolFile `json:"files"`
	DownloadBytes int64      `json:"download_bytes"`
	// Dataset is the dataset the model was trained on, whose terms are not the licence of the weights.
	Dataset string `json:"dataset,omitempty"`
	// Published is false while a file of the set has not been built yet: the install is refused.
	Published bool `json:"published"`
	Installed bool `json:"installed"`
}

// faceOffer describes the face set, or nil when this build or platform has none.
func (s *svc) faceOffer() *ToolOffer {
	if s.tools == nil {
		return nil
	}

	entries, ok := s.tools.faceEntries()
	if !ok {
		return nil
	}

	offer := &ToolOffer{Name: aihost.SetName, Dataset: s.tools.Face.Dataset(), Published: true}

	for i := range entries {
		e := &entries[i]

		u, err := url.Parse(e.URL)
		if err != nil {
			return nil
		}

		offer.Files = append(
			offer.Files,
			ToolFile{Name: e.File, Bytes: e.Bytes, Host: u.Hostname(), License: e.License},
		)
		offer.DownloadBytes += e.Bytes
		offer.Published = offer.Published && e.Published
	}

	_, offer.Installed = s.tools.FaceStack()

	return offer
}

// installFace is the face branch of ToolsInstall: it announces the three files with their size and
// licences and starts the download in the background. A file that is not published yet is refused with
// reason not_published, before any connection.
func (s *svc) installFace(ctx context.Context, c Caller, force bool) (*ToolsInstallResult, error) {
	entries, ok := s.tools.faceEntries()
	if !ok {
		goos, goarch := s.tools.platform()

		return nil, newError(command.CodeValidation, "there is no %q set for %s/%s.", aihost.SetName, goos, goarch).
			withPath("name")
	}

	res := &ToolsInstallResult{State: "running"}
	licences := make([]string, 0, len(entries))

	for i := range entries {
		e := &entries[i]

		u, perr := url.Parse(e.URL)
		if perr != nil || u.Scheme != "https" || !slices.Contains(s.tools.Face.Hosts, u.Hostname()) {
			return nil, internalError()
		}

		if !e.Published {
			return nil, newError(codeTranscribeOff, "%s is not published for this platform yet.", e.File).
				withDetail("reason", "not_published").withDetail("tool", aihost.SetName).
				withHint("a release has to build it first; nothing was downloaded")
		}

		res.Files = append(res.Files, ToolFile{Name: e.File, Bytes: e.Bytes, Host: u.Hostname(), License: e.License})
		res.DownloadBytes += e.Bytes

		if !slices.Contains(licences, e.License) {
			licences = append(licences, e.License)
		}
	}

	if _, installed := s.tools.FaceStack(); installed && !force {
		return &ToolsInstallResult{
			State: "installed", Files: res.Files,
			Text: "face tracking is already installed and its files verify; pass force to install it again",
		}, nil
	}

	res.Text = fmt.Sprintf("installing face tracking: %d files, %s from %s (licences: %s; the model was trained on %s)",
		len(res.Files), sizesText(res.DownloadBytes), hostsText(res.Files), strings.Join(licences, ", "),
		s.tools.Face.Dataset())

	if err := s.beginInstall(res.DownloadBytes); err != nil {
		return nil, err
	}

	bg := context.WithoutCancel(ctx)

	go func() { s.endInstall(s.installEntries(bg, entries)) }()

	s.auditInstall(ctx, c.Actor, aihost.SetName)

	return res, nil
}

func hostsText(files []ToolFile) string {
	var hosts []string

	for _, f := range files {
		if !slices.Contains(hosts, f.Host) {
			hosts = append(hosts, f.Host)
		}
	}

	return strings.Join(hosts, ", ")
}
