# 08: Asset library with origin, rights and reuse

## Goal

A creator stops hunting through folders: every image, clip, sound and font they have is in one library, with where it came from and whether it may be reused. An agent finds a reaction clip or a logo from a past production by searching, not by guessing a path.

## Context and related decisions

- ADR-0010 (content-addressed media store), ADR-0039 (assets belong to the workspace), ADR-0036 item 6 (provenance).
- Author decisions D-045 (reusable clips, agent-side downloads), D-064. PRD-004 RF-320 to RF-323; PRD-001 RF-004.
- Absorbs the earlier plans on the rights ledger and the evergreen clip library.

## Scope

- Modules: `service/assets/` (new package around the existing media operations), `core/asset/` (new: pure rules of origin, tags, reuse), `web/database/migrations/`, `web/app/` (controllers, CLI, MCP), `web/resources/js/pages/VisualAssets/` and `VideoAssets/`, SDD-011, SDD-005, SDD-000.
- Assets become workspace-wide; a project references the assets it uses.

## Out of scope

- Generating assets (epics 10, 11) and downloading from links inside the product (never: ADR-0020 item 6 stays; an agent downloads on its own side and imports a file).
- Editing an asset.

## Dependencies

06, 07.

## Technical design

- **Model**: `assets` gains workspace scope; `project_assets` (project id, asset id) records use. New columns or a side table for **origin**: `kind` (`imported`, `generated`, `fetched_by_agent`, `rendered_externally`), `source` (a path, a provider and model, a URL as plain text), `rights_note`, `created_by` (actor), `prompt` (generated), `cost`. Plus `tags` and `reusable` with a `description`.
- **Rules in `core/asset`**: validation of origin by kind (a generated asset must name provider and model; a fetched one must carry a source and a rights note), tag normalization, "used by" computation.
- **Service**: `AssetList` with filters (kind of media, tag, project, origin, reusable, text), `AssetGet`, `AssetSetInfo`, `AssetTag`, `AssetUses`; `MediaImport` accepts an optional origin block. Text results fit the token budget and mark source and prompt as untrusted text.
- **Migration**: existing per-project assets become workspace assets referenced by their project; content-addressed storage means no file moves.
- **Search**: SQLite full-text index over name, tags, description and origin text.
- **UI**: two sections over the same library, filtered by media kind: Visual Assets (images, fonts, documents) and Video Assets (video and audio); grid with thumbnails, filters, a detail panel with origin and uses, drag into an open timeline.

## Tasks

- [x] 1. Amend SDD-011, SDD-005 and SDD-000 (model, origin kinds, operations). Verify: `task lint:md` exits 0.
- [x] 2. Implement `core/asset` with table tests. Verify: `go test ./core/asset/...` passes.
- [x] 3. Write the migration to workspace assets with fixtures of earlier workspaces. Verify: `go test ./web/database/... -run TestWorkspaceAssetsMigration` passes.
- [x] 4. Implement the service operations, the origin on import and the search index. Verify: `go test ./service/assets/...` passes.
- [x] 5. Expose on HTTP, CLI and MCP with parity tests and untrusted-text marking. Verify: `go test ./web/contract/...` and `go test -run TestSecurity_ ./...` pass.
- [ ] 6. Build the two sections and the detail panel in both languages. Verify: `task test:web` and `web/e2e/specs/asset-library.spec.ts` pass.
- [ ] 7. Keep the editor's media library working on top of the new model. Verify: the existing library specs pass.

## Test strategy

- Unit: origin validation per kind; tag normalization; uses.
- Migration: earlier workspaces keep every asset and every clip still resolves its media.
- Contract: filters return the same set on the three surfaces.
- End to end: import with an origin, tag as reusable, find it from another project.

## Acceptance criteria

1. An asset imported in one project is visible and usable from another without a second copy on disk.
2. `AssetList` filtered by `reusable` and a tag returns the tagged asset on HTTP, CLI and MCP.
3. An import with `kind: fetched_by_agent` and no rights note is refused with a stable error.
4. The detail of an asset shows its origin and the projects that use it.
5. Source, prompt and rights note are returned as delimited untrusted text.
6. `go test ./core/asset/... ./service/assets/... ./web/database/... ./web/contract/...` and `task test:web` pass; the reference scenario still exports.

## Risks and mitigation

- **The editor loses a clip's media after the migration.** Mitigation: fixtures of real earlier workspaces; storage is by content hash, so only references change.
- **Rights notes give a false sense of permission.** Mitigation: the UI says the note is the user's own record, not a licence check.

## Size

M

## Hand-off for review

The diff of `epic/08-asset-library`; migration fixtures; the outputs of the acceptance commands; how to import, tag and find an asset on each surface.
