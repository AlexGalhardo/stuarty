# 118: Professional Sound Editor: multi-track audio core, waveform and spectrogram UI, mastering FX rack, live recording and multi-format export

## Goal

Stuarty gains a dedicated, professional **Sound Editor** (`Editor de Som`, `/edit-sound`) alongside its Video Editor and Image Editor—usable both standalone (drop an audio file or record a voiceover, edit, master, and export) and linked inside production projects. Creators get sample-accurate non-destructive multi-track editing, an interactive waveform and spectrogram canvas, live microphone recording and punch-in, a visual channel strip and mastering FX rack (Parametric EQ with live FFT spectrum, Compressor, De-Esser, Gate, Reverb/Delay, Noise Reduction, and True-Peak Limiter), real-time broadcast LUFS loudness metering with platform presets, and multi-format export (WAV, FLAC, MP3, AAC, Opus)—all powered by Stuarty's deterministic Go command core, 100% MIT/Apache/LGPL audio DSP tooling, and full HTTP, CLI, and MCP agent parity.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback):
  - Research open-source audio editors and tooling on the internet and create a dedicated epic to build a **professional Sound Editor** inside Stuarty.
- Open-source research and architecture findings (2026-11-27) audited against Stuarty's rules (ADR-0002 Go command core, ADR-0011 LGPL FFmpeg only, ADR-0038 multi-document architecture, AGENTS.md rule 5 & 9 — **strictly zero GPL components**):
  - **Reference UX studied (architecture inspiration only — GPL-licensed, never linked or copied)**: **Audacity / Tenacity** (block-based waveform editing, spectral repair view, noise profile cleanup, podcast chains), **Ardour** (non-destructive multi-track routing, buses, automation lanes, lock-free audio graph), **master_me** (one-click live/podcast mastering strip), **Adobe Audition** (waveform + multitrack views, essential sound, match loudness).
  - **Already built in Stuarty (`engine/audio/` + `core/timeline` from epics 34, 51, 58, 71)**: pure-Go audio DSP (`chain.go`, `processors.go`, `limiter.go`, `loudness.go`, `meter.go`, `mix.go`, `denoise.go`, `noise_reduction.go`, `wav.go`), audio preview rendering (`service/audio_preview.go`), loudness measurement (`service/audio_measure.go`), sound asset peak generation (`service/peaks_job.go`), and studio voice presets (`studio`, `broadcast`, `room_rescue`).
  - **Recommended MIT / Apache-2.0 / LGPL open-source tooling for the Sound Editor**:
    1. **Browser real-time playback & metering**: native **Web Audio API (`AudioWorklet`)** + HTML5 `<canvas>` peak/waveform/FFT spectrogram renderer (zero new GPL dependencies, zero-latency scrubbing, live RTA spectrum & LUFS/True-Peak meters).
    2. **Offline deterministic DSP & multi-format export**: extend Stuarty's existing pure-Go `engine/audio/` DSP graph + **LGPL FFmpeg** (`loudnorm` EBU R128 / ITU-R BS.1770-4, `atempo`/`aresample`, WAV 16/24/32-bit float, FLAC, LGPL LAME MP3, AAC/M4A, Opus/OGG).
    3. **Optional Rust/WASM or local ONNX studio DSP upgrades (gated behind author dependency approval STOP)**:
       - **Pure-Rust MIT/Apache DSP** (`nnnoiseless` [MIT pure-Rust RNNoise], `fundsp` [MIT/Apache-2.0], `dasp` [MIT/Apache-2.0], `symphonia` [MPL-2.0]) compiled to WebAssembly for bit-exact browser/export real-time denoising & FX preview.
       - **DeepFilterNet** (MIT/Apache-2.0 ONNX model) hosted via Stuarty's existing optional local ONNX helper (`aihelper/`, ADR-0050) for studio-grade speech denoising & voice isolation, plus optional open-source **stem separation** (vocals/music/SFX via ONNX) or connected AI audio providers (ElevenLabs Voice Isolation / SFX from epic 109).
- Design workflow: design the `/edit-sound` landing page and Sound Editor studio screen first in `docs/design/final/stuarty-redesign-final-trilha.figcraft` (after epic 116) before building the UI!
- New specification reserved: **SDD-026: Audio documents and the Sound Editor** (written as task 1, names first in `docs/sdds/000-shared-definitions.md`).

## Scope

