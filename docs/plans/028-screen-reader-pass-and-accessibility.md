# Plan 028: Screen reader pass (NVDA and VoiceOver) and WCAG 2.2 AA conformance

This plan answers whether the editor works with real screen readers: a person runs the screen reader checklist with NVDA on Windows and VoiceOver on macOS, every failure is recorded with its element and spoken text, every finding is fixed, and the pass is repeated for the screens that changed since the BETA. When delivered, the results file has no `fail` row and the browser, theme and language matrix report is ready for the release notes.

- Owner: senior-e (all steps); a person operates the screen readers
- Effort: M
- Depends on: a person with NVDA (Windows) and VoiceOver (macOS), and Firefox and WebKit full-suite runs; plan 020 for the final UI budgets
- Origin: EPIC-19 (P3-UI-004): TASK-0090, TASK-0092; EPIC-20 (P3-QA-002): TASK-0094, TASK-0095; EPIC-35 (P4-UI-004): TASK-0166, TASK-0167
- Status: BLOCKED (needs NVDA on Windows and VoiceOver on macOS operated by a person; the checklist already exists)
- Written against: 8787714a

## 1. Why

SDD-012 section 18.4 item 4 and RF-075 require a manual screen reader pass, and D-024 requires WCAG 2.2 AA. Automated axe runs cannot tell whether the spoken text is useful. No screen reader pass has been done, so there are no findings to fix yet.

## 2. Current state (drift check)

```text
ls web/e2e/manual web/e2e/manual/results
go test ./web/e2e/manual/results/...
cd web && bunx playwright test a11y-axe-both-themes
git log --oneline -n 10 -- web/e2e/manual web/e2e/a11y-allow.json
```

Already true (from the former card statuses):

- `web/e2e/manual/screen-reader-checklist.md` exists and covers SR-01..SR-16 (import, split, trim, caption, export, palette); `go test ./web/e2e/manual/results/...` ok (2026-11-16).
- axe passes in Chromium in both themes and both languages; the `wcag22` suite has 23 passed.
- Full browser suite on 2026-11-16 and later (ef392015.. and 7512b269): Chromium, Firefox and WebKit, light and dark, en and pt-BR; remaining failures are load flakes (axe export 120 s, 429 in scenarios).
- Not done: no screen reader was run; Firefox and WebKit full suites were not run at that tip; new `feat(ui)` commits entered since the BETA (the layout of the final UI), with no baseline difference recorded.

## 3. Steps

### Step 1: Run the checklist with NVDA and VoiceOver

Owner: senior-e with a person operating the readers. Files: `web/e2e/manual/screen-reader-checklist.md`, `web/e2e/manual/results/`.

Run the checklist with NVDA on Windows and VoiceOver on macOS and record each failure with the element and the spoken text.

Commit: `test(ui): record the screen reader results`.

Acceptance:

- [ ] `web/e2e/manual/results/screen-reader-<date>.md` has a row for every scenario and for each reader (NVDA, VoiceOver), with `pass` or the number of the open defect.
- [ ] The checklist has one result row per task and per reader, and each `fail` row carries the defect number opened for it.
- [ ] `go test ./web/e2e/manual/results/...` passes.

### Step 2: Fix every finding

Owner: senior-e. Files: `web/resources/js/editor/**` (fixes only), `web/e2e/a11y-allow.json`, `web/e2e/specs/a11y-axe-both-themes.spec.ts`.

Fix each finding of the manual pass, one commit per finding, each with a test.

Commit: `fix(ui): <english summary>` per finding.

Acceptance:

- [ ] In `web/`: `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass; each fix has a test that fails before and passes after.

### Step 3: Re-run the checklist after the fixes

Owner: senior-e with a person operating the readers. Files: `web/e2e/manual/results/`.

Acceptance:

- [ ] The screen reader checklist is run again after the fixes and `web/e2e/manual/results/screen-reader-<date>.md` has no `fail` row.

### Step 4: Repeat the checklist for the screens changed since the BETA

Owner: senior-e. Files: `web/e2e/` (read), `web/e2e/a11y-allow.json`.

Repeat the checklist for the screens changed since the BETA, including the final UI layout.

Acceptance:

- [ ] `go test ./web/e2e/...` passes.
- [ ] In `web/`: `bun run lint && bun run typecheck && bun run check:thin-client && bun run test` pass.

### Step 5: Matrix report for the release notes

Owner: senior-e; estagiario-1 runs the suites and reports. Files: `web/e2e/` (read).

Run `bunx playwright test` in Chromium, Firefox and WebKit, in dark and light mode, in English and pt-BR, and report each failure.

Acceptance:

- [ ] The matrix report (browser, theme, language, result) is attached to the release notes by the release plan (plan 031).
- [ ] `task lint` reports no findings.

## 4. Out of scope

- New features of the editor.
- Browser performance budgets (plan 020).
- Assistive technology other than NVDA and VoiceOver.

## 5. STOP conditions

- No person with the screen readers is available: stop; automated axe results cannot replace the pass.
- A finding needs a design change of the final UI: stop and ask the orchestrator.
- A `fail` row has no defect number: stop; it cannot be closed.

## 6. Maintenance

Repeat the checklist for any screen changed after this pass and at every release candidate; add a row to the checklist whenever a new task or screen is added to the editor. Keep `web/e2e/a11y-allow.json` free of entries without a written reason.
