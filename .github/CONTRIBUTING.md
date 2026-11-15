# Contributing to Vukem Video

Thank you for helping. Vukem Video is in the Prototype phase, so most of the code does not exist yet; the planning documents in [`docs/`](../docs/README.md) are the contract. Start with the requirements ([`docs/prds/`](../docs/prds)) and the specification of the area you will touch ([`docs/sdds/`](../docs/sdds)).

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities privately, as described in [SECURITY](SECURITY.md), never in a public issue.

## Rules that are never traded

1. **One command core.** Every editing rule lives in `core/` (Go, pure, no I/O). The UI, the MCP server, the CLI and the HTTP API are thin adapters over the service. A rule about timelines written in a handler, a tool or in TypeScript is rejected in review, even "for now".
2. **The UI never computes an edit result.** It draws what the core says and sends commands. `bun run check:thin-client` (in `web/`) enforces it.
3. **Names come from one place.** Operations, tables, error codes and variables are in [`docs/sdds/000-shared-definitions.md`](../docs/sdds/000-shared-definitions.md). Change it first, then everything else.
4. **A specification is the contract.** When the code must differ from an SDD, fix the SDD in the same pull request and say why.
5. **No outbound network connection by default, no telemetry.**
6. **No new dependency without the maintainer's approval.** Versions are exact, never `latest`, `next`, `rc`, `beta` or `alpha`.
7. **Everything in the repository is in English.**

## Commits and pull request titles

Conventional Commits 1.0 in English:

```text
<type>(<scope>)!: <description>

[body]

[footer(s)]
```

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style`, `revert`.
- The description is imperative, starts with a lower-case letter, has no trailing period, and the header is at most 100 characters.
- The scope is optional. When present it is one of `core`, `timeline`, `schema`, `command`, `plan`, `lint`, `engine`, `raster`, `ingest`, `export`, `image`, `transcribe`, `installers`, `service`, `workspace`, `web`, `api`, `mcp`, `cli`, `ui`, `site`, `docs`, `repo`, `ci`, `deps`, `deps-dev`, `release`, `adr`, `sdd`, `go`, `security`.
- A breaking change has `!` after the type or scope and a `BREAKING CHANGE:` footer that describes the impact.
- A security change on a `feat`, `fix`, `perf` or `refactor` has a `Security: <one line on the risk closed>` footer.
- The task or the issue goes in the body: `Refs: P1-ENG-003` or `Fixes #123`.
- One logical change per commit and per pull request.

Pull requests are rebase-merged (ADR-0028): every commit of the branch reaches `main` as it is and is what the changelog reads, so each commit must be a small, self-contained Conventional Commit, and the footers above go in the commit that carries the change. Fold fixup commits before asking for the merge.

Release notes have five sections only (D-027), so pick the commit type with that in mind: `feat` goes to **Added**, a commit marked breaking (`!` or `BREAKING CHANGE:`) to **Changed**, `revert` or a `Removed:` footer to **Removed**, `fix` to **Fixed**, and a `Security:` footer or the scope `security` to **Security**. Every other type is left out of the notes. A release is cut about every 90 commits on `main` (ADR-0028). `CHANGELOG.md` is generated from the commits by the release tool and is not edited by hand.

## Pull requests and completion

1. The maintainers gather a release in one pull request, `release/vX.Y.Z` to `main`, titled `chore(release): vX.Y.Z updates`, with about 90 microcommits (D-031). A contribution from outside is a branch whose commits the maintainer integrates into the open release branch; open your pull request against `release/vX.Y.Z` when one is open.
2. Fill in the pull request template: the task or issue, the checks you ran, the thin-client rule and, if you found friction with the vukem framework, its record.
3. A report that something works is a claim, not evidence. A change is done when its output exists and is tested, and every check of its task has been run and its result is visible in the pull request. A change that cannot meet a check is not silently changed: explain why in the pull request.
4. The maintainer reviews, fixes the last defects, integrates the commits into the release branch, refreshes the release pull request body with `task release:pr-body -- vX.Y.Z` (sections Added, Changed, Removed, Fixed, Security with linked commit hashes), rebase-merges it and pushes the tag. Nobody else pushes to `main`.

## Checks

Run `task ci` before you push: `task lint`, `task vet`, `task test`, `task tidy:check` and `task notices:check`. The pull request pipeline (`ci.yml`) runs these jobs, each with `timeout-minutes: 20`; the ruleset requires them, so a name below is the name of the check:

