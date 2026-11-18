# 31: Motion: easing curves, animated crop and mask, presets

## Goal

A creator makes a title slide in with a soft overshoot, reveals a photo through a growing circle and pushes in on a detail: the handful of motion moves that most edited videos use, without leaving the editor.

## Context and related decisions

- ADR-0042 (the editor's scope for motion, audio and export), ADR-0002, ADR-0029.
- Author decision D-065 (a minimal motion set of our own; the author decided for animated crop and mask). PRD-001 RF-080, RF-081, RF-082.
- SDD-001 (the timeline schema), SDD-002 (the catalogue), SDD-013 (the plan). Read `.agents/skills/renderer-parity/`.
- Absorbs the earlier plan on animated crop and mask.

## Scope

- Modules: `core/timeline/` (keyframe model), `core/command/` (commands and golden cases), `core/plan/` (sampling per frame), `engine/compositor/` and `renderer/` (only if the plan gains a field they must read), the keyframe editor in `web/resources/js/editor/`, SDD-001, SDD-002, SDD-013.
- **Easing** (RF-080): each keyframe segment has an easing: the named ones that exist today plus an editable cubic bezier (four numbers) and a small set of named presets (ease in, out, in-out, back, overshoot).
- **Animated crop and mask** (RF-081): keyframes on crop edges and on the shape mask's position, size, corner radius and feather.
- **Presets** (RF-082): layer animation presets (slide, fade, scale, pop, push in, pull out) as parametrised keyframe sets applied by one command, for clips and for text.

## Out of scope

- Motion tracking, expressions, particles, 3D, puppet tools, shape path morphing.
- A graph editor with value curves for several properties at once: one easing curve per segment is the scope.
- Nested compositions (epic 32).

## Dependencies

03.

## Technical design

- **Easing in the core**: `core/plan` evaluates the bezier with fixed-point arithmetic and a specified number of iterations, so every executor receives already sampled values per frame; no executor evaluates a curve. The control points are validated (x within 0 and 1).
- **Schema**: SDD-001 gains the `bezier` easing kind and the new animatable properties; documents of earlier versions open unchanged (a missing easing is linear, as today).
- **Commands**: `keyframe.set_easing`; the existing keyframe commands accept the crop and mask properties; `animation.apply_preset` (preset id, parameters, in or out, duration) expands in the core into ordinary keyframes, so the result is editable and needs no new concept downstream. `animation.list_presets` describes them for agents.
- **Plan**: crop and mask values become per-frame values in the plan where they were constants; check that both executors read them per frame (the Go compositor for export today, the browser executor for preview) and extend the parity cases.
- **UI**: in the keyframe editor, a segment's easing opens a small curve editor (drag two handles, pick a named preset, type the four numbers); crop and mask get keyframe toggles in the inspector; a presets menu applies an animation to the selection. The UI sends commands and draws the curve it is given.
- **Lint**: a preset longer than its clip, and keyframes outside the clip, are reported.

## Tasks

- [ ] 1. Amend SDD-001, SDD-002 and SDD-013 (easing kind and its arithmetic, animatable crop and mask, presets). Verify: `task lint:md` exits 0 and the tests that read SDD-002 pass.
- [ ] 2. Implement the bezier easing in `core/plan` with table tests and a property test (monotonic time, end points exact). Verify: `go test ./core/plan/... -run TestEasing` passes.
- [ ] 3. Implement `keyframe.set_easing` with golden cases. Verify: `go test ./core/command/...` passes with the updated registry snapshot.
- [ ] 4. Make crop and mask animatable, in commands and in the plan, with golden cases. Verify: `go test ./core/command/... ./core/plan/...` passes.
- [ ] 5. Make both executors read the per-frame values; add parity cases. Verify: `go test ./engine/compositor/... ./engine/export/...` passes with new golden frames, and the parity tests of `renderer-parity` pass.
- [ ] 6. Implement `animation.apply_preset` and `animation.list_presets` with golden cases. Verify: `go test ./core/command/... -run TestAnimationPreset` passes.
- [ ] 7. Expose on the three surfaces and regenerate documents. Verify: `go test ./web/contract/...` and `task generate:check` pass.
- [ ] 8. Build the curve editor, the keyframe toggles and the presets menu, in both languages, keyboard operable. Verify: `task test:web`, `bun run check:thin-client` and `web/e2e/specs/motion-easing.spec.ts` pass.
- [ ] 9. Check that a document saved by release 0.29.0 opens and exports identically. Verify: `go test ./core/timeline/... -run TestOldDocuments` passes.

## Test strategy

- Unit and property tests for the easing arithmetic.
- Golden cases for every new command; golden frames for animated crop and mask.
- Preview and export parity on a project that uses all three features.
- End to end: edit a curve, animate a mask, apply a preset.

## Acceptance criteria

1. A segment's easing can be set to a custom bezier, and the exported frames follow it (golden frames).
2. Crop and a shape mask can be keyframed, and preview matches export within tolerance.
3. `animation.apply_preset` produces ordinary keyframes that can be edited afterwards.
4. The three features are reachable on HTTP, CLI and MCP and described in the generated agent documents.
5. Documents of release 0.29.0 export the same frames as before.
6. `go test ./core/... ./engine/compositor/... ./engine/export/... ./web/contract/...`, `task test:web` and `task generate:check` pass.

## Risks and mitigation

- **Curve evaluation that differs between executors.** Mitigation: only the core evaluates; executors receive values.
- **Conflict with epic 26 in the preview executor.** Mitigation: the plan is the interface; whichever merges second rebases its executor change and reruns parity.
- **STOP** before adding a curve-editor library.

## Size

L

## Hand-off for review

The diff of `epic/31-motion-easing-crop-and-mask`; the preset list; golden frames; parity results; the outputs of the acceptance commands.
