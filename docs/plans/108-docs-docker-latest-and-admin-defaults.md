# 108: Documentation and website Docker `:latest` image and simplified default admin credentials

## Goal

Every Docker command in Stuarty's documentation, README, and website guides uses `ghcr.io/alexgalhardo/stuarty:latest` (instead of placeholder `:X.Y.Z` that fails when copy-pasted) and uses matching, memorable default example credentials (`STUARTY_ADMIN_EMAIL=admin@stuarty.com` and `STUARTY_ADMIN_PASSWORD="admin@stuarty.com"`) so anyone trying Stuarty with Docker can copy, paste, run, and sign in immediately on the first try.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #8**:
    1. Stuarty's documentation (`docs/` and the web site `site/` primarily) must always instruct users to pull/run `ghcr.io/alexgalhardo/stuarty:latest` rather than `ghcr.io/alexgalhardo/stuarty:X.Y.Z`.
    2. Make the documented example admin email and password equal:

       ```bash
       STUARTY_ADMIN_EMAIL=admin@stuarty.com \
       STUARTY_ADMIN_PASSWORD="admin@stuarty.com" \
       ```

- Codebase verification:
  - `web/app/auth/users.go:23` sets `MinPasswordLength = 12`. `"admin@stuarty.com"` is 17 characters long, so `BootstrapAdmin` (`web/app/auth/bootstrap.go`) already accepts `STUARTY_ADMIN_EMAIL=admin@stuarty.com` and `STUARTY_ADMIN_PASSWORD="admin@stuarty.com"` without weakening any backend password-length validation!
  - Current files showing `:X.Y.Z` or `admin@example.com` / `"choose-a-long-password"` include:
    - `site/content/docs/en/getting-started/install-docker.mdx`
    - `site/content/docs/en/self-hosting/docker.mdx`
    - `site/content/docs/en/self-hosting/backups-and-upgrades.mdx`
    - `site/content/docs/en/self-hosting/network-mode.mdx`
    - `site/lib/latest-release.ts`
    - `docs/guides/pt-BR/comecar.md`
    - `deploy/.env.example` and `deploy/README.md` / `deploy/README.pt-BR.md`
    - `README.md` (verify all Docker snippets)
  - Note on AGENTS.md rule 9: base images inside `Dockerfile`s and CI build images stay pinned to exact version tags/digests; this epic updates the **user-facing documentation and quick-start examples** to `ghcr.io/alexgalhardo/stuarty:latest` and adds a short one-line note reminding production operators to change the example password and optionally pin a version tag on a public VPS.

## Scope

- Modules owned:
  - `site/content/docs/en/getting-started/install-docker.mdx`
  - `site/content/docs/en/self-hosting/docker.mdx`, `backups-and-upgrades.mdx`, `network-mode.mdx`
  - `site/lib/latest-release.ts` and `site/scripts/latest-release.test.ts`
  - `docs/guides/pt-BR/comecar.md` (and any other guide in `docs/guides/` referencing `ghcr.io/alexgalhardo/stuarty:X.Y.Z`)
  - `deploy/.env.example`, `deploy/README.md`, `deploy/README.pt-BR.md`
  - `README.md`
  - `web/app/auth/bootstrap_test.go` (add a unit test confirming `STUARTY_ADMIN_EMAIL=admin@stuarty.com` + `STUARTY_ADMIN_PASSWORD=admin@stuarty.com` bootstraps cleanly)
- Must not touch: `Dockerfile` base image pins, `.github/workflows/`, `docs/sdds/007-security-model.md` cosign digest verification examples (`verify-a-release.mdx` keeps `$V` / `@sha256:` for cryptographic release verification), `core/`, `engine/`, `renderer/`.

## Out of scope

- Changing local-account password strength rules (`core/account/rules.go` remains unchanged for encrypted local workspaces).
- Unpinning base images in `Dockerfile` or CI workflows.

## Dependencies

- Epics 25, 40, and 87 (VPS package, documentation site, site design).

## Technical design

