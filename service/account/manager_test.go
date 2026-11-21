package account

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	rules "github.com/AlexGalhardo/stuarty/core/account"
)

// fastParams keep the tests quick; the product uses DefaultParams.
var fastParams = Params{Time: 1, MemoryKiB: 64, Threads: 1}

type memStore struct {
	mu  sync.Mutex
	rec *Record
}

func (s *memStore) Get(context.Context) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rec == nil {
		return Record{}, ErrNoAccount
	}

	return clone(*s.rec), nil
}

func (s *memStore) Create(_ context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rec != nil {
		return ErrExists
	}

	c := clone(rec)
	s.rec = &c

	return nil
}

func (s *memStore) Update(_ context.Context, rec Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.rec == nil {
		return ErrNoAccount
	}

	c := clone(rec)
	s.rec = &c

	return nil
}

func clone(r Record) Record {
	r.Verifier = slices.Clone(r.Verifier)
	r.KeyByPassword = slices.Clone(r.KeyByPassword)
	r.KeyByRecovery = slices.Clone(r.KeyByRecovery)
	r.VaultKey = slices.Clone(r.VaultKey)
	r.KDF.Salt = slices.Clone(r.KDF.Salt)

	return r
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

const (
	goodPassword = "Correct-Horse-9"
	otherPass    = "Another-Pass-7!"
)

func goodInput() rules.Input {
	return rules.Input{
		Username: "stuart", Email: "stuart@example.com", Password: goodPassword, Confirmation: goodPassword,
	}
}

func newManager(t *testing.T) (*Manager, *memStore, *clock) {
	t.Helper()

	st := &memStore{}
	clk := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	m := NewManager(Options{Store: st, Now: clk.now, Params: fastParams})

	if err := m.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	return m, st, clk
}

func created(t *testing.T) (*Manager, *memStore, *clock, string) {
	t.Helper()

	m, st, clk := newManager(t)

	recovery, err := m.Create(context.Background(), goodInput(), nil)
	if err != nil {
		t.Fatal(err)
	}

	return m, st, clk, recovery
}

func TestNewWorkspaceHasNoAccountAndWorksAsBefore(t *testing.T) {
	m, _, _ := newManager(t)

	if m.State() != StateNone {
		t.Fatalf("state %s", m.State())
	}

	if err := m.Check(); err != nil {
		t.Fatalf("the gate is closed with no account: %v", err)
	}

	got, err := m.Sealer().Seal("t", "c", "r", "clear")
	if err != nil || got != "clear" {
		t.Fatalf("with no account a field stays clear: %q %v", got, err)
	}
}

func TestCreateLeavesTheAccountLockedAndShowsTheRecoveryKeyOnce(t *testing.T) {
	m, _, _, recovery := created(t)

	if m.State() != StateLocked {
		t.Fatalf("state %s", m.State())
	}

	if !errors.Is(m.Check(), ErrLocked) {
		t.Fatal("the gate is open on a new account")
	}

	if _, err := m.VaultKey(); !errors.Is(err, ErrLocked) {
		t.Fatalf("the vault key is available while locked: %v", err)
	}

	groups := strings.Split(recovery, "-")
	if len(groups) != 13 || len(groups[0]) != 4 || len(groups[12]) != 4 {
		t.Fatalf("recovery key shape %q", recovery)
	}

	if _, err := m.Create(context.Background(), goodInput(), nil); !errors.Is(err, ErrExists) {
		t.Fatalf("a second account: %v", err)
	}
}

func TestCreateRefusesWhatTheRulesRefuse(t *testing.T) {
	m, st, _ := newManager(t)

	in := goodInput()
	in.Confirmation = "different"
	in.Username = "AB"

	_, err := m.Create(context.Background(), in, nil)

	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("got %v", err)
	}

	if st.rec != nil {
		t.Fatal("a record was stored for a refused input")
	}
}

func TestPasswordAndRecoveryKeyEachUnlockAndAWrongOneDoesNot(t *testing.T) {
	ctx := context.Background()
	m, _, _, recovery := created(t)

	if err := m.Login(ctx, "stuart", "Wrong-Pass-1!"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("a wrong password: %v", err)
	}

	if m.State() != StateLocked {
		t.Fatal("a wrong password unlocked")
	}

	if err := m.Login(ctx, "nobody", goodPassword); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("an unknown name with the right password: %v", err)
	}

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	if m.State() != StateUnlocked || m.Check() != nil {
		t.Fatal("the right password did not unlock")
	}

	m.Lock()

	if err := m.Login(ctx, "STUART@Example.com", goodPassword); err != nil {
		t.Fatalf("sign-in by e-mail, any case: %v", err)
	}

	m.Lock()

	// The recovery key sets a new password and signs in.
	if err := m.Recover(ctx, "AAAA-BBBB", otherPass, otherPass); !errors.Is(err, ErrWrongRecoveryKey) {
		t.Fatalf("a bad recovery key: %v", err)
	}

	if err := m.Recover(ctx, recovery, otherPass, otherPass); err != nil {
		t.Fatal(err)
	}

	if m.State() != StateUnlocked {
		t.Fatal("recovery did not sign in")
	}

	m.Lock()

	if err := m.Login(ctx, "stuart", goodPassword); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("the old password after a recovery: %v", err)
	}

	if err := m.Login(ctx, "stuart", otherPass); err != nil {
		t.Fatalf("the new password: %v", err)
	}
}

