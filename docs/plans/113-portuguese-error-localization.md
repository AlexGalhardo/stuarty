# 113: Complete Portuguese localization of all error messages, hints and diagnostics

## Goal

When Stuarty is set to Brazilian Portuguese (`pt-BR`), every error alert, toast, validation message, export failure, agent error, lint finding, and recovery hint across the entire application is displayed in clear, natural Portuguese—never falling back to raw English backend strings like `"The video encoder failed. Run \"stuarty doctor\"..."`.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #20**: Several error notices and warnings are still displayed in English (including the export failure dialog in v0.60.0: `"The video encoder failed. Run \"stuarty doctor\" and try the export again..."`); translate everything into Portuguese as well.
- Root causes verified in the codebase:
  1. `web/resources/js/components/Failure.tsx:72-81` only translates the headline (`errorHeadline(t, failure.code)`) and ALWAYS renders the backend's English `failure.message` and `failure.hint` verbatim inside `<p lang="en">`! It never even invokes `errorText()`!
  2. `web/resources/js/editor/ui/errorText.ts` only covers **12 static error codes** (`STATIC_BODIES`) and **6 parameterized switch cases**, falling back to raw English (`asReceived`, `localized: false`) for **dozens of `E_*` error codes** (including `E_EXPORT_FAILED`, `E_VERIFY_FAILED`, `E_AGENT_UNAVAILABLE`, `E_PROVIDER`, `E_PRICE_UNKNOWN`, `E_COST_CEILING`, `E_APPROVAL_REQUIRED`, `E_PRIVILEGED_SESSION`, `E_LOCKED`, `E_NOT_FOUND`, `E_CONFLICT`, `E_INVALID_STATE`, `E_LIMIT`, `E_UNSUPPORTED_MEDIA`, `E_TYPE_MISMATCH`, `E_WEBHOOK_*`, `E_RECORDING_*`, `E_SOCIAL_*`, etc.)!
  3. Over 25 UI components across `web/resources/js/` (including `Failure.tsx`, `ErrorBody.tsx`, `FailureView.tsx`, `ExportDialog.tsx`, `ExportOptions.tsx`, `BatchProgress.tsx`, `AutoCaptions.tsx`, `InstallDialog.tsx`, `Library.tsx`, `RecordPanel.tsx`, `CutsStage.tsx`, `ConflictNotice.tsx`, `ImageEditor.tsx`, `Error.tsx`, `CaptionPanel.tsx`, `Webhooks/Index.tsx`, `Library/parts.tsx`) render raw server English strings tagged `lang="en"`.
  4. Some Go service/engine errors return free-text English reasons without a structured `details.reason` key (e.g., `engine/export/encode.go` already sets `details.reason = "encoder"` / `"render"` / `"size_limit"`, while several `service/` errors only carry a generic `E_VALIDATION` or `E_INVALID_STATE` message without `details.reason`).
- ADR-0007 (thin UI client: the core/service remains the source of error codes and structured `details`; the UI renders localized wording from `code` + `details.reason` + structured `details` params in `en.json` and `pt-BR.json`), SDD-000 (canonical error code table), SDD-012.

## Scope

