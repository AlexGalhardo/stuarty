# 53: Constant speed for compositions

## Goal

A creator who places a nested composition (an intro, a sequence of lower thirds) on the timeline can play it at half or double speed as one clip, and the picture and the sound of the composition speed up or slow down in every video that uses it.

## Context and related decisions

- ADR-0042 (the editor's motion and audio scope), ADR-0029 (one renderer), ADR-0039 (a project holds several timelines).
- Author decision D-094. PRD-001 RF-083 (nested compositions).
- Epic 32 (the composition clip kind, which this epic extends). At the time of writing it is not on `release/0.30.0`: `core/timeline` has no composition kind yet (checked by search).
- SDD-002 section B.5 (the `clip.set_speed` row: clips and audio items) and section C.25 (the duration rule `ceil(duration x old / new)`, the rescaling of keyframes, fades and transitions, the ripple).
- SDD-001 section 3.5 (`speed`, `speed_curve` and `reverse` of a clip and an audio item).
- SDD-004 section 3.3 (the source frame of a clip), section 3.11 (the speed curve and the mean speed), section 3.12 (reverse), section 7 (the audio mix; step 1 uses a tempo filter for a clip's audio).
- SDD-012 (the Speed section of the inspector, which for an audio item is not shown).
- The code this epic builds on: `core/command/clip_speed.go` (the constant speed from 1/4 to 4, and the curve) and `core/timeline/types.go` (the `Speed` and `SpeedCurve` fields of an item).

## Scope

- Modules: `core/timeline/` (a composition clip takes a constant `speed`), `core/command/clip_speed.go` (the target kind and its refusals), `core/plan/` (the child frame of a composition under a constant speed), `engine/audio/` (the nested bus under a constant speed, under the rule of task 1), `engine/export/` (uses the plan; golden files), `web/resources/js/editor/inspector/` (the Speed section for a composition clip), `web/resources/lang/en.json` and `pt-BR.json`, SDD-001 (section 3.5), SDD-002 (section B.5 and C.25), SDD-004 (sections 3.3, 3.11 and 7), SDD-012 (the Speed section).
- Constant speed only: `clip.set_speed` with `speed` on a composition clip, from 1/4 to 4, with the same parsing and rounding as a clip (SDD-002 section C.25).

## Out of scope

- A speed curve on a composition (refused with `E_VALIDATION`), and reverse or freeze frame of a composition (refused).
- A speed inside the nested timeline: its own clips keep their speed; only the composition clip has the speed of the epic.
- Time remapping beyond a constant speed (the same limit as epic 32).

## Dependencies

32 (the composition clip and the nested mix).

Not in progress together with 51 (both change `engine/audio` and SDD-004 section 7). It can run beside 52 once 32 is merged, since they change different modules.

## Technical design

- **Command.** `clip.set_speed` takes a composition clip: `speed` as for a clip. `curve` and `reverse` are `E_VALIDATION`, with a hint to use a constant speed. The duration is `ceil(D x old / new)`; keyframes, fades and transitions are rescaled as for a clip (SDD-002 section C.25). The inverse restores the old speed and duration.
- **Child frame.** At output frame `p` of the composition clip, with speed `s`, the child frame is the start of the source range plus `floor((p - start) x s)`, in the conformed frame rate of the child. The senior confirms this rounding against SDD-004 section 3.3, and writes any difference into SDD-004 before the code.
- **Audio: a decision.** The nested bus is in-process PCM, so the tempo filter of a clip's audio (SDD-004 section 7, step 1) cannot be used on it. Either the bus is time-scaled by a specified, deterministic method that keeps the pitch (a new stage of `engine/audio`, specified first in SDD-004), or it is resampled and its pitch changes with its speed. **Decision for the orchestrator before task 4.** The recommendation is the pitch-keeping method if it can be specified in SDD-004 without a new dependency; otherwise the resample, with the difference from a clip written into SDD-004. **STOP** until one of them is chosen.
- **Parity.** Preview and export read the same plan (SDD-004 section 10). The browser draws the group (epic 52 when merged, otherwise the path of epic 32) at the child frame of the plan.
- **Surfaces.** No new operation and no new action: `clip.set_speed` is already on the three surfaces, and the contract test covers the composition target.

## Tasks

- [x] 1. Decide and specify the audio rule of a sped-up bus (pitch kept or not) and the child frame mapping, in SDD-004 sections 3.3, 3.11 and 7; amend SDD-002 (section B.5 and C.25), SDD-001 (section 3.5) and SDD-012 (the Speed section). Verify: `task lint:md` exits 0. **STOP** if the decision is not given.
- [x] 2. The command: accept a constant speed on a composition clip; refuse a curve and a reverse with stable errors; golden cases. Verify: `go test ./core/command/... -run TestCompositionSpeed` passes.
- [ ] 3. The plan: the child frame mapping, with golden plans. Verify: `go test ./core/plan/... -run TestCompositionSpeed` passes.
- [ ] 4. Audio: the nested bus under the rule of task 1, with a golden WAV. Verify: `go test ./engine/audio/... -run TestCompositionSpeedMix` passes.
- [ ] 5. Export and preview: golden frames at 0.5 and at 2, and the parity spec. Verify: `go test ./engine/export/... -run TestCompositionSpeed` passes, and `web/e2e/specs/preview-export-parity.spec.ts` passes with a composition at speed.
- [ ] 6. The Speed section of the inspector for a composition clip, in both languages. Verify: `bun run check:i18n` and `task test:web` pass, and `web/e2e/specs/composition-speed.spec.ts` (new, `bun run e2e`) passes.
- [ ] 7. Surfaces and generated documents. Verify: `go test ./web/contract/...` passes and `task generate:check` exits 0.

## Test strategy

- Golden: command cases for each refusal and each success; plans at speeds 0.5, 1 and 2; the golden frames of the export; a golden WAV of the nested bus.
- Property: after `clip.set_speed` to a speed and back, the duration equals the original within the rounding of SDD-002 section C.25; undo restores the clip exactly.
- Parity: preview and export of a project with a composition at 0.5 and at 2.
- End to end: set the speed of a composition in the inspector; the preview and the exported file show the change.

## Acceptance criteria

1. `clip.set_speed` with `speed` `2x` on a composition halves its duration by the rule `ceil(D x old / new)`: `go test ./core/command/... -run TestCompositionSpeed` passes.
2. A curve or a reverse on a composition is refused with `E_VALIDATION` and a hint: the same test passes.
3. The export of a composition at 0.5 and at 2 plays the child frames at the rate of task 2 (golden frames): `go test ./engine/export/... -run TestCompositionSpeed` passes.
4. The preview equals the export within the tolerance of ADR-0029 at both speeds: `web/e2e/specs/preview-export-parity.spec.ts` passes.
5. The nested audio at each speed equals its golden WAV and follows the pitch rule chosen in task 1: `go test ./engine/audio/... -run TestCompositionSpeedMix` passes.
6. `go test ./core/... ./engine/... ./web/contract/...`, `task generate:check`, `task lint:md`, `task test:web`, `bun run check:i18n` and `bun run typecheck` pass.

## Risks and mitigation

- **The pitch decision.** Keeping the pitch in Go needs a specified method; a resample is simpler and changes the pitch. **STOP** until the orchestrator or the author chooses (task 1). The epic does not choose silently.
- **Epic 32 not merged.** The composition kind is not on `release/0.30.0` yet. **STOP**: this epic cannot start before epic 32.
- **Rounding drift.** A long composition at 3/2 accumulates rounding. The ceiling rule of SDD-002 section C.25 is used as written, and the drift is tested over a composition of ten minutes.
- **Curves creeping in.** A curve on a composition is out of scope, so it is refused, not approximated with a constant.

## Size

M

## Hand-off for review

The diff of `epic/53-constant-speed-for-compositions`; the golden frames and WAV files; the parity results; the pitch decision as written into SDD-004.
