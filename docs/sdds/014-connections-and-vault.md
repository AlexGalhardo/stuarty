# SDD-014: Connections and the secret vault

- Status: Draft (2026-11-19). Written by epic 04; the code follows in the same epic.
- Depends on: ADR-0032 (outbound only to connected destinations), ADR-0033 (the vault), ADR-0019, ADR-0020, SDD-000 (names), SDD-007 (threat model, scope matrix, secrets, outbound paths)
- Implements: PRD-003 RF-206, RF-216, RF-220, NFR-046; PRD-002 RF-130, RF-137
- Owner area: `service/vault/`, `service/connections/`, `internal/outbound/`, the connection controllers, commands and tool in `web/app/`, the Connections page

## Contents

1. Overview
2. The connection record
3. The vault
4. The master key
5. Service operations and scopes
6. The outbound gate
7. Errors
8. Files, table and variables
9. Logs and the diagnostic bundle
10. Surfaces
11. Tests

## 1. Overview

A connection is a destination the user connected on purpose: a social account, an AI provider, a WordPress site, a webhook URL. This document specifies the record of a connection, the encrypted file that holds its credential, the key that opens that file, and the one place in the product that opens a connection to a destination (`internal/outbound`).

Three rules carry the design:

1. **With no connection, nothing leaves the machine.** The vault is created on the first credential, never at start-up, and the gate has no destination to reach.
2. **A request names its connection.** The gate refuses a host that is not in the destination list of that connection, and refuses every request when the connection is missing or removed, at the moment of the request.
3. **A credential never leaves `service/`.** It is read inside a closure that makes the outbound call. No result, tool, command, endpoint, log line or bundle carries one.

No third-party connector is built here. The registry of connector kinds has one kind, `test`, used by the tests and by the Connections page to show the whole path. Epics 09, 18 and 23 add real kinds.

## 2. The connection record

| Field | Type | Rule |
| --- | --- | --- |
| `id` | string | `con_` and 10 lower-case Crockford base32 characters (SDD-000 section 2) |
| `kind` | string | a kind of the connector registry; `test` is the only one in this epic |
| `label` | string | 1 to 60 characters, no control character; shown to the person, never used as an identifier |
| `hosts` | list of strings | 1 to 16 destination hosts: a lower-case DNS name or an IP literal, no scheme, no port, no path, no wildcard. A request may reach exactly these hosts, on any port |
| `accepts` | list of strings | what the destination may receive: any of `text`, `image`, `video`, `audio`, `metadata`, `document`; at least one. A connector refuses to send anything else |
| `created_at` | time | RFC 3339, UTC |
| `last_used_at` | time or null | set by the gate when a request goes through the connection |
| `secret_set` | boolean | whether the vault holds the credential of this connection. This is all a result says about the secret |

At most 50 connections per workspace (`E_LIMIT_EXCEEDED`). The reference to the secret in the vault is the id of the connection; it is stored in the database row (`secret_ref`) and never returned.

A connection is owned by the workspace, not by a user: in network mode only an `admin` creates or removes one, and every caller that may read can list them.

## 3. The vault

One file, `<workspace>/vault.json`, created with mode 0600 on the first `Put`. It is JSON so that a person can see it is not plaintext and a test can read its shape:

```json
{
  "format": "stuarty-vault",
  "version": 1,
  "salt": "<16 random bytes, base64>",
  "check": { "nonce": "<12 bytes, base64>", "data": "<base64>" },
  "entries": { "<entry id>": { "nonce": "<12 bytes, base64>", "data": "<base64>" } }
}
```

- **Cipher.** AES-256-GCM from the standard library. The key of the entries is derived from the master key with HKDF-SHA256 (`crypto/hkdf`), the salt of the file and the info string `stuarty-vault-entry-v1`. Each entry has its own random 12-byte nonce, and its additional data is `stuarty-vault/v1` followed by a zero byte and the entry id, so an entry cannot be moved to another id.
- **The check.** `check` is the encryption of a fixed text under the same key with the additional data `stuarty-vault/check`. It lets a wrong key be told apart from a corrupted file even when the vault has no entry.
- **What is in clear.** The format name, the version, the salt, the nonces and the entry ids. An entry id is a connection id (`con_...`), not a secret. A secret is at most 16 KiB.
- **Writes.** Each `Put`, `Delete` and `Rotate` rewrites the whole file through a temporary file in the same folder, a flush and a rename, so a crash leaves the old or the new file. One mutex serialises the operations of a process; the workspace lock (SDD-005 section 6.2) makes the process the only writer.
- **Interface.**