- Modules owned:
  - `core/` and `service/` / `engine/` (audit every `E_*` error emission to ensure every distinct user-facing failure carries a stable `code` and, where an error code has sub-cases like `E_EXPORT_FAILED`, `E_AGENT_UNAVAILABLE`, `E_PROVIDER`, `E_INVALID_STATE`, `E_VALIDATION`, `E_LIMIT`, a stable machine-readable `details.reason` and structured parameters like `limit`, `max`, `field`, `encoder`—without breaking any existing Go error-code contract!).
  - `web/resources/js/lib/localizeError.ts` (new unified, app-wide error + hint + lint-finding + progress-message localizer shared by BOTH the dashboard and the video/image editors, unifying and expanding `editor/ui/errorText.ts` and `editor/ui/errorHeadline.ts`).
  - `web/resources/js/components/Failure.tsx`, `web/resources/js/pages/Error.tsx`, `web/resources/js/editor/commands/ErrorBody.tsx`, `web/resources/js/editor/ui/FailureView.tsx`, `web/resources/js/editor/export/ExportDialog.tsx`, `ExportOptions.tsx`, `BatchProgress.tsx`, `ProgressText.tsx`, `AutoCaptions.tsx`, `InstallDialog.tsx`, `RecordPanel.tsx`, `CutsStage.tsx`, `ConflictNotice.tsx`, `ImageEditor.tsx`, `CaptionPanel.tsx`, `Webhooks/Index.tsx`, `Library/parts.tsx` (route every error, hint, lint finding, verify check, and status message through `localizeError` so `lang="en"` is only used as a last-resort fallback for truly unknown third-party text).
  - `web/resources/lang/en.json` and `web/resources/lang/pt-BR.json` (100% coverage of every `E_*` error code in `docs/sdds/000-shared-definitions.md`, every `details.reason` sub-case, every `L_*` lint rule message/hint, and every export/job progress phase).
  - `web/scripts/check-error-i18n.ts` (new check verifying that EVERY `E_*` error code defined in `docs/sdds/000-shared-definitions.md` and EVERY `L_*` lint code has a headline, message, and hint entry in BOTH `en.json` and `pt-BR.json`!).
- Must not touch: `renderer/`, `desktop/packaging/`, `site/` (unless reference docs regenerate).

## Out of scope

- Changing CLI JSON `message` / `hint` English defaults for scripts and AI agents (machine/CLI APIs keep English canonical `message`/`hint` alongside structured `code` and `details`, so existing CLI/MCP contracts and tests stay 100% intact!).
- Adding new languages beyond `en` and `pt-BR`.

## Dependencies

- Epics 74 and 99 (UI foundation and final design). Should coordinate `en.json`/`pt-BR.json` merges with epic 112.

## Technical design

1. **Structured error reasons in Go (`service/`, `engine/`)**:
   - Keep English `message` and `hint` on the wire for CLI/MCP/API consumers (zero breaking contract changes!).
   - Ensure every distinct user-facing error sets `details.reason` (e.g., `E_EXPORT_FAILED` with `reason`: `encoder`, `render`, `audio`, `verify`, `size_limit`, `webp_unavailable`, `cancelled`; `E_AGENT_UNAVAILABLE` with `reason`: `protocol_version`, `no_http_mcp`, `sign_in_required`, `start_failed`, `not_found`; `E_PROVIDER` with `reason`: `auth`, `rate_limited`, `refused`, `unavailable`, `timeout`, `bad_request`; `E_VALIDATION` with `reason` + `field` / `min` / `max` / `allowed`).

2. **Unified Frontend Error & Lint Localizer (`web/resources/js/lib/localizeError.ts`)**:
   - Resolution order for any `{ code, message, hint, details }`:
     1. Exact sub-case key: `ui.errors.reason.<code>.<details.reason>` (message) and `ui.errors.hint.<code>.<details.reason>` (hint), interpolated with safe structured params from `details` (`limit`, `field`, `encoder`, `seconds`, `item`, `time`, etc.).
     2. Code-level key: `ui.errors.message.<code>` and `ui.errors.hint.<code>`.
     3. Last-resort fallback: raw server `message` / `hint` (only then marked `lang="en"`), while "Copy details" in dialogs like Export always preserves the technical `code` + scrubbed diagnostic payload for bug reports.
   - Same pattern for `L_*` lint findings (`ui.lint.message.<code>`, `ui.lint.hint.<code>`) and job progress phases (`ui.jobs.phase.<phase>`).

3. **Zero-drift CI check (`web/scripts/check-error-i18n.ts`)**:
   - Reads all `E_*` and `L_*` codes from `docs/sdds/000-shared-definitions.md` (the single source of truth per AGENTS.md rule 3) and asserts that every single code has localized headline + message entries in BOTH `en.json` and `pt-BR.json`, with no untranslated English duplicates in `pt-BR.json`!

## Tasks

