# 115: Video editor parity with CapCut, Premiere Pro and DaVinci Resolve: speed ramps, stabilization, motion tracking, creator transitions, color wheels and beat-sync montage

## Goal

Creators editing videos in Stuarty get the remaining high-demand features from **CapCut**, **Adobe Premiere Pro**, and **DaVinci Resolve** that Stuarty does not yet have—**variable speed ramping** with curve presets, **video stabilization**, **2D motion tracking (pin text, stickers, or blur to a moving subject)**, **modern creator transitions and J/L-cut split handles**, **3-way color wheels (Lift/Gamma/Gain), Auto-Color and real-time video scopes**, and **one-click Beat-Sync Montage**—all backed by `core/timeline` and deterministic preview/export parity.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #16 (Video editor gap analysis)**: Researched the most heavily used features in CapCut, Adobe Premiere Pro, and DaVinci Resolve (2025–2026) and audited them against `docs/sdds/002-command-catalogue.md` (Stuarty's 97 existing video commands):
    - **Already built in Stuarty**: multi-track timeline, trim/split/slip/ripple, freeze frame, constant speed, chroma key, blend modes, animated crop & masks, keyframe easing curves, animation presets, basic color & `.cube` LUTs, shadow/outline, audio processors, noise reduction, voice presets, audio ducking, beat markers (`audio.mark_beats`), text-based editing & filler removal, face-aware vertical cuts, screen recording auto-zoom/cursor/click highlights, dynamic caption styles, nested compositions, batch presets, H.265/AV1.
    - **Highest-demand features genuinely missing**:
      1. **Speed Ramping / Variable Speed Curves (CapCut / Premiere / Resolve)**: currently `clip.set_speed` only supports a flat constant multiplier; creators rely daily on dynamic curve ramps (`Montage`, `Hero`, `Bullet`, `Flash In`, `Flash Out`, custom Bezier speed points) within a single clip!
      2. **Video Stabilization (Premiere Warp Stabilizer / CapCut / Resolve)**: handheld smartphone/camera footage needs smooth stabilization—completely missing today.
      3. **2D Motion Tracking / Pin-to-Object (CapCut / Premiere / Resolve)**: attach text callouts, stickers, shapes, or sensitive-data blur regions so they automatically follow a moving person or object across frames.
      4. **Modern Creator Transitions & J/L-Cut Quick Workflow (CapCut / Premiere)**: expanded transition library (`whip_pan`, `zoom_in`, `zoom_out`, `glitch`, `light_leak`, `slide_push`, `spin`) + split audio/video L-cut & J-cut handles (`item.set_audio_offset`).
      5. **3-Way Color Wheels (Lift/Gamma/Gain), Auto-Color Balance & Video Scopes (Resolve / Premiere Lumetri)**: visual color wheels, one-click Auto-Color balance analysis, and live Waveform / Vectorscope / RGB Parade scopes in the preview panel.
      6. **Auto Beat-Sync Montage (CapCut)**: automatically align/trim selected clips and transitions onto detected beat markers (`audio.mark_beats`) in one click.
- ADR-0002 (pure command core in Go), ADR-0004 (integer frame time model — speed-ramp curves map output frame integers deterministically to exact source frame timestamps!), ADR-0011 (LGPL FFmpeg only — check whether `vidstab` is GPL! **CRITICAL**: `libvidstab` is GPL-2.0 and therefore **FORBIDDEN by ADR-0011 / AGENTS.md rule 5**! Video stabilization MUST be implemented in LGPL-compatible pure Go/Rust motion estimation + deterministic affine transform plan, NEVER `libvidstab`!), ADR-0029 (one Rust WASM renderer for preview and export), SDD-000, SDD-001, SDD-002, SDD-004, SDD-013.

## Scope

