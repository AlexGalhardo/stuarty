# Plan 020: Prove the editor performance and preview budgets on the idle reference machine

This plan answers whether the browser editor meets its latency, memory and preview-fidelity budgets (NFR-001, NFR-003, NFR-004, NFR-008, NFR-009, NFR-011) when measured on the reference machine with nothing else running. When delivered, the preview-vs-export, scrub, pause, edit and soak specs pass with recorded numbers, the final values of the UI constants are written in SDD-012, and the full browser suite exits 0.

- Owner: senior-e (all steps); orchestrator for the idle-machine session
- Effort: M
- Depends on: an idle reference machine with every agent stopped (SDD-008 section 8); plan 021 for the preview-vs-export golden frames when the engine changes
- Origin: EPIC-02 (P1-UI-003): TASK-0012; EPIC-10 (P2-QA-001): TASK-0050; EPIC-11 (P2-QA-002): TASK-0053; EPIC-12 (P2-QA-003): TASK-0056; EPIC-13 (P2-QA-004): TASK-0060; EPIC-34 (P4-UI-001): TASK-0161, TASK-0162, TASK-0163
- Status: BLOCKED (measures a performance budget; needs the reference machine idle with no agents running, and Linux/macOS runners; the 10-minute soak needs an idle machine alone)
- Written against: 8787714a

## 1. Why

Every earlier measurement was taken on a shared or loaded machine, so several budgets are recorded as unproven: `vv:edit` p95 161 to 320 ms against 50 ms, cold `vv:truth` p95 395 to 1678 ms against 500 ms, a tab at 956 MB of 1024 MB, long tasks of 58 to 179 ms. SDD-012 section 19.1 itself says these must be repeated on the idle reference machine. No budget is relaxed by this plan; code that misses a budget is fixed.

## 2. Current state (drift check)

```text
cd web && bun run lint && bun run typecheck && bun run check:thin-client && bun run test
cd web && bunx playwright test preview-vs-export
ls web/e2e/reference-machine
git log --oneline -n 10 -- web/e2e/specs web/e2e/support
```

Already true (from the former card statuses):

- Lint, typecheck and preview-vs-export of the three prototype scenes pass; `text-style` passes with SSIM 0.9941; `filter-invert` is intermittent under load; `video-layers` of the perf project needs the idle machine.
- The reference machine record exists (release v0.24.0): `web/e2e/reference-machine/doctor-2026-11-16.json` (Ryzen 7 5700X3D, RTX 4060, Windows 11 26300) and `reference-machine.test.ts`; `doctor` only complains that whisper is not installed.
- Full browser suite run on 2026-11-16: Chromium 284/288, Firefox 283/288, WebKit 214/216; three test defects fixed (templates, transform-handles, mask-preview); the remaining failures are load-only (export 120 s, HTTP 429) and pass alone.
- Last known numbers: NFR-003 passes (p95 26 to 65 ms); cold NFR-004 p95 395 to 931 ms, above 500 ms in several runs; heap 15 MB but tab 956 MB of 1024 MB with software WebGL on a loaded machine.
- SDD-012 section 22.3 is stale: it still lists items 2 and 3 as unmet (font.add was fixed by e876961c) and says the soak was not run (a2dbd15e ran it).

## 3. Steps

### Step 1: Prepare the idle reference session

Owner: orchestrator. Files: none.

Stop every agent and background process; run `vukem-video doctor --json` and compare with the stored reference record; if the machine differs, store a new record next to the old one.

Commit: none (or `test(ui): record the reference machine doctor output` if a new record is stored).

Acceptance:

- [ ] `vukem-video doctor --json` output is saved and matches or supersedes `web/e2e/reference-machine/doctor-2026-11-16.json`.
- [ ] No other agent or dev server runs during the measurements (state how it was checked).

### Step 2: Preview against export for every golden case

Owner: senior-e. Files: `web/e2e/specs/preview-vs-export.spec.ts`, `web/e2e/support/ssim.ts`, `web/e2e/specs/video-layers.spec.ts`.

Run the spec on the idle machine; fix the intermittent `filter-invert` case and the `video-layers` case of the perf project at their cause.

Commit: `test(ui): <english summary>` or none if only measurements.

Acceptance:

