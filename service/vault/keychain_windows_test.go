package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
	"unsafe"
)

func deleteItem(t *testing.T, service, account string) {
	t.Helper()

	name, err := target(service, account)
	if err != nil {
		t.Fatal(err)
	}

	proc := advapi32.NewProc("CredDeleteW")
	if r, _, callErr := proc.Call(uintptr(unsafe.Pointer(name)), credTypeGeneric, 0); r == 0 {
		t.Logf("cleaning the credential: %v", callErr)
	}
}

// TestMasterKeyWindowsCredentialManager round-trips a key through the real
// Credential Manager, under a service name no product item uses, and removes
// the item afterwards.
func TestMasterKeyWindowsCredentialManager(t *testing.T) {
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}

	service := "stuarty-test-" + hex.EncodeToString(suffix)
	kc := OSKeychain()

	t.Cleanup(func() { deleteItem(t, service, "vault:test") })

	if _, err := kc.Get(service, "vault:test"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Get before Set: %v, want ErrKeyNotFound", err)
	}

	key := []byte(FormatKey(bytes.Repeat([]byte{0x5A}, KeySize)))
	if err := kc.Set(service, "vault:test", key); err != nil {
		t.Fatal(err)
	}

	got, err := kc.Get(service, "vault:test")
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("Get = %q, %v", got, err)
	}

	next := []byte(FormatKey(bytes.Repeat([]byte{0x6B}, KeySize)))
	if err := kc.Set(service, "vault:test", next); err != nil {
		t.Fatal(err)
	}

	if got, _ = kc.Get(service, "vault:test"); !bytes.Equal(got, next) {
		t.Error("Set did not replace the item")
	}
}
