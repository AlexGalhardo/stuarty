package service

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

// CodeWebhookNotFound is the code of SDD-022 section 8.
const CodeWebhookNotFound = "E_WEBHOOK_NOT_FOUND"

// recentEvent is how old a state may be for the event that says it to be sent: an update of a publication that was
// settled long ago is not news.
const recentEvent = 10 * time.Minute

// Webhook is a webhook as a surface sees it (SDD-022 section 2). It never carries the signing secret.
type Webhook struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"          untrusted:"true"`
	URL          string               `json:"url"`
	Events       []string             `json:"events"`
	Enabled      bool                 `json:"enabled"`
	SecretSet    bool                 `json:"secret_set"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	LastDelivery *WebhookLastDelivery `json:"last_delivery"`
}

// WebhookLastDelivery is the newest delivery of a webhook.
type WebhookLastDelivery struct {
	ID     string    `json:"id"`
	State  string    `json:"state"`
	Status int       `json:"status"`
	At     time.Time `json:"at"`
}

// WebhookEventListRequest has no member.
type WebhookEventListRequest struct{}

// WebhookEventListResult is the events a webhook may choose, each with the sentence that explains it.
type WebhookEventListResult struct {
	Events []core.EventInfo `json:"events"`
}

// WebhookListRequest has no member.
type WebhookListRequest struct{}

// WebhookListResult is every webhook, oldest first.
type WebhookListResult struct {
	Webhooks []Webhook `json:"webhooks"`
}

// WebhookRef names a webhook.
type WebhookRef struct {
	ID string `json:"id"`
}

// WebhookCreateRequest is SDD-022 section 6. Enabled defaults to true.
type WebhookCreateRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled,omitempty"`
}

// WebhookSecretResult is a webhook with its signing secret, in the answer of the creation and of the rotation and in
// no other: the person keeps it, and the product cannot show it again (ADR-0033).
type WebhookSecretResult struct {
	Webhook
	Secret string `json:"secret"`
}

// WebhookUpdateRequest changes the members that are set.
type WebhookUpdateRequest struct {
	ID      string    `json:"id"`
	Name    *string   `json:"name,omitempty"`
	URL     *string   `json:"url,omitempty"`
	Events  *[]string `json:"events,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}

// WebhookDeliveriesRequest lists the deliveries of a webhook, newest first. Limit is 1 to 100, default 50.
type WebhookDeliveriesRequest struct {
	ID     string `json:"id"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// WebhookDelivery is a delivery with the record of its attempts. It never carries the body of an answer.
type WebhookDelivery struct {
	webhooks.Delivery

	At         time.Time          `json:"at"`
	AttemptLog []webhooks.Attempt `json:"attempt_log"`
}

// WebhookDeliveriesResult is a page of deliveries.
type WebhookDeliveriesResult struct {
	Deliveries []WebhookDelivery `json:"deliveries"`
}

// WebhookRedeliverRequest names the delivery to send again.
type WebhookRedeliverRequest struct {
	ID         string `json:"id"`
	DeliveryID string `json:"delivery_id"`
}

func webhookNotFound() *Error {
	return newError(CodeWebhookNotFound, "the webhook or the delivery does not exist.").
		withHint("list the webhooks with webhook:list")
}

func webhookError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, webhooks.ErrNotFound):
		return webhookNotFound()
	}

	return connectionError(err)
}

// webhookCaller is the scope of the method and no project-limited key: a webhook belongs to the workspace.
func (s *svc) webhookCaller(ctx context.Context, method string) (Caller, error) {
	c, err := authorize(ctx, method, "")
	if err != nil {
		return Caller{}, err
	}

	if c.ProjectID != "" {
		return Caller{}, newError(CodeForbidden, "the key is limited to one project and cannot use webhooks.").
			withHint("use a key without a project limit")
	}

	return c, nil
}

