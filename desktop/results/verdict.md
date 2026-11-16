# Desktop detection runs (P3-DSK-001)

Date: 2026-11-15. Host: Windows 11 Pro 10.0.26300. Page: `desktop/detect` of `release/v0.9.0` (commit 70250a9c). Spec: `web/e2e/specs/desktop-detect.spec.ts`, run headless in Chromium (project `desktop-detect`) and in the installed Microsoft Edge (`channel: "msedge"`, a scratch config outside the repository).

| Engine | Version | Verdict | `video-seek` p50 | `video-seek` p95 | NFR-003 (100 ms) | Results file |
| --- | --- | --- | --- | --- | --- | --- |
| Chromium (Playwright build) | 156.0.8078.4 | pass | 3.7 ms | 6 ms | met | `win32-chrome-156.0.8078.4-2026-11-15.json` |
| WebView2 (Edge proxy) | 157.0.0.0 (user agent); Edge binary 157.0.4341.60 | pass | 7.1 ms | 10.9 ms | met | `win32-chrome-157.0.0.0-2026-11-15.json` |

Required tests that failed: none (15 of 15 in each run). Not required and not passed: `fast-seek`, in both runs (reported only).

Notes on the runs:

- The results files are named by the server from the user agent, so the Edge file says `chrome`; its engine is the WebView2 (Edge) one. The user agent reports 157.0.0.0, the installed binary 157.0.4341.60.
- Both runs are headless. The verdict of a headed run on Windows was not measured.
- Not run on this machine: WebKit through Playwright (its browser is not installed here and was not downloaded).
- Not run yet: WKWebView on macOS and WebKitGTK 6.0 on Linux, the engines that decide the shell (ADR-0023, items 4 and 5). They need runs on those systems; a CI job for them is being added by senior-e. The Windows runs above do not decide that question.

Options for the author (ADR-0023, item 5), not decided here: Wails v3 with the system webviews, or Electron on the platforms whose verdict is `fail`.

Author decision (2026-11-15): Wails v3 (v3.1.0, the last stable release) with the system webviews, recorded in ADR-0023 and in the author's decision log. The runs on macOS (WKWebView) and Linux (WebKitGTK 6.0) are still owed by P3-DSK-001 on real machines; a `fail` there reopens the Electron option for that platform.

## Run of 2026-11-16 (TASK-0096)

Host: Windows 11 Pro 10.0.26300. Page: `desktop/detect` of `release/v0.24.0`. Both runs are headed (a window, not headless) and driven with Playwright through a scratch script outside the repository: the same two manual steps as `desktop-detect.spec.ts` (drag the box, click Run again), then Send results.

| Engine | Version | Verdict | `video-seek` p50 | `video-seek` p95 | NFR-003 (100 ms) | Results file |
| --- | --- | --- | --- | --- | --- | --- |
| Chromium (Playwright build) | 156.0.8078.4 | pass | 10 ms | 21 ms | met | `win32-chrome-156.0.0.0-2026-11-16.json` |
| Edge (the engine of WebView2) | 157.0.4341.60 | fail (first run) | 7 ms | 26 ms | met | `win32-chrome-157.0.0.0-2026-11-16.json` |
| Edge (the engine of WebView2) | 157.0.4341.60 | pass (second run) | 9 ms | 23 ms | met | `win32-chrome-157.0.0.0-2026-11-16-2.json` |

The `fail` of the first Edge run is the `pointer` test only ("0 real moves captured"): the drag had too few intermediate moves. The second run moves in steps and passes; the first file is kept because it is a real run. `fast-seek` is not required and is missing in both engines. No CSP violation was reported in any run. The machine was loaded by other agents, so the `video-seek` numbers are not a performance measurement (the budget of NFR-003 is measured alone).

Not run here: WKWebView (macOS) and WebKitGTK 6.0 (Linux); the real WebView2 window of the desktop shell was not driven again, Edge stands in for it as on 2026-11-15.
