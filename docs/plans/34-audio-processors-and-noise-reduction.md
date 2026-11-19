# 34: Audio processors and noise reduction

## Goal

A creator who recorded in their bedroom gets a voice that sounds clean and even: less hiss, less boom, controlled peaks, softer "s" sounds. A few presets do it for them; the controls are there when they want them.

## Context and related decisions

- ADR-0042, ADR-0011 (only LGPL FFmpeg; the GPL filters stay absent), ADR-0003.
- Author decision D-066 (equaliser, compressor, limiter, gate and de-esser, plus local noise reduction with a permissively licensed model). PRD-001 RF-085, RF-086, NFR-043.
- SDD-004 (the render pipeline), SDD-002. Read `.agents/skills/deterministic-media/`.

## Scope

- Modules: `engine/audio/` (the processors, in Go, inside the existing mixer), `core/timeline/` and `core/command/` (the processor chain of a clip and of a track, with golden cases), `core/plan/` (the audio plan), the audio inspector in `web/resources/js/editor/`, SDD-001, SDD-002, SDD-004.
- Processors: parametric equaliser (up to six bands, with high-pass and low-pass), compressor, limiter, noise gate, de-esser, noise reduction. Presets: "voice", "voice with music", "podcast", "clean up a noisy room".

## Out of scope

- A mixer view with buses and sends, plug-in formats (VST, AU), spectral editing, reverb removal, automatic dialogue replacement.
- Music stem separation.
- Processors implemented as FFmpeg filters: the packaged build's filter list does not carry them and the mixer is already in Go.

## Dependencies

03.

## Technical design

- **Where it runs**: `engine/audio` already mixes in Go and already has a limiter and a denoise file. Read them first: this epic extends that code, it does not start a second path. Every processor is a pure function over PCM blocks with explicit state, so preview and export run the same code and match by construction.
- **Determinism**: fixed block size, specified coefficient formulas (biquads from the standard audio cookbook formulas, written into SDD-004), no dependence on the machine's vector width; golden WAV files compared exactly on the same system and within a stated tolerance across systems, following the existing audio goldens.
- **Model**: a clip and a track each have an ordered `processors` list (kind, parameters, enabled). Order matters and is the user's; the lint warns about a limiter that is not last and about more than one noise reduction in a chain.
- **Commands**: `audio.processor_add`, `audio.processor_set`, `audio.processor_move`, `audio.processor_remove`, `audio.apply_preset`; parameters validated in the core with ranges and units (decibels, hertz, milliseconds, ratio).
- **Noise reduction**: first a spectral method with no model (a noise profile estimated from the quietest part of the clip, or from a range the user marks), which needs no dependency. A model-based method is added only with a model whose licence is permissive and whose file is approved by the author; it is an optional download by the explicit install step, like the transcription model, never fetched on its own.
- **Preview**: processed audio is rendered by the server into the cache per clip revision and streamed to the preview as today's audio is; changing a parameter re-renders a short window first so the person hears the change quickly.
- **Metering**: `AudioMeasure(clip or range)` returns integrated loudness, true peak and noise floor before and after the chain, for agents and for the inspector.

## Tasks

- [x] 1. Read `engine/audio`; amend SDD-004 (the processors, their formulas, the order, the tolerance) and SDD-001 and SDD-002 (the chain and the commands). Verify: `task lint:md` exits 0.
- [ ] 2. Implement the chain model and the five commands with golden cases. Verify: `go test ./core/command/... ./core/timeline/...` passes with the updated registry snapshot.
- [ ] 3. Implement the equaliser (bands, high-pass, low-pass). Verify: `go test ./engine/audio/... -run TestEqualiser` passes (frequency response within stated bounds; golden WAV).
- [ ] 4. Implement the compressor, the gate and the de-esser; extend the limiter. Verify: `go test ./engine/audio/... -run "TestCompressor|TestGate|TestDeEsser|TestLimiter"` passes.
- [ ] 5. Implement spectral noise reduction with a marked or estimated profile. Verify: `go test ./engine/audio/... -run TestNoiseReduction` passes (noise floor reduced by a stated amount on the fixtures; speech band preserved within bounds).
- [ ] 6. Wire the chain into the mixer for clips and tracks, in preview and export. Verify: `go test ./engine/export/... -run TestAudioChain` passes and the exported audio equals the previewed render for the same revision.
- [ ] 7. Implement the presets and `AudioMeasure`. Verify: `go test ./service/... -run "TestAudioPreset|TestAudioMeasure"` passes.
- [ ] 8. Expose on the three surfaces and regenerate documents. Verify: `go test ./web/contract/...` and `task generate:check` pass.
- [ ] 9. Build the audio inspector (chain, per-processor controls, presets, before and after meters), in both languages. Verify: `task test:web`, `bun run check:thin-client` and `web/e2e/specs/audio-processors.spec.ts` pass.
- [ ] 10. Propose a model-based noise reduction with its licence, size and source, and STOP for the author; implement it only if approved. Verify: the proposal and the decision are in the hand-off.

## Test strategy

- Unit: each processor against analytic expectations (response curves, gain reduction, attack and release times).
- Golden audio for each processor and for the presets.
- Parity: preview render equals export for the same revision.
- Performance: processing faster than real time on the reference machine, number recorded.

## Acceptance criteria

1. Each processor can be added to a clip and to a track, reordered and removed, on HTTP, CLI and MCP.
2. The "voice" preset on the noisy fixture lowers the measured noise floor and keeps integrated loudness at the project target (numbers in the test).
3. The exported audio of a revision equals the previewed audio of the same revision.
4. Golden audio matches on Windows and in the Linux Docker test within the stated tolerance (`bash scripts/test-linux-docker.sh ./engine/audio/...`).
5. No GPL component is added and `task notices:check` exits 0.
6. `go test ./core/... ./engine/audio/... ./engine/export/... ./web/contract/...`, `task test:web` and `task generate:check` pass.

## Risks and mitigation

- **Audio artefacts from a naive noise reduction.** Mitigation: conservative defaults, a strength control, listening fixtures reviewed by the author.
- **Floating point differences between systems.** Mitigation: the existing tolerance policy for audio; specified formulas.
- **STOP** at task 10, and before any DSP library.

## Size

L

## Hand-off for review

The diff of `epic/34-audio-processors-and-noise-reduction`; the formulas added to SDD-004; before and after measurements on the fixtures; the model proposal and its outcome.
