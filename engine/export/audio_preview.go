package export

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/audio"
)

// MaxPreviewSeconds is the longest source RenderAudioPreview renders: the length of an item the document allows.
const MaxPreviewSeconds = 4 * 3600

// RenderAudioPreview renders one source made by plan.PreviewSource, with its asset range, speed, noise reduction and
// item chain, through the mixer of the export, and writes it as a 48 kHz stereo 16-bit WAV at out (SDD-004 section
// 7.1, "Processed audio on the timeline"). The sound is taken before the envelope and before the loudness gain.
// out must not exist: the caller writes a name of its own and renames it when this returns.
func RenderAudioPreview(ctx context.Context, job Job, src plan.AudioSource, out string) error {
	if job.Runner == nil || job.Doc == nil || job.ArtifactsDir == "" {
		return errors.New("export: the preview job needs a runner, a document and the artifacts folder")
	}

	if src.Samples <= 0 || src.Samples > MaxPreviewSeconds*audio.SampleRate {
		return errors.New("export: the preview source is empty or too long")
	}

	dir, err := job.Runner.NewJobRunDir(job.JobID)
	if err != nil {
		return wrapRun(err, ReasonRender, "A work folder for the audio preview could not be made.", "")
	}

	defer func() { _ = dir.Remove() }()

	secs := float64(src.Samples) / audio.SampleRate
	s := newState(ctx, &job, dir, int(secs*float64(job.Doc.Settings.FPS.Num)/float64(job.Doc.Settings.FPS.Den))+1)
	s.deadline = max(s.deadline, time.Duration(secs*float64(time.Second))*4)

	raw, err := dir.Path("preview-f32.wav")
	if err != nil {
		return err
	}

	err = writeFile(raw, func(w io.Writer) error {
		return audio.MixToFloatWAV(ctx, []plan.AudioSource{src}, src.Samples, &audioOpener{s: s}, w)
	})
	if err != nil {
		return wrapAudio(err, "")
	}

	if err := writeGainOnce(raw, out, src.Samples, 0, false); err != nil {
		_ = os.Remove(out)

		return wrapAudio(err, "")
	}

	return nil
}
