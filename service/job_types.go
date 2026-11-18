package service

import (
	"encoding/json"
	"time"
)

// Job kinds (SDD-000 section 6).
const (
	JobIngest = "ingest"
	JobExport = "export"
	JobFrame  = "frame"
)

// Job states.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// Codes that describe the outcome of a job (SDD-000 section 8); they travel
// inside the Job, not as the error of a call.
const (
	CodeIngestFailed = "E_INGEST_FAILED"
	CodeExportFailed = "E_EXPORT_FAILED"
	CodeVerifyFailed = "E_VERIFY_FAILED"
	CodeCancelled    = "E_CANCELLED"
)

// JobStartRequest starts a job of a project; only the kind "export" exists in
// this build.
type JobStartRequest struct {
	ProjectID string          `json:"project_id"`
	Kind      string          `json:"kind"`
	Args      json.RawMessage `json:"args,omitempty"`
	WaitMS    int             `json:"wait_ms,omitempty"`
	DryRun    bool            `json:"dry_run,omitempty"`
}

// ExportArgs are the arguments of an export job.
type ExportArgs struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
	// Format is mp4 (the default), gif, wav or aac (SDD-004 section 8.1).
	Format string `json:"format,omitempty"`
	// At is the time of the frame of a png export, "0s" when absent; only for format png (VV-037).
	At string `json:"at,omitempty"`
}

// JobGetRequest reads a job.
type JobGetRequest struct {
	JobID  string `json:"job_id"`
	WaitMS int    `json:"wait_ms,omitempty"`
}

// JobCancelRequest asks for the cancellation of a job.
type JobCancelRequest struct {
	JobID string `json:"job_id"`
}

// JobListRequest lists the jobs of a project. State empty is the queued and
// running jobs plus the 10 most recent final ones; "active", "all" and each
// state filter.
type JobListRequest struct {
	ProjectID string `json:"project_id"`
	State     string `json:"state,omitempty"`
	Page
}

// JobListResult is a page of jobs, newest first.
type JobListResult struct {
	ProjectID  string  `json:"project_id"`
	Jobs       []Job   `json:"jobs"`
	NextCursor *string `json:"next_cursor"`
	Text       string  `json:"text"`
}

// VerifyCheck is one check of the verification of an export (ADR-0018 item 6).
type VerifyCheck struct {
	Name   string `json:"name"` // streams, duration, dimensions, pixel_format, faststart
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// ExportResult is the result of a succeeded export (SDD-003 section 2.8).
type ExportResult struct {
	Revision     int64         `json:"revision"`
	Format       string        `json:"format,omitempty"` // mp4, gif, wav or aac; empty on a result stored before formats
	Path         string        `json:"path,omitempty"`   // absolute; local callers only
	DownloadURL  string        `json:"download_url"`
	Bytes        int64         `json:"bytes"`
	SHA256       string        `json:"sha256,omitempty"`
	Duration     TimeValue     `json:"duration"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	FPS          string        `json:"fps"`
	VideoCodec   string        `json:"video_codec"`
	AudioCodec   string        `json:"audio_codec"`
	LoudnessLUFS *float64      `json:"loudness_lufs,omitempty"`
	Checks       []VerifyCheck `json:"checks"`
}

// JobResult has exactly one member set.
type JobResult struct {
	Export     *ExportResult     `json:"export,omitempty"`
	Transcribe *TranscribeResult `json:"transcribe,omitempty"`
}

// Job is the job object of SDD-003 section 2.8.
type Job struct {
	JobID     string      `json:"job_id"`
	Kind      string      `json:"kind"`
	ProjectID *string     `json:"project_id"` // null for an ingest job
	State     string      `json:"state"`
	Progress  float64     `json:"progress"`
	Message   string      `json:"message,omitempty"`
	Result    *JobResult  `json:"result,omitempty"`
	Error     *Error      `json:"error,omitempty"`
	Lint      *LintTotals `json:"lint,omitempty"`
	// Warnings are notes about an accepted export that are not a reason to refuse it, such as a huge GIF.
	Warnings   []string   `json:"warnings,omitempty"`
	Actor      string     `json:"actor"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Text       string     `json:"text"`
}

// exportInput is the input column of an export job.
type exportInput struct {
	Revision int64       `json:"revision"`
	Start    int64       `json:"start_frame"`
	End      int64       `json:"end_frame"`
	Lint     *LintTotals `json:"lint,omitempty"`
	// Warnings are the notes of the start (VV-056), shown on the job.
	Warnings []string `json:"warnings,omitempty"`
	// Document is the canonical document of Revision, kept with the job so
	// that the export renders what was asked for even when edits land while
	// the job waits (SDD-003 section 2.8 rule 9).
	Document json.RawMessage `json:"document,omitempty"`
	// Format is "bundle" for the export of a .vvbundle (SDD-005 section 16); empty is a video.
	Format string `json:"format,omitempty"`
	// IncludeLog false leaves the operation log out of a bundle; nil means true.
	IncludeLog *bool `json:"include_log,omitempty"`
}