- `Lint (Go)`, `Lint (Markdown)`, `Types and lint (web)`
- `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)`: `go test -race`, the coverage gate on Linux, and `BenchmarkExportCommonCase` with the floor of 1.5 x real time (`VUKEM_VIDEO_BENCH_MIN_RT`)
- `Generated files`: `task codegen` leaves `core/schema/` and `web/resources/js/generated/` unchanged; after a change to a generator run `task codegen` and commit
- `Release build (snapshot)`: GoReleaser dry run, six archives under 60,000,000 bytes
- `Reference scenario (ubuntu-latest)`, `(windows-latest)`, `(macos-latest)`: the reference scenario through the CLI process, MCP over stdio and MCP over HTTP against a real `serve`; each export is checked with ffprobe and the three documents are compared; skips where only a GPL FFmpeg exists (macOS)
- `Browser tests (Playwright)`: `bun run e2e` in `web/`: the functional specs and nothing that measures a time budget. The budgets of SDD-012 section 19 (200-item timeline frames, edit round trip, scrub latency, pause to exact frame, video layers, `perf-edit-200`) belong to the reference machine and run only by hand: `VUKEM_VIDEO_PERF=1 bun run e2e` in `web/` adds the `perf` project, which runs alone after the others
- `Sandbox (bwrap)`: `TestSecurity_T16_` must pass, not skip
- `Notices`, `Vulnerabilities`, `Workflow pins`

`Golden cases (PATH FFmpeg)` is reported and does not block. `nightly.yml` runs once a day the long benchmarks, the 60 s export and the soak tests; a failure opens a `ci-failure` issue. `bash scripts/ci-time.sh` prints the median and the maximum of the last 10 pull request runs of `CI` (the budget is a median under 15 minutes and no run over 20, NFR-042). The same checks run on Linux, macOS and Windows. Work in `web/` uses Bun, not npm.

### Third-party notices

`THIRD_PARTY_NOTICES.md` and the licence texts in `licenses/go-modules/` and `licenses/npm/` are generated (SDD-009 section 12). When a change touches `go.mod`, `go.sum`, `web/package.json` or `web/bun.lock`, or an entry of `.config/third-party.json`, run `task notices` and commit the result; `task notices:check` (the `Notices` job of CI) fails with a diff when they are stale, and with the module name when a licence is outside the allow-list (MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, MPL-2.0, 0BSD, Unlicense, OFL-1.1 for fonts, LGPL-3.0-or-later for FFmpeg only, and, pending the author's confirmation, the SQLite public-domain dedication, BlueOak-1.0.0 and CC-BY-4.0 for npm data). `licenses/fonts/`, `licenses/programs/` and `licenses/vendored/` are kept by hand and listed in the inventory; a new bundled font, program or copied source adds its entry and its licence file there. `task notices` needs `bun install --frozen-lockfile` in `web/` (it runs it).

### Container image

`task image:build` builds `.config/docker/vukem-video.Dockerfile` as `vukem-video:local`, and `task image:smoke` runs `scripts/image-smoke.sh` on it: user 10001, `VUKEM_VIDEO_HOME=/data` with a volume, FFmpeg without `--enable-gpl` or `--enable-nonfree` that encodes with `libopenh264`, `prlimit`, the licence files under `/usr/share/doc/vukem-video/`, `/up` on a loopback port, and the CLI scenario with `--network none`. Docker is required. Build from a clone: from a git worktree the `.git` file points outside the build context, so the image gets no version stamp and the version check is skipped. `image.yml` builds on pull requests that change what goes into the image and pushes nothing; `release.yml` publishes. To check a version by hand: `docker build --build-arg VERSION=0.3.0 ...` after `git tag v0.3.0` on a clean tree, then `bash scripts/image-smoke.sh <image> 0.3.0`.

### Releases

During ALPHA a release is one `v0.N.0` per group of `ROADMAP.md` (kept in the workspace root, outside this repository) that ships something runnable, plus the rule of ADR-0028: about one minor every 90 commits on `main`. Minors may carry breaking changes; a patch never does, and `release prepare` refuses a patch version when a commit in the range is marked `!`.

- `task release:preflight` runs `task ci`, `task generate:check`, `task site:build`, `task site:translations` and `task image:smoke`; run it on the release branch before `release prepare`.
- `task release:binaries SNAPSHOT=1` builds the six archives and their SBOMs in `dist/` without publishing or signing (GoReleaser and syft on `PATH`); each archive holds `LICENSE.md`, `README.md`, `CHANGELOG.md`, `THIRD_PARTY_NOTICES.md`, `licenses/` and the three files under `agent/`. `task release:snapshot` does the same without the SBOMs.
- Pushing the tag is the only manual step: `release.yml` publishes the notes, the archives, the signed checksums, the provenance and the image. `task release:verify -- vX.Y.Z` (`scripts/release-verify.sh`) then checks the signature, the checksums, the attestation and the image digests; add `--no-image` for a release that publishes no image, and `--dispatched` for a run started by `workflow_dispatch`.

## Editor settings

Indentation is tabs of width 4, except in files whose format needs spaces (YAML, Markdown lists and tables). Git expects LF endings in the repository.

## Security-sensitive changes

A change that touches authentication, path handling, the FFmpeg invocation policy or anything that opens a network connection needs a regression test and a `Security:` footer. Read [SDD-007](../docs/sdds/007-security-model.md) first.
