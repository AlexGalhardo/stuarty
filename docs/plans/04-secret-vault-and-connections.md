# 04: Secret vault and connections

## Goal

A creator can let Stuarty hold the keys to their accounts without fear: every credential is encrypted on disk, no agent can read one, and one screen shows every place the product is allowed to talk to. Until they connect something, nothing leaves their machine.

## Context and related decisions

- ADR-0032 (outbound only to connected destinations), ADR-0033 (the vault), ADR-0019, ADR-0020 items 2 to 8.
- Author decisions D-052, D-058. PRD-002 RF-130, RF-133, RF-137; PRD-003 RF-206, RF-216, RF-220, NFR-046; PRD-004 NFR-060.
- Research note `web/07-stack-desktop-web-self-hosted` (keychains, headless servers).

## Scope

- Modules: `service/vault/` (new), `service/connections/` (new), `internal/outbound/` (the one place that opens a connection), `service/config`, `web/app/` (controllers, CLI and MCP for connections), `web/database/migrations/`, the Connections page in `web/resources/js/pages/Connections/`, `docs/sdds/007-security-model.md`, a new `docs/sdds/014-connections-and-vault.md`.
- A connection is a record: kind, label, the destination hosts it may reach, what it may receive, when it was created and last used, and a reference to its secret in the vault.
- No real third-party connector here: a test connector and the model are enough. Connectors come with epics 09, 18 and 23.

## Out of scope

- The approval queue and cost ceilings (epic 05).
- OAuth flows of any provider or network (epics 12, 18, 24).
- Encrypting projects or media.

## Dependencies

03.

## Technical design

- **Vault** (`service/vault`): one file in the workspace, authenticated encryption per entry, opened by a 256-bit master key. Interface: `Put(id, secret)`, `Get(id)`, `Delete(id)`, `Rotate(newKey)`. `Get` is callable only from `service/` packages that make the outbound call; it is not part of `service.Service`.
- **Master key provider**: on a desktop, the operating system keychain; on a server, `STUARTY_MASTER_KEY` or a file named by `STUARTY_MASTER_KEY_FILE` (a Docker secret). First start generates the key. A missing key with an existing vault is a clear error that says how to restore the key or reset the connections.
- **Libraries**: the keychain and the cipher need dependencies. Propose them with licence and maintenance status and STOP for the author's approval before adding them (candidates and their state are in the research note).
- **Connections** (`service/connections`): `ConnectionList`, `ConnectionGet`, `ConnectionCreate`, `ConnectionRemove`, `ConnectionTest` on `service.Service`, with scope `admin` to create and remove. Results never include a secret, only whether one is set.
- **Outbound gate** (`internal/outbound`): every outbound request names its connection; the gate refuses a host that is not in that connection's destination list and refuses any request when the connection is missing or removed. Loopback and the explicit helper downloads of RF-205 keep their existing rules. The `no-egress` script is updated to the new rule.
- **Untrusted text**: a response body from a destination is returned through `service/untrusted` like media text.
- **Threat model**: add rows to SDD-007 for the vault file, the master key, a stolen workspace copy, a connection used for another host, and a secret in a log or a diagnostic bundle; each row names its test.

## Tasks

- [x] 1. Write SDD-014 (model, interfaces, errors, files) and add its names to SDD-000; add the threat rows to SDD-007. Verify: `task lint:md` exits 0 and `go run ./internal/tools/threatcheck` lists the new rows as needing tests.
- [x] 2. Propose the keychain and cipher libraries and wait for approval. Verify: the approval is recorded in the hand-off.
- [x] 3. Implement the vault with the file format and rotation. Verify: `go test ./service/vault/...` passes, including a test that the file never contains a known plaintext.
- [x] 4. Implement the master key providers (keychain, variable, file). Verify: `go test ./service/vault/... -run TestMasterKey` passes on Windows and in the Linux Docker test (`bash scripts/test-linux-docker.sh ./service/vault/...`).
- [ ] 5. Implement connections, their migration and the service operations with scopes. Verify: `go test ./service/connections/... ./web/database/...` passes.
- [ ] 6. Put the outbound gate in `internal/outbound` and route every existing outbound call through it. Verify: `go test ./internal/outbound/...` passes and `bash scripts/no-egress.sh` passes with no connection configured.
- [ ] 7. Expose connections on HTTP, CLI and MCP, with parity tests. Verify: `go test ./web/contract/... -run TestParity` passes; `UPDATE_SURFACE=1` is used for the new names.
- [ ] 8. Build the Connections page (list, add, test, remove, last used) in both languages. Verify: `bun run typecheck`, `bun run check:i18n`, `task test:web` and the new spec `web/e2e/specs/connections.spec.ts` pass.
- [ ] 9. Make the diagnostic bundle and the logs free of secrets. Verify: `go test -run TestSecurity_ ./...` passes with the new tests of the SDD-007 rows.

## Test strategy

- Unit: vault round trip, wrong key, corrupted file, rotation, concurrent access; gate decisions.
- Security suite: one `TestSecurity_` test per new threat row.
- Integration: a fake destination server that records what it receives; a removed connection stops traffic at once.
- End to end: the Connections page spec.

## Acceptance criteria

1. With no connection, `bash scripts/no-egress.sh` passes and the product opens no outbound connection.
2. A request through a connection to a host outside its destination list fails with a stable error code and reaches no network.
3. `strings` on the vault file finds none of the test secrets; a copy of the workspace without the master key reveals no credential (`TestSecurity_` test).
4. No `ConnectionList`, `ConnectionGet`, MCP tool, CLI command or HTTP route returns a secret.
5. Removing a connection deletes its vault entry and the next request through it fails.
6. `go run ./internal/tools/threatcheck` exits 0 with the new rows covered.
7. The Connections page lists a connection with its destinations and last use, in English and in Portuguese.

## Risks and mitigation

- **A lost master key locks every connection.** Mitigation: the error explains the recovery; the installation documents where the key is.
- **An outbound path that skips the gate.** Mitigation: a test fails the build when a package other than `internal/outbound` opens a non-loopback connection (`internal/tools/importcheck`).
- **STOP** before adding a dependency (task 2), and if the keychain cannot be reached without cgo on a platform.

## Size

L

## Hand-off for review

The diff of `epic/04-secret-vault-and-connections`; the approved libraries; the outputs of the acceptance commands; how to create a test connection and watch the gate refuse another host.
