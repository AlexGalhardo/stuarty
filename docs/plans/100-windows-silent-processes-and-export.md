# 100: Windows silent child processes, FFmpeg shim resolution and reliable video export

## Goal

Opening Stuarty Desktop on Windows launches only the application window, with zero console or Windows Terminal windows popping up for the local server or for FFmpeg. Frame extraction and video export work reliably even when a user has a Scoop-installed FFmpeg shim on `PATH` or a hardware encoder that rejects a project's resolution or flags, falling back cleanly to the next verified encoder and reporting actionable diagnostics when an encode fails.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  1. `stuarty-desktop-0-60-0.exe` opens a visible Windows Terminal window to start the localhost server (`stuarty.exe serve`); the desktop shell must hide every background child process completely.
  2. Frame extraction on Windows opened a Windows Terminal window executing `C:\Users\aleex\scoop\shims\ffmpeg.exe ... -f rawvideo pipe:1` and failed with Win32 error `2147942632 (0x800700e8)` (`ERROR_NO_DATA`, pipe being closed).
  3. Exporting a video failed with `E_EXPORT_FAILED` (`The video encoder failed. Run "stuarty doctor"...`).
- Root causes verified in the codebase:
  - `desktop/shell/pdeath_other.go` defines `prepareCommand(*exec.Cmd)` as a no-op on Windows (`//go:build !linux && !darwin`), so `desktop/shell/process.go` spawns `stuarty.exe serve` without `SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}`. By contrast, `service/agenthost/tree_windows.go` already sets both flags.
  - `engine/ffmpeg/run.go` runs FFmpeg and ffprobe through `github.com/AlexGalhardo/vukem/contrib/engine`. On Windows, when `contrib/engine` spawns a console binary without `CREATE_NO_WINDOW`, Windows Terminal intercepts console allocation, and Scoop shims (`scoop\shims\ffmpeg.exe` + sibling `ffmpeg.shim`) wrap the real binary in a stub whose stdout/stdin pipe breaks with `0x800700e8` under a windowless GUI parent.
  - `engine/export/encode.go` tries only the single cached `Chosen` encoder from `DetectEncoder` (which probes a tiny synthetic frame), so a hardware encoder (`h264_nvenc`, `h264_amf`, `h264_qsv`, `h264_mf`) that passes the tiny probe but rejects a real project's geometry (e.g., portrait `720x1280` or `1080x1920`, odd dimensions, or driver session limits) fails the entire export with a generic `The video encoder failed.` instead of falling back to the next working candidate in `EncoderOrder`.
- ADR-0011 (LGPL FFmpeg as a separate process, encoder order, no GPL), ADR-0041 (Wails desktop shell), ADR-0047 (record any friction in `vukem` before working around it), SDD-004 §8, SDD-007 §6, SDD-011 §8.

## Scope

- Modules owned:
  - `desktop/shell/` (add `pdeath_windows.go`, restrict `pdeath_other.go` to `!linux && !darwin && !windows`, audit all `exec.Command` calls in `desktop/shell/` and `desktop/shell/app/`).
  - `engine/ffmpeg/` (`discover.go` Scoop/shim target resolution on Windows, `run.go` / `encoders.go` Windows silent process execution and per-geometry encoder probing/fallback; audit `vukem/contrib/engine` and record a `VF-NNN` if `contrib/engine` omits `CREATE_NO_WINDOW` on Windows).
  - `engine/export/` (`encode.go`, `codec.go`, `errors.go`: automatic fallback across verified candidates in `EncoderOrder` when a non-forced encoder fails on a real frame geometry, plus structured `details.encoder` and `details.encoder_reason`).
  - `engine/capturehost/` and `engine/aihost/` (audit Windows process creation flags so neither flashes a console window).
  - `docs/sdds/004-render-pipeline.md`, `docs/sdds/011-ingest-and-media.md`, `docs/sdds/012-ui-architecture.md`.
- Must not touch: `renderer/`, `web/resources/js/`, `core/timeline/`, `core/image/`.

## Out of scope

- Shipping or signing new FFmpeg release artifacts (owned by epics 46 and 98).
- Localizing export error strings in the frontend (owned by epic 113).
- Adding GPL encoders (`libx264` stays user-opt-in only per ADR-0011).