func TestTheRecoveryKeyParsesWhatAPersonTypes(t *testing.T) {
	raw := bytes.Repeat([]byte{0xA7, 0x13}, 16)
	text := FormatRecoveryKey(raw)

	for _, typed := range []string{
		text, strings.ToLower(text), strings.ReplaceAll(text, "-", " "), strings.ReplaceAll(text, "-", ""),
		"  " + text + "\n",
	} {
		got, err := ParseRecoveryKey(typed)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("%q: %v", typed, err)
		}
	}

	for _, bad := range []string{"", "ABCD", text + "A", strings.Replace(text, text[:1], "U", 1)} {
		if _, err := ParseRecoveryKey(bad); !errors.Is(err, ErrWrongRecoveryKey) {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestChangingThePasswordKeepsTheData(t *testing.T) {
	ctx := context.Background()
	m, _, _, recovery := created(t)

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	sealed, err := m.Sealer().Seal("project_items", "body", "pit_1", "my private script")
	if err != nil {
		t.Fatal(err)
	}

	if err := m.ChangePassword(ctx, "Wrong-Pass-1!", otherPass, otherPass); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("a wrong current password: %v", err)
	}

	var ve *ValidationError
	if err := m.ChangePassword(ctx, goodPassword, "short", "short"); !errors.As(err, &ve) {
		t.Fatalf("a weak new password: %v", err)
	}

	if err := m.ChangePassword(ctx, goodPassword, otherPass, otherPass); err != nil {
		t.Fatal(err)
	}

	m.Lock()

	if err := m.Login(ctx, "stuart", goodPassword); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("the old password after the change: %v", err)
	}

	if err := m.Login(ctx, "stuart", otherPass); err != nil {
		t.Fatal(err)
	}

	got, err := m.Sealer().Open("project_items", "body", "pit_1", sealed)
	if err != nil || got != "my private script" {
		t.Fatalf("the data did not survive the change: %q %v", got, err)
	}

	m.Lock()

	// The recovery key still opens the same data.
	if err := m.Recover(ctx, recovery, goodPassword, goodPassword); err != nil {
		t.Fatal(err)
	}

	got, err = m.Sealer().Open("project_items", "body", "pit_1", sealed)
	if err != nil || got != "my private script" {
		t.Fatalf("the data did not survive the recovery: %q %v", got, err)
	}
}

func TestLockWipesTheKeysAndRefusesTheFields(t *testing.T) {
	ctx := context.Background()
	m, _, _, _ := created(t)

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	sealed, err := m.Sealer().Seal("t", "c", "r", "text")
	if err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	data, vaultKey, sealerKey := m.keys.Data, m.keys.Vault, m.sealer.key
	m.mu.Unlock()

	var locked int

	m.OnLock(func() { locked++ })
	m.Lock()

	for name, buf := range map[string][]byte{"data key": data, "vault key": vaultKey, "sealer copy": sealerKey} {
		if !bytes.Equal(buf, make([]byte, len(buf))) || len(buf) == 0 {
			t.Errorf("the %s buffer was not zeroed", name)
		}
	}

	if locked != 1 {
		t.Fatalf("the lock hooks ran %d times", locked)
	}

	if _, err := m.Sealer().Open("t", "c", "r", sealed); !errors.Is(err, ErrLocked) {
		t.Fatalf("open while locked: %v", err)
	}

	if _, err := m.Sealer().Seal("t", "c", "r", "x"); !errors.Is(err, ErrLocked) {
		t.Fatalf("seal while locked: %v", err)
	}

	if _, err := m.VaultKey(); !errors.Is(err, ErrLocked) {
		t.Fatalf("vault key while locked: %v", err)
	}

	m.Lock()

	if locked != 1 {
		t.Fatal("a second lock ran the hooks again")
	}
}

