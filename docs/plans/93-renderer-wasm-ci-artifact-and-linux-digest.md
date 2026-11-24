# 93: One renderer module digest on Linux and Windows

## Goal

The WebAssembly module of the renderer that Stuarty embeds is built to the same bytes on the Linux runner and on a creator's or a developer's Windows machine, so that `bash scripts/build-renderer.sh check` means the same thing everywhere: the module in the binary is the one the source builds. Today it does not: the pinned digest does not reproduce on Windows.

## Context and related decisions

- Author decisions D-127 and D-135 (item 3): the `Renderer` CI job uploads the `.wasm` as an artifact, it is compared with the Windows build, and the digest of the Linux build is pinned again. A workflow change goes in its own commit.
- Issue VV-276: `bash scripts/build-renderer.sh` on Windows 11 gives a digest other than the one pinned in `engine/renderhost/renderer.wasm.sha256` (`506368e3...` against the pinned one of the time), at a source with no change in `renderer/`, also in a detached worktree of the integration branch. `build-renderer.sh verify` proves only that two builds on one machine agree.
- What exists, checked against the repository before this epic was written:
  - The `Renderer (ubuntu-24.04)` job of `.github/workflows/ci.yml` runs the Rust gate and `bash scripts/build-renderer.sh check`, and **already uploads** `renderer/dist/stuarty_renderer.wasm` and its `.sha256` as the artifact `renderer-wasm-linux` with `if: always()` (commit `ee191ff36`), so the module is kept even when the check fails. The first item of D-135 is therefore done; what is not done is the comparison, the fix of what differs and the pin.
  - `scripts/build-renderer.sh` has the reproducible build of rule 9 of `.agents/rules/rust.md`: `--locked`, a fixed target folder, `SOURCE_DATE_EPOCH` from the commit time of HEAD, one `--remap-path-prefix` per source folder (`source_remaps`, written for the Windows case: a remap rewrites the start of a path and keeps the separator that follows), and two guards that fail the build when a machine path or a backslash source path is left in the module. Commit `e6c20ba62` gave rustc forward slashes instead of rewriting the module afterwards.
  - The embedded module and its digest: `engine/renderhost/renderer.wasm` and `renderer.wasm.sha256`, written by `build-renderer.sh embed` on the author's Windows machine; the Go host and the browser check the digest (`module_digest`, SDD-013 sections 4.1 and 5).
  - Epic 49 task 7 ("the pinned digest") waits for this. Any change under `renderer/` changes the digest: epic 91 changes it again, so the final pin is taken after epic 91 merges, or repeated.
- ADR-0029, `.agents/rules/rust.md` (rules 5 and 9: bit-exact CPU path; reproducible builds), SDD-013 section 4.1 (the module is a build artifact), ADR-0045 (CI is not a gate while the epics are implemented; the orchestrator pushes and runs it).
- Outbound requests: none by the senior. Anything that needs a run of CI or the download of its artifact (`gh run download`) is done by the orchestrator, who pushes the branch; the tasks say so.

## User stories

- As a developer on Windows, I want `check` to pass on my machine when it passes on CI, so that I do not re-embed a module that differs for no reason.
- As the team, I want the pinned digest to be the one the Linux runner builds, because the runner is what releases are built from.
- As a user who verifies a release, I want the module in the binary to be reproducible from the source.

## Scope

- Modules: `scripts/build-renderer.sh`, `.github/workflows/ci.yml` (the `renderer` job, in its own commit; a shared file, touched by one epic at a time), `engine/renderhost/renderer.wasm` and `renderer.wasm.sha256`, a small tool `internal/tools/wasmdiff` (standard library only) that compares two modules section by section, SDD-013 section 4.1 and `.agents/rules/rust.md` rule 9 if the build rule changes, and a record under `docs/quality/`.
- Must not touch: the Rust source (`renderer/stuarty-renderer*/src`), the host code, the exports of the module, the golden files. A cause in the Rust source (a `HashMap` order, a path in a panic message that comes from the source rather than the build) is a STOP: it is an epic-91-like change in `renderer/`.

## Out of scope

- A macOS build or digest (epic 43); the native GPU host binary `stuarty-render` (it is not reproducible across systems and is not embedded); signing and the release pipeline; a Docker-based build for Windows (a possible fallback, a decision for the author).
- Changing what the module does.

## Dependencies

