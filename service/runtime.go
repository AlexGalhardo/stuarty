package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/frame"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service/pathguard"
)

// Kinds of the queue messages (SDD-005 section 14.1). The payload of each is
// {"job_id": "job_..."} and nothing else.
const (
	QueueKindIngest = "video.ingest"
	QueueKindExport = "video.export"
	QueueKindFrame  = "video.frame"
	// QueueKindExportBatch is the parent job of an export batch (SDD-004 section 8.4). It runs on the default
	// queue: it holds no encoder, its children are exports that take their own slots.
	QueueKindExportBatch = "video.export_batch"
)

// Enqueuer hands a job to the workers. Enqueue runs inside the write
// transaction that created the job row, so that both exist or neither does;
// an implementation on a transactional queue inserts its message through the
// transaction, one on an inline queue runs the job after the commit.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx Tx, kind, jobID string) error
}

// Ingester runs the ingest of one stored original; ingest.Run is the default.
type Ingester func(ctx context.Context, in ingest.Input) (*ingest.Result, error)

// ExportInput is one export for an Exporter.
type ExportInput struct {
	JobID string
	Doc   *timeline.Document
	// Nested are the timelines the compositions of Doc use, by id, as they were when the export started.
	Nested map[string]*timeline.Document
	// OutPath is the final file. The exporter writes it only after the file
	// passed its verification, and leaves nothing there otherwise.
	OutPath    string
	Start, End int64 // frames; End 0 is the end of the timeline
	// Format is mp4, gif, wav or aac; empty is mp4.
	Format string
	// MaxBytes is STUARTY_MAX_EXPORT_BYTES: the encoder stops at this size
	// instead of filling the disk first; 0 means no ceiling.
	MaxBytes int64
	// Preset is the id of a platform preset and Codec the codec asked for (h264, hevc or av1); both are for
	// mp4 (SDD-004 sections 8.2 and 8.3). Empty is none.
	Preset, Codec string
	// OnProgress receives the frames written and the total.
	OnProgress func(done, total int)
}

// ExportOutput is what a verified export produced.
type ExportOutput struct {
	Bytes        int64
	SHA256       string
	DurationMS   int64
	Frames       int
	VideoCodec   string
	AudioCodec   string
	LoudnessLUFS *float64
	// CodecFallback is the codec the preset preferred when the video is H.264 instead, and Notes say so.
	CodecFallback string
	Notes         []string
	// Width and Height are the pixels of the file when they differ from the project's
	// (a GIF is capped); zero means the project's size.
	Width, Height int
	// Checks are the verification checks that ran. The job succeeds only when
	// every one is ok and the file is on disk with the size reported.
	Checks []VerifyCheck
}

// Exporter renders and verifies a project. An error that is an
// *ffmpeg.Error carries its code (E_EXPORT_FAILED, E_VERIFY_FAILED,
// E_LINT_BLOCKING, ...) into the job.
type Exporter interface {
	Export(ctx context.Context, in ExportInput) (*ExportOutput, error)
}

// Limits are the ceilings of SDD-007 section 9.1 that this part of the
// service enforces. A zero field takes its default.
type Limits struct {
	MaxImportBytes     int64 // STUARTY_MAX_IMPORT_BYTES
	MinFreeBytes       int64 // STUARTY_MIN_FREE_BYTES
	MaxIngests         int   // STUARTY_MAX_INGESTS: ingest jobs running at once
	MaxQueuedIngests   int   // ingest jobs waiting
	MaxExports         int   // STUARTY_MAX_EXPORTS: exports queued or running
	MaxExportsPerActor int   // STUARTY_MAX_EXPORTS_PER_KEY: running; two more may wait
	// MaxTranscribes is STUARTY_MAX_TRANSCRIBES: transcriptions running at once.
	MaxTranscribes int
	// MaxTranscribesPerActor is STUARTY_MAX_TRANSCRIBES_PER_KEY: the
	// transcriptions of one actor that are queued or running.
	MaxTranscribesPerActor int
	// MaxTranscribeSeconds is STUARTY_MAX_TRANSCRIBE_SECONDS: the longest audio of one job.
	MaxTranscribeSeconds int64
	// MaxExportBytes is STUARTY_MAX_EXPORT_BYTES: the largest exported file.
	MaxExportBytes int64
	Ceilings       ffmpeg.Ceilings
}

const (
	defaultMaxImportBytes   = 8 << 30
	defaultMinFreeBytes     = 2 << 30
	defaultMaxIngests       = 2
	defaultMaxQueuedIngests = 50
	defaultMaxExports       = 2
	defaultMaxTranscribes   = 1
	defaultTranscribeSecs   = 7200
	defaultMaxExportBytes   = 4 << 30
	queuedPerActor          = 2
)

// WithDefaults is the limits with every unset field at its default; the
// workers of the owner size themselves from it.
func (l Limits) WithDefaults() Limits { return l.withDefaults() }

