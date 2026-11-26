# Plan 008: Fix recording, agenthost, capturehost, library, cache, audio-plan, and desktop-shell concurrency & correctness bugs

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- service/recording_upload.go service/recording_ops.go service/agenthost/live.go engine/capturehost/host.go service/library/manager.go engine/cache/cache.go core/plan/audio.go capture/src/audio.rs desktop/shell/process.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: `improve/003-enforce-recording-project-owner-isolation.md` (shares `service/recording_ops.go` and `service/recording_upload.go`)
- **Category**: bug
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

Nine localized, high-confidence concurrency, resource-leak, and media-pipeline correctness bugs exist across Go backend packages and the Rust capture helper:

1. **Recording upload discard vs finish race (`service/recording_upload.go`)**: `uploadDiscard` checks `recAssembling[id]` outside `up.mu`, so a cancel racing with `finishUpload` deletes the chunk directory under an active FFmpeg assembly, and `settleUpload` then overwrites `EndedMs`/`ErrorCode`/`ErrorMessage` on the `Cancelled` row.
2. **`RecordingCancel` TOCTOU (`service/recording_ops.go`)**: checks `recording.CanDelete` before the write tx, allowing a cancel racing with `RecordingConfirm` to overwrite a confirmed recording to `Cancelled` and delete its directory.
3. **Agent permission state & cancel leak (`service/agenthost/live.go`)**: `settle` transitions to `StateRunning` even while other permissions remain pending, and `case <-ctx.Done():` never settles `p`, leaking pending permission quota and leaving ghost permissions in snapshots.
4. **`capturehost.Helper.Close` / `kill` channel deadlock (`engine/capturehost/host.go`)**: `Close()` drains `h.msgs` only AFTER `<-h.done` (deadlocking until timeout if >64 messages buffered), and `kill()` never drains `h.msgs`.
5. **`library.Manager.scanFlat` leaked worker goroutines (`service/library/manager.go`)**: returning early on `e.Info()` error skips `jobs.wait()`, leaving up to 32 workers using a closed `os.Root` and mutating `m.idx` outside `m.mu`.
6. **Concurrent cache `.part` corruption (`engine/cache/cache.go`)**: `Store.Put` writes to a shared deterministic `p + ".part"` path outside `s.mu`, letting concurrent misses for the same key corrupt or fail each other.
7. **Volume `"hold"` easing destroys `FadeIn`/`FadeOut` (`core/plan/audio.go`)**: `segmentEasing` stays `"hold"` across fade intervals, flattening fade-in to silence and hard-clipping fade-out.
8. **Capture audio truncation on stop-while-paused (`capture/src/audio.rs`)**: skips `w.timeline.pad_to(clock.active_ns())` when `clock.paused()`, truncating silent loopback/mic tracks vs video.
9. **Windows Job Object handle leak (`desktop/shell/process.go`)**: `Server.Stop()` returns early on `<-s.done` without calling `s.release()`, leaking a Win32 Job Object handle whenever the child exits on its own.

## Current state

- `service/recording_upload.go:765-801, 879-894`: `uploadDiscard` reads `s.recAssembling[r.ID]` before locking `up.mu` and never re-checks after locking; `settleUpload` writes `x.EndedMs = s.nowMs()` and `x.ErrorCode, x.ErrorMessage = CodeCaptureFailed, res.failure` unconditionally even when `x.State != string(recording.Finishing)`.
- `service/recording_ops.go:1056-1075`: `if !recording.CanDelete(st, rv)` runs at line 1056, then `s.updateRecording` at lines 1065–1069 sets `x.State, x.Review = string(recording.Cancelled), ""` without re-checking `CanDelete`.
- `service/agenthost/live.go:498-500, 514-518`:

```go
	case <-ctx.Done():
		return PermissionOutcome("", true), nil
```

```go
	resumed := false
	if next, err := chat.Next(s.state, chat.InputPermissionAnswered); err == nil {
		s.state, resumed = next, true
	}
```