55 (the last merged change of `renderer/`), and 91 for the final pin: the tasks that investigate and fix the build do not depend on 91; the re-pin of task 6 is done on the source of the day and repeated by epic 91 when it merges (its task 3 embeds the module). Not in progress together with another epic that edits `.github/workflows/ci.yml` or `scripts/build-renderer.sh`.

## Surfaces

None: build scripts, CI and a build artifact. The binary embeds a different module only if the digest changes, with the same behaviour.

## Core commands and permissions

None.

## Technical design

- **Compare before changing anything.** Three modules from the same commit: the one embedded (`engine/renderhost/renderer.wasm`), the one `bash scripts/build-renderer.sh wasm` builds on Windows, and the artifact `renderer-wasm-linux` of the `Renderer` job. `wasmdiff` reads the WebAssembly binary format (the sections, the code entries, the data segments, the custom sections), prints each section's size and SHA-256, the first byte where two modules differ and which section, function body or data segment holds it, and the text of a differing data segment (panic locations and `core`/`alloc` source paths are the likely cause). A test builds two small modules that differ in one data segment and asserts the report names it.
- **Fix the cause where it is.** The likely causes, to be confirmed by the comparison and not assumed: a source path that the remaps do not cover on one system (the registry folder of a crate, a standard library path under the toolchain folder), the order of the data segments (the linker orders panic locations by content, so a path with a different separator reorders them), a different `SOURCE_DATE_EPOCH` (it comes from the commit time of HEAD, and the runner's checkout may differ from a developer's worktree), a different toolchain resolved on the machine (the toolchain is pinned in `rust-toolchain.toml`; the component list and host triple can still change the standard library path), or `cargo` metadata that embeds a hash of the build path (`-C metadata`). The fix goes into `scripts/build-renderer.sh` (a remap, an environment variable, a flag) and nowhere else.
- **Pin from the runner.** Once a Windows build equals the Linux artifact byte for byte, `bash scripts/build-renderer.sh embed` is run so that `renderer.wasm` and its digest are the ones both systems build, and `check` passes on both. If no fix makes them equal, the Linux digest is the pin (the runner is the release builder): the script's `check` mode then compares against the pinned digest only where it is authoritative and prints a notice elsewhere. That is a change of rule 9 and the author's decision (a STOP below), not something to do silently.
- **The workflow change, in its own commit, by the author's rule.** In the `renderer` job: write the digest and the size of the module to the job summary (`$GITHUB_STEP_SUMMARY`), keep the upload with `if: always()` and a retention (`retention-days`) long enough for a comparison, and upload the second build of the `verify` step too when it fails, so that a non-reproducible build can be diffed. No new action is used (the job already uses `actions/checkout` and `actions/upload-artifact`, pinned by full SHA with a version comment, and the "every action is pinned" step of the workflow keeps passing). After the pin holds on Windows, a second job runs `bash scripts/build-renderer.sh check` on a Windows runner, so the equality is proved on every run and not by a person once; that job is added in a separate commit as well, and joins the aggregator of required checks.
- **Evidence.** `docs/quality/<date>-renderer-digest.md` records the three digests before, the report of `wasmdiff`, the cause, the change, the digests after on both systems, and the run of CI that produced them.

## Tasks

