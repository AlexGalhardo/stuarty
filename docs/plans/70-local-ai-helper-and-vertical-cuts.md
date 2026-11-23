# 70: Local AI helper and face-aware vertical cuts

## Goal

A creator turns a long video into several vertical cuts that keep the speaker's face in frame, on their own computer, with models downloaded only when they ask.

## Context and related decisions

- Author decisions (2026-11-23): local by default, small models downloaded on request, external APIs optional and through approvals and cost ceilings; YuNet for faces.
- ADR-0050 (local AI runtime and model licence policy, accepted by author decision D-133), ADR-0021 (the install pattern of whisper.cpp), `engine/transcribe/manifest.json` (pinned downloads).
- `core/command/settings_reframe.go`, `clip.set_framing`, the skill `web/resources/skills/long-talking-video/SKILL.md`.

## User stories

- As a creator, I want vertical cuts from a long video so that I can publish shorts.
- As a creator, I want the face kept in frame so that I do not reframe by hand.
- As an agent, I want to propose cuts so that a person chooses which to keep.

## Scope

- Modules: `engine/aihost/` (new), the helper executable defined by ADR-0050, `core/` (the tracking and reframe rules, commands), `service/`, the adapters, the editor UI, SDD-024 (new), SDD-000, SDD-007.
- Must not touch: `engine/transcribe/`, `capture/`, `renderer/`.

## Out of scope

BEN2 and depth (72), MediaPipe unless a measured YuNet result is insufficient, Python in the product.

## Dependencies

ADR-0050 accepted. 69 for the word ranges that choose cut points.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop, Windows | Full |
| Docker and VPS | CPU execution, if the image includes the helper [to validate size] |
| Desktop, macOS and Linux | After 43 |
| HTTP, CLI, MCP | A job through the existing `job_start`, reframe commands through `edit` |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation or command | Scope | Does |
| --- | --- | --- |
| `ModelInstall` | `admin` | **A person only.** Downloads a pinned model after showing its size and licence |
| Job kind `face_track` | `media` | Produces a face track of a clip |
| `clip.apply_face_framing` | `edit` | Writes framing keyframes from a track |
| `CutsPropose` | `edit` | Proposes ranges and creates no timeline until accepted |

## Technical design

- **One helper over ONNX Runtime** (MIT), a separate process under SDD-007 section 6. It reads frames from FFmpeg and writes JSON results. Execution order: DirectML on Windows, then CPU. The CUDA build is not shipped by default (ADR-0050).
- **YuNet** is downloaded on request, with a pinned SHA-256. Without it, the feature explains what to install.
- **Tracking** in the core: smoothing, dead zone and a maximum pan speed, deterministic for one track.
- A model download is an outbound connection: it runs only on the person's explicit request (ADR-0032).

## Tasks

- [ ] 1. SDD-024, SDD-000 and SDD-007. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Spike: YuNet through ONNX Runtime on the reference machine, CPU and DirectML. Verify: frames per second and detection rate on a fixture in `docs/perf/`.
- [ ] 3. The helper and `engine/aihost`, with a fake model. Verify: `go test ./engine/aihost/...` passes.
- [ ] 4. The model manifest, install and verification. Verify: tests for a wrong hash, an interrupted download and no download without request.
- [ ] 5. The tracking and framing rules. Verify: `go test ./core/...` with golden tracks.
- [ ] 6. The `face_track` job and the cut proposals. Verify: `go test ./service/...` passes.
- [ ] 7. Adapters with parity. Verify: `go test ./web/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 8. The UI: install prompt, progress, proposal review. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `task test:web` and `bun run e2e` pass.

## Test strategy

Fake model for host and service; a licensed face video fixture for the real run; the no-egress test.

## Acceptance criteria

1. **Given** no model installed, **when** the product runs, **then** it opens no outbound connection (no-egress test).
2. **Given** a model with a wrong hash, **when** installed, **then** it is refused and removed.
3. **Given** a 16:9 fixture with one moving speaker, **when** reframed to 9:16, **then** the face stays inside the frame in at least 95% of frames.
4. **Given** the same track and options, **when** framing runs twice, **then** the keyframes are identical.
5. **Given** a proposal, **when** created, **then** no timeline changes until a person accepts.
6. `task lint`, `task test`, `task test:web` pass; the notices list includes every new component.

## Risks and mitigation

- Binary size and execution providers: measured in the spike.
- YuNet training-data origin: the residual risk is accepted (D-133). This epic records the origin in ADR-0050. **STOP** if a licence forbids this use.
- **STOP** if ONNX Runtime needs cgo in the server, if a weights licence is not compatible, or if CPU speed makes the feature unusable.

## Size

XL: a new runtime, a model manager and a tracking feature. If task 3 runs long, tasks 5 to 8 become a follow-up epic.

## Priority

Should. High value, and it builds the base for optional models.

## Hand-off for review

The commits, the spike numbers, each criterion with its command and result.
