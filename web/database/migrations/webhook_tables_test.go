package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

// The webhook tables (SDD-005 section 5.16): an old workspace gains them with its rows untouched, a webhook takes its
// deliveries and attempts with it, one event is one delivery of a webhook, and Down leaves the workspace as it was.
func TestWebhookTablesUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	const id = "20261121110000_create_webhook_tables"

	var upTo []schema.Migration

	for _, m := range allMigrations() {
		if m.ID < id {
			upTo = append(upTo, m)
		}
	}

	old, err := schema.NewMigrator(db, upTo)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := old.Up(ctx); err != nil {
		t.Fatal(err)
	}

	execAll(
		t,
		db,
		`INSERT INTO projects (id, name, created_ms, updated_ms, created_by) VALUES ('prj_0000000001', 'Old', 1, 1, 'u')`,
	)

	full, err := schema.NewMigrator(db, allMigrations())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatal(err)
	}

	count := func(q string) int {
		t.Helper()

		var n int
		if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}

		return n
	}

	if n := count(`SELECT count(*) FROM projects`); n != 1 {
		t.Fatalf("%d projects after the upgrade", n)
	}

	const delivery = `INSERT INTO webhook_deliveries (id, webhook_id, event, event_id, kind, body, state, max_attempts, next_ms,
 created_ms) VALUES `

	execAll(t, db,
		`INSERT INTO webhooks (id, name, url, events, created_by, created_ms, updated_ms)
 VALUES ('whk_0000000001', 'n', 'https://hooks.example.com/h', '["job.failed"]', 'u', 1, 1)`,
		delivery+`('wdl_0000000001', 'whk_0000000001', 'job.failed', 'evt_1', 'event', '{}', 'pending', 6, 1, 1)`,
		// A redelivery has the id of the event it repeats.
		delivery+`('wdl_0000000002', 'whk_0000000001', 'job.failed', 'evt_1', 'redelivery', '{}', 'pending', 6, 1, 1)`,
		`INSERT INTO webhook_attempts (delivery_id, n, at_ms, status, duration_ms) VALUES ('wdl_0000000001', 1, 1, 500, 3)`,
	)

	if _, err := db.ExecContext(ctx, delivery+
		`('wdl_0000000003', 'whk_0000000001', 'job.failed', 'evt_1', 'event', '{}', 'pending', 6, 1, 1)`); err == nil {
		t.Error("one event made two deliveries of one webhook")
	}

	execAll(t, db, `DELETE FROM webhooks WHERE id = 'whk_0000000001'`)

	if d, a := count(`SELECT count(*) FROM webhook_deliveries`), count(`SELECT count(*) FROM webhook_attempts`); d != 0 ||
		a != 0 {
		t.Fatalf("a removed webhook left %d deliveries and %d attempts", d, a)
	}

	// Down of this migration and of every one made after it, newest first.
	var want []string

	for _, m := range allMigrations() {
		if m.ID >= id {
			want = append([]string{m.ID}, want...)
		}
	}

	rolled, err := full.Rollback(ctx, len(want))
	if err != nil || !slices.Equal(rolled, want) {
		t.Fatalf("Rollback = %v, %v; want %v", rolled, err, want)
	}

	if n := count(`SELECT count(*) FROM sqlite_master WHERE name LIKE 'webhook%'`); n != 0 {
		t.Fatalf("Down left %d webhook objects", n)
	}

	if n := count(`SELECT count(*) FROM projects`); n != 1 {
		t.Fatalf("%d projects after Down", n)
	}

	if _, err := full.Up(ctx); err != nil {
		t.Fatalf("Up after Down: %v", err)
	}
}
