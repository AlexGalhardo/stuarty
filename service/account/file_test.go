package account

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"testing"
)

func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = byte(i + 1)
	}

	return k
}

func TestSealedFileRoundTripsAndOpensAnEnvelopeBuiltByHand(t *testing.T) {
	s, err := NewKeySealer(testKey())
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("---\nname: note\n---\nhello")

	sealed, err := s.SealFile("memory/note.md", plain)
	if err != nil {
		t.Fatal(err)
	}

	if !IsSealedFile(sealed) || bytes.Contains(sealed, []byte("hello")) {
		t.Fatal("the file is not sealed")
	}

	got, err := s.OpenFile("memory/note.md", sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip: %q %v", got, err)
	}

	// A vector made with the standard library alone: magic, version, nonce, ciphertext and tag.
	block, _ := aes.NewCipher(testKey())
	aead, _ := cipher.NewGCM(block)
	nonce := bytes.Repeat([]byte{7}, 12)
	hand := append([]byte("STFL\x01"), nonce...)
	hand = aead.Seal(hand, nonce, plain, []byte("stuarty/file/v1\x00memory/note.md"))

	got, err = s.OpenFile("memory/note.md", hand)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("the envelope of the specification does not open: %v", err)
	}
}

func TestSealedFileRefusesAWrongKeyAPathACutFileAndAFlippedByte(t *testing.T) {
	s, _ := NewKeySealer(testKey())
	other := testKey()
	other[0] ^= 0xff
	o, _ := NewKeySealer(other)

	sealed, err := s.SealFile("skills/a/SKILL.md", []byte("body"))
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func() error{
		"wrong key":  func() error { _, e := o.OpenFile("skills/a/SKILL.md", sealed); return e },
		"wrong path": func() error { _, e := s.OpenFile("skills/b/SKILL.md", sealed); return e },
		"cut":        func() error { _, e := s.OpenFile("skills/a/SKILL.md", sealed[:len(sealed)-3]); return e },
		"header only": func() error {
			_, e := s.OpenFile("skills/a/SKILL.md", sealed[:5])
			return e
		},
		"clear": func() error { _, e := s.OpenFile("skills/a/SKILL.md", []byte("plain")); return e },
		"flipped": func() error {
			c := bytes.Clone(sealed)
			c[len(c)-1] ^= 1
			_, e := s.OpenFile("skills/a/SKILL.md", c)

			return e
		},
	}

	for name, fn := range cases {
		if err := fn(); !errors.Is(err, ErrUnsealed) {
			t.Errorf("%s: %v", name, err)
		}
	}

	again, _ := s.SealFile("skills/a/SKILL.md", []byte("body"))
	if bytes.Equal(again, sealed) {
		t.Error("two seals of one file are the same bytes: the nonce is not fresh")
	}
}

func TestManagerFilesFollowTheState(t *testing.T) {
	m, _, _ := newManager(t)
	f := m.Files()

	if f.Sealing() || f.Ready() != nil {
		t.Fatal("no account: clear and ready")
	}

	if out, err := f.SealFile("a", []byte("x")); err != nil || string(out) != "x" {
		t.Fatal("no account must keep clear text")
	}

	if _, err := m.Create(context.Background(), goodInput(), nil); err != nil {
		t.Fatal(err)
	}

	if !f.Sealing() || !errors.Is(f.Ready(), ErrLocked) {
		t.Fatal("locked: sealing and not ready")
	}

	if _, err := f.SealFile("a", []byte("x")); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked seal: %v", err)
	}

	if err := m.Login(context.Background(), "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	sealed, err := f.SealFile("a", []byte("x"))
	if err != nil || !IsSealedFile(sealed) {
		t.Fatal(err)
	}

	m.Lock()

	if _, err := f.OpenFile("a", sealed); !errors.Is(err, ErrLocked) {
		t.Fatalf("after lock: %v", err)
	}
}

func TestOnUnlockRunsAfterTheKeysAndWhileTheGateIsClosed(t *testing.T) {
	m, _, _, recovery := created(t)

	var (
		ran             int
		gate, ready     error
		stateInHook     State
		sealedInHook    bool
		sealerErrInHook error
	)

	m.OnUnlock(func() {
		ran++
		gate = m.Check()
		stateInHook = m.State()
		_, sealerErrInHook = m.Sealer().Seal("t", "c", "k", "v")
		_, err := m.Files().SealFile("a", []byte("x"))
		sealedInHook = err == nil
		ready = m.Files().Ready()
	})

	if err := m.Login(context.Background(), "stuart", "Wrong-Pass-1!"); err == nil || ran != 0 {
		t.Fatalf("a failed sign-in must not run the hook: %v %d", err, ran)
	}

	if err := m.Login(context.Background(), "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	if ran != 1 || !errors.Is(gate, ErrLocked) || stateInHook != StateUnlocked || sealerErrInHook != nil ||
		!sealedInHook || ready != nil {
		t.Fatalf("hook: ran=%d gate=%v state=%v sealer=%v sealed=%v ready=%v",
			ran, gate, stateInHook, sealerErrInHook, sealedInHook, ready)
	}

	if err := m.Check(); err != nil {
		t.Fatalf("the gate must open after the hooks: %v", err)
	}

	m.Lock()

	if err := m.Recover(context.Background(), recovery, otherPass, otherPass); err != nil || ran != 2 {
		t.Fatalf("recover must run the hook too: %v %d", err, ran)
	}
}

func TestOnUnlockRunsOutsideTheLockOfTheManager(t *testing.T) {
	m, _, _, _ := created(t)

	done := make(chan struct{})

	m.OnUnlock(func() {
		// Each of these takes the lock of the manager: a hook run under it would deadlock.
		_ = m.State()
		_ = m.Username()
		_, _ = m.VaultKey()
		m.Touch()
		close(done)
	})

	if err := m.Login(context.Background(), "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	<-done
}
