# Plan 001: Unblock PR #172 CI checks, restore missing test gates, and fix macOS git hooks

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- scripts/upgrade-test.sh service/upgrade_test.go .github/workflows/pr-title.yml .github/labels.txt docs/sdds/009-releases-and-ci-cd.md .github/workflows/ci.yml .github/workflows/nightly.yml web/scripts/check-thin-client.ts web/scripts/check-i18n.ts web/package.json Taskfile.yml scripts/gate.sh .githooks/pre-commit`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

Several core verification gates are either actively failing on PR #172 (`release/v0.60.0`) or silently passing without running their checks:

1. `upgrade / Upgrade (windows)` fails deterministically because `scripts/upgrade-test.sh:110` does not strip `\r` from `gh release view`, and `service/upgrade_test.go:700` has a broken regex (`d{14}` instead of `\d{14}`) that silently disables the cross-OS downgrade-refusal test on every OS.
2. `PR title / Conventional commits` fails on PR #172 because commit `36d4c8442` used scope `secrets`, which is missing from the allowlist, and `.githooks/` has no `commit-msg` hook to catch bad scopes locally before pushing.
3. `.github/workflows/ci.yml` never runs the 231 Vitest unit tests (`bun run test`), `check:thin-client`, `check:hooks`, or `web/app/agentdocs/out` drift detection; `.github/workflows/nightly.yml` runs soak tests without `-tags soak` and exits 0; `Taskfile.yml` and `scripts/gate.sh` omit `-coverpkg=./...` for `covergate`; and `.githooks/pre-commit` uses Bash 4 `mapfile`, breaking commits on macOS Bash 3.2.

Fixing these establishes a trustworthy verification baseline before executing security, bug-fix, and performance plans.

## Current state

- `scripts/upgrade-test.sh:105-116`:

```bash
	mkdir -p "$dl" "$binaries/$version"

	# A release can be published before its archives exist (v0.25.0 has notes and no binaries).
	# There is nothing to open, so the matrix continues with the releases that shipped this OS.
	if command -v gh >/dev/null 2>&1; then
		names="$(gh release view "$version" -R "$repo" --json assets -q '.assets[].name' || true)"
		if ! printf '%s\n' "$names" | grep -qx "$archive"; then
			log "$version: no $archive on the release, skipping"
			rm -rf "$dl"
			continue
		fi
	fi
```

- `service/upgrade_test.go:700`:

```go
var migrationID = regexp.MustCompile(`d{14}_[a-z0-9_]+`)
```

- `.github/workflows/pr-title.yml:40-92,119`: allowed scopes omit `secrets` (causing `36d4c8442 ci(secrets): allow example chat actor ids in Go tests` on `release/v0.60.0` to fail). Note `internal/tools/labelsync` tests that `.github/workflows/pr-title.yml`, `.github/labels.txt` (`area:<scope>`), and `docs/sdds/009-releases-and-ci-cd.md` §6.2 all agree (`.agents/rules/commits.md` rule 2).
- `.github/workflows/ci.yml:89-109`:

```yaml
  web:
    name: Types and lint (web)
    runs-on: ubuntu-latest
    timeout-minutes: 20
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: oven-sh/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6 # v2.2.0
        with:
          bun-version: ${{ env.BUN_VERSION }}
      - run: bun install --frozen-lockfile
      - run: bun run typecheck
      - run: bun run lint
      - run: bun run check:contrast
      - run: bun run check:i18n
      # `bun run test` is added with the unit test runner, which needs the
      # author's approval (SDD-012 section 3.1).
```

- `.github/workflows/ci.yml:437`: checks `core/schema web/resources/js/generated site/generated site/content/generated internal/ffmpegpins` but omits `web/app/agentdocs/out` (present in `Taskfile.yml:191`).
- `.github/workflows/nightly.yml:72-81`:

```yaml
      - name: Soak and Fixture5Min tests
        run: |
          go test -count=1 -v -run 'Soak|Fixture5Min' -timeout=15m ./... | tee soak.txt
          {
            echo '### Soak and 5-minute fixture'
            echo '```'
            grep -E -- "^(--- |ok|FAIL)" soak.txt | grep -v "no test files" || true
            echo '```'
            if ! grep -q -- "--- PASS:" soak.txt; then echo "No soak or fixture test exists yet."; fi
          } >> "$GITHUB_STEP_SUMMARY"
