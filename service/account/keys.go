// Package account is the local account of a workspace (ADR-0052, SDD-007 section
// 3.9): the Argon2id key derivation, the data key and its two wrappings, the
// recovery key, the sealing of the fields of the store, and the Manager that
// holds the data key in memory while a person is signed in. The rules a field
// must follow are in core/account; nothing here is returned by a tool.
package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"slices"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"

	rules "github.com/AlexGalhardo/stuarty/core/account"
)

// Sizes, in bytes.
const (
	KeySize   = 32
	SaltSize  = 16
	NonceSize = 12
)

// The errors of the keys. None of them carries a value.
var (
	// ErrWrongPassword is a password that does not open the account. An unknown name is the same error.
	ErrWrongPassword = errors.New("account: the password is not right")
	// ErrWrongRecoveryKey is a recovery key that does not open the account.
	ErrWrongRecoveryKey = errors.New("account: the recovery key is not right")
	// ErrCorrupt is a record whose keys cannot be read.
	ErrCorrupt = errors.New("account: the account record is damaged")
)

// KDF is the Argon2id configuration stored next to the salt, so a later version can raise it.
type KDF struct {
	Alg       string `json:"alg"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memory_kib"`
	Threads   uint8  `json:"threads"`
	Salt      []byte `json:"salt"`
}

const kdfAlg = "argon2id"

// Params are the cost of one derivation. DefaultParams is what the product uses;
// the SDD records the time measured for it.
type Params struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultParams are the reference parameters of SDD-007 section 3.9.
var DefaultParams = Params{Time: 7, MemoryKiB: 256 * 1024, Threads: 4}

// Limits of the parameters a stored record may name, so a damaged or hostile
// record cannot make a sign-in allocate without bound.
const (
	maxKDFMemoryKiB = 1 << 20
	maxKDFTime      = 16
)

const (
	labelKEK      = "stuarty-account-kek-v1"
	labelVerifier = "stuarty-account-verifier-v1"
	labelRecovery = "stuarty-account-recovery-v1"
	aadAccount    = "stuarty/account/v1\x00"
)

// recoveryAlphabet is the Crockford base32 alphabet: no I, L, O or U.
const recoveryAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var recoveryEncoding = base32.NewEncoding(recoveryAlphabet).WithPadding(base32.NoPadding)

func newKDF(p Params) (KDF, error) {
	salt, err := random(SaltSize)
	if err != nil {
		return KDF{}, err
	}

	return KDF{Alg: kdfAlg, Time: p.Time, MemoryKiB: p.MemoryKiB, Threads: p.Threads, Salt: salt}, nil
}

func (k KDF) valid() bool {
	return k.Alg == kdfAlg && k.Time >= 1 && k.Time <= maxKDFTime && k.MemoryKiB >= 8 &&
		k.MemoryKiB <= maxKDFMemoryKiB && k.Threads >= 1 && len(k.Salt) == SaltSize
}

// derive runs Argon2id and splits its output into the key-encryption key and
// the verifier. Knowing the verifier does not give the key.
func derive(password string, k KDF) (kek, verifier []byte, err error) {
	if !k.valid() {
		return nil, nil, ErrCorrupt
	}

	master := argon2.IDKey([]byte(password), k.Salt, k.Time, k.MemoryKiB, k.Threads, KeySize)
	defer wipe(master)

	if kek, err = hkdf.Key(sha256.New, master, nil, labelKEK, KeySize); err != nil {
		return nil, nil, err
	}

	if verifier, err = hkdf.Key(sha256.New, master, nil, labelVerifier, KeySize); err != nil {
		wipe(kek)

		return nil, nil, err
	}

	return kek, verifier, nil
}

// recoveryKEK is the key-encryption key of a recovery key, which has full entropy and needs no Argon2id.
func recoveryKEK(recovery []byte) ([]byte, error) {
	return hkdf.Key(sha256.New, recovery, nil, labelRecovery, KeySize)
}

// FormatRecoveryKey writes 32 bytes as 13 groups of four characters of Crockford base32.
func FormatRecoveryKey(key []byte) string {
	text := recoveryEncoding.EncodeToString(key)

	var b strings.Builder

	for i := 0; i < len(text); i += 4 {
		if i > 0 {
			b.WriteByte('-')
		}

		b.WriteString(text[i:min(i+4, len(text))])
	}

	return b.String()
}

