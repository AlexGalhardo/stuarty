# 94: Asset storage size, show in folder and rename, with their edge cases

## Goal

A creator sees how much disk each asset takes and how much each library takes in total, opens the folder on their computer where an asset's file is stored, and renames any asset on its own card, with every action refused clearly when it would break a project, leak a path or let an agent act as a person.

## Context and related decisions

- Requested by the author on 2026-11-24, on top of epic 81 (asset library screens, done in v0.50.0).
- What exists today:
  - `AssetRename` (`service/asset_ops.go`, SDD-011 §14.8) changes the display name of a file in every project of the caller that holds it. The stored file is never renamed. `RenameMenu` is on `MediaCard` and `SoundRow`.
  - The library rows carry `Bytes` from `asset_info.size_bytes` (`service/asset_library.go`), and the card shows a size.
  - The desktop shell has `showInFolder` (`desktop/shell/bridge.go`, `reveal.go`): Explorer with the file selected, Finder, or the folder on Linux. It accepts any absolute path to an existing regular file that the page sends. The export dialog uses it today.
- The UI never computes a rule (SDD-012 §1, `bun run check:thin-client`). Agents never gain a person's privilege (ADR-0051, SDD-007). Files and paths stay on the user's machine (ADR-0032, no telemetry). Sealed fields of epic 57 stay sealed.
- A project can be text only (author decision D-134): a library or a project with no media is a normal state.

## Scope

