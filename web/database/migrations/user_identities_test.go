package migrations

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem/schema"
)

const userIdentitiesID = "20261119160000_create_user_identities_and_auth_settings"

// The tables of the optional Google sign-in (SDD-014 section 12.3). The id sorts before migrations that other
// epics merged earlier, so a workspace that already ran those still has it pending: it must run at the next
// start all the same, and its down must leave the rest as it was.
func TestUserIdentitiesMigrationRunsAfterLaterOnes(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)

	without := slices.DeleteFunc(allMigrations(), func(m schema.Migration) bool { return m.ID == userIdentitiesID })
	if len(without) != len(allMigrations())-1 {
		t.Fatalf("the migration %s is not registered", userIdentitiesID)
	}

	before, err := schema.NewMigrator(db, without)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := before.Up(ctx); err != nil {
		t.Fatal(err)
	}

	m, err := newMigrator(db)
	if err != nil {
		t.Fatal(err)
	}

	ran, err := m.Up(ctx)
	if err != nil || !slices.Equal(ran, []string{userIdentitiesID}) {
		t.Fatalf("Up = %v, %v; want only %s", ran, err, userIdentitiesID)
	}

	want := map[string][]string{
		"user_identities": {
			"provider TEXT 1 - 1", "subject TEXT 1 - 2", "user_id TEXT 1 - 0", "email TEXT 1 - 0",
			"created_ms INTEGER 1 - 0",
		},
		"auth_settings": {"key TEXT 1 - 1", "value TEXT 1 - 0", "updated_ms INTEGER 1 - 0"},
	}

	for table, cols := range want {
		if got := columns(t, db, table); !slices.Equal(got, cols) {
			t.Errorf("%s columns = %v, want %v", table, got, cols)
		}
	}

	const index = `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'user_identities_user_provider'`
	if n := count(t, db, index); n != 1 {
		t.Errorf("the unique index of (user_id, provider) is missing")
	}

	if rolled, err := m.Rollback(ctx, 1); err != nil || !slices.Equal(rolled, []string{userIdentitiesID}) {
		t.Fatalf("Rollback = %v, %v; want only %s", rolled, err, userIdentitiesID)
	}

	const left = `SELECT count(*) FROM sqlite_master WHERE name IN ('user_identities', 'auth_settings')`
	if n := count(t, db, left); n != 0 {
		t.Errorf("%d tables of the Google sign-in are left after the down", n)
	}

	if n := count(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'checkpoints'`); n != 1 {
		t.Error("the down took a table of another migration with it")
	}
}