func (s *svc) webhookView(ctx context.Context, r webhooks.Row) Webhook {
	w := Webhook{
		ID: r.ID, Name: r.Name, URL: r.URL, Events: r.Events, Enabled: r.Enabled, SecretSet: true,
		CreatedAt: time.UnixMilli(r.CreatedMs).UTC(), UpdatedAt: time.UnixMilli(r.UpdatedMs).UTC(),
	}

	if d, err := s.store.Webhooks().Latest(ctx, r.ID); err == nil {
		w.LastDelivery = &WebhookLastDelivery{
			ID: d.ID, State: d.State, Status: d.LastStatus, At: time.UnixMilli(max(d.FinishedMs, d.CreatedMs)).UTC(),
		}
	}

	return w
}

// WebhookEventList returns the events a webhook may choose.
func (s *svc) WebhookEventList(ctx context.Context, _ WebhookEventListRequest) (*WebhookEventListResult, error) {
	if _, err := authorize(ctx, "WebhookEventList", ""); err != nil {
		return nil, err
	}

	return &WebhookEventListResult{Events: core.Events()}, nil
}

// WebhookList returns every webhook.
func (s *svc) WebhookList(ctx context.Context, _ WebhookListRequest) (*WebhookListResult, error) {
	if _, err := s.webhookCaller(ctx, "WebhookList"); err != nil {
		return nil, err
	}

	rows, err := s.store.Webhooks().List(ctx)
	if err != nil {
		return nil, asError(err)
	}

	out := &WebhookListResult{Webhooks: make([]Webhook, 0, len(rows))}
	for _, r := range rows {
		out.Webhooks = append(out.Webhooks, s.webhookView(ctx, r))
	}

	return out, nil
}

// WebhookGet returns one webhook.
func (s *svc) WebhookGet(ctx context.Context, req WebhookRef) (*Webhook, error) {
	if _, err := s.webhookCaller(ctx, "WebhookGet"); err != nil {
		return nil, err
	}

	r, err := s.store.Webhooks().Get(ctx, req.ID)
	if err != nil {
		return nil, webhookError(err)
	}

	w := s.webhookView(ctx, r)

	return &w, nil
}

// WebhookCreate validates a webhook, generates its secret, puts it in the vault and records the webhook, with an
// audit row that holds no address and no secret. The secret is in this answer and nowhere else.
func (s *svc) WebhookCreate(ctx context.Context, req WebhookCreateRequest) (*WebhookSecretResult, error) {
	c, err := s.webhookCaller(ctx, "WebhookCreate")
	if err != nil {
		return nil, err
	}

	name, ferr := core.NormalizeName(req.Name)
	if ferr != nil {
		return nil, webhookField(ferr)
	}

	if _, ferr = core.CheckURL(req.URL, outbound.NotWebhookAddr); ferr != nil {
		return nil, webhookField(ferr)
	}

	events, ferr := core.NormalizeEvents(req.Events)
	if ferr != nil {
		return nil, webhookField(ferr)
	}

	secret, err := core.NewSecret(rand.Reader)
	if err != nil {
		return nil, asError(err)
	}

	id, now := s.newIDs("whk"), s.nowMs()
	row := webhooks.Row{
		ID: id, Name: name, URL: req.URL, Events: events, Enabled: req.Enabled == nil || *req.Enabled,
		CreatedBy: c.Actor, CreatedMs: now, UpdatedMs: now,
	}

	if err := s.conns.PutSecret(id, []byte(secret)); err != nil {
		return nil, connectionError(err)
	}

	// The limit is counted in the transaction: two creations at once cannot both take the last place.
	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		count, err := tx.Webhooks().Count(ctx)
		if err != nil {
			return err
		}

		if count >= core.MaxWebhooks {
			return newError(
				"E_LIMIT_EXCEEDED",
				"the workspace has the most webhooks it may have (%d).",
				core.MaxWebhooks,
			).
				withHint("remove a webhook first")
		}

		if err := tx.Webhooks().Insert(ctx, row); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "webhook.create", "", id)
	})
	if werr != nil {
		_ = s.conns.DeleteSecret(id)

		return nil, asError(werr)
	}

	return &WebhookSecretResult{Webhook: s.webhookView(ctx, row), Secret: secret}, nil
}

