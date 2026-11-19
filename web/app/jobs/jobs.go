// Package jobs connects the job machinery of service/ to the vukem queue
// (SDD-005 section 14.1): the queue handlers that deliver a job id to the
// service, and the Enqueuer that inserts the message in the transaction that
// created the job row. It holds no rule about jobs; the claim, the progress and
// the outcome are service.JobRunner.
package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/database"
	"github.com/AlexGalhardo/vukem/queue"
)

// Queue names, in the strict priority order of QUEUE_NAMES.
const (
	QueueIngest  = "ingest"
	QueueExport  = "export"
	QueueDefault = "default"
)

// Queues is the value of QUEUE_NAMES.
var Queues = []string{QueueIngest, QueueExport, QueueDefault}

// Timeouts are the per-run ceilings of SDD-007 section 6.5 plus 10 minutes, so
// that the queue never kills a job that its own deadline still allows
// (SDD-005 section 14.1).
const (
	IngestTimeout = 4*time.Hour + 10*time.Minute
	ExportTimeout = 2*time.Hour + 10*time.Minute
	FrameTimeout  = 2 * time.Minute
	// BatchTimeout is the ceiling of the parent of an export batch (SDD-004 section 8.4): up to 12 exports
	// one after the other, each under its own ExportTimeout and deadline.
	BatchTimeout = 24 * time.Hour
)

// Payload is the whole payload of a message: the id of the job row.
type Payload struct {
	JobID string `json:"job_id"`
}

// Registrations are the handlers of video.ingest, video.export, video.export_batch and
// video.frame. Retries are off: a failure ends the job, it is never silently
// retried (SDD-011 section 7.2).
func Registrations(r service.JobRunner) []queue.Registration {
	run := func(ctx context.Context, p Payload) error { return r.RunJob(ctx, p.JobID) }

	return []queue.Registration{
		queue.HandleKind(
			service.QueueKindIngest,
			run,
			queue.Retries(0),
			queue.OnQueue(QueueIngest),
			queue.Timeout(IngestTimeout),
		),
		queue.HandleKind(
			service.QueueKindExport,
			run,
			queue.Retries(0),
			queue.OnQueue(QueueExport),
			queue.Timeout(ExportTimeout),
		),
		queue.HandleKind(
			service.QueueKindExportBatch,
			run,
			queue.Retries(0),
			queue.OnQueue(QueueDefault),
			queue.Timeout(BatchTimeout),
		),
		queue.HandleKind(
			service.QueueKindFrame,
			run,
			queue.Retries(0),
			queue.OnQueue(QueueDefault),
			queue.Timeout(FrameTimeout),
		),
	}
}

// executor is what the transactions of web/app/store give.
type executor interface{ Executor() database.Executor }

// Dispatcher is the service.Enqueuer over a vukem queue. On the database driver
// the message is inserted in the transaction of the job row; on queue.Sync the
// job runs after the commit, inside the call.
type Dispatcher struct{ Queue *queue.Queue }

var _ service.Enqueuer = Dispatcher{}

// Enqueue implements service.Enqueuer.
func (d Dispatcher) Enqueue(ctx context.Context, tx service.Tx, kind, jobID string) error {
	ex, ok := tx.(executor)
	if !ok {
		return errors.New("jobs: the transaction does not expose its executor")
	}

	return d.Queue.DispatchKindTx(ctx, ex.Executor(), kind, Payload{JobID: jobID})
}

// NewQueue builds a queue on a driver with the three handlers registered.
func NewQueue(driver queue.Driver, r service.JobRunner) (*queue.Queue, error) {
	q := queue.New(driver, nil)
	if err := q.Register(Registrations(r)...); err != nil {
		return nil, err
	}

	return q, nil
}
