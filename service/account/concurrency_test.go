package account

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// While the account is being made the studio is closed: a write that passed through in clear text at that moment
// would land in a column that is being sealed, and would never open again.
func TestCreate_ClosesTheStudioWhileItRuns(t *testing.T) {
	m, _, _ := newManager(t)
	sealer := m.Sealer()
	failure := errors.New("the migration failed")

	_, err := m.Create(context.Background(), goodInput(), func(context.Context, Record, Sealer, []byte) error {
		if err := m.Check(); !errors.Is(err, ErrLocked) {
			t.Errorf("Check during the creation = %v, want ErrLocked", err)
		}

		if _, err := sealer.Seal("project_items", "body", "pit_1", "words"); !errors.Is(err, ErrLocked) {
			t.Errorf("Seal during the creation = %v, want ErrLocked", err)
		}

		if _, err := sealer.Open("project_items", "body", "pit_1", "words"); !errors.Is(err, ErrLocked) {
			t.Errorf("Open during the creation = %v, want ErrLocked", err)
		}

		if _, err := m.VaultKey(); !errors.Is(err, ErrLocked) {
			t.Errorf("VaultKey during the creation = %v, want ErrLocked", err)
		}

		if !Sealing(sealer) {
			t.Error("the library index must keep no body during the creation")
		}

		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Create = %v", err)
	}

	if err := m.Check(); err != nil {
		t.Errorf("after a failed creation the studio stays as it was, Check = %v", err)
	}

	if got, err := sealer.Seal("project_items", "body", "pit_1", "words"); err != nil || got != "words" {
		t.Errorf("after a failed creation the fields are clear again, Seal = %q, %v", got, err)
	}
}

func TestCreate_TwoAtOnceMakeOneAccount(t *testing.T) {
	m, _, _ := newManager(t)

	var (
		applied atomic.Int32
		created atomic.Int32
		wg      sync.WaitGroup
	)

	apply := func(ctx context.Context, rec Record, _ Sealer, _ []byte) error {
		applied.Add(1)

		return m.store.Create(ctx, rec)
	}

	for range 4 {
		wg.Go(func() {
			if _, err := m.Create(context.Background(), goodInput(), apply); err == nil {
				created.Add(1)
			} else if !errors.Is(err, ErrExists) {
				t.Errorf("Create = %v, want ErrExists", err)
			}
		})
	}

	wg.Wait()

	if applied.Load() != 1 || created.Load() != 1 {
		t.Errorf("the migration ran %d times and %d creations succeeded, want 1 and 1", applied.Load(), created.Load())
	}
}

// A person who closes the tab must not stop the creation in the middle.
func TestCreate_RunsToItsEndWhenTheCallerLeaves(t *testing.T) {
	m, _, _ := newManager(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := m.Create(ctx, goodInput(), func(c context.Context, rec Record, _ Sealer, _ []byte) error {
		if c.Err() != nil {
			t.Errorf("the creation ran with a cancelled context: %v", c.Err())
		}

		return m.store.Create(c, rec)
	}); err != nil {
		t.Fatal(err)
	}

	if m.State() != StateLocked {
		t.Errorf("state = %s, want locked", m.State())
	}
}

// If the creation reports a failure after the row is in, the state must follow the table: a state of none would
// write clear text into a sealed column.
func TestCreate_StateFollowsTheTableAfterAFailure(t *testing.T) {
	m, _, _ := newManager(t)

	apply := func(c context.Context, rec Record, _ Sealer, _ []byte) error {
		if err := m.store.Create(c, rec); err != nil {
			return err
		}

		return errors.New("failed after the commit")
	}

	if _, err := m.Create(context.Background(), goodInput(), apply); err == nil {
		t.Fatal("Create returned no error")
	}

	if m.State() != StateLocked {
		t.Errorf("state = %s, want locked: the account is in the table", m.State())
	}
}

// A burst of wrong passwords sent together is counted one by one: the limit is not read before the wait.
func TestLogin_ABurstIsCountedAttemptByAttempt(t *testing.T) {
	m, _, _ := newManager(t)

	if _, err := m.Create(context.Background(), goodInput(), nil); err != nil {
		t.Fatal(err)
	}

	var (
		wrong, limited atomic.Int32
		wg             sync.WaitGroup
	)

	for range maxFailures * 6 {
		wg.Go(func() {
			err := m.Login(context.Background(), "stuart", "Wrong-Horse-1")

			var rl *RateLimitedError

			switch {
			case errors.Is(err, ErrWrongPassword):
				wrong.Add(1)
			case errors.As(err, &rl):
				limited.Add(1)
			default:
				t.Errorf("Login = %v", err)
			}
		})
	}

	wg.Wait()

	if wrong.Load() != maxFailures {
		t.Errorf("%d passwords were tried, want at most %d (%d refused)", wrong.Load(), maxFailures, limited.Load())
	}
}

// The idle lock rewrites the record: it must not put back the keys of a password that changed at the same time.
func TestSetIdleLock_DoesNotUndoAPasswordChange(t *testing.T) {
	m, st, _ := newManager(t)
	ctx := context.Background()

	if _, err := m.Create(ctx, goodInput(), nil); err != nil {
		t.Fatal(err)
	}

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	current, next := goodPassword, otherPass

	for range 8 {
		var wg sync.WaitGroup

		wg.Go(func() {
			if err := m.ChangePassword(ctx, current, next, next); err != nil {
				t.Errorf("ChangePassword = %v", err)
			}
		})

		wg.Go(func() {
			for range 4 {
				if err := m.SetIdleLock(ctx, 5); err != nil {
					t.Errorf("SetIdleLock = %v", err)
				}
			}
		})

		wg.Wait()

		rec, err := st.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}

		keys, err := OpenWithPassword(rec, next)
		if err != nil {
			t.Fatalf("the new password does not open the record: %v", err)
		}

		keys.Wipe()

		current, next = next, current
	}
}