func webhookField(f *core.FieldError) error {
	return newError(command.CodeValidation, "%s", f.Message).withPath(f.Path)
}

// WebhookUpdate changes the name, the address, the events or the state of a webhook. Never the secret.
func (s *svc) WebhookUpdate(ctx context.Context, req WebhookUpdateRequest) (*Webhook, error) {
	c, err := s.webhookCaller(ctx, "WebhookUpdate")
	if err != nil {
		return nil, err
	}

	if _, err := s.store.Webhooks().Get(ctx, req.ID); err != nil {
		return nil, webhookError(err)
	}

	var name string

	if req.Name != nil {
		n, ferr := core.NormalizeName(*req.Name)
		if ferr != nil {
			return nil, webhookField(ferr)
		}

		name = n
	}

	if req.URL != nil {
		if _, ferr := core.CheckURL(*req.URL, outbound.NotWebhookAddr); ferr != nil {
			return nil, webhookField(ferr)
		}
	}

	var events []string

	if req.Events != nil {
		list, ferr := core.NormalizeEvents(*req.Events)
		if ferr != nil {
			return nil, webhookField(ferr)
		}

		events = list
	}

	now := s.nowMs()

	var row webhooks.Row

	// The row is read in the transaction: an update that changes the name cannot put back an address another one
	// has just replaced.
	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		cur, err := tx.Webhooks().Get(ctx, req.ID)
		if err != nil {
			return err
		}

		if req.Name != nil {
			cur.Name = name
		}

		if req.URL != nil {
			cur.URL = *req.URL
		}

		if req.Events != nil {
			cur.Events = events
		}

		if req.Enabled != nil {
			cur.Enabled = *req.Enabled
		}

		cur.UpdatedMs = now
		row = cur

		if err := tx.Webhooks().Update(ctx, cur); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "webhook.update", "", cur.ID)
	})
	if werr != nil {
		return nil, webhookError(werr)
	}

	if row.Enabled && s.hooks != nil {
		s.hooks.Wake()
	}

	w := s.webhookView(ctx, row)

	return &w, nil
}

// WebhookRemove deletes the secret from the vault, then the webhook with its deliveries.
func (s *svc) WebhookRemove(ctx context.Context, req WebhookRef) (*Webhook, error) {
	c, err := s.webhookCaller(ctx, "WebhookRemove")
	if err != nil {
		return nil, err
	}

	row, err := s.store.Webhooks().Get(ctx, req.ID)
	if err != nil {
		return nil, webhookError(err)
	}

	view := s.webhookView(ctx, row)

	if err := s.conns.DeleteSecret(row.ID); err != nil {
		return nil, connectionError(err)
	}

	now := s.nowMs()

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Webhooks().Delete(ctx, row.ID); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "webhook.remove", "", row.ID)
	})
	if werr != nil {
		return nil, webhookError(werr)
	}

	return &view, nil
}

// WebhookRotateSecret replaces the signing secret and answers with the new one, once.
func (s *svc) WebhookRotateSecret(ctx context.Context, req WebhookRef) (*WebhookSecretResult, error) {
	c, err := s.webhookCaller(ctx, "WebhookRotateSecret")
	if err != nil {
		return nil, err
	}

	row, err := s.store.Webhooks().Get(ctx, req.ID)
	if err != nil {
		return nil, webhookError(err)
	}

	secret, err := core.NewSecret(rand.Reader)
	if err != nil {
		return nil, asError(err)
	}

	if err := s.conns.PutSecret(row.ID, []byte(secret)); err != nil {
		return nil, connectionError(err)
	}

	now := s.nowMs()

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "webhook.rotate_secret", "", row.ID)
	})
	if werr != nil {
		return nil, asError(werr)
	}

	return &WebhookSecretResult{Webhook: s.webhookView(ctx, row), Secret: secret}, nil
}