func TestTheIdleLockLocksAfterTheTimeAndOnlyWhenOn(t *testing.T) {
	ctx := context.Background()
	m, _, clk, _ := created(t)

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	clk.t = clk.t.Add(10 * 24 * time.Hour)
	m.Tick()

	if m.State() != StateUnlocked {
		t.Fatal("the idle lock is off by default and locked anyway")
	}

	var ve *ValidationError
	if err := m.SetIdleLock(ctx, 1441); !errors.As(err, &ve) {
		t.Fatalf("an idle lock out of range: %v", err)
	}

	if err := m.SetIdleLock(ctx, 5); err != nil {
		t.Fatal(err)
	}

	clk.t = clk.t.Add(4 * time.Minute)
	m.Tick()

	if m.State() != StateUnlocked {
		t.Fatal("locked too early")
	}

	m.Touch()
	clk.t = clk.t.Add(4 * time.Minute)
	m.Tick()

	if m.State() != StateUnlocked {
		t.Fatal("activity did not postpone the lock")
	}

	clk.t = clk.t.Add(time.Minute)
	m.Tick()

	if m.State() != StateLocked {
		t.Fatal("the idle lock did not lock")
	}

	if err := m.SetIdleLock(ctx, 0); !errors.Is(err, ErrLocked) {
		t.Fatalf("a setting changed while locked: %v", err)
	}
}

func TestSecurity_T103_TheRecordHoldsNoPasswordNoRecoveryKeyNoClearKey(t *testing.T) {
	ctx := context.Background()
	m, st, _ := newManager(t)

	recovery, err := m.Create(ctx, goodInput(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	m.mu.Lock()
	dataKey := slices.Clone(m.keys.Data)
	vaultKey := slices.Clone(m.keys.Vault)
	m.mu.Unlock()

	raw, _ := ParseRecoveryKey(recovery)

	rec, _ := st.Get(ctx)
	haystack := bytes.Join([][]byte{
		[]byte(rec.Username), []byte(rec.Email), rec.KDF.Salt, rec.Verifier, rec.KeyByPassword, rec.KeyByRecovery,
		rec.VaultKey,
	}, []byte{0})

	for name, needle := range map[string][]byte{
		"password": []byte(goodPassword), "recovery key": raw, "recovery text": []byte(recovery),
		"data key": dataKey, "vault key": vaultKey,
	} {
		if bytes.Contains(haystack, needle) {
			t.Errorf("the record holds the %s in clear", name)
		}
	}

	// The verifier is not the key that wraps the data: knowing it opens nothing.
	if _, err := unwrap(rec.Verifier, wrapAAD("data-by-password"), rec.KeyByPassword); err == nil {
		t.Error("the verifier opens the wrapped data key")
	}

	// Two accounts with the same password share no salt, verifier or wrapping.
	m2, st2, _ := newManager(t)
	if _, err := m2.Create(ctx, goodInput(), nil); err != nil {
		t.Fatal(err)
	}

	rec2, _ := st2.Get(ctx)
	if bytes.Equal(rec.KDF.Salt, rec2.KDF.Salt) || bytes.Equal(rec.Verifier, rec2.Verifier) ||
		bytes.Equal(rec.KeyByPassword, rec2.KeyByPassword) {
		t.Error("two accounts with one password look alike")
	}
}

func TestSecurity_T103_AWrongPasswordAndAWrongRecoveryKeyOpenNothing(t *testing.T) {
	ctx := context.Background()
	m, st, _, _ := created(t)
	rec, _ := st.Get(ctx)

	if keys, err := OpenWithPassword(rec, otherPass); !errors.Is(err, ErrWrongPassword) || keys.Data != nil {
		t.Fatalf("a wrong password gave %v %v", keys, err)
	}

	other := FormatRecoveryKey(bytes.Repeat([]byte{7}, KeySize))
	if keys, err := OpenWithRecoveryKey(rec, other); !errors.Is(err, ErrWrongRecoveryKey) || keys.Data != nil {
		t.Fatalf("a wrong recovery key gave %v %v", keys, err)
	}

	// A record whose verifier was forged still cannot unwrap the data key: the wrapping is authenticated.
	forged := clone(rec)

	kek, verifier, err := derive(otherPass, forged.KDF)
	if err != nil {
		t.Fatal(err)
	}

	wipe(kek)

	forged.Verifier = verifier

	if keys, err := OpenWithPassword(forged, otherPass); err == nil || keys.Data != nil {
		t.Fatalf("a forged verifier opened the account: %v", err)
	}

	// A damaged KDF record is refused before any allocation.
	bad := clone(rec)
	bad.KDF.MemoryKiB = 1 << 30

	if _, err := OpenWithPassword(bad, goodPassword); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("an absurd KDF: %v", err)
	}

	_ = m
}

func TestSecurity_T104_AfterFiveFailuresTheSignInIsRefusedUntilTheWindowPasses(t *testing.T) {
	ctx := context.Background()
	m, _, clk, _ := created(t)

	for range maxFailures {
		if err := m.Login(ctx, "stuart", otherPass); !errors.Is(err, ErrWrongPassword) {
			t.Fatalf("got %v", err)
		}
	}

	var rl *RateLimitedError

	if err := m.Login(ctx, "stuart", goodPassword); !errors.As(err, &rl) || rl.RetryAfter <= 0 {
		t.Fatalf("the sixth attempt, even with the right password, must be refused: %v", err)
	}

	if err := m.Recover(ctx, "x", otherPass, otherPass); !errors.As(err, &rl) {
		t.Fatalf("the recovery shares the limit: %v", err)
	}

	clk.t = clk.t.Add(failureWindow)

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatalf("after the window: %v", err)
	}
}