- [ ] 1. The tool `internal/tools/wasmdiff` with its test. Verify: `go test ./internal/tools/wasmdiff/...` passes and `go run ./internal/tools/wasmdiff engine/renderhost/renderer.wasm engine/renderhost/renderer.wasm` reports no difference (exit 0).
- [ ] 2. The workflow change, its own commit: the digest in the job summary, a retention on the artifact, the second build uploaded when `verify` fails. Verify: the "every action is pinned by a full commit SHA" check of the workflow passes locally (`grep -RnE "^[[:space:]]*(- )?uses: [^@ ]+@" .github/workflows | grep -vE "@[0-9a-f]{40} # v[0-9]"` prints nothing) and `bash .agents/skills/verify/scripts/verify.sh --quick --base origin/release/v0.50.0` passes; the orchestrator pushes the branch so that the job runs and confirms the summary line appears.
- [ ] 3. Record the three digests of one commit and diff them. The orchestrator runs the job and downloads the artifact (`gh run download <run> -n renderer-wasm-linux`) into a scratch folder and hands the senior its path; the senior builds on Windows with `bash scripts/build-renderer.sh wasm`. Verify: `go run ./internal/tools/wasmdiff <windows .wasm> <linux .wasm>` prints the first difference and its section, and `docs/quality/<date>-renderer-digest.md` holds the three digests and the report.
- [ ] 4. Fix the cause in `scripts/build-renderer.sh`. Verify: a Windows build and the Linux artifact of the same commit have the same SHA-256 (`go run ./internal/tools/wasmdiff` exits 0 on them), and `bash scripts/build-renderer.sh verify` still passes on Windows.
- [ ] 5. Make the module and its digest the ones both systems build. Verify: `bash scripts/build-renderer.sh embed` then `bash scripts/build-renderer.sh check` exit 0 on Windows, `go test ./engine/renderhost/... ./engine/export/... -run 'TestImage|TestHost|TestModule'` pass with every golden file unchanged, and the `Renderer` job passes on Linux (run by the orchestrator).
- [ ] 6. The cross-system check in CI, its own commit: a Windows job that runs `bash scripts/build-renderer.sh check`, added to the aggregator of required checks. Verify: the orchestrator reads both `Renderer` jobs green on the pushed branch; `verify.sh --quick --base origin/release/v0.50.0` passes locally.
- [ ] 7. Close what waited for this: tick epic 49 task 7 with the evidence and bring VV-276 to the orchestrator as fixed with the command and result; update SDD-013 section 4.1 and rule 9 if the build rule changed. Verify: `task lint:md` exits 0 and `bash scripts/build-renderer.sh check` exits 0.

## Test strategy

The tool has unit tests on two small hand-made modules. The build is proved by comparing bytes, not by a test of the Rust code: a Windows build and a Linux build of one commit must have one SHA-256, and the existing renderer parity and golden tests must pass unchanged on the re-embedded module (a re-embed with a different module that draws differently would fail them). No Rust file is edited.

## Acceptance criteria

1. The `Renderer` job's summary shows the module digest and size, and its artifact `renderer-wasm-linux` is kept with a retention long enough to compare (read from a CI run by the orchestrator).
2. A module built with `bash scripts/build-renderer.sh wasm` on Windows and the artifact of the `Renderer` job of the same commit have the same SHA-256 (`go run ./internal/tools/wasmdiff` exits 0), or the author has decided the Linux digest is the pin (see STOP).
3. `engine/renderhost/renderer.wasm.sha256` is the digest the Linux runner builds, and `bash scripts/build-renderer.sh check` exits 0 on Linux (CI) and on Windows.
4. Every golden file and renderer parity test passes unchanged on the embedded module (`go test ./engine/renderhost/... ./engine/export/...`).
5. The workflow changes are in commits of their own and change nothing else in `ci.yml`; every action is still pinned by a full SHA with a version comment.
6. `docs/quality/<date>-renderer-digest.md` records the digests before and after, the cause and the CI runs.
7. `task lint` and `task test` pass; no new dependency; the Rust source is unchanged.

## Risks and mitigation

- The difference may be in the Rust source or its dependencies (an iteration order, an absolute path in a crate that is not remappable). Mitigation: the comparison names it before any change; **STOP** if the cause is in `renderer/` source or in `Cargo.lock`: that is an epic of `renderer/`, and the pin of the Linux digest (below) is the fallback.
- **STOP** if no change of the script makes the Windows and Linux modules equal: report the options with a recommendation, namely (a) the Linux runner is the authority and `check` is authoritative only there, (b) a pinned container image builds the module on any system (a fixed-tag Docker image, the author's decision on a new tool), (c) keep two digests. This changes rule 9 of `.agents/rules/rust.md`, so it is the author's.
- **STOP** if a step needs an outbound request from the senior (a run of CI, the download of the artifact): the orchestrator does it and gives the path.
- **STOP** if a new dependency or a new GitHub Action is needed.
- A re-embed in the middle of epic 91 conflicts: the dependencies say which merges first, and the pin is repeated (one command) by the epic that merges second.
- The workflow is a shared file: the orchestrator orders it with any other epic that edits `ci.yml`.

## Size

M: a small tool, one build-script fix whose cause is not known yet, two workflow commits and a re-pin, with a CI run in the middle.

## Priority

Should. Releases are built on Linux and developers work on Windows; a `check` that only passes on one of them is a gate nobody trusts. It also unblocks epic 49 task 7.

## Hand-off for review

The commits (the two workflow changes each alone); the three digests before and after; the report of `wasmdiff`; the cause and the fix; the CI runs the orchestrator made and their links; which of the three outcomes of the STOP applied, if any; the results of the commands of the tasks on Windows; what was not verified (macOS).
