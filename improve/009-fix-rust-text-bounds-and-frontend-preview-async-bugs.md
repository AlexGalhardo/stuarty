# Plan 009: Fix Rust text-rasterizer float bounds and frontend preview, Web Lock, SSE, and recorder leaks

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- renderer/stuarty-renderer-text/src/text.rs renderer/stuarty-renderer-text/src/scan.rs renderer/stuarty-renderer-text/src/shape.rs web/resources/js/editor/preview/PixiExecutor.ts web/resources/js/editor/preview/RustExecutor.ts web/resources/js/editor/preview/render/client.ts web/resources/js/editor/preview/decode/client.ts web/resources/js/editor/preview/decode/decodeWorker.ts web/resources/js/editor/api/eventsLock.ts web/resources/js/editor/api/http.ts web/resources/js/editor/record/browserRecorder.ts web/resources/js/editor/record/useRecordFlow.ts web/resources/js/pages/Write/TextEditor.tsx web/resources/js/dashboard/approval/useApprovalQueue.ts web/resources/js/dashboard/chat/useChat.ts`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

This plan fixes verified correctness, hang, and resource-leak bugs in the Rust text rasterizer and the React/TypeScript frontend:

1. **Rust text/shape float bounds & scanline hang (`renderer/stuarty-renderer-text/`)**: `validate()` accepts `f64::INFINITY` and arbitrary blur/offset floats, allowing `3 * blur` integer overflow/wrap-around (bypassing `MAX_PIXELS`) and multi-billion-iteration negative-`y` loops in `scan.rs`.
2. **Preview freezes, fallback hangs & GPU/decoder leaks**:
   - `PixiExecutor.videoTexture()` caches `<video>` textures without checking `texture.source.resource !== video`, permanently freezing playback after `MediaPool` destroys idle `<video>` elements after 5 seconds; `this.stills` grows unbounded, `loadBitmap()` leaks bitmaps after `dispose()`, and `dispose()` leaks `entry.lut`.
   - `RenderClient` and `DecodeClient` never attach `worker.onerror`/`onmessageerror`, hanging `ExecutorSwitch` fallback indefinitely if a worker fails to load/crashes.
   - `DecodeClient.sweep()` drops `"opening"` assets while `decodeWorker` finishes `openAsset()`, leaking hardware `VideoDecoder`s.
   - `RustExecutor.upload()` lacks per-item generation guards, letting slower `createImageBitmap` calls overwrite newer frames or re-upload released items.
3. **Web Lock, hardware camera/mic, and React async races**:
   - `eventsLock.ts` can run `becomeOwner()` after `close()`, permanently holding the cross-tab Web Lock and leaking an `EventSource`.
   - `browserRecorder.ts` + `useRecordFlow.ts` leak live camera/screen `MediaStream` tracks (hardware LED stays on), `AudioContext`, and server recordings on `MediaRecorder` startup failure or unmount.
   - `Write/TextEditor.tsx`, `useApprovalQueue.ts`, `useChat.ts`, and `HttpTransport.send` have stale-response overwrite, zombie-approval resurrection, and abort-listener leak bugs.

## Current state

- `renderer/stuarty-renderer-text/src/text.rs:107-116, 524-529`: `validate()` checks `st.size > 0.0 && st.line_height > 0.0 && st.max_width > 0.0` (all true for `INFINITY`) and does not validate `shadow_blur`/`shadow_dx`/`shadow_dy`/`stroke_width`/`background_padding`/`background_radius`; line 528 computes `(3 * blur) as f64` without overflow check.
- `renderer/stuarty-renderer-text/src/scan.rs:78-90`: `while y < y_max` starts at `ay.floor() as i32` and loops one row at a time even while `y < 0`.
- `web/resources/js/editor/preview/PixiExecutor.ts:1519-1534`: `videoTexture()` caches `this.stills.get(key)` without verifying `texture.source.resource === video`.
- `web/resources/js/editor/preview/RustExecutor.ts:803-829`: `upload()` deduplicates by `tex:${item}:${tag}` and unconditionally calls `this.render.texture(item, bitmap, ...)` + `this.uploaded.set(item, tag)` when the promise resolves.
- `web/resources/js/editor/preview/render/client.ts:51-57`: attaches only `worker.onmessage`, never `worker.onerror` or `worker.onmessageerror`.
- `web/resources/js/editor/api/eventsLock.ts:126-141`: `close()` calls `pending?.abort()`, which per Web Locks spec only aborts *waiting* requests — an already-granted callback still executes `becomeOwner()` after `channel = null`.
- `web/resources/js/editor/record/browserRecorder.ts:159-221`: `mixAudio`, `new MediaRecorder`, `setInterval`, and `.start()` have no `try/catch` rollback to stop `display`/`micStream`/`cameraStream` and close `AudioContext` on error.
- `web/resources/js/pages/Write/TextEditor.tsx:64-83`: `load()` awaits `client.get(projectId, itemId)` with no request-generation guard.
- `web/resources/js/dashboard/approval/useApprovalQueue.ts:47-65, 97-105`: `mergeList` appends all items from `listApprovals` not currently in `was`, resurrecting items already removed by SSE/decide before the initial fetch completed.
- `web/resources/js/editor/api/http.ts:28-37, 79-81`: `send()` attaches `{ once: true }` abort listener without checking `request.signal?.aborted` first and never calls `removeEventListener` in `finally`.
- Conventions: Biome tabs + double quotes + semicolons (`cd web && bun run lint`), Vitest unit tests colocated as `*.test.ts(x)`. In Rust, `unsafe_code = "deny"`, `clippy::all = "deny"`, no floats changing CPU pixel parity (bounds validation rejects invalid specs only).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Rust renderer text tests | `cd renderer && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test -p stuarty-renderer-text` | `ok`, exit 0 |
| Web typecheck & lint | `cd web && bun run typecheck && bun run lint && bun run check:thin-client` | exit 0 |
| Web unit tests | `cd web && bun run test` | all pass, exit 0 |

## Scope

**In scope** (the only files you should modify):

- `renderer/stuarty-renderer-text/src/text.rs`
- `renderer/stuarty-renderer-text/src/scan.rs`
- `renderer/stuarty-renderer-text/src/shape.rs`
- `web/resources/js/editor/preview/PixiExecutor.ts` (+ `PixiExecutor.test.ts` if present, or colocated test)
- `web/resources/js/editor/preview/RustExecutor.ts` (+ colocated test)
- `web/resources/js/editor/preview/render/client.ts` (+ `client.test.ts`)
- `web/resources/js/editor/preview/decode/client.ts` (+ `decodeCore.test.ts` / `client.test.ts`)
- `web/resources/js/editor/preview/decode/decodeWorker.ts`
- `web/resources/js/editor/api/eventsLock.ts` (+ `eventsLock.test.ts`)
- `web/resources/js/editor/api/http.ts` (+ `http.test.ts`)
- `web/resources/js/editor/record/browserRecorder.ts` (+ `browserRecorder.test.ts`)
- `web/resources/js/editor/record/useRecordFlow.ts`
- `web/resources/js/pages/Write/TextEditor.tsx` (+ colocated test)
- `web/resources/js/dashboard/approval/useApprovalQueue.ts` (+ `useApprovalQueue.test.ts`)
- `web/resources/js/dashboard/chat/useChat.ts` (+ `useChat.test.ts`)

**Out of scope** (do NOT touch):

- Rebuilding/re-pinning the committed `.wasm` digest if Rust changes stay inside `stuarty-renderer-text`? STOP: check whether `scripts/build-renderer.sh` embeds `stuarty-renderer-text` into the committed WASM module! If YES, see STOP conditions — either run `bash scripts/build-renderer.sh` or split the Rust step into its own commit when the Rust toolchain is present.
- Whole-store Zustand refactors in `useChat` / `useInbox` (handled in Plan 011).

## Git workflow

- Branch: `fix/renderer-text-and-frontend-bugs` cut from `release/v0.60.0`.
- Commit in logical groups:
  1. `fix(renderer): validate float finiteness and clamp negative scanlines in text rasterizer`
  2. `fix(preview): rebind PixiExecutor video textures, guard RustExecutor uploads, and handle worker errors`
  3. `fix(ui): guard Web Lock, browserRecorder cleanup, and async fetch races`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Validate float finiteness/bounds and clamp scanlines in `renderer/stuarty-renderer-text/`

1. In `renderer/stuarty-renderer-text/src/text.rs`:
   - In `validate()`: require `.is_finite()` on `pixel_scale`, `size`, `line_height`, `max_width`, `shadow_blur`, `shadow_dx`, `shadow_dy`, `stroke_width`, `background_padding`, `background_radius`, and any other numeric style fields; require non-negative values (`>= 0.0`) on `shadow_blur`, `stroke_width`, `background_padding`, `background_radius`; cap `shadow_blur` (e.g., `<= 256.0`) and `size`/`max_width` to sane upper bounds so `3 * blur` can never overflow `usize`.
   - At line 528, use `blur.saturating_mul(3)` as defense-in-depth.
2. In `renderer/stuarty-renderer-text/src/shape.rs`: require `.is_finite()` and non-negative bounds on shape dimensions, radii, and `stroke_width`.
3. In `renderer/stuarty-renderer-text/src/scan.rs:78-90`:
   - If `y < 0`, skip straight to `y = 0` analytically: if `by <= 0.0`, return early; otherwise compute `x = ax + (0.0 - ay) * dxdy` and set `y = 0` before entering `while y < y_max`! (Verify bit-exactness on existing golden/unit tests when `ay < 0.0`.)
4. Add Rust unit tests in `text.rs`, `shape.rs`, and `scan.rs` for `f64::INFINITY`, `f64::NAN`, huge `shadow_blur` (`1e18`), and `ay = -1e7` completing instantly without panic or hang.

**Verify**: `cd renderer && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` -> `PASS`, exit 0.

### Step 2: Fix preview executor & worker bugs (`PixiExecutor`, `RustExecutor`, `RenderClient`, `DecodeClient`/`decodeWorker`)

1. **`PixiExecutor.ts`**:
   - In `videoTexture()` (lines 1519–1534): if `texture !== undefined && texture.source.resource !== video`, destroy the stale texture/source (`texture.destroy(true)`), delete from `this.stills`, and recreate for the live `video`.
   - Bound `this.stills` (e.g., LRU eviction above 128 entries destroying evicted textures), call `bitmap.close()` in `loadBitmap()` when `this.disposed` became true, and call `entry.lut?.destroy()` in `dispose()`.
2. **`RustExecutor.ts`** (lines 803–829):
   - Track a per-item latest desired tag map (`this.wantedTag: Map<string, string>`) updated before `make()` and cleared on `render.release(...)`.
   - After `make()` resolves, if `this.disposed || this.render === null || this.wantedTag.get(item) !== tag`, call `bitmap.close()` and return without calling `this.render.texture(...)` or mutating `this.uploaded`.
3. **`render/client.ts` & `decode/client.ts` & `decode/decodeWorker.ts`**:
   - In `RenderClient` constructor: attach `worker.onerror` and `worker.onmessageerror` handlers that invoke `this.failed?.(new RenderError(...))`, reject any pending frame promises, and mark disposed.
   - In `DecodeClient.ensureWorker()`: attach `worker.onerror` and `worker.onmessageerror` handlers that transition all tracked assets to `"fallback"` and trigger `onReady` so preview seamlessly falls back to `<video>`.
   - In `DecodeClient.sweep()`: do not sweep entries with `status === "opening"`, OR in `decodeWorker.ts`, track a `dropped` set / generation counter checked after every `await` in `openAsset()` so a dropped asset closes its `AssetDecoder` instead of leaking it in `assets`. (Implementing BOTH is cleanest.)

**Verify**: `cd web && bun run typecheck && bun run lint && bun run test` -> all pass, exit 0.

### Step 3: Fix `eventsLock.ts`, `browserRecorder.ts`/`useRecordFlow.ts`, `TextEditor.tsx`, `useApprovalQueue.ts`, `useChat.ts`, and `http.ts`

1. **`web/resources/js/editor/api/eventsLock.ts`**: track `let gen = 0;` incremented on `open()` and `close()`. Capture `const myGen = gen;` before `deps.locks.request(...)`; inside the lock callback, if `myGen !== gen || channel === null`, return immediately so the lock releases cleanly without opening `stream`.
2. **`web/resources/js/editor/record/browserRecorder.ts` & `useRecordFlow.ts`**:
   - In `startBrowserRecording`: wrap everything after `beginRecordingUpload` (`mixAudio`, `make("screen", ...)`, `make("camera", ...)`, `setInterval`, `recorder.start(...)`) in `try/catch` that clears `timer`, stops any started `recorders`, calls `stopAll(display, micStream, cameraStream)`, calls `mix.close()`, and calls `client.cancelRecording(begun.recording_id).catch(() => undefined)` before rethrowing.
   - In `useRecordFlow.ts:75-84`: in the unmount cleanup, if `browser.current !== null`, stop/halt it cleanly.
3. **`web/resources/js/pages/Write/TextEditor.tsx:64-83`**: track `loadSeqRef = useRef(0)`; increment on each `load()` (and in `useEffect` cleanup); ignore resolved/rejected results when `seq !== loadSeqRef.current`.
4. **`web/resources/js/dashboard/approval/useApprovalQueue.ts:47-70`**: track a `decided = new Set<string>()` inside the effect (and ref for `decide()` calls); record every approval ID removed/decided via SSE or `decide()`, and exclude `decided` IDs from the initial `listApprovals` response before calling `mergeList` (mirroring `web/resources/js/shell/useInbox.ts:42-51`).
5. **`web/resources/js/dashboard/chat/useChat.ts:157-199, 315-326`**: track refs for `active`, `projectId`, and a session-open generation counter; check them after every `await` in `open()`, `refreshSession()`, and `start()` before calling `setSession`, `setEvents`, or opening an `EventSource`.
6. **`web/resources/js/editor/api/http.ts:28-37, 79-81`**: define `const onAbort = () => controller.abort();`; if `request.signal?.aborted`, call `controller.abort()` immediately, else attach `addEventListener("abort", onAbort, { once: true })`; in `finally`, call `request.signal?.removeEventListener("abort", onAbort)`.

**Verify**: `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` -> all pass, exit 0.

## Test plan

- Rust (`renderer/stuarty-renderer-text`): unit tests for non-finite floats, huge `shadow_blur`, and large negative `ay` scanline coordinate.
- Vitest (`web/`):
  - `PixiExecutor`: recreated `<video>` element rebinds texture; `dispose()` destroys LUT and late `loadBitmap()` closes bitmap.
  - `RustExecutor`: out-of-order `upload()` closes stale `ImageBitmap` without overwriting newer tag.
  - `RenderClient` & `DecodeClient`: `worker.onerror` rejects `init()` / transitions assets to `"fallback"`; `decodeWorker` drop during `openAsset` closes decoder.
  - `eventsLock.test.ts`: `close()` before granted lock microtask does NOT call `stream.open` and releases the lock immediately.
  - `browserRecorder.test.ts`: `MediaRecorder` constructor throw stops all stream tracks, closes `AudioContext`, and cancels server recording.
  - `useApprovalQueue.test.ts`: SSE removal arriving before initial `listApprovals` resolves does NOT resurrect the decided approval.
  - `http.test.ts`: pre-aborted signal aborts immediately; completed request removes its abort listener.

## Done criteria

- [ ] `cd renderer && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test` exits 0 *(Step 1 deferred per STOP condition: `cargo` not installed on executor host)*
- [x] `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` exits 0
- [x] Every new regression test listed in Test plan exists and passes *(frontend Steps 2-3; Rust Step 1 pending `cargo`)*
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `cargo` is not installed on the executor machine — complete Steps 2 & 3 (TypeScript/React) and explicitly report Step 1 (Rust) as skipped/pending a Rust-enabled runner, OR stop before Step 1 if a single atomic PR is required.
- Modifying `renderer/stuarty-renderer-text` changes the embedded WASM renderer module bytes — check `.agents/rules/rust.md` item 9 and run `bash scripts/build-renderer.sh` only if the pinned digest must be updated (and verify renderer parity tests pass).
- Changing `scan.rs` analytic `y < 0` jump shifts any existing text golden by even 1 subpixel — verify rounding (`(0.0 - ay) * dxdy` vs iterative `x_next`) against `stuarty-renderer-text` golden tests and stop if any golden differs.

## Maintenance notes

- Follow `web/resources/js/shell/useInbox.ts:42-51` whenever pairing an initial REST snapshot with a live SSE stream: always track IDs seen/removed over SSE before the initial snapshot resolves.
- Reviewer focus: verify `browserRecorder.ts` tests mock `MediaRecorder` throwing on the second track (`camera`) and assert `display`, `micStream`, and `cameraStream` tracks all have `.stop()` invoked.