1. **Docs & website Docker snippet standardization**:
   - Replace every user-facing quick-start / run / pull / compose example of `ghcr.io/alexgalhardo/stuarty:X.Y.Z` with `ghcr.io/alexgalhardo/stuarty:latest` (keep cryptographic verification examples in `verify-a-release.mdx` using `$V` / `@sha256:` intact since those verify a specific signed release).
   - Standardize the example environment variables across `site/lib/latest-release.ts`, `site/content/docs/en/getting-started/install-docker.mdx`, `site/content/docs/en/self-hosting/docker.mdx`, `docs/guides/pt-BR/comecar.md`, and `README.md` to:

     ```bash
     -e STUARTY_ADMIN_EMAIL=admin@stuarty.com \
     -e STUARTY_ADMIN_PASSWORD="admin@stuarty.com" \
     ghcr.io/alexgalhardo/stuarty:latest
     ```

   - Keep a concise one-line production reminder underneath ("Change the example password before exposing the container outside `127.0.0.1`.") so security guidance stays honest without complicating the quick-start command.

2. **Automated guard**:
   - Add a check in `site/scripts/latest-release.test.ts` verifying that `dockerCommand` emits `ghcr.io/alexgalhardo/stuarty:latest`, `STUARTY_ADMIN_EMAIL=admin@stuarty.com`, and `STUARTY_ADMIN_PASSWORD="admin@stuarty.com"`, and that no quick-start doc page under `site/content/docs/` contains the unrunnable placeholder `ghcr.io/alexgalhardo/stuarty:X.Y.Z`.
   - Add a Go unit test in `web/app/auth/` confirming `BootstrapAdmin` succeeds when `STUARTY_ADMIN_EMAIL` and `STUARTY_ADMIN_PASSWORD` are both `admin@stuarty.com`.

## Tasks

- [x] 1. Audit every occurrence of `ghcr.io/alexgalhardo/stuarty:X.Y.Z`, `STUARTY_ADMIN_EMAIL`, and `STUARTY_ADMIN_PASSWORD` across `docs/`, `site/`, `deploy/`, and `README.md`, and update all quick-start and self-hosting pages to `:latest` and `admin@stuarty.com` / `"admin@stuarty.com"`. Verify: `git grep "ghcr.io/alexgalhardo/stuarty:X.Y.Z" -- docs/ site/ deploy/ README.md` returns 0 matches.
- [x] 2. Update `site/lib/latest-release.ts` and `site/scripts/latest-release.test.ts` (including a doc-lint assertion against `:X.Y.Z` in `site/content/docs/`). Verify: `cd site && bun test && bun run check:last-updated && bun run build`.
- [x] 3. Add a Go regression test in `web/app/auth/` verifying `BootstrapAdmin` creates the admin user cleanly when email and password are both `admin@stuarty.com`, and run `task lint`. Verify: `go test ./web/app/auth/...` and `task lint` exit 0.

## Test strategy

- Go unit test: `BootstrapAdmin` with `admin@stuarty.com` / `admin@stuarty.com` succeeds and signs in cleanly.
- Site test suite (`site/scripts/latest-release.test.ts`): asserts generated quick-start command uses `:latest` and matching `admin@stuarty.com` credentials, and scans `site/content/docs/` to prevent regressions back to `:X.Y.Z`.
- Full site build + `check:last-updated` + `task lint:md`.

## Acceptance criteria

1. Zero occurrences of `ghcr.io/alexgalhardo/stuarty:X.Y.Z` remain in `docs/`, `site/`, `deploy/`, or `README.md`; all user-facing Docker run/pull commands use `ghcr.io/alexgalhardo/stuarty:latest`.
2. All user-facing Docker run commands in `docs/`, `site/`, and `README.md` use `STUARTY_ADMIN_EMAIL=admin@stuarty.com` and `STUARTY_ADMIN_PASSWORD="admin@stuarty.com"`.
3. `BootstrapAdmin` test proves `admin@stuarty.com` / `admin@stuarty.com` starts and authenticates with zero errors.
4. `go test ./web/app/auth/...`, `cd site && bun test && bun run check:last-updated && bun run build`, and `task lint` exit 0.

## Risks and mitigation

- **Betterleaks / secret scanners flagging an example password.** Mitigation: verify `task lint` and Betterleaks do not false-positive on the example value `admin@stuarty.com` (it is a plain email string, not a high-entropy token); **STOP** if a secret scanner flags it and requires an allowlist entry in a protected CI check.
- **Stale `lastUpdated` frontmatter dates on edited MDX pages.** Mitigation: run `cd site && bun run last-updated` and `bun run check:last-updated` before committing.

## Size

S

## Hand-off for review

The commits of `epic/108-docs-docker-latest-and-admin-defaults`, grep verification, Go test output, and `site` test/build output.
