# Plan 002: Eliminate Go and Playwright CI timeouts with a template SQLite DB, sharding, prebuilt E2E artifacts, and caching

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- web/app/store/open.go web/app/servicetest/env_test.go web/contract/testenv/testenv.go web/e2e/playwright.config.ts .github/workflows/ci.yml`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: `improve/001-unblock-pr172-ci-and-restore-missing-gates.md`
- **Category**: perf
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

On PR #172 (`release/v0.60.0`), 6 Go test shards (`rest`, `contract-1`, `contract-3`, `service-1`) timed out at 20 minutes and Playwright browser shards ran 10–20 minutes each. Four compounding causes drive this:

1. `store.Open` runs all ~82 SQLite migrations (including table rebuilds) from scratch on every test — ~1,000+ times across `servicetest` (918 tests) and `contract` (96 tests), taking ~1s per test under `-race`.
2. `.github/workflows/ci.yml` runs `-coverpkg=./...` on all 16 Windows and 12 macOS Go shards even though `Upload coverage` only runs on Linux (`runner.os == 'Linux'`), and leaves `rest` unsharded despite containing `web/app/{http,cli,mcp,owner,store}` and `service/*`.
3. Across all 18 Playwright shards, CI runs 24 Vite builds (Chromium shards run `bun run build` then `bun run e2e`, which runs `vite build --mode e2e` again), launches up to 5 concurrent `go run ./web/cmd/app serve` compiles per shard (never setting `STUARTY_E2E_BIN`), never caches FFmpeg/Bun/Playwright, and boots an unused `accountPort` server on every Chromium shard while never running `--project=account` (local-account auth/lock/recovery E2E).

## Current state

- `web/app/servicetest/env_test.go:87-99`:

```go
func newEnvWith(tb testing.TB, with func(e *env, o *service.Options)) *env {
	tb.Helper()

	e := &env{t: tb, dir: newDir(tb), clock: &clock{t: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}}

	st, err := store.Open(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = st.Close() })
```

- `web/contract/testenv/testenv.go:130-139`: calls `store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})` for every `testenv.New` call.
- `.github/workflows/ci.yml:231,245,248-251,284-289`:

```yaml
            export-*)
              ...
              go test -count=1 -timeout=17m -covermode=atomic -coverpkg=./... -coverprofile=coverage.out -run "^(${names})"'$' ./engine/export/
              exit
              ;;
            ...
            service-* | contract-*)
              ...
              go test -race -count=1 -timeout=17m -coverpkg=./... -coverprofile=coverage.out -run "^(${names})"'$' "$pkg"
              exit
              ;;
            rest) pkgs="$(go list ./... | grep -vE '/engine(/|$)|/web/app/servicetest$|/web/contract$')" ;;
          esac
          # shellcheck disable=SC2086
          go test -race -count=1 -timeout=17m -coverpkg=./... -coverprofile=coverage.out $pkgs
```

  And at lines 284–286: `if: runner.os == 'Linux'` — Windows and macOS discard `coverage.out`.

- `.github/workflows/ci.yml:313-315`: `test` aggregate job verifies exact shard count (`ubuntu-latest: 12`, `windows-latest: 16`, `macos-latest: 12`) — MUST be updated whenever shards are split!
- `.github/workflows/ci.yml:472-477`: Chromium shards run `bun run build` then `bun run e2e -- --project=chromium --project=chromium-person --project=failing-encoder --project=desktop-detect --shard="$SHARD"/6` (omitting `--project=account`, and `bun run e2e` in `web/package.json:21` runs `vite build --mode e2e && playwright test ...`).
- `web/e2e/playwright.config.ts:422-429`:

```ts
	].filter((server) => {
		if (detectOnly) return server.command.includes("desktop/detect");
		if (server.url === `${detectUrl}/`) return needsDetect;
		if (server.url === `http://127.0.0.1:${failPort}/up`)
			return needsFailingEncoder;
		if (server.url === `http://127.0.0.1:${personPort}/up`) return needsPerson;
		return needsApp;
	}),
```

  Missing a `needsAccount` filter for `accountPort` (lines 377–391).

- Conventions:
  - Pin every GitHub Action (`actions/cache`, `actions/upload-artifact`, `actions/download-artifact`) to an exact 40-char commit SHA with `# vX.Y.Z` comment (verified by `CI / Workflow pins`).
  - Keep `web/database/migrations/` and `web/app/store/open_test.go` running raw `store.Open` so full migration execution remains tested.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Store package tests | `go test -race -count=1 ./web/app/store/...` | `ok`, exit 0 |
| Servicetest smoke | `go test -race -count=1 -run 'TestProject\|TestApproval' ./web/app/servicetest/` | `ok`, exit 0 |
| Contract smoke | `go test -race -count=1 -run 'TestHTTP' ./web/contract/` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |
| Web typecheck & lint | `cd web && bun run typecheck && bun run lint` | exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `web/app/store/template_test_helper.go` (or `web/app/store/template.go` — export a small test helper `OpenFromTemplate` for cross-package test helpers `servicetest` and `testenv`, plus `web/app/store/template_test.go`)
- `web/app/servicetest/env_test.go`
- `web/contract/testenv/testenv.go`
- `web/e2e/playwright.config.ts`
- `.github/workflows/ci.yml`

**Out of scope** (do NOT touch):

- Production `store.Open` migration logic in `web/app/store/open.go` (keep production startup unchanged).
- `web/database/migrations/*_test.go` (must continue running full migrations).
- Individual Playwright spec files under `web/e2e/specs/`.

## Git workflow

- Branch: `perf/ci-template-db-and-caching` cut from `release/v0.60.0` (after Plan 001).
- Commit per step: e.g., `perf(store): pre-migrate template SQLite database for test harnesses`, `perf(ci): drop unused non-Linux coverpkg, shard rest, and cache E2E builds`.
- Do NOT push or merge to `main`.

## Steps

### Step 1: Add a `sync.Once` pre-migrated SQLite template helper in `web/app/store/`

1. Read `web/app/store/open.go` completely to confirm what files `store.Open` creates inside `dir` (`db.sqlite`, WAL/SHM files, and any metadata/version markers) and how `Close()` checkpoints WAL.
2. Add `web/app/store/template.go` exporting `SeedTemplateDB(ctx context.Context, dir workspace.Dir, opts Options) error` (or `OpenTemplate` guarded for tests) that:
   - Uses a `sync.Once` (keyed by `opts.SchemaVersion` if variable) to create a single temp workspace in `os.TempDir()`, run `store.Open` once with `opts`, explicitly run `PRAGMA wal_checkpoint(TRUNCATE)` on `st.DB()`, close `st`, read the resulting `db.sqlite` bytes into memory (removing the temp dir immediately).
   - Writes those cached bytes to `filepath.Join(string(dir), "db.sqlite")` with `0o600` permissions before delegating to `store.Open(ctx, dir, opts)` — so `store.Open` still runs all pragma checks, schema-version checks, and seal setup while executing 0 migrations.
   - Note: Only seed when `opts.Seal == nil` (unencrypted initial DB before account sealing, matching `env_test.go:92` and `testenv.go:132`).
3. Add `web/app/store/template_test.go` verifying that `OpenTemplate` produces the exact same `sqlite_master` table/index schema and `migrations` rows as raw `store.Open`, and that two concurrent calls are completely isolated (writes in one store never appear in the other).

**Verify**: `go test -race -count=1 ./web/app/store/...` -> `PASS`, exit 0.

### Step 2: Adopt `store.OpenTemplate` in `servicetest` and `contract/testenv`

1. Update `web/app/servicetest/env_test.go:92` to call `store.OpenTemplate(context.Background(), e.dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})`.
2. Update `web/contract/testenv/testenv.go:132` to call `store.OpenTemplate(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})`.
3. Measure before/after runtime on a representative subset (e.g., `go test -race -count=1 -run 'TestApproval' ./web/app/servicetest/`).

**Verify**: `go test -race -count=1 -run 'TestProject|TestApproval' ./web/app/servicetest/ && go test -race -count=1 -run 'TestHTTP' ./web/contract/` -> all pass, exit 0.

### Step 3: Gate `accountPort` startup in `web/e2e/playwright.config.ts` and wire `--project=account` in CI

1. In `web/e2e/playwright.config.ts`:
   - Read lines 1–100 to see how `needsDetect`, `needsFailingEncoder`, `needsPerson`, and `needsApp` are computed from `process.argv` (`--project=...`).
   - Add `const needsAccount = runs("account");` (matching `needsPerson`), and add an `accountPort` check (`server.url === ... + "/up"` returning `needsAccount`) inside the `.filter(...)` at lines 422–429.
2. In `.github/workflows/ci.yml`:
   - Add a dedicated single-runner step or job (or run `--project=account` unsharded on shard 1 of Chromium, since `account-first-run.spec.ts` uses `mode: "serial"`) so all 8 local-account first-run/lock/recovery E2E tests execute in PR CI without splitting serial state across shards.

**Verify**: `cd web && bun run typecheck && bun run lint` -> exit 0.

### Step 4: Optimize `.github/workflows/ci.yml` Go shards, E2E prebuilds, and caches

1. **Linux-only coverage instrumentation**: In `test-shard` (`ci.yml:223-251`), build `cover_flags=""`; set `cover_flags="-covermode=atomic -coverpkg=./... -coverprofile=coverage.out"` ONLY when `RUNNER_OS=Linux` (matching `Upload coverage` at line 285).
2. **Split `rest` shard**: Replace `rest` on all three OSes with `web-app` (`./web/app/... ./web/cmd/... ./web/database/...` excluding `servicetest`) and `core-service` (everything else outside `engine/`, `servicetest`, `contract`, and `web-app`), and increment `shards` in the `test` aggregate matrix (`ci.yml:313-315`) by 1 per OS (`ubuntu-latest: 13`, `windows-latest: 17`, `macos-latest: 13`). Verify no package is dropped or duplicated by testing the `go list` expressions locally!
3. **Prebuild E2E artifacts once**:
   - Add an `e2e-build` job (Ubuntu) that runs `bun install --frozen-lockfile`, `bun run build` (verifying production build once), `bun run build:e2e`, `go build -o dist/stuarty-e2e ./web/cmd/app`, and `go build -o dist/desktop-detect ./desktop/detect`, then uploads `web/resources/public/build` and `dist/` as artifacts.
   - Make `browser` and `browser-engines` `needs: [e2e-build]`, download those artifacts, remove the redundant `bun run build` / `bun run build:e2e` from all 18 shards, set `STUARTY_E2E_BIN` to the prebuilt binary (and support `STUARTY_DETECT_BIN` in `playwright.config.ts:416`), and invoke `bunx playwright test -c e2e/playwright.config.ts ...` directly instead of `bun run e2e`.
4. **Add pinned `actions/cache` steps**:
   - Check the latest pinned SHA for `actions/cache` (note: do NOT invent a SHA — if offline, look up the real release commit SHA before adding, or stop and report as a STOP condition).
   - Cache FFmpeg download dir keyed on `.config/ffmpeg.env`, `~/.bun/install/cache` keyed on `web/bun.lock`, and `~/.cache/ms-playwright` keyed on `web/bun.lock` + browser name.

**Verify**:

- Run the exact `go list` shard partition commands locally and assert the union of `export`, `engine`, `service`, `contract`, `web-app`, and `core-service` equals `go list ./...` with zero missing packages.
- `task lint:go && task vet` -> exit 0.

## Test plan

- `web/app/store/template_test.go`:
  - `TestOpenTemplateMatchesFreshSchema`: asserts identical `sqlite_master` entries and `migrations` rows vs raw `store.Open`.
  - `TestOpenTemplateIsolatesInstances`: asserts parallel workspaces opened from template do not share state or WAL files.
- Verify Go shard partition script locally against `go list ./...`.

## Done criteria

- [x] `go test -race -count=1 ./web/app/store/...` exits 0 including template tests
- [x] `go test -race -count=1 -run 'TestProject|TestApproval' ./web/app/servicetest/` exits 0
- [x] `go test -race -count=1 -run 'TestHTTP' ./web/contract/` exits 0
- [x] `cd web && bun run typecheck && bun run lint` exits 0
- [x] Local set-equality check proves `ci.yml` shard `go list` rules cover 100% of `go list ./...`
- [x] `shards` count in `ci.yml` `test` job matches the exact number of `test-shard` matrix rows per OS
- [x] Every added GitHub Action in `.github/workflows/ci.yml` is pinned to a real 40-char commit SHA
- [x] `task lint:go && task vet` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `store.Open` writes workspace-specific absolute paths or random IDs/salts into `db.sqlite` during migration — if any migration embeds per-workspace state, that migration cannot be templated blindly.
- You cannot verify the authentic 40-char commit SHA of `actions/cache` (never guess or fabricate an action SHA).
- Splitting `rest` changes a required branch-protection check display name (keep `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)`, and `Browser tests (Playwright)` names unchanged!).

## Maintenance notes

- Whenever `test-shard` matrix rows change in `.github/workflows/ci.yml`, `EXPECTED_SHARDS` in the `test` aggregate job (`ci.yml:313-315`) MUST be updated in lockstep.
- Any new migration added to `web/database/migrations/` is automatically included in the `sync.Once` template DB because `OpenTemplate` runs real `store.Open` once per test binary.