- Modules owned:
  - `docs/design/final/stuarty-redesign-final-trilha.figcraft` (design frames for `/edit-sound` launcher and `/edit-sound/{audio_id}` studio: multi-track timeline + waveform/spectrogram view, FX rack with visual EQ curve, mixer & LUFS loudness meter, export dialog).
  - `docs/sdds/026-audio-documents.md` (new SDD), `docs/sdds/000-shared-definitions.md`, `docs/sdds/003-agent-interfaces.md`, `docs/sdds/005-data-model-and-persistence.md`, `docs/sdds/007-security-model.md`, `docs/sdds/012-ui-architecture.md`.
  - `core/audio/` (new pure-Go domain package, following `core/image/` of ADR-0038: audio document model `aud_` IDs at 48 kHz sample-accurate integer frames, multi-track [`voice`, `music`, `sfx`, `bus`], non-destructive clips, split/trim/slip/ripple/delete-silence/fade-in/fade-out/crossfade, volume & pan automation envelopes, per-track & master FX chains [parametric EQ, compressor, de-esser, noise gate, noise reduction, reverb, delay, limiter, pitch/speed], markers/podcast chapters, undo/redo, checkpoints & diff, audio lint [clipping > -0.1 dBTP, LUFS target deviation, dead air > 3s, phase/mono compatibility]).
  - `web/database/migrations/` (tables for `audio_documents`, `audio_revisions`, `audio_checkpoints`—sealed under ADR-0053 when local account encryption is active!).
  - `engine/audio/` (extend existing pure-Go DSP engine with parametric multi-band EQ, algorithmic reverb/delay, automation envelope interpolation, spectrogram FFT tile generator, true-peak oversampled meter, and ITU-R BS.1770-4 integrated/short-term/momentary LUFS analyzer) and `engine/export/` (multi-format audio export: WAV 16/24/32f, FLAC, MP3, AAC/M4A, Opus with ID3/Vorbis metadata & podcast chapter tags).
  - `service/audio_doc*.go` (CRUD, command batch apply, history/revert/checkpoint/diff, standalone quick-open from file or Sound Asset hash, live mic take ingestion, loudness analysis job, export job, "Save to Sound Assets" & "Send to Video Editor").
  - `web/app/` (HTTP routes `/api/v1/audio-docs/...`, CLI `stuarty sound:*`, MCP `sound` domain actions—check NFR-020 budget!).
  - `web/routes/dashboard.go` and `web/routes/audio_pages.go` (register `GET /edit-sound` and `GET /edit-sound/{audio_id}`).
  - `web/resources/js/dashboard/sections.ts` (add `edit-sound` in the `Create` sidebar group right beside `edit-video` and `edit-image`, plus double-click on Sound Assets in `SoundRow.tsx` opening `/edit-sound?asset=<hash>`!).
  - `web/resources/js/pages/EditSound/` and `web/resources/js/sound-editor/` (standalone drag-and-drop launcher + full Sound Editor UI: multi-track waveform & spectrogram `<canvas>`, sample-accurate zoom/scrub, selection & split/fade handles, live mic recording & punch-in with monitoring/countdown, interactive Parametric EQ curve + FFT analyzer, Compressor/De-Esser/Gate/Reverb/Noise Reduction/Limiter FX rack with one-click voice/podcast presets, live LUFS & True Peak loudness meters, markers/chapters editor, and multi-format Export dialog).
  - `web/resources/lang/en.json` and `web/resources/lang/pt-BR.json`.
- Must not touch: `renderer/` pixel renderer, `core/image/`, `core/timeline/`, `service/providers/`.

## Out of scope

- MIDI sequencing, virtual synthesizers (VST3/AU/CLAP third-party binary plugin hosting — arbitrary native plugin DLLs conflict with SDD-007 security policy), and music notation/scoring.
- Any GPL audio library (Audacity/Ardour/FFTW-GPL/`master_me`/SoX-GPL are strictly excluded per AGENTS.md rule 5).
- Downloading DeepFilterNet or stem-separation ONNX weights automatically (optional local models stay opt-in per ADR-0050 / epic 72).

## Dependencies

- Epics 34, 51, 58, 71, 105, 106, and 116 (existing `engine/audio/` DSP, sound assets, standalone editor pattern, loose asset library, and versioned FigCraft design).

## Technical design

