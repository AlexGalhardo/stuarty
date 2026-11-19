package service

import (
	"context"
	"encoding/json"
	"path"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// ExportFileRequest names a finished export by its job.
type ExportFileRequest struct {
	JobID string `json:"job_id"`
}

// ExportFile resolves the file of a succeeded export: exports/<project>/<job>.<ext>
// under the workspace, built from the database row and never from the request
// (SDD-003 section 6.5, SDD-007 section 5.3). A job of another user is not found
// and a job of another project of a limited key is forbidden, exactly like JobGet.
// The path is relative to the workspace; the caller serves it with Range support.
func (s *svc) ExportFile(ctx context.Context, req ExportFileRequest) (*File, error) {
	c, err := authorize(ctx, "ExportFile", "")
	if err != nil {
		return nil, err
	}

	job, gerr := s.store.Jobs().Get(ctx, req.JobID)
	if gerr != nil {
		return nil, jobNotFound(req.JobID)
	}

	if !s.canSee(ctx, c, &job) {
		return nil, s.refuseJob(ctx, c, &job, req.JobID)
	}

	if job.ProjectID == nil || job.Kind != JobExport || job.State != JobSucceeded {
		return nil, newError(command.CodeInvalidState, "the job is not a succeeded export.").
			withHint("wait for the job to succeed, then ask for the file")
	}

	if strings.ContainsAny(job.ID+*job.ProjectID, `/\`) {
		return nil, internalError()
	}

	if s.isBundle(&job) {
		var res JobResult
		if json.Unmarshal(job.Result, &res) != nil || res.Export == nil {
			return nil, internalError()
		}

		return &File{
			Path:         path.Join("exports", *job.ProjectID, bundleFileName(res.Export.Revision, job.ID)),
			DownloadName: s.downloadName(ctx, &job, ".vvbundle"),
			ContentType:  "application/zip",
			ETag:         exportETag(&job),
			Immutable:    true,
			MaxAge:       31536000,
		}, nil
	}

	// A job row with no readable input is a video, as it was before formats.
	var in exportInput

	_ = json.Unmarshal(job.Input, &in)

	f, _, ok := exportFormatOf(in.Format)
	if !ok {
		return nil, internalError()
	}

	return &File{
		Path:         path.Join("exports", *job.ProjectID, job.ID+f.ext),
		DownloadName: s.downloadName(ctx, &job, f.ext),
		ContentType:  f.contentType,
		ETag:         exportETag(&job),
		Immutable:    true,
		MaxAge:       31536000,
	}, nil
}

// exportETag is "sha256-<first 16 hex of the file hash>" when the result
// records the hash, and the job id otherwise: the id pins the content too.
func exportETag(job *JobRow) string {
	var res struct {
		Export struct {
			SHA256 string `json:"sha256"`
		} `json:"export"`
	}

	if json.Unmarshal(job.Result, &res) == nil && len(res.Export.SHA256) >= 16 {
		return `"sha256-` + res.Export.SHA256[:16] + `"`
	}

	return `"job-` + job.ID + `"`
}

// isBundle reports whether the export job wrote a .vvbundle.
func (s *svc) isBundle(job *JobRow) bool {
	var in exportInput

	return json.Unmarshal(job.Input, &in) == nil && in.Format == bundleExportFormat
}

// downloadName is the file name a browser saves an export under:
// <project name>-<yyyy-mm-dd><ext>, with the name reduced to ASCII letters and
// digits so it is safe in a header. An unreadable project falls back to the job id.
func (s *svc) downloadName(ctx context.Context, job *JobRow, ext string) string {
	row, err := s.store.Projects().Get(ctx, *job.ProjectID)
	if err != nil {
		return job.ID + ext
	}

	var b strings.Builder

	dash := true

	for _, r := range row.Name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)

			dash = false
		case !dash:
			b.WriteByte('-')

			dash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if len(slug) > 60 {
		slug = strings.TrimRight(slug[:60], "-")
	}

	if slug == "" {
		return job.ID + ext
	}

	ms := job.CreatedMs
	if job.FinishedMs != nil {
		ms = *job.FinishedMs
	}

	return slug + "-" + time.UnixMilli(ms).UTC().Format("2006-01-02") + ext
}
