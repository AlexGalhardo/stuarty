package export

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/AlexGalhardo/stuarty/engine/audio"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
)

// buildAudio makes an audio only export: the mix of section 7 as a WAV, or as AAC in an MP4 container
// (SDD-004 section 8). No picture is composed and no video encoder is needed.
func (s *state) buildAudio(ctx context.Context, f Format) (built, error) {
	mix, loud, err := s.mixAudio(ctx)
	if err != nil {
		return built{}, err
	}

	partial, err := s.dir.Path(s.job.JobID + ".partial" + f.Ext())
	if err != nil {
		return built{}, err
	}

	b := built{partial: partial, loud: loud, encoder: "pcm_s16le"}

	if f == FormatWAV {
		if err := os.Rename(mix, partial); err != nil {
			return built{}, exportErr(ReasonAudio, "The mix could not be kept as the exported file.", err, "")
		}
	} else {
		b.encoder = "aac"
		if err := s.encodeAAC(ctx, mix, partial); err != nil {
			return built{}, err
		}
	}

	if s.job.OnProgress != nil {
		s.job.OnProgress(Progress{Frame: s.frames, Total: s.frames})
	}

	b.ver, err = s.verifyAudio(ctx, partial, f)

	return b, err
}

// encodeAAC encodes the mix with the settings of the audio of an MP4 export (SDD-004 section 8).
func (s *state) encodeAAC(ctx context.Context, mix, partial string) error {
	opts := []ffargs.Opt{
		ffargs.Enum("-c:a", "aac"), ffargs.Bitrate("-b:a", "192k"),
		ffargs.Int("-ar", audio.SampleRate), ffargs.Int("-ac", 2),
		ffargs.Enum("-movflags", "+faststart"),
	}
	opts = append(opts, ffargs.Bitexact()...)

	inputs := []ffargs.Input{ffargs.FileInput(mix)}

	meta, hasChapters, err := s.chapterInput()
	if err != nil {
		return err
	}

	chapters := 0
	if hasChapters {
		inputs = append(inputs, meta)
		chapters = len(inputs) - 1
	}

	r := s.job.Runner
	argv, err := ffargs.Cmd{
		Progress: true,
		Inputs:   inputs,
		Outputs: []ffargs.Output{{
			Format: "mp4", Path: partial, Maps: []string{"0:a:0"}, NoVideo: true, Chapters: chapters, Opts: opts,
		}},
	}.Argv(r.Policy(s.dir))
	if err != nil {
		return exportErr(ReasonAudio, "The audio encoder command was refused.", err, "")
	}

	res, err := r.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: s.dir, Argv: argv, Deadline: s.deadline, DeadlineCeiling: ffmpeg.ExportDeadlineCeiling,
		OutputPath: partial, MaxOutputBytes: s.job.MaxBytes,
	})
	if err != nil {
		return wrapRun(err, ReasonAudio, "The audio encoder failed.", res.Stderr)
	}

	return nil
}

type probedAudioStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Duration   string `json:"duration"`
}

// verifyAudio is the verification of an audio only export: one stream, audio, the codec of the format,
// 48 kHz stereo, a duration within one frame of N / fps, and a file that is whole (the RIFF sizes of a
// WAV, the boxes of an MP4).
func (s *state) verifyAudio(ctx context.Context, partial string, f Format) (Verified, error) {
	fps := s.job.Doc.Settings.FPS
	want := float64(s.frames) * float64(fps.Den) / float64(fps.Num)
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

	var pf struct {
		Streams []probedAudioStream `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(raw, &pf); err != nil {
		return Verified{}, verifyError([]Check{{Name: "readable", Expected: "ffprobe JSON", Got: err.Error()}})
	}

	codec := "pcm_s16le"
	if f == FormatAAC {
		codec = "aac"
	}

	var failed []Check
	fail := func(name, expected, got string) { failed = append(failed, Check{name, expected, got}) }

	var media []probedAudioStream

	data := 0

	for _, x := range pf.Streams {
		if x.CodecType == "data" {
			data++
		} else {
			media = append(media, x)
		}
	}

	switch {
	case len(media) != 1 || data > s.chapterTracks():
		fail("streams", "one audio stream and no video stream"+s.chapterTrackNote(), strconv.Itoa(len(pf.Streams))+" streams")
	default:
		a := media[0]
		if a.CodecType != "audio" || a.CodecName != codec {
			fail("audio_stream", "one "+codec+" stream", a.CodecType+" "+a.CodecName)
		}

		if a.SampleRate != strconv.Itoa(audio.SampleRate) || a.Channels != 2 {
			fail("audio_format", "48000 Hz, 2 channels", a.SampleRate+" Hz, "+strconv.Itoa(a.Channels)+" channels")
		}
	}

	// A WAV carries its duration in the format, and AAC in an MP4 too: the container is what a player reads.
	checkDuration(&failed, "audio_duration", pf.Format.Duration, want, frameSec)

	switch f {
	case FormatWAV:
		if err := checkWAVComplete(partial); err != nil {
			fail("complete", "RIFF and data sizes that fill the file", err.Error())
		}
	case FormatAAC:
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
	case FormatMP4, FormatGIF:
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

// checkWAVComplete reads the 44 byte header the engine writes and requires its sizes to match the file.
func checkWAVComplete(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()

	h := make([]byte, 44)
	if _, err := io.ReadFull(fh, h); err != nil {
		return fmt.Errorf("a header shorter than 44 bytes: %w", err)
	}

	if string(h[0:4]) != "RIFF" || string(h[8:16]) != "WAVEfmt " || string(h[36:40]) != "data" {
		return errors.New("not the RIFF/WAVE layout of the engine")
	}

	riff, data := int64(binary.LittleEndian.Uint32(h[4:8])), int64(binary.LittleEndian.Uint32(h[40:44]))
	if riff+8 != fi.Size() || data+44 != fi.Size() {
		return fmt.Errorf("RIFF says %d bytes and data %d, the file has %d", riff+8, data+44, fi.Size())
	}

	return nil
}
