# 30: Image operations with a model

## Goal

A creator removes the background of a photo, or asks for a generated background behind a product, from inside the image editor: with the price shown first, and the result arriving as a new layer they can undo.

## Context and related decisions

- ADR-0036, ADR-0038, ADR-0032.
- Author decisions D-061, D-062. PRD-004 RF-340, RF-362, RF-363.

## Scope

- Modules: `core/provider/` (the `image_edit` capability), `service/providers/<name>/` (the capability on a provider already approved in epic 10, or one new adapter), the job kind `image.edit`, `core/image/` (one command that inserts the result), the image editor's toolbar (epic 28), SDD-016, SDD-021.
- Two operations: **remove background** (returns the layer with transparency) and **generate into the document** (a new image layer from a prompt, sized to the canvas or to a selected layer).

## Out of scope

- Inpainting with a painted mask, upscaling, face retouching, style transfer.
- A local model for background removal: a candidate for later, with its own licence review.

## Dependencies

10, 28.

## Technical design

- **Capability**: `ImageEditor` (`Estimate`, `RemoveBackground(asset)`, optional further operations declared by the provider). Generation into the document reuses `ImageGenerator` of epic 10.
- **Job**: `image.edit` estimates, passes through approval and ceilings, uploads the source layer's asset through the connection, stores the result as an asset with origin `generated` (operation, provider, model, source asset, cost).
- **Into the document**: when the job ends, one command batch `layer.replace_source` (remove background: the layer keeps its transform and gets the new asset) or `layer.add_image` (generation) is applied with the requesting actor, so it is in the history and undoable. The original asset is never changed.
- **What leaves the machine**: the approval summary and the dialog say that the image will be sent to the provider, and to which one.
- **UI**: two toolbar actions with the estimate, progress on the layer, and the result selected when done.

## Tasks

- [x] 1. Confirm the provider offers background removal through its official API, with pricing and terms; if a new provider is needed, STOP for approval. Verify: the decision is in the hand-off.
- [x] 2. Amend SDD-016 and SDD-021. Verify: `task lint:md` exits 0.
- [x] 3. Implement the capability and the adapter method with contract fixtures. Verify: `go test ./service/providers/...` passes with no network.
- [x] 4. Implement `image.edit` and `layer.replace_source` with golden cases. Verify: `go test ./core/image/... ./service/... -run "TestReplaceSource|TestImageEdit"` passes.
- [ ] 5. Expose on the three surfaces. Verify: `go test ./web/contract/...` passes.
- [ ] 6. Add the two toolbar actions in both languages. Verify: `task test:web` and `web/e2e/specs/image-model-ops.spec.ts` pass with a fake provider.

## Test strategy

- Contract: the adapter from recorded pairs.
- Integration: approval gate, the result as an undoable command, the original untouched.
- End to end: remove a background with a fake provider and undo it.

## Acceptance criteria

1. "Remove background" on an image layer ends with the same layer, same transform, a transparent background, and one undo step restores the original.
2. "Generate" adds a new image layer whose asset has a complete origin.
3. No image leaves the machine before the estimate is shown and, above the ceiling, approved.
4. Both operations are reachable on HTTP, CLI and MCP.
5. `go test ./core/image/... ./service/providers/... ./web/contract/...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Users do not expect their photo to be uploaded.** Mitigation: the dialog and the approval summary say it plainly.
- **STOP** at task 1 when a new provider or a dependency is needed.

## Size

S

## Hand-off for review

The diff of `epic/30-image-operations-with-a-model`; the provider decision; fixtures; the outputs of the acceptance commands.
