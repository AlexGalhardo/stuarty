# Plan 013: Unify frontend API transport, expand `check-thin-client` coverage, and remove dead `Pending` scaffolding

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- web/resources/js/editor/api/client.ts web/resources/js/editor/api/http.ts web/resources/js/editor/api/transport.ts web/resources/js/image-editor/api/imageApi.ts web/resources/js/pages/Publish/api.ts web/resources/js/pages/Publish/calendarApi.ts web/resources/js/pages/Library/client.ts web/resources/js/pages/Write/client.ts web/scripts/check-thin-client.ts web/app/http/endpoints.go web/app/http/handlers.go web/app/http/api.go web/app/mcp/server.go web/app/mcp/tools.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: `improve/009-fix-rust-text-bounds-and-frontend-preview-async-bugs.md` (shares `web/resources/js/editor/api/http.ts`)
- **Category**: tech-debt
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

Three high-leverage architectural leaks make frontend and adapter maintenance increasingly error-prone:

1. **Inverted layering & 6 duplicated `call<T>` wrappers with inconsistent retry**: 218 imports from outside `editor/` reach into `web/resources/js/editor/api/*`; 82 sites inline `new HttpTransport(...)` mixing `""` (root-relative `/auth/*`, `/openapi.json`) vs `"/api/v1"` on the same `ApiClient` interface (a silent-404 footgun); and 6 domain API clients copy-paste the same `transport.send` / `TransportError` / `fromResponse` wrapper — with 5 of the 6 silently omitting `withRetry`!
2. **41K LoC outside `check-thin-client.ts`**: `editorRelative()` returns `null` for all 36,207 LoC outside `editor/` and `image-editor/`; 4 new folders inside `editor/` (`record/`, `stages/`, `teleprompter/`, `find/` — 5,046 LoC) are missing from `MAY_IMPORT`; and `image-editor/` is skipped by `checkImportGraph`. Real layering violations have already crept into those blind spots (`RecordTeleprompter.tsx` imports upward into `../../pages/Write/client`).
3. **Dead `Pending` route/tool scaffolding & unused `Options.Registry`**: zero HTTP endpoints or MCP tools are pending, yet `Endpoint.Pending`, `pending()`, `mcp.Pending`, `mcp.Options.Registry`, and unused `oxc-transform-react` in `site/package.json` remain live dead code.

## Current state

- `web/resources/js/editor/api/client.ts:164-589`: mixes `/api/v1` methods and root-relative `/auth/*` + `/openapi.json` methods; `call<T>` (lines 561–589) is duplicated in `image-editor/api/imageApi.ts:68-92`, `pages/Publish/api.ts:41-58`, `pages/Publish/calendarApi.ts:50-67`, `pages/Library/client.ts:189-210`, and `pages/Write/client.ts:272-293` (none of those 5 use `withRetry`).
- `web/scripts/check-thin-client.ts:119-143`:

```ts
const FEATURES = ["timeline", "inspector", "library", "export", "history"];
const MAY_IMPORT: Record<string, string[]> = {
	api: [],
	state: ["api"],
	commands: ["state", "api", "ui", "a11y"],
	preview: ["state", "api", "ui"],
	ui: [],
	a11y: [],
	layout: [...FEATURES, "commands", "preview", "state", "ui", "a11y"],
	boot: ["*"],
	...Object.fromEntries(
		FEATURES.map((f) => [f, ["state", "commands", "ui", "a11y", "preview", "api"]]),
	),
};

function editorRelative(file: string): string | null {
	const image = file.lastIndexOf("/image-editor/");
	if (image >= 0) return file.slice(image + 1);
	const marker = "/editor/";
	const at = file.lastIndexOf(marker);
	return at < 0 ? null : file.slice(at + marker.length);
}
```