- [ ] `bunx playwright test preview-vs-export` passes for every case: luma SSIM of at least 0.97 and no layer centre displaced by more than 1 px (NFR-011).
- [ ] The three reference scenes of the prototype pass the same limits.

### Step 3: Scrub and pause latency

Owner: senior-e. Files: `web/e2e/specs/scrub-latency.spec.ts`, `web/e2e/specs/play-pause-truth.spec.ts`, `web/e2e/support/measures.ts`.

Acceptance:

- [ ] `vv:scrub` p95 is at most 100 ms over 30 seeks (NFR-003): a proxy frame appears within 100 ms p95 during scrub.
- [ ] `vv:truth` cold p95 is at most 500 ms (NFR-004): the exact frame appears within 500 ms p95 after pausing.

### Step 4: Edit latency and remote update

Owner: senior-e. Files: `web/e2e/specs/perf-edit-200.spec.ts`, `web/e2e/specs/two-actor-live-update.spec.ts`.

Acceptance:

- [ ] `vv:edit` p95 is at most 50 ms and `vv:edit-apply` p95 is at most 8 ms (NFR-001).
- [ ] `vv:remote` p95 is at most 200 ms.

### Step 5: Five-minute project soak

Owner: senior-e. Files: `web/e2e/specs/soak-5min.spec.ts`, `web/e2e/support/memory.ts`.

Acceptance:

- [ ] JS heap is at most 300 MB and the whole tab (renderer plus GPU process) is at most 1 GB after the 10 minutes (NFR-008); the numbers are printed by the spec.
- [ ] The 5-minute project with 200 items and 2,000 words opens and plays with no `longtask` above 50 ms (NFR-009).

### Step 6: Full browser suite

Owner: senior-e. Files: `web/e2e/` (read), `web/e2e/playwright.config.ts`.

Acceptance:

- [ ] `bunx playwright test` exits 0 for scenarios 1 to 27 (scenario 28 runs nightly).
- [ ] The status line of SDD-012 section 22.2 is updated, and section 22.3 no longer lists items 2 and 3 as unmet nor the soak as not run.

### Step 7: Three runs on the release candidate and the packaged desktop app

Owner: senior-e. Files: `web/e2e/specs/perf-edit-200.spec.ts`, `web/e2e/specs/soak-5min.spec.ts`, `web/e2e/specs/preview-vs-export.spec.ts`, `docs/sdds/012-ui-architecture.md`.

Run each performance spec three times on the reference machine with the release candidate build and with the packaged desktop application.

Acceptance:

- [ ] Three recorded runs of each spec exist (numbers listed per run).

### Step 8: Write the final constants and numbers

Owner: senior-e. Files: `docs/sdds/012-ui-architecture.md`, plus the code that misses a budget.

Write the measured numbers and the final values of `VIRTUALIZE_ABOVE`, `MAX_VIDEO_LAYERS`, `SNAP_PX`, the plan window prefetch and the cache sizes in SDD-012; fix code that misses a budget (no budget is relaxed here).

Commit: `docs(sdd): record the final UI budgets and constants`, plus one `fix(ui): <english summary>` per code fix with its test.

Acceptance:

- [ ] In all 3 runs: `vv:edit` p95 at most 50 ms, `vv:remote` p95 at most 200 ms, `vv:scrub` p95 at most 100 ms, `vv:truth` p95 at most 500 ms, JS heap at most 300 MB, tab below 1 GB, SSIM at least 0.97 and displacement at most 1 px.
- [ ] SDD-012 section 19.1 and section 22.2 are updated with the numbers.
- [ ] In `web/`: `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass, and `task lint` reports no findings.

## 4. Out of scope

- Relaxing any budget.
- Engine or renderer changes (plan 021 and the Rust renderer epics); only report the engine-side cause of a cold `ViewFrame` miss.
- Screen reader and accessibility work (plan 028).

## 5. STOP conditions

- The machine cannot be made idle or no GPU is present: stop; the numbers would not close the NFRs.
- A budget is missed and the cause is in `engine/`: stop and report to the orchestrator (the former card names the engine as owner of the cold NFR-004 miss).
- The Rust renderer replaces the preview executor before this plan runs: the numbers must be taken again against it; ask the orchestrator.

## 6. Maintenance

Measure again whenever the preview executor, the compositor or the timeline virtualisation changes. Keep the reference machine record next to the numbers so a later run can tell whether the machine changed.
