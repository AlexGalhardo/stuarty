# 116: Preserve the FINAL FigCraft design system and frames in `docs/design/final/`

## Goal

The canonical FigCraft design source (`stuarty-redesign-final-trilha.figcraft`), its Tailwind v4 token stylesheet (`tokens-tailwind-final.css`), and an English design-system index live permanently inside `docs/design/final/` under version control, so every future UI epic edits and versions new screens directly in FigCraft and the full visual history of Stuarty stays preserved in git.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback):
  - Keep the FINAL FigCraft design reference permanently inside `docs/` tracked by git (instead of only in git-ignored local folders), so it remains part of the repository's history and serves as the live source of truth for all upcoming UI epics (epics 101 to 118).
  - Every new UI screen or major layout update (standalone editors, OAuth-first agent setup, branded provider/MCP cards, Activity History, Screen Recording studio page, Professional Sound Editor, Canva/Photoshop and video editor tools) must update the versioned FigCraft file before or alongside implementation so design and code never drift apart.
- AGENTS.md rule 8 (everything committed in the repository is in English) and `.agents/rules/development-folder.md` (nothing in `docs/` may reference git-ignored local paths).
- Git repository hygiene: the master `.figcraft` file (`~11 MB`) and `tokens-tailwind-final.css` (`~10 KB`) are the canonical editable sources; bulk generated PNG exports (~100 MB across 540 files) should not bloat git unless the author explicitly confirms committing PNG snapshots or using Git LFS (included as a STOP condition in task 1).

## Scope

- Modules owned:
  - `docs/design/final/` (new directory: `stuarty-redesign-final-trilha.figcraft`, `tokens-tailwind-final.css`, `README.md` in English documenting all pages/frames and the FigCraft editing workflow, and curated English summary synced with `docs/design-system.md`).
  - `docs/design-system.md` and `docs/README.md` (link to `docs/design/final/stuarty-redesign-final-trilha.figcraft` and `docs/design/final/tokens-tailwind-final.css` as the canonical versioned UI reference).
  - `.agents/rules/naming.md` or `.agents/rules/plans.md`? Not touched unless approved by the author (`REVIEW.md` marks `.agents/rules/` as human-approved).
- Must not touch: `core/`, `service/`, `engine/`, `renderer/`, `web/`, `desktop/`.

## Out of scope

- Implementing product code or editing existing UI components (owned by the individual UI epics).
- Committing raw scratch scripts (`_work/`, `*.mjs`, `*.sh`) or Portuguese local audit logs into `docs/`.

## Dependencies

- Epic 99 (final design screens shipped).

## Technical design

1. **Canonical versioned layout (`docs/design/final/`)**:
   - `docs/design/final/stuarty-redesign-final-trilha.figcraft` — the single editable FigCraft source of truth containing `01 Design System`, all desktop/mobile product screens (including frames 24a–24f for Screen Recording), states, and landing frames.
   - `docs/design/final/tokens-tailwind-final.css` — the canonical Tailwind CSS v4 design tokens matching `web/resources/css/app.css`.
   - `docs/design/final/README.md` — concise English guide listing the FigCraft pages, frame inventory, token parity check against `web/resources/css/app.css`, and the workflow for adding/updating frames in future UI epics.
2. **Documentation linkage**:
   - Add `docs/design/final/` to the reading map in `docs/README.md` and link it from `docs/design-system.md`.
   - Verify `task lint:md` passes cleanly on all newly committed Markdown files (zero Portuguese prose and zero references to git-ignored paths).
3. **Token drift verification**:
   - Verify every CSS custom property in `docs/design/final/tokens-tailwind-final.css` matches `web/resources/css/app.css` (or document any intentional difference in `docs/design/final/README.md`).

## Tasks

- [x] 1. Confirm with the author whether `docs/design/final/` should store only the canonical sources (`stuarty-redesign-final-trilha.figcraft`, `tokens-tailwind-final.css`, English `README.md`) or also selected PNG reference captures (~100 MB for all 540 PNGs). **STOP** if committing >20 MB of PNG binaries is requested without Git LFS confirmation. Verify: decision recorded in the hand-off.
- [x] 2. Copy `stuarty-redesign-final-trilha.figcraft` and `tokens-tailwind-final.css` from the local FINAL design bundle into `docs/design/final/`, and write the English `docs/design/final/README.md` (with zero local-folder path references and zero scratch scripts copied). Verify: `git status docs/design/final/` shows only the clean canonical files and `task lint:md` exits 0.
- [x] 3. Update `docs/design-system.md` and `docs/README.md` to reference `docs/design/final/stuarty-redesign-final-trilha.figcraft` and document the FigCraft update step for all new UI epics. Verify: `task lint:md` exits 0 and no public document under `docs/design/final/` cites a git-ignored local path.

## Test strategy

- Markdown lint (`task lint:md`) across `docs/`.
- Repository cleanliness check: verify zero scratch `.mjs`/`.sh` files, zero Portuguese markdown files, and zero references to git-ignored local folders inside `docs/design/final/`.
- Token consistency check comparing `docs/design/final/tokens-tailwind-final.css` against `web/resources/css/app.css`.

## Acceptance criteria

1. `docs/design/final/stuarty-redesign-final-trilha.figcraft`, `docs/design/final/tokens-tailwind-final.css`, and `docs/design/final/README.md` exist in git and are linked from `docs/README.md` and `docs/design-system.md`.
2. Zero scratch scripts (`_work/`, `.mjs`, `.sh`), zero Portuguese prose, and zero references to git-ignored paths exist under `docs/design/final/`.
3. `task lint:md` exits 0.

## Risks and mitigation

- **Git repo bloat from committing 540 PNG exports (~100 MB).** Mitigation: commit the editable `.figcraft` file and CSS tokens as the single source of truth; **STOP** at task 1 if the author wants all PNGs committed too.
- **Drift between FigCraft and shipped UI.** Mitigation: every subsequent UI epic (101–118) includes updating `docs/design/final/stuarty-redesign-final-trilha.figcraft` for its new screens before final hand-off.

## Size

S

## Hand-off for review

The commits of `epic/116-figcraft-design-source-in-docs`, file size summary under `docs/design/final/`, token comparison notes, and `task lint:md` output.
