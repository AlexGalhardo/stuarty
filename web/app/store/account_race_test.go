package store

import (
	"context"
	"errors"
	"testing"

	rules "github.com/AlexGalhardo/stuarty/core/account"
	"github.com/AlexGalhardo/stuarty/service/account"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

// pausingSealer lets one write seal in clear text and then holds it before it reaches the database: the window
// between the sealing of a value and its statement, which the account creation cannot see.
type pausingSealer struct {
	account.Sealer
	sealed  chan struct{}
	release chan struct{}
}

func (p *pausingSealer) Seal(table, column, rowKey, plain string) (string, error) {
	out, err := p.Sealer.Seal(table, column, rowKey, plain)
	if err == nil && table == "webhooks" {
		close(p.sealed)
		<-p.release
	}

	return out, err
}

func (p *pausingSealer) Sealing() bool { return account.Sealing(p.Sealer) }

// The write that sealed its value in clear text just before the creation began reaches the database after the
// creation has sealed everything. It must not land in clear text in a sealed column (epic 57 review, item D).
func TestSecurity_T109_AWriteRacingTheCreationNeverLandsInClearText(t *testing.T) {
	const words = "CANARY-RACE-hook-url-token"

	st := openStore(t, newDir(t), Options{})

	m := account.NewManager(account.Options{
		Store: st.Accounts(), Params: account.Params{Time: 1, MemoryKiB: 64, Threads: 1},
	})
	if err := m.Load(ctx); err != nil {
		t.Fatal(err)
	}

	gate := &pausingSealer{Sealer: m.Sealer(), sealed: make(chan struct{}), release: make(chan struct{})}
	st.SetSealer(gate)

	written := make(chan error, 1)

	go func() {
		written <- st.Webhooks().Insert(ctx, webhooks.Row{
			ID: "whk_1", Name: "receiver", URL: "https://hooks.example.com/" + words,
			Events: []string{"export.ready"}, Enabled: true, CreatedBy: "local:ui", CreatedMs: 9, UpdatedMs: 9,
		})
	}()

	<-gate.sealed

	if _, err := m.Create(ctx, rules.Input{
		Username: "stuart", Email: "stuart@example.com", Password: "Correct-Horse-9", Confirmation: "Correct-Horse-9",
	}, func(c context.Context, rec account.Record, fields account.Sealer, _ []byte) error {
		return st.CreateAccount(c, rec, fields, nil)
	}); err != nil {
		t.Fatal(err)
	}

	close(gate.release)

	err := <-written

	var stored int
	if rows, qerr := st.db.QueryContext(ctx, "SELECT url FROM webhooks"); qerr == nil {
		for rows.Next() {
			var url string
			if rows.Scan(&url) == nil && !account.IsSealed(url) {
				stored++
			}
		}

		_ = rows.Close()
	}

	if stored != 0 {
		t.Fatalf("a write that raced the creation left %d clear value(s) in a sealed column (write error: %v)", stored, err)
	}

	if !errors.Is(err, account.ErrLocked) {
		t.Fatalf("the write that lost the race = %v, want ErrLocked", err)
	}
}