- Modules owned:
  - `docs/sdds/000-shared-definitions.md`, `docs/sdds/001-timeline-schema.md`, `docs/sdds/002-command-catalogue.md`, `docs/sdds/004-render-pipeline.md`, `docs/sdds/013-renderer.md`.
  - `core/timeline/`, `core/command/`, `core/plan/` (new commands with golden cases: `clip.set_speed_ramp`, `clip.set_stabilization`, `clip.set_motion_track` + `item.attach_track`, expanded transition kinds in `item.set_transition`, `item.set_audio_offset` for J/L-cuts, `clip.set_color_wheels` & `clip.auto_color`, `timeline.snap_to_beats`).
  - `engine/ingest/` or `engine/analysis/` (pure-Go / Rust LGPL-clean 2D optical-flow / block-matching analysis jobs producing: (a) stabilization camera-motion transform track, (b) 2D point/region motion track keyframes for pin-to-object, and (c) auto-color histogram/white-balance parameters — zero GPL dependencies!).
  - `engine/export/` (variable speed-ramp frame sampling + pitch-preserving segmented `atempo` audio ramp, stabilization transform application, tracked attachment evaluation, new transitions, color wheels).
  - `renderer/` (Rust WASM shader/compositor passes for new transitions [`whip_pan`, `zoom_blur`, `glitch`, `light_leak`, `slide_push`, `spin`] and 3-way Lift/Gamma/Gain color wheels — byte-identical preview & export!).
  - `web/app/` (HTTP, CLI, MCP parity for all new commands and analysis jobs within NFR-020 budget).
  - `web/resources/js/editor/` (Speed Ramp curve editor & CapCut-style presets, Stabilization toggle & smoothness/zoom sliders, Motion Track target box & "Attach to track" picker, Transitions gallery & J/L-cut split handles on timeline clips, 3-Way Color Wheels + Auto-Color button + live Waveform/Vectorscope/RGB Parade preview scope widget, "Sync clips to beats" toolbar action).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `core/image/`, `web/resources/js/image-editor/`, `desktop/`, `service/providers/`.

## Out of scope

- Any GPL library or FFmpeg GPL filter (`libvidstab` is strictly forbidden per ADR-0011; stabilization uses our own MIT/LGPL-clean motion estimator + renderer affine transform).
- 3D camera solving or AI optical-flow frame interpolation models (can be considered in a future optional local-model epic).
- Image editor features (owned by epic 114).

## Dependencies

- Epics 26, 31, 34, 53, 69, 71, 85, 114, and 116 (run sequentially after epic 114 because both touch `renderer/`; update `docs/design/final/stuarty-redesign-final-trilha.figcraft` for Speed Ramp, Motion Track, Color Wheels/Scopes, and Beat-Sync panels).

## Technical design

1. **Speed Ramping (`clip.set_speed_ramp`)**:
   - Stores an ordered list of integer speed-ramp control points `[{pos_permille: 0..1000, speed_pct: 25..1600, easing}]` plus built-in preset IDs (`montage`, `hero`, `bullet`, `jump_cut`, `flash_in`, `flash_out`).
   - `core/plan` integrates the piecewise speed curve in integer/rational arithmetic to compute exact output duration (in frames) and exact `source_frame` for every output frame `t`, guaranteeing 100% deterministic preview and export sync!
   - Audio uses segmented `atempo` chain (or optional mute on ramps > 4x) in `engine/export/audio.go`.

2. **LGPL-Clean Video Stabilization (`clip.stabilize` / `clip.set_stabilization`)**:
   - Analysis job decodes proxy frames via LGPL FFmpeg `pipe:1` and estimates frame-to-frame 2D translation + rotation via pure-Go/Rust phase-correlation / pyramidal Lucas-Kanade block matching (MIT/stdlib, zero GPL!).
   - Stores smoothed compensation trajectory as an artifact; `core/plan` applies the smoothed `(dx, dy, angle)` + minimal auto-crop zoom (`smoothness: 0..100`, `max_crop_pct: 0..25`) directly through the existing transform/crop frame plan, so preview and export match automatically!

3. **2D Motion Tracking (`clip.track_motion` + `item.attach_track`)**:
   - User draws a target box `(x, y, w, h)` on a frame; analysis job tracks the template patch across the clip's frames using the same LGPL-clean tracker, saving a dense motion-track artifact.
   - Any overlay item (`text`, `shape`, `clip`, or `blur_region`) can attach via `item.attach_track(track_ref, offset_x, offset_y, follow_scale)` and `core/plan` resolves its per-frame `(x, y)` deterministically!

4. **Creator Transitions, J/L-Cuts, Color Wheels, Scopes & Beat-Sync Montage**:
   - **Transitions**: add `whip_pan`, `zoom_in`, `zoom_out`, `glitch`, `light_leak`, `slide_push`, `spin` to `core/command` + `renderer/` with deterministic golden frame tests.
   - **J/L-Cuts**: `item.set_audio_offset(audio_in_delta, audio_out_delta)` lets a video clip's linked audio start earlier (J-cut) or extend past the video cut (L-cut) with draggable split handles on the timeline.
   - **Color Wheels & Scopes**: extend `clip.set_color` with integer `lift`, `gamma`, `gain` RGB offsets (-100..100) evaluated in `renderer/`; add `clip.auto_color` computed from frame luminance/chroma histogram; render live Waveform, Vectorscope, and RGB Parade scopes in the browser preview canvas from the already-rendered preview frame pixels (read-only visualization, zero core rule in TypeScript!).
   - **Beat-Sync Montage**: `timeline.snap_to_beats` takes selected video clips and a reference audio clip's beat markers (from existing `audio.mark_beats`) and deterministically trims/places cuts on every 1st, 2nd, or 4th beat with optional transition preset.

