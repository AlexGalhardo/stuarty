# 72: Optional local models: background removal and depth

## Goal

A creator who wants more can install optional models to remove the background of a webcam or video and to add depth effects, with no impact on anyone who does not install them.

## Context and related decisions

- Author decision (2026-11-23): tools are added where they make sense, optional, after their licences are checked.
- ADR-0050, epic 70 (the helper and the model manager).
- Today background removal exists only for images through a provider (`service/providers/openai/image_edit.go`), and video has chroma key (`core/command/chroma_key.go`).

## Tool decisions

| Tool | Decision |
| --- | --- |
| BEN2 base | **Blocked** until the licence of its weights is confirmed in writing or by a licence file |
| Depth-Anything-V2 Small | Allowed (Apache-2.0) |
| Depth-Anything-V2 Base, Large, Giant | **Never** (CC-BY-NC-4.0) |
| WhisperX | **Not shipped** (Python runtime, ADR-0021). whisper.cpp stays |
| MediaPipe | Only if epic 70 measures YuNet as insufficient, through a new decision |
| RobustVideoMatting | **Never** (GPL-3.0) |

## User stories

- As a creator, I want to remove my webcam background without a green screen so that it fits my layout.
- As a creator, I want a depth effect on an image so that a thumbnail gains motion.
- As an agent, I want to request these as jobs so that I can use them in a finishing flow when they are installed.

## Scope

- Modules: the model manifest of epic 70, the helper's model adapters, `core/` (mask and depth commands), `service/`, the adapters, the editor UI, SDD-024, the licence notices.
- Must not touch: `capture/`, `engine/transcribe/`.

## Out of scope

Real-time matting while recording, Depth Base or larger, training or fine-tuning.

## Dependencies

70, and the author's decision on the BEN2 weights for tasks 2 and 3.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Optional install, job progress, result as a mask or depth layer |
| HTTP, CLI, MCP | Jobs through the existing `job_start`, commands through `edit` |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation or command | Scope | Does |
| --- | --- | --- |
| `ModelInstall` (extended) | `admin` | A person only |
| Job kind `video_matte` | `media` | Writes a matte sidecar for a clip |
| Job kind `depth_map` | `media` | Writes a relative depth map |
| `clip.use_matte` | `edit` | Applies a matte to a clip, undoable |

## Technical design

- Each model is optional, downloaded on request and pinned by SHA-256, and its licence is shown before install.
- BEN2 runs per frame. Temporal flicker is measured before it is promised for video.
- Depth output is relative and used for parallax and blur, never as a measurement.

## Tasks

- [ ] 1. SDD-024, SDD-000 and the notices. Verify: `task lint:md` exits 0.
- [ ] 2. **After the author's decision:** spike of BEN2 base for speed and flicker. Verify: a report in `docs/perf/`.
- [ ] 3. **After the author's decision:** the matte job and `clip.use_matte`. Verify: `go test ./core/... ./service/...` passes.
- [ ] 4. Depth-Anything-V2 Small through a converted ONNX file with recorded provenance. Verify: a hash check and a depth golden on a fixture.
- [ ] 5. UI for install, progress and use. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.
- [ ] 6. Adapters with parity. Verify: `go test ./web/...` and `go run ./internal/tools/surfacebudget` pass.

## Test strategy

Fake model for jobs; licensed fixtures for real runs; a test that the catalogue contains no excluded model.

## Acceptance criteria

1. **Given** no optional model, **when** the product runs, **then** no feature fails and no download occurs.
2. **Given** the model catalogue, **when** tested, **then** it has no Depth Base, Large or Giant, no RobustVideoMatting and no Python dependency.
3. **Given** an install request, **when** it is shown, **then** the size and licence appear before the download.
4. **Given** the BEN2 question unresolved, **when** the epic is delivered, **then** BEN2 stays out of the catalogue and the depth path still passes.
5. `task lint`, `task test`, `task test:web` pass.

## Risks and mitigation

- Licence of the BEN2 weights: the epic ships without it if unresolved.
- A community ONNX port of Depth: provenance recorded, or converted by the project.
- **STOP** if any weights licence is unclear, or if a model needs Python at run time.

## Size

L: two model families and their jobs and UI.

## Priority

Could. Valuable, but optional by design.

## Hand-off for review

The commits, the licence evidence, each criterion with its command and result.