1. **Third Document Type (`core/audio/`, following ADR-0038)**:
   - Sample-accurate integer sample timeline (`sample_rate`: `48000` default or `44100`; positions and durations in integer samples so zero floating-point drift occurs across machines or edits).
   - Pure `audio.ApplyBatch(doc, ops, env)` with full undo/redo compensating batches, conflict detection per object, checkpoints, and diff—identical ergonomics to `core/timeline` and `core/image` so AI agents operate the Sound Editor fluently!
   - Core command catalogue (~28 commands): `audio_doc.create`, `track.add/update/move/remove/solo/mute`, `clip.add/move/trim/split/delete/duplicate/set_gain/set_fade/crossfade/reverse/set_speed`, `clip.remove_silences`, `envelope.set_point/remove_point`, `fx.add/set/move/remove/apply_preset`, `master.set_target_lufs`, `marker.add/set/remove`, `loudness.normalize`.

2. **Hybrid Real-Time Preview + Deterministic Go/FFmpeg Export**:
   - **Browser Preview (`web/resources/js/sound-editor/audioEngine.ts`)**: Web Audio API (`AudioContext` + `AudioWorklet` metering & BiquadFilter/DynamicsCompressor/Convolver/Gain graph) decodes normalized proxy/PCM chunks for instant play/scrub/solo/mute/EQ-tweaking at <20 ms latency, plus server-rendered preview segments (`service/audio_preview.go`) for heavy offline effects (noise reduction / spectral cleanup / de-esser) so what you hear matches export.
   - **Deterministic Export (`engine/audio/` + LGPL FFmpeg)**: mixes all tracks, automation envelopes, per-track FX, and master bus in 64-bit float internal precision (`engine/audio/`), measures ITU-R BS.1770-4 LUFS and oversampled True Peak (dBTP), applies target loudness normalization (YouTube `-14 LUFS`, Podcast Stereo `-16 LUFS`, Podcast Mono `-19 LUFS`, Broadcast `-23 LUFS`), and encodes via LGPL FFmpeg to WAV (16/24/32f), FLAC, MP3, AAC/M4A, or Opus with metadata tags and chapter markers.

3. **Zero-Dependency Baseline + Optional Rust/ONNX Upgrade Path**:
   - Baseline ships with **zero new Go, npm, or Rust dependencies** (pure Go `engine/audio/` + LGPL FFmpeg + native Web Audio API & HTML5 Canvas!).
   - Optional task/decision gate (STOP before adding any crate): evaluate adding `nnnoiseless` (MIT RNNoise) in WASM or DeepFilterNet ONNX in `aihelper/` (ADR-0050) for neural denoising & stem separation if the author approves those dependencies.

4. **Standalone & Integrated Studio Workflows**:
   - `/edit-sound`: drop any WAV/MP3/FLAC/M4A/OGG/Opus (or video file to extract its audio!) OR click "Record voice" to launch an instant standalone editing session without creating a project first (mirroring `/edit-video` and `/edit-image` of epic 105).
   - Double-clicking any Sound Asset in `/sound-assets` (`SoundRow.tsx`) opens `/edit-sound?asset=<hash>`.
   - One-click **"Save mastered audio to Sound Assets"** or **"Send to Video Editor"** drops the finished audio straight into the library or an open video timeline!

## Tasks

