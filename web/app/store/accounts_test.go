package store

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/account"
)

func accountRecord() account.Record {
	return account.Record{
		Username: "stuart", Email: "stuart@example.com",
		KDF:      account.KDF{Alg: "argon2id", Time: 1, MemoryKiB: 64, Threads: 1, Salt: bytes.Repeat([]byte{1}, 16)},
		Verifier: bytes.Repeat([]byte{2}, 32), KeyByPassword: bytes.Repeat([]byte{3}, 60),
		KeyByRecovery: bytes.Repeat([]byte{4}, 60), VaultKey: bytes.Repeat([]byte{5}, 60),
		IdleLockMinutes: 15, CreatedMs: 10, UpdatedMs: 10,
	}
}

func TestAccountRepoRoundTripAndOneAccount(t *testing.T) {
	s := openStore(t, newDir(t), Options{})
	ctx := context.Background()
	repo := s.Accounts()

	if _, err := repo.Get(ctx); !errors.Is(err, account.ErrNoAccount) {
		t.Fatalf("an empty workspace: %v", err)
	}

	if err := repo.Update(ctx, accountRecord()); !errors.Is(err, account.ErrNoAccount) {
		t.Fatalf("an update with no account: %v", err)
	}

	rec := accountRecord()
	if err := repo.Create(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if !sameRecord(got, rec) {
		t.Fatalf("round trip: %+v", got)
	}

	if err := repo.Create(ctx, rec); !errors.Is(err, account.ErrExists) {
		t.Fatalf("a second account: %v", err)
	}

	rec.IdleLockMinutes, rec.UpdatedMs = 0, 20
	if err := repo.Update(ctx, rec); err != nil {
		t.Fatal(err)
	}

	if got, _ = repo.Get(ctx); got.IdleLockMinutes != 0 || got.UpdatedMs != 20 {
		t.Fatalf("after the update: %+v", got)
	}
}

func sameRecord(got, rec account.Record) bool {
	return got.Username == rec.Username && got.Email == rec.Email && got.KDF.Time == 1 &&
		bytes.Equal(got.KDF.Salt, rec.KDF.Salt) && bytes.Equal(got.Verifier, rec.Verifier) &&
		bytes.Equal(got.KeyByPassword, rec.KeyByPassword) && bytes.Equal(got.KeyByRecovery, rec.KeyByRecovery) &&
		bytes.Equal(got.VaultKey, rec.VaultKey) && got.IdleLockMinutes == 15
}