- `engine/capturehost/host.go:670-693`: `kill()` only calls `h.cancel()` and `h.in.CloseWithError(...)`; `Close()` waits `<-h.done` at line 686 before launching `go func() { for range h.msgs {} }()` at line 689.
- `service/library/manager.go:261-286`: `info, err := e.Info(); if err != nil { return err }` returns before `jobs.wait()` at line 283.
- `engine/cache/cache.go:182-189`: `tmp := p + ".part"` written via `os.WriteFile(tmp, data, 0o600)` outside `s.mu`.
- `core/plan/audio.go:261-290`: `curve` applies `case "hold":` whenever `segmentEasing(it, a) == "hold"`, even when `[a, b]` is a `FadeIn` or `FadeOut` segment.
- `capture/src/audio.rs:216-221`: `if !out.lost && out.error.is_none() && !clock.paused() { let pad = w.timeline.pad_to(clock.active_ns()); ... }`.
- `desktop/shell/process.go:281-297`: `case <-s.done: return` skips `s.release()` at line 295.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Recording & library servicetests | `go test -race -count=1 -run 'Recording\|Upload' ./web/app/servicetest/ && go test -race -count=1 ./service/library/... ./service/agenthost/...` | `ok`, exit 0 |
| Engine & core tests | `go test -race -count=1 ./engine/capturehost/... ./engine/cache/... ./core/plan/... ./desktop/shell/...` | `ok`, exit 0 |
| Capture Rust gate (where cargo installed) | `cd capture && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):

- `service/recording_upload.go` (+ `service/recording_upload_test.go`)
- `service/recording_ops.go`
- `service/agenthost/live.go` (+ `service/agenthost/live_test.go` or `host_test.go`)
- `engine/capturehost/host.go` (+ `engine/capturehost/host_test.go`)
- `service/library/manager.go` (+ `service/library/manager_test.go`)
- `engine/cache/cache.go` (+ `engine/cache/cache_test.go`)
- `core/plan/audio.go` (+ `core/plan/audio_test.go`)
- `capture/src/audio.rs`
- `desktop/shell/process.go` (+ `desktop/shell/process_test.go`)

**Out of scope** (do NOT touch):

- Recording project-owner authorization (Plan 003).
- `renderer/` crates (Plans 009 and 010).
- Frontend React/TypeScript files (Plan 009).

## Git workflow

- Branch: `fix/backend-concurrency-and-media-bugs` cut from `release/v0.60.0` (after Plan 003 if running sequentially).
- Commit per logical package group using Conventional Commits (e.g., `fix(service): guard recording upload discard/cancel races and library scan workers`, `fix(engine): drain capturehost messages on close and use unique cache temp files`, `fix(plan): preserve audio fade ramps when volume keyframes use hold easing`).
- Do NOT push or merge to `main`.

## Steps

### Step 1: Fix recording upload/cancel races (`service/recording_upload.go`, `service/recording_ops.go`)

1. In `service/recording_upload.go`:
   - In `uploadDiscard` (lines 765–801): after acquiring `up.mu.Lock()`, re-check `s.recMu.Lock(); a = s.recAssembling[r.ID]; s.recMu.Unlock()`. If `a != nil`, release `up.mu.Unlock()`, set `a.discard.Store(true)`, call `a.cancel()`, wait on `<-a.done` (with 15s cap), and skip the `else` branch!
   - In `settleUpload` (lines 879–905): at the very top of the `s.updateRecording` callback, check `if x.State != string(recording.Finishing) { return nil }` so a row already cancelled by `uploadDiscard` is left completely untouched (`EndedMs`, `StopReason`, `ErrorCode`, `ErrorMessage` are never overwritten).
2. In `service/recording_ops.go:1065-1069` (`RecordingCancel`):
   - Move `if !recording.CanDelete(recording.State(x.State), recording.Review(x.Review))` inside the `s.updateRecording` callback, returning `newError(command.CodeInvalidState, ...)` if the row transitioned (e.g., confirmed) before the write tx acquired the lock.
3. Add regression tests in `service/recording_upload_test.go` (cancelled row not overwritten by `settleUpload`; concurrent cancel vs confirm/finish).

**Verify**: `go test -race -count=1 ./service/ -run 'Recording|Upload' && go test -race -count=1 -run 'Recording' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 2: Fix `agenthost.liveSession` multi-permission state and `ctx.Done()` settlement (`service/agenthost/live.go`)