// WebhookTest sends one webhook.test event, one attempt, and answers with the delivery as it ended.
func (s *svc) WebhookTest(ctx context.Context, req WebhookRef) (*WebhookDelivery, error) {
	c, err := s.webhookCaller(ctx, "WebhookTest")
	if err != nil {
		return nil, err
	}

	row, err := s.store.Webhooks().Get(ctx, req.ID)
	if err != nil {
		return nil, webhookError(err)
	}

	// A test that cannot be signed is an error now, not a delivery that waits for the unlock.
	if b, err := s.conns.Secret(row.ID); err != nil {
		return nil, connectionError(err)
	} else {
		clear(b)
	}

	del, _, err := s.hooks.Enqueue(
		ctx,
		row,
		core.TestData{WebhookID: row.ID, Nonce: s.newIDs("tst")},
		s.now(),
		webhooks.KindTest,
	)
	if err != nil {
		return nil, asError(err)
	}

	now := s.nowMs()
	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "webhook.test", "", row.ID)
	})

	sent, err := s.hooks.Send(ctx, row.ID, del.ID)
	if err != nil {
		return nil, webhookError(err)
	}

	return s.deliveryView(ctx, sent)
}

func (s *svc) deliveryView(ctx context.Context, d webhooks.Delivery) (*WebhookDelivery, error) {
	log, err := s.store.Webhooks().Attempts(ctx, d.ID)
	if err != nil {
		return nil, asError(err)
	}

	if log == nil {
		log = []webhooks.Attempt{}
	}

	d.Body = nil

	return &WebhookDelivery{Delivery: d, At: time.UnixMilli(d.CreatedMs).UTC(), AttemptLog: log}, nil
}

// WebhookDeliveryList lists the deliveries of a webhook, newest first.
func (s *svc) WebhookDeliveryList(ctx context.Context, req WebhookDeliveriesRequest) (*WebhookDeliveriesResult, error) {
	if _, err := s.webhookCaller(ctx, "WebhookDeliveryList"); err != nil {
		return nil, err
	}

	if _, err := s.store.Webhooks().Get(ctx, req.ID); err != nil {
		return nil, webhookError(err)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	list, err := s.store.Webhooks().Deliveries(ctx, req.ID, limit, max(req.Offset, 0))
	if err != nil {
		return nil, asError(err)
	}

	out := &WebhookDeliveriesResult{Deliveries: make([]WebhookDelivery, 0, len(list))}

	for i := range list {
		v, err := s.deliveryView(ctx, list[i])
		if err != nil {
			return nil, err
		}

		out.Deliveries = append(out.Deliveries, *v)
	}

	return out, nil
}

// WebhookRedeliver stores a copy of a delivery, to be sent again with a fresh signature by the worker.
func (s *svc) WebhookRedeliver(ctx context.Context, req WebhookRedeliverRequest) (*WebhookDelivery, error) {
	c, err := s.webhookCaller(ctx, "WebhookRedeliver")
	if err != nil {
		return nil, err
	}

	if _, err := s.store.Webhooks().Get(ctx, req.ID); err != nil {
		return nil, webhookError(err)
	}

	src, err := s.store.Webhooks().GetDelivery(ctx, req.ID, req.DeliveryID)
	if err != nil {
		return nil, webhookError(err)
	}

	del, err := s.hooks.Redeliver(ctx, src)
	if err != nil {
		return nil, asError(err)
	}

	now := s.nowMs()
	_ = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		return appendAudit(ctx, tx, now, c.Actor, "webhook.redeliver", "", req.ID)
	})

	return s.deliveryView(ctx, del)
}

// WebhookDispatcherOf returns the dispatcher of a Service made by New; the server runs it next to the queue workers
// and the publication scheduler. A command that is not a server never does.
func WebhookDispatcherOf(s Service) (*webhooks.Dispatcher, bool) {
	v, ok := s.(*svc)
	if !ok || v.hooks == nil {
		return nil, false
	}

	return v.hooks, true
}