## Dependencies

- Epics 46 and 98 (own FFmpeg build and installer staging).

## Technical design

1. **Silent child processes on Windows (`desktop/shell/pdeath_windows.go`)**:
   - Split `prepareCommand(cmd *exec.Cmd)` for Windows into `desktop/shell/pdeath_windows.go` (`//go:build windows`), setting:

     ```go
     cmd.SysProcAttr = &syscall.SysProcAttr{
         HideWindow:    true,
         CreationFlags: windows.CREATE_NO_WINDOW,
     }
     ```

   - Audit every production `exec.Command` / `exec.CommandContext` across `desktop/shell/`, `engine/capturehost/`, `engine/aihost/`, and `web/cmd/app/open.go` (only interactive URL/folder openers keep normal flags; all background daemons/helpers/tools use `HideWindow: true` + `CREATE_NO_WINDOW`).
   - Check `github.com/AlexGalhardo/vukem/contrib/engine` source for Windows `CREATE_NO_WINDOW` / `HideWindow`. Per ADR-0047, if `contrib/engine` omits it, record a `VF-NNN` entry with exact file and line references from the vukem source and ensure FFmpeg/ffprobe invocations never spawn a visible console window on Windows.

2. **Scoop shim and symlink resolution (`engine/ffmpeg/discover.go`)**:
   - Keep discovery priority: `SourceConfigured` -> `SourceBundled` -> `SourcePath`.
   - On Windows, when `SourcePath` (or `SourceConfigured`) points to a Scoop shim (`<dir>\ffmpeg.exe` with a sibling `<dir>\ffmpeg.shim` containing `path = <target>`), parse the `path = ...` line strictly (no shell expansion, must be an absolute path to a regular `.exe` file, verified via `os.Stat` and `evalSymlinks` before `openTool`) and resolve both `ffmpeg.exe` and its sibling `ffprobe.exe` directly to the real binaries. Log `resolved_from_shim=true` at info level so `stuarty doctor` shows the real binary path.
   - Verify with a regression test using a fake `.shim` file that `Discover` resolves the underlying binary and passes `pipe:1` / `pipe:0` data cleanly without `0x800700e8`.

3. **Reliable video export encoder fallback and diagnostics (`engine/export/encode.go`, `engine/export/codec.go`)**:
   - In `DetectEncoder`, keep caching the ordered list of all candidates with `Status == EncoderOK` (not just the single `Chosen` one).
   - In `export.Run` / `encode`, when the caller did not force `STUARTY_ENCODER` and the primary encoder fails with an encoder initialization/runtime failure (before writing a valid output, e.g., `h264_mf` or a GPU encoder rejecting portrait `720x1280` or `-bf 2` or `-movflags` or exceeding hardware session slots), log the failure and automatically retry the encode with the next `EncoderOK` candidate in `EncoderOrder` (for example `h264_mf` -> `libopenh264` or `h264_nvenc` -> `h264_mf` -> `libopenh264`), removing the partial file between attempts.
   - Fix `h264_mf` options in `engine/export/encode.go`: Media Foundation (`h264_mf`) on Windows rejects `-bf 2` on many drivers and rejects `-colorspace`/`-bitexact` combinations on certain resolutions; align `encoderOpts("h264_mf", ...)` with what passes on Windows (e.g., `bframes = false` when needed) and validate with a real portrait (`720x1280`) and landscape (`1920x1080`) encode test when FFmpeg is available.
   - Enrich `E_EXPORT_FAILED` `Details` with `encoder` (name), `tried_encoders` (slice), and `encoder_reason` (scrubbed first stderr line, no local paths, per SDD-007 §8) so both `stuarty doctor` and the Export dialog's "Copy details" give an exact, diagnosable reason.

## Tasks

