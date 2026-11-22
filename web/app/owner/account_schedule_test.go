package owner

import (
	"context"
	"errors"
	"testing"
	"time"

	core "github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/account"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// A post whose time comes while the studio is locked is neither sent nor lost: the scheduler cannot read it, and
// the row is as it was when the person signs in again.
func TestSecurity_T116_ALockedStudioNeitherSendsNorLosesAScheduledPost(t *testing.T) {
	ctx := context.Background()

	o, _, project := existingWorkspace(t, accountEnv(t))

	defer func() { _ = o.Close() }()

	sched, ok := service.SchedulerOf(o.Service)
	if !ok {
		t.Fatal("the owner has no scheduler")
	}

	sched.SetPaused(false)

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	// The time has just passed, inside its late window: a scheduler that could read the row would send it.
	now := time.Now().UTC()
	at := now.Add(-time.Minute)

	if err := o.Store.Publications().Insert(ctx, &pub.Publication{
		ID: "pub_due", ProjectID: project, Title: "Due", Text: "the words of the post", CreatedAt: now, UpdatedAt: now,
		CreatedBy: "local:ui",
		Targets: []pub.Target{{
			ID: "tgt_due", PublicationID: "pub_due", ConnectionID: "con_none", Network: "youtube",
			State: core.TargetScheduled, ScheduledAt: &at, Zone: "UTC", LateWindowMinutes: 60, CreatedAt: now,
			UpdatedAt: now,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	o.Account.Lock()

	if _, err := sched.Tick(ctx); !errors.Is(err, account.ErrLocked) {
		t.Fatalf("a pass of the scheduler while locked: %v", err)
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	got, err := o.Store.Publications().Get(ctx, "pub_due")
	if err != nil || got.Text != "the words of the post" || len(got.Targets) != 1 {
		t.Fatalf("the post after the lock: %+v %v", got, err)
	}

	if tg := got.Targets[0]; tg.State != core.TargetScheduled || tg.Attempts != 0 || tg.LeaseOwner != "" {
		t.Fatalf("the target changed while locked: state %s attempts %d lease %q", tg.State, tg.Attempts, tg.LeaseOwner)
	}
}
