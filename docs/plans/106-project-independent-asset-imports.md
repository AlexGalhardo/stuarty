# 106: Project-independent drag-and-drop asset imports, library copy prompt and project allocation details

## Goal

A creator can drag and drop or browse files from their computer straight into Visual Assets, Video Assets, or Sound Assets—without having to create a project first and without ever having to type a raw file path. Every manual import and AI-generated file is saved cleanly in Stuarty's assets folder, loose assets can be linked into any project in one click, each asset's detail panel clearly shows every project (and timeline/item) where it is allocated, and importing a new file always shows a confirmation modal asking whether to keep a copy in the Stuarty asset library.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #5**:
    1. Allow easy import of files and assets into Stuarty without being tied to a pre-created project.
    2. Support drag-and-drop and computer file picker everywhere (video, image, audio, LUT), never forcing the user to type a filesystem path as the primary/only method (path input may remain as an optional collapsed "Advanced / server path" fallback for CLI/VPS/agents).
    3. Store all manual imports and AI-generated media in Stuarty's assets folder (`<workspace>/store/` / managed assets library) so loose assets persist independently of projects.
    4. Make it effortless to link loose assets into projects (and unlink/remove from a project without losing the library asset).
    5. In the Details panel of every asset, clearly display where that asset is allocated across projects (project name, link, and specific timeline/image/publication slot where it is used; or "Unallocated / Library only" when loose).
    6. Whenever the user imports a new file (audio, image, video, etc.)—whether from the Asset Library, Video Editor, Image Editor, or a Project—show a confirmation modal asking: `"Você quer guardar uma cópia desse import na sua biblioteca de assets do Stuarty?"` / `"Do you want to keep a copy of this import in your Stuarty asset library?"` (with clear choices: keep copy in library vs use in current session/project only, plus a quick preview of the filename/type/size).
- Current limitations verified in the codebase:
  - `service/asset_library.go:75` (`libraryScan`) currently scans ONLY `ListProjectAssets` across existing projects and `libraryScanProjectsWithNoTimeline`; there is NO first-class workspace-level loose-asset table, so an asset cannot exist in the library unless it belongs to a project!
  - `web/resources/js/assetlibrary/AssetLibrary.tsx:794` only renders an "Import" button on `section === "audio"` (`SoundImport.tsx`), and `SoundImport.tsx` forces the user to pick an existing project AND type a raw filesystem path (`<Input value={path} />`)! Video Assets and Visual Assets have NO upload/import button at all!
  - `AssetDetail` (`AssetLibrary.tsx:603-641`) lists project links, but does not distinguish specific allocations (timeline name, image document name, or "Library only — not allocated to any project yet") and lacks a one-click unlink/allocate UX for loose assets.
- ADR-0002 (rules in `core/asset/`), ADR-0010 (content-addressed media store), ADR-0039, SDD-000, SDD-005, SDD-007 §7 (file import & upload limits, pathguard, untrusted filenames), SDD-011 §14.

## Scope

