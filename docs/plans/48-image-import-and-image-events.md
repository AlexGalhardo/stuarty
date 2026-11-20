# 48: Image import and the image event stream

## Goal

A creator drags a photo from their computer into a new thumbnail in one step, and an editor open on that image shows what an agent or another window changed, without a reload.

## Context and related decisions

- ADR-0038 (image documents on the same core and renderer), ADR-0017 (the HTTP API and SSE), ADR-0006 (the operation log), ADR-0039 (a project holds its image documents).
- Author decision D-093. PRD-004 RF-330 and RF-341.
- SDD-021 section 10 (an image enters a document from the store today; importing a file straight into an image "arrives with the screen (epic 28) and the production (epic 06)"), section 11 (the service operations), section 12 (the limits).
- SDD-012 section 24.3 (the screen polls the revision every 2 seconds), section 24.13 decisions 3 and 5 (changes of another actor wait for an event stream; `ImageImport` is the operation the file import waits for).
- SDD-006 sections 2 to 6 (the event stream, `batch.committed`, `Last-Event-ID` and the replay from the log, the single-process hub), SDD-000 section 5 (event names) and section 14 (image names), SDD-003 section 2.6 (`MediaImport` and its path rules), section 4.2 (the `image` tool), section 6.3 (the routes), section 7.1 (the parity table).

## Scope

- Modules: `service/image.go` and `service/image_types.go` (`ImageImport`), `service/events/` and `web/app/events/` (the image topic and its publishing after a commit), `web/app/http/image.go`, `web/app/http/eventstream.go` and `web/app/http/endpoints.go` (the import and events routes), `web/app/cli/image.go` (`image:import`), `web/app/mcp/image.go` (the `import` action of the `image` tool), `web/resources/js/image-editor/api/`, `web/resources/js/image-editor/state/` and `web/resources/js/image-editor/ImageEditor.tsx` (the stream replaces the polling; the import actions), `web/resources/lang/en.json` and `pt-BR.json` (a shared file, one epic at a time), SDD-000 (sections 1, 5 and 14), SDD-003 (sections 4.2, 6.3 and 7.1), SDD-006 (a section for the image stream), SDD-012 (sections 24.3 and 24.13), SDD-021 (sections 10 and 11).
- `ImageImport`: an image file becomes a new image document (one image layer, fitted to the canvas and centred), or one more layer of an existing document when it is given an `image_id`.
- An event stream of each image document: one event per committed revision, in the shape of `batch.committed`, with replay after a reconnect.

## Out of scope

- Importing a video, an audio file, a `.cube` table (epic 49 uses the route of this epic when it exists) or a font into an image.
- A production item of kind `image` in a project (see the risks).
- A native file picker of the desktop shell beyond the path that the route already takes.
- The event stream of the video project (unchanged) and the events of jobs.

## Dependencies

27 (the image document core) and 28 (the image editor, which the stream updates). Not in progress together with 49 and 50: all three change `core/image`, `service/image*.go` and SDD-021.

## Technical design

- **`ImageImport`** (the name SDD-012 section 24.13 reserves). Request: `path` (an absolute path on the machine of the server, read with the path rules of `MediaImport`, SDD-003 section 2.6) or `hash` (an image already in the store, as `media_import` accepts library hashes), `image_id` (optional), `name`, `width` and `height` or `preset` (for a new document), `wait_ms` (as `media_import`), `base_revision`, `idempotency_key` and `dry_run`.
  - The file enters the content store by the ingest of SDD-011 section 4.7 (the normalized PNG, the proxy and the thumbnail). The limits of the ingest (`E_LIMIT`, `import_pixels`) apply unchanged.
  - Without `image_id`: a new document. The canvas is the size of the source when both sides are at most 4,096 pixels; otherwise `width` and `height`, or a `preset`, are required (`E_LIMIT` with a hint). The document and its first `layer.add_image` (fit `contain`, centred) are written in one transaction, so a failed import leaves no empty document.
  - With `image_id`: one batch of `layer.add_image`, with `base_revision`, `idempotency_key` and `dry_run` as in `ImageEdit`.
  - Scope `edit`. In network mode the owner rules of SDD-021 section 10 apply, and the server chooses the id.
- **Surfaces**, one operation, one action, one command and one route (SDD-000 section 1): MCP `image` with `action` `import`; CLI `image:import` (`--path`, `--id`, `--name`, `--width`, `--height`, `--preset`); HTTP `POST /images/import` with a JSON body. The `image` tool gains one enum value and its fields; the budget of SDD-003 section 4.8 measures it.
- **Browser upload.** The screen has no path on the server's disk, so a file chosen in the browser needs its own route. That route is a decision (see the risks); until it is taken, the browser shows the import as unavailable and says why.
- **Event stream.** `GET /images/{id}/events` (scope `read`, the `since` and `Last-Event-ID` rules of SDD-006 section 2), on the SSE hub under the topic `image:<id>`. The event `image.updated` has the revision as its id and carries `revision`, `actor`, `diff` (the diff of the batch, as `batch.committed`) and `at`. A dry run and a batch with an empty diff publish nothing (SDD-021 section 4). After a reconnect the replay reads the rows of `image_operations` (SDD-005 section 5.13) through the code path of SDD-006 section 4.3; a gap sends `resync.required` (SDD-006 section 3.6).
- **Names.** `image.updated`, the route and `ImageImport` are added to SDD-000 before the code (rule 3 of AGENTS.md).
- **The screen.** The editor opens the stream while it is visible and applies another actor's diff as a mechanical patch, or refetches on a gap, as SDD-012 section 5.4 says for the video editor. Its own commits are reconciled by revision and never applied twice (SDD-006 section 6.2). Polling stays only as the fallback when the stream is down; SDD-012 section 24.3 is amended.
- **Import actions.** The image list offers "Import image" (a new document) and the editor offers "Put a file in this image" (`image_id`). Both in English and Portuguese.

