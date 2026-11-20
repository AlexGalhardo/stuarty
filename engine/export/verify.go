package export

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	exportcore "github.com/AlexGalhardo/stuarty/core/export"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// verifyTimeout bounds each ffprobe run of the verification.
const verifyTimeout = 60 * time.Second

type probedStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	PixFmt    string `json:"pix_fmt"`
	Duration  string `json:"duration"`
	Rate      string `json:"r_frame_rate"`
}

type probedFile struct {
	Streams []probedStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Verified is what verification learned about a good file.
type Verified struct {
	DurationMS int64
	Size       int64
	// Width and Height are the pixels of the file when they are not the project's (a GIF is
	// capped, VV-056); zero means the project's size.
	Width, Height int
}

// verify applies section 8 step 7 to the partial file: one video stream of the codec that was written and one
// AAC stream, the project's size, yuv420p, both durations within one frame of N / fps, one packet per frame,
// moov before mdat, and boxes that fill the file; with a preset, also the limits of the preset on the real file
// (SDD-004 section 8.2). Any failure is E_VERIFY_FAILED with the list of failed checks.
func (s *state) verify(ctx context.Context, partial string, frames int, codec exportcore.Codec) (Verified, error) {
	st := s.job.Doc.Settings
	fps := st.FPS
	want := float64(frames) * float64(fps.Den) / float64(fps.Num)
	frameSec := float64(fps.Den) / float64(fps.Num)

	pol := s.job.Runner.Policy(s.dir)
	argv, err := ffargs.ProbeJSON(pol, partial)
	if err != nil {
		return Verified{}, exportErr(ReasonRender, "The verification command was refused.", err, "")
	}
	raw, res, err := s.job.Runner.RunFFprobe(ctx, s.dir, argv, verifyTimeout)
	if err != nil {
		return Verified{}, wrapRun(err, ReasonRender, "The exported file could not be read back.", res.Stderr)
	}
	var pf probedFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return Verified{}, verifyError([]Check{{Name: "readable", Expected: "ffprobe JSON", Got: err.Error()}})
	}

	pargv, err := ffargs.ProbeEncodedPackets(pol, partial)
	if err != nil {
		return Verified{}, exportErr(ReasonRender, "The verification command was refused.", err, "")
	}
	pkts, pres, err := s.job.Runner.RunFFprobe(ctx, s.dir, pargv, verifyTimeout)
	if err != nil {
		return Verified{}, wrapRun(err, ReasonRender, "The exported file could not be read back.", pres.Stderr)
	}

	var failed []Check
	fail := func(name, expected, got string) { failed = append(failed, Check{name, expected, got}) }

	var video, audio []probedStream
	data := 0
	for _, x := range pf.Streams {
		switch x.CodecType {
		case "video":
			video = append(video, x)
		case "audio":
			audio = append(audio, x)
		case "data":
			data++
		}
	}
	name := string(codec.Normal())
	if len(video) != 1 || video[0].CodecName != name {
		fail("video_stream", "one "+name+" stream", describe(video))
	}
	if len(audio) != 1 || audio[0].CodecName != "aac" {
		fail("audio_stream", "one aac stream", describe(audio))
	}
	if len(pf.Streams) != len(video)+len(audio)+data || data != s.chapterTracks() {
		fail("streams", "a video and an audio stream"+s.chapterTrackNote(), strconv.Itoa(len(pf.Streams))+" streams")
	}
	if len(video) == 1 {
		v := video[0]
		if v.Width != st.Width || v.Height != st.Height {
			fail("dimensions", fmt.Sprintf("%dx%d", st.Width, st.Height), fmt.Sprintf("%dx%d", v.Width, v.Height))
		}
		if v.PixFmt != "yuv420p" {
			fail("pixel_format", "yuv420p", v.PixFmt)
		}
		// The nominal rate of the stream, not avg_frame_rate: the average divides the packet count by
		// the stream duration, which an encoder with B-frames (h264_videotoolbox) ends one frame short,
		// so a correct file reads 31.03 fps at 30. The count and the duration have their own checks.
		if !sameRate(v.Rate, fps) {
			fail("frame_rate", fmt.Sprintf("%d/%d", fps.Num, fps.Den), v.Rate)
		}
		checkDuration(&failed, "video_duration", v.Duration, want, frameSec)
		got := strings.TrimSpace(string(pkts))
		if exp := name + "," + strconv.Itoa(frames); got != exp {
			fail("video_packets", exp, got)
		}
	}
	if len(audio) == 1 {
		checkDuration(&failed, "audio_duration", audio[0].Duration, want, frameSec)
	}

	b, err := readBoxes(partial)
	switch {
	case err != nil:
		fail("mp4_boxes", "readable top-level boxes", err.Error())
	case !b.complete:
		fail("complete", "boxes that fill the file", "a box runs past the end of the file")
	case b.moov < 0 || b.mdat < 0 || b.moov > b.mdat:
		fail("faststart", "moov before mdat", fmt.Sprintf("moov at %d, mdat at %d", b.moov, b.mdat))
	}

	checks, err := s.verifyChapters(ctx, partial)
	if err != nil {
		return Verified{}, err
	}

	failed = append(failed, checks...)

	for _, f := range s.presetFindings(partial, want, codec) {
		fail(f.CheckName(), f.Limit, f.Got)
	}

	if len(failed) > 0 {
		return Verified{}, verifyError(failed)
	}
	out := Verified{}
	if d, err := strconv.ParseFloat(pf.Format.Duration, 64); err == nil {
		out.DurationMS = int64(math.Round(d * 1000))
	}
	if fi, err := os.Stat(partial); err == nil {
		out.Size = fi.Size()
	}
	return out, nil
}