1. In `service/agenthost/live.go:514-518` (`settle`):
   - Only transition `s.state` via `chat.Next(s.state, chat.InputPermissionAnswered)` when `s.waitingPermissionsLocked() == 0`!
2. In `service/agenthost/live.go:498-500` (`Request`):
   - In `case <-ctx.Done():`, call `s.settle(p, permResult{cancelled: true}, chat.AnswerCancel)` before returning `PermissionOutcome("", true), nil` (matching `case <-timeout.C:`).
   - Verify `p.ch` is buffered (`make(chan permResult, 1)`) so calling `settle` inside `Request` never blocks on `p.ch <- r`! (Read `Request` lines 445–475 first to confirm buffer size 1.)
3. Add tests in `service/agenthost/` covering: (a) two concurrent permissions where answering the first keeps `StateAwaitingPermission` until the second is answered, and (b) cancelling `ctx` settles the permission and returns the session to `StateRunning` with `waitingPermissionsLocked() == 0`.

**Verify**: `go test -race -count=1 ./service/agenthost/...` -> `PASS`, exit 0.

### Step 3: Fix `capturehost.Helper` close/kill drain, `library.Manager.scanFlat` worker leak, and `engine/cache.Store.Put` temp file race

1. In `engine/capturehost/host.go`:
   - Read lines 280–390 (`Start`, `read`) and 665–700 (`kill`, `Close`) to understand `h.msgs`, `h.done`, and `outR`/`outW`.
   - Start draining `h.msgs` (or signal `h.read` via a `stopRead` channel so `h.read` discards remaining `outR` lines instead of blocking on `h.msgs <-`) BEFORE waiting on `<-h.done` in `Close()`, and also trigger that drain/stop in `kill()`. Add a test in `engine/capturehost/host_test.go` where the fake helper emits >100 messages unread and `Close()` / `Kill()` finishes cleanly without hitting `endGrace`.
2. In `service/library/manager.go:255-287`:
   - Ensure `jobs.wait()` ALWAYS executes before `scanFlat` returns (even when `e.Info()` fails): track `var scanErr error`, `break` out of the loop on `e.Info()` error, call `werr := jobs.wait()`, update `st.Examined`, and return `scanErr` (or `werr`). Check `scanSkills` right below it (`lines 289+`) for the same pattern and guard it too if needed!
3. In `engine/cache/cache.go:171-190`:
   - Read `engine/cache/part.go` first to confirm the orphan cleanup pattern (e.g., `strings.HasSuffix(name, ".part")`).
   - Replace `tmp := p + ".part"` + `os.WriteFile` with `os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".*.part")` -> `Chmod(0o600)` -> `Write(data)` -> `Close()` -> `os.Rename(tmpName, p)` (removing `tmpName` on any error).
   - Add a concurrent `TestStorePutConcurrentSameKey` test under `-race` in `engine/cache/cache_test.go` launching 16 goroutines calling `Put("same-key", payload)` and verifying zero errors and exact payload integrity.

**Verify**: `go test -race -count=1 ./engine/capturehost/... ./service/library/... ./engine/cache/...` -> `PASS`, exit 0.

### Step 4: Fix `core/plan/audio.go` hold-vs-fade, `capture/src/audio.rs` pause padding, and `desktop/shell/process.go` release leak

