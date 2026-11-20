package servicetest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

var formatTable = []struct{ format, ext, contentType string }{
	{"", ".mp4", "video/mp4"},
	{"mp4", ".mp4", "video/mp4"},
	{"gif", ".gif", "image/gif"},
	{"wav", ".wav", "audio/wav"},
	{"aac", ".m4a", "audio/mp4"},
}

func (m *mediaEnv) startFormat(project, format string) (*service.Job, error) {
	args := json.RawMessage(`{}`)
	if format != "" {
		args = json.RawMessage(fmt.Sprintf(`{"format":%q}`, format))
	}

	return m.svc.JobStart(m.as(ui), service.JobStartRequest{ProjectID: project, Kind: "export", Args: args})
}

// P3-SRV-005: every format starts through JobStart, reaches succeeded, is written under
// exports/<project>/<job>.<ext> and is served with the content type of its format.
func TestExportFormatsEachSucceedsAndIsServed(t *testing.T) {
	var (
		mu   sync.Mutex
		seen = map[string]string{}
	)

	m := newMediaEnv(
		t,
		mediaCfg{exporter: exportFn(func(_ context.Context, in service.ExportInput) (*service.ExportOutput, error) {
			mu.Lock()
			seen[in.OutPath] = in.Format
			mu.Unlock()

			return writeOutput(in, okChecks)
		})},
	)
	project := m.exportable()
	audible, _, _, _ := m.speechProject()

	for _, c := range formatTable {
		target := project
		if c.format == "wav" || c.format == "aac" {
			target = audible
		}

		job, err := m.startFormat(target, c.format)
		if err != nil {
			t.Fatalf("format %q: %v", c.format, err)
		}

		if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
			t.Fatalf("format %q: job = %+v", c.format, job)
		}

		e := job.Result.Export
		want := filepath.Join(m.ws, "exports", target, job.JobID+c.ext)

		if e.Path != want {
			t.Errorf("format %q: path = %s, want %s", c.format, e.Path, want)
		}

		wantName := c.format
		if wantName == "" {
			wantName = "mp4"
		}

		mu.Lock()
		got := seen[e.Path]
		mu.Unlock()

		if got != wantName {
			t.Errorf("format %q: the exporter was asked for %q", c.format, got)
		}

		if _, err := os.Stat(e.Path); err != nil {
			t.Errorf("format %q: %v", c.format, err)
		}

		if e.Format != wantName {
			t.Errorf("format %q: result format = %q", c.format, e.Format)
		}

		f, ferr := m.svc.ExportFile(m.as(ui), service.ExportFileRequest{JobID: job.JobID})
		if ferr != nil {
			t.Fatalf("format %q: ExportFile: %v", c.format, ferr)
		}

		if f.ContentType != c.contentType || f.Path != path.Join("exports", target, job.JobID+c.ext) {
			t.Errorf("format %q: file = %+v", c.format, f)
		}
	}
}

func TestExportFormatsRefuseWhatIsNotAFormat(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project := m.exportable()

	for _, format := range []string{"mov", "GIF", "bundle", "../x"} {
		_, err := m.startFormat(project, format)

		var se *service.Error
		if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != "args.format" {
			t.Errorf("format %q: err = %v, want E_VALIDATION at args.format", format, err)
		}
	}

	_, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: "export", Args: json.RawMessage(`{"format":7}`),
	})
	if err == nil {
		t.Error("a format that is not a string was accepted")
	}

	_, err = m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: "export", Args: json.RawMessage(`{"fps":121}`),
	})

	var se *service.Error
	if !errors.As(err, &se) || se.Code != command.CodeValidation || se.Path != "args.fps" {
		t.Errorf("fps is a project setting, not an argument: %v", err)
	}

	list, lerr := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project, State: "all"})
	if lerr != nil || len(list.Jobs) != 0 {
		t.Errorf("a refused start left jobs: %+v, %v", list, lerr)
	}
}

// A real export of every format on the pinned FFmpeg; the test skips without it.
func TestExportFormatsRealEngine(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{realExport: true, ingest: ingest.Run})

	project, track := m.withTrack()
	if _, err := m.addText(ui, project, track, 1, "0s", "hello"); err != nil {
		t.Fatal(err)
	}

	m.mustEdit(ui, project, m.head(project).Revision, op("settings.set", `{"width":320,"height":240}`))

	// A sound for the audio-only formats: they refuse a project that is silent.
	sound := m.importOne(project, m.write("tone.wav", string(silentWav()))).Assets[0].AssetID
	audioTrack := m.mustEdit(ui, project, m.head(project).Revision, op("track.add", `{"kind":"audio","name":"A"}`))
	m.mustEdit(ui, project, m.head(project).Revision, op(
		"audio.add",
		fmt.Sprintf(`{"track_id":%q,"asset_id":%q,"start":"0s","duration":"1s"}`, audioTrack.Created[0], sound),
	))

	for _, c := range formatTable {
		job, err := m.startFormat(project, c.format)
		if err != nil {
			t.Fatalf("format %q: %v", c.format, err)
		}

		if job.State != service.JobSucceeded || job.Result == nil || job.Result.Export == nil {
			t.Fatalf("format %q: job = %s", c.format, jobSummary(job))
		}

		e := job.Result.Export
		if !strings.HasSuffix(e.Path, c.ext) || e.Bytes == 0 || len(e.SHA256) != 64 || len(e.Checks) == 0 {
			t.Errorf("format %q: result = %+v", c.format, e)
		}

		switch c.format {
		case "gif":
			if e.VideoCodec != "gif" || e.AudioCodec != "" {
				t.Errorf("gif codecs: %+v", e)
			}
		case "wav":
			if e.VideoCodec != "" || e.AudioCodec != "pcm_s16le" {
				t.Errorf("wav codecs: %+v", e)
			}
		case "aac":
			if e.VideoCodec != "" || e.AudioCodec != "aac" {
				t.Errorf("aac codecs: %+v", e)
			}
		default:
			if e.VideoCodec != "h264" || e.AudioCodec != "aac" {
				t.Errorf("mp4 codecs: %+v", e)
			}
		}
	}
}

// silentWav is one second of 8 kHz mono PCM silence: a real file the ingest can probe.
func silentWav() []byte {
	const rate, bytes = 8000, 16000

	le := func(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

	out := append([]byte("RIFF"), le(36+bytes)...)
	out = append(out, "WAVEfmt "...)
	out = append(out, le(16)...)
	out = append(out, 1, 0, 1, 0)
	out = append(out, le(rate)...)
	out = append(out, le(rate*2)...)
	out = append(out, 2, 0, 16, 0)
	out = append(out, "data"...)
	out = append(out, le(bytes)...)

	return append(out, make([]byte, bytes)...)
}

func jobSummary(j *service.Job) string {
	return fmt.Sprintf("%+v", *j)
}

// A wav or aac of a project with no sound is refused with the finding, not written.
func TestAudioOnlyExportRefusesASilentProject(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{exporter: goodExporter()})
	project := m.exportable()

	for _, format := range []string{"wav", "aac"} {
		_, err := m.startFormat(project, format)
		if code(err) != service.CodeLintBlocking {
			t.Errorf("format %q: code = %q, want %q", format, code(err), service.CodeLintBlocking)
		}
	}

	if _, err := m.startFormat(project, "mp4"); err != nil {
		t.Errorf("mp4 of a silent project: %v", err)
	}
}