// ParseRecoveryKey reads what a person typed or pasted: case and the dashes and spaces between groups are
// ignored, and the letters Crockford folds together (I and L as 1, O as 0) are folded.
func ParseRecoveryKey(text string) ([]byte, error) {
	var clean strings.Builder

	for _, r := range strings.ToUpper(text) {
		switch r {
		case '-', ' ', '\t', '\r', '\n':
			continue
		case 'I', 'L':
			r = '1'
		case 'O':
			r = '0'
		}

		clean.WriteRune(r)
	}

	key, err := recoveryEncoding.DecodeString(clean.String())
	if err != nil || len(key) != KeySize {
		return nil, ErrWrongRecoveryKey
	}

	return key, nil
}

func random(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}

	return b, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// wrap seals plain under key: nonce || ciphertext || tag.
func wrap(key, aad, plain []byte) ([]byte, error) {
	aead, err := gcm(key)
	if err != nil {
		return nil, err
	}

	nonce, err := random(NonceSize)
	if err != nil {
		return nil, err
	}

	return aead.Seal(nonce, nonce, plain, aad), nil
}

// unwrap opens what wrap sealed; any failure is the one error, so a caller cannot tell why.
func unwrap(key, aad, sealed []byte) ([]byte, error) {
	if len(sealed) < NonceSize {
		return nil, errOpen
	}

	aead, err := gcm(key)
	if err != nil {
		return nil, err
	}

	plain, err := aead.Open(nil, sealed[:NonceSize], sealed[NonceSize:], aad)
	if err != nil {
		return nil, errOpen
	}

	return plain, nil
}

var errOpen = errors.New("account: cannot open")

func wrapAAD(label string) []byte { return []byte(aadAccount + label) }

// Keys is what an unlocked account holds: the key of the fields and the master key of the vault. The
// caller owns the buffers and wipes them with Wipe.
type Keys struct {
	Data  []byte
	Vault []byte
}

// Wipe zeroes both buffers.
func (k *Keys) Wipe() {
	wipe(k.Data)
	wipe(k.Vault)
	k.Data, k.Vault = nil, nil
}

func wipe(b []byte) { clear(b) }

// seal builds the record of a new account from fresh keys and returns the keys and the recovery key to show. The
// password is derived in its composed Unicode form (VV-380).
func seal(username, email, password string, p Params) (rec Record, keys Keys, recovery string, err error) {
	return sealSecret(username, email, rules.Normalize(password), p)
}

// sealSecret is seal for the exact bytes that are derived. Only a test calls it with a password that is not
// normalized, to make the record that an older version made.
func sealSecret(username, email, secret string, p Params) (rec Record, keys Keys, recovery string, err error) {
	kdf, err := newKDF(p)
	if err != nil {
		return Record{}, Keys{}, "", err
	}

	kek, verifier, err := derive(secret, kdf)
	if err != nil {
		return Record{}, Keys{}, "", err
	}

	defer wipe(kek)

	if keys.Data, err = random(KeySize); err != nil {
		return Record{}, Keys{}, "", err
	}

	if keys.Vault, err = random(KeySize); err != nil {
		wipe(keys.Data)

		return Record{}, Keys{}, "", err
	}

	recoveryRaw, err := random(KeySize)
	if err != nil {
		keys.Wipe()

		return Record{}, Keys{}, "", err
	}

	defer wipe(recoveryRaw)

	rkek, err := recoveryKEK(recoveryRaw)
	if err != nil {
		keys.Wipe()

		return Record{}, Keys{}, "", err
	}

	defer wipe(rkek)

	rec = Record{Username: username, Email: email, KDF: kdf, Verifier: verifier}

	if rec.KeyByPassword, err = wrap(kek, wrapAAD("data-by-password"), keys.Data); err == nil {
		if rec.KeyByRecovery, err = wrap(rkek, wrapAAD("data-by-recovery"), keys.Data); err == nil {
			rec.VaultKey, err = wrap(keys.Data, wrapAAD("vault"), keys.Vault)
		}
	}

	if err != nil {
		keys.Wipe()

		return Record{}, Keys{}, "", err
	}

	return rec, keys, FormatRecoveryKey(recoveryRaw), nil
}

