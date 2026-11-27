# 110: Native Google Drive, Figma and open-source creative MCP connectors (ArtCraft, OpenPencil and custom MCPs)

## Goal

Creators can connect **Google Drive**, **Figma**, and external open-source creative **MCP servers** (with one-click presets for **ArtCraft**, **OpenPencil**, and custom stdio/HTTP MCP servers) right from Stuarty's Connections page. They can browse and import files from Google Drive or frames from Figma directly into Stuarty's Asset Library or Image Editor, export finished videos/images back to Google Drive with human approval, and let their hosted agents orchestrate ArtCraft, OpenPencil, and other open-source creative tools seamlessly alongside Stuarty.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #15 (integrations & open-source MCP part)**: Add native connections for **Google Drive**, **Figma**, and **MCPs of other open-source creative products** such as the **ArtCraft** suite (`storytold/artcraft` — PhotoCraft, VectorCraft, etc.) and **OpenPencil** (`ZSeven-W/openpencil` — AI-native open-source vector design tool with built-in MCP server), styled with clean branded connection cards like Cursor, Antigravity, and Claude.
- Architecture and security alignment:
  - **ADR-0032** (outbound only to connected destinations; exporting/writing externally waits for human approval; zero outbound when disconnected).
  - **ADR-0033** (OAuth tokens and personal access tokens sealed in the encrypted vault; never returned on any surface).
  - **ADR-0035** (Google Drive and Figma OAuth connections use the user's own developer app credentials, with step-by-step setup guides matching the existing `SetupGuide.tsx` pattern for YouTube/Instagram/X/TikTok; Figma also supports a Personal Access Token for quick single-user self-hosting).
  - **SDD-007 §6 & §8** (local stdio MCP servers run under the explicit process policy—no shell, absolute resolved binary, allowlisted env, tree kill, privileged-session-only registration—and all remote HTTP MCP servers pass through `connections.Manager.With` outbound host gate; every tool result returned by an external MCP server is treated as untrusted text!).
  - **Zero new Go dependencies**: Google Drive REST API v3, Figma REST API v1, and MCP JSON-RPC 2.0 (stdio + Streamable HTTP) are all implemented with Go's standard library (`net/http`, `encoding/json`, `os/exec`).

## Scope

- Modules owned:
  - `docs/sdds/000-shared-definitions.md`, `docs/sdds/014-connections-and-vault.md`, `docs/sdds/007-security-model.md` (new threats for external MCP execution, Drive upload approval, Figma image fetch SSRF, untrusted external tool outputs).
  - `core/connection/` (add connector kinds `google_drive`, `figma`, `mcp_server`; validation rules for external MCP server definitions—transport `stdio` or `http`, preset metadata, allowed tools/read-only flag).
  - `service/connectors/gdrive/` (Google Drive API v3 adapter: OAuth 2.0 PKCE with user's own app, `ListFiles`, `DownloadToAssetLibrary`, `UploadFromExport` behind human approval).
  - `service/connectors/figma/` (Figma API v1 adapter: OAuth 2.0 or Personal Access Token, `ListFiles`, `ListFrames`, `RenderFramePNG` -> import directly into Asset Library or as a layer in Image Editor).
  - `service/mcpclient/` (new MCP client manager: connects to user-configured external MCP servers over stdio or Streamable HTTP, lists tools, runs health checks, forwards external MCP server descriptors to hosted agent sessions where permitted, and exposes direct invoke/test endpoints with strict timeouts, payload caps, and untrusted output marking).
  - `service/connector_ops.go` (Service methods for Drive browse/import/export, Figma browse/import, and External MCP CRUD/test/tools-list).
  - `web/app/` (HTTP routes, CLI commands `drive:*`, `figma:*`, `mcp-ext:*`, and extend existing `connection` MCP tool actions within NFR-020 budget).
  - `web/resources/js/pages/Connections/` (`IntegrationsSection.tsx` with branded cards for Google Drive, Figma, ArtCraft, OpenPencil, and "+ Add custom MCP server", plus file/frame picker dialogs reusable from Asset Library and Image Editor).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `renderer/`, `engine/export/`, `desktop/`, `service/providers/`.

## Out of scope

- Auto-downloading or bundling ArtCraft or OpenPencil binaries (Stuarty never bundles third-party apps; presets detect installed local binaries or accept the user's running MCP endpoint URL and link to official install instructions).
- Two-way continuous folder sync with Google Drive (imports and exports are explicit user/approved actions).
- Google account sign-in to Stuarty itself (ADR-0052 keeps local account sign-in; Google Drive is strictly a storage connector like YouTube).

## Dependencies

- Epics 04, 84, 106, and 116 (vault/connections, Connections UI, project-independent Asset Library imports, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — update FigCraft frames for the Google Drive, Figma, and creative MCP cards).

## Technical design

1. **Google Drive connector (`service/connectors/gdrive/`)**:
   - Connection kind: `google_drive`, default hosts `www.googleapis.com`, `oauth2.googleapis.com`, `accounts.google.com`.
   - Scopes requested: minimal `drive.readonly` for browse/import, or `drive.file` when the user enables exporting Stuarty renders to Drive.
   - Operations:
     - `DriveList(connection_id, folder_id?, query?, cursor?)` -> lists folders and media files (video, image, audio) with untrusted names.
     - `DriveImport(connection_id, file_id, save_to_library)` -> streams file through gated client bounded by `STUARTY_MAX_MEDIA_BYTES` directly into Stuarty's Asset Library (triggering normal ingest + provenance `origin.kind = "imported"`, `origin.source = "google_drive:<file_id>"`).
     - `DriveExport(connection_id, export_id|asset_hash, folder_id?)` -> creates an approval-gated job (ADR-0032) that uploads the finished file to Drive once approved.

2. **Figma connector (`service/connectors/figma/`)**:
   - Connection kind: `figma`, default hosts `api.figma.com` and Figma's documented S3 image render host (`figma-alpha-api.s3.us-west-2.amazonaws.com` verified strictly by host allowlist).
   - Auth: OAuth 2.0 (user's own Figma app) OR Personal Access Token (`X-Figma-Token`), stored sealed in vault.
   - Operations:
     - `FigmaListFrames(connection_id, file_key)` -> reads document pages & top-level frames (`GET /v1/files/{key}?depth=2`).
     - `FigmaImportFrame(connection_id, file_key, node_ids[], format, scale, save_to_library, image_id?)` -> calls `GET /v1/images/{key}`, downloads PNG/JPG bounded by `STUARTY_MAX_MEDIA_BYTES`, saves into Asset Library, and optionally adds directly as a layer in an open Image Editor document.

3. **Open-Source Creative MCP Hub (`service/mcpclient/`)**:
   - Connection kind: `mcp_server`, supporting two transports:
     - **Streamable HTTP / SSE** (`url` + optional vault-sealed Bearer/header token, routed through `connections.Manager.With`).
     - **Local stdio** (privileged-session-only registration, absolute binary path resolved via `exec.LookPath`, allowlisted env, Windows `HideWindow: true` + `CREATE_NO_WINDOW`, process tree kill, 30s start / 60s call timeout, 1 MiB line cap).
   - Built-in **one-click presets** (text metadata + auto-detection on `PATH`, never auto-installing):
     - **ArtCraft** (`storytold/artcraft` — PhotoCraft / VectorCraft / creative IDE MCP server).
     - **OpenPencil** (`ZSeven-W/openpencil` — `op` / OpenPencil MCP server for `.op` vector design-as-code).
     - **Custom MCP Server** (any open-source stdio or HTTP MCP server).
   - Health check (`Test`) runs MCP `initialize` + `tools/list` and displays the discovered tool names and descriptions (as untrusted text!) directly on the card.
   - Hosted agent sessions (`SDD-017`) can optionally attach user-enabled external MCP servers (per-connection toggle `"Share with agents"`, off by default for safety) so the creator's agent can combine Stuarty + ArtCraft + OpenPencil in one workflow!

## Tasks

- [ ] 1. Amend SDD-000, SDD-014, and SDD-007 with `google_drive`, `figma`, and `mcp_server` connection contracts, security threats (stdio exec privilege, SSRF on Figma image URLs, Drive upload approval, untrusted MCP tool outputs), and MCP budget check. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Implement `core/connection/` validation for `google_drive`, `figma`, and `mcp_server` (presets metadata, transport validation, path/URL rules). Verify: `go test ./core/connection/...` passes.
- [ ] 3. Implement `service/connectors/gdrive/` and `service/connectors/figma/` with contract tests (recorded JSON/binary fixtures, size caps, SSRF host checks, Drive export approval gate). Verify: `go test ./service/connectors/...` passes with no network.
- [ ] 4. Implement `service/mcpclient/` (stdio + HTTP MCP JSON-RPC 2.0 client, Windows `CREATE_NO_WINDOW`, process-tree kill, timeouts, `initialize` + `tools/list` + `tools/call` test harness using a fake local MCP helper binary). Verify: `go test ./service/mcpclient/... ./service/...` passes.
- [ ] 5. Expose HTTP routes, CLI commands, and `connection` MCP tool actions with parity, security, and `surfacebudget` tests. Verify: `go test ./web/app/... ./web/contract/...`, `go test -run TestSecurity_ ./...`, and `go run ./internal/tools/surfacebudget` pass.
- [ ] 6. Build branded connection cards for Google Drive, Figma, ArtCraft, OpenPencil, and Custom MCP in `web/resources/js/pages/Connections/IntegrationsSection.tsx` plus Drive/Figma import pickers in Asset Library & Image Editor; add `en.json` + `pt-BR.json` strings and Playwright E2E specs. Verify: `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright specs pass.

## Test strategy

- Unit & contract tests (no network): fake Google Drive API server, fake Figma API + image CDN server, fake stdio and HTTP MCP servers; test SSRF rejection when Figma returns an unexpected download host; test `E_PRIVILEGED_SESSION` when a non-person tries to register a stdio MCP server.
- Integration tests: Drive export waits in `waiting_approval` until approved; Figma frame import stores asset with provenance in `library_assets`; MCP server shutdown leaves zero orphan child processes.
- E2E (Playwright): connect fake Google Drive, Figma, and OpenPencil MCP preset cards on `/connections`, inspect discovered tools, and import a fake Figma frame into Visual Assets.

## Acceptance criteria

1. `/connections` shows branded connection cards for **Google Drive**, **Figma**, **ArtCraft (MCP)**, **OpenPencil (MCP)**, and **Custom MCP Server**, matching the visual language of the AI provider cards.
2. Connecting Google Drive allows browsing files, importing media into Stuarty's Asset Library, and uploading finished exports (gated by human approval per ADR-0032).
3. Connecting Figma allows browsing frames of a file and importing rendered frames directly into the Asset Library or Image Editor.
4. Connecting ArtCraft, OpenPencil, or a custom MCP server (stdio or HTTP) verifies the connection via `initialize` + `tools/list`, displays discovered tools safely as untrusted text, and optionally exposes enabled servers to hosted agent sessions.
5. `go test ./core/connection/... ./service/connectors/... ./service/mcpclient/... ./service/...`, `go test -run TestSecurity_ ./...`, `go run ./internal/tools/surfacebudget`, `task lint`, `task test:web`, and `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n` pass.

## Risks and mitigation

- **Arbitrary command execution via stdio MCP servers.** Mitigation: apply the exact security controls of SDD-017 §3 (`E_PRIVILEGED_SESSION` person-only admin write, full command preview before save, absolute resolved binary path, no shell, allowlisted env, no secrets in env).
- **MCP `tools/list` byte budget (NFR-020).** Mitigation: extend existing `connection` tool actions rather than adding new top-level tools; **STOP** if `surfacebudget` exceeds the ceiling.
- **External tool prompt injection.** Mitigation: all external MCP tool names, descriptions, and outputs pass through `untrusted` delimiters and are rendered as plain text nodes only.

## Size

L

## Hand-off for review

The commits of `epic/110-drive-figma-and-creative-mcps`, updated SDD-014/SDD-007/SDD-000, Go/security/Vitest/Playwright outputs, `surfacebudget` output, and screenshots of the new Integrations & Creative MCPs cards.