- [ ] 1. Audit all `E_*` and `L_*` codes in `docs/sdds/000-shared-definitions.md` and every `lang="en"` site in `web/resources/js/`; list any Go errors missing structured `details.reason` or params and update SDD-000 if new reason names are added. Verify: `task lint:md` exits 0.
- [ ] 2. Add missing structured `details.reason` / params in `service/` and `engine/` without changing any error code or breaking existing Go contract tests. Verify: `go test ./core/... ./engine/... ./service/... ./web/contract/...` passes.
- [ ] 3. Implement `web/resources/js/lib/localizeError.ts` and populate complete `en.json` and `pt-BR.json` translations for every `E_*` code, every `details.reason` sub-case (including `E_EXPORT_FAILED.encoder`, `E_VERIFY_FAILED`, `E_LOCKED`, `E_AGENT_UNAVAILABLE.*`, `E_PROVIDER.*`, `E_VALIDATION.*`, etc.), every `L_*` lint rule, and job progress strings. Verify: `cd web && bun run check:i18n && bunx vitest run resources/js/lib/localizeError.test.ts`.
- [ ] 4. Wire `localizeError` into `Failure.tsx`, `Error.tsx`, `ErrorBody.tsx`, `FailureView.tsx`, `ExportDialog.tsx`, and all remaining `lang="en"` error/lint/progress displays across `web/resources/js/`. Verify: `cd web && bun run typecheck && bun run check:thin-client && bunx vitest run`.
- [ ] 5. Implement `web/scripts/check-error-i18n.ts` (`bun run check:error-i18n`), run it against SDD-000 and both lang files, and add a Playwright E2E test in `pt-BR` triggering representative failures (login error, `E_EXPORT_FAILED`, validation error, missing file, locked session) and asserting Portuguese text with zero English fallback phrases. Verify: `cd web && bun run check:error-i18n`, `task lint`, `task test:web`, and the Playwright spec pass.

## Test strategy

- Unit tests: `localizeError.test.ts` testing every `E_*` code from SDD-000 in both `en` and `pt-BR`, verifying parameterized interpolation (`E_OVERLAP`, `E_EXPORT_FAILED` with `reason: "encoder"`, `E_QUEUE_FULL`, `E_RATE_LIMITED`, `E_COST_CEILING`, `E_VALIDATION`), and verifying `localized: true` for all known codes.
- Static guard: `bun run check:error-i18n` guarantees 100% alignment between SDD-000 error/lint catalogues and `en.json` + `pt-BR.json`.
- E2E (Playwright): switch locale to `pt-BR`, trigger export failure / validation failure / auth failure / 404 error page, and assert every visible headline, message, and hint is in Portuguese (`Falha na exportação`, `O codificador de vídeo falhou.`, etc.).

## Acceptance criteria

1. `cd web && bun run check:error-i18n` exits 0, proving 100% of `E_*` and `L_*` codes in SDD-000 have localized entries in both `en.json` and `pt-BR.json`.
2. In `pt-BR`, `FailureNote`, `ErrorBody`, `FailureView`, `Error.tsx`, and `ExportDialog` display Portuguese messages and hints (e.g., `E_EXPORT_FAILED` with `reason: "encoder"` displays Portuguese equivalents of `"The video encoder failed."` and its doctor hint) instead of raw English `<p lang="en">`.
3. CLI, HTTP API, and MCP responses preserve their canonical English `message`/`hint` and structured `code`/`details` with zero contract regressions.
4. `go test ./service/... ./engine/...`, `task lint`, `task test:web`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bun run check:error-i18n`, and the Playwright `pt-BR` error spec pass.

## Risks and mitigation

- **Existing tests asserting `lang="en"` or exact English error strings.** Mitigation: update tests like `Webhooks/Index.test.tsx` and `Failure.test.ts` to assert localized output when the code is known and `lang="en"` only for unknown/third-party fallback errors.
- **Shared `lang/*.json` file conflicts.** Mitigation: coordinate merge timing with epics 103, 111, and 112 per `.agents/rules/plans.md` §4.

## Size

M

## Hand-off for review

The commits of `epic/113-portuguese-error-localization`, output of `check:error-i18n` and `check:i18n`, Go/Vitest/Playwright outputs, and screenshots of the Export failure modal and error pages in `pt-BR`.