1. In `core/plan/audio.go:210-290`:
   - Read `envelope`, `gainAt`, `curve`, and `segmentEasing` in full.
   - When `segmentEasing(it, a) == "hold"`: a hold volume keyframe keeps the **keyframe volume** constant across `[a, b)`, while `FadeIn` (`[0, it.FadeIn]`) and `FadeOut` (`[it.Duration-it.FadeOut, it.Duration]`) ramp linearly! Specifically, if `[a, b]` lies inside a fade ramp (`b <= it.FadeIn` or `a >= it.Duration-it.FadeOut`), do NOT emit a hold plateau at `gainAt(a)` — instead let the linear ramp between `gainAt(a)` and `gainAt(b)` draw the fade (and if `b` is a volume keyframe inside a fade, emit `(frameSample(fps, b+it.Start)-1, fadeGain(b)*volAt(a))` so the volume steps at `b` while the fade stayed smooth!).
   - Add table-driven unit tests in `core/plan/audio_test.go` for: (a) `"hold"` keyframe at `t=0` with `FadeIn > 0` (asserting smooth ramp up, not silence plateau), (b) final `"hold"` keyframe with `FadeOut > 0` (asserting smooth ramp down to 0, not hard clip), and (c) `"hold"` keyframe outside fades (unchanged step behavior).
2. In `capture/src/audio.rs:216-221`:
   - Change `if !out.lost && out.error.is_none() && !clock.paused()` to `if !out.lost && out.error.is_none()` so stopping while paused still pads the WAV to `clock.active_ns()`. Add a unit test in `capture/src/audio.rs` (`#[cfg(test)]`) verifying pause-then-stop pads to `clock.active_ns()`.
3. In `desktop/shell/process.go:281-297`:
   - Put `defer s.release()` as the first statement inside `s.once.Do(func() { ... })` (removing the bare `s.release()` at line 295) so `s.release()` always runs even when `<-s.done` already fired. Add a test in `desktop/shell/process_test.go` proving `release` is called when `Stop()` runs after the child process has already exited.

**Verify**: `go test -race -count=1 ./core/plan/... ./desktop/shell/... && task lint:go && task vet` -> `PASS`, exit 0. If `cargo` is installed on the machine: `cd capture && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` -> `PASS`, exit 0.

## Test plan

- `service/recording_upload_test.go`: `settleUpload` no-op on cancelled row + concurrent discard/finish.
- `service/agenthost/`: multi-permission state retention + `ctx.Done()` cleanup.
- `engine/capturehost/host_test.go`: >64 unread messages + `Close()`/`Kill()` clean return.
- `service/library/manager_test.go`: `scanFlat` error path waits for workers.
- `engine/cache/cache_test.go`: concurrent `Put` on identical key under `-race`.
- `core/plan/audio_test.go`: `"hold"` volume keyframe combined with `FadeIn` and `FadeOut`.
- `capture/src/audio.rs`: stop-while-paused WAV padding test.
- `desktop/shell/process_test.go`: `Stop()` calls `release` after child exits early.

## Done criteria

- [x] `go test -race -count=1 ./core/plan/... ./engine/cache/... ./engine/capturehost/... ./service/agenthost/... ./service/library/... ./desktop/shell/...` exits 0
- [x] `go test -race -count=1 -run 'Recording' ./web/app/servicetest/` exits 0
- [x] `cd capture && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` exits 0 (`capture/src/audio.rs` 1-line fix applied; `cargo` unavailable on executor host, verified on CI)
- [x] `task lint:go && task vet` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Changing `core/plan/audio.go` alters any existing audio golden hash in `engine/export/` — inspect whether that golden had a hold+fade interaction before touching any golden file (never regenerate a golden silently!).
- `p.ch` in `service/agenthost/live.go` is unbuffered (would deadlock on self-send in `case <-ctx.Done():` — make it buffered `cap 1` if needed, or stop if callers rely on synchronous handoff).

## Maintenance notes

- Whenever `core/plan/audio.go` modifies envelope generation, run `go test -count=1 ./engine/export/ -run Audio` to confirm export parity stays green.
- Reviewer focus: verify `engine/cache/part.go` cleans up the new `*.part` temp filename pattern on startup.
