# ADR-0033: Secrets live in an encrypted file opened by a master key

- Status: Accepted (2026-11-18)
- Deciders: author
- Links: author decision D-058; ADR-0019, ADR-0020, ADR-0032; SDD-007

## Context

Stuarty stores OAuth tokens of social networks, API keys of AI providers and the credentials of blog destinations. Today the only secret is a local token in a file readable by its owner.

The operating system keychain exists on a desktop (Windows Credential Manager, macOS Keychain, the Secret Service on Linux). It does not exist on a headless Linux server, where there is no session bus, and it has size limits per item (research note `web/07-stack-desktop-web-self-hosted`). The product must behave the same on a desktop and on a VPS.

## Decision

1. **One vault file per workspace.** Every third-party credential is stored in one encrypted file inside the workspace. Nothing else in the workspace holds a credential in clear text.
2. **A master key opens it.** On a desktop the master key is kept in the operating system keychain. On a server it comes from a Docker secret or an environment variable, the same way `APP_KEY` already does. The key is generated at installation and never written into the workspace.
3. **Agents never see a secret.** A credential is used inside `service/` to make a call; no tool, command or endpoint returns one, and none is ever logged or accepted from a query string (ADR-0019 item 6).
4. **A lost key loses the connections, not the work.** Without the master key the vault cannot be opened; the user reconnects the accounts. Projects, media and documents are not encrypted by this vault.
5. **The cipher and the libraries are chosen by the epic** that builds the vault and need the author's approval as new dependencies.

## Alternatives

- **The keychain on a desktop and environment variables on a server.** Two behaviours, and keychain items are too small for some tokens.
- **Files with restricted permissions.** Rejected: secrets in clear text on disk and in every backup.

## Consequences

Positive: one format for both targets, and a copy of the workspace does not leak credentials without the key.

Negative: the installation must create, store and document the master key; a backup guide must say where it is.

Negative: every connector depends on this vault, so it is built before any of them.
