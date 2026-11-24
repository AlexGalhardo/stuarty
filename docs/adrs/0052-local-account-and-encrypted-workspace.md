# ADR-0052: A local account with a password opens the studio, and its key encrypts the user's content

- Status: Accepted (2026-11-20)
- Deciders: author
- Amends: ADR-0032 item 4 (what makes a session a person), ADR-0033 items 2 and 4 (where the vault key lives, and what a lost key loses), ADR-0034 items 1 and 3 (local mode has a sign-in, and Google sign-in is removed), ADR-0019 item 1 (local mode has no login)
- Links: author decisions D-120, D-117; ADR-0019, ADR-0032, ADR-0033, ADR-0034, ADR-0040; SDD-005, SDD-007, SDD-012; epic 57

## Context

Local mode opened with no sign-in: the workspace was readable by anyone who reached the machine, a launch link gave an editor session, and the vault key sat in the operating system keychain or in a file that the product itself could read at any time. D-117 let the desktop shell and the browser it opens count as a person. A creator who keeps unpublished work, briefs, chats with an agent and the keys of their accounts in Stuarty asked (D-120) for the studio to be closed by a password: when it is locked, nobody and no agent reads the work.

The store is SQLite through `modernc.org/sqlite`, which cannot encrypt the whole file. `golang.org/x/crypto` (Argon2id) is already a dependency and AES-256-GCM is in the standard library, so the design needs no new dependency.

## Decision

1. **One local account per installation, created on the first run.** A username, an e-mail address (format checked on the machine, nothing is sent) and a password. The rules are in `core/account` and are the same on every surface. Only an Argon2id-derived verifier of the password is stored, never the password.
2. **A data key, wrapped twice.** A random 256-bit data key encrypts the user's content. It is stored only wrapped: once by a key derived from the password (Argon2id), and once by a recovery key that is shown a single time at creation. Changing the password re-wraps the data key; nothing is re-encrypted. A forgotten password with the recovery key recovers the data; without both, nothing is recoverable, and the pages say so in plain words.
3. **What is encrypted.** The vault (its master key is now a random key stored wrapped by the data key, and the connections are re-sealed under it), and the fields of the database that hold the user's own words: the body of text items (texts, briefs, captions) and of their revisions, the messages of the chat transcripts and the title of a chat session (its first message), the text and the per-network fields of a publication, the body of a post published to a blog destination, the input and the result of a job that keeps words (a generation, a draft, a transcription), the summary of an approval request, the description and the five written parts of the goal of a project. AES-256-GCM, a fresh nonce per write, the table, the column and the row id as associated data. Ids, timestamps, states and sizes stay readable, so every query keeps working: no query ever needs an encrypted field, and nothing is decrypted in SQL. Media files are not encrypted.
4. **The key exists only while a person is signed in.** The data key lives in the memory of the server process after a sign-in and is wiped on Logout, on exit and after an idle timeout the user sets (it can be off). While locked, every operation of the Service, every route, CLI command and MCP tool that reads or writes user data answers `E_LOCKED`; the sign-in page, the account routes and the health check stay open. No surface ever returns the password, the data key, the recovery key or a wrapped key.
5. **A session opened by typing the password is a person.** It is privileged (it can approve, set a price, keep a memory note, mark a skill as read). A launch code, `stuarty open`, the local token, an API key, a hosted agent and every MCP caller are not: they reach the data only while a person has unlocked the studio, and never gain a privilege. This replaces D-117 (the desktop shell's own launch is no longer a person; the person types the password in the window).
6. **Google sign-in is removed.** Network mode keeps its password sign-in (ADR-0019 item 3 stands for it). The YouTube connection stays (ADR-0035). Network mode is not part of the encrypted workspace: it is run by an operator on a server, with several users and unattended starts; its data stays protected by the vault master key of ADR-0033 and the operator's disk.
7. **An existing workspace is migrated at the account's creation.** The server copies the database and the vault first, encrypts the existing fields, reads every encrypted field back, and only then deletes the copies, together with the older copies of the database that the schema migrations kept in clear; on any failure it restores them and reports.
8. **While no account exists, the workspace behaves as before** (clear fields, the vault key from its provider), and the interface sends the person to create the account before anything else. A fresh `STUARTY_MASTER_KEY_FILE` is read only to migrate an existing vault.

## Alternatives

- **An encrypted SQLite build (SQLCipher or a VFS).** Rejected: cgo or a new dependency; the product builds without cgo.
- **Encrypting every column.** Rejected: ids and states are needed in queries and in the event stream, and they hold nothing the user wrote.
- **One key from the password, with no data key.** Rejected: a password change would re-encrypt everything, and a recovery key could not exist.
- **No recovery key.** Rejected by the author: a forgotten password would lose the work with no way back, for a creator who is not a system administrator.

## Consequences

Positive: a copy of the workspace or of `db.sqlite` does not leak the user's words or credentials; an agent cannot read the studio while it is locked.

Negative: agents that run unattended stop while the studio is locked (`E_LOCKED` says how to unlock; the idle timeout is a choice). Scheduled publications wait for the next unlock; a target whose time passed is marked missed by the rules of SDD-020.

Negative: the library of memory, prompts and skills is files with a search index (ADR-0040). The index holds the bodies and the search is full text; encrypting it needs a decision that this ADR does not take (see the open question of epic 57). Until then those files stay as they are, and the SDD says so.

Negative: the documents of timelines and images are not sealed, and neither is the edit log that repeats them. The engine reads them, their digests are compared and the start-up migrates them with no key, so sealing them is a design of its own. Until it is decided, the captions on a video, its text layers and the text of an image document are readable in `db.sqlite`, and the user documentation says so.

Negative: the memory of a Go process cannot be wiped with certainty; the key buffers are zeroed on lock, and the residual risk is in SDD-007 section 14.