// OpenWithPassword checks the password against the verifier, in constant time, and unwraps the keys.
func OpenWithPassword(rec Record, password string) (Keys, error) {
	keys, _, err := openWithPassword(rec, password)

	return keys, err
}

// openWithPassword tries the composed form of the password first. An account made before the normalization
// (VV-380) derived its key from the bytes as they were typed, so when the composed form fails the typed bytes
// and the decomposed form are tried too, and legacy says that one of them opened it: the caller then rewrites
// the verifier from the composed form. The extra derivations happen only for a password that has a character
// with two forms, and only after the first one failed.
func openWithPassword(rec Record, password string) (keys Keys, legacy bool, err error) {
	composed := rules.Normalize(password)

	candidates := []string{composed}
	for _, other := range []string{password, norm.NFD.String(password)} {
		if !slices.Contains(candidates, other) {
			candidates = append(candidates, other)
		}
	}

	for i, candidate := range candidates {
		keys, err = openWithSecret(rec, candidate)
		if !errors.Is(err, ErrWrongPassword) {
			return keys, i > 0 && err == nil, err
		}
	}

	return Keys{}, false, ErrWrongPassword
}

func openWithSecret(rec Record, secret string) (Keys, error) {
	kek, verifier, err := derive(secret, rec.KDF)
	if err != nil {
		return Keys{}, err
	}

	defer wipe(kek)
	defer wipe(verifier)

	if subtle.ConstantTimeCompare(verifier, rec.Verifier) != 1 {
		return Keys{}, ErrWrongPassword
	}

	data, err := unwrap(kek, wrapAAD("data-by-password"), rec.KeyByPassword)
	if err != nil {
		return Keys{}, ErrCorrupt
	}

	return openVault(rec, data)
}

// OpenWithRecoveryKey unwraps the keys with the recovery key.
func OpenWithRecoveryKey(rec Record, recoveryText string) (Keys, error) {
	raw, err := ParseRecoveryKey(recoveryText)
	if err != nil {
		return Keys{}, err
	}

	defer wipe(raw)

	rkek, err := recoveryKEK(raw)
	if err != nil {
		return Keys{}, err
	}

	defer wipe(rkek)

	data, err := unwrap(rkek, wrapAAD("data-by-recovery"), rec.KeyByRecovery)
	if err != nil {
		return Keys{}, ErrWrongRecoveryKey
	}

	return openVault(rec, data)
}

func openVault(rec Record, data []byte) (Keys, error) {
	vaultKey, err := unwrap(data, wrapAAD("vault"), rec.VaultKey)
	if err != nil {
		wipe(data)

		return Keys{}, ErrCorrupt
	}

	if len(data) != KeySize || len(vaultKey) != KeySize {
		wipe(data)
		wipe(vaultKey)

		return Keys{}, ErrCorrupt
	}

	return Keys{Data: data, Vault: vaultKey}, nil
}

// rewrap returns rec with the data key wrapped by a new password: a new salt, a new verifier, a new wrapping.
// The recovery wrapping and the sealed fields do not change.
func rewrap(rec Record, dataKey []byte, newPassword string, p Params) (Record, error) {
	kdf, err := newKDF(p)
	if err != nil {
		return Record{}, err
	}

	kek, verifier, err := derive(rules.Normalize(newPassword), kdf)
	if err != nil {
		return Record{}, err
	}

	defer wipe(kek)

	wrapped, err := wrap(kek, wrapAAD("data-by-password"), dataKey)
	if err != nil {
		wipe(verifier)

		return Record{}, err
	}

	rec.KDF, rec.Verifier, rec.KeyByPassword = kdf, verifier, wrapped

	return rec, nil
}

// rewrapRecovery returns rec with the data key wrapped by a new recovery key, and that key to show. The password
// record and the sealed fields do not change, and the old recovery key opens nothing any more.
func rewrapRecovery(rec Record, dataKey []byte) (Record, string, error) {
	raw, err := random(KeySize)
	if err != nil {
		return Record{}, "", err
	}

	defer wipe(raw)

	rkek, err := recoveryKEK(raw)
	if err != nil {
		return Record{}, "", err
	}

	defer wipe(rkek)

	wrapped, err := wrap(rkek, wrapAAD("data-by-recovery"), dataKey)
	if err != nil {
		return Record{}, "", err
	}

	rec.KeyByRecovery = wrapped

	return rec, FormatRecoveryKey(raw), nil
}
