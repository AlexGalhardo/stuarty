# Plan 012: Repair critical docs drift (ADR-0052/0053, ADR-0048, desktop build) and close Rust/Dependabot CI gaps

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- site/content/docs/en/concepts/local-and-network-mode.mdx site/content/docs/en/guide/desktop.mdx site/content/docs/en/getting-started/install-windows.mdx site/content/docs/en/self-hosting/binary.mdx site/content/docs/en/self-hosting/security.mdx site/content/docs/en/guide/memory-skills-prompts.mdx site/content/docs/en/guide/accessibility.mdx .github/CONTRIBUTING.md AGENTS.md docs/plans/00-roadmap.md docs/README.md desktop/README.md .github/dependabot.yml .github/workflows/vuln.yml .github/workflows/ci.yml aihelper/deny.toml renderer/stuarty-renderer-gpu/tests/support/mod.rs`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: `improve/001-unblock-pr172-ci-and-restore-missing-gates.md`
- **Category**: docs
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

This plan fixes actively misleading documentation and closes remaining CI/security monitoring blind spots:

1. **Dangerous backup & login misinformation on the live docs site**: 7 `.mdx` pages still tell users "local mode has no login", "projects and notes are plain unencrypted files", and "losing the key won't lose projects" — directly contradicting ADR-0052 & ADR-0053 (mandatory password account, sealed documents/library, in-memory library index). Additionally, `site/content/docs/en/guide/desktop.mdx:29` gives a build command that crashes immediately with `E_INVALID_STATE` on `~/.stuarty`, and `install-windows.mdx:48` contains a literal ASCII `0x0C` form-feed byte in `` `licenses\x0cffmpeg-build` ``.
2. **Stale contributor/agent guides**: `.github/CONTRIBUTING.md:5` contains a mangled rename sentence (*"The product was called Stuarty until 2026-11-18..."*), `AGENTS.md:130` tells agents to diff against frozen `release/0.30.0`, `docs/plans/00-roadmap.md` still shows finished Epics 11/49 and accepted ADRs 0049–0051 as pending, and `desktop/README.md` documents only 1 of 8 bridge operations.
3. **Untested Rust/Desktop/Dependabot surfaces**: `aihelper/`, `capture/`, and `desktop/shell/app` tests never run in CI or `gate.sh`; `aihelper/deny.toml` lacks `[advisories]` and `allow-registry`; `renderer/stuarty-renderer-gpu` silently skips all 6 GPU parity tests on CI runners without `mesa-vulkan-drivers`; and `.github/dependabot.yml` + `.github/workflows/vuln.yml` do not scan `site/`, `desktop/shell/app`, Cargo, or Docker.

## Current state

- `site/content/docs/en/concepts/local-and-network-mode.mdx:14-15`: states `"There is no login"` in local mode (compare canonical `site/content/docs/en/getting-started/account.mdx:15,53-57`).
- `site/content/docs/en/getting-started/install-windows.mdx:19,48`: line 19 says `"it signs you in without a password"`; line 48 has a literal `0x0C` byte inside `` `licenses<0x0c>ffmpeg-build` ``.
- `.github/CONTRIBUTING.md:5,33,43-48,82,106`: mangled line 5, outdated scope list, pre-ADR-0048 single draft PR wording, and `"Rust lives only in renderer/"`.
- `AGENTS.md:130,132`: references `--base release/0.30.0` and omits `capture/` from the repo tree and Rust gate.
- `.github/dependabot.yml:1-41`: monitors only `/` (`gomod`, `github-actions`) and `/web` (`bun`).
- `aihelper/deny.toml:1-27`: lacks `[advisories]` and `allow-registry = ["https://github.com/rust-lang/crates.io-index"]` (compare `renderer/deny.toml:41-44` and `capture/deny.toml:28-31`).
- `renderer/stuarty-renderer-gpu/tests/support/mod.rs:15-30`: returns `None` silently on `GpuError::NoAdapter`, making all 6 GPU parity tests return `Ok` without executing.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Markdown lint | `task lint:md` | exit 0 |
| Site checks & build | `cd site && bun install --frozen-lockfile && bun run check:mdx && bun run build && bun run check:built && bun run test:site` | exit 0 |
| Refresh site last-updated | `cd site && bun run last-updated && bun run check:last-updated` | exit 0 |
| Notices & Rust pin test | `go test -count=1 ./internal/tools/notices/...` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):

- `site/content/docs/en/concepts/local-and-network-mode.mdx`
- `site/content/docs/en/guide/desktop.mdx`
- `site/content/docs/en/getting-started/install-windows.mdx`
- `site/content/docs/en/self-hosting/binary.mdx`
- `site/content/docs/en/self-hosting/security.mdx`
- `site/content/docs/en/guide/memory-skills-prompts.mdx`
- `site/content/docs/en/guide/accessibility.mdx`
- `site/content/docs/en/contributing/guide.mdx`
- `site/content/docs/en/contributing/repository-layout.mdx`
- `site/content/docs/en/contributing/renderer.mdx`
- `site/content/docs/en/contributing/commits-and-prs.mdx`
- `site/content/last-updated.json` (regenerated via `bun run last-updated`)
- `.github/CONTRIBUTING.md`
- `AGENTS.md`
- `desktop/README.md`
- `docs/README.md`
- `docs/plans/00-roadmap.md`
- `docs/prds/003-distribution-and-docs.md`
- `.agents/rules/commits.md`
- `.github/dependabot.yml`
- `.github/workflows/vuln.yml`
- `.github/workflows/ci.yml`
- `.github/workflows/desktop-detect.yml`
- `aihelper/deny.toml`
- `renderer/stuarty-renderer-gpu/tests/support/mod.rs`
- `internal/tools/notices/renderer_test.go`
- `Taskfile.yml`
- `scripts/gate.sh`

**Out of scope** (do NOT touch):

- Product runtime Go/TS code under `core/`, `service/`, `web/app/`, `web/resources/js/`.
- Portuguese docs under `docs/guides/pt-BR/` (separate localization pass).
- Generated API/MCP reference pages (`site/content/generated/`).

## Git workflow

- Branch: `docs/repair-drift-and-close-ci-gaps` cut from `release/v0.60.0` (after Plan 001).
- Split into two clean commits:
  1. `docs(site): align local-account, encryption, desktop, and contributor docs with ADR-0048..0053`
  2. `ci(repo): add aihelper, capture, desktop shell, GPU adapter, and Dependabot/vuln checks`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Align live docs site pages with ADR-0052, ADR-0053, and real desktop/contributing setup

1. Read `site/content/docs/en/getting-started/account.mdx` first to match the canonical wording for local password accounts, recovery keys, and workspace encryption.
2. Update the 7 user-facing `.mdx` pages:
   - `local-and-network-mode.mdx`: replace "There is no login" with the local-account sign-in and recovery-key model (ADR-0052), noting how CLI/MCP local token works alongside the unlocked studio.
   - `guide/desktop.mdx`: remove "no sign-in is needed" and D-117 privilege claim; fix the `go build` snippet at lines 29–34 to set `STUARTY_HOME` (or note `-ldflags "-X main.version=..."` matching `desktop/README.md:40-43`) so a dev build does not crash on `~/.stuarty` with `E_INVALID_STATE`.
   - `getting-started/install-windows.mdx`: fix line 19 ("signs you in without a password") and replace the literal `0x0C` form-feed byte on line 48 with `` `licenses\ffmpeg-build` ``.
   - `self-hosting/binary.mdx`: update line 23 to match local-account sign-in.
   - `self-hosting/security.mdx`: align vault master-key wrapping, encrypted documents/projects/library, and backup/recovery-key warnings with ADR-0052 & ADR-0053.
   - `guide/memory-skills-prompts.mdx`: update line 20 so it reflects the in-memory search index (ADR-0053 item 3) rather than an on-disk index file.
   - `guide/accessibility.mdx`: update line 31 ("Local mode has no sign-in") under WCAG 3.3.8.
3. Update the 4 contributing `.mdx` pages (`guide.mdx`, `repository-layout.mdx`, `renderer.mdx`, `commits-and-prs.mdx`): reflect the Rust WASM renderer + `capture/` + `aihelper/` + `deploy/`, remove the bogus `wasm-bindgen-cli` install instruction, list all approved crates from `.agents/rules/rust.md`, update allowed scopes (including `upgrade`, `servicetest`, `auth`, `secrets`), and replace phase-era `Refs: P1-ENG-003` with `Refs: epic NN task N`.
4. Run `cd site && bun run last-updated` so `site/content/last-updated.json` passes `check:last-updated`!

**Verify**: `cd site && bun install --frozen-lockfile && bun run check:mdx && bun run check:last-updated && bun run build && bun run check:built && bun run test:site` -> all pass, exit 0.

### Step 2: Repair contributor, agent, desktop, PRD, and roadmap docs

1. `.github/CONTRIBUTING.md`: fix the mangled line 5 ("The product was called Vukem Video until 2026-11-18 and was renamed to Stuarty in Epic 02 (ADR-0044)"), update the scope list at line 33, update lines 43–48 & 106 to describe the ADR-0048 release train, fix line 82 to mention `renderer/`, `capture/`, and `aihelper/`, and add a brief "Running locally" section (`go run ./web/cmd/app serve` + `cd web && bun run dev`).
2. `AGENTS.md`: add `capture/` and `deploy/` to the repo tree (lines 25–34), update `--base release/0.30.0` (line 130) to `--base release/vX.Y.Z`, and add `capture/` to the Rust gate row (line 132).
3. `.agents/rules/commits.md`: remove stale "while no release is cut" phrasing.
4. `desktop/README.md`: list all 8 bridge operations from `desktop/shell/bridge.go:16-30` and replace `P3-DSK-001` with Epic 43.
5. `docs/README.md`: add ADR-0049, ADR-0050, ADR-0051, SDD-016, and SDD-019 in numerical order, and fix "up to release 0.29.0".
6. `docs/plans/00-roadmap.md` & `docs/prds/003-distribution-and-docs.md`: update `00-roadmap.md` header (ADR-0048 train instead of PR #106), mark completed Epics 11/49, clear already-implemented Epic 56/58 notes, mark ADR-0049..0051 Accepted, and add ADR-0052/ADR-0048 amendment notes to PRD-003 RF-202, RF-209, and RF-217.

**Verify**: `task lint:md` -> exit 0.

### Step 3: Close `aihelper/`, `capture/`, `desktop/shell/app`, GPU parity, Dependabot, and `vuln.yml` blind spots

1. `aihelper/deny.toml`: add `allow-registry = ["https://github.com/rust-lang/crates.io-index"]` and `[advisories]` matching `renderer/deny.toml:41-44` and `capture/deny.toml:28-31`.
2. `internal/tools/notices/renderer_test.go:55-61`: generalize `TestRendererToolchainIsPinned` to inspect all five `Cargo.toml` files (`renderer/stuarty-renderer`, `renderer/stuarty-renderer-gpu`, `renderer/stuarty-renderer-text`, `aihelper`, `capture`) and parse inline-table dependencies (`version = "=..."`) in addition to bare `crate = "=..."` lines.
3. `.github/workflows/ci.yml`, `Taskfile.yml`, `scripts/gate.sh`, `.github/workflows/desktop-detect.yml`:
   - Add Rust gate jobs in `ci.yml` (and tasks in `Taskfile.yml` + steps in `scripts/gate.sh`) for `aihelper/` and `capture/` (`cargo fmt --check`, `cargo clippy --locked --all-targets -- -D warnings`, `cargo test --locked`, `cargo deny check`; run `capture/` on both `ubuntu-24.04` and `windows-latest`).
   - In `.github/workflows/ci.yml` `renderer` job on `ubuntu-24.04`: install `mesa-vulkan-drivers` so `stuarty-renderer-gpu` tests execute against lavapipe instead of skipping silently; in `renderer/stuarty-renderer-gpu/tests/support/mod.rs:15-30`, panic instead of returning `None` when `std::env::var("CI").is_ok()` and no adapter is found.
   - In `.github/workflows/desktop-detect.yml` (`shell-build`) and `scripts/gate.sh`: add `go test -tags purego -count=1 ./...` in `desktop/shell/app`.
4. `.github/dependabot.yml` & `.github/workflows/vuln.yml`:
   - Add `bun` (`/site`), `gomod` (`/desktop/shell/app`), `cargo` (`/renderer`, `/aihelper`, `/capture`), `docker` (`/.config/docker`), and `docker-compose` (`/deploy`) to `.github/dependabot.yml`.
   - Extend `.github/workflows/vuln.yml` to run `govulncheck` in `desktop/shell/app`, `bun audit` in `site/`, and `cargo deny check advisories` across `renderer/`, `aihelper/`, and `capture/`.

**Verify**: `go test -count=1 ./internal/tools/notices/... && (cd desktop/shell/app && go test -tags purego -count=1 ./...) && task lint:go && task lint:md` -> all pass, exit 0.

## Test plan

- `site/` full verification (`check:mdx`, `check:last-updated`, `build`, `check:built`, `test:site`).
- `task lint:md` across all modified Markdown files.
- `go test -count=1 ./internal/tools/notices/...` proving all 5 `Cargo.toml` manifests enforce exact pins.
- `cd desktop/shell/app && go test -tags purego -count=1 ./...`.
- If `cargo` is installed: `cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test && cargo deny check` in `renderer/`, `aihelper/`, and `capture/`.

## Done criteria

- [x] `grep -rn 'There is no login\|signs you in without a password\|The product was called Stuarty' site/content/docs/en/ .github/CONTRIBUTING.md` returns no matches
- [x] `python3 -c 'data = open("site/content/docs/en/getting-started/install-windows.mdx", "rb").read(); assert b"\x0c" not in data'` exits 0
- [x] `cd site && bun run check:mdx && bun run check:last-updated && bun run build && bun run check:built && bun run test:site` exits 0
- [x] `go test -count=1 ./internal/tools/notices/...` exits 0
- [x] `cd desktop/shell/app && go test -tags purego -count=1 ./...` exits 0
- [x] `task lint:go && task lint:md && task vet` exits 0
- [x] No files outside the in-scope list are modified (`git status`)
- [x] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `site/scripts/check-doc-claims.test.ts` asserts specific old wording in any `.mdx` file you updated — read the test assertion to verify whether the test itself was asserting pre-ADR-0052 behavior before updating it.
- Any crate in `renderer/`, `aihelper/`, or `capture/` fails `TestRendererToolchainIsPinned` or `cargo deny check` — report the exact crate/license instead of loosening `deny.toml` (remember `.agents/rules/rust.md` item 2: `ort =2.0.0-rc.15` is an approved exception D-141, so allow that single exact `=` pin!).

## Maintenance notes

- Whenever a new ADR supersedes user-facing authentication, storage, or backup behavior, grep `site/content/docs/en/` for contradictory claims in the same PR.
- Reviewer focus: confirm `site/content/last-updated.json` was refreshed via `bun run last-updated` and not edited by hand.
