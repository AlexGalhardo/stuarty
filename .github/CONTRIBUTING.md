# Contributing to Stuarty

Thank you for helping. Stuarty is where a creator plans, creates, edits, manages and publishes content in one place, self-hosted, with AI agents as first-class users. The planning documents in [`docs/`](../docs/README.md) are the contract. Start with the requirements ([`docs/prds/`](../docs/prds)), the decisions ([`docs/adrs/`](../docs/adrs)) and the specification of the area you will touch ([`docs/sdds/`](../docs/sdds)). The work is the epics of [`docs/plans/`](../docs/plans/00-roadmap.md).

The product was called Stuarty until 2026-11-18. The code still carries the former name (`stuarty`, `STUARTY_*`) until the rename epic of ADR-0044 is merged; commands in this guide use the names the code accepts today.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities privately, as described in [SECURITY](SECURITY.md), never in a public issue.

## Rules that are never traded

1. **One command core.** Every rule of a domain (video, image and the others of ADR-0031) lives in `core/` (Go, pure, no I/O). The UI, the MCP server, the CLI and the HTTP API are thin adapters over the service. A rule written in a handler, a tool or in TypeScript is rejected in review, even "for now".
2. **The UI never computes an edit result.** It draws what the core says and sends commands. `bun run check:thin-client` (in `web/`) enforces it.
3. **Names come from one place.** Operations, tables, error codes and variables are in [`docs/sdds/000-shared-definitions.md`](../docs/sdds/000-shared-definitions.md). Change it first, then everything else.
4. **A specification is the contract.** When the code must differ from an SDD, fix the SDD in the same change and say why.
5. **Outbound only to connected destinations, approval before publishing, no telemetry** (ADR-0032). With no connection configured the product opens no outbound connection.
6. **No new dependency without the maintainer's approval.** Versions are exact, never `latest`, `next`, `rc`, `beta` or `alpha`.
7. **Everything in the repository is in English.**

## Commits

Conventional Commits 1.0 in English:

```text
<type>(<scope>)!: <description>

[body]

[footer(s)]
```

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style`, `revert`.
- The description is imperative, starts with a lower-case letter, has no trailing period, and the header is at most 100 characters.
- The scope is optional. When present it is one of `core`, `timeline`, `schema`, `command`, `plan`, `lint`, `engine`, `raster`, `ingest`, `export`, `image`, `transcribe`, `installers`, `service`, `workspace`, `web`, `api`, `mcp`, `cli`, `ui`, `site`, `docs`, `repo`, `ci`, `deps`, `deps-dev`, `release`, `adr`, `sdd`, `go`, `security`, `desktop`, `http`, `ffmpeg`, `renderer`, `docker`, `library`, `events`, `cmd`, `contract`, `pathguard`, `remote`, `timeval`, `store`, `notices`, `projects`, `preview`, `owner` or `e2e`. A new domain adds its scope to the three places named in `.agents/rules/commits.md`.
- A breaking change has `!` after the type or scope and a `BREAKING CHANGE:` footer that describes the impact.
- A security change on a `feat`, `fix`, `perf` or `refactor` has a `Security: <one line on the risk closed>` footer.
- The epic or the issue goes in the body: `Refs: epic 07 task 3` or `Fixes #123`.
- One logical change per commit.

The commit type decides the section of the release notes when releases are cut again: `feat` goes to **Added**, a commit marked breaking to **Changed**, `revert` or a `Removed:` footer to **Removed**, `fix` to **Fixed**, and a `Security:` footer or the scope `security` to **Security**. `CHANGELOG.md` is generated from the commits and is not edited by hand.

## How a change is delivered

The process is ADR-0045.

1. The maintainers work on one integration branch that carries one pull request. Each epic of `docs/plans/` is a branch from it. A contribution from outside is a branch from the integration branch, with a pull request against it.
2. A report that something works is a claim, not evidence. A change is done when its output exists and is tested, and every acceptance criterion of its epic has been run with its result visible. A change that cannot meet a criterion is not silently changed: say why.
3. An independent reviewer reads the epic, its acceptance criteria and the diff, runs the checks again, fixes what it finds and approves. Review rules are in [`REVIEW.md`](../REVIEW.md).
4. The maintainer merges the approved epic into the integration branch and marks it done in the roadmap. Nobody else pushes to `main`.
5. If you found friction with the vukem framework, say so in the pull request (ADR-0047).

## Checks

CI is not a gate while the epics are implemented (ADR-0045), so the checks are run on your machine and their output goes with the change:

