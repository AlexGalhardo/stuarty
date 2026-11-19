# 10: Image and voice generation

## Goal

A creator, or their agent, asks for a thumbnail background or a narration and gets it inside the project, as an asset that remembers which model made it, from which prompt and for how much. Nothing is generated without the price being known first.

## Context and related decisions

- ADR-0036 (provider layer, provenance of generated media), ADR-0032 (approval of costly generations), ADR-0010.
- Author decisions D-059, D-061. PRD-004 RF-361 (images and voice), RF-362, RF-363, RF-364.
- Builds on the interfaces of epic 09 and on the asset origin of epic 08.

## Scope

- Modules: `core/provider/` (validation and estimation for the `image` and `voice` capabilities), `service/providers/` (the capability interfaces get their first adapters), `service/providers/<name>/`, the job kinds `image.generate` and `voice.generate`, `web/app/` (controllers, CLI, MCP), the "Generate" action in `web/resources/js/pages/VisualAssets/` and `VideoAssets/`, SDD-016, SDD-000.
- One image provider and one voice provider to begin with, chosen in task 1 and approved by the author. A provider already connected for text is reused when it offers the capability.

## Out of scope

- Video generation (epic 11). Editing an image with a model: remove background, extend, inpaint (epic 30).
- Voice cloning from a recording of a person: not offered until the author decides its consent rules.
- Music generation.

## Dependencies

08, 09.

## Technical design

- **Capabilities**: `ImageGenerator` (`Estimate`, `Generate` with prompt, size or aspect ratio, count, optional reference asset) and `VoiceGenerator` (`Estimate`, `Generate` with text, voice id, language, format; `Voices(ctx)` lists the provider's voices at run time).
- **Rules in `core/provider`**: sizes and counts within the model's limits read from the catalogue; text length limits for voice; estimation per image and per character or second, from the catalogue or the user's price table.
- **Jobs**: each kind estimates, passes through `core/approval` (epic 05), calls the adapter through `internal/outbound`, and stores every result in the media store as an asset with origin `generated` (provider, model, prompt, cost, actor). A result delivered as a URL is downloaded only from a host in the connection's destination list.
- **Failure**: a refused prompt, a rate limit and a timeout are distinct stable errors; a partial batch keeps what arrived and reports the rest.
- **Service**: `ImageGenerate`, `VoiceGenerate`, `VoiceList`; each returns a job id and, when done, asset ids. An optional `project_id` links the assets to the project.
- **Untrusted text**: the prompt is stored and returned as untrusted text; a provider's revised prompt too.
- **UI**: a "Generate" dialog in the two asset sections: provider and model, prompt, options, the estimate, then the job progress and the result in the grid.

## Tasks

- [x] 1. Propose the first image provider and the first voice provider (API, pricing source, terms on commercial use of outputs); STOP for approval. Verify: the approval is in the hand-off.
- [x] 2. Amend SDD-016 and SDD-000 (capabilities, job kinds, errors). Verify: `task lint:md` exits 0.
- [x] 3. Implement validation and estimation for both capabilities with table tests. Verify: `go test ./core/provider/...` passes.
- [ ] 4. Implement the image adapter with contract tests from recorded responses. Verify: `go test ./service/providers/...` passes with no network.
- [ ] 5. Implement the voice adapter and `VoiceList` the same way. Verify: `go test ./service/providers/...` passes with no network.
- [ ] 6. Implement the two job kinds: estimate, approval, ledger, storage as assets with origin. Verify: `go test ./service/... -run "TestImageGenerate|TestVoiceGenerate"` passes.
- [ ] 7. Expose on HTTP, CLI and MCP with parity tests. Verify: `go test ./web/contract/...` passes.
- [ ] 8. Build the "Generate" dialog in both languages. Verify: `task test:web` and `web/e2e/specs/generate-assets.spec.ts` pass against a fake provider server.

## Test strategy

- Unit: limits and estimates per capability.
- Contract: each adapter against recorded pairs, including a refused prompt, a rate limit and a result by URL.
- Integration: a job above the ceiling waits for approval; an approved job stores assets with a complete origin.
- End to end: generate an image and a narration with a fake provider and drop both on a timeline.

## Acceptance criteria

1. `ImageGenerate` and `VoiceGenerate` return jobs that end with assets whose origin names provider, model, prompt, cost and actor, on HTTP, CLI and MCP.
2. The estimate is returned before any outbound call, and the ledger holds the real cost afterwards.
3. A generation above the user's ceiling makes no outbound call until it is approved.
4. A result URL on a host outside the connection's destinations is refused with a stable error.
5. A generated narration can be placed on a timeline and exported (`go test ./service/... -run TestGeneratedVoiceOnTimeline`).
6. `go test ./core/provider/... ./service/providers/... ./web/contract/...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Terms of use of outputs differ per provider.** Mitigation: task 1 records them; the asset origin keeps provider and model so the user can check later.
- **Large results fill the disk.** Mitigation: the existing disk guard applies before download.
- **STOP** at task 1, and for any SDK: plain HTTP first.

## Size

M

## Hand-off for review

The diff of `epic/10-image-and-voice-generation`; the approved providers and their terms; recorded fixtures; the outputs of the acceptance commands.
