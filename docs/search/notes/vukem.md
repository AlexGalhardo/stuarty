# Research: vukem conventions (primary source)

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).
>
> **Corrections after the architect's review (2026-11-14).**
>
> 1. Module versions: the versions listed below come from the GitHub Releases page, which stopped listing module releases on 2026-10-18. The git tags are newer: `drivers/websocket` is at `v0.1.6` and `contrib/engine` at `v0.1.1` (checked with `git tag -l` in a full clone). Read every `drivers/websocket v0.1.1` below as `v0.1.6`. This gap is recorded as VF-008.
> 2. Section 2.9 and friction F17 describe `web/` as a git repository with no commits. That was a transient state of the workspace during the research. The workspace was reorganized afterwards: the repository is `C:\dev\vukem-video\video\`, the old Studio tree is in `video\web\`, and the vukem clone is in `C:\dev\vukem-video\vukem\`. Paths written below as `C:\dev\vukem-video\web` mean `C:\dev\vukem-video\video\web`.
> 3. The framework frictions that were confirmed are recorded as VF-001 to VF-010 in `vukem-feedback.md` at the workspace root. Frictions F1 to F18 below are the raw list; F12, F13, F14, F16, F17 and F18 were not turned into entries (not reproduced, about the Studio fork rather than vukem, or not about vukem at all).

All sources below accessed 2026-11-14. Sources: clone `--depth 50` of `https://github.com/AlexGalhardo/vukem` (HEAD `f8fefef`, session scratchpad), `gh release list/view -R AlexGalhardo/vukem`, `gh api`, the real app `C:\dev\vukem-video\web` (read-only), `https://vukem.dev/docs/introduction` and `https://www.freecut.net/docs`.

Labels: **FACT** = verified in the source; **INFERENCE** = deduction (explicit reasoning); **OPINION** = my judgment.

## Index