```go
type Vault struct{ /* path, key provider, mutex */ }

func Open(path string, keys KeyProvider) *Vault        // reads nothing until the first call
func (v *Vault) Put(id string, secret []byte) error    // creates the file and the key when the vault is new
func (v *Vault) Get(id string) ([]byte, error)         // ErrNotFound when the id is absent
func (v *Vault) Has(id string) (bool, error)           // reads the clear ids only; needs no key
func (v *Vault) Delete(id string) error                // absent is not an error
func (v *Vault) Rotate(newKey []byte) error            // re-encrypts every entry; the caller then saves newKey
```

- **Who may call `Get`.** `Get` is not part of `service.Service`. Only `service/connections` and the owner wiring import `service/vault`; `internal/tools/importcheck` fails the build when another package does.
- **Entry ids** match `^[a-z0-9_.:-]{1,80}$`.

## 4. The master key

A master key is 32 random bytes. A provider finds it and, for a new vault, may store a generated one:

```go
type KeyProvider interface {
	Name() string
	Load() ([]byte, error)   // ErrKeyNotFound when the provider holds none
	Save(key []byte) error   // ErrReadOnly when the provider cannot store one
}
```

The providers, in the order the owner tries them (the first that is configured is the only one used):

| # | Provider | Configured by | Stores a new key |
| --- | --- | --- | --- |
| 1 | the environment | `STUARTY_MASTER_KEY` | no (read only) |
| 2 | a file (a Docker secret) | `STUARTY_MASTER_KEY_FILE`, the path of a file | yes, when the file does not exist and its folder is writable; an existing file is never overwritten |
| 3 | the operating system keychain | nothing; used on a desktop when the platform has a store | yes |
| 4 | none | - | no |

A key in the environment or a file is 32 bytes written as base64 or as 64 hex digits, with surrounding white space ignored. Setting both variables is a start-up error (SDD-007 section 4.5). The keychain item is named `stuarty`, with the account `vault:` and 16 hex digits of the SHA-256 of the absolute workspace path, so two workspaces do not share a key. This build reads the Windows Credential Manager; the other platforms have no keychain client yet and use provider 1 or 2 (a question for the author is open, `development/QUESTIONS.md`).

Opening rules:

- **The vault file does not exist.** The provider is asked for a key; if it has none, a key is generated with `crypto/rand` and saved. If the provider cannot save one (read only, or provider 4), `Put` fails with `ErrNoKeyProvider`, whose message names `STUARTY_MASTER_KEY_FILE`.
- **The vault file exists.** The provider must return a key, else `ErrKeyMissing`; the key must open `check`, else `ErrWrongKey`. A file that does not parse or whose version is unknown is `ErrCorrupt`.
- The key is never written into the workspace, never logged and never returned. It is held in memory for the life of the process.
- **Recovery.** `ErrKeyMissing` and `ErrWrongKey` say: restore the key from where it was (the variable, the secret file or the keychain item), or start again by removing `vault.json` from the workspace and connecting the accounts again. Projects, media and documents are not encrypted by the vault and are not lost (ADR-0033 item 4).

## 5. Service operations and scopes

Five operations on `service.Service`, in `service/connections.go`:

| Operation | Scope | Result |
| --- | --- | --- |
| `ConnectionList` | read | `connections`: the records of section 2, oldest first |
| `ConnectionGet` | read | one record; `E_CONNECTION_NOT_FOUND` for an unknown id |
| `ConnectionCreate` | admin | the record created. The request carries `kind`, `label`, `hosts`, `accepts` and `secret`; the secret goes into the vault and is not in the result |
| `ConnectionRemove` | admin | the record removed. Deletes the vault entry first, then the row; the next request through the connection fails |
| `ConnectionTest` | admin | `ok`, `status` and a `message` made of the response body as untrusted text (SDD-007 section 8); the request goes through the gate |

`ConnectionCreate` and `ConnectionRemove` are audited (SDD-005 table `audit`) with the actor, the id, the kind and the label; the secret is never an audit field. `ConnectionTest` records `last_used_at`.

A project-limited key is refused for all five (they name no project and change the workspace).

## 6. The outbound gate

`internal/outbound` gains one constructor for a destination the user connected:

```go
type Destination struct{ ID string; Hosts []string }

type Registry interface {
	Destination(ctx context.Context, id string) (Destination, error) // ErrNoConnection when it does not exist
	Used(ctx context.Context, id string, at time.Time)
}

func Connection(reg Registry, id string, timeout time.Duration) *http.Client
```

For every request, and for every redirect hop (none is followed; a redirect is returned to the caller):

