package contract

import (
	"bytes"
	"context"
	be "encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/vukem-video/service"
)

// The reference scenario (P1-QA-001, Prototype exit criteria 1 and 2): create a
// 9:16 project, import a portrait clip, put it on the timeline, split it and
// delete one part, add a caption, lint, look at a frame and export. The CLI
// process, MCP over stdio and MCP over HTTP each drive it against a real
// `serve`, each export is verified with ffprobe as SDD-004 section 8 says, and
// the three project documents are compared after the identifiers the scenario
// supplies (NFR-025). It needs the pinned LGPL FFmpeg and skips without one
// (macOS in CI has only a GPL build, risk R6).

const referenceProject = "prj_7k2m9x4q1b"

// framer is a driver that can look at a frame of the project.
type framer interface {
	viewFrame(t *testing.T, project string)
}

func (d *cliProc) viewFrame(t *testing.T, project string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, d.p.bin, "view", "--project", project, "--at", "1s", "--json", "--actor", actor)
	cmd.Env = d.p.env

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("cli view: %v\n%s%s", err, stdout.String(), stderr.String())
	}

	var doc struct {
		Mime string `json:"mime"`
		Path string `json:"path"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil || doc.Mime != "image/jpeg" {
		t.Fatalf("cli view: %v %q", err, stdout.String())
	}

	if info, err := os.Stat(doc.Path); err != nil || info.Size() == 0 {
		t.Fatalf("cli view saved no image at %q: %v", doc.Path, err)
	}
}

func (d *mcpProc) viewFrame(t *testing.T, project string) {
	t.Helper()

	res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{
		Name: "view", Arguments: map[string]any{"project_id": project, "at": "1s"},
	})
	if err != nil || res.IsError {
		t.Fatalf("%s view: %v %+v", d.label, err, res)
	}

	for _, c := range res.Content {
		if img, ok := c.(*sdk.ImageContent); ok && len(img.Data) > 0 && img.MIMEType == "image/jpeg" {
			return
		}
	}

	t.Fatalf("%s view returned no jpeg image block", d.label)
}

// probed is the part of ffprobe output the verification reads.
type probed struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		PixFmt     string `json:"pix_fmt"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// verifyExport is SDD-004 section 8 on the file the export job wrote: H.264,
// AAC, 1080x1920, 30 fps, a duration within one frame of the timeline and the
// moov atom before the media data (faststart).
func verifyExport(t *testing.T, p *procEnv, label string, wantSeconds float64) {
	t.Helper()

	jobs, err := p.service().JobList(callerCtx(), service.JobListRequest{ProjectID: referenceProject, State: "all"})
	if err != nil {
		t.Fatal(err)
	}

	var path string

	for i := range jobs.Jobs {
		j := &jobs.Jobs[i]
		if j.Kind == service.JobExport && j.State == service.JobSucceeded && j.Result != nil && j.Result.Export != nil {
			path = j.Result.Export.Path
		}
	}

	if path == "" {
		t.Fatalf("%s: no succeeded export job in %+v", label, jobs.Jobs)
	}

	out, err := exec.CommandContext(t.Context(), p.ffprobe,
		"-v", "error", "-print_format", "json", "-show_streams", "-show_format", path).Output()
	if err != nil {
		t.Fatalf("%s: ffprobe: %v", label, err)
	}

	var pr probed
	if err := json.Unmarshal(out, &pr); err != nil {
		t.Fatal(err)
	}

	var video, audio bool

	for _, s := range pr.Streams {
		switch s.CodecType {
		case "video":
			video = true

			wrong := s.CodecName != "h264" || s.Width != 1080 || s.Height != 1920
			if wrong || s.RFrameRate != "30/1" || s.PixFmt != "yuv420p" {
				t.Errorf("%s: video stream is %+v, want h264 1080x1920 30/1 yuv420p", label, s)
			}
		case "audio":
			audio = true

			if s.CodecName != "aac" {
				t.Errorf("%s: audio codec is %s, want aac", label, s.CodecName)
			}
		}
	}

	if !video || !audio {
		t.Errorf("%s: video=%v audio=%v, want both", label, video, audio)
	}

	got, err := strconv.ParseFloat(pr.Format.Duration, 64)
	if err != nil || math.Abs(got-wantSeconds) > 1.0/30+0.001 {
		t.Errorf("%s: duration %v, want %vs within one frame (%v)", label, pr.Format.Duration, wantSeconds, err)
	}

	if !moovBeforeMdat(t, path) {
		t.Errorf("%s: the moov atom is not before mdat: the file is not faststart", label)
	}
}

// moovBeforeMdat walks the top-level boxes of an MP4 file.
func moovBeforeMdat(t *testing.T, path string) bool {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	for {
		var head [8]byte
		if _, err := io.ReadFull(f, head[:]); err != nil {
			return false
		}

		size := int64(be.BigEndian.Uint32(head[:4]))

		switch string(head[4:]) {
		case "moov":
			return true
		case "mdat":
			return false
		}

		if size == 1 { // a 64-bit size follows the name
			var big [8]byte
			if _, err := io.ReadFull(f, big[:]); err != nil {
				return false
			}

			size = int64(be.BigEndian.Uint64(big[:])) - 8
		}

		if size < 8 {
			return false
		}

		if _, err := f.Seek(size-8, io.SeekCurrent); err != nil {
			return false
		}
	}
}

func TestReferenceScenarioThroughTheRealServer(t *testing.T) {
	driveRealServer(t, "reference", referenceProject, true, func(t *testing.T, d driver, p *procEnv) {
		t.Helper()

		fr, ok := d.(framer)
		if !ok {
			t.Fatalf("%s cannot view a frame", d.name())
		}

		fr.viewFrame(t, referenceProject)
		verifyExport(t, p, d.name(), 2)
	})
}