- `web/app/http/endpoints.go:33-35`, `web/app/http/api.go:155-157`, `web/app/http/handlers.go:206-213, 339-343`, `web/app/mcp/server.go:41-50`: unused `Pending` scaffolding and `mcp.Options.Registry`.
- Conventions: Biome formatting (tabs, double quotes, semicolons), `@/` path alias maps to `web/resources/js/` (`web/tsconfig.json:13`), zero new dependencies.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Web typecheck, lint, thin-client, tests | `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` | exit 0 |
| Go HTTP & MCP tests | `go test -race -count=1 ./web/app/http/... ./web/app/mcp/...` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |
| Thin-client script tests | `cd web && bun run test scripts/` | all pass, exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `web/resources/js/lib/api/` (create: `transport.ts`, `http.ts`, `errors.ts`, `retry.ts`, `call.ts`, `index.ts` — re-exported from `editor/api/` for backwards compatibility during incremental migration!)
- `web/resources/js/editor/api/{transport,http,errors,retry,client}.ts`
- `web/resources/js/image-editor/api/{imageApi,modelEdit}.ts`
- `web/resources/js/pages/Publish/{api,calendarApi}.ts`
- `web/resources/js/pages/Library/client.ts`
- `web/resources/js/pages/Write/client.ts`
- `web/resources/js/editor/record/RecordTeleprompter.tsx` (fix upward import from `pages/Write/client`)
- `web/scripts/check-thin-client.ts` (+ `web/scripts/check-thin-client.test.ts` if present)
- `web/app/http/{endpoints,handlers,api}.go`
- `web/app/mcp/{server,tools}.go` (and callers passing `Options.Registry` in `web/app/mcp/*_test.go`, `web/app/cli/`, `web/bootstrap/`)
- `site/package.json` & `site/bun.lock` (only if removing unused `oxc-transform-react` — verify zero imports first!)

**Out of scope** (do NOT touch):

- Rewriting all 218 external imports of `editor/api/` in one giant diff — move the primitives to `lib/api/` and re-export from `editor/api/` so non-editor pages can migrate incrementally without massive churn, while updating the 6 domain API clients directly!
- `openapi-typescript` codegen (`ARCH-02`), `builders.ts` typing (`ARCH-03`), or Go `service/` package split (`ARCH-09`) — tracked separately in `improve/README.md`.

## Git workflow

- Branch: `refactor/api-transport-thin-client-and-dead-code` cut from `release/v0.60.0` (after Plan 009).
- Commit in three clean steps:
  1. `refactor(ui): extract shared lib/api transport and callJson with unified retry`
  2. `refactor(web): expand check-thin-client to new editor folders and studio domains`
  3. `refactor(http): remove dead Pending route/MCP scaffolding and unused mcp.Options.Registry`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Extract shared `web/resources/js/lib/api/` and consolidate all 6 `call<T>` wrappers

1. Move `Transport`, `HttpTransport`, `TransportError`, `ApiError`/`fromResponse`, and `withRetry` into `web/resources/js/lib/api/` and add a single shared `callJson<T>(transport, req, opts?)` helper (with opt-out/opt-in retry policy matching `editor/api/client.ts:561-589`).
2. Provide shared singleton/factory helpers in `web/resources/js/lib/api/index.ts`:
   - `apiTransport` (`new HttpTransport("/api/v1")`) and `rootTransport` (`new HttpTransport("")`) so callers stop passing raw base strings by memory.
   - Update `editor/api/{transport,http,errors,retry}.ts` to re-export from `@/lib/api` so all existing imports keep compiling cleanly.
3. Update all 6 domain API clients (`editor/api/client.ts`, `image-editor/api/imageApi.ts`, `image-editor/api/modelEdit.ts`, `pages/Publish/api.ts`, `pages/Publish/calendarApi.ts`, `pages/Library/client.ts`, `pages/Write/client.ts`) to use the shared `Transport` interface and `callJson<T>` from `@/lib/api`.
4. Fix `web/resources/js/editor/record/RecordTeleprompter.tsx:10-11` so it uses a shared writing-read helper from `@/lib/api` (instead of importing upward into `../../pages/Write/client` and instantiating `new HttpTransport`).

**Verify**: `cd web && bun run typecheck && bun run lint && bun run test` -> all pass, exit 0.

### Step 2: Expand `web/scripts/check-thin-client.ts` to cover all `editor/` subfolders, `image-editor/`, and studio pages

