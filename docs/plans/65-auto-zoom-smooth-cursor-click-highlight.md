# 65: Automatic zoom, smooth cursor and click highlight

## Goal

A plain screen recording looks produced: the view follows the clicks, the cursor moves smoothly and clicks are visible, all editable afterwards.

## Context and related decisions

- Author decision (2026-11-23): recording finishing is essential.
- AGENTS rule 1 (rules in the core), rule 5 (one renderer; the renderer decides nothing), `.agents/rules/rust.md`, `.agents/skills/renderer-parity/`.
- `core/command/animation.go` (`keyframe.set`, `keyframe.set_easing`), `core/command/clip_adjust.go` (`clip.set_framing`).

## User stories

- As a creator, I want zooms generated from my clicks so that viewers see what I am doing.
- As a creator, I want to adjust or remove each zoom so that the result stays mine.
- As an agent, I want to generate zooms from the event file so that I can finish a tutorial on request.

## Scope

- Modules: `core/` (the zoom planner, the cursor track, new commands), the timeline schema and SDD-001, the frame plan, `renderer/` if a cursor layer needs it, the inspector UI, SDD-002, SDD-013, SDD-023.
- Must not touch: `capture/`.

## Out of scope

Motion blur, cursor themes beyond a small set, effects on Wayland recordings without click data.

## Dependencies

62. The recording must keep the cursor out of the video when finishing is wanted, so task 1 confirms the capture option set by 62.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop | Full, on recordings with events |
| Web | Manual zoom only: web recordings have no global click data |
| HTTP, CLI, MCP | Through the existing `edit` tool and `POST /projects/{id}/edit` |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000 and SDD-002. All are undoable commands with scope `edit`.

| Command | Does |
| --- | --- |
| `recording.auto_zoom` | Plans zooms from the events and writes keyframes |
| `recording.set_cursor` | Sets size, smoothing and visibility of the cursor track |
| `recording.set_click_highlight` | Sets style (ring, wave, pulse), color and duration |

## Technical design

- The planner in `core/` is pure and deterministic: same events and options, same keyframes. It groups nearby clicks and enforces minimum hold and maximum zoom.
- Zooms are ordinary keyframes, so the existing editor adjusts them.
- The cursor is a separate item drawn from the event data; a smoothing filter in the core outputs positions per frame.
- The renderer only draws what the frame plan gives it. Any renderer change passes the parity gates.

## Tasks

- [ ] 1. SDD-000, SDD-001, SDD-002, SDD-013. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff.
- [ ] 2. The zoom planner, with golden cases. Verify: `go test ./core/...` passes.
- [ ] 3. Cursor smoothing and the cursor item. Verify: `go test ./core/...` passes.
- [ ] 4. Click highlight. Verify: `go test ./core/...` passes.
- [ ] 5. Frame plan and renderer support. Verify: the Rust gate of `renderer/` and the parity gates of `.agents/skills/renderer-parity/`.
- [ ] 6. Inspector and timeline controls. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.
- [ ] 7. End to end from a recorded fixture to an export. Verify: an export test compares frames with golden images.

## Test strategy

Golden tests on event fixtures, renderer parity, one end-to-end export.

## Acceptance criteria

1. **Given** the same events and options, **when** `recording.auto_zoom` runs twice, **then** the keyframes are identical.
2. **Given** a fixture with five separated clicks, **when** it runs, **then** five zooms are created, each within the limits of SDD-023.
3. **Given** a generated zoom, **when** undo runs, **then** the timeline revision content returns to the previous state.
4. **Given** the preview and the export of one frame, **when** compared, **then** they pass the parity gate.
5. **Given** a recording with no events, **when** the command runs, **then** it fails with a code from SDD-000 and changes nothing.
6. `task lint`, `task test`, `task test:web` pass.

## Risks and mitigation

- Motion sickness from aggressive zoom: conservative defaults and a maximum.
- Renderer changes affect parity: gated.
- **STOP** if the effect needs logic in the renderer or in TypeScript, or if the schema change cannot migrate existing projects.

## Size

L: a planner, a new item type and a renderer path.

## Priority

Must. Competitors treat it as standard for screen recording.

## Hand-off for review

The commits, the parity output, each criterion with its command and result.
