# 51: Processed audio preview on the timeline

## Goal

A creator who has added noise reduction or a voice chain to a clip hears the processed voice when they play or scrub the timeline, the way the export will sound, and not only after pressing "Listen to the processed sound" and waiting for a render.

## Context and related decisions

- ADR-0042 (the editor's scope for motion, audio and export), ADR-0003 (the Go mixer and the export), ADR-0011 (only LGPL FFmpeg; the processors are in Go, in `engine/audio`).
- Author decision D-094. It answers the open question of SDD-004 section 7.1: "a render cached per clip revision and played by the timeline preview is an open question for the author (epic 34 review)".
- PRD-001 RF-085 and RF-086 (audio processors and noise reduction). NFR-043 (no GPL component).
- SDD-004 section 7 (the mix; step 3 applies one loudness gain to the whole mix), section 7.1 (the processors, and the paragraph on the preview), section 8.1 (the audio-only WAV), section 10 (the preview executor: the audio elements and the audio clock).
- SDD-003 section 6.3 and section 6.5 (the routes and the byte-serving rules), SDD-007 (the table of byte-serving routes), SDD-011 section 6.2 (atomic writes) and section 6.4 (garbage collection of derived files), SDD-000 section 9 (the workspace layout) and section 10 (the configuration), SDD-012 (the preview).
- Epic 34 (the processors and the "Listen" render, done).

## Scope

- Modules: `engine/audio/` and `engine/export/audio*.go` (an audio-only render of one item's chain, before the loudness gain), `service/` (the cache of processed audio: key, render, store; `service/audio_measure.go` is the nearest existing code), `core/plan/` (only if the content key needs a field the audio plan does not carry), `web/app/http/` (the route of a cached file), `web/resources/js/editor/preview/` (`mediaSync.ts`, `audioGraph.ts` and `mediaPool.ts`: the audio source of an item switches to the processed file), `web/resources/js/editor/inspector/panels/AudioProcessors.tsx` (the states "processing" and "ready"), `web/resources/lang/en.json` and `pt-BR.json`, SDD-004 (section 7.1), SDD-003 (sections 6.3 and 6.5), SDD-007 (the table of byte-serving routes), SDD-012 (the preview), SDD-000 (the folder of the workspace).
- Clips and audio items with their own chain. Each processed file is keyed by the content of its audio entry (the chain with its parameters, the speed, the volume, the fades and the asset), not by the timeline's revision, so an edit of another clip does not invalidate it.

## Out of scope

- The chain of a track (the bus chain of epic 34): the timeline keeps playing a track with a bus chain without it, and the inspector says so. This is the one difference from the export that stays, until a later epic.
- The picture, the export (already processed), and any DSP in the browser: the processing is in Go only.
- A model-based noise reduction (epic 34, task 10, stays a STOP for the author).

## Dependencies

34 (the processors and the processed-sound render, done).

Not in progress together with 53 (both change `engine/audio` and SDD-004 section 7), nor with 52 (both change `web/resources/js/editor/preview/` and SDD-012).

## Technical design

- **Key.** The `sha256` of the canonical form of the audio entry that the plan gives the item (SDD-004 section 7.1): the `chain` with complete parameters, the speed, the volume, the fades and the asset hash. The same key gives the same bytes; a changed parameter is a new key.
- **Gain.** Step 3 of SDD-004 section 7 applies one loudness gain to the whole mix, so the cached file holds the processed audio before that gain, and the preview applies the gain the plan gives. Then the preview and the export differ only by the rule of a window (SDD-004 section 7.1). The senior confirms in the code which gain the plan carries per source.
- **Window first.** After a change of a parameter, the cache renders first the two seconds at the playhead, so that the change is heard within about a second; the rest of the item is rendered after it, in the background, as one job of the kind the "Listen" render of epic 34 uses. Until a window is ready the item plays unprocessed, and the inspector shows "processing". The change is never silent.
- **Store.** The files go in the workspace, in a folder for processed previews that SDD-000 section 9 names. They are written atomically (SDD-011 section 6.2), are immutable and named by their key, and so are served with the caching of SDD-003 section 6.5. The garbage collection of SDD-011 section 6.4 applies to them.
- **Route.** `GET /projects/{id}/audio-previews/{key}`, scope `read`, checked against the project as the other byte routes are (SDD-003 section 6.3 and 6.5). A key that the project did not hand out is `404`.
- **Player.** The audio element of the item switches to the processed file when its key is ready, with the same clock and the same drift rule as the proxy (SDD-004 section 10, item 2). No DSP in TypeScript.
- **Measure.** The time from a change of a parameter to the first ready window is measured on the reference clip with every agent stopped; the number goes into the hand-off.

## Tasks

- [x] 1. Amend SDD-004 section 7.1 (the key, the window rule, the gain rule; the track chain is excluded; the open question is answered by author decision D-094), SDD-003 (sections 6.3 and 6.5), SDD-007 (the byte-serving table), SDD-000 (the folder) and SDD-012 (the preview). Verify: `task lint:md` exits 0.
- [x] 2. Render one item's chain before the loudness gain, and key it. Verify: `go test ./engine/audio/... ./engine/export/... -run "TestAudioChain|TestAudioPreviewKey"` passes.
- [x] 3. The cache in `service/`: the window first, the background job, the atomic store, the reuse by key. Verify: `go test ./service/... -run TestAudioPreviewCache` passes.
- [x] 4. The route, with `Range` and the project check. Verify: `go test ./web/app/http/... -run TestAudioPreviewRoute` passes (206 for a range, 404 for a key the project did not hand out).
- [x] 5. The preview switches the item's audio element; the inspector shows "processing" and "ready". Verify: `task test:web` and `bun run check:thin-client` pass, and `web/e2e/specs/audio-processed-preview.spec.ts` (new, `bun run e2e`) passes.
- [x] 6. Parity: the export of the whole timeline equals the preview of the same revision within the audio tolerance. Verify: `go test ./engine/export/... -run TestAudioChainExportEqualsPreview` passes.
- [x] 7. Both languages, and the measurement. Verify: `bun run check:i18n` passes; the measured time is in the hand-off.

## Test strategy

- Unit: the key (the same input gives the same key; a change in another clip leaves it); the length of the window; the status codes of the route.
- Golden audio: the processed clip before the gain equals the golden WAV of epic 34 for the same chain.
- Parity: the preview equals the export for the whole timeline (`TestAudioChainExportEqualsPreview`).
- End to end: change a parameter; the processed window plays within the measured time; the element's source changes to the processed file.

## Acceptance criteria

1. After a change of a parameter the processed window of the item is ready within three seconds on the test fixture: `go test ./service/... -run TestAudioPreviewWindowLatency` passes; the number on the reference clip is in the hand-off.
2. Two items with the same chain share one file, and an edit of another item creates no new file: `go test ./service/... -run TestAudioPreviewKeyIsPerClip` passes.
3. The audio the preview plays for a whole timeline equals the export: `go test ./engine/export/... -run TestAudioChainExportEqualsPreview` passes.
4. The processed file is served with `Range`, and only to its own project: `go test ./web/app/http/... -run TestAudioPreviewRoute` passes.
5. `web/e2e/specs/audio-processed-preview.spec.ts` passes.
6. `task lint:md`, `task generate:check`, `go test ./core/... ./engine/audio/... ./engine/export/... ./service/... ./web/contract/...`, `task test:web`, `bun run check:i18n` and `bun run check:thin-client` pass.

## Risks and mitigation

- **Disk growth.** Each key is a file. Mitigation: the collection of SDD-011 section 6.4, and a ceiling in the configuration (SDD-000 section 10), both written into the spec before the code.
- **A window that differs from the export.** The start of a window differs from the export (SDD-004 section 7.1). It is stated in the spec and tested; it is not hidden.
- **STOP** before a DSP library or any DSP in the browser (NFR-043, ADR-0011), and before the track's chain is added to this epic: that is a change of scope.
- **A new byte route is a new surface.** Its entry in SDD-007 follows the rule of the bitmaps route. If the author wants another rule, **STOP**.

## Size

M

## Hand-off for review

The diff of `epic/51-processed-audio-preview-on-the-timeline`; the measured latency and whether the machine was idle while it was measured; the golden and parity outputs; the results of the acceptance commands.
