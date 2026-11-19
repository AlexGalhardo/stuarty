package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// stillRange is the one frame of a png export (VV-037): args.at, "0s" when it is not given, as the
// frame [at, at+1). start and end belong to a video and are refused. A time past the last frame is
// refused too, so a typo does not silently export the last frame.
func stillRange(doc *timeline.Document, a ExportArgs) (start, end int64, err *Error) {
	if a.Start != "" || a.End != "" {
		return 0, 0, newError(command.CodeValidation, "a png export is one frame and takes at, not start and end.").
			withPath("args.start").withHint("use at, such as at=1.5s")
	}

	at := int64(0)

	if a.At != "" {
		fps := timeval.FPS{Num: int64(doc.Settings.FPS.Num), Den: int64(doc.Settings.FPS.Den)}

		var perr error
		if at, perr = timeval.Parse(a.At, fps); perr != nil {
			return 0, 0, newError(command.CodeInvalidTime, "at is not a time.").withPath("args.at").
				withHint("write a time like 1.5s or 45f")
		}
	}

	if last := endFrame(doc); at < 0 || at >= max(last, 1) {
		return 0, 0, newError(command.CodeValidation, "at is frame %d; the timeline has frames 0 to %d.",
			at, max(last, 1)-1).withPath("args.at")
	}

	return at, at + 1, nil
}

// runStillExport is the handler of a png export: the frame at in.Start composed at the size of the
// project, as a lossless PNG, written atomically to exports/<project>/<job>.png.
func (s *svc) runStillExport(ctx context.Context, job *JobRow, pw *progress, in exportInput) (json.RawMessage, error) {
	if s.media.Frames == nil || job.ProjectID == nil {
		return nil, internalError()
	}

	doc, derr := decodeRow(&ProjectRow{Document: in.Document})
	if derr != nil {
		return nil, derr
	}

	if err := s.checkDisk(); err != nil {
		return nil, err
	}

	pw.report(ctx, 0.05, "Rendering the frame")

	body, err := s.framesFor(doc).Still(ctx, doc, int(in.Start))
	if err != nil {
		return nil, viewError(err)
	}

	cfg, perr := png.DecodeConfig(bytes.NewReader(body))
	if perr != nil || cfg.Width != doc.Settings.Width || cfg.Height != doc.Settings.Height {
		return nil, newError(CodeVerifyFailed, "The still failed its check and was not written.").
			withDetail("checks", []VerifyCheck{{Name: "dimensions", OK: false, Detail: "the picture is not the project size"}})
	}

	if s.limits.MaxExportBytes > 0 && int64(len(body)) > s.limits.MaxExportBytes {
		have, ceiling := sizesApart(int64(len(body)), s.limits.MaxExportBytes)

		return nil, newError(
			CodeExportFailed,
			"The export is %s; the export limit is %s (STUARTY_MAX_EXPORT_BYTES).",
			have,
			ceiling,
		).
			withDetail("reason", exportSizeLimitReason).
			withLimit("export_bytes", int64(len(body)), s.limits.MaxExportBytes).
			withDetail("variable", "STUARTY_MAX_EXPORT_BYTES")
	}

	out, aerr := filepath.Abs(filepath.Join(s.workspace, "exports", *job.ProjectID, exportFileName(job.ID, in.Format)))
	if aerr != nil || writeFileAtomic(out, body) != nil {
		return nil, internalError()
	}

	sum := sha256.Sum256(body)
	fps := doc.Settings.FPS

	raw, merr := json.Marshal(JobResult{Export: &ExportResult{
		Revision: in.Revision, Format: FormatPNG, Path: out, DownloadURL: "/api/v1/jobs/" + job.ID + "/file",
		Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Duration: timeValue(fps, 1),
		Width: cfg.Width, Height: cfg.Height, FPS: fpsString(fps.Num, fps.Den),
		Checks: []VerifyCheck{{
			Name: "dimensions", OK: true, Detail: "png, " + strconv.Itoa(cfg.Width) + "x" + strconv.Itoa(cfg.Height),
		}},
	}})
	if merr != nil {
		_ = os.Remove(out)

		return nil, internalError()
	}

	return raw, nil
}