```

- `Taskfile.yml:112-117` and `scripts/gate.sh:46`: run `go test ... -coverprofile=... ./...` without `-coverpkg=./...`, whereas `.github/workflows/ci.yml:231,245,251` passes `-coverpkg=./...`.
- `.githooks/pre-commit:15-17`:

```bash
mapfile -t go_files < <(staged '*.go')
mapfile -t md_files < <(staged '*.md')
mapfile -t web_files < <(staged 'web/resources/js' 'web/e2e')
```

- Conventions:
  - Build scripts must work on macOS Bash 3.2 (`.agents/rules/ci.md` rule 9; see `scripts/upgrade-test.sh:59-65` for the `while IFS= read -r` pattern).
  - External GitHub Actions must stay pinned to exact 40-char SHAs (copy `actions/setup-node` SHA from `.github/workflows/site.yml:68`).
  - Any Node fallback in `web/` or `site/` must carry a D-115 comment explaining why Bun fails (see `site/scripts/api-reference.test.ts:1`).
  - Note `REVIEW.md` rule 9: workflow/check edits belong in their own PR/commit separate from product feature code.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Label/scope sync test | `go test -count=1 ./internal/tools/labelsync/...` | `ok`, exit 0 |
| Upgrade unit test | `go test -count=1 -run TestMigrationsIn ./service/...` | `PASS`, exit 0 |
| Commit check script | `bash .agents/skills/ci-preflight/scripts/check-commits.sh .` | exit 0, no `BAD` lines |
| Go lint | `task lint:go` | exit 0 |
| Web checks & unit tests | `cd web && bun install --frozen-lockfile && bun run typecheck && bun run lint && bun run check:thin-client && bun run check:i18n && bun run test` | exit 0 |
| Shell syntax check | `bash -n scripts/upgrade-test.sh .githooks/pre-commit .githooks/commit-msg scripts/gate.sh` | exit 0 |
| Workflow pin test | `go test -count=1 ./internal/tools/...` | `ok`, exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `scripts/upgrade-test.sh`
- `service/upgrade_test.go`
- `.github/workflows/pr-title.yml`
- `.github/labels.txt`
- `docs/sdds/009-releases-and-ci-cd.md`
- `.github/workflows/ci.yml`
- `.github/workflows/nightly.yml`
- `web/scripts/check-thin-client.ts`
- `web/scripts/check-i18n.ts`
- `web/package.json`
- `Taskfile.yml`
- `scripts/gate.sh`
- `.githooks/pre-commit`
- `.githooks/commit-msg` (create)

**Out of scope** (do NOT touch):

- `ci.yml` test sharding, SQLite template DB, and Playwright build caching (handled by `improve/002-eliminate-ci-timeouts-sqlite-template-and-caching.md`).
- Adding `aihelper/` and `capture/` Rust CI jobs (handled by `improve/012-repair-critical-docs-drift-and-close-rust-security-ci-gaps.md`).
- Any product runtime code under `core/`, `engine/`, `service/*.go` (non-test), `web/app/`, or `web/resources/js/`.
- Any new dependency in `go.mod` or `web/package.json`.

## Git workflow

- Branch: `fix/ci-unblock-and-gates` cut from `release/v0.60.0` (in a sibling worktree if using the team workflow).
- Commit per logical step using Conventional Commits with an allowed scope (e.g., `ci(upgrade): strip CR in upgrade-test.sh and fix migrationID regex`).
- Do NOT push or merge to `main`.

## Steps

### Step 1: Fix Windows CRLF handling in `scripts/upgrade-test.sh` and the broken regex in `service/upgrade_test.go`

1. In `scripts/upgrade-test.sh:105-116`:
   - Pipe `gh release view` through `tr -d '\r'` (`names="$(gh release view "$version" -R "$repo" --json assets -q '.assets[].name' | tr -d '\r' || true)"`).
   - Move `mkdir -p "$binaries/$version"` to AFTER the `grep -qx "$archive"` check (around line 117), so skipped releases do not leave empty directories under `$binaries` that fool `find "$binaries" -mindepth 1 -type d` at line 147.
2. In `service/upgrade_test.go:700`:
   - Change `` var migrationID = regexp.MustCompile(`d{14}_[a-z0-9_]+`) `` to `` var migrationID = regexp.MustCompile(`\d{14}_[a-z0-9_]+`) ``.
   - Add a small unit test `TestMigrationsInFindsRealIDs` in `service/upgrade_test.go` that writes a synthetic `db.sqlite` file containing `"20261119140000_create_approval_tables"` into `t.TempDir()`, calls `migrationsIn(t, dir)`, and asserts the map contains `"20261119140000_create_approval_tables"`.

**Verify**: `bash -n scripts/upgrade-test.sh && go test -count=1 -v -run TestMigrationsIn ./service/` -> `PASS`, exit 0.

### Step 2: Add `secrets` to commit scopes in lockstep and add `.githooks/commit-msg`

1. Add `secrets` to all three synchronized scope lists:
   - `.github/workflows/pr-title.yml` (both the `scopes:` block at lines 40–92 and the `scopes='...'` regex at line 119).
   - `.github/labels.txt` (add `area:secrets` in the same format as adjacent `area:` entries).
   - `docs/sdds/009-releases-and-ci-cd.md` §6.2 scope list.
2. Create `.githooks/commit-msg` (executable, `chmod +x`, Bash 3.2 compatible) that validates the first line of the commit message file (`$1`) against the exact Conventional Commits type/scope/length (`<= 100`) pattern in `.github/workflows/pr-title.yml`, skipping `Merge *`, `Revert "*"`, and `Potential fix for *`.

**Verify**: `go test -count=1 ./internal/tools/labelsync/... && bash .agents/skills/ci-preflight/scripts/check-commits.sh . && bash -n .githooks/commit-msg` -> all exit 0 with zero `BAD` commits.

### Step 3: Restore missing web gates and `web/app/agentdocs/out` check in `.github/workflows/ci.yml`

1. Inspect `web/scripts/check-thin-client.ts` for `Bun.Glob` (around lines 354 and 407) and replace it with standard Node `fs.readdirSync(..., { recursive: true })` so it runs cleanly under `node` (avoiding the Linux Bun `typescript/unstable/sync` crash documented in commit `f0de9153f`).
2. Add a concise D-115 comment at line 1 of both `web/scripts/check-i18n.ts` and `web/scripts/check-thin-client.ts` explaining why Node is used (matching `site/scripts/api-reference.test.ts:1`). Update `web/package.json` `"check:thin-client"` to `"node scripts/check-thin-client.ts"`.
3. In `.github/workflows/ci.yml`:
   - In the `web` job (lines 89–109), add `actions/setup-node` using the exact pinned SHA from `.github/workflows/site.yml` (`node-version: "24"`), remove the obsolete comment on lines 108–109, and add steps:
     - `bun run check:thin-client`
     - `bun run test`
     - `bun run check:hooks` (run after `bun run build` in this job or ensure `check:hooks` builds/inspects the bundle — read `web/scripts/check-hooks.ts` first to confirm whether it requires `bun run build` beforehand; if it does, run `bun run build` before `bun run check:hooks`).
   - In the `generated` job (`ci.yml:437`), add `web/app/agentdocs/out` to the `for d in ...` loop so it matches `Taskfile.yml:191`.

**Verify**: `cd web && bun install --frozen-lockfile && bun run typecheck && bun run lint && bun run check:thin-client && bun run check:i18n && bun run test` -> all exit 0.

### Step 4: Fix nightly soak job, local `covergate` parity, and macOS Bash 3.2 `.githooks/pre-commit`

1. In `.github/workflows/nightly.yml:74-81`:
   - Add `-tags soak` and `STUARTY_SOAK_DURATION=5m` to the `go test` invocation (`STUARTY_SOAK_DURATION=5m go test -tags soak -count=1 -v -run 'TestSoak' -timeout=15m ./web/app/servicetest/ | tee soak.txt`).
   - Change line 80 so that if `! grep -q -- "--- PASS:" soak.txt`, the step fails (`exit 1`) instead of silently printing `"No soak or fixture test exists yet."` and succeeding.
2. In `Taskfile.yml:115` and `scripts/gate.sh:46`:
   - Add `-coverpkg=./...` to `go test` before invoking `covergate`, and remove `-short` in `scripts/gate.sh:46` so local release-gate coverage matches `.github/workflows/ci.yml`.
3. In `.githooks/pre-commit:15-17`:
   - Replace the three `mapfile -t` calls with Bash 3.2-compatible `while IFS= read -r` loops (matching `scripts/upgrade-test.sh:60-65`) and expand arrays safely under `set -u` (`${arr[@]+"${arr[@]}"}`).

**Verify**: `bash -n .githooks/pre-commit scripts/gate.sh && go test -tags soak -list 'TestSoak' ./web/app/servicetest/ | grep -q '^TestSoak$'` -> exit 0.

## Test plan

- `TestMigrationsInFindsRealIDs` in `service/upgrade_test.go` (modeled on existing tests in `service/upgrade_test.go`) verifying `migrationID` matches real `YYYYMMDDHHMMSS_<name>` migration identifiers.
- Existing `internal/tools/labelsync` tests verifying lockstep sync of `pr-title.yml`, `.github/labels.txt`, and `docs/sdds/009-releases-and-ci-cd.md`.
- Existing 231 Vitest unit tests (`cd web && bun run test`) + `check:thin-client` + `check:i18n` + `check:hooks`.
- Dry-run test of `.githooks/commit-msg` on one valid and one invalid commit message in a temp file.

## Done criteria

- [x] `bash -n scripts/upgrade-test.sh .githooks/pre-commit .githooks/commit-msg scripts/gate.sh` exits 0
- [x] `grep -n 'mapfile' .githooks/pre-commit` returns no matches
- [x] `go test -count=1 -run TestMigrationsIn ./service/` exits 0
- [x] `go test -count=1 ./internal/tools/labelsync/...` exits 0
- [x] `bash .agents/skills/ci-preflight/scripts/check-commits.sh .` exits 0 with no `BAD` lines
- [x] `cd web && bun run typecheck && bun run lint && bun run check:thin-client && bun run check:i18n && bun run test` exits 0
- [x] `grep -n 'web/app/agentdocs/out' .github/workflows/ci.yml` returns a match in the `generated` job
- [x] `grep -n -- '-tags soak' .github/workflows/nightly.yml` returns a match
- [x] `task lint:go && task lint:md` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any of the 231 Vitest tests or `bun run check:thin-client` or `bun run check:hooks` fails on clean HEAD — report the exact failing tests/rules instead of weakening checks or editing product code.
- `check:hooks` requires a full production Vite build that takes >3 minutes or fails in CI conditions — note it and decide whether `check:hooks` belongs in the `web` job or a build job.
- `TestUpgradeOlderBinaryRefusesCandidate` cannot be run locally without `gh` auth (that is expected; unit-test `migrationsIn` locally and note full upgrade matrix runs in CI).
- A check in `.github/workflows/ci.yml` fails `Workflow pins` (every added action MUST use an exact 40-char SHA).

## Maintenance notes

- Keep `.githooks/commit-msg` regex synchronized with `.github/workflows/pr-title.yml` whenever a new scope is added to `pr-title.yml`, `.github/labels.txt`, and `docs/sdds/009-releases-and-ci-cd.md`.
- Once `scripts/upgrade-test.sh` passes on Windows and `TestUpgradeOlderBinaryRefusesCandidate` actually executes against real older releases, watch the first CI run closely in case any older release binary has a genuine downgrade-refusal defect that was previously masked by the broken regex.
