# 66: Recording layouts, backgrounds and teleprompter

## Goal

A creator frames screen and webcam in a ready layout on a background, and reads the project's own script from a teleprompter while recording.

## Context and related decisions

- Author decision (2026-11-23): layouts, backgrounds and a teleprompter from the Markdown script.
- Epic 17 and SDD-019 (scripts), `core/command/clip_adjust.go` (`clip.set_framing`, `clip.set_mask`).

## User stories

- As a creator, I want ready layouts for screen and webcam so that I do not position them by hand.
- As a creator, I want my script on a teleprompter so that I do not need another app.
- As an agent, I want to choose a layout and prepare the script so that the person only records.

## Scope

- Modules: `core/` (layout presets as commands), the recording dialog, a teleprompter window, the desktop shell, SDD-002, SDD-023.
- Must not touch: `capture/` except excluding the teleprompter window from capture, `renderer/`.

## Out of scope

Background removal of the webcam (72), script generation, voice-following scroll.

## Dependencies

62. Epic 64 for the agent's preparation of a script.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop | Layouts and a separate teleprompter window, excluded from capture on Windows [to validate] |
| Web | Layouts; the teleprompter is a page panel, which is captured if that tab or screen is shared, and the dialog says so |
| HTTP, CLI, MCP | Layouts through `edit`; the script through existing writing operations |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000 and SDD-002.

| Command | Scope | Does |
| --- | --- | --- |
| `recording.apply_layout` | `edit` | Places screen and webcam clips by a named layout, for the four project formats |
| `recording.set_background` | `edit` | Sets color, gradient or image behind the framed screen, with padding, radius and shadow |

The teleprompter reads scripts with existing writing operations and adds no write operation.

## Technical design

- Layouts are data in the core and resolve to ordinary framing, mask and position values, so they stay editable.
- The teleprompter holds speed, size and mirror as a local preference of the UI; it computes no edit.
- On Windows the window uses display affinity to stay out of a capture [to validate in task 4].

## Tasks

- [ ] 1. SDD-000, SDD-002, SDD-023. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff.
- [ ] 2. Layout presets for 9:16, 1:1, 16:9 and 4:5. Verify: `go test ./core/...` passes.
- [ ] 3. Backgrounds. Verify: `go test ./core/...` and the export golden test pass.
- [ ] 4. The teleprompter window and its exclusion from capture. Verify: a real capture test on Windows shows no teleprompter pixels, or the limit is documented.
- [ ] 5. The layout picker and teleprompter controls. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.

## Test strategy

Golden layouts per format, one capture test for the exclusion, Vitest for the controls.

## Acceptance criteria

1. **Given** each of the four formats, **when** a layout is applied, **then** no clip exceeds the frame and the result matches its golden.
2. **Given** an applied layout, **when** undo runs, **then** the previous framing returns.
3. **Given** a project script, **when** the teleprompter opens, **then** it shows that script and scrolls at the set speed.
4. **Given** a Windows screen capture with the teleprompter open, **when** the output is compared, **then** the teleprompter is absent.
5. `task lint`, `task test`, `task test:web` pass.

## Risks and mitigation

- Display affinity may behave differently across capture modes: tested on the real helper.
- **STOP** if the teleprompter cannot be kept out of the capture on Windows: the author decides whether it ships with a warning.

## Size

M: presets over existing commands and one window.

## Priority

Should. It raises quality, with no dependency for later epics.

## Hand-off for review

The commits, the golden images, each criterion with its command and result.
