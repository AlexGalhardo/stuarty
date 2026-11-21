package migrations

import "github.com/AlexGalhardo/vukem/schema"

// The outbound webhooks (SDD-022, SDD-005 section 5.16): the webhook, its deliveries with the stored metadata body,
// and the record of each attempt. No table holds the signing secret, which is in the vault, and none holds the body of
// an answer.
func m20261121110000CreateWebhookTables() schema.Migration {
	return schema.Migration{
		ID: "20261121110000_create_webhook_tables",
		Up: func(s *schema.Schema) error {
			stmts := []string{
				`CREATE TABLE webhooks (
  id          TEXT    NOT NULL,
  name        TEXT    NOT NULL,
  url         TEXT    NOT NULL,
  events      TEXT    NOT NULL,
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_by  TEXT    NOT NULL,
  created_ms  INTEGER NOT NULL,
  updated_ms  INTEGER NOT NULL,
  PRIMARY KEY (id)
)`,
				`CREATE TABLE webhook_deliveries (
  id            TEXT    NOT NULL,
  webhook_id    TEXT    NOT NULL,
  event         TEXT    NOT NULL,
  event_id      TEXT    NOT NULL,
  kind          TEXT    NOT NULL CHECK (kind IN ('event','test','redelivery')),
  body          TEXT    NOT NULL,
  state         TEXT    NOT NULL CHECK (state IN ('pending','succeeded','failed')),
  max_attempts  INTEGER NOT NULL,
  attempts      INTEGER NOT NULL DEFAULT 0,
  next_ms       INTEGER NOT NULL,
  last_status   INTEGER NOT NULL DEFAULT 0,
  last_error    TEXT    NOT NULL DEFAULT '',
  created_ms    INTEGER NOT NULL,
  finished_ms   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  FOREIGN KEY (webhook_id) REFERENCES webhooks (id) ON DELETE CASCADE
)`,
				`CREATE INDEX webhook_deliveries_due_index ON webhook_deliveries (state, next_ms)`,
				`CREATE INDEX webhook_deliveries_webhook_index ON webhook_deliveries (webhook_id, created_ms)`,
				`CREATE UNIQUE INDEX webhook_deliveries_event_unique ON webhook_deliveries (webhook_id, event_id) WHERE kind = 'event'`,
				`CREATE TABLE webhook_attempts (
  delivery_id     TEXT    NOT NULL,
  n               INTEGER NOT NULL,
  at_ms           INTEGER NOT NULL,
  status          INTEGER NOT NULL,
  duration_ms     INTEGER NOT NULL,
  error           TEXT    NOT NULL DEFAULT '',
  response_bytes  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (delivery_id, n),
  FOREIGN KEY (delivery_id) REFERENCES webhook_deliveries (id) ON DELETE CASCADE
)`,
			}

			for _, q := range stmts {
				if err := s.Raw(q); err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(s *schema.Schema) error {
			for _, t := range []string{"webhook_attempts", "webhook_deliveries", "webhooks"} {
				if err := s.DropIfExists(t); err != nil {
					return err
				}
			}

			return nil
		},
	}
}