## Tasks

- [ ] 1. Specify all new timeline schema fields, commands (`clip.set_speed_ramp`, `clip.set_stabilization`, `item.attach_track`, `item.set_audio_offset`, `clip.set_color_wheels`, `timeline.snap_to_beats`, expanded transitions), analysis artifacts, and renderer contracts in SDD-000, SDD-001, SDD-002, SDD-004, and SDD-013. Verify: `task lint:md` exits 0.
- [ ] 2. Implement core commands, rational speed-ramp frame mapping, J/L-cut bounds, beat-sync montage rules, and golden cases in `core/timeline/`, `core/command/`, and `core/plan/`, then run `task codegen`. Verify: `go test ./core/...` passes.
- [ ] 3. Implement LGPL-clean 2D motion tracker, stabilization trajectory smoother, and auto-color analyzer in `engine/` (zero GPL libraries!), plus speed-ramp audio/video wiring in `engine/export/`. Verify: `go test ./engine/...` passes and `bash scripts/ffmpeg-verify.sh` confirms zero GPL dependencies.
- [ ] 4. Implement new transitions (`whip_pan`, `zoom_in`, `zoom_out`, `glitch`, `light_leak`, `slide_push`, `spin`) and Lift/Gamma/Gain color wheels in `renderer/`, rebuild WASM reproducibly, and prove browser-vs-export parity (`renderer-parity` skill). Verify: `cd renderer && cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` and `go test ./engine/export/... -run "Parity|Golden"` pass.
- [ ] 5. Expose new analysis jobs and commands across HTTP, CLI, and MCP (using existing `edit` batch tool so MCP `tools/list` budget holds). Verify: `go test ./service/... ./web/app/... ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 6. Build the Video Editor UI controls (Speed Ramp curve editor & presets, Stabilization controls, Motion Track box & pin picker, Transitions gallery & J/L-cut handles, Color Wheels + Auto-Color + live Scopes, Beat-Sync Montage action), add `en.json` + `pt-BR.json` strings, and add Playwright E2E specs. Verify: `task lint`, `task test`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the video editor Playwright specs pass.

## Test strategy

- Core golden tests: every new command has valid + error golden cases, undo/redo round trips, and deterministic frame-mapping tests for speed ramps across 24/30/60 fps.
- Analysis unit tests: synthetic translated/shaky frame sequence stabilizes within 1 px residual jitter; synthetic moving square is tracked accurately across 60 frames.
- Renderer parity tests: browser WASM vs `wazero` export golden frames for every new transition and Lift/Gamma/Gain color wheel.
- E2E (Playwright): apply a Speed Ramp preset, run Beat-Sync Montage, attach a text callout to a tracked motion path, open video scopes, and export MP4 cleanly.

## Acceptance criteria

1. Applying a **Speed Ramp** preset (`montage`, `hero`, `bullet`) or custom curve dynamically remaps video playback speed and audio cleanly with identical frame timing in preview and exported MP4.
2. **Video Stabilization** and **2D Motion Tracking** run with zero GPL dependencies (no `libvidstab`), smoothing shaky synthetic footage and pinning a text or blur overlay onto a moving object across preview and export.
3. All 7 new transitions (`whip_pan`, `zoom_in`, `zoom_out`, `glitch`, `light_leak`, `slide_push`, `spin`), J/L-cut audio split handles, 3-way Color Wheels, Auto-Color, live Video Scopes, and one-click Beat-Sync Montage work end to end.
4. Rust gate, `task lint`, `task test`, `task test:web`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and Playwright video-editor specs pass.

## Risks and mitigation

- **GPL licence contamination via FFmpeg `vidstab`.** Mitigation: strictly prohibited by ADR-0011; implement tracking and stabilization using our own MIT/LGPL-clean Go/Rust motion estimator + frame-plan transforms, verified by `cargo deny check` and licence checks.
- **Audio artifacts on extreme speed ramps.** Mitigation: clamp smooth `atempo` segments to `[0.5, 2.0]` cascades and crossfade segment boundaries cleanly in `engine/export/audio.go`.
- **STOP** if motion tracking or stabilization requires a new Go or Rust dependency not already approved in `go.mod` / `.agents/rules/rust.md`.

## Size

L

## Hand-off for review

The commits of `epic/115-video-editor-capcut-premiere-resolve-parity`, rebuilt deterministic WASM artifact + digest, Rust gate output, parity test results, `surfacebudget` output, Vitest/Playwright outputs, and screenshots of Speed Ramp, Motion Tracking, Color Wheels/Scopes, and Beat-Sync Montage.
