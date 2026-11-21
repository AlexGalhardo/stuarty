# 57: A local account and an encrypted workspace

## Goal

The first time a creator opens Stuarty they create an account with a password, and from then on their projects, texts, memory and credentials are encrypted with a key that only exists while they are signed in, so nobody and no agent reads their work while the studio is locked.

## Context and related decisions

- Author decision D-120 (2026-11-20). It amends D-117 (a session opened with the password is a person) and, on that point, ADR-0032 and ADR-0033; this epic writes the ADR that records it first.
- Today local mode has no sign-in: the vault key comes from the operating system or `STUARTY_MASTER_KEY_FILE` (ADR-0033, SDD-007), network mode has password sign-in and Sign in with Google (SDD-007), and the `stuarty open` link gives an unprivileged browser session (SDD-007 section 5.3 item 7).
- The store is SQLite through `modernc.org/sqlite` (no cgo), which cannot encrypt the whole file; `golang.org/x/crypto` is already a module dependency (Argon2id); AES-256-GCM is in the Go standard library. No new dependency.
- The web UI: the sidebar has a language dropdown at the top (`web/resources/js/dashboard/Sidebar.tsx`); there is a profile page.

## Scope

- Modules: `service/` (accounts, sign-in, lock and unlock, the key in memory, `E_LOCKED`), the store (an account table, field encryption, the migration of an existing workspace), the vault (its key wrapped by the account key), `web/app/` (the first-run and sign-in pages, the session, the routes gated by the lock), the CLI and MCP (`E_LOCKED`), `web/resources/js/` (create account, sign in, recovery key, Settings, Logout, the sidebar footer), the desktop shell (it opens the sign-in page), SDD-000, SDD-007, SDD-005, SDD-012, a new ADR, the user docs, the threat model.
- Must not touch: the projects page and the project fields (epic 56); field encryption of the new project description and goal waits until epic 56 is integrated, then this epic adds them to the encrypted set.

## Out of scope

- Several local accounts; a confirmation email or any check that leaves the machine; encrypting media files; changing the YouTube or AI connections.

## Dependencies

04 (vault), 05 (approvals), 07 (sessions). Next to epic 56 on separate modules (see Scope).

## Technical design

- **Account.** One per installation: `username` (4 to 16, lowercase letters only), `email` (12 to 48, format checked locally: local part, `@`, a domain with a dot and a top-level part of two letters or more), password (8 to 32, at least one uppercase, one lowercase, one digit and one special character) and its confirmation, both with a show/hide eye. Rules in the core, the same on every surface. Only an Argon2id verifier of the password is stored, never the password.
- **Keys.** Argon2id (parameters in the SDD, tuned for about 0.5 s on the reference machine) derives a key-encryption key from the password; it wraps a random data key. A recovery key, shown once at account creation, wraps the same data key, so a forgotten password with the recovery key recovers the data; without both, nothing is recoverable, and the page says so plainly. Changing the password rewraps the data key; nothing is re-encrypted.
- **What is encrypted.** The vault, and the user content fields of the database (texts and their revisions, briefs, chat messages, memory notes, prompts and skills bodies, captions text, publication texts; the project description and goal once epic 56 is in) with AES-256-GCM, a fresh nonce per write, the row id as associated data. Ids, timestamps, states and sizes stay readable for the queries. Media files are not encrypted.
- **Lock.** The data key lives only in the server's memory after sign-in and is wiped on Logout, on server exit and after an idle timeout the user can set in Settings. While locked, every route, CLI command and MCP tool that reads or writes user data returns `E_LOCKED` ("unlock Stuarty with your password"); the sign-in page and the health check stay open. No tool ever returns the password, the data key or the recovery key.
- **Person.** A browser or desktop session opened by typing the password is a person (approve, set a price, keep a memory note, mark a skill as read). The `stuarty open` link and every agent, key and MCP caller are not.
- **Google sign-in** is removed (routes `google.*`, the sign-in button, its OAuth flow, the docs); network mode keeps password sign-in. The YouTube connection stays.
- **Migration of an existing workspace.** On first start of this version with data and no account, the create-account page appears; after creation the server copies the database, encrypts the existing fields, reads every encrypted field back, and only then deletes the copy; on any failure it restores the copy and reports.
- **UI.** First run: create account, then the recovery key page (copy, confirm it was saved), then sign in. The sidebar loses the language dropdown; its footer gets **Settings** (language, the signed-in user, change password, light/dark theme, idle lock) and **Logout**.

## Tasks

- [ ] 1. The ADR (amends D-117, ADR-0032 and ADR-0033 on the person session and the vault key) and SDD-000, SDD-007, SDD-005, SDD-012, the threat model entries. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. The account, the rules and the keys (Argon2id, wrap, recovery key, change password), with tests of every boundary of the rules and of a wrong password. Verify: `go test ./core/... ./service/...` passes.
- [ ] 3. Lock and unlock in the server, `E_LOCKED` on every data route, CLI command and MCP tool, the idle timeout. Verify: a test that walks every registered operation while locked and expects `E_LOCKED`, except the allowed list.
- [ ] 4. Field encryption in the store and the vault key wrapped by the data key. Verify: a test that reads the SQLite file directly and finds no plaintext of a text, a memory note or a credential.
- [ ] 5. The migration of an existing workspace with backup, verification and restore. Verify: tests for success and for a failure in the middle.
- [ ] 6. Remove Google sign-in; the person session for a password sign-in. Verify: `go test -run TestSecurity_ ./...` passes and a test that the `stuarty open` session stays unprivileged.
- [ ] 7. The UI: create account (the eye on both password fields, errors under each field), recovery key, sign in, Settings, Logout, the sidebar footer. Verify: `bun run typecheck`, `check:thin-client`, `check:i18n`, Vitest and Playwright specs of the whole first run and of Logout.
- [ ] 8. After epic 56 is integrated: add the project description and goal to the encrypted fields. Verify: the plaintext test of task 4 covers them.

## Acceptance criteria

1. A new workspace shows the create-account page before anything else; the dashboard is reachable only after sign-in.
2. While locked, no route, CLI command or MCP tool returns or changes user data (`E_LOCKED`), proved by a test over every operation.
3. The SQLite file and the vault hold no plaintext of user content or credentials.
4. The password and the recovery key each unlock the data; a wrong one does not; changing the password keeps the data.
5. An existing workspace migrates without loss, and a failed migration restores it.
6. A password session can approve in the browser; the `stuarty open` link and agents cannot.
7. Google sign-in is gone; network mode password sign-in still works.
8. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...` pass; no new dependency.

## Risks and mitigation

- Data loss from a forgotten password: the recovery key, shown once with a confirmation step, and plain words on the page.
- Agents that run unattended stop working while the studio is locked: `E_LOCKED` says how to unlock, and the idle timeout is the user's choice (it can be off).
- A query that needs an encrypted field: stop and report it; do not decrypt in SQL.

## Size

XL

## Hand-off for review

The commits, each acceptance criterion as done or not with the command and its result, the Argon2id parameters measured, how to try the first run, and anything that contradicts a specification.
