# 60: Video editor polish: dark menus, a wider sidebar, rename, the history sidebar

## Goal

A creator editing a video reads every menu in the dark theme, has room in the left sidebar, renames an imported file where they see it, and follows the history of the project in its own sidebar, newest first.

## Context and related decisions

- Author decision D-125 (2026-11-20), from the author's review of the editor: dropdowns in the dark theme show a light background; the left sidebar of the editor is cramped; the top strip with "Add Text" and "Add Shape" repeats the left sidebar; imported files cannot be renamed; the history is a tab, not a sidebar.
- Files are stored by content hash (`artifacts/<sha256>/`); the name a person sees lives in the database. Rename changes that name in Stuarty and the name used by everything that leaves it (export, download, "reveal"), never the stored file (D-125).
- ADR-0002 (the UI computes nothing), SDD-012 (the editor), SDD-011 (the library), SDD-000 (operations), SDD-003 (parity).

## Scope

- Modules: `web/resources/js/editor/` (menus, layout, the history sidebar), the library and asset rename in `core/`/`service/`, the adapters (HTTP, MCP, CLI), SDD-000, SDD-011, SDD-012, en and pt-BR strings.
- Must not touch: the dashboard text scale (`html[data-text-scale]`), the sidebar of the dashboard, sign-in (epic 57), the projects page (epic 56).

## Out of scope

- Renaming the stored files; a new storage layout; editing the history (undo stays as it is).

## Dependencies

08, 21 (merged).

## Technical design

- **Dark menus.** Every dropdown, select and context menu of the editor uses the popover tokens of the theme (`bg-popover text-popover-foreground`), so the dark theme shows a dark background and light text; a native `<select>` gets `color-scheme: dark` in the dark theme. Contrast AA checked by `check:contrast`.
- **Wider left sidebar.** Twice its current width, with the panels inside it using the room; it still collapses as today.
- **The top strip** with "Add Text" and "Add Shape" is removed; the same actions stay in the left sidebar.
- **Rename.** A right-click (and the keyboard context-menu key) on an imported file in the editor and in the library offers "Rename". A new operation (name it in SDD-000 first, e.g. `AssetRename`) changes the display name: 1 to 120 characters, no path separators or control characters, the extension kept. Exports, downloads and "reveal" use the name. Parity on HTTP, MCP (an action of an existing tool, no new tool: NFR-020) and CLI.
- **History sidebar.** The history becomes a sidebar right next to the left sidebar, collapsible with a button like the left sidebar, newest entry on top, each entry with its time, actor (person or agent) and what changed, as the core already describes it. Its open state is remembered per person.

## Tasks

- [x] 1. SDD-000, SDD-011 and SDD-012: the rename operation and the editor layout. Verify: `task lint:md` exits 0.
- [x] 2. The rename operation with its rules and parity tests. Verify: `go test ./core/... ./service/... ./web/...` passes and `go run ./internal/tools/surfacebudget` exits 0.
- [x] 3. Dark menus, the wider sidebar, the strip removed. Verify: `bun run typecheck`, `check:thin-client`, `check:contrast`, Vitest, and a Playwright spec that opens each editor menu in the dark theme and checks the computed background and text colors.
- [x] 4. Rename in the editor and the library by right-click and keyboard. Verify: a Playwright spec (rename, see the new name in the library and in an export file name).
- [ ] 5. The history sidebar. Verify: a Playwright spec (open, collapse, newest first after an edit).

## Acceptance criteria

1. In the dark theme every editor menu has a dark background and light text, AA contrast.
2. The left sidebar is twice as wide; the top strip is gone.
3. A file can be renamed by right-click on every surface, and the export uses the new name.
4. The history is a collapsible sidebar, newest first.
5. `task lint`, `task test`, `task test:web`, `bun run check:thin-client` pass; no new dependency.

## Size

M

## Hand-off for review

The commits, each acceptance criterion with the command and its result, screenshots of the editor in both themes.