- [x] 1. Audit `vukem/contrib/engine` and all `exec.Command` call sites in `desktop/`, `engine/`, and `service/` on Windows; record any `vukem` friction as `VF-NNN` per ADR-0047 and update SDD-011 §8 and SDD-012 §21. Verify: `task lint:md` exits 0 and the audit table is recorded in the commit message / hand-off.
- [x] 2. Add `desktop/shell/pdeath_windows.go` setting `HideWindow: true` and `CREATE_NO_WINDOW` in `prepareCommand`, update `pdeath_other.go` build tags, and add a Windows unit test asserting `SysProcAttr` flags on `prepareCommand`. Verify: `go test ./desktop/shell/...` passes.
- [x] 3. Implement Scoop `.shim` resolution in `engine/ffmpeg/discover.go` (Windows) and ensure silent windowless FFmpeg/ffprobe/capturehost/aihost execution with unit and regression tests (including a fake `.shim` fixture and `pipe:1` rawvideo streaming test). Verify: `go test ./engine/ffmpeg/... ./engine/capturehost/... ./engine/aihost/...` passes.
- [x] 4. Implement ordered encoder fallback across `EncoderOK` candidates in `engine/export/`, fix `h264_mf` portrait/B-frame compatibility, and include scrubbed `details.encoder`, `details.tried_encoders`, and `details.encoder_reason` in `E_EXPORT_FAILED`. Verify: `go test ./engine/export/...` passes, including a fallback test where the first encoder fails and the second succeeds, and a portrait `720x1280` encode test.
- [x] 5. Verify desktop build (`-H windowsgui`) launches the server and runs FFmpeg frame extraction + MP4 export with zero console/terminal windows created. Verify: `go build ./desktop/shell/app` and `task lint && task test` pass.

## Test strategy

- Unit tests: `prepareCommand` Windows `SysProcAttr` flags; Scoop `.shim` parser (valid path, quoted path, missing file, relative path refused, directory traversal refused); `encoderOpts` for `h264_mf`; `E_EXPORT_FAILED` structured details scrubbing.
- Integration tests: fake encoder #1 fails on portrait `720x1280` and export automatically falls back to candidate #2 and produces a verified MP4; `RunFFmpeg` streaming rawvideo on `pipe:1` through a resolved binary without broken-pipe errors.
- Smoke test on Windows: build `stuarty-desktop.exe` with `-H windowsgui`, launch against a temporary workspace, extract a video frame and export a short vertical `720x1280` video, confirming no console window spawns and export succeeds.

## Acceptance criteria

1. On Windows, starting `stuarty-desktop.exe` (built with `-H windowsgui`) spawns `stuarty.exe serve` with `HideWindow: true` and `CREATE_NO_WINDOW`, and zero visible console or Windows Terminal windows appear during startup, frame extraction, or export.
2. When `PATH` points to a Scoop `ffmpeg.exe` shim with a valid sibling `ffmpeg.shim`, `ffmpeg.Discover` resolves the real target binary and `RunFFmpeg` streams `-f rawvideo pipe:1` without `0x800700e8` errors.
3. When the primary detected H.264 encoder fails during export (and `STUARTY_ENCODER` is not forced), `engine/export` automatically falls back to the next `EncoderOK` candidate in `EncoderOrder` and completes the export; if all fail, `E_EXPORT_FAILED` carries scrubbed `details.encoder`, `details.tried_encoders`, and `details.encoder_reason`.
4. `go test ./desktop/... ./engine/ffmpeg/... ./engine/export/... ./engine/capturehost/... ./engine/aihost/...` and `task lint` exit 0.

## Risks and mitigation

- **vukem `contrib/engine` owns `os/exec` for FFmpeg.** Mitigation: inspect `contrib/engine` first in task 1; if it lacks `CREATE_NO_WINDOW` on Windows, record `VF-NNN` per ADR-0047. **STOP** if fixing silent FFmpeg execution requires bumping `github.com/AlexGalhardo/vukem` in `go.mod` without the author's approval (AGENTS.md rule 9).
- **Scoop `.shim` path injection.** Mitigation: only read `.shim` files located in the exact directory returned by `exec.LookPath` / `STUARTY_FFMPEG`, accept only an absolute path to a regular file, and still verify version and LGPL licence via `openTool`.
- **Fallback hiding a forced encoder error.** Mitigation: when `STUARTY_ENCODER` is explicitly set (`Forced: true`), never fall back silently; fail immediately with that encoder's diagnostic.

## Size

M

## Hand-off for review

The commits of `epic/100-windows-silent-processes-and-export`, any `VF-NNN` recorded for `vukem`, the `go test` and `task lint` outputs, and the Windows smoke-test evidence confirming zero terminal popups and a passing portrait MP4 export.
