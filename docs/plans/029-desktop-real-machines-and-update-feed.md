# Plan 029: The desktop shell on real machines, the shell verdict and the update feed

This plan answers whether the desktop shell works on real Windows 11, macOS and Linux machines: the page runs on four web engines with one results file each, the shell shows `/projects` and opens an editor route on all three systems, an installed N-1 build finds and installs N, and the reference scenario finishes in the installed app within ten minutes without any outbound connection. When delivered, the verdict file ends with a recommendation and the author's decision is the only thing left.

- Owner: estagiario-1 (runs and records, reviewed by senior-e); senior-e (shell, checklist runs); senior-d (update feed); orchestrator (closing the verdict)
- Effort: L
- Depends on: real macOS and Linux hardware (WKWebView and WebKitGTK 6.0) and the release artifacts running on Windows 11, macOS and Linux; the author for the shell decision
- Origin: EPIC-21 (P3-DSK-001): TASK-0096, TASK-0097, TASK-0099; EPIC-22 (P3-DSK-002): TASK-0104; EPIC-23 (P3-DSK-004): TASK-0106, TASK-0107; EPIC-26 (P3-REL-004): TASK-0122, TASK-0123
- Status: BLOCKED (needs the release artifacts running on real Windows 11, macOS and Linux; only Windows is available here; WKWebView and WebKitGTK have no hardware)
- Written against: 8787714a

## 1. Why

ADR-0023 items 2 to 6 and RF-213 require evidence on the real web engines before the shell choice is final, and the installed application has never been run on macOS and Linux. The update feed has only unit tests.

## 2. Current state (drift check)

```text
ls desktop/results
go test ./desktop/results/...
go test ./desktop/shell
cat desktop/checklist.md
```

Already true (from the former card statuses):

- Chromium 156 and Edge 157 (WebView2) results are recorded in `desktop/results/`; `desktop/results/verdict.md` has the video-seek table for two of the four engines; there is no failed-test line for macOS and Linux; `verdict.md` ends with the author's decision rather than a recommendation with reasons; ADR-0023 has the decision but no results note with the four-run table.
- `desktop/checklist.md` exists with 10 steps and a table per system (2026-11-16, senior-e): install, first run, import a file by dialog, edit, export, quit, update to a newer build, reopen.
- The shell: screenshots exist only for Windows (`desktop/results/windows`); the module `desktop/shell/app` has its own lint (0 problems on Windows); the root module lint is clean on 3 GOOS.
- Update: unit tests pass (`TestUpdate_ValidUpdate`, `_WrongDigest`, `_WrongSignature`); no real build of the previous minor was updated on any system. The feed signs `published_at` and `expires` and rejects an expired feed (f3408a78).

## 3. Steps

### Step 1: Run the page on four engines

Owner: estagiario-1. Files: `desktop/results/` (`<platform>-<webview version>-<date>.json`).

Run the page in Chromium, in the macOS WKWebView, in the Linux WebKitGTK 6.0 and in the Windows WebView2 (for the record), and version one results file per run.

Commit: `feat(repo): record the desktop web engine results`.

Acceptance:

- [ ] `go test ./desktop/results/...` passes, with a test that fails before and passes after.
- [ ] Four versioned results files exist, each with the `verdict` field and the measured values.

### Step 2: Compare `video-seek` with NFR-003

Owner: estagiario-1. Files: `desktop/results/`, `docs/adrs/0023-desktop-shell-deferred.md` (a results note; the status stays with the author).

Compare p50 and p95 of `video-seek` with NFR-003 (100 ms) and write the table in the ADR note.

Commit: `feat(repo): add the desktop results note to ADR-0023`.

Acceptance:

- [ ] `go test ./desktop/results/...` passes.
- [ ] The ADR-0023 note holds the table of the four runs.

### Step 3: Close the shell verdict

Owner: orchestrator. Files: `desktop/results/verdict.md`, `docs/adrs/0023-desktop-shell-deferred.md`.

Acceptance:

- [ ] `desktop/results/verdict.md` has, for each mandatory test that failed, a line with the webview, its version and the test id; the reviewer compares it with the `desktop/results/*.json` files.
- [ ] `desktop/results/verdict.md` ends with the recommendation (Wails v3 or Electron, with reasons) and the line `Author decision: pending` until the author answers.
- [ ] The results note of ADR-0023 is updated.

### Step 4: The shell window on three systems

Owner: senior-e. Files: `desktop/shell/` (`main.go` for Wails v3, or the Electron `main` if chosen), `desktop/shell/menu.go`, `desktop/shell/process.go`, `go.mod`, `desktop/results/`.

Acceptance:

- [ ] On each of the three systems the window shows `/projects` and opens an editor route, recorded with one screenshot per system in `desktop/results/`.
- [ ] `go test ./desktop/shell` passes and `task lint` reports no findings.

Commit: `test(ui): <english summary>` or none if only evidence.

### Step 5: Prove the update path from N-1 to N

Owner: senior-d. Files: `.github/workflows/release.yml`, `scripts/release-verify.sh`, `scripts/update-feed.sh`.

Test that an installed build of version N-1 finds and installs N (three VMs or the matching CI runners).

Commit: `ci(cli): <english summary>`.

Acceptance:

- [ ] A test covers the change (fails before, passes after).
- [ ] A build of the previous minor offers the update, verifies the signature and restarts on the new version; a manifest with a wrong digest is refused and the application keeps running.

### Step 6: Run the installed app checklist on three systems

Owner: senior-e (estagiario-2 runs the checklist). Files: `desktop/results/`, `desktop/checklist.md`.

Run `desktop/checklist.md` on Windows 11, macOS and Linux from the release artifacts and record the result per step.

Commit: `feat(ui): record the installed app results`.

Acceptance:

- [ ] `go test ./desktop/results/...` passes; in `web/`: `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass.
- [ ] The result per step is recorded for each of the three systems.

### Step 7: Close the installed app proof

Owner: senior-e. Files: `desktop/results/`, `desktop/README.md`.

Acceptance:

- [ ] The reference scenario (the steps of `prototype-reference`) finishes inside the installed app on each system, proven in `desktop/results/`.
- [ ] From download to the first exported video takes under 10 minutes on each system (NFR-040 clock recorded).
- [ ] The installed build opens no outbound connection by default (RF-206), verified by the system firewall log or a saved `netstat` capture in the results.
- [ ] Docs updated: `desktop/README.md`; any vukem friction found is recorded; `task lint` reports no findings.

## 4. Out of scope

- Buying signing certificates.
- Choosing the shell (the author decides from the verdict).
- The nightly and CI job definitions (plan 030).

## 5. STOP conditions

- No macOS or Linux hardware: stop; do not write results for engines that were not run.
- A mandatory test fails on an engine: record it in the verdict and stop; do not mark the engine as passing.
- The author has not decided between Wails v3 and Electron and a step needs the choice: stop and ask the orchestrator.
- The 10 minute clock is exceeded: report the slow step; do not relax NFR-040.

## 6. Maintenance

Repeat the four-engine run whenever the webview or the shell version changes, and the installed checklist at every release candidate. A new native menu item or window feature adds a line to `desktop/checklist.md`.
