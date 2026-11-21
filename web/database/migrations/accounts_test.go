package migrations

import (
	"context"
	"testing"
)

// The account table (SDD-005 section 5.16): one row at most, and an idle lock inside its range.
func TestAccountsTable(t *testing.T) {
	db := openMigrated(t)
	ctx := context.Background()

	insert := func(id, idle int) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO accounts (id, username, email, kdf, verifier, key_by_password, key_by_recovery, vault_key,
 idle_lock_minutes, created_ms, updated_ms) VALUES (?, 'stuart', 'stuart@example.com', '{}', x'00', x'00', x'00', x'00',
 ?, 1, 1)`, id, idle)

		return err
	}

	if err := insert(1, 0); err != nil {
		t.Fatal(err)
	}

	for name, err := range map[string]error{
		"a second row":          insert(2, 0),
		"the same row again":    insert(1, 0),
		"an idle lock below 0":  insert(1, -1),
		"an idle lock too long": insert(1, 1441),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	if n := count(t, db, `SELECT count(*) FROM accounts`); n != 1 {
		t.Errorf("%d rows", n)
	}
}