// webhookSecrets reads the signing secret of a webhook from the vault for the dispatcher.
type webhookSecrets struct{ s *svc }

func (w webhookSecrets) Secret(_ context.Context, id string) ([]byte, error) {
	b, err := w.s.conns.Secret(id)
	if err != nil {
		return nil, webhooks.ErrVaultLocked
	}

	return b, nil
}

// webhookEvents turns an event of the bus into the events a webhook may receive (SDD-022 section 3). It reads the
// members the event holds and copies identifiers, states and counts only.
func (s *svc) webhookEvents(e Event) []core.Data {
	switch e.Type {
	case EventPublicationUpdated:
		if e.Publication == nil {
			return nil
		}

		return s.publicationWebhookEvents(e)
	case EventApprovalRequested:
		a := e.Approval
		if a == nil {
			return nil
		}

		d := core.ApprovalRequestedData{
			ApprovalID: a.ID,
			Kind:       a.Kind,
			ExpiresAt:  a.ExpiresAt.UTC().Format(time.RFC3339),
		}
		if a.ProjectID != nil {
			d.ProjectID = *a.ProjectID
		}

		if a.JobID != nil {
			d.JobID = *a.JobID
		}

		if a.ConnectionID != nil {
			d.ConnectionID = *a.ConnectionID
		}

		return []core.Data{d}
	case "job.updated":
		switch {
		case e.State == JobSucceeded && e.Kind == JobExport && e.Result != nil && e.Result.Export != nil:
			x := e.Result.Export

			return []core.Data{core.ExportReadyData{
				JobID: e.JobID, ProjectID: e.ProjectID, Kind: e.Kind, Format: x.Format, Bytes: x.Bytes, Width: x.Width,
				Height: x.Height,
			}}
		case e.State == JobSucceeded && e.Kind == JobImageExport && e.Result != nil && e.Result.Image != nil:
			x := e.Result.Image

			return []core.Data{core.ExportReadyData{
				JobID: e.JobID, Kind: e.Kind, Format: x.Format, Bytes: x.Bytes, Width: x.Width, Height: x.Height,
			}}
		case e.State == JobFailed:
			d := core.JobFailedData{JobID: e.JobID, ProjectID: e.ProjectID, Kind: e.Kind, State: e.State}
			if e.Error != nil {
				d.ErrorCode = e.Error.Code
			}

			return []core.Data{d}
		}
	}

	return nil
}

func (s *svc) publicationWebhookEvents(e Event) []core.Data {
	var out []core.Data

	now := s.now()
	p := e.Publication

	for i := range p.Targets {
		t := &p.Targets[i]

		if t.State == corepub.TargetPublished && t.PublishedAt != nil && now.Sub(*t.PublishedAt) <= recentEvent {
			out = append(out, core.PublicationPublishedData{
				PublicationID: p.ID, TargetID: t.ID, ProjectID: p.ProjectID, Network: t.Network,
				ConnectionID: t.ConnectionID, URL: core.PublicPostURL(t.URL, corepub.Visibility(t.Fields.Visibility)),
				PublishedAt: t.PublishedAt.UTC().Format(time.RFC3339),
			})
		}

		if t.State == corepub.TargetFailed && now.Sub(t.UpdatedAt) <= recentEvent {
			d := core.PublicationFailedData{
				PublicationID: p.ID,
				TargetID:      t.ID,
				ProjectID:     p.ProjectID,
				Network:       t.Network,
				ConnectionID:  t.ConnectionID,
				ApprovalID:    t.ApprovalID,
			}
			if t.Error != nil {
				d.ErrorKind = t.Error.Kind
			}

			out = append(out, d)
		}
	}

	return out
}

// fanWebhooks makes the deliveries of an event for the webhooks that chose it. It writes rows and sends nothing.
func (s *svc) fanWebhooks(e Event) {
	if s.hooks == nil {
		return
	}

	for _, d := range s.webhookEvents(e) {
		s.hooks.Publish(context.Background(), d, s.now())
	}
}