func TestSecurity_T105_ASealedValueDoesNotOpenInAnotherCellOrAfterATamper(t *testing.T) {
	key := bytes.Repeat([]byte{9}, KeySize)

	s, err := NewKeySealer(key)
	if err != nil {
		t.Fatal(err)
	}

	sealed, err := s.Seal("text_revisions", "body", "pit_1/2", "secret words")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(sealed, "secret") || !IsSealed(sealed) {
		t.Fatalf("not sealed: %q", sealed)
	}

	if got, err := s.Open("text_revisions", "body", "pit_1/2", sealed); err != nil || got != "secret words" {
		t.Fatalf("%q %v", got, err)
	}

	for name, cell := range map[string][3]string{
		"another row":    {"text_revisions", "body", "pit_1/3"},
		"another column": {"text_revisions", "actor", "pit_1/2"},
		"another table":  {"project_items", "body", "pit_1/2"},
	} {
		if _, err := s.Open(cell[0], cell[1], cell[2], sealed); !errors.Is(err, ErrUnsealed) {
			t.Errorf("%s: a value opened in a cell it was not sealed for: %v", name, err)
		}
	}

	tampered := sealed[:len(sealed)-2] + "AA"
	if tampered == sealed {
		tampered = sealed[:len(sealed)-2] + "BB"
	}

	if _, err := s.Open("text_revisions", "body", "pit_1/2", tampered); !errors.Is(err, ErrUnsealed) {
		t.Errorf("a tampered value opened: %v", err)
	}

	if _, err := s.Open("text_revisions", "body", "pit_1/2", "clear text"); !errors.Is(err, ErrUnsealed) {
		t.Errorf("a clear value was accepted as sealed: %v", err)
	}

	other, _ := NewKeySealer(bytes.Repeat([]byte{8}, KeySize))
	if _, err := other.Open("text_revisions", "body", "pit_1/2", sealed); !errors.Is(err, ErrUnsealed) {
		t.Errorf("another key opened the value: %v", err)
	}

	again, _ := s.Seal("text_revisions", "body", "pit_1/2", "secret words")
	if again == sealed {
		t.Error("two seals of one value are identical: the nonce is reused")
	}

	empty, _ := s.Seal("t", "c", "r", "")
	if !IsSealed(empty) || len(empty) <= len(Prefix) {
		t.Error("an empty value is not sealed")
	}
}

func TestSecurity_T106_NoErrorOfTheAccountHoldsASecret(t *testing.T) {
	ctx := context.Background()
	m, _, _, recovery := created(t)

	secrets := []string{goodPassword, otherPass, recovery, strings.ReplaceAll(recovery, "-", "")}

	var messages []string

	record := func(err error) {
		if err != nil {
			messages = append(messages, err.Error())

			if ve, ok := errors.AsType[*ValidationError](err); ok {
				for _, p := range ve.Problems {
					messages = append(messages, p.Field, p.Reason)
				}
			}
		}
	}

	record(m.Login(ctx, "stuart", otherPass))
	record(m.Login(ctx, otherPass, goodPassword))
	record(m.Recover(ctx, recovery+"x", otherPass, "different"))
	record(m.Recover(ctx, goodPassword, "weak", "weak"))
	record(m.ChangePassword(ctx, goodPassword, otherPass, otherPass))
	_, err := m.Create(ctx, rules.Input{Username: otherPass, Email: goodPassword, Password: recovery}, nil)
	record(err)

	for _, msg := range messages {
		for _, secret := range secrets {
			if strings.Contains(msg, secret) {
				t.Errorf("an error message holds a secret: %q", msg)
			}
		}
	}
}

// BenchmarkDeriveDefault measures one derivation at DefaultParams; SDD-007 section 3.9 records the time.
func BenchmarkDeriveDefault(b *testing.B) {
	kdf, err := newKDF(DefaultParams)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		kek, verifier, err := derive(goodPassword, kdf)
		if err != nil {
			b.Fatal(err)
		}

		wipe(kek)
		wipe(verifier)
	}
}