- [ ] 1. Design the `/edit-sound` launcher and `/edit-sound/{audio_id}` Sound Editor screens in `docs/design/final/stuarty-redesign-final-trilha.figcraft`, and write `docs/sdds/026-audio-documents.md` + update SDD-000, SDD-003, SDD-005, SDD-007, and SDD-012. **STOP** if adding optional Rust DSP crates (`nnnoiseless`/`fundsp`) or raising the NFR-020 MCP byte budget is needed. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Implement `core/audio/` (document schema, ~28 commands, undo/redo, checkpoints, diff, audio lint, golden cases for every command) and run `task codegen`. Verify: `go test ./core/audio/... ./core/schema/...` passes.
- [ ] 3. Add DB migration for `audio_documents` / `audio_revisions` / `audio_checkpoints` (with ADR-0053 sealing when local account is locked) and extend `engine/audio/` + `engine/export/` (multi-band parametric EQ, reverb/delay, automation envelopes, spectrogram FFT tiles, ITU-R BS.1770-4 LUFS + True Peak analyzer, WAV/FLAC/MP3/AAC/Opus export with chapters). Verify: `go test ./engine/audio/... ./engine/export/... ./web/database/...` passes.
- [ ] 4. Implement `service/audio_doc*.go` (CRUD, batch edit, standalone quick-open, mic take ingest, loudness analysis, export job, Save to Sound Assets) and expose HTTP, CLI, and MCP surfaces with parity and security tests. Verify: `go test ./service/... ./web/app/... ./web/contract/...`, `go test -run TestSecurity_ ./...`, and `go run ./internal/tools/surfacebudget` pass.
- [ ] 5. Register `/edit-sound` and `/edit-sound/{audio_id}`, add `edit-sound` to `dashboard/sections.ts` and `shell/breadcrumb.ts`, wire double-click on `SoundRow.tsx`, and build the standalone launcher and Sound Editor UI (`web/resources/js/pages/EditSound/` and `web/resources/js/sound-editor/`: waveform & spectrogram canvas, multi-track timeline, live mic recording, interactive EQ & FX rack, LUFS/True-Peak meters, chapters, multi-format export); add `en.json` and `pt-BR.json` strings. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run resources/js/sound-editor/ resources/js/pages/EditSound/`.
- [ ] 6. Add Playwright E2E tests proving a user can drop an audio file into `/edit-sound` (with 0 projects), trim, apply a Parametric EQ + Studio Voice preset, normalize to `-16 LUFS`, and export a verified WAV/MP3 file (and inspect spectrogram/meters), plus verify screenshots against FigCraft. Verify: `task lint`, `task test`, `task test:web`, and the Playwright sound-editor specs pass.

## Test strategy

- Core golden tests: every `core/audio` command has valid + error golden cases, undo/redo inverse proof, and fuzz tests.
- DSP & export verification tests: sine-wave & speech fixture tests verifying EQ frequency response, compressor gain reduction, True Peak <= -1.0 dBTP after limiter, integrated LUFS within +/-0.5 LU of target (`-14`, `-16`, `-19`, `-23`), byte-identical determinism across two runs, and ffprobe verification of WAV/FLAC/MP3/AAC/Opus headers and chapter metadata.
- Security tests: sealed audio document titles/chapters unreadable when local workspace is locked (`E_LOCKED`), network-mode user isolation, zero GPL licence check (`scripts/ffmpeg-verify.sh`), and `STUARTY_MAX_MEDIA_BYTES` caps.
- UI & E2E: Vitest tests for waveform/EQ UI controls and `check:thin-client`; Playwright E2E full standalone import -> edit -> FX -> normalize -> export round-trip.

## Acceptance criteria

1. The main sidebar (`Create` group) includes **Edit Sound** (`Editor de Som`) linking to `/edit-sound`, and double-clicking any Sound Asset opens it directly in the Sound Editor.
2. A creator can drop an audio file (or record from a microphone) into `/edit-sound` with zero pre-created projects, edit across multiple tracks with interactive waveform and spectrogram views, apply Parametric EQ, Compressor, De-Esser, Gate, Noise Reduction, Reverb, and Limiter presets, monitor real-time LUFS/True Peak loudness, and export verified WAV, FLAC, MP3, AAC, or Opus files.
3. Every Sound Editor edit is stored as a deterministic `core/audio` command with full undo/redo, checkpoints/diff, and parity across UI, HTTP API, CLI, and MCP.
4. Zero GPL components are introduced (`scripts/ffmpeg-verify.sh` and licence checks exit 0), and exported audio meets target LUFS within +/-0.5 LU and True Peak <= -1.0 dBTP.
5. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and Playwright sound-editor specs pass.

## Risks and mitigation

- **New dependency approval (AGENTS.md rule 9).** Mitigation: the core MVP requires zero new dependencies (pure Go `engine/audio/` + LGPL FFmpeg + Web Audio API + Canvas); **STOP** before adding any Rust DSP crate (`nnnoiseless`, `fundsp`, `symphonia`) or ONNX neural model (`DeepFilterNet`).
- **MCP `tools/list` byte budget (NFR-020).** Mitigation: route audio-document operations through the existing `edit` / `asset` domain parameter pattern; **STOP** if `surfacebudget` requires a documented raise.
- **Large multi-hour podcast files in browser memory.** Mitigation: generate downsampled peak tiles and spectrogram chunks on the server (`service/peaks_job.go` + streamed range playback) so the browser never loads a multi-gigabyte uncompressed WAV into RAM.

## Size

L

## Hand-off for review

The commits of `epic/118-professional-sound-editor`, new `docs/sdds/026-audio-documents.md`, updated FigCraft frames, DSP/LUFS/export verification outputs, `surfacebudget` output, Vitest/Playwright outputs, and light/dark screenshots of the Sound Editor.