## Tasks

- [ ] 1. Amend SDD-000 (the operation `ImageImport`, the route, the event `image.updated` in a stream of an image), SDD-003 (section 4.2, action `import`; section 6.3, the routes; section 7.1, the table), SDD-006 (a section for the image stream), SDD-012 (sections 24.3 and 24.13) and SDD-021 (sections 10 and 11). Verify: `task lint:md` exits 0.
- [ ] 2. Check whether `project_items` already holds an image (SDD-005 section 5.12 says the kind is reserved), then implement `ImageImport` in `service/` with the atomic creation and the path rules, and a table test for each refusal. Verify: `go test ./service/... -run TestImageImport` passes.
- [ ] 3. Publish `image.updated` after every committed image batch, and serve `GET /images/{id}/events` with replay. Verify: `go test ./web/app/http/... -run TestImageEvents` passes, including the replay after `Last-Event-ID`.
- [ ] 4. Expose on MCP, CLI and HTTP. Verify: `go test ./web/contract/...` passes; `go run ./internal/tools/surfacebudget` exits 0; `task generate:check` exits 0.
- [ ] 5. Replace the polling by the stream in the editor; add the import actions in both languages. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.
- [ ] 6. End to end: two windows on one image, an edit by the CLI appears in the second window without a reload; a file is imported from the screen. Verify: `web/e2e/specs/image-live.spec.ts` (new) and `web/e2e/specs/image-import.spec.ts` (new) pass with `bun run e2e`, and `web/e2e/specs/a11y-image-editor.spec.ts` still passes.
- [ ] 7. The browser upload route, only when the author has decided it (see the risks). Verify: `go test ./web/app/http/... -run TestImageUpload` passes, including a file over the limit refused with `E_LIMIT`.

## Test strategy

- Service: import from a path and from a hash; each refusal (an unreadable path, a file that is not an image, over the ingest limit, an unknown hash, a path outside the allowed roots); atomicity (a failed layer leaves no document).
- Events: one event per committed revision, none for a dry run or an empty diff; replay of the missed revisions after a reconnect; `resync.required` after a gap.
- Contract: the same import through HTTP, CLI and MCP gives the same result (`web/contract`).
- End to end: two pages on one image; the live update; the import from the screen.

## Acceptance criteria

1. `image:import --path <png>` creates a document with one image layer whose asset is the content hash of the file: `go test ./service/... -run TestImageImport` passes.
2. A refused import (an unsupported format, an unknown hash, a file over the ingest limit) creates no document: `go test ./service/... -run TestImageImportIsAtomic` passes.
3. An agent's edit reaches an open editor within one second, with no reload, and with no polling request while the stream is up: `bun run e2e` passes on `web/e2e/specs/image-live.spec.ts`.
4. After a dropped connection, `Last-Event-ID` replays the missed revisions in order: `go test ./web/app/http/... -run TestImageEvents` passes.
5. The import is reachable through the MCP action `import`, the CLI command `image:import` and `POST /images/import`, with parity: `go test ./web/contract/...` passes.
6. The MCP tool list stays within the NFR-020 budget: `go run ./internal/tools/surfacebudget` exits 0.
7. `task lint:md`, `task generate:check`, `bun run check:i18n`, `bun run check:thin-client` and `task test:web` pass; `go test -run TestSecurity_ ./...` passes.

## Risks and mitigation

- **A browser file upload is a new untrusted input.** The screen cannot give a server path. **STOP** before task 7: the upload needs an entry in the threat model of SDD-007 and a decision of the author. Until then the browser shows the import as unavailable, with the reason.
- **A production item of kind `image`.** ADR-0039 says a project holds its image documents. SDD-005 section 5.12 still lists `image` as a reserved kind of `project_items`, and SDD-021 section 10 says the item waits for epic 06, which the roadmap marks done. Check the table before task 2. If an image must become a project item here, **STOP**: that is a change of scope, not part of this epic.
- **The NFR-020 budget.** The `image` tool gains an action. **STOP** if `surfacebudget` fails: only the author raises the budget (SDD-003 section 4.8, author decision D-094).
- **A single-process event hub.** The limits of SDD-006 section 5.5 apply; this epic does not change them.
- **Untrusted images.** The ingest and its FFmpeg run are the existing ones (SDD-011 section 4.7); no new parser is added.

## Size

M

## Hand-off for review

The diff of `epic/48-image-import-and-image-events`; the outputs of the acceptance commands; the report or recording of the two-window check; whether task 7 was done or stopped at its STOP.