// presetFindings are the limits of the preset of the job that the written file breaks: the same function as
// ExportCheck, over the real duration and size (SDD-004 section 8.2).
func (s *state) presetFindings(partial string, seconds float64, codec exportcore.Codec) []exportcore.Finding {
	p := s.job.Preset
	if p == nil {
		return nil
	}

	st := s.job.Doc.Settings
	size := int64(0)

	if fi, err := os.Stat(partial); err == nil {
		size = fi.Size()
	}

	return p.Check(exportcore.Input{
		Width: st.Width, Height: st.Height, FPSNum: st.FPS.Num, FPSDen: st.FPS.Den,
		DurationMS: int64(math.Round(seconds * 1000)), Bytes: max(size, 1), Codec: codec,
	})
}

func describe(ss []probedStream) string {
	if len(ss) == 0 {
		return "none"
	}
	parts := make([]string, len(ss))
	for i, x := range ss {
		parts[i] = x.CodecType + " " + x.CodecName
	}
	return strings.Join(parts, ", ")
}

// checkDuration requires the duration ffprobe printed to be within one frame of what the project says.
func checkDuration(failed *[]Check, name, got string, want, frameSec float64) {
	exp := strconv.FormatFloat(want, 'f', 3, 64) + " s +- " + strconv.FormatFloat(frameSec, 'f', 3, 64) + " s"
	d, err := strconv.ParseFloat(got, 64)
	if err != nil || math.Abs(d-want) > frameSec {
		*failed = append(*failed, Check{Name: name, Expected: exp, Got: got})
	}
}

// sameRate says whether ffprobe's r_frame_rate ("60/1") is the project's frame rate, as fractions.
func sameRate(got string, fps timeline.Rational) bool {
	num, den, ok := strings.Cut(got, "/")
	n, err1 := strconv.ParseInt(num, 10, 64)
	d, err2 := strconv.ParseInt(den, 10, 64)

	return ok && err1 == nil && err2 == nil && d > 0 && n*int64(fps.Den) == int64(fps.Num)*d
}