1. **Size.** Every asset shows its size in a human unit (B, KB, MB, GB, decimal, one rule in `core/`), in the card, the sound row and the detail. Each library (visual, video, sound) shows its file count and total size, and the detail of an asset says how many projects use the same file. A file shared by several projects or assets by content hash is counted once in a total, and the detail says so.
2. **Show in folder.** On the desktop app, a "Show in folder" action on the card menu and in the detail opens the file manager with the stored file selected. The page sends the asset id, never a path: the service resolves the stored path and checks it. In the browser (local mode) the action is replaced by "Copy path", and only for a person session on the local machine. In network mode neither appears and no route returns a path.
3. **Rename.** Rename from the card, the sound row and the detail, with the rules below, the same result on every surface, and undo-free semantics stated in the UI (the name changes, the file does not).
4. **Audit of edge cases** (each one a test that names it):
   - Size: a file missing on disk (shown as "missing", size unknown, never 0), a file still importing (size unknown until ready), a size over 4 GiB, a generated asset, a library in a locked studio (the read answers `E_LOCKED` like every other).
   - Show in folder: an asset in the trash (refused with a clear reason), a missing file (refused, with "the file is missing" and no path), a file outside the workspace media folder (refused: the resolved path must be under the workspace's media root after symlink resolution), a path with control or bidirectional characters (shown isolated), an agent or an API key caller (refused with `E_PRIVILEGED_SESSION`; there is no MCP tool), the studio locked (refused with `E_LOCKED`), a desktop without a file manager (a clear error).
   - Rename: an empty or blank name, a name over the title limit, control and bidirectional characters (refused or isolated as SDD-000 rules say), a name equal to the current one (no-op with no audit row), two assets with the same name (allowed, they are told apart by size and date), an asset in use by an open editor (the editor shows the new name through its event, nothing breaks), an asset in the trash (refused), a rename racing another rename (last write wins with the revision rule of SDD-011, no lost update), an agent rename (allowed with the edit scope, as today, and audited with the actor), a locked studio (`E_LOCKED`).

## Out of scope

Moving or renaming the stored file on disk, choosing where files are stored, deleting files from the folder view, quotas, and sealing library files (epic 92).

## Dependencies

81 (done), 57 (done). Coordinate with 85 and 86 if they are in progress: this epic changes only the asset library and the detail panel.

## Technical design

- `core/asset`: `FormatSize(bytes)` and the total rule (count each content hash once), with table tests.
- `service`: `AssetLibraryList` returns `total_bytes`, `file_count` and, per row, `bytes` (nullable when unknown) and `state` (`ready`, `importing`, `missing`); `AssetReveal(ctx, AssetRevealRequest{AssetID})` returns the checked absolute path only for a person session in local mode, otherwise a stable error. The check resolves symlinks and requires the media root as prefix.
- HTTP: `POST /api/v1/assets/{id}/reveal` (person session, local mode only; 404 in network mode). The desktop page calls it and passes the returned path to `showInFolder`; `desktop/shell` keeps `CheckRevealPath` and adds the media-root check by asking the server through the same route, never trusting the page.
- CLI: `asset:path <id>` prints the path for the local owner (same rule). MCP: no tool, documented in SDD-003 as a person-only operation.
- UI: size in `MediaCard`, `SoundRow` and `AssetDetail`; library header total; "Show in folder" (desktop) or "Copy path" (browser, local); rename on all three, using `RenameMenu`. Every refusal shows the service message and hint.
- SDD-000 (names, error codes), SDD-011 §14 (size, reveal, rename rules), SDD-007 (a threat row for path disclosure and reveal by an agent, with `TestSecurity_` tests), SDD-012 §23.5.

## Tasks

- [x] 1. SDD-000, SDD-011, SDD-007 and SDD-012. Verify: `bunx markdownlint-cli2@0.24.1 docs/sdds/*.md` exits 0 and `go run ./internal/tools/threatcheck` exits 0 once task 4 adds the tests.
- [x] 2. `core/asset` size format and totals. Verify: `go test ./core/asset/...` passes with a table that includes 0, 999, 1000, 4 GiB and unknown.
- [x] 3. Library totals, row state and size in `service`, with the missing and importing cases. Verify: `go test ./service/... -run 'AssetLibrary'` passes.
- [ ] 4. `AssetReveal` with every refusal of the scope, the HTTP route and the CLI command, and the security tests. Verify: `go test ./service/... ./web/app/http/... ./web/app/cli/... -run 'Reveal|AssetPath'` and `go test -run TestSecurity_ ./...` pass.
- [ ] 5. Rename edge cases on every surface (service, HTTP, CLI, MCP) with parity. Verify: `go test ./web/contract/... -run 'Asset'` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 6. UI: size, totals, Show in folder or Copy path, rename on card, row and detail, every refusal shown. Verify: in `web/`, `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `bun run test -- assetlibrary` pass.
- [ ] 7. End to end and desktop. Verify: `bun run e2e -- asset-rename asset-library-states` passes with `--workers=2`, and a desktop check (or the bridge test in `web/resources/js/lib/desktopBridge.test.ts` plus `go test ./desktop/shell/...`) shows the reveal path goes through the server check.

## Test strategy

Unit tests for the size rule and the path check (including a symlink out of the media root and a path with control characters); service and HTTP tests for every refusal; contract parity for rename; security tests for an agent, an API key and network mode; e2e for the library and the detail in both themes.

## Acceptance criteria

1. Every asset shows its size, a missing or importing file says so instead of 0, and each library header shows the file count and the total, with a shared file counted once.
2. On the desktop, "Show in folder" opens the file manager at the stored file; the page never sends a path, and a path outside the media root, a trashed asset or a missing file is refused with a clear message.
3. In the browser in local mode, "Copy path" gives the stored path to a person; in network mode no route, tool or page reveals a path.
4. An agent, an API key or a locked studio can never reveal or copy a path, and the refusal is stable and documented.
5. Rename works from the card, the row and the detail, gives the same result on every surface, refuses the invalid names of the scope, and an open editor shows the new name without breaking.
6. `task lint`, `task test`, `task test:web` and `go test -run TestSecurity_ ./...` pass; no new dependency; the MCP and `SKILL.md` budgets are not raised.

## Risks and mitigation

- A path shown in the browser is personal data on screen: only for the local person, never logged, never in an event.
- **STOP** if revealing a path in the browser needs a new kind of session or credential, if the desktop shell cannot reach the server check, or if a budget would have to be raised.

## Size

M

## Hand-off for review

The diff against the open release branch, the commands above with their results, screenshots of the three libraries and the detail in both themes at 1440 and 390 px, and the list of edge cases with the test that covers each.
