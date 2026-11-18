# Plan 011: Finish the Rust renderer (Phase 5) and make it the only renderer

This plan answers what is left to finish Phase 5 (ADR-0029, SDD-013) and in what order: prove on Linux and macOS what is already proven on Windows, bring the browser preview onto the Rust renderer over WebGPU, measure on an idle machine, then make the Rust renderer the default and delete the Go compositor and the PixiJS executor. When it is delivered, one renderer draws every pixel of preview and export, and Phase 5's exit criteria hold.

- Owner: senior-b (`renderer/`, `engine/`), senior-e (`web/resources/`, `web/e2e/`), senior-d (CI workflows), orchestrator (measurements on the idle reference machine, author gates)
- Effort: L
- Depends on: Linux and macOS runners (GitHub Actions); a WebGPU adapter in the browser test runs; the idle reference machine for every measured budget
- Origin: Phase 5 epics EPIC-40 (P5-UI-001): TASK-0192, TASK-0193, TASK-0194; EPIC-42 (P5-UI-002): TASK-0203; EPIC-43 (P5-RND-003): TASK-0207; EPIC-44 (P5-RND-004): TASK-0211; EPIC-45 (P5-RND-005): TASK-0214; EPIC-46 (P5-UI-003): TASK-0215, TASK-0216, TASK-0217, TASK-0218; EPIC-47 (P5-RND-006): TASK-0221; EPIC-48 (P5-RND-007): TASK-0224; EPIC-49 (P5-RND-008): TASK-0225, TASK-0226, TASK-0227. EPIC-38, 39, 41 and 50 are complete.
- Status: TODO (steps 1-4 can start now; step 5 needs the author's go for the CSP change; steps 7-8 need the idle machine)
- Written against: `8787714a`

## 1. Why

The author confirmed on 2026-11-18 that Phase 5 continues to completion. Most of the code exists and is proven on Windows: the CPU path is byte-identical to the Go compositor (native and WebAssembly), the export runs it through `wazero` behind `VUKEM_VIDEO_EXPORT_RENDERER`, the GPU path matches the CPU path within SSIM 0.9969, text rasterization moved to Rust, WebCodecs decodes in Chromium and Firefox. What is missing is evidence on the other systems and browsers, the browser preview on the Rust renderer, the measured budgets, and the switch that removes the duplicates. Until the switch, two compositors and two preview executors must be kept in parity, which costs every render change twice.

## 2. Current state (drift check)

```text
git grep -n "VUKEM_VIDEO_EXPORT_RENDERER" engine/ service/ | head -3     # go | rust | gpu, default go
ls renderer                                                              # vukem-renderer, vukem-renderer-gpu, vukem-renderer-text
ls engine/compositor engine/renderhost engine/gpuhost                    # all three present
grep -n '"pixi.js"' web/package.json                                     # 8.23.0, still a dependency
cd renderer && cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check   # green on the author's Windows machine
```

What each epic already proved (from the closed parts of the cards, all 2026-11-16):

| Area | Proven | Missing |
| --- | --- | --- |
| CPU path vs Go compositor (EPIC-43) | 0 differing bytes on 33 goldens and 3 scenes, native and wazero, on Windows (`7a2aa9f9`, `b7c2ba48`); fuzz 1.8M runs without panic (`e3f003a9`) | the same on Linux and macOS |
| Export through wazero (EPIC-44) | module embedded and digest-checked (`bc8bdbb5`), switch (`39ddee6f`), threat line T-36 (`8b4a96b6`), export goldens equal on Windows | Linux, macOS; export speed within 20% of Go and NFR-007 memory on an idle machine (`BenchmarkExport60s`) |
| GPU path (EPIC-45) | 44 vectors, worst SSIM 0.99693 (RTX 4060 Vulkan) and 0.99694 (D3D12 software), offset 0 | Metal, a Linux adapter, WebGPU in the browser |
| Text in Rust (EPIC-47) | 56 vectors byte-identical native and wasm; Go rasterizer removed; fonts and licences checked, on Windows | the same bytes on Linux and macOS; `BenchmarkCaptionMiss` 11.5 ms on a busy machine against the 8.8 ms limit, to re-measure idle |
| PixiJS on WebGPU, stage A (EPIC-40) | renderer selection with WebGL fallback (`cb4dab32`) | drawn signal, `data-drawn` and CSP nonces under WebGPU; `preview-vs-export` with both renderers in three browsers |
| WebCodecs, stage B (EPIC-42) | Chromium and Firefox pass the 58 `preview-vs-export` and 4 webcodecs specs; 500 repeats without a black texture; seek p50 3 ms p95 28 ms (busy machine) | WebKit on macOS (WebKit on Windows does not decode VP9, recorded in SDD-012 §22.4); NFR-003 and NFR-004 idle; soak shows no growth of open frames |
| Preview on the Rust renderer, stage D (EPIC-46) | nothing: deferred by the author on 2026-11-16 (question Q11, option 3) until the GPU path existed in the browser | everything |
| Native GPU export host, stage F (EPIC-48, optional) | `vukem-render` binary over pipes under the FFmpeg process policy, T-37, fallback to CPU, SSIM 1.0000 on 18 frames | wire `Job.GPUHost` in the owner process, package the binary, 4K export measured faster than CPU on an idle machine with a GPU |
| Switch and removal (EPIC-49) | nothing | everything |

Also open and related: the cold paused-frame budget (`TestViewFrameColdP95`, 365-385 ms against 350 ms) is red on the author's machine (GitHub issue #98); step 7 measures it idle before and after the switch.

## 3. Steps

### Step 1: Run the Rust gate and the parity harnesses on Linux and macOS in CI

- Owner: senior-d (workflow), senior-b (fixes).
- Files: the renderer job of `.github/workflows/` (the job of former TASK-0188 already runs `cargo fmt/clippy/test/deny` and two reproducible WebAssembly builds on three systems); add the Go parity tests that compare the Go compositor with the Rust CPU path (native and wazero) and the export goldens with `VUKEM_VIDEO_EXPORT_RENDERER=rust`, on `ubuntu`, `macos` and `windows` runners.
- Commit: `ci(renderer): prove renderer parity on the three systems`.
- Acceptance:
  - [ ] every golden frame is byte-identical between the Go compositor and the Rust CPU path, native and WebAssembly, on Linux, macOS and Windows (job logs linked in the report);
  - [ ] export goldens are byte-identical with the Rust renderer selected on the three systems;
  - [ ] raster and export goldens of the text rasterizer are byte-identical on the three systems; `TestBundledFontsHaveLicence` passes;
  - [ ] `cargo clippy -- -D warnings`, `cargo test`, `cargo deny check` pass everywhere; no `unsafe` outside the reviewed FFI boundary (`.agents/rules/rust.md`);
  - [ ] `go run ./internal/tools/threatcheck` passes.
- Closes the open asserts of EPIC-43 and the non-benchmark asserts of EPIC-44 and EPIC-47. A byte difference on one system is a bug to fix in `renderer/` (never a tolerance), with a test.

### Step 2: GPU parity on Metal and a Linux adapter

- Owner: senior-b.
- Run the GPU vs CPU parity harness on the macOS runner (Metal) and on Linux with a software Vulkan adapter (or a GPU runner when available); record per adapter.
- Commit: `test(renderer): record GPU parity per adapter on macOS and Linux`.
- Acceptance:
  - [ ] each golden within SSIM 0.99 and 0.5 px of the CPU path on every adapter tested, recorded per adapter in SDD-013;
  - [ ] `cargo clippy -- -D warnings`, `cargo test`, `cargo deny check` pass.

### Step 3: Stage A in a browser with a real WebGPU adapter

- Owner: senior-e.
- Make the Playwright runs get a WebGPU adapter: headed Chrome on the author's Windows machine (RTX 4060), and in CI Chromium with a software WebGPU adapter if the pinned Chromium supports one; record which browsers get WebGPU.
- Keep the drawn signal, `data-drawn` and the CSP nonce rules working with the WebGPU renderer (former TASK-0192), then run `preview-vs-export` with both renderers in the three browsers (former TASK-0193).
- Commit: `test(preview): run preview-vs-export on WebGPU and WebGL`.
- Acceptance:
  - [ ] `preview-vs-export` passes in every case with WebGPU where available and with WebGL, in Chromium, Firefox and WebKit, with `KNOWN_DIVERGENCES` empty;
  - [ ] no `securitypolicyviolation` in any spec (the guard of `web/e2e/` helpers);
  - [ ] in `web/`: `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass;
  - [ ] SDD-012 §11 updated.

### Step 4: Stage B on WebKit for macOS

- Owner: senior-e. Run the WebCodecs specs and `preview-vs-export` on the macOS runner's WebKit.
- Commit: `test(preview): cover WebCodecs on WebKit for macOS`.
- Acceptance:
  - [ ] `preview-vs-export` passes in every case with WebCodecs in Chromium, Firefox and WebKit (WebKit on Windows may use the `<video>` fallback, as recorded);
  - [ ] the black-texture case does not occur in 500 repeated golden cases (`--repeat-each`);
  - [ ] the seek lands on the requested frame (the drawn signal reports the source frame) in 100 random seeks;
  - [ ] SDD-012 §11 updated. (NFR-003, NFR-004 and the soak are measured in step 7.)

### Step 5: Stage D: the preview draws with the Rust renderer over WebGPU

- Owner: senior-e, with senior-b for the module build and senior-c for the CSP header.
- Gate: the author's request of 2026-11-18 to finish the renderer ends the Q11 deferral; the CSP change (`script-src` with `wasm-unsafe-eval`, defined in SDD-012) is a security change: write it into SDD-007 and SDD-012 first and get the orchestrator's review before code.
- Work (former TASK-0215, 0216, 0217): load the WebAssembly module in the decode worker (served with the editor build), draw with WebGPU, fall back to the CPU path on a 2D canvas; WebCodecs frames go straight to the renderer as textures and the drawn signal reports the renderer's frame; when the gate passes, remove the PixiJS executor and the `pixi.js` dependency in the same change.
- Commits: one per former card, e.g. `feat(preview): draw the preview with the Rust renderer`, `feat(preview): feed WebCodecs frames to the renderer`, `refactor(preview): remove the PixiJS executor`.
- Acceptance:
  - [ ] preview against export within SSIM 0.99 and 0.5 px in every golden case in Chromium, Firefox and WebKit;
  - [ ] no `securitypolicyviolation`; `check:thin-client` passes (the renderer decides nothing);
  - [ ] the switch to the truth-on-pause frame shows no visible change (pixel difference recorded);
  - [ ] SDD-012 §11 and §20 updated.

### Step 6 (optional, stage F): finish the native GPU export host

- Owner: senior-b. Wire `Job.GPUHost` in the owner process, package `vukem-render` with the release archives and installers (senior-d for packaging), keep the CPU path through `wazero` as default and fallback.
- Commit: `feat(export): use the native GPU host when an adapter is present`.
- Acceptance:
  - [ ] parity with the CPU path on the export goldens, as in step 2;
  - [ ] the `TestSecurity_` process-policy tests pass for `vukem-render`;
  - [ ] SDD-004 and SDD-007 updated. (The 4K speed measurement is in step 7.)
- If the author drops stage F, mark this step REJECTED with the reason and skip it.

### Step 7: Measure every budget on the idle reference machine

- Owner: orchestrator, with every agent stopped and the machine idle (AGENTS.md section 9).
- Measure, before and after the switch of step 8 where it applies: export speed with the Rust renderer within 20% of the Go compositor and NFR-007 memory (`BenchmarkExport60s`); `BenchmarkCaptionMiss` at most 8.8 ms; NFR-001, NFR-003, NFR-004 and NFR-008 in the browser with the Rust preview; the 5-minute soak shows no growth of open `VideoFrame`s; `TestViewFrameColdP95` against 350 ms; with stage F, 4K export faster on the GPU host than on the CPU path; every NFR of SDD-004 §12 and SDD-012 §19, written into those sections with the commit.
- Acceptance:
  - [ ] every number recorded with machine, commit and date; a number over its limit is fixed in the responsible code with a test, or blocks step 8 (no budget is relaxed here).

### Step 8: Make the Rust renderer the default and delete the duplicates (the delivery of this plan)

- Owner: senior-b (engine), senior-e (preview).
- Make `rust` the default export renderer, delete `engine/compositor` and the switch's `go` value, and delete what remains of the PixiJS executor, in one release. Update SDD-004, SDD-012 and the status note of ADR-0003 (items 3 and 4 now fully replaced by ADR-0029); the changelog entry is written by the release tool from the commits.
- Commit: `feat(export): make the Rust renderer the only renderer`.
- Acceptance:
  - [ ] `go test ./engine/... ./service/...` and, in `web/`, `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass;
  - [ ] `git grep -n "engine/compositor"` finds nothing outside history and ADRs;
  - [ ] every Phase 5 exit criterion of ADR-0029 holds (re-run steps 1-4 on the release candidate);
  - [ ] `bash .agents/skills/verify/scripts/verify.sh --full` passes.

## 4. Out of scope

- New effects or transitions (they follow SDD-004 formulas first and land in both paths until step 8, then only in Rust).
- Any studio plan (001-010).

## 5. STOP conditions

- A byte difference between systems that comes from the platform's floating point or SIMD: stop and report; the fix belongs in the renderer's fixed-point arithmetic, and the decision is the orchestrator's.
- No WebGPU adapter can be obtained in any automated browser run: run step 3 headed on the author's machine only, record it, and ask whether CI may skip WebGPU.
- The CSP change of step 5 is not approved: stop step 5; steps 6-8 then keep PixiJS for the preview, and step 8 deletes only the Go compositor.
- A budget of step 7 fails and the fix is not small: stop and report the numbers before step 8.

## 6. Maintenance

Until step 8, every render change goes into both the Go compositor and the Rust CPU path, with the parity harness green. After step 8, `renderer/` is the only place pixels are composed (AGENTS.md rule 5), and the parity harness becomes a GPU-vs-CPU harness only.
