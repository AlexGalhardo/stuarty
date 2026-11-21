// Package webhooks is the delivery side of the outbound webhooks (SDD-022): the records, the store they live in, and
// the dispatcher that turns an event into deliveries and sends them on a bounded worker, with retries and a record of
// every attempt. It knows nothing of the service that owns it: the secret is read through a small interface and the
// client is built by internal/outbound, the only place that opens a connection (SDD-007 section 10.2).
package webhooks

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Errors of the store and the secret.
var (
	// ErrNotFound is a webhook or a delivery that does not exist.
	ErrNotFound = errors.New("webhooks: not found")
	// ErrVaultLocked is a secret that cannot be read now: the vault cannot be opened or the secret is not in it.
	ErrVaultLocked = errors.New("webhooks: the vault cannot be opened")
)

// Row is a webhook as the table holds it.
type Row struct {
	ID        string
	Name      string
	URL       string
	Events    []string
	Enabled   bool
	CreatedBy string
	CreatedMs int64
	UpdatedMs int64
}

// Delivery is one send of one event to one webhook.
type Delivery struct {
	ID        string `json:"id"`
	WebhookID string `json:"webhook_id"`
	Event     string `json:"event"`
	EventID   string `json:"event_id"`
	// Kind is event, test or redelivery.
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"max_attempts"`
	LastStatus  int    `json:"last_status"`
	LastError   string `json:"last_error,omitempty"`
	// Body is the stored JSON body, metadata only (core/webhook.Check). It is not part of a list.
	Body       []byte `json:"-"`
	NextMs     int64  `json:"-"`
	CreatedMs  int64  `json:"-"`
	FinishedMs int64  `json:"-"`
}

// Attempt is the record of one attempt.
type Attempt struct {
	N             int    `json:"n"`
	AtMs          int64  `json:"-"`
	Status        int    `json:"status"`
	DurationMs    int64  `json:"duration_ms"`
	Error         string `json:"error,omitempty"`
	ResponseBytes int    `json:"response_bytes"`
}

// Store is the table of webhooks, their deliveries and their attempts (SDD-005 section 5.16). On a transaction it is
// bound to it.
type Store interface {
	Count(ctx context.Context) (int, error)
	List(ctx context.Context) ([]Row, error)
	// Get returns one, or ErrNotFound.
	Get(ctx context.Context, id string) (Row, error)
	Insert(ctx context.Context, r Row) error
	Update(ctx context.Context, r Row) error
	// Delete removes the webhook, its deliveries and their attempts.
	Delete(ctx context.Context, id string) error

	// Enqueue stores a delivery. It answers false, and stores nothing, when the webhook already has a delivery of
	// this event id (a test send excepted).
	Enqueue(ctx context.Context, d Delivery) (bool, error)
	// Due lists the pending deliveries of enabled webhooks whose time came, oldest first.
	Due(ctx context.Context, nowMs int64, limit int) ([]Delivery, error)
	// NextDue is the time of the next pending delivery of an enabled webhook, or 0 when there is none.
	NextDue(ctx context.Context) (int64, error)
	// GetDelivery returns a delivery of a webhook with its body, or ErrNotFound.
	GetDelivery(ctx context.Context, webhookID, id string) (Delivery, error)
	// Deliveries lists the deliveries of a webhook, newest first, with no body.
	Deliveries(ctx context.Context, webhookID string, limit, offset int) ([]Delivery, error)
	// Latest returns the newest delivery of a webhook, or ErrNotFound.
	Latest(ctx context.Context, webhookID string) (Delivery, error)
	// Attempts lists the attempts of a delivery, oldest first.
	Attempts(ctx context.Context, deliveryID string) ([]Attempt, error)
	// Record stores an attempt and the new state of its delivery in one step, keeps the last core.KeepAttempts attempts
	// of the webhook, and deletes the deliveries finished before oldestMs.
	Record(ctx context.Context, d Delivery, a Attempt, oldestMs int64) error
	// Reschedule moves the time of a pending delivery without counting an attempt.
	Reschedule(ctx context.Context, id string, nextMs int64, reason string) error
}

// Secrets reads the signing secret of a webhook. An error that means the vault cannot be opened, or that the secret
// is not there, is ErrVaultLocked.
type Secrets interface {
	Secret(ctx context.Context, id string) ([]byte, error)
}

// Doer is what sends a request. internal/outbound builds the real one.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Clock is the time the dispatcher reads and waits on; tests move it.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
