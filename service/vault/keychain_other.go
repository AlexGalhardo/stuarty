//go:build !windows

package vault

// OSKeychain is the client of the operating system secret store, or nil when
// this build has none. Only Windows has one for now (the macOS Keychain and
// the Linux Secret Service are an open question for the author), so there the
// key comes from STUARTY_MASTER_KEY or STUARTY_MASTER_KEY_FILE.
func OSKeychain() Keychain { return nil }
