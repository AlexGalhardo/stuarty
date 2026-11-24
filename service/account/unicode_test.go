package account

import (
	"bytes"
	"context"
	"errors"
	"testing"

	rules "github.com/AlexGalhardo/stuarty/core/account"
)

// One password, two Unicode forms: an é typed as one character or as two (VV-380).
const (
	pwComposed   = "Abcdeé1!"
	pwDecomposed = "Abcdeé" + "1!"
)

func TestLogin_TheTwoUnicodeFormsOfAPasswordOpenTheSameAccount(t *testing.T) {
	for name, c := range map[string]struct{ made, typed string }{
		"made composed, typed decomposed": {pwComposed, pwDecomposed},
		"made decomposed, typed composed": {pwDecomposed, pwComposed},
	} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := newManager(t)
			ctx := context.Background()

			in := rules.Input{
				Username: "stuart", Email: "stuart@example.com", Password: c.made, Confirmation: c.made,
			}
			if _, err := m.Create(ctx, in, nil); err != nil {
				t.Fatal(err)
			}

			if err := m.Login(ctx, "stuart", c.typed); err != nil {
				t.Fatalf("the other form of the password = %v", err)
			}
		})
	}
}

// An account made before the normalization derived its key from the bytes as typed. It must still open, in the
// form it was made with and in the other one, and its verifier is rewritten so that the next sign-in is the
// ordinary one.
func TestLogin_AnAccountMadeFromTheRawBytesStillOpensAndIsRewritten(t *testing.T) {
	for name, typed := range map[string]string{
		"typed as it was made": pwDecomposed,
		"typed composed":       pwComposed,
	} {
		t.Run(name, func(t *testing.T) {
			m, st, _ := newManager(t)
			ctx := context.Background()

			rec, keys, _, err := sealSecret("stuart", "stuart@example.com", pwDecomposed, fastParams)
			if err != nil {
				t.Fatal(err)
			}

			keys.Wipe()

			if err := st.Create(ctx, rec); err != nil {
				t.Fatal(err)
			}

			if err := m.Load(ctx); err != nil {
				t.Fatal(err)
			}

			if err := m.Login(ctx, "stuart", typed); err != nil {
				t.Fatalf("an account made before the normalization = %v", err)
			}

			m.Lock()

			after, err := st.Get(ctx)
			if err != nil {
				t.Fatal(err)
			}

			if bytes.Equal(after.Verifier, rec.Verifier) {
				t.Error("the verifier of the old form was not rewritten")
			}

			if _, legacy, err := openWithPassword(after, pwComposed); err != nil || legacy {
				t.Errorf("after the rewrite the composed form opens in the ordinary way: legacy %v, %v", legacy, err)
			}

			if err := m.Login(ctx, "stuart", pwDecomposed); err != nil {
				t.Errorf("the next sign-in, in the other form = %v", err)
			}
		})
	}
}

func TestLogin_AWrongPasswordWithAccentsIsStillRefusedAndCountedOnce(t *testing.T) {
	m, _, _ := newManager(t)
	ctx := context.Background()

	in := rules.Input{Username: "stuart", Email: "stuart@example.com", Password: pwComposed, Confirmation: pwComposed}
	if _, err := m.Create(ctx, in, nil); err != nil {
		t.Fatal(err)
	}

	if err := m.Login(ctx, "stuart", "Abcdéé1!"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("a wrong password = %v", err)
	}

	if len(m.failures) != 1 {
		t.Errorf("%d failures counted, want 1", len(m.failures))
	}
}

func TestChangePassword_TheNewPasswordIsKeptInItsComposedForm(t *testing.T) {
	m, st, _, _ := created(t)
	ctx := context.Background()

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	if err := m.ChangePassword(ctx, goodPassword, pwDecomposed, pwComposed); err != nil {
		t.Fatalf("the confirmation in the other form = %v", err)
	}

	rec, err := st.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, legacy, err := openWithPassword(rec, pwComposed); err != nil || legacy {
		t.Fatalf("the composed form must open the new password in the ordinary way: legacy %v, %v", legacy, err)
	}
}

// VV-382: a new recovery key is made with the password; the old one stops opening the account, the password and
// the data stay as they are.
func TestNewRecoveryKey_ReplacesTheKeyAndKeepsThePasswordAndTheData(t *testing.T) {
	m, st, _, first := created(t)
	ctx := context.Background()

	if _, err := m.NewRecoveryKey(ctx, goodPassword); !errors.Is(err, ErrLocked) {
		t.Fatalf("NewRecoveryKey while locked = %v, want ErrLocked", err)
	}

	if err := m.Login(ctx, "stuart", goodPassword); err != nil {
		t.Fatal(err)
	}

	before, err := st.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.NewRecoveryKey(ctx, otherPass); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("NewRecoveryKey with a wrong password = %v, want ErrWrongPassword", err)
	}

	second, err := m.NewRecoveryKey(ctx, goodPassword)
	if err != nil {
		t.Fatal(err)
	}

	if second == "" || second == first {
		t.Fatalf("the new key %q must be new", second)
	}

	rec, err := st.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := OpenWithRecoveryKey(rec, first); !errors.Is(err, ErrWrongRecoveryKey) {
		t.Errorf("the old recovery key = %v, want ErrWrongRecoveryKey", err)
	}

	keys, err := OpenWithRecoveryKey(rec, second)
	if err != nil {
		t.Fatalf("the new recovery key does not open: %v", err)
	}

	oldKeys, err := OpenWithPassword(before, goodPassword)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(keys.Data, oldKeys.Data) || !bytes.Equal(keys.Vault, oldKeys.Vault) {
		t.Error("the data key changed: what is sealed would not open")
	}

	if _, err := OpenWithPassword(rec, goodPassword); err != nil {
		t.Errorf("the password no longer opens: %v", err)
	}

	if !bytes.Equal(rec.Verifier, before.Verifier) || !bytes.Equal(rec.KeyByPassword, before.KeyByPassword) {
		t.Error("the password record changed")
	}
}
