# 33: Hand-off from an external motion engine

## Goal

When a video needs an animation the editor cannot make, a creator's agent builds it in a dedicated motion engine on its own side and brings it back as a clip that fits: right size, right length, transparent where it should be, and replaceable when the animation is rendered again.

## Context and related decisions

- ADR-0042 (motion beyond the minimal set comes from an external engine through a hand-off), ADR-0027 (Remotion is not adopted: its licence is commercial), ADR-0020 item 6 (the product downloads nothing).
- Author decisions D-065, D-071 (HyperFrames, Apache-2.0, adopted as a source for skills). PRD-001 RF-084; PRD-004 RF-389.
- Research note `web/04-ferramentas-similares`.
- Absorbs the earlier plan on external engines and the hand-off.

## Scope

- Modules: `core/command/` (a media replacement command if missing), `service/` (the motion brief and the re-import), `engine/ingest/` (clips with alpha: what is accepted and how it is normalised), `web/app/` (controllers, CLI, MCP), `web/resources/skills/` (one built-in skill), `licenses/`, SDD-011, SDD-002, SDD-018.
- The product never runs, bundles or downloads the engine. The agent, or the user, runs it.

## Out of scope

- Running HyperFrames or any engine inside Stuarty; a Node runtime in the product.
- Remotion.
- A live link that re-renders automatically.

## Dependencies

08.

## Technical design

- **Motion brief**: `MotionBrief(timeline_id, range, track)` returns everything an external render needs to fit: width, height, frame rate, duration in frames, the safe areas of the project's format, the brand kit (colours, fonts by name, logo asset), the frame before and after the range as reference images, and the captions inside the range. Text fields are delimited as untrusted.
- **Import with origin**: the rendered file is imported like any media, with origin `rendered_externally` (epic 08): engine name and version, the source folder or description, and a `render_key` the agent chooses.
- **Alpha**: `engine/ingest` already normalises animated media with transparency; this epic writes down in SDD-011 which containers and codecs with alpha are accepted, adds tests for each, and makes a file that claims alpha but has none a lint finding instead of a silent opaque clip.
- **Validation at import**: `MotionCheck(asset, brief)` compares size, frame rate and duration with the brief and reports each mismatch with the exact value expected; nothing is silently stretched.
- **Re-render**: `clip.replace_media` (added if it does not exist) swaps a clip's asset keeping position, trim, transform, keyframes and effects; importing with an existing `render_key` offers "replace everywhere it is used" as one atomic batch, undoable.
- **Skill**: `motion-with-an-external-engine` (built-in): get the brief, render on the agent's side with HyperFrames, import with origin, check, place or replace. Adapted material carries its source and licence; the notice goes in `licenses/`.

## Tasks

- [x] 1. Amend SDD-011 (alpha inputs), SDD-002 (`clip.replace_media`) and SDD-018 (the skill). Verify: `task lint:md` exits 0.
- [ ] 2. Implement `MotionBrief` on the three surfaces. Verify: `go test ./service/... -run TestMotionBrief` and `go test ./web/contract/...` pass.
- [ ] 3. Add ingest tests for each accepted alpha input and the "claims alpha, has none" finding. Verify: `go test ./engine/ingest/... -run TestAlpha` passes.
- [ ] 4. Implement `MotionCheck`. Verify: `go test ./service/... -run TestMotionCheck` passes.
- [ ] 5. Implement `clip.replace_media` with golden cases, and the replace-by-`render_key` batch. Verify: `go test ./core/command/... ./service/... -run "TestReplaceMedia|TestRenderKey"` passes.
- [ ] 6. Write the skill, read the licence of its source, add the notice. Verify: `go test ./web/resources/skills/...` passes (lint and registry check) and `task notices:check` exits 0.
- [ ] 7. One real round with HyperFrames on the author's machine (brief, render, import, replace), or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: brief contents; check mismatches.
- Golden cases for `clip.replace_media`.
- Ingest: one fixture per accepted alpha input.
- Scenario: a scripted agent imports a rendered clip, places it, re-imports with the same key and replaces it.

## Acceptance criteria

1. `MotionBrief` returns size, frame rate, duration, safe areas, brand kit and reference frames for a range, on HTTP, CLI and MCP.
2. A clip with transparency imported from each accepted input composes over video with correct edges (golden frames).
3. `MotionCheck` reports a wrong size, rate or duration with the expected value.
4. Re-importing with the same `render_key` replaces the clip everywhere in one undoable batch, keeping its keyframes.
5. The product contains no downloader and no engine: `bash scripts/no-egress.sh` passes, and no new dependency is added.
6. `go test ./core/command/... ./engine/ingest/... ./service/... ./web/contract/...` passes.

## Risks and mitigation

- **Alpha formats differ in premultiplication.** Mitigation: SDD-011 states what is expected; a fixture per format with an edge test.
- **The external engine changes its command line.** Mitigation: the skill names the version it was tested with.
- **STOP** if the licence of the skill's source is not what the research note recorded.

## Size

M

## Hand-off for review

The diff of `epic/33-external-motion-engine-hand-off`; the accepted alpha inputs; the outputs of the acceptance commands; whether a real round was verified.
