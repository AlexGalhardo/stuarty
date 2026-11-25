# 99: Finish the final design where the screens are still partial

## Goal

Every screen the final design draws shows what the frame shows: the data the screens lack comes from the service, and the editors get the stages and controls the redesign left out.

## Context and related decisions

- The audit of 2026-11-25 (`development/redesign/FINAL/auditoria-0.57/AUDITORIA.md`, local) found no screen equal to the final design; groups 1 to 5 of the redesign shipped in v0.57.0 and v0.58.0 and left the parts below "partial" (author decisions D-154 to D-156).
- ADR-0007 and SDD-012 §1: the UI computes no result; any value a screen shows comes from the service. SDD-000 names first.
- NFR-020: the MCP tool list is at its byte budget; new data goes into existing reads where it can, not into new tools.
- Out of this epic, waiting for the author: API keys in local mode (20), local models (31), recording as a page (24a to 24f), external motion (32).

## Scope

- Group A, the data: `core/`, `service/`, the HTTP, CLI and MCP adapters, `docs/sdds/`.
- Group B, the video editor: `web/resources/js/editor/**` (not `editor/record/**` or `editor/teleprompter/**`).
- Group C, the image editor: `web/resources/js/image-editor/**`, `pages/EditImage/*`.
- Group D, the screens that use group A's data: the projects, writing, asset library, skills, memory, approvals and publishing pages.
- Must not touch: `renderer/`, the capture helper.

## Out of scope

The four screens that wait for the author; any rule computed in TypeScript; a new MCP tool.

## Dependencies

Groups 1 to 5 of the redesign (v0.57.0, v0.58.0). D depends on A.

## Technical design

- **A. Data the screens lack**, each added to an existing read where possible and named in SDD-000 first:
  1. The tags in use in the asset library, with their counts (the tag filter becomes a picker).
  2. The kind of a sound asset (music, voice, effect), set at import or by the person, so the sound library can filter by it.
  3. Whether a skill is loaded in the current agent session, so the skills list can say Loaded.
  4. Who wrote a memory note (the agent's display name) and the project's name, so the row reads as the design does.
  5. The production flow facts: post type, word count, reading time, the scheduled date of the next post, and per step the skill used and the cost spent.
  6. The zoom keyframes of a screen take as a list (count and times), so the finishing panel can list them.
- **B. The video editor stages**: 26 edit by text with the transcript as the stage and the review at the right; 27 vertical cuts with the proposal cards on the stage and the framing and caption panel; 30 studio voice with the before and after waveforms; 28 the caption inspector with Style, Text and Time tabs; 29 the region grid (X, Y, W, H) and a strength control whose presets come from the core; 25 the list of zooms from A6.
- **C. The image editor**: a command palette (Ctrl K) over the operations the editor already offers; the tool rail with the design's eight primary tools and the rest behind a "more" control; the zoom bar centred under the image with minus, percentage, plus and Fit.
- **D. The screens with A's data**: the asset tag picker, the sound kind filter, Loaded skills, the memory row with the agent's name and the project, the flow badges and the per-step skill and cost.

## Tasks

- [x] 1. A: SDD-000, the SDDs of each domain, then core and service with tests, then the adapters. Verify: `go test ./core/... ./service/...`, `go test -timeout 40m ./web/contract/`, `go run ./internal/tools/surfacebudget`.
- [x] 2. B: the stages and inspectors of 25 to 30. Verify: `bun run typecheck`, `check:thin-client`, `bunx vitest run`, the editor e2e specs.
- [x] 3. C: palette, rail and zoom bar. Verify: the same web checks and the image e2e specs.
- [ ] 4. D: the screens with A's data. Verify: the same web checks and the specs of those pages.
- [ ] 5. Captures of the changed screens in both themes, compared with the frames by the designer. Verify: the designer's verdict per screen.

## Test strategy

Unit and contract tests for every new field; parity across HTTP, CLI and MCP; Playwright for each changed screen; the designer's visual comparison.

## Acceptance criteria

1. Every screen listed as partial in the redesign hand-offs is equal to its frame, or its remaining gap is named with the reason.
2. No value on a screen is computed in TypeScript (`check:thin-client`).
3. The MCP budget holds (`surfacebudget` exits 0).
4. `task lint`, `task test`, `task test:web` and the typecheck pass.

## Risks and mitigation

- New fields grow the API: add them to existing reads, with SDD-000 names and contract tests.
- **STOP** if a field needs a new MCP tool or a change of an ADR.

## Size

L (four groups, A first for D).

## Hand-off for review

The commits per group, the captures, and each criterion with its command.