1. the registry is asked for the connection at that moment: a missing or removed connection is `ErrNoConnection` and no socket is opened;
2. the scheme is `https`, or `http` for a loopback host;
3. the host of the URL is in the connection's list (case-insensitive, exact), else `ErrDestination`; a URL with userinfo is refused;
4. at dial time the resolved address is checked: a link-local or unspecified address is refused (`ErrNotPublic`), so a name that resolves to a cloud metadata address cannot be reached; loopback and private addresses are allowed because a self-hosted server (a WordPress on the same network) is a legitimate destination the user typed;
5. the dial is shown to the `DialRecorder` with the purpose `connection`; `Used` is called once the request is allowed.

No proxy is used, TLS is verified, keep-alive is off. The paths of SDD-007 section 10.1 stay as they are: the transcription provider (path 1), the helper download (2), the OpenTelemetry exporter (3) and the desktop update (6) are chosen by the operator or by a person's explicit action, each through its own constructor of `internal/outbound`, so `internal/outbound` is still the only package that builds a client. They become connections when their epics need a credential (epics 09 and 12). The loopback clients of the CLI and the stdio MCP server (path 4) are unchanged.

`ConnectionTest` and every connector reach the network only through `connections.Manager.With`, which hands a closure the credential and the gated client of one connection.

## 7. Errors

Added to SDD-000 section 8:

| Code | Meaning | HTTP |
| --- | --- | --- |
| `E_CONNECTION_NOT_FOUND` | the connection does not exist or was removed | 404 |
| `E_CONNECTION_DESTINATION` | the request is for a host the connection does not list, or the scheme is not allowed | 403 |
| `E_VAULT_LOCKED` | the vault cannot be opened: the master key is missing, wrong or cannot be stored; `details.reason` is `key_missing`, `wrong_key`, `no_key_provider` or `corrupt`, and the hint says how to recover | 503 |

`E_VALIDATION` covers a bad kind, label, host or `accepts`, with `path` naming the field.

## 8. Files, table and variables

- The file `vault.json` is added to the workspace layout of SDD-000 section 9.
- The table `connections` (migration `20261119100000_create_connections_table`): `id` (primary key), `kind`, `label`, `hosts` (JSON text), `accepts` (JSON text), `secret_ref`, `created_ms`, `created_by`, `last_used_ms` (null). It holds no secret.
- The variables `STUARTY_MASTER_KEY` and `STUARTY_MASTER_KEY_FILE` (SDD-000 section 10). `doctor` reports them as `set` or `unset` and never their values, and reports which provider is active and whether a vault file exists.

## 9. Logs and the diagnostic bundle

A secret is never an argument of a log call, never a field of an error and never in a bundle. The master key and the vault file are not part of `doctor --bundle`; the bundle lists the connections by id, kind, label and hosts only. `TestSecurity_T42_SecretNeverInLogsOrBundle` seeds canaries as secrets and as the master key, runs the operations, a failing test connector and the bundle, and searches the logs, the answers and the bundle for them.

## 10. Surfaces

- HTTP: `GET /connections`, `GET /connections/{id}`, `POST /connections`, `DELETE /connections/{id}`, `POST /connections/{id}/test`.
- CLI: `connection:list`, `connection:show`, `connection:add`, `connection:remove`, `connection:test`. `connection:add` reads the secret from standard input or from a file named by `--secret-file`; it has no flag that takes the secret itself, so it never reaches a process list.
- MCP: one read-only tool, `connection`, that lists the connections or reads one. Creating, removing and testing a connection have no tool: they take a credential or reach the network, so they are for a person (the CLI and the Connections page).
- The UI page lists the connections with their hosts, what they accept and the last use, and adds, tests and removes them. The page computes nothing: it shows what `ConnectionList` says (SDD-012 section 1).

## 11. Tests

| What | Test |
| --- | --- |
| The vault file holds no known plaintext | `TestSecurity_T38_VaultFileHoldsNoPlaintext` |
| The master key is not in the workspace | `TestSecurity_T39_MasterKeyNeverInWorkspace` |
| A copy of the workspace without the key reveals no credential | `TestSecurity_T40_WorkspaceCopyRevealsNoCredential` |
| A connection is refused for another host, and stops when removed | `TestSecurity_T41_ConnectionRefusesOtherHostAndRemoved` |
| A secret is not in a log or a bundle | `TestSecurity_T42_SecretNeverInLogsOrBundle` |
| No surface returns a secret | `TestSecurity_T43_NoSurfaceReturnsASecret` |

Unit tests cover the vault round trip, a wrong key, a corrupted file, rotation, concurrent use and each provider; the gate has a fake destination server that records what it receives.