1. Read `web/scripts/check-thin-client.ts` in full (especially `checkImportGraph` around lines 330–365 and `R6` network rules).
2. Add `record`, `stages`, `teleprompter`, and `find` to `FEATURES` / `MAY_IMPORT` in `web/scripts/check-thin-client.ts:119-132`.
3. Fix `checkImportGraph` so `image-editor/<subfolder>/...` is checked with its own subfolder graph instead of falling through on folder `"image-editor"`.
4. Extend `R6` (no raw `fetch` / `EventSource` outside `api/` or `lib/api/`) to `pages/`, `dashboard/`, `assetlibrary/`, and `shell/` — first run `node scripts/check-thin-client.ts` to see every existing violation; fix any accidental violations in-scope or document legitimate exceptions explicitly (never weaken existing rules!).
5. Update/add tests in `web/scripts/check-thin-client.test.ts` (if present) covering `editor/record/`, `image-editor/`, and `pages/`.

**Verify**: `cd web && bun run check:thin-client && bun run typecheck && bun run lint && bun run test` -> all pass, exit 0.

### Step 3: Remove dead `Pending` scaffolding and unused `mcp.Options.Registry`

1. Verify zero references set `Pending: true` / `Pending:` across `web/app/http/endpoints.go` and `web/app/mcp/`.
2. Remove `Endpoint.Pending` (`web/app/http/endpoints.go:33-35`), the pending branch in `web/app/http/api.go:155-157`, `pending()` and its startup loop in `web/app/http/handlers.go:206-213, 339-343`, `mcp.Pending` (`web/app/mcp/server.go:50`), and the stale comment in `web/app/mcp/tools.go:17-18`.
3. Remove `Options.Registry` (`web/app/mcp/server.go:41-44`) and update all struct literals referencing `Registry:` (grep `Registry:` across `web/`).
4. Verify whether `oxc-transform-react` in `site/package.json:39` is referenced anywhere (`grep -rn 'oxc-transform-react' site/`); if 0 references and `site` builds/tests cleanly without it, note or remove it in a separate check (respecting AGENTS.md rule 9 / `notices:check` if `THIRD_PARTY_NOTICES.md` tracks `site/`).

**Verify**: `go test -race -count=1 ./web/app/http/... ./web/app/mcp/... ./web/app/cli/... && task lint:go && task vet` -> all pass, exit 0.

## Test plan

- `web/resources/js/lib/api/call.test.ts` (create): unit tests for `callJson` covering success, `ApiError` mapping, `TransportError` propagation, and retry on transient failures vs no-retry on 4xx.
- `web/scripts/check-thin-client`: run `bun run check:thin-client` and its unit test suite verifying the newly covered folders (`record`, `stages`, `teleprompter`, `find`, `image-editor`, `pages`).
- Go HTTP & MCP unit/contract suites: `go test -race -count=1 ./web/app/http/... ./web/app/mcp/...`.

## Done criteria

- [x] `web/resources/js/lib/api/` holds the single `callJson` / `Transport` / `HttpTransport` / `errors` / `retry` implementation
- [x] All 6 domain API clients use `callJson` from `@/lib/api`
- [x] `RecordTeleprompter.tsx` no longer imports from `../../pages/Write/client`
- [x] `check-thin-client.ts` covers `record`, `stages`, `teleprompter`, `find`, `image-editor`, and `R6` across studio pages
- [x] `grep -rn 'Pending' web/app/http/endpoints.go web/app/mcp/server.go` returns no dead scaffolding matches
- [x] `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run test` exits 0
- [x] `go test -race -count=1 ./web/app/http/... ./web/app/mcp/... && task lint:go && task vet` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Enabling `R6` across `pages/`, `dashboard/`, `assetlibrary/`, and `shell/` reveals >10 legitimate direct `fetch`/`EventSource` call sites that require moving into new API clients — keep `R6` focused on `editor/*` + `image-editor/*` + `pages/Publish|Write|Library` and report the remaining files for a follow-up plan.
- Any test in `web/contract/` or `web/app/http/openapi_test.go` references `Endpoint.Pending` or `mcp.Pending` — read those tests first and update only if they test the removed dead feature itself.
- Removing `oxc-transform-react` from `site/package.json` fails `bun run build` in `site/` (Astro/Vite peer dependency) — leave `site/package.json` untouched!

## Maintenance notes

- New non-editor domain clients MUST import from `@/lib/api`, never `@/editor/api/`.
- Future follow-up plans (`ARCH-02`: wire `openapi-typescript` to retire 3,697 LoC of hand-maintained wire types; `ARCH-03`: type `builders.ts` against `@/generated/commands.ts`) build cleanly on top of this shared `lib/api/` foundation.
