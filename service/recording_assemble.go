package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg/ffargs"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// The assembly of an uploaded recording (SDD-025 section 17.5): the chunks of a track are joined in order, remuxed
// with the LGPL FFmpeg as a copy of the streams so the file has its duration and index, probed with the ceilings of
// an import, and moved into the folder of the recording. Everything is done in a run directory of the runner, under
// the argv policy of SDD-007 section 6.

// assembleUpload makes the file of every track a browser uploaded. The screen is required: a recording that has no
// chunk of it, or whose file has no decodable duration, fails. A track the browser asked for and never sent is
// dropped.
func (s *svc) assembleUpload(ctx context.Context, r recordings.Row) uploadResult {
	if s.media.Runner == nil {
		return failedUpload("ffmpeg_missing", "no FFmpeg is installed to assemble the recording; the chunks are kept")
	}

	var (
		out    uploadResult
		folder = s.recordingFolder(r.ID)
		cont   = recording.Container(r.Encoder)
	)

	for _, t := range decodeTracks(r.Tracks) {
		if !recording.UploadKind(t.Kind) || !cont.Valid() {
			return failedUpload("assemble", "the recording names a track or a container that is not uploaded")
		}

		n, _, _ := scanParts(folder, t.Kind)
		if n == 0 {
			if t.Kind == recording.TrackScreen {
				return failedUpload("no_frames", "the server received no chunk of the screen")
			}

			continue
		}

		info, err := s.assembleTrack(ctx, r.ID, folder, t.Kind, cont, n)
		if err != nil {
			if ctx.Err() != nil {
				return failedUpload("assemble", "the assembly was stopped")
			}

			return failedUpload(err.reason, "%s", err.message)
		}

		t.File, t.DurationMS, t.Bytes, t.StartMS = cont.File(t.Kind), info.durationMS, info.bytes, 0
		out.tracks = append(out.tracks, t)
		out.bytes += info.bytes

		if t.Kind == recording.TrackScreen {
			out.width, out.height, out.durationMS = info.width, info.height, info.durationMS
		}
	}

	if len(out.tracks) == 0 {
		return failedUpload("no_frames", "the recording has no track")
	}

	return out
}

type trackFile struct {
	durationMS    int64
	bytes         int64
	width, height int
}

type assembleError struct{ reason, message string }

func (s *svc) assembleTrack(
	ctx context.Context, id, folder string, kind recording.TrackKind, cont recording.Container, parts int,
) (trackFile, *assembleError) {
	runner := s.media.Runner

	dir, err := runner.NewJobRunDir("up_" + id + "_" + string(kind))
	if err != nil {
		return trackFile{}, &assembleError{"assemble", "the working folder could not be made"}
	}

	defer func() { _ = dir.Remove() }()

	joined, err := dir.Path(string(kind) + ".join")
	if err != nil {
		return trackFile{}, &assembleError{"assemble", "the working file could not be named"}
	}

	if err := joinParts(joined, folder, kind, parts); err != nil {
		return trackFile{}, &assembleError{"assemble", "the chunks could not be joined"}
	}

	pr, err := runner.Probe(ctx, dir, joined, s.limits.Ceilings)
	if err != nil {
		return trackFile{}, &assembleError{"assemble", probeMessage(err)}
	}

	var video, audio bool

	for _, st := range pr.Streams {
		video = video || st.CodecType == "video"
		audio = audio || st.CodecType == "audio"
	}

	if !video {
		return trackFile{}, &assembleError{"assemble", "the upload has no picture"}
	}

	maps := []string{"0:v:0"}
	if audio {
		maps = append(maps, "0:a:0")
	}

	opts := []ffargs.Opt{ffargs.Enum("-c:v", "copy"), ffargs.Enum("-c:a", "copy")}
	if cont == recording.ContainerMP4 {
		opts = append(opts, ffargs.Enum("-movflags", "+faststart"))
	}

	made, err := dir.Path(string(kind) + "." + string(cont))
	if err != nil {
		return trackFile{}, &assembleError{"assemble", "the working file could not be named"}
	}

	argv, err := ffargs.Cmd{
		LogErrorOnly: true,
		Inputs:       []ffargs.Input{ffargs.FileInput(joined)},
		Outputs:      []ffargs.Output{{Format: string(cont), Path: made, Maps: maps, Opts: opts}},
	}.Argv(runner.Policy(dir))
	if err != nil {
		return trackFile{}, &assembleError{"assemble", "the FFmpeg arguments were refused"}
	}

	size := int64(0)
	if st, serr := os.Stat(joined); serr == nil {
		size = st.Size()
	}

	deadline := ffmpeg.IngestDeadlineFor(time.Duration(pr.Format.Seconds*float64(time.Second)), size)

	if _, err := runner.RunFFmpeg(ctx, ffmpeg.Request{
		Dir: dir, Argv: argv, Deadline: deadline, DeadlineCeiling: ffmpeg.IngestDeadlineCeiling,
		OutputPath: made, MaxOutputBytes: s.recordingLimits().MaxBytes,
	}); err != nil {
		return trackFile{}, &assembleError{"assemble", "FFmpeg could not remux the upload"}
	}

	done, err := runner.Probe(ctx, dir, made, s.limits.Ceilings)
	if err != nil {
		return trackFile{}, &assembleError{"assemble", probeMessage(err)}
	}

	info := trackFile{durationMS: int64(done.Format.Seconds * 1000)}
	if info.durationMS <= 0 {
		return trackFile{}, &assembleError{"no_frames", "the assembled file has no duration"}
	}

	for _, st := range done.Streams {
		if st.CodecType == "video" && info.width == 0 {
			info.width, info.height = st.Width, st.Height
		}
	}

	st, err := os.Stat(made)
	if err != nil {
		return trackFile{}, &assembleError{"assemble", "the assembled file is missing"}
	}

	info.bytes = st.Size()

	final := filepath.Join(folder, cont.File(kind))
	_ = os.Remove(final)

	if err := moveFile(made, final); err != nil {
		return trackFile{}, &assembleError{"assemble", "the assembled file could not be placed"}
	}

	return info, nil
}

func probeMessage(err error) string {
	if fe, ok := errors.AsType[*ffmpeg.Error](err); ok && fe.Message != "" {
		return "the upload is not a file the product reads: " + fe.Message
	}

	return "the upload could not be read"
}

// joinParts writes chunks 0 to n-1 of a track, in order, into one file.
func joinParts(dst, folder string, kind recording.TrackKind, n int) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	for i := range n {
		if err := appendFile(out, partPath(folder, kind, i)); err != nil {
			_ = out.Close()

			return err
		}
	}

	return out.Close()
}

func appendFile(dst io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	_, err = io.Copy(dst, f)

	return err
}

// moveFile renames, or copies when the run directory is on another volume than the recordings.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}

	in, err := os.Open(from)
	if err != nil {
		return err
	}

	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}
