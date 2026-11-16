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
- The scope is optional. When present it is one of `core`, `timeline`, `schema`, `command`, `plan`, `lint`, `engine`, `raster`, `ingest`, `export`, `image`, `transcribe`, `installers`, `service`, `workspace`, `web`, `api`, `mcp`, `cli`, `ui`, `site`, `docs`, `repo`, `ci`, `deps`, `deps-dev`, `release`, `adr`, `sdd`, `go`, `security`, `desktop`, `http`, `ffmpeg`, `renderer`, `docker`, `library`, `events`, `cmd`, `contract`, `pathguard`, `remote`, `timeval`, `store`, `notices`, `projects`, `preview`, `owner` or `e2e`.
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
- `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)`: until the BETA exit the Go suite runs only on macOS (`go test -race`, the coverage gate and `BenchmarkExportCommonCase` with the floor of 1.5 x real time, `VUKEM_VIDEO_BENCH_MIN_RT`); the other two skip it. `task gate -- <tag>` runs the full local release gate
- `Generated files`: `task codegen` leaves `core/schema/` and `web/resources/js/generated/` unchanged; after a change to a generator run `task codegen` and commit
- `Release build (snapshot)`: GoReleaser dry run, six archives under 60,000,000 bytes
- `Reference scenario (ubuntu-latest)`, `(windows-latest)`, `(macos-latest)`: the reference scenario through the CLI process, MCP over stdio and MCP over HTTP against a real `serve`; each export is checked with ffprobe and the three documents are compared; skips where only a GPL FFmpeg exists (macOS)
- `Browser tests (Playwright)`: `bun run e2e` in `web/`: the functional specs and nothing that measures a time budget. The budgets of SDD-012 section 19 (200-item timeline frames, edit round trip, scrub latency, pause to exact frame, video layers, `perf-edit-200`) belong to the reference machine and run only by hand: `VUKEM_VIDEO_PERF=1 bun run e2e` in `web/` adds the `perf` project, which runs alone after the others
- `Browser tests (firefox)`, `Browser tests (webkit)`: the same specs on the other two engines, except those tagged `CHROMIUM_ONLY`; run one by hand with `bunx playwright test -c e2e/playwright.config.ts --project=firefox` in `web/`
- `Bundle export (<os>)`, `Bundle matrix (<os>)`: a bundle of the reference project made on each system is imported on all three, and the document and media hashes must be equal; `bash scripts/bundle-matrix.sh` runs it by hand
- `Sandbox (bwrap)`: `TestSecurity_T16_` must pass, not skip
- `Renderer (ubuntu-latest)`, `(windows-latest)`, `(macos-latest)`: the Rust gate of `renderer/` and the WebAssembly module built twice from clean with equal digests (see [The Rust renderer](#the-rust-renderer))
- `Notices`, `Vulnerabilities`, `Workflow pins`

`Golden cases (PATH FFmpeg)` is reported and does not block. `nightly.yml` runs once a day the long benchmarks, the 60 s export and the soak tests; a failure opens a `ci-failure` issue. `bash scripts/ci-time.sh` prints the median and the maximum of the last 10 pull request runs of `CI` (the budget is a median under 15 minutes and no run over 20, NFR-042). The same checks run on Linux, macOS and Windows. Work in `web/` uses Bun, not npm.

### The Rust renderer

Rust lives only in `renderer/` (ADR-0029, rule `.agents/rules/rust.md`); you need it only to change the renderer.

1. Install [rustup](https://rustup.rs). The first `cargo` call in the repository installs the toolchain pinned in `rust-toolchain.toml` (an exact stable version with `rustfmt`, `clippy` and the `wasm32-unknown-unknown` target).
2. Install the two tools at the versions of CI: `cargo install --locked cargo-deny --version 0.20.2` and `cargo install --locked wasm-bindgen-cli --version <the wasm-bindgen version in renderer/Cargo.lock>`. `build-renderer.sh` refuses a CLI that differs from the lockfile.
3. Before every commit that touches `renderer/`, in `renderer/`: `cargo fmt --check`, `cargo clippy --locked --all-targets -- -D warnings`, `cargo test --locked`, `cargo deny check`.
4. `bash scripts/build-renderer.sh` builds the native crate and the module into `renderer/dist/`; `bash scripts/build-renderer.sh verify` builds the module twice from clean and fails unless the digests match.
5. Crates are at exact versions (`=x.y.z`) and `Cargo.lock` is committed. Only `wgpu`, `wasm-bindgen`, `bytemuck` and `cargo-deny` are approved; any other crate needs the maintainer's approval. The licence allowlist of `renderer/deny.toml` must stay within the one of `internal/tools/notices`, which also lists the shipped crates in `THIRD_PARTY_NOTICES.md` (`task notices`).

`bash scripts/gate.sh` runs all of this as its `rust` step when `cargo` is installed.

### Third-party notices

`THIRD_PARTY_NOTICES.md` and the licence texts in `licenses/go-modules/` and `licenses/npm/` are generated (SDD-009 section 12). When a change touches `go.mod`, `go.sum`, `web/package.json` or `web/bun.lock`, or an entry of `.config/third-party.json`, run `task notices` and commit the result; `task notices:check` (the `Notices` job of CI) fails with a diff when they are stale, and with the module name when a licence is outside the allow-list (MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, MPL-2.0, 0BSD, Unlicense, OFL-1.1 for fonts, LGPL-3.0-or-later for FFmpeg only, and, pending the author's confirmation, the SQLite public-domain dedication, BlueOak-1.0.0 and CC-BY-4.0 for npm data). `licenses/fonts/`, `licenses/programs/` and `licenses/vendored/` are kept by hand and listed in the inventory; a new bundled font, program or copied source adds its entry and its licence file there. `task notices` needs `bun install --frozen-lockfile` in `web/` (it runs it).

### Container image

`task image:build` builds `.config/docker/vukem-video.Dockerfile` as `vukem-video:local`, and `task image:smoke` runs `scripts/image-smoke.sh` on it: user 10001, `VUKEM_VIDEO_HOME=/data` with a volume, FFmpeg without `--enable-gpl` or `--enable-nonfree` that encodes with `libopenh264`, `prlimit`, the licence files under `/usr/share/doc/vukem-video/`, `/up` on a loopback port, and the CLI scenario with `--network none`. Docker is required. Build from a clone: from a git worktree the `.git` file points outside the build context, so the image gets no version stamp and the version check is skipped. `image.yml` builds on pull requests that change what goes into the image and pushes nothing; `release.yml` publishes. To check a version by hand: `docker build --build-arg VERSION=0.3.0 ...` after `git tag v0.3.0` on a clean tree, then `bash scripts/image-smoke.sh <image> 0.3.0`.

### The own FFmpeg build

Before 1.0 the project builds its own LGPL FFmpeg (D-017, ADR-0011) with no OpenH264 linked: a patch loads Cisco's library at run time with `dlopen` (SDD-011 section 8.4). The inputs are in `.config/ffmpeg-build/`: `sources.env` (pinned source tarballs, the four OpenH264 headers and the pinned patch, with SHA-256), `patches/0001-libopenh264-dlopen.patch` (the loader), `configure-flags.txt` (the only place of the configure flags) and `configure-allowlist.txt` (the `--enable-*` flags a build may carry). `bash scripts/ffmpeg-build.sh <os> <arch> <out>` builds one platform; `go run ./internal/tools/ffmpegconf -allow .config/ffmpeg-build/configure-allowlist.txt -flags .config/ffmpeg-build/configure-flags.txt` checks the flags without building. `ffmpeg-build.yml` (on demand) builds every platform twice, compares the digests, verifies each build with `bash scripts/ffmpeg-verify.sh archive <file>`, signs the assets with cosign and attests them. When a release exists, fill the `FFMPEG_OWN_*` values of `.config/ffmpeg.env` and set `FFMPEG_SOURCE="own"` (the image takes `--build-arg FFMPEG_SOURCE=own`, `scripts/install-ffmpeg.sh` takes the `FFMPEG_SOURCE` variable); `bash scripts/ffmpeg-verify.sh signatures <dir>` holds the `cosign verify-blob` and `gh attestation verify` commands. On Windows and Linux x86_64 it also enables the hardware encoders NVENC (`nv-codec-headers`), Quick Sync (`libvpl`, built static, needs cmake) and AMD AMF (headers only) (D-034); `ffmpeg-verify.sh` requires their encoders there. A new `--enable-*` flag needs the author: check the licence first.

### Releases

During ALPHA a release is one `v0.N.0` per group of `ROADMAP.md` (kept in the workspace root, outside this repository) that ships something runnable, plus the rule of ADR-0028: about one minor every 90 commits on `main`. Minors may carry breaking changes; a patch never does, and `release prepare` refuses a patch version when a commit in the range is marked `!`.

`v1.0.0` is cut directly from the BETA exit: there is no `-rc` tag (ADR-0025 item 2, SDD-009 section 4). From then on `release prepare` also refuses a minor or a patch when a commit in the range carries a `!` header or a `BREAKING CHANGE:` footer; the next major (`v2.0.0`) is accepted.

### Release checklist

Before the release tag, the maintainer runs the agent harness and attaches its output to the release:

- [ ] Run the workflow `agent-harness` (Actions, "Run workflow") with the defaults: scenario `all`, 10 runs, model `claude-sonnet-5-5` (`internal/tools/agentharness`, SDD-003 section 9 check 29). It gives an agent only `SKILL.md` and the CLI, and runs the Prototype scenario and the template scenario 10 times each. Each scenario must complete in at least 9 of 10 runs.
- [ ] Attach the 20 transcripts to the release: download the artifact `agent-harness-results` (folder `transcripts/`, with `summary.json`) and upload it to the GitHub release.
- [ ] Record the vendor token count of `tools/list` that the same run logs (`tools-list-tokens.log`, at most 6,000) in the release notes.

The workflow calls the Anthropic API, so it costs money. It runs only when started by hand, never on a schedule, a push or a pull request. It needs the repository secret `ANTHROPIC_API_KEY`; without it the first step fails and names the secret. Locally the same run is `VUKEM_VIDEO_HARNESS=1 ANTHROPIC_API_KEY=... go run ./internal/tools/agentharness --runs 10 --bin ./vukem-video --clip ./clip.mp4 --out ./transcripts`, and the token count is `VUKEM_VIDEO_BUDGET_RELEASE=1 ANTHROPIC_API_KEY=... ANTHROPIC_MODEL=<model> go test ./web/app/mcp -run TestToolListVendorTokens -v`.

### What 1.0 promises under SemVer

From `v1.0.0` a minor or a patch release keeps working for everything below. Breaking any of it is a major release, and the commit is marked `!`.

- The MCP tool names, their input and output schemas and their annotations.
- The CLI commands, their flags and their exit codes.
- The HTTP API: routes, methods, request and response schemas, error codes and statuses.
- The timeline document schema: a document written by an older 1.x opens in a newer 1.x, and unknown fields are preserved.
- The workspace layout and its migrations: a newer 1.x opens the workspace of an older 1.x and migrates it forward without losing data.

`web/contract/testdata/surface.txt` lists the MCP tools, CLI commands and HTTP routes, and `TestSurfaceFrozen` (`go test ./web/contract/ -run TestSurfaceFrozen`) fails when a name is removed or renamed. A new name needs `UPDATE_SURFACE=1` in the same change; a removal needs a deliberate edit of the file and a major version.

Not promised, and free to change in any release: the wording of the text formats (concise and full views, edit and lint texts, error messages and hints; the codes are promised, the sentences are not), the golden text files that pin that wording, and every internal package (`core/`, `engine/`, `service/`, `internal/`; the module is an application and nobody imports it).

- `task release:preflight` runs `task ci`, `task generate:check`, `task site:build`, `task site:translations` and `task image:smoke`; run it on the release branch before `release prepare`.
- `task release:binaries SNAPSHOT=1` builds the six archives and their SBOMs in `dist/` without publishing or signing (GoReleaser and syft on `PATH`); each archive holds `LICENSE.md`, `README.md`, `CHANGELOG.md`, `THIRD_PARTY_NOTICES.md`, `licenses/` and the three files under `agent/`. `task release:snapshot` does the same without the SBOMs.
- Pushing the tag is the only manual step: `release.yml` publishes the notes, the archives, the signed checksums, the provenance and the image. `task release:verify -- vX.Y.Z` (`scripts/release-verify.sh`) then checks the signature, the checksums, the attestation and the image digests; add `--no-image` for a release that publishes no image, `--no-pages` while GitHub Pages has not deployed the feed yet, and `--dispatched` for a run started by `workflow_dispatch`. The release also carries the installers of the desktop application (`desktop.yml`) and, for a stable tag, `feed.json`; `scripts/desktop-verify.sh vX.Y.Z <installer>` is how a person checks one download.

#### Secrets and variables of the desktop release

The BETA desktop installers are not signed with a paid certificate (D-033), so there are no code signing secrets. The desktop auto-update feed (SDD-009 sections 10.5 and 10.6) needs one secret and one variable, which the maintainer adds in the repository settings:

| Name | Kind | Value | Used by |
| --- | --- | --- | --- |
| `UPDATE_FEED_SIGNING_KEY` | secret | the 64 hex characters of an Ed25519 seed; never committed, never on a developer machine after it is stored | the `preflight` and `feed` jobs of `release.yml` |
| `UPDATE_FEED_PUBLIC_KEY` | variable | the public half of the seed, 64 hex characters (`UPDATE_FEED_SIGNING_KEY=<seed> go run ./internal/tools/updatefeed pubkey` prints it) | `desktop.yml`, which builds it into the application, and `release-verify.sh` |

A stable tag without either fails in the first minutes of `release.yml`, naming the missing one; a pre-release needs neither. `SITE_URL` (a variable, optional) moves the host of the feed together with the site. Rotating the key needs a new desktop release signed with the old one (SDD-009 section 10.5).

## Editor settings

Indentation is tabs of width 4, except in files whose format needs spaces (YAML, Markdown lists and tables). Git expects LF endings in the repository.

## Security-sensitive changes

A change that touches authentication, path handling, the FFmpeg invocation policy or anything that opens a network connection needs a regression test and a `Security:` footer. Read [SDD-007](../docs/sdds/007-security-model.md) first.