- Modules owned:
  - `core/asset/` (rules for loose library assets, allocation records across projects/timelines/images, and `save_to_library` import policy).
  - `web/database/migrations/` (add `library_assets` table storing workspace-owned loose assets by `asset_hash`, `owner_user_id`, `display_name`, `kind`, `sound_kind`, `save_to_library` / `pinned`, `created_ms`, so assets live in the library independently of any project).
  - `service/asset_library.go`, `service/media.go`, `service/generate.go`, `service/image_import.go`, `service/assets/` (`AssetImport` / `AssetUpload` standalone projectless import accepting multipart browser upload or desktop path; auto-register AI-generated assets in `library_assets`; enrich `AssetGet` `uses` with exact allocation details: `project_id`, `project_name`, `target_kind` `"timeline"|"image"|"project_pending"|"publication"`, `target_id`, `target_title`; add `AssetUnlinkFromProject`).
  - `web/app/` (multipart upload HTTP endpoint `POST /api/v1/assets/upload` + JSON `POST /api/v1/assets/import`, CLI `stuarty asset:import <file...>`, MCP `asset` tool action within NFR-020 budget).
  - `web/resources/js/assetlibrary/` (`AssetDropzone.tsx` + `LibraryCopyModal.tsx` reusable across all sections, replace path-only `SoundImport.tsx` with unified drag-and-drop + file-picker import on **Visual Assets**, **Video Assets**, and **Sound Assets**, upgrade `AssetDetail` allocation list and project linker).
  - `web/resources/js/editor/library/` and `web/resources/js/image-editor/import/ImportDialog.tsx` (wire the same `LibraryCopyModal` confirmation when importing files inside the video editor, image editor, or project AddMedia dialog, and make drag-and-drop + file picker the primary UX with path collapsed under "Use server path").
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`, `docs/sdds/000-shared-definitions.md`, `docs/sdds/005-data-model-and-persistence.md`, `docs/sdds/007-security-model.md`, `docs/sdds/011-ingest-and-media.md`.
- Must not touch: `renderer/`, `engine/export/`, `desktop/update/`, `service/providers/`.

## Out of scope

- Double-click on asset card to launch editors (owned by epic 104).
- Standalone video/image editing canvases (owned by epic 105).
- Cloud storage imports from Google Drive or Figma (owned by epic 110).

## Dependencies

- Epics 08, 81, 94, and 116 (asset library, screens, storage size/reveal/rename, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — update FigCraft frames for the dropzone, library copy modal, and project allocation details).

## Technical design

1. **Workspace-owned loose assets (`library_assets` table + `service/asset_library.go`)**:
   - New table `library_assets(asset_hash, owner_user_id, name, kind, sound_kind, created_ms, trashed_ms)` indexed by `(owner_user_id, asset_hash)`.
   - `libraryScan` unions `library_assets` (active rows) with project holdings and image document assets so:
     - An asset imported directly into the Asset Library (or generated by AI without a project) is immediately visible in Visual/Video/Sound Assets with `projects: 0` ("Library only").
     - Removing an asset from a project does NOT delete it from the Asset Library if it is kept in `library_assets`.
     - All AI generations (image, voice, video) automatically insert a row in `library_assets` so generated media is always saved in the assets folder and discoverable in the library.

2. **Streaming upload + desktop native picker (`POST /api/v1/assets/upload`)**:
   - Web mode: streams `multipart/form-data` directly to a workspace temp file bounded by `STUARTY_MAX_MEDIA_BYTES` (never buffering whole videos in RAM), sniffs MIME/container with `engine/ffmpeg` probe, content-addresses into `<workspace>/store/`, runs normal ingest (normalized media/proxy/thumbnail/peaks), and registers in `library_assets`.
   - Desktop mode: uses `window.vukemDesktop.pickFiles` / `onDropPaths` to pass native paths straight to `POST /api/v1/assets/import` without copying through HTTP multipart, while keeping the exact same confirmation modal and drag-and-drop visual UX.
   - Optional `save_to_library: boolean` flag (default `true` when importing from the Asset Library; chosen by the user in `LibraryCopyModal` when importing from an editor or project):
     - `true`: registers the content-addressed copy in `library_assets` so it remains in Visual/Video/Sound Assets across all projects.
     - `false`: scopes the asset only to the target project/timeline/image document (standard project-scoped ingest, cleaned up by garbage collection if that project is deleted).

3. **Confirmation modal (`LibraryCopyModal.tsx`)**:
   - Triggered whenever the user selects or drops one or more files in any import surface (Asset Library, Video Editor library, Image Editor import, Project AddMedia).
   - Displays the file name(s), detected media kind, size, and the exact prompt:
     - `pt-BR`: `"Você quer guardar uma cópia desse import na sua biblioteca de assets do Stuarty?"`
     - `en`: `"Do you want to keep a copy of this import in your Stuarty asset library?"`
   - Offers two clear actions: **"Keep copy in library"** (`save_to_library: true`, primary) and **"Use only here"** (`save_to_library: false`, when importing inside a project/editor; inside the Asset Library itself the primary action confirms saving into the library and Cancel aborts).

4. **Allocation details in `AssetDetail`**:
   - Extend `AssetUse` in `core/asset/` and `service/asset_library.go` with `allocations: []AssetAllocation` (`{ project_id, project_name, target_kind, target_id, target_title }`), covering video timelines, image documents, and pending project items.
   - In `AssetDetail`, render each project with its exact allocations (e.g., `"Launch Video -> Timeline: Main Cut"`, `"Thumbnail Pack -> Image: YouTube Cover"`) with clickable links and one-click "Link to project" / "Unlink" controls.

## Tasks

- [ ] 1. Specify `library_assets`, `POST /api/v1/assets/upload`, `save_to_library`, and `AssetAllocation` in SDD-000, SDD-005, SDD-007 (upload threat checks), and SDD-011 §14. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Implement core rules in `core/asset/`, DB migration for `library_assets`, and service operations (`AssetUpload`, `AssetImportStandalone`, `AssetUnlink`, enriched `AssetGet` allocations, auto-library registration for AI-generated assets). Verify: `go test ./core/asset/... ./service/... ./web/database/...` passes.
- [ ] 3. Expose HTTP multipart upload, CLI `asset:import`, and MCP `asset` tool actions with parity, security, and budget checks. Verify: `go test ./web/app/... ./web/contract/...`, `go test -run TestSecurity_ ./...`, and `go run ./internal/tools/surfacebudget` pass.
- [ ] 4. Build `AssetDropzone.tsx` and `LibraryCopyModal.tsx`, replace path-only `SoundImport.tsx`, and enable drag-and-drop + file-picker import across Visual Assets, Video Assets, and Sound Assets plus upgraded project allocation details in `AssetDetail`. Verify: `cd web && bun run typecheck && bun run check:thin-client && bunx vitest run resources/js/assetlibrary/`.
- [ ] 5. Wire `LibraryCopyModal` and drag-and-drop / file-picker-first import into `ImportDialog.tsx` (Image Editor), Video Editor media import, and `Projects/AddMedia.tsx`, collapsing raw path entry under an optional "Server path" disclosure. Verify: `cd web && bun run check:i18n && bunx vitest run`.
- [ ] 6. Add Playwright E2E tests covering: (a) dragging/selecting an image, video, and audio file into the Asset Library with 0 projects -> confirmation modal appears -> asset stored in library -> linked into a new project -> allocation shown in Asset Details; (b) importing inside an editor with "Keep copy" vs "Only here". Verify: `task lint`, `task test`, `task test:web`, and the Playwright specs pass.

## Test strategy

- Unit tests: `core/asset` allocation grouping, loose-asset filtering, filename sanitization; `service` multipart upload size cap (`E_PAYLOAD_TOO_LARGE`), invalid MIME rejection, AI-generated asset auto-save in `library_assets`, unlink preserving library copy.
- Security tests: upload traversal filename (`../../etc/passwd`), oversized multipart rejected before full disk write, scope checks (`read` refused, `media` allowed), cross-user isolation in network mode.
- Web & E2E: Vitest tests for `AssetDropzone`, `LibraryCopyModal`, and `AssetDetail` allocations; Playwright E2E verifying drag-and-drop/file picker across all 3 asset sections and confirmation modal wording in `en` and `pt-BR`.

## Acceptance criteria

1. With zero projects created, a user can drag and drop or browse a video, image, or audio file directly in Video Assets, Visual Assets, or Sound Assets without typing any filesystem path.
2. Every file import in the UI opens the confirmation modal asking `"Você quer guardar uma cópia desse import na sua biblioteca de assets do Stuarty?"` (in `pt-BR`) / `"Do you want to keep a copy of this import in your Stuarty asset library?"` (in `en`) before completing the import.
3. Confirmed manual imports and all AI-generated assets are stored in Stuarty's assets store, persist in the Asset Library independently of any project, and can be linked into any project in one click.
4. Opening Details on any asset displays every project and specific timeline/image document where that asset is allocated (or clearly states it is not yet allocated to any project).
5. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright asset import specs pass.

## Risks and mitigation

- **Disk growth from unallocated library copies.** Mitigation: reuse existing content-addressed deduplication (`sha256` store, ADR-0010) so keeping a copy in the library and linking it into 10 projects stores the bytes on disk only once, and display exact storage usage (epic 94).
- **Large video uploads through browser multipart.** Mitigation: stream directly to disk with `io.CopyN` bounded by `STUARTY_MAX_MEDIA_BYTES`, never `io.ReadAll`, and use native path hand-off in Desktop mode.
- **STOP** if `surfacebudget` exceeds NFR-020 when extending the `asset` MCP tool.

## Size

L

## Hand-off for review

The commits of `epic/106-project-independent-asset-imports`, migration, security test results, `surfacebudget` output, Vitest/Playwright outputs, and screenshots of the drag-and-drop zone, confirmation modal, and Asset Detail allocations panel.