func (l Limits) withDefaults() Limits {
	if l.MaxImportBytes <= 0 {
		l.MaxImportBytes = defaultMaxImportBytes
	}

	if l.MinFreeBytes < 0 {
		l.MinFreeBytes = 0
	} else if l.MinFreeBytes == 0 {
		l.MinFreeBytes = defaultMinFreeBytes
	}

	if l.MaxIngests <= 0 {
		l.MaxIngests = defaultMaxIngests
	}

	if l.MaxQueuedIngests <= 0 {
		l.MaxQueuedIngests = defaultMaxQueuedIngests
	}

	if l.MaxExports <= 0 {
		l.MaxExports = defaultMaxExports
	}

	if l.MaxExportsPerActor <= 0 {
		l.MaxExportsPerActor = 1
	}

	if l.MaxTranscribes <= 0 {
		l.MaxTranscribes = defaultMaxTranscribes
	}

	if l.MaxTranscribesPerActor <= 0 {
		l.MaxTranscribesPerActor = 1
	}

	if l.MaxTranscribeSeconds <= 0 {
		l.MaxTranscribeSeconds = defaultTranscribeSecs
	}

	if l.MaxExportBytes <= 0 {
		l.MaxExportBytes = defaultMaxExportBytes
	}

	return l
}

// Media is what the import and the ingest need from the process: the content
// store, the path guard and the FFmpeg runner. A zero Media makes MediaImport
// answer E_INTERNAL; the tests of the other operations never set it.
type Media struct {
	Content *enginestore.Store
	Guard   *pathguard.Guard
	Runner  *ffmpeg.Runner
	// FFmpegMissing is Discover's error when ffmpeg or ffprobe cannot be used.
	// An ingest or an export returns it; editing does not read it.
	FFmpegMissing error
	Caps          *ffmpeg.Capabilities
	Ingest        Ingester
	// Frames renders the frames and sheets of ViewFrame and ViewSheet.
	Frames *frame.Frames
	// FreeBytes reports the free space of the volume of a directory.
	FreeBytes func(dir string) (uint64, error)
}

// JobRunner is the worker side of the service: the queue handlers and the
// start-up recovery call it. It is not an operation of the Service interface,
// because no caller of the API may drive a job by hand.
type JobRunner interface {
	// RunJob runs the job whose message was delivered. It returns nil when the
	// job was finished, was taken by another delivery or does not exist, and
	// the context error when a shutdown interrupted it (the job is requeued).
	RunJob(ctx context.Context, jobID string) error
	// Recover is the start-up recovery of SDD-005 section 14.5.
	Recover(ctx context.Context) error
	// Cancel asks a job to stop (SDD-005 section 14.4) and returns when the
	// request is recorded; a queued job is cancelled at once, a running one
	// as soon as its context ends.
	Cancel(ctx context.Context, jobID string) error
}

// RunnerOf returns the JobRunner of a Service made by New.
func RunnerOf(s Service) (JobRunner, bool) {
	impl, ok := s.(*svc)

	return impl, ok
}

// registry is the in-process map of the running jobs to their cancel
// functions (SDD-005 section 14.4 step 3).
type registry struct {
	mu sync.Mutex
	m  map[string]context.CancelFunc
}

func (r *registry) add(id string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.m == nil {
		r.m = map[string]context.CancelFunc{}
	}

	r.m[id] = cancel
}

func (r *registry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.m, id)
}

func (r *registry) cancel(id string) {
	r.mu.Lock()
	cancel := r.m[id]
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// errorJSON is the typed error object stored on a job or an asset.
func errorJSON(e *Error) json.RawMessage {
	b, err := json.Marshal(e)
	if err != nil {
		return marshalOrNull(&Error{Code: command.CodeInternal, Message: codeInternalMessage})
	}

	return b
}

func parseError(raw json.RawMessage) *Error {
	if len(raw) == 0 {
		return nil
	}

	var e Error

	ok := json.Unmarshal(raw, &e) == nil && e.Code != ""
	if !ok {
		return nil
	}

	return &e
}

// fromEngine turns the typed errors of the engine and of the path guard into
// a service error. Anything else is E_INTERNAL without its text, which may hold
// paths.
func fromEngine(err error) *Error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return newError(CodeCancelled, "the job was cancelled.")
	}

	if pe, ok := errors.AsType[*pathguard.Error](err); ok {
		return &Error{Code: pe.Code, Message: pe.Message, Hint: pe.Hint}
	}

	if fe, ok := errors.AsType[*ffmpeg.Error](err); ok {
		out := &Error{Code: fe.Code, Message: fe.Message, Hint: fe.Hint}
		out.Details = withReason(fe.Details, fe.Reason)

		return out
	}

	if se, ok := errors.AsType[*enginestore.Error](err); ok {
		return &Error{Code: se.Code, Message: se.Message, Details: withReason(se.Details, se.Reason)}
	}

	if se, ok := errors.AsType[*Error](err); ok {
		return se
	}

	if full, ok := errors.AsType[*QueueFullError](err); ok {
		return queueFullError(full)
	}

	if ce, ok := errors.AsType[*command.Error](err); ok {
		return fromCommand(ce)
	}

	return internalError()
}

func withReason(details map[string]any, reason string) map[string]any {
	if reason == "" {
		return details
	}

	out := make(map[string]any, len(details)+1)
	maps.Copy(out, details)

	out["reason"] = reason

	return out
}
