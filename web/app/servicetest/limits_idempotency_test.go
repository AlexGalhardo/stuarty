package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-005 section 10 and SDD-007 section 9.1: at 10,000 valid keys for one actor and project a new
// keyed request is refused and nothing is evicted, so no key already handed out stops replaying.
func TestLimitIdempotencyKeysAreRefusedNotEvicted(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project, track := m.withTrack()
	ctx := context.Background()
	now := m.head(project).UpdatedMs

	err := m.store.Write(ctx, func(ctx context.Context, tx service.Tx) error {
		for i := range 10000 {
			row := service.IdempotencyRow{
				ProjectID: project, Actor: ui, Key: fmt.Sprintf("seed-%08d", i), RequestHash: "h", Revision: 1,
				Result: json.RawMessage(`{}`), CreatedMs: now, ExpiresMs: now + 86400000,
			}
			if err := tx.Idempotency().Put(ctx, row); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	req := service.EditRequest{
		ProjectID:      project,
		BaseRevision:   m.head(project).Revision,
		IdempotencyKey: "one-too-many",
		Operations: []command.Operation{
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"0s","duration":"1s","text":"x"}`, track)),
		},
	}

	_, err = m.svc.Edit(m.as(ui), req)
	se := requireLimit(t, "key 10,001", err, service.CodeRateLimited, "idempotency_keys")

	if se.Details["retry_after"] == nil {
		t.Errorf("details = %v, want retry_after", se.Details)
	}

	if m.head(project).Revision != req.BaseRevision {
		t.Error("a refused keyed request moved the project")
	}

	if _, err := m.store.Idempotency().Get(ctx, project, ui, "seed-00000000", now); err != nil {
		t.Errorf("the oldest key was evicted: %v", err)
	}

	// Another actor has its own 10,000, and a request without a key is not counted.
	req.IdempotencyKey = "other-actor"
	if _, err := m.svc.Edit(m.as(agent), req); err != nil {
		t.Errorf("another actor: %v", err)
	}

	req.IdempotencyKey, req.BaseRevision = "", m.head(project).Revision
	req.Operations = []command.Operation{
		op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"5s","duration":"1s","text":"y"}`, track)),
	}

	if _, err := m.svc.Edit(m.as(ui), req); err != nil {
		t.Errorf("an edit without a key: %v", err)
	}
}
