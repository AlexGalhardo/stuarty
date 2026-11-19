package vault

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows Credential Manager, through advapi32 (x/sys/windows has no
// wrapper for it). A generic credential is stored per user, with the
// workspace-specific target name; the blob is the base64 key.

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	maxBlob                 = 2560 // CRED_MAX_CREDENTIAL_BLOB_SIZE
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW  = advapi32.NewProc("CredReadW")
	procCredWriteW = advapi32.NewProc("CredWriteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

// credential is CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsKeychain struct{}

// OSKeychain is the Windows Credential Manager.
func OSKeychain() Keychain { return windowsKeychain{} }

func target(service, account string) (*uint16, error) {
	return windows.UTF16PtrFromString(service + "/" + account)
}

func (windowsKeychain) Get(service, account string) ([]byte, error) {
	name, err := target(service, account)
	if err != nil {
		return nil, err
	}

	var cred *credential

	r, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(name)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&cred)),
	)
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil, ErrKeyNotFound
		}

		return nil, callErr
	}

	defer procCredFree.Call(uintptr(unsafe.Pointer(cred))) //nolint:errcheck // nothing to do when freeing fails

	return append([]byte(nil), unsafe.Slice(cred.CredentialBlob, cred.CredentialBlobSize)...), nil
}

func (windowsKeychain) Set(service, account string, secret []byte) error {
	name, err := target(service, account)
	if err != nil {
		return err
	}

	if len(secret) > maxBlob {
		return ErrSecretTooLarge
	}

	user, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return err
	}

	cred := credential{
		Type: credTypeGeneric, TargetName: name, UserName: user, Persist: credPersistLocalMachine,
		CredentialBlobSize: uint32(len(secret)), //nolint:gosec // bounded by maxBlob above
	}
	if len(secret) > 0 {
		cred.CredentialBlob = &secret[0]
	}

	r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&cred)), 0)
	if r == 0 {
		return callErr
	}

	return nil
}
