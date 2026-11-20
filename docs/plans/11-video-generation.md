# 11: Video generation

## Goal

A creator asks for a few seconds of generated footage, from a prompt or from one of their images, sees what it will cost, approves, and finds the clip in the library ready to drop on the timeline.

## Context and related decisions

- ADR-0036, ADR-0032 (a costly generation waits for a person), ADR-0010.
- Author decisions D-061 (video generation from the first epics), D-052. PRD-004 RF-361 (video), RF-362, RF-363.

## Scope

- Modules: `core/provider/` (the `video` capability), `service/providers/<name>/` (one adapter), the job kind `video.generate`, `web/app/` (controllers, CLI, MCP), the "Generate" dialog of epic 10 (a video tab), SDD-016, SDD-000.
- One provider, chosen in task 1 and approved by the author (D-105): Google through the Gemini API, one adapter `service/providers/google/`.

## Out of scope

- Editing a video with a model (extend, restyle, lip sync), avatars and talking heads.
- Any provider reached through a service with no official API (RF-365).

## Dependencies

10.

## Technical design

- **Capability**: `VideoGenerator` (`Estimate`, `Submit`, `Poll`, `Fetch`): generation is asynchronous at every provider, so the adapter submits, the job polls with backoff, and the file is fetched at the end.
- **Job**: `video.generate` survives a restart: the provider's task id is stored with the job, and a restarted server resumes polling instead of submitting again (no double charge). Cancelling asks the provider to cancel when it can, and records the cost already incurred.
- **Approval**: by default every video generation needs a person, whatever the ceiling, because the unit cost is high; a rule can lift it (epic 05).
- **Inputs**: prompt, duration and aspect ratio within the model's limits (from the catalogue), optionally a first-frame image asset, uploaded through the same connection.
- **Result**: fetched from a host in the connection's destinations, ingested like an imported file (probe, proxy, thumbnail), stored as an asset with origin `generated`.
- **Service**: `VideoGenerate` returns a job id; the job result is an asset id.

## Tasks

- [x] 1. Propose the provider (API, pricing source, terms, limits); STOP for approval. Verify: the approval is in the hand-off. Decided by the author (D-105): Google through the Gemini API with the user's own API key; SDD-016 section 2.3 has the API, the pricing source, the terms and the limits, and says that the model is `gemini-omni-1.2-flash` because Google shut down the Veo 3.1 preview models on 2026-10-22.
- [x] 2. Amend SDD-016 and SDD-000. Verify: `task lint:md` exits 0.
- [x] 3. Implement validation and estimation with table tests. Verify: `go test ./core/provider/...` passes.
- [ ] 4. Implement the adapter with contract tests from recorded responses (submit, pending, done, failed, refused). Verify: `go test ./service/providers/...` passes with no network.
- [x] 5. Implement the job with resume after restart and cancellation. Verify: `go test ./service/... -run TestVideoGenerate` passes, including the restart test that counts one submission.
- [ ] 6. Expose on HTTP, CLI and MCP; add the video tab to the dialog. Verify: `go test ./web/contract/...`, `task test:web` and `web/e2e/specs/generate-video.spec.ts` pass against a fake provider.

## Test strategy

- Unit: limits and estimates.
- Contract: the adapter's four states and its errors.
- Integration: approval by default, a rule that lifts it, resume after restart, cancellation.
- End to end: generate with a fake provider and place the clip on a timeline.

## Acceptance criteria

1. `VideoGenerate` without a rule stays in `waiting_approval` and makes no outbound call.
2. After approval the job ends with a playable asset whose origin is complete.
3. Restarting the server during polling yields exactly one submission to the provider (test).
4. An image asset can be the first frame.
5. `go test ./core/provider/... ./service/providers/... ./service/... -run "Video"` and `task test:web` pass with no network access.

## Risks and mitigation

- **A double charge after a crash.** Mitigation: the provider task id is persisted before the first poll.
- **A provider that changes its API often.** Mitigation: one adapter, contract fixtures, the catalogue at run time.
- **STOP** at task 1.

## Size

M

## Hand-off for review

The diff of `epic/11-video-generation`; the approved provider; recorded fixtures; the outputs of the acceptance commands.