- `task lint`: the Go, TypeScript and Markdown linters.
- `task vet`, `task test` and, for `core/`, `service/` and `web/app/`, `task test:race`.
- `task test:web`: the web unit tests; `bun run typecheck` and `bun run check:thin-client` in `web/`.
- `task tidy:check` and `task notices:check` when dependencies changed; `task codegen` when a contract changed.
- The tests of what you touched, including the browser specs in `web/e2e/` for a change of the UI.
- `bash .agents/skills/verify/scripts/verify.sh --quick` before a commit.

Work in `web/` uses Bun, not npm.

The pull-request pipeline (`ci.yml`) still exists and will be required again. Its jobs, for reference:

- `Lint (Go)`, `Lint (Markdown)`, `Types and lint (web)`
- `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)`: the Go suite with the race detector, the coverage gate and `BenchmarkExportCommonCase` with the floor of 1.5 x real time (`STUARTY_BENCH_MIN_RT`). `task gate -- <tag>` runs the full local gate
- `Generated files`: `task codegen` leaves `core/schema/` and `web/resources/js/generated/` unchanged; after a change to a generator run `task codegen` and commit
- `Release build (snapshot)`: GoReleaser dry run, six archives under 60,000,000 bytes
- `Reference scenario (ubuntu-latest)`, `(windows-latest)`, `(macos-latest)`: the reference scenario through the CLI process, MCP over stdio and MCP over HTTP against a real `serve`; each export is checked with ffprobe and the three documents are compared; skips where only a GPL FFmpeg exists (macOS)
- `Browser tests (Playwright)`: `bun run e2e` in `web/`: the functional specs and nothing that measures a time budget. The budgets of SDD-012 section 19 (200-item timeline frames, edit round trip, scrub latency, pause to exact frame, video layers, `perf-edit-200`) belong to the reference machine and run only by hand: `STUARTY_PERF=1 bun run e2e` in `web/` adds the `perf` project, which runs alone after the others
- `Browser tests (firefox)`, `Browser tests (webkit)`: the same specs on the other two engines, except those tagged `CHROMIUM_ONLY`; run one by hand with `bunx playwright test -c e2e/playwright.config.ts --project=firefox` in `web/`
- `Bundle export (<os>)`, `Bundle matrix (<os>)`: a bundle of the reference project made on each system is imported on all three, and the document and media hashes must be equal; `bash scripts/bundle-matrix.sh` runs it by hand
- `Sandbox (bwrap)`: `TestSecurity_T16_` must pass, not skip
- `Renderer (ubuntu-24.04)`: the Rust gate of `renderer/` and the WebAssembly module built twice from clean with equal digests and equal to the embedded one (see [The Rust renderer](#the-rust-renderer))
- `Notices`, `Vulnerabilities`, `Workflow pins`

`Golden cases (PATH FFmpeg)` is reported and does not block. `nightly.yml` runs once a day the long benchmarks, the 60 s export and the soak tests.

### The Rust renderer

Rust lives only in `renderer/` (ADR-0029, rule `.agents/rules/rust.md`); you need it only to change the renderer.

1. Install [rustup](https://rustup.rs). The first `cargo` call in the repository installs the toolchain pinned in `rust-toolchain.toml` (an exact stable version with `rustfmt`, `clippy` and the `wasm32-unknown-unknown` target).
2. Install the tool at the version of CI: `cargo install --locked cargo-deny --version 0.20.2`. The module has no wasm-bindgen glue, so no wasm-bindgen CLI is needed.
3. Before every commit that touches `renderer/`, in `renderer/`: `cargo fmt --check`, `cargo clippy --locked --all-targets -- -D warnings`, `cargo test --locked`, `cargo deny check`.
4. `bash scripts/build-renderer.sh` builds the native crate and the module into `renderer/dist/`; `bash scripts/build-renderer.sh verify` builds the module twice from clean and fails unless the digests match; `bash scripts/build-renderer.sh check` also fails unless the module embedded in `engine/renderhost/` is the one this source builds.
5. Crates are at exact versions (`=x.y.z`) and `Cargo.lock` is committed. The approved crates are listed in `.agents/rules/rust.md`; any other crate needs the maintainer's approval. The licence allowlist of `renderer/deny.toml` must stay within the one of `internal/tools/notices`, which also lists the shipped crates in `THIRD_PARTY_NOTICES.md` (`task notices`).

`bash scripts/gate.sh` runs all of this as its `rust` step when `cargo` is installed.

### Third-party notices

`THIRD_PARTY_NOTICES.md` and the licence texts in `licenses/go-modules/` and `licenses/npm/` are generated (SDD-009 section 12). When a change touches `go.mod`, `go.sum`, `web/package.json` or `web/bun.lock`, or an entry of `.config/third-party.json`, run `task notices` and commit the result; `task notices:check` fails with a diff when they are stale, and with the module name when a licence is outside the allow-list (MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, MPL-2.0, 0BSD, Unlicense, OFL-1.1 for fonts, LGPL-3.0-or-later for FFmpeg only, and, pending the author's confirmation, the SQLite public-domain dedication, BlueOak-1.0.0 and CC-BY-4.0 for npm data). `licenses/fonts/`, `licenses/programs/` and `licenses/vendored/` are kept by hand and listed in the inventory; a new bundled font, program or copied source adds its entry and its licence file there. `task notices` needs `bun install --frozen-lockfile` in `web/` (it runs it).

### Container image

`task image:build` builds `.config/docker/stuarty.Dockerfile` as `stuarty:local`, and `task image:smoke` runs `scripts/image-smoke.sh` on it: user 10001, `STUARTY_HOME=/data` with a volume, FFmpeg without `--enable-gpl` or `--enable-nonfree` that encodes with `libopenh264`, `prlimit`, the licence files under `/usr/share/doc/stuarty/`, `/up` on a loopback port, and the CLI scenario with `--network none`. Docker is required. Build from a clone: from a git worktree the `.git` file points outside the build context, so the image gets no version stamp and the version check is skipped.

### The own FFmpeg build

The project builds its own LGPL FFmpeg (author decision D-017, ADR-0011) with no OpenH264 linked: a patch loads Cisco's library at run time with `dlopen` (SDD-011 section 8.4). The inputs are in `.config/ffmpeg-build/`: `sources.env` (pinned source tarballs, the four OpenH264 headers and the pinned patch, with SHA-256), `patches/0001-libopenh264-dlopen.patch` (the loader), `configure-flags.txt` (the only place of the configure flags) and `configure-allowlist.txt` (the `--enable-*` flags a build may carry). `bash scripts/ffmpeg-build.sh <os> <arch> <out>` builds one platform; `go run ./internal/tools/ffmpegconf -allow .config/ffmpeg-build/configure-allowlist.txt -flags .config/ffmpeg-build/configure-flags.txt` checks the flags without building. `ffmpeg-build.yml` (on demand) builds every platform twice, compares the digests, verifies each build with `bash scripts/ffmpeg-verify.sh archive <file>`, signs the assets with cosign and attests them. On Windows and Linux x86_64 it also enables the hardware encoders NVENC (`nv-codec-headers`), Quick Sync (`libvpl`, built static, needs cmake) and AMD AMF (headers only) (author decision D-034); `ffmpeg-verify.sh` requires their encoders there. A new `--enable-*` flag needs the author: check the licence first.

## Releases

No release is cut while the epics are implemented (ADR-0045). When they are done, the integration pull request is split into releases starting at `v0.30.0`, the first one under the name Stuarty (ADR-0044), and the rules of ADR-0028 return, revised. What stays true meanwhile:

- Versions follow SemVer. Stuarty is in `0.x`: a minor release may break, a patch never does.
- `web/contract/testdata/surface.txt` lists the MCP tools, CLI commands and HTTP routes, and `TestSurfaceFrozen` (`go test ./web/contract/ -run TestSurfaceFrozen`) fails when a name is removed or renamed. A new name needs `UPDATE_SURFACE=1` in the same change; a removal needs a deliberate edit of the file and a commit marked `!`.
- A document or a workspace written by a released version opens in every later version (PRD-001 NFR-012, PRD-003 RF-208).
- The wording of the text formats (views, edit and lint texts, error messages and hints) is free to change; the error codes are not.

The release tooling is unchanged and documented in SDD-009: `task release:preflight`, `task release:binaries SNAPSHOT=1`, `task release:verify -- vX.Y.Z`, the agent harness (`internal/tools/agentharness`, which calls a paid API and runs only by hand) and the desktop update feed, whose signing key is the repository secret `UPDATE_FEED_SIGNING_KEY` with its public half in the variable `UPDATE_FEED_PUBLIC_KEY`. The desktop installers are not signed with a paid certificate (author decision D-033).

## Editor settings

Indentation is tabs of width 4, except in files whose format needs spaces (YAML, Markdown lists and tables). Git expects LF endings in the repository.

## Security-sensitive changes

A change that touches authentication, the secret vault, a connection to an outbound destination, the approval queue, path handling, the FFmpeg invocation policy or anything that opens a network connection needs a regression test and a `Security:` footer. Read [SDD-007](../docs/sdds/007-security-model.md) and ADR-0032 first.