1. [Releases](#1-releases)
2. [Repository conventions](#2-repository-conventions)
3. [Structure of a vukem app and capability matrix](#3-structure-of-a-vukem-app-and-capability-matrix)
4. [Docs site](#4-docs-site)
5. [Friction](#5-friction)
6. [Gaps in this research](#6-gaps-in-this-research)

---

## 1. Releases

### 1.1 Versioning scheme and cadence

| Item | Finding | Label / source |
| --- | --- | --- |
| Scheme | SemVer 2.0.0, tag `vMAJOR.MINOR.PATCH`; pre-1.0 ("v0.x: minor may break, patch never") | FACT: `docs/tooling.md#versioning` (<https://github.com/AlexGalhardo/vukem/blob/main/docs/tooling.md>) |
| Current state | `v0.43.0` (2026-11-12). 92 releases on GitHub (`gh release list --limit 300`): 83 in 2026-10 and 9 in 2026-11 | FACT: `gh release list` |
| Cadence | Very high: 32 releases on 2026-10-16 alone; then ~1 to 2 per week; v0.38 to v0.43 in 6 days (2026-11-07 to 2026-11-12) | FACT: `gh release list --json publishedAt`; my count |
| Rule for when to release | "One release per task group in `docs/PLAN.md`" (never merge groups) | FACT: `docs/tooling.md#release-process` |
| Pre-release | `v0.0.1` to `v0.2.0` marked `isPrerelease=true`; from `v0.5.0` to `v0.43.0` `false` | FACT: `gh release view` |
| Module tags | `drivers/<name>/vX.Y.Z` and `contrib/<name>/vX.Y.Z` (e.g.: `drivers/s3/v0.3.0`); they do not trigger `release.yml` | FACT: `docs/tooling.md`, step 4 of Release process |
| Tag | Annotated (`gh api .../git/ref/tags/v0.43.0` returns `object.type = "tag"`); never move or delete a published tag; fix = new patch + `retract` | FACT |
| License | MIT | FACT: `gh api repos/AlexGalhardo/vukem` |

Latest module versions that have a GitHub Release (the module Releases list stops at 2026-10-18; INFERENCE: the later tags exist but stopped getting a GitHub Release, or became tags only; I did not verify via `git ls-remote --tags`):
`drivers/postgres v0.5.0`, `drivers/otel v0.4.0`, `drivers/mongo v0.2.1`, `drivers/redis v0.4.0`, `drivers/websocket v0.1.1`, `drivers/s3 v0.3.0`, `drivers/mysql v0.1.1`, `contrib/stripe v0.4.0`, `contrib/graphql v0.4.0`, `contrib/grpc v0.4.0`, `contrib/imaging v0.1.0`.

### 1.2 Exact format of tag, title and notes

| Item | Format | Real example |
| --- | --- | --- |
| Core tag | `v` + X.Y.Z | `v0.43.0` |
| Module tag | `<dir>/vX.Y.Z` | `drivers/s3/v0.3.0` |
| Core title | Same as the tag, with no extra text (the early `v0.0.x` releases had a suffix: `v0.0.3 — app lifecycle`, `v0.0.5 — middleware`) | `v0.43.0` |
| Module title | Same as the tag | `drivers/s3/v0.3.0` |
| Core body | Literal copy of the version section in `CHANGELOG.md`, **without** the `## [x]` heading and **without** the comparison link. Starts directly at `### Added`. English. | see 1.5 |
| Sections | `### Added`, `### Changed`, `### Fixed`, `### Security` (+ `Removed`/`Deprecated` when present). Fixed order by the `<!-- N -->` prefix of cliff | FACT: `.config/cliff.toml` |
| Item | `- **BREAKING:** **<scope>:** Message starting with a capital letter ([abc1234](https://github.com/AlexGalhardo/vukem/commit/<sha completo>))` | FACT: body of `v0.43.0` |
| PR link | None. Only a commit link (squash/rebase with one Conventional Commit per entry) | FACT |
| Module body | Own header: ``Module `github.com/AlexGalhardo/vukem/drivers/s3` ``, a `go get ...@vX.Y.Z` block, the sentence ``Changes under `drivers/s3/` since `drivers/s3/v0.2.0`.``, then sections | FACT: `gh release view drivers/s3/v0.3.0` |
| Core assets (from ~v0.30 on) | 15 files (see 1.3). Early releases (`v0.5.0`) had only `checksums.txt` + 6 files; modules: 0 assets | FACT: `gh release view --json assets` |

Pre-release: `docs/tooling.md` says "v0.x and pre-releases are marked pre-release", but the releases `v0.5.0`..`v0.43.0` are `isPrerelease=false`. See friction F1.

### 1.3 Published assets (v0.43.0, FACT)

```text
checksums.txt
checksums.txt.sigstore.json          # bundle cosign keyless do checksums.txt
vukem.intoto.jsonl                   # provenance SLSA (actions/attest-build-provenance)
vukem_0.43.0_{darwin,linux}_{amd64,arm64}.tar.gz
vukem_0.43.0_windows_{amd64,arm64}.zip
<cada arquivo acima>.sbom.json       # SBOM SPDX por archive (syft)
```

The archive contains `LICENSE`, `README.md`, `CHANGELOG.md`. Static binary (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`).

### 1.4 Tools and relationship with the CHANGELOG

FACT (`.config/cliff.toml`, `.config/goreleaser.yaml`, `internal/tools/release`):

- **git-cliff** generates the versioned section of `CHANGELOG.md` from Conventional Commits (Keep a Changelog 1.1.0). `docs`, `test`, `ci`, `build`, `chore`, `style` are omitted; `feat` becomes Added, `fix` Fixed, `perf`/`refactor` Changed, `revert` Removed, footer `Security:` becomes Security, footer `Deprecated:` becomes Deprecated.
- **`internal/tools/release`** (Go, `task release -- vX.Y.Z`): requires a clean tree on `main`, generates the section below `[Unreleased]`, updates the status table of `PLAN.md`, commits `chore(release): vX.Y.Z`, creates an annotated tag, pushes `main` and the tag in two separate pushes, publishes the GitHub Release with the body extracted from the CHANGELOG, and **refuses an empty body**. Then it bumps the vukem requirement in all nested modules.
- **GoReleaser v2** only attaches binaries, checksums, SBOMs and signature (`release.mode: keep-existing`, `changelog.disable: true`): it does not touch the notes.
- **cosign keyless** via Actions OIDC (`sign-blob --bundle`) on `checksums.txt` + `actions/attest-build-provenance`.
- The `[Unreleased]` section always exists. File footer: comparison links `[0.43.0]: .../compare/v0.42.0...v0.43.0`.
- Already released sections are never edited by hand.

Real header of `CHANGELOG.md` (1658 lines):

```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **ci:** Upload the Ubuntu coverage profile to [Codecov](...).
...
## [0.43.0] - 2026-11-12
```

Excerpt from `cliff.toml` (commit parsers):

```toml
tag_pattern = '^v[0-9]+\.[0-9]+\.[0-9]+'
sort_commits = "oldest"
commit_parsers = [
  { message = '^chore\(release\)', skip = true },
  { footer = "^Deprecated", group = "<!-- 2 -->Deprecated" },
  { message = "^(docs|test|ci|build|chore|style)", skip = true },
  { footer = "^Security", group = "<!-- 5 -->Security" },
  { message = "^feat", group = "<!-- 0 -->Added" },
  { message = "^(perf|refactor)", group = "<!-- 1 -->Changed" },
  { message = "^revert", group = "<!-- 3 -->Removed" },
  { message = "^fix", group = "<!-- 4 -->Fixed" },
]
```

### 1.5 Two real release notes, in full

**`v0.43.0`** (<https://github.com/AlexGalhardo/vukem/releases/tag/v0.43.0>, published 2026-11-12T16:07:34Z, title `v0.43.0`, `isPrerelease=false`, 15 assets). FACT: `gh release view v0.43.0`.

```markdown
### Added

- **BREAKING:** **cli:** Move the project upgrade to vukem update and update Go and JS dependencies ([8b4ac79](https://github.com/AlexGalhardo/vukem/commit/8b4ac793c77405f097e6e45c71c01e8740fea3ee))
- **cli:** Add vukem upgrade to update the CLI itself ([830fccc](https://github.com/AlexGalhardo/vukem/commit/830fccca76e44f467ffd4c2ebd26b7380db43bbf))
- **cli:** Register upgrade as the CLI self-update ([be34f8c](https://github.com/AlexGalhardo/vukem/commit/be34f8c27d21bffd3463cdeb30bc3fbecc952685))
- **tools:** Add starterdeps to keep scaffolded dependencies on the latest stable release ([d419e35](https://github.com/AlexGalhardo/vukem/commit/d419e35107a188e7673ab894b1a36bc2688a7395))
- **cli:** Scaffold the to-do example app in vukem new ([6439ee8](https://github.com/AlexGalhardo/vukem/commit/6439ee896c88f31a81bf4249401b9b8c09e2f3fd))
- **tools:** Cover images, actions, package managers and docs in starterdeps ([ff4774e](https://github.com/AlexGalhardo/vukem/commit/ff4774e02532152507c657510255f665ab848c24))
- **tools:** Pin the toolchain image base digest and Bun checksums in starterdeps ([032c2d8](https://github.com/AlexGalhardo/vukem/commit/032c2d83c4da88fa1826b6db26fd8fef18ba81f2))
- **cli:** Scaffold the personal finance SaaS example ([06d52d9](https://github.com/AlexGalhardo/vukem/commit/06d52d948e8e7826b413558dafec586363b21fc7))

### Fixed

- **cli:** Never downgrade the CLI to an older latest release ([65fdd8d](https://github.com/AlexGalhardo/vukem/commit/65fdd8d2fe95da03edefc19324a816beeb10eb71))
- **cli:** Resolve both paths when vukem upgrade checks for a go install binary ([1d5909f](https://github.com/AlexGalhardo/vukem/commit/1d5909feb3c3064cc3819828768b117e99f4b91b))

### Security

- **tenancy:** Add row-level multi-tenancy middleware, context and ORM scope ([d930d79](https://github.com/AlexGalhardo/vukem/commit/d930d79a09cf3faaa5bb84f6c159ae743a4fe752))
```

**`drivers/s3/v0.3.0`** (module; title `drivers/s3/v0.3.0`, published 2026-10-18, 0 assets). FACT: `gh release view drivers/s3/v0.3.0`.

````markdown
Module `github.com/AlexGalhardo/vukem/drivers/s3`.

```bash
go get github.com/AlexGalhardo/vukem/drivers/s3@v0.3.0
```

Changes under `drivers/s3/` since `drivers/s3/v0.2.0`.

### Added

- **s3:** Add idempotent region-aware CreateBucket ([1586a42](https://github.com/AlexGalhardo/vukem/commit/1586a42c3c0a4d101c8159e9f0b065b491cb916a))
````

Short example from the early era, `v0.5.0` (2026-10-16, 7 assets, no commit links; the format with links only appears later):

```markdown
### Added

- **auth:** Add hashers, session and token guards, and auth middleware
- **social:** Add OAuth2/OIDC social login with PKCE
```

### 1.6 Release workflow (core), real excerpts from `.github/workflows/release.yml`

Trigger and tag resolution (accepts `workflow_dispatch` because a push of more than 3 tags does not fire an event):

```yaml
on:
  push:
    tags: ["v*"]
  workflow_dispatch:
    inputs:
      version:
        description: "Version tag to build (vX.Y.Z). The tag must already be pushed."
        required: true
permissions:
  contents: read
jobs:
  goreleaser:
    runs-on: ubuntu-latest
    permissions:
      contents: write        # upload assets to the GitHub release
      id-token: write        # Sigstore OIDC identity for cosign and attestations
      attestations: write    # build provenance attestations
    steps:
      - name: Resolve the tag
        id: tag
        run: |
          tag="${INPUT_VERSION:-$REF_NAME}"
          case "$tag" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "::error::not a version tag: $tag"; exit 1;; esac
          # Only vX.Y.Z moves the image's latest tag; vX.Y.Z-rc.1 and friends do not.
          if [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then echo "stable=true" >>"$GITHUB_OUTPUT"; else echo "stable=false" >>"$GITHUB_OUTPUT"; fi
      - uses: actions/checkout@<sha> # v7.0.1
        with: { ref: "refs/tags/${{ steps.tag.outputs.tag }}", fetch-depth: 0, persist-credentials: false }
      - uses: actions/setup-go@<sha> # v7.0.0
        with: { go-version-file: go.mod }
      - uses: sigstore/cosign-installer@<sha> # v4.1.2
        with: { cosign-release: v3.1.3 }
      - uses: anchore/sbom-action/download-syft@<sha> # v0.24.3
        with: { syft-version: v1.54.1 }
      - uses: goreleaser/goreleaser-action@<sha> # v7.3.0
        with: { version: "v2.18.2", args: "release --clean" }
      - name: Verify the checksums signature
        run: |
          cosign verify-blob dist/checksums.txt \
            --bundle dist/checksums.txt.sigstore.json \
            --certificate-identity "https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release.yml@${GITHUB_REF}" \
            --certificate-oidc-issuer https://token.actions.githubusercontent.com
      - uses: actions/attest-build-provenance@<sha> # v4.2.2
        id: attest
        with: { subject-checksums: ./dist/checksums.txt }
      - name: Upload the provenance bundle   # vukem.intoto.jsonl, para o Scorecard enxergar
        run: |
          cp "$BUNDLE" vukem.intoto.jsonl
          gh release upload "$TAG" vukem.intoto.jsonl --clobber --repo "$GITHUB_REPOSITORY"
```

`image` job (stable tag only), **GHCR only** image:

```yaml
  image:
    needs: goreleaser
    if: needs.goreleaser.outputs.stable == 'true'
    env:
      IMAGE: ghcr.io/alexgalhardo/vukem
    steps:
      - name: Resolve the image tags
        run: |
          version="${TAG#v}"
          { echo "tags<<EOF"; echo "$IMAGE:$version"; echo "$IMAGE:${version%.*}"; echo "$IMAGE:latest"; echo "EOF"; } >>"$GITHUB_OUTPUT"
      - uses: docker/build-push-action@<sha> # v7.4.0
        with:
          file: .config/docker/vukem.Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          sbom: true
          provenance: mode=max
      - name: Sign the image digest
        run: cosign sign --yes "$IMAGE@$DIGEST"
      - uses: actions/attest-build-provenance@<sha> # v4.2.2
        with: { subject-name: ghcr.io/alexgalhardo/vukem, push-to-registry: true }
```

FACT: image tags = `X.Y.Z`, `X.Y`, `latest`; registry only `ghcr.io/alexgalhardo/vukem` (grep for `docker.io`/`alexgalhardo/vukem` in `.github`, `.config`, `docs/tooling.md` finds only GHCR). The image is a toolchain (CLI + Go + Bun + git), not an app. All actions are pinned by full SHA + `# vX.Y.Z` comment.

### 1.7 App release workflow (`C:\dev\vukem-video\web\.github\workflows\release.yml`)

FACT. Lean version (no binaries or image): extracts the CHANGELOG section from the default branch and creates the release.

```yaml
- name: Extract changelog notes
  run: |
    git show "origin/$DEFAULT_BRANCH:CHANGELOG.md" > "$RUNNER_TEMP/CHANGELOG.md"
    version="${TAG#v}"
    awk -v ver="$version" '
      $0 ~ "^## \\[" ver "\\]" { capture=1; next }
      capture && /^## \[/ { exit }
      capture { print }
    ' "$RUNNER_TEMP/CHANGELOG.md" > "$RUNNER_TEMP/notes.md"
    if [ ! -s "$RUNNER_TEMP/notes.md" ]; then echo "::error::CHANGELOG.md has no section for $version"; exit 1; fi
- name: Publish the GitHub release
  run: gh release create "$TAG" --title "$TAG" --notes-file "$RUNNER_TEMP/notes.md" --repo "$GITHUB_REPOSITORY"
```

The app has 5 tags (`v0.1.0` to `v0.4.0`, `v0.3.1` included), a `CHANGELOG.md` with `[Unreleased]` and a comparison footer, and a `cliff.toml` with the same `tag_pattern`. The binary/Docker/cosign pipeline described for vukem does not exist in the app (the comment itself says "added when the application ships a binary").

---

## 2. Repository conventions

### 2.1 Folder structure (`vukem` repo)

FACT (`ls` of the clone, `AGENTS.md#where-things-live`):

| Path | Role |
| --- | --- |
| root `*.go` (`app.go`, `boot.go`, `serve.go`, `http.go`, `background.go`, `database.go`...) | `vukem` package (core: App, providers, boot, server, options) |
| `router/`, `web/`, `middleware/`, `session/`, `csrf/`, `i18n/`, `config/`, `errs/` | HTTP and core infrastructure |
| `orm/`, `schema/`, `database/` | query builder/ORM, migrations/schema, connection and dialects |
| `auth/`, `authz/`, `crypt/`, `signed/`, `ratelimit/`, `security/`, `challenge/` | authentication, authorization, crypto, signed URLs, rate limit |
| `queue/`, `schedule/`, `events/`, `broadcast/`, `cache/`, `mail/`, `notify/`, `flags/` | subsystems |
| `upload/`, `storage/`, `archive/`, `mediatype/`, `export/` | files |
| `inertia/`, `htmx/`, `openapi/`, `apidocs/`, `jsonrpc/`, `llmstxt/` | frontend and APIs |
| `cmd/`, `internal/cli/` | `vukem` CLI |
| `internal/tools/` | repo tools: apicheck, commitlint, configref, covergate, fixinline, labelsync, modcheck, release, snippetcheck, starterdeps |
| `drivers/*` | separate Go modules: mongo, mysql, otel, postgres, redis, s3, turso, websocket |
| `contrib/*` | separate Go modules: cloudflare, engine, graphql, grpc, http3, imaging, qrcode, stripe, studio |
| `examples/` | `api`, `blog`, `saas` |
| `docs/` | design docs (one per area) + `PLAN.md` (roadmap) |
| `site/` | vukem.dev site (Astro) |
| `.config/` | `cliff.toml`, `goreleaser.yaml`, `compose.yaml`, `docker/`, `ruleset-protect-main.json`, `apicheck-exceptions.json` |
| `.github/` | workflows, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, CODEOWNERS, templates, `labels.txt`, `dependabot.yml` |
| `.claude/` | `CLAUDE.md`, `rules/`, `settings.json`, `skills/` (incl. `vukem-release`) |

### 2.2 `go.mod` and dependencies

FACT: `go 1.27.4`. The core requires only `github.com/google/go-cmp v0.7.0`, `golang.org/x/crypto v0.59.0` and `modernc.org/sqlite v1.61.0` (pure Go SQLite, no cgo). `go.work` lists the core + all drivers/contrib/`internal/bench` and `ignore ./site`. Architecture rule: "core never imports drivers/*or contrib/*"; new deps only through the "dependency policy" in `docs/architecture.md`. No `replace` in a published go.mod. The `vukem-studio` app (web) has `go 1.27.4` and `require github.com/AlexGalhardo/vukem v0.43.0` (module `github.com/AlexGalhardo/vukem-studio`).

### 2.3 Task runner, lint, format, hooks

| Topic | Finding | Source |
| --- | --- | --- |
| Runner | `Taskfile.yml` (go-task). Targets: `ci`, `ci:full`, `fmt`, `lint` (`lint:go`, `lint:md`), `test` (`-short`), `test:e2e`, `test:race`, `test:integration`, `bench`, `cover`, `vet`, `vuln`, `api:check`, `mod:check`, `snippets:check`, `config:docs`, `config:check`, `tidy:check`, `changelog`, `release`, `release:modules`, `release:binaries`, `secrets:scan`, `deps:starters`, `site:dev`, `site:build`, `hooks`, `setup` | FACT `Taskfile.yml` |
| Go lint | golangci-lint v2.14 (`.golangci.yml`), formatting with gofumpt + gci + golines via `golangci-lint fmt`, `go vet`, `go fix -diff`, decreasing budget of `//nolint:gocognit` (4) | FACT `ci.yml`, `AGENTS.md` |
| Markdown | markdownlint-cli2 (`.markdownlint-cli2.jsonc`: `MD013` off, `MD024 siblings_only`, `MD004 dash`, ignores `plans/**`, `.vukem/**`, `.claude/skills/**`) | FACT |
| EditorConfig | `.editorconfig`: 2 spaces by default; **Go, go.mod, Makefile, `*.tmpl/gohtml` with TAB (4)**; LF; `.ps1/.bat` CRLF | FACT |
| Hooks | `lefthook.yml`. pre-commit: fmt, lint (`--new-from-rev=HEAD`), tidy:check, markdownlint. commit-msg: `go run ./internal/tools/commitlint {1}`. pre-push: `go vet`, `govulncheck`, refresh of `last-updated.json` if `site/content/**` changed | FACT |
| Hooks in the app | `.githooks/commit-msg` and `.githooks/pre-push` (bash, instead of lefthook); pre-push regenerates `site/content/last-updated.json` and, if it changes, commits and aborts the push | FACT `web/.githooks/pre-push` |
| Secret scan | Betterleaks (`.betterleaks.toml`, `.betterleaksignore`), workflow `secrets.yml` | FACT |
| API compatibility | `task api:check` (apidiff); a rename keeps an alias `//go:fix inline` | FACT |

### 2.4 Tests

FACT (`AGENTS.md`, `docs/testing.md`): "test first"; stdlib + go-cmp, table-driven, no network in unit tests; security tests `TestSecurity_<Axx>_<Behaviour>` for each default; `go test -short` day to day; `-race` in CI; integration with a build tag per driver (Postgres, MySQL, Redis...); a weekly 1h fuzz run per target (`fuzz.yml`); Ubuntu coverage is uploaded to Codecov via OIDC, and there is a `covergate` (fails the job if coverage drops below the floor). Helper package `vukemtest` for testing apps (fake clock, etc.). Linux/macOS/Windows matrix ("Windows 11 stays first-class").

App `web`: `tests/*.go` (auth, jobs, voice uploads, locale, railway_config...), `app/studio/*_test.go`, and `e2e/` with Playwright (`playwright.config.ts`, `specs/{ok,model,render}.spec.ts`), `go test -race -count=1 ./...` in CI and a separate Windows job.

### 2.5 vukem CI workflows (15 files in `.github/workflows`)

| Workflow | Role (FACT, YAML header) |
| --- | --- |
| `ci.yml` | Go lint, apidiff, openapi, markdown, test matrix (Linux/macOS/Windows, race), vuln, and per-driver/contrib jobs (mongo, otel, websocket, graphql, stripe, cloudflare, grpc, imaging, qrcode, studio, engine, redis, mysql, turso, postgres) |
| `pr-title.yml` | `amannn/action-semantic-pull-request` v6.1.1; types build/chore/ci/docs/feat/fix/perf/refactor/revert/style/test; `subjectPattern: ^[^A-Z].*[^.]$` |
| `release.yml` | GoReleaser binaries, cosign, SBOM, provenance, GHCR image |
| `image.yml` | builds the image in PRs that affect it, without publishing |
| `site.yml` | site build on every PR/push; deploy to GitHub Pages on push to `main` |
| `docs-last-updated.yml` | checks `last-updated.json` on every PR |
| `codeql.yml`, `vuln.yml` (weekly), `scorecard.yml`, `secrets.yml` | security/supply chain |
| `fuzz.yml` (weekly), `bench.yml` (manual) | quality |
| `downstream.yml` | manual canary: builds the downstream apps against the checkout |
| `starter-deps.yml` | weekly: `starterdeps` rewrites the scaffold pins and opens a PR |
| `ci-failure.yml` | opens a `ci-failure` issue when a scheduled workflow fails; an open issue blocks the next release |

Workflows of the `web` app: `ci.yml` (markdown, types (Bun 1.4.4 + biome + tsc), Playwright e2e with ffmpeg, go fmt/vet/test -race, go-windows, vuln, secrets, image), `pr-title.yml`, `release.yml`, `scorecard.yml`, `security-audit.yml` (skills/agent), `site.yml`.

### 2.6 Commits, branch, PR

FACT: Conventional Commits 1.0 in English, scope = package/area (list in `docs/tooling.md`), header up to 100 chars, imperative description, lowercase initial, no final period. Breaking = `!` + footer `BREAKING CHANGE:`. Security = footer `Security: <closed risk>` in feat/fix/perf/refactor. Branches `feat/<name>`, `fix/<name>`, `docs/<name>`. The PR title becomes the squash commit (or a batch of commits is rebase-merged). The `protect-main` ruleset (`.config/ruleset-protect-main.json`) requires a PR and green checks. PR template with a checklist (conventional title, `task ci`, `TestSecurity_*`, godoc + site page, no new `//nolint`, no new dep without a decision, actions pinned by SHA, `CHANGELOG.md` untouched, `BREAKING CHANGE:` + `docs/upgrade/`). Task reference in the body: `Refs: P.3.2`.

### 2.7 CONTRIBUTING, SECURITY, AGENTS.md, CLAUDE.md

- `.github/CONTRIBUTING.md`, `.github/SECURITY.md`, `.github/CODE_OF_CONDUCT.md`, `CODEOWNERS`, issue forms in `.github/ISSUE_TEMPLATE`. SECURITY.md documents how to verify the signature (cosign) and the image by digest. FACT.
- `AGENTS.md` with 51 lines (rule: "≤ 50 lines" in P.0.11): sections "Start here", "Commands", "Rules", "Where things live", "When stuck". `CLAUDE.md` with 7 lines: points to AGENTS.md. `.claude/rules/*` with rules per path (Go, tests, security, workflows, release). FACT.
- AGENTS rule: "Downstream apps (... `vukem-studio` = vukem-video ...) log framework findings in the single `vukem-feedback.md` at the workspace root", and the fixes go into one PR, one commit per entry. The file `C:\dev\vukem-video\vukem-feedback.md` exists, with `VF-001` (format: Date, Context, Expected, Actual, Suggestion, Severity, Type). FACT.

### 2.8 `plans/*.md` and ROADMAP

- **There is no `plans/` folder in the vukem repo nor in `C:\dev\vukem-video` (up to 3 levels).** The `.markdownlint-cli2.jsonc` only ignores it (`plans/**`). FACT (find/ls). The real vukem plan is `docs/PLAN.md` ("single source of truth for what to build next").
- Real structure of `docs/PLAN.md` (FACT):

```markdown
# Vukem — Plan
## How to use this file          (6 steps: read AGENTS, find the first unmarked task of the current phase, test first, mark [x], one release per group, never skip a phase)
Task IDs: `<phase>.<group>.<task>` (e.g. `P.3.2`). Reference them in commit bodies (`Refs: P.3.2`).
## Status                        (tabela: Current phase | Next task | Updated — reescrita pelo tool de release)
## Design references             (tabela Topic | Doc)
## Definition of Done            (lista que vale para toda tarefa)
## Phase overview                (tabela Phase | Version | Goal | Exit criteria)
## Phase 1 — Prototype (`v0.1.x`)
**Goal:** ...
### P.0 Repository foundation
- [x] **P.0.1** Initialise module ... (`go 1.27.0`) ...
  → verify: `go build ./...` succeeds.
- [x] **P.0.2** Add `.editorconfig`, ...
  → verify: `git ls-files --eol` shows `i/lf` ...
### P.1 Config & environment — needs: P.0
```

  Each task has an ID, a statement and an executable `→ verify:` line. Groups may mark `needs:`.

- App ROADMAP (`C:\dev\vukem-video\web\ROADMAP.md`, 197 lines, FACT): legend `[x]` done, `[/]` partial, `[ ]` not started; "A phase is done only when every audit box under it is `[x]`"; phases `## Phase N — Title`, subsections `### N.M`, final box `#### Audit — phase N` with verifiable criteria (commands, URLs, CI run IDs). Ex.: `- [x] Scorecard workflow has run on main (runs 42485982009 ..., 2026-11-08 ...)`.

### 2.9 Note on the `web` repo

`git log` in `C:\dev\vukem-video\web` answers "your current branch 'main' does not have any commits yet" (FACT, read-only): this folder has an empty `.git`. The app README says "The first release is `v0.1.0`" while the CHANGELOG is already at `0.4.0` (INFERENCE: outdated text).

---

## 3. Structure of a vukem app and capability matrix

### 3.1 Scaffold (FACT: `site/content/docs/getting-started/directory-structure.mdx` and real app `web/`)

```text
app/            controllers/ models/ requests/ jobs/registry.go listeners/registry.go   (+ no app real: accounts/ studio/ jobs/*.go)
bootstrap/      app.go (New(opts...)), providers.go, commands.go    # "nothing is wired by magic"
cmd/app/        main.go  =>  func main() { os.Exit(vukem.Main(bootstrap.New)) }
database/       migrations/ (timestamp + registry.go)  factories/  seeders/
resources/      resources.go (go:embed de views/ e public/), views/, js/, css/, lang/, vite/
routes/         web.go, registry.go, schedule.go (+ auth.go, jobs.go...)
storage/        SQLite e arquivos locais (ignorado no git)
tests/          testes do app
Dockerfile, .dockerignore, package.json, vite.config.ts, .env.example
```

The `bootstrap.New` of the real app (`web/bootstrap/app.go`) uses `vukem.WithProviders`, `WithMigrations`, `WithSeeders`, `WithListeners`, `WithJobsFunc(jobs.All)`, `WithSchedule`, `WithViews`, `WithVite`, `WithInertia(inertia.Config{})`, `WithTranslations`, `WithRoutes(fn)`. Routes: `r.Get("/jobs/{job}", h.Show).Name("jobs.show").Use(private...)`. The controller receives `*web.Ctx`. No globals, no `init()`, no reflection-based DI.

### 3.2 Inertia + React + Vite

FACT: **own adapter, in the core** (`inertia/`, "Vukem's Inertia v3 adapter", docs `frontend/inertia.mdx`), not gonertia. Props, lazy props, shared props, forms/validation/flash, error pages, asset versioning, history encryption, Precognition, SSR (with strict mode, `ClientShell`), typed pages (`codegen:pages` generates `page-props.ts`), typed routes. Vite 8: dev server with a "hot file" and manifest in production, CSP nonces + SRI (`frontend/vite.mdx`). Starter: `vukem new --inertia react|vue|svelte`. Real app: `@inertiajs/react 3.9.1`, `vite 8.3.4`, React 19.3.1, Tailwind 4.3.3, shadcn, Biome 2.6.6, TypeScript 7.0.2, Bun 1.4.4 (`web/package.json`).

### 3.3 Matrix: what vukem offers and what it does not offer

Maturity legend: **stable** = in the core and covered by CI; **module** = separate Go module with its own 0.x version; **experimental** = the docs/code say so explicitly.

| Item | Exists? | Where | Maturity / note |
| --- | --- | --- | --- |
| WebSocket | Yes, **outside the core** | `drivers/websocket` (v0.1.1) + `docs/digging-deeper/broadcasting.mdx` | 0.1.x module; public/private/presence channels with fail-closed authorization |
| SSE | Yes, core | `c.SSE`, package `sse`, `digging-deeper/sse.mdx` | heartbeats, `Last-Event-ID`, Hub per topic, multi-instance via Redis |
| Queues/background jobs | Yes, core | `queue/`, `digging-deeper/queue.mdx` | drivers `sync`, `memory`, `database` (core) and Redis (module). At-least-once, retries/backoff/timeout, `Unique(key)`, strict priority per queue, `QUEUE_LIMITS`, `DispatchTx`, failures table and `queue:failed` commands. Worker runs inside `serve` or in `queue:work` |
| Job progress / cancel running job | **Not found** | grep in `queue/*.go` finds no progress API nor per-job cancellation; only `ctx` cancellation on shutdown | the real app stores state in its own row (`app/jobs/save.go`, `claim`). INFERENCE: needs its own model |
| Large upload | Yes | `upload.Receive` (streaming multipart to disk), `upload.Save`, `upload.Tus` (resumable, tus 1.0.0 protocol with `creation,expiration,termination`; `MaxSize` required), `upload.Direct`+`Verify` (presigned S3), type sniffing, dimension check, ClamAV, thumbnails | core; tus present but only the listed extensions (no `concatenation`/`checksum`: FACT in `upload/tus.go`) |
| Serve video with seek | Yes | `c.File`/`StaticFS` use `http.ServeContent` (Range/206) | FACT `basics/static-files.mdx:145` and `responses.mdx:308` |
| Storage | Yes | `storage.Disk` (Put/Open/Exists/Delete/List/URL): `NewLocal`, `NewMemory`, S3 in `drivers/s3` (v0.3.0); signed URLs (`signed`) | local writes atomically; S3 = module |
| Auth: session | Yes | `auth`, `session`, `install:auth` (login, social OAuth2/OIDC with PKCE, 2FA, passkeys) | core |
| Auth: API keys/tokens with scopes | Yes | `auth.Tokens` opaque, abilities (scopes), `install:api`, "My API tokens" page (`security/api-tokens.mdx`) | core; JWT in `security/jwt.mdx` |
| Authorization / multi-tenant | Yes | `authz/`; row-level `tenancy` (added in v0.43.0, Security section) | tenancy is very recent (v0.43.0): INFERENCE, little exposure |
| Migrations | Yes | `schema/`, `vukem migrate`, `migrate:check`, `migrate:diff`, `db:pull`; timestamped Go files with `registry.go` | core |
| ORM / query builder | Yes | `orm/` (generics: `orm.QueryOf[T](db)`), joins, aggregates, subqueries, relations, Upsert, savepoints, optimistic lock, `codegen:models` | core; several BREAKING in v0.41.0 |
| Databases | SQLite (pure Go, core), PostgreSQL (`drivers/postgres`), MySQL/MariaDB (`drivers/mysql`), Turso (**experimental**), MongoDB (`drivers/mongo`) | `database/overview.mdx` | SQLite with safe defaults; workers share the single connection in SQLite (FACT `queue.mdx`) |
| CLI / generators | Yes | `vukem new` (starters HTMX, templ, Inertia react/vue/svelte, minimal, api; examples `todo`, `finance`), `make:*`, `install:*`, `codegen`, `doctor`, `route:list`, `serve`, `build`, `update`, `upgrade`, `bench` | core |
| OpenAPI | Yes | `openapi/`, `apidocs/` (UI), TypeScript/Go client generation, `z` (typed schemas) as the source of the constraints | core; "BREAKING" in v0.41.0 in the TS client format |
| MCP / agents | Partial | `vukem boost:install` registers an MCP server **for agents that program the app** (`boost:mcp`, stdio) | **there is no app MCP/RPC for external clients**. There is `jsonrpc/` (with `openrpc.go`) and `llms.txt` (`llmstxt/`). INFERENCE: a video-editing MCP must be written on top of `jsonrpc` or an external lib |
| Middleware | Yes | `func(http.Handler) http.Handler`; `r.Use`, `r.UseServer`, fixed order validated | core |
| Rate limit | Yes | `ratelimit.Middleware`, key per IP (/64 in IPv6), token or account, `memory`/database/Redis store (`RATE_LIMIT_STORE`) | core |
| Observability | Yes | structured logging, `drivers/otel` (v0.4.0), health checks (`WithHealthCheck`), `devtools`, `analytics`, `loadtest` | otel = module |
| i18n | Yes | `i18n/`, `WithTranslations`, `inertia.I18n("ui.")`, `lang` cookie | core |
| SPA outside Inertia | **Partial / no dedicated docs** | `StaticFS` serves `resources/public` (embedded folder), Inertia's `ClientShell` gives a shell for client-rendered pages. I did not find an "SPA fallback" (route `*` → `index.html`) in `middleware/static*.go` nor in the docs (grep `SPA`/`Fallback`) | INFERENCE: serving a pure SPA editor requires a catch-all route written by hand or using Inertia |
| Embed assets in the binary | Yes | `go:embed` in `resources/resources.go`; `deployment/single-binary.mdx` (static build, cross-compile); `StaticFS` | core |
| Single binary | Yes | `CGO_ENABLED=0`, pure Go SQLite, embedded assets | FACT in the docs and the app's Dockerfile |
| Subprocesses (ffmpeg) | Yes, module | `contrib/engine`: env built from scratch, confined cwd, limited output, resource limits, cancellation kills the process tree (`proc_windows.go`) | no `go.mod` tag in the listed releases; INFERENCE: low maturity; the real app uses `app/studio/exec.go` |
| Image/thumbnails | Module | `contrib/imaging` v0.1.0 | |
| Scheduler | Yes | `schedule/`, `routes/schedule.go` | |
| Webhooks, billing | Yes | `digging-deeper/webhooks.mdx`, `contrib/stripe` | |
| HTTP/2/3, Cloudflare | Yes, modules | `contrib/http3`, `contrib/cloudflare` | |
| GraphQL/gRPC | Yes, modules | `contrib/graphql`, `contrib/grpc` | |
| API stability | **pre-1.0** | `docs/stability.md`: compatibility promise only from `v1.0.0`; today `v0.43.0` with `BREAKING` in almost every release | see F6 |

### 3.4 Maturity state of vukem itself (FACT)

`docs/PLAN.md`: current phase **BETA** (stabilization B.5), next `S.3.1`, updated 2026-11-12. 1.0 criterion: three of the maintainer's projects in production on Railway, with no RC (`docs/brainstorming.md` Q14). In other words, vukem is not 1.0; `v0.41.0` and `v0.43.0` have `BREAKING` entries (e.g.: `config: Treat a missing APP_ENV as production`, `cli: Move the project upgrade to vukem update`).

### 3.5 What the real `web` app shows in practice (FACT)

- Render job: `queue.Handle(handleRender(app), queue.Retries(0), queue.Timeout(10*time.Minute))`; payload only with `JobID`; the handler does `Claim` (conditional UPDATE) so it does not run twice, and `studio.Run` calls the planner and `ffmpeg`.
- `.env`/Dockerfile.app: `QUEUE_TABLE`/`QUEUE_FAILED_TABLE` renamed so they do not collide with the domain table `jobs` (see F5).
- Multi-stage Docker: `oven/bun:1.4.4-alpine` (assets) → `golang:1.27.4-alpine3.24` (build with `-tags` from `.buildtags`) → `alpine:3.24.2` with `ffmpeg`, non-root user uid 10001, `/app/storage`.
- Auth: Google-only login with allowlist (`routes/allowlist.go`), `auth.Required`/`auth.Verified` + own middleware.
- i18n: `resources/lang` (en, pt-BR), `Inertia.I18n("ui.")`.
- Routes return Inertia; download via `h.Download`.

---

## 4. Docs site

### 4.1 vukem.dev (`site/` folder of the repo)

FACT (`site/package.json`, `astro.config.mjs`, `content/navigation.ts`, `.github/workflows/site.yml`):

| Item | Finding |
| --- | --- |
| Framework | **Astro 7.4.7**, `output: "static"`, `outDir: "out"`, `trailingSlash: "always"`, React 19 islands (`@astrojs/react`), Tailwind 4.3.3, `@base-ui/react`, MDX (`@mdx-js/mdx`), `shiki` 4.5 for code, `rehype-slug`, `remark-gfm` |
| Content | `site/content/docs/<category>/<page>.mdx`; 12 categories (Getting started, Coming from, Architecture, The basics, APIs, Database, Security, Digging deeper, Frontend, Testing, CLI, Deployment); 104 .mdx pages. Didactic style: "The idea", "Quick start" with `<Steps>`, `<Term tip>`, "Common mistakes", "Next steps", "You will learn" |
| Navigation | `site/content/navigation.ts`: array `{title, dir, prefix?, pages[]}`; page `<dir>/<slug>.mdx` served at `/docs/<prefix>/<slug>`; "Getting started" lives at the root of `/docs` |
| Search | `src/pages/search-index.json.ts` (index generated at build) + `Ctrl K` dialog loaded on first use (changelog v0.41.0 "Load the search dialog on first open") |
| Site i18n | **Not found**: no locale folder; the site is English only |
| Versioning | `content/versions/0.20` (snapshot of the 0.20 docs); redirects `/docs` and `/docs/0.20`; canonical for old versions (changelog v0.41.0) |
| Agents | `llms.txt`, `llms-full.txt`, `robots.txt`, `sitemap.xml` as Astro endpoints; **Markdown twin of each page** (v0.41.0 "Serve a markdown twin of every docs page"); "Agents" link at the top points to `/llms.txt` |
| Dates | "Last updated: `<date>` UTC" per page, from git: `content/last-updated.json` (generated by `npm run last-updated`, never edited by hand); 3 checks (`Site / Last updated`, `Docs last updated`, pre-push hook) |
| Quality | `check:mdx`, `eslint`, script tests (`link-anchors`, `markdown-twin`, `trailing-slash`...), `snippetcheck` (the Go blocks in the docs are type-checked in the core CI) |
| Deploy | GitHub Pages: `actions/upload-pages-artifact` on push to `main` (`site.yml`); changelog rendered at `/changelog` from `CHANGELOG.md` |
| Docs home page (observed via WebFetch of `https://vukem.dev/docs/introduction`) | top bar "Docs / Agents / Changelog / GitHub icon"; search "Search… Ctrl K"; sidebar on the left; "On this page" on the right; footer "Last updated: November 11, 2026 at 02:48 PM UTC", "Edit this page on GitHub" and previous/next. Version selector and theme toggle **did not appear** in the converted content (I do not claim they do not exist) |

### 4.2 Comparison with `C:\dev\vukem-video\web\site` (FACT)

| Item | vukem.dev | `web/site` (studio-docs) |
| --- | --- | --- |
| Base | Astro 7 | Astro 7 (same shell, copied), `package.json` `name: studio-docs` |
| Categories | 12 | 6: Getting started, Pipeline, Tools, Skills, Deployment, Security |
| Scripts | includes `legacy-path-redirect`, `trailing-slash`, `current-equivalent`, `link-anchors`, `markdown-twin`, `llms*`, `sitemap` | has `serve.ts` (standalone Node server), `static-path.ts`; **missing**: markdown twin, link-anchors, trailing-slash, legacy redirects and `index.astro` |
| Deploy | GitHub Pages | Dockerfile + Railway (`studio-production-b48f.up.railway.app`); the ROADMAP records `studio.vukem.dev` with the CNAME still to be pointed |
| Home | own landing page | `/` redirects to `/docs/introduction` |
| Types | `tsc` of the site | root `bun run check` calls `bun --cwd site run check` (`tsc --noEmit`) |

INFERENCE: `web/site` is an old fork of the shell; later fixes made in vukem's `site/` (markdown twin, anchors, trailing slash) were not inherited.

### 4.3 `https://www.freecut.net/docs` (observed on 2026-11-14)

FACT: `curl` on `/docs`, `/docs/introduction`, `/sitemap.xml`, `/robots.txt`, `/llms.txt` always returns the **same 1408-byte HTML** (`<title>FreeCut</title>`, `<div id="root">`, bundle `main-CbVc0t0C.js`): SPA fallback for any path. There is no static sitemap, robots or llms.txt (the response is the shell with HTTP 200). The docs are a **route inside the React app** (not a separate static site, no `generator` meta). Rendered via Playwright, the navigation is:

```text
FreeCut | Docs | Open FreeCut (/projects) | Home | Docs overview (/docs)
START            Getting Started (/docs/getting-started) | How FreeCut Works (/docs/concepts) | Workspaces and Storage (/docs/workspaces) | Projects (/docs/projects) | Editor Layout and Navigation (/docs/editor)
CORE EDITING     Media Library (/docs/media) | Basic Timeline Editing (/docs/timeline) | Editing Tools (/docs/editing-tools) | Preview and Playback (/docs/preview) | Source Monitor Workflow (/docs/source-monitor) | Clip Properties (/docs/properties)
CREATIVE TOOLS   Text, Captions, and Subtitles (/docs/text-captions-subtitles) | Shapes and Masks (/docs/shapes-masks) | Audio Editing (/docs/audio) | Effects and Color (/docs/effects-color) | Transitions (/docs/transitions) | Animate with keyframes (/docs/keyframes) | Apply and reuse motion (/docs/motion-library) | Use the Motion workspace (/docs/motion) | Scene Browser and AI Analysis (/docs/scene-browser) | Local AI Tools (/docs/local-ai)
OUTPUT           Exporting (/docs/export) | Render Queue (/docs/render-queue)
REFERENCE        Keyboard Shortcuts (/docs/keyboard-shortcuts) | Settings (/docs/settings) | Troubleshooting (/docs/troubleshooting) | Reference (/docs/reference)
```

Total: 30 pages in 5 sections (Start 5, Core editing 6, Creative tools 10, Output 2, Reference 4, plus "Docs overview"). The `/docs` page is "FreeCut User Guide" with "Start editing" and "Fix a problem" shortcuts and cards per section. There is a search field (detected `input[placeholder*=earch]`). The bundle has per-language chunks (`de-*.js`, `es-*.js`), so the app has i18n; I did not confirm whether the docs are translated. Note: an FAQ page in the bundle says the current focus is "beta hardening: align product docs with shipped behavior" (excerpt from `app-shell-Dhf_E_sb.js`).

---

## 5. Friction

Only items with evidence are included. Label per item.

| ID | Friction | Evidence | Label |
| --- | --- | --- | --- |
| F1 | Doc says `v0.x` releases are marked pre-release, but the practice is `isPrerelease=false` from `v0.5.0` to `v0.43.0` (only `v0.0.1` to `v0.2.0` are pre-release) | `docs/tooling.md` step 2 of "Release process"; `gh release view v0.43.0 --json isPrerelease` = false | FACT |
| F2 | Versioning docs contradict each other about BETA: the phase table in `PLAN.md` says `v1.0.0-beta.N → rc.N`; `tooling.md` and `brainstorming.md#Q14` say `v0.N.0` per group and "no release candidates" | `docs/PLAN.md` "Phase overview"; `docs/tooling.md` "Versioning"; `docs/brainstorming.md` Q14 | FACT |
| F3 | `CHANGELOG.md` is "generated, never edited" (PR template: "`CHANGELOG.md` untouched"), yet `[Unreleased]` has hand-written entries (no commit hash, with `ci:` types that cliff would omit) and the app's CONTRIBUTING says "Update `CHANGELOG.md` under `[Unreleased]`". Two conventions coexist | `CHANGELOG.md` lines 8+; `.github/pull_request_template.md`; `web/README.md` "Contributing" | FACT (dual convention) / INFERENCE (that they are manual) |
| F4 | vukem's image is published only to GHCR; there is no Docker Hub. Our global rule requires GHCR + Docker Hub, so the new project cannot copy `release.yml` as is | `release.yml` job `image` (`IMAGE: ghcr.io/alexgalhardo/vukem`) | FACT |
| F5 | Default queue tables (`jobs`, `failed_jobs`) collide with a common domain table; the real app had to set `QUEUE_TABLE=queue_jobs` and `QUEUE_FAILED_TABLE=queue_failed_jobs`. Already logged as VF-001 | `C:\dev\vukem-video\vukem-feedback.md` VF-001; `web/Dockerfile.app` | FACT |
| F6 | Pre-1.0 API moving fast: `BREAKING` in recent releases (`v0.41.0`: `config: Treat a missing APP_ENV as production`, `orm: Skip trashed rows in Update`, `z: Make Slice and Map generic schemas`; `v0.43.0`: `cli: Move the project upgrade to vukem update`). Recurring upgrade cost for the app | body of releases `v0.41.0` and `v0.43.0` | FACT |
| F7 | No progress or per-job cancellation API in the queue; the real app reinvents state/`Claim` and `Retries(0)` to avoid races | grep in `queue/*.go` (only shutdown `ctx`); `web/app/jobs/render.go` | FACT (absence in the code) / INFERENCE (that it is a gap for video rendering) |
| F8 | No app MCP (only development MCP via `boost:mcp`); no API-key package scoped specifically to MCP | `site/content/docs/cli/overview.mdx` lines 315, 377, 673 | FACT |
| F9 | No documentation of pure SPA (catch-all → `index.html`) outside Inertia; the documented path is Inertia or HTMX | search for `SPA`/`fallback` in `site/content/docs/frontend`, `basics/static-files.mdx`, `deployment/single-binary.mdx` (only SSR `fallback`/`c.Back`) | FACT (did not find it) / INFERENCE (that there is no helper) |
| F10 | WebSocket is not in the core (module `drivers/websocket` v0.1.1), and `contrib/engine` (ffmpeg) and `contrib/studio` are recent/experimental; `contrib/studio/doc.go` says "This module is experimental" | `drivers/websocket`, `contrib/studio/doc.go`, `gh release list` | FACT |
| F11 | Ambiguous name `vukem-studio`: it is (a) the `contrib/studio` module (data browser, `vukem studio` command) and (b) the video app `github.com/AlexGalhardo/vukem-studio` (`web/go.mod`) | `contrib/studio/doc.go`; `web/go.mod`; `AGENTS.md` ("vukem-studio = vukem-video") | FACT |
| F12 | The docs say to extract notes with literal `index($0,"## ["v"]")` and warn that the regex form "never matches". The app's `release.yml` uses the form `$0 ~ "^## \\[" ver "\\]"` (with double backslash, which does match), but with the SemVer `.` acting as a regex wildcard | `docs/tooling.md` "Changelog extraction in app workflows"; `web/.github/workflows/release.yml` | FACT / INFERENCE (real risk is minimal) |
| F13 | Version drift in docs: `docker-and-railway.mdx` says runtime `alpine:3.24.4`; `web/Dockerfile.app` uses `alpine:3.24.2` (the weekly `starterdeps` should keep the scaffold; the app does not inherit it) | `site/content/docs/deployment/docker-and-railway.mdx:144`; `web/Dockerfile.app` | FACT |
| F14 | The macOS CI can end `cancelled` when GitHub does not assign a runner; the documented fix is to re-run | commit `f8fefef` (`docs(ci): record the macOS test job cancelled by an unassigned runner`); `docs/maintenance.md` "Known CI issues" | FACT |
| F15 | Module releases on GitHub stop on 2026-10-18 in the listing (`drivers/s3` v0.3.0 etc.); newer versions exist only as tags, if at all. Makes it hard to know the current version of `contrib/engine` and `contrib/studio` (which do not even appear in the release list) | `gh release list --limit 300` | FACT (list) / INFERENCE (cause) |
| F16 | Fork of `site/` without inheriting fixes: `web/site` has no markdown twin, link-anchors or trailing-slash tests | comparison of `ls` of `scripts/` between the two sites | FACT |
| F17 | `web` is a `.git` with no commits; the app README says the first release is `v0.1.0`, while the CHANGELOG already goes to `0.4.0` | `git log` in `web`; `web/README.md` "Stability" | FACT |
| F18 | `freecut.net/docs`: any path returns 200 with the SPA shell (no real 404, no sitemap/robots/llms.txt); docs not indexable by agents without rendering JS | `curl` on `/docs`, `/sitemap.xml`, `/robots.txt`, `/llms.txt` | FACT |

Not found: broken links in `docs/introduction` (I did not test all links; I only loaded the page), nor an inconsistency between `go.mod` and the Go version doc (both `1.27.4`).

---

## 6. Gaps in this research

- I did not read all 92 releases: I read the body of `v0.43.0`, `v0.41.0` (truncated in the output, long Fixed section), `v0.30.0`, `v0.5.0`, `v0.0.3`, `drivers/s3/v0.3.0` and the list/metadata of all of them. I did not open `v0.0.1`, `v0.2.0` or `v0.9`..`v0.38` individually (name, date and flag only).
- I did not run or compile vukem; the capabilities come from docs and code read, not from execution. Maturity of `contrib/engine` and `contrib/studio`: I only read the header of `engine.go` and `doc.go`.
- I did not read `docs/architecture.md`, `docs/security.md`, `docs/testing.md` in full; I used `AGENTS.md`, `tooling.md`, `PLAN.md`, `CONTRIBUTING.md`, the PR template.
- I did not verify module tags after 2026-10-18 (`git ls-remote --tags`).
- The version selector and theme toggle of vukem.dev: the WebFetch converted the page to text and did not show them; I did not confirm via browser.
- Upload limits (`MaxBodyBytes` per route) and SQLite performance under concurrent timeline writes: not tested. `web.DefaultMaxBody` is 10 MiB (FACT in `basics/requests.mdx:279`), so video routes need explicit `upload.Rules`/`Tus`.
- `freecut.net/docs`: I did not confirm whether the 30 pages exist in other languages nor the build framework.
