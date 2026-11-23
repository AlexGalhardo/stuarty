# 73: A real end-to-end round with HyperFrames

## Goal

A creator's agent brings a real HyperFrames animation into a video through the flow of epic 33, and the product can say that this flow was proven with the real engine and name the version it was proven with.

## Context and related decisions

- Author decision (2026-11-23): HyperFrames is the only external motion engine with official support. Remotion stays out, with no skill and no bundling. Any externally rendered clip can still be imported through the existing flow.
- Epic 33 records its real round as not verified (task 7). `TestRenderKeyScenarioWithAFakeEngine` covers the same round with a fake engine.
- ADR-0042 (the product never installs, bundles or runs the engine), ADR-0027 (Remotion is cut), ADR-0032 (the hosted HeyGen MCP connector stays out), author decisions D-065, D-071.
- `web/resources/skills/motion-with-an-external-engine/SKILL.md`, `service/motion.go`, `service/motion_check.go`, `docs/planning/screen-recording/research.md` section 6.

## User stories

- As a creator, I want an animation from HyperFrames to fit my video so that I do not fix size or length by hand.
- As an agent, I want the skill to name the tested engine version so that I can reproduce the render.
- As a creator, I want a new render to replace the old clip everywhere so that my edits are kept.

## Scope

- Modules: `web/resources/skills/motion-with-an-external-engine/` (the tested version and any correction the round proves necessary), `licenses/` if the notice changes, `docs/plans/33-external-motion-engine-hand-off.md` (the note of task 7), the user guide of the flow.
- A product fix found by the round is a `VV` entry and is not made here, unless it is one small change to the epic 33 code that the round cannot pass without.
- Must not touch: the recording modules, the renderer, the FFmpeg policy.

## Out of scope

Running or installing the engine from Stuarty, a Node runtime in the product, Remotion, the hosted HeyGen MCP connector, a live re-render link.

## Dependencies

33. It is independent of the recording epics and of the redesign. The user installs Node, Chrome and an LGPL FFmpeg on their machine.

## Surfaces

| Surface | Change |
| --- | --- |
| HTTP, CLI, MCP | No new operation. The round uses `MotionBrief`, import with origin, `MotionCheck` and replacement by `render_key` |
| Web and desktop | None |
| The engine | Runs on the user's side, outside the product |

## Core commands and permissions

No new command. The round uses the existing operations of epic 33 and `clip.replace_media`, with their existing scopes.

## Technical design

1. Get a brief with `MotionBrief` for a timeline range.
2. Render on the user's side with npm `hyperframes` at an exact version (0.9.86 at research time; the engine is 0.x, so the skill names the version actually tested).
3. Import the file with origin `rendered_externally`, the engine name and version, and a `render_key`.
4. Run `MotionCheck`, place the clip, render again with a change, import with the same `render_key` and replace everywhere.
5. Whether HyperFrames accepts a custom FFmpeg path is [to validate], and decides whether the round can use an LGPL build.

## Tasks

- [x] 1. Record the environment: exact versions of `hyperframes`, Node, Chrome and FFmpeg, and the FFmpeg licence flags. Verify: `ffmpeg -version` in the hand-off shows no `--enable-gpl` or `--enable-nonfree`.
- [x] 2. The real round of the technical design on a 9:16 project, with one clip with transparency. Verify: `MotionCheck` reports no mismatch, and a frame from the export shows the animation over the video.
- [x] 3. Render again and replace by `render_key`. Verify: one batch replaces every use, keyframes stay, and one undo restores the former clip.
- [x] 4. Update the skill with the tested version and any corrected step, and the note of epic 33 task 7. Verify: `go test ./web/resources/skills/...` and `task notices:check` exit 0.
- [ ] 5. Prove the product gained nothing. Verify: `bash scripts/no-egress.sh` passes on Linux or is reported as not run, and `git diff` shows no new dependency.

## Test strategy

A manual, recorded round with the real engine. The existing fake-engine scenario stays as the automated regression test.

## Acceptance criteria

1. **Given** a brief, **when** the file rendered by the named version is imported, **then** `MotionCheck` reports no mismatch.
2. **Given** a clip with transparency, **when** it is exported over video, **then** the edges compose correctly in a saved frame.
3. **Given** a second render with the same `render_key`, **when** it is imported, **then** every use is replaced in one undoable batch that keeps position, trim and keyframes.
4. **Given** the skill, **when** read, **then** it names the exact tested version and contains no Remotion step and no hosted connector.
5. **Given** the product, **when** inspected, **then** it has no engine, no Node runtime and no downloader for it (`bash scripts/no-egress.sh`, and `git diff` on the manifests).
6. `go test ./web/resources/skills/...` and `task notices:check` pass.

## Risks and mitigation

- The 0.x engine changes between versions: the skill pins what was tested.
- **STOP** if the only way to render needs a GPL FFmpeg on the project's side.
- **STOP** if the licence of the tested version is no longer Apache-2.0.
- **STOP** if the round needs the product to install or run the engine.

## Size

S: one verified round and a small skill update.

## Priority

Should. It closes a recorded verification gap in a supported flow at low cost.

## Hand-off for review

The versions, the commands run, the saved frame, each criterion with its result, and anything not run.
