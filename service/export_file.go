package service

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
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
			Path:        path.Join("exports", *job.ProjectID, bundleFileName(res.Export.Revision, job.ID)),
			ContentType: "application/zip",
			ETag:        exportETag(&job),
			Immutable:   true,
			MaxAge:      31536000,
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
		Path:        path.Join("exports", *job.ProjectID, job.ID+f.ext),
		ContentType: f.contentType,
		ETag:        exportETag(&job),
		Immutable:   true,
		MaxAge:      31536000,
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
