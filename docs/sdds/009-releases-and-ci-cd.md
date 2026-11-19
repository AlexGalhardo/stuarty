# SDD-009: Releases and CI/CD in the vukem pattern

- Amended (2026-11-18) for Stuarty: no release is cut and CI is not a gate while the epics are implemented (ADR-0045, which suspends ADR-0028); the tooling described here is unchanged and is used again when the integration pull request is split into releases, starting at v0.30.0 under the new name (ADR-0044). The phase names of this document (Prototype, ALPHA, BETA, 1.0) no longer schedule any work.
- Status: Accepted (2026-11-14). Drafted by a delegated engineer, audited independently, corrected under the architect's decisions.
- Depends on: SDD-000, SDD-007, ADR-0001, ADR-0011, ADR-0014, ADR-0024, ADR-0025, ADR-0026, author decisions D-005, D-006, D-008, D-015, D-016, D-017, D-020, D-021, D-025
- Implements: RF-200, RF-209, RF-210, RF-214, RF-215, NFR-041, NFR-042, NFR-043, NFR-044, NFR-045 (and the repository part of RF-211, RF-212 through SDD-010)
- Owner area: senior-d (agent surfaces, documentation site, releases and CI)

## Contents

1. [Scope, sources and conventions of this document](#1-scope-sources-and-conventions-of-this-document)
2. [Release rules checklist](#2-release-rules-checklist)
3. [A real vukem release note and our first one](#3-a-real-vukem-release-note-and-our-first-one)
4. [Versioning, phases and tags](#4-versioning-phases-and-tags)
5. [Changelog and `cliff.toml`](#5-changelog-and-clifftoml)
6. [Commits and pull request titles](#6-commits-and-pull-request-titles)
7. [The release procedure](#7-the-release-procedure)
8. [The release tool](#8-the-release-tool)
9. [Workflows](#9-workflows)
10. [GoReleaser](#10-goreleaser)
11. [The container image](#11-the-container-image)
12. [Third-party notices](#12-third-party-notices)
13. [CI gates, coverage and time budget](#13-ci-gates-coverage-and-time-budget)
14. [Local developer commands](#14-local-developer-commands)
15. [Git hooks, editor and lint configuration](#15-git-hooks-editor-and-lint-configuration)
16. [Repository protection, labels, secrets and Dependabot](#16-repository-protection-labels-secrets-and-dependabot)
17. [Files that move from `web/` to the root](#17-files-that-move-from-web-to-the-root)
18. [Wiring the local repository to the remote history](#18-wiring-the-local-repository-to-the-remote-history)
19. [Implementation tasks owned by this SDD](#19-implementation-tasks-owned-by-this-sdd)
20. [Acceptance](#20-acceptance)

## 1. Scope, sources and conventions of this document

This SDD specifies everything that builds, tests, versions, publishes and protects the repository. The documentation site content and i18n are SDD-010; this SDD only specifies the workflows that build and deploy it.

Sources, all read on 2026-11-14:

- The vukem repository at `C:\dev\stuarty\vukem\`, read at the tag `v0.43.0`. Every citation of a vukem file in this document, written `path:line` or `file:line`, means `v0.43.0:<path>:<line>` and is read with `git -C C:\dev\stuarty\vukem show v0.43.0:<path>`, never from the working tree (the working tree is ahead of the tag). Lines were rechecked against the tag on 2026-11-14.
- Where the vukem working tree already has what we need beyond the tag, this document says "present on vukem main after v0.43.0" (the Docker Hub publishing, commit `bb1fffa0`); it is not called missing.
- The Studio application's CI in `video/web/.github/`, `video/web/.githooks/`, `video/web/cliff.toml`, `video/web/Dockerfile.app`. The Studio is about to be deleted; its workflows are sources of ideas only and are never copied into the new pipeline.
- The workspace root `C:\dev\stuarty\` holds `ROADMAP.md`, `AGENTS.md`, `STATUS.md` and `vukem-feedback.md` OUTSIDE this repository (D-025). The repository has no ROADMAP and no STATUS file; it has a public `AGENTS.md` for contributors.
- research note `vukem-practices-and-releases` and research note `notes/vukem`.

Conventions:

- **Decision of this SDD** marks a choice made where the ADRs or vukem were silent or contradictory. All are collected in the final answer of the author of this SDD and are listed in section 19 for the ROADMAP (kept in the workspace root, outside this repository, D-025).
- **Adapted** marks a place where we deviate from vukem on purpose, with the reason.
- Values that are only known when a task runs (a checksum, a commit, a digest) are written as `<recorded by REL-...>`. The task identifiers `REL-*` are defined in section 19; the ROADMAP must use them.
- Every external GitHub Action is pinned by full commit SHA with a version comment. Every tool in a workflow has a pinned version (an exact version, or the minor line the tool itself pins, such as `golangci-lint` `v2.14` as vukem does). Docker images have a fixed version tag. Nothing uses `latest`.
- Pinned versions below are copied from vukem at the line cited. A later Dependabot bump moves SHA and comment together (`.github/dependabot.yml`, section 16).

## 2. Release rules checklist

Every row is a rule the release process must satisfy. "Trace" is the observed vukem practice. "Ours" says what is the same and what is adapted.

| # | Rule | Trace in vukem (at `v0.43.0`) | Ours |
| --- | --- | --- | --- |
| R1 | Semantic Versioning, tag `vMAJOR.MINOR.PATCH` with the leading `v` | `docs/tooling.md:78` | Same |
| R2 | Before 1.0 a minor may break and a patch never does | `docs/tooling.md:79` | Same |
| R3 | Tags are annotated; a pushed tag is never moved or deleted; a bad release is fixed by a new patch | `docs/tooling.md:82`; `internal/tools/release/main.go:143` (annotated tag created), `main.go:148-149` (the two pushes) | Same. No `retract` is needed because we publish no Go module for import |
| R4 | One release per task group of the plan, never several groups in one | `docs/tooling.md:104` | One release per ROADMAP task group, same rule |
| R5 | The release title is exactly the tag | `internal/tools/release/main.go` (`--title version`); research note `vukem-practices-and-releases` section 1 | The workflow creates the release with `--title "$TAG"` (`release.yml` below, step "Publish the release notes") |
| R6 | The body is the changelog section of that version, copied, without the `## [x]` heading, starting at `### Added` | `docs/tooling.md:100`; Studio `release.yml` awk extraction; `docs/tooling.md:159-172` | Same, extracted by the workflow with the literal-match awk of `docs/tooling.md:164-168`, **adapted**: one extra stop line at the first comparison link (`[x.y.z]: ...`) so the last section does not swallow the link footer |
| R7 | A release with an empty body is refused | `docs/tooling.md:118`; `internal/tools/release/main.go` (`refusing to publish ... empty release body`) | The workflow fails when the extracted section has no non-blank character |
| R8 | Sections in fixed order: Added, Changed, Deprecated, Removed, Fixed, Security | `.config/cliff.toml:50-62` (`<!-- N -->` group prefixes) | Same, section 5 |
| R9 | Entry format `- **<scope>:** Message ([abc1234](commit URL))`, `**BREAKING:**` in front when it breaks, commit links and no pull request links | `.config/cliff.toml:23` | Same, only the repository URL changes |
| R10 | `CHANGELOG.md` is Keep a Changelog 1.1.0 with a permanent `[Unreleased]` section and comparison links at the bottom; released sections are never edited by hand | `.config/cliff.toml:4-6, 27-40`; `docs/tooling.md:100` | Same. The `[Unreleased]` section is not written by hand either (ADR-0025 item 3) |
| R11 | Version sections are generated by git-cliff from Conventional Commits; `docs`, `test`, `ci`, `build`, `chore`, `style` are omitted | `.config/cliff.toml:53-62`; `docs/tooling.md:90-98` | Same |
| R12 | `Security:` footer on a `feat`, `fix`, `perf` or `refactor` commit files it under Security; a body that only mentions security does not | `.config/cliff.toml:57-60` | Same |
| R13 | The release commit is `chore(release): vX.Y.Z`, skipped by cliff | `internal/tools/release/main.go:139`; `.config/cliff.toml:51` | Same |
| R14 | Tag push triggers `release.yml`; `workflow_dispatch` with a version input recovers a tag whose push started no run; version tags are pushed one at a time | `release.yml:15-22`; `docs/tooling.md:153-157` | Same |
| R15 | Binaries by GoReleaser: static, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`, linux, darwin, windows on amd64 and arm64 | `.config/goreleaser.yaml:8-18` | Same, with binary `stuarty` from `./web/cmd/app` |
| R16 | Archives `tar.gz`, `zip` on Windows, containing the licence, README and CHANGELOG | `.config/goreleaser.yaml:20-26` | Same plus the third-party notices (RF-215) |
| R17 | GoReleaser never writes the notes: `release.mode: keep-existing`, `changelog.disable: true` | `.config/goreleaser.yaml:48-53` | Same |
| R18 | `checksums.txt` signed with cosign keyless, verified right after signing | `.config/goreleaser.yaml:38-43`; `release.yml:81-86` | Same |
| R19 | One SPDX SBOM per archive by syft | `.config/goreleaser.yaml:31-33`; `release.yml:72-74` | Same |
| R20 | Build provenance attestation of `checksums.txt`, and the bundle attached to the release as `<name>.intoto.jsonl` | `release.yml:87-100` | Same, file `stuarty.intoto.jsonl` |
| R21 | The image job runs only for stable tags `vX.Y.Z`; a pre-release tag publishes no image and never moves `latest` | `release.yml:53-58, 102-104` | Same |
| R22 | Image tags `X.Y.Z`, `X.Y`, `latest`; multi-architecture `linux/amd64,linux/arm64`; SBOM and `provenance: mode=max`; the digest is signed with cosign and attested | `release.yml:120-164` | Same, **adapted**: two registries (GHCR and Docker Hub), the author's standing rule (ADR-0025 item 6). At `v0.43.0` vukem publishes only to GHCR (`release.yml:107`, recorded as VF-010); the Docker Hub steps are present on vukem main after v0.43.0 (commit `bb1fffa0`, `release.yml` of the working tree) and may be copied from there |
| R23 | Every action pinned by full commit SHA with a version comment; tools fetched in workflows use exact versions | `docs/tooling.md:151` | Same |
| R24 | Top-level `permissions: contents: read`; jobs widen only what they write | `release.yml:24-33, 109-113`; `docs/tooling.md:151` | Same |
| R25 | Release only from a green `main`; an open `ci-failure` issue blocks the next release | `docs/tooling.md:113`; `ci-failure.yml`; `AGENTS.md` | Same, checked by the release tool (section 8) |
| R26 | A tag that went up without a run is recovered by dispatching `release.yml`, never by re-tagging | `docs/tooling.md:130, 157` | Same |
| R27 | `main` accepts only pull requests with green required checks and linear history; the PR title is a checked Conventional Commit | `.config/ruleset-protect-main.json:12-40`; `pr-title.yml:1-33` | Same, section 16 |
| R28 | Releases are cut through a pull request while the ruleset requires pull requests | `docs/maintenance.md:21-29` | This is **the** procedure here, section 7 (D-005 says remote `main` is protected) |
| R29 | Post-release checks: workflow green, `cosign verify-blob`, image `cosign verify`, package visibility public | `docs/maintenance.md:46-51` | Same, section 7 step 11 |
| R30 | The release date and the `Updated` row of the plan status table are written by the tool, never by hand | `docs/tooling.md:116`; `internal/tools/release/main.go:70-90` | **Adapted**: the release date is written by the tool into `CHANGELOG.md`; the plan status lives in the workspace root (D-025), outside this repository, so the tool writes no plan file and the orchestrator updates the status after the release (section 7 step 12) |

Deviations from vukem, all intentional and explained where they occur:

- The GitHub release and its notes are created by `release.yml`, not by a maintainer's machine, because NFR-044 says a release is produced by pushing a tag and nothing else (section 9.3).
- The release tool prepares the release commit on a branch and tags later on the merged commit, which removes the tag-delete dance of `docs/maintenance.md:26` (section 8).
- A second registry (Docker Hub) and the third-party notices (R16, R22).

## 3. A real vukem release note and our first one

### 3.1 The real note

Release `v0.43.0` of `AlexGalhardo/vukem`, published 2026-11-12, title `v0.43.0`. Verbatim from `gh release view v0.43.0` (copied from research note `vukem-practices-and-releases` section 3):

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

Properties to copy: no heading and no date inside the body; sections in the order Added, Changed, Fixed, Security; each entry begins with an optional `**BREAKING:**`, then the bold scope, then a capitalised message, then the short hash linked to the full-hash commit URL; entries inside a section are in commit order, oldest first (`sort_commits = "oldest"`, `.config/cliff.toml:49`).

### 3.2 Our first release note, `v0.5.0`

Illustrative. The real entries are whatever the Conventional Commits since `v0.4.0` say; the hashes below are made up. The `**BREAKING:**` entry is real by construction: the commit that deletes the Studio is written with `!` (section 6). The body of the GitHub release is exactly this text. The changelog heading that precedes it in `CHANGELOG.md` (`## [0.5.0] - 2026-12-01`) is not part of the body.

```markdown
### Added

- **core:** Add the timeline document types and their JSON Schema ([6854f17](https://github.com/AlexGalhardo/stuarty/commit/6854f171519cb928540077d9d397d1dd4d5365c4))
- **command:** Add the command registry with apply, inverse and actionable errors ([57794e6](https://github.com/AlexGalhardo/stuarty/commit/57794e637edd94041d38d0906528571347c5fb34))
- **plan:** Add the frame plan that says what is on screen at frame t ([1b02f1e](https://github.com/AlexGalhardo/stuarty/commit/1b02f1e13c761440bc229328cd4029b5d775210c))
- **lint:** Add structural checks that return plain text findings ([5d85df6](https://github.com/AlexGalhardo/stuarty/commit/5d85df6543399001774ff7fc740f1f2991e6a5cb))
- **ingest:** Add normalized media, proxy and thumbnail generation ([4f2e65e](https://github.com/AlexGalhardo/stuarty/commit/4f2e65eb7fb03ec4c56952d555add8d2f9e6e1fe))
- **service:** Add the service boundary with revisions, undo and redo ([1b2c6ad](https://github.com/AlexGalhardo/stuarty/commit/1b2c6adf6a782f69d7ccc9a1b905b49d71cfe4f7))
- **api:** Add the HTTP API under /api/v1 and the OpenAPI 3.1 document ([eecac20](https://github.com/AlexGalhardo/stuarty/commit/eecac20f3712524487c5bc5a8df0cfdd524fc248))
- **mcp:** Add the MCP server with the edit, view and lint tools ([b329252](https://github.com/AlexGalhardo/stuarty/commit/b32925249de66da3389d6199cffb1cfb12b6009b))
- **cli:** Add project:create, edit, view and export to stuarty ([9d0bb76](https://github.com/AlexGalhardo/stuarty/commit/9d0bb769ee1fc748395d66ebbcfaf76c07c2f2ed))
- **ui:** Add the browser editor with live preview and the four basic operations ([d621c7c](https://github.com/AlexGalhardo/stuarty/commit/d621c7cf0329903518fbeb542bd331a0c6745111))
- **site:** Add the documentation site in English and Brazilian Portuguese ([2028dc6](https://github.com/AlexGalhardo/stuarty/commit/2028dc6aff0695d610f083554d15fd99afb16e70))

### Changed

- **BREAKING:** **repo:** Replace the Studio application with the Stuarty editor ([079a3e6](https://github.com/AlexGalhardo/stuarty/commit/079a3e6ce84328842b8cafae9dd27a99f937c9bd))
- **repo:** Move the Studio tree under web/ ([1ec9385](https://github.com/AlexGalhardo/stuarty/commit/1ec9385a0cf834ba9f68a702bb35ee06cc38ba35))
- **timeline:** Store item times as whole frames on the project frame rate ([e8cdff8](https://github.com/AlexGalhardo/stuarty/commit/e8cdff8b5324d58e74f6112b3237fd938425ee4f))

### Fixed

- **ingest:** Apply the display rotation of phone videos before normalizing ([2a69fb7](https://github.com/AlexGalhardo/stuarty/commit/2a69fb78dc49152ee3bfbcb16d924b8ef30bce36))
- **export:** Keep every output dimension even for 4:2:0 ([93b6bcd](https://github.com/AlexGalhardo/stuarty/commit/93b6bcdcf5cc9d22d7f1bea0946de2e486def032))

### Security

- **api:** Require the local token and check Host and Origin on every request ([bb7aec9](https://github.com/AlexGalhardo/stuarty/commit/bb7aec9ed40f05a334f45748d5443dd44d1f4376))
```

How each visible property is produced:

- The `**BREAKING:**` entry comes from `refactor(repo)!: replace the Studio application with the Stuarty editor` with a `BREAKING CHANGE:` footer (section 6.4). It is under Changed because `refactor` maps to Changed (`.config/cliff.toml:61`). There is no Removed section: cliff fills Removed only from `revert` commits (`.config/cliff.toml:61`), so a deletion is reported as a breaking change. This is vukem friction 1 in the final answer.
- The `Move the Studio tree under web/` entry comes from the pure-rename commit of section 18.
- The Security entry comes from a `feat(api)` or `fix(api)` commit with a `Security: <risk closed>` footer.
- `ci`, `docs`, `build`, `test`, `chore` and `style` commits (for example every commit of this SDD's workflows) never appear.

## 4. Versioning, phases and tags

1. Versions follow SemVer 2.0.0 (R1). The single source of the version is the git tag; there is no version field to keep in step, except `web/package.json` `version`, which the release commit does not touch (Decision of this SDD: it stays `0.0.0` and `private: true`, because the Studio's `0.4.0` there was a second source of truth that drifted).
2. Phase mapping (ADR-0025 item 2), consistent with vukem's `docs/tooling.md:106-110`:

| Phase | Tags | One release per | Ends when |
| --- | --- | --- | --- |
| Legacy | `v0.1.0` only: the whole Studio history (D-026) | | |
| Prototype, ALPHA, BETA | `v0.2.0`, `v0.3.0` ... (minor bump per release) | about 90 commits on `main` since the previous tag, and every phase exit (ADR-0028) | the exit criteria of the phase hold |
| 1.0 | `v1.0.0` directly, no release candidates | the BETA exit | |

Amended on 2026-11-14 by D-026 and ADR-0028: the Studio releases are folded into `v0.1.0` and the editor starts at `v0.2.0`. Wherever this section or a later one still names `v0.5.0` as the first editor release, read the first editor release (`v0.2.0`); wherever it names `v0.4.0` as the last Studio tag, read `v0.1.0`.

Amended on 2026-11-14 by D-029: the product is local only. `v0.2.0`, the release of the Prototype, already ships the container image (section 11, run on `127.0.0.1` only) and installers of the local server (Windows 11 `.exe`, Linux `.deb` and AppImage, macOS `.pkg`, without paid code signing); where this section or section 11 says the image first ships in ALPHA, read `v0.2.0`. The installer build is specified by the task P1-REL-010, which records its tools here.

3. The first editor tag is `v0.2.0` (it was `v0.5.0` before D-026). It is cut when the Studio replacement is merged and the whole pipeline of section 9 is green on `main`; it may be small. Cutting it early is deliberate: it is the first run of `release.yml` and of signing, and should happen while the content at risk is small. `v0.5.0` publishes the signed archives with checksums, SBOMs and provenance. The container image (RF-210, two registries) and the third-party notices (RF-215) are ALPHA: they first ship in the first ALPHA release, and only from that release do the archives carry `THIRD_PARTY_NOTICES.md` and the registries serve the image tags.
4. A patch release is a bug fix only and never carries a breaking change (R2). A breaking change before 1.0 is a minor bump and is marked `!` (section 6). From 1.0 a breaking change (a `!` header or a `BREAKING CHANGE:` footer) is a major bump: `release prepare` refuses any other version when the range holds one (P4-REL-001). What 1.0 promises and what it does not is in `.github/CONTRIBUTING.md`.
4a. `v1.0.0` is cut directly from the BETA exit, with no `-rc` tag, on the same release branch and the same procedure as any other release (sections 7 and 8). The release tool does not need a special case: the previous tag is a `v0.x`, so the major rule starts to apply after it.
5. Pre-release suffixes (`-rc.1`, `-beta.1`) are allowed by the workflow (they build binaries, publish no image, never move `latest`, R21) but the project does not use them (ADR-0025 item 2).
6. A tag is annotated (`git tag -a vX.Y.Z -m vX.Y.Z`), is created only by the release tool on the merged release commit, is pushed alone (`git push origin vX.Y.Z`, one tag per push, `docs/tooling.md:155`) and is never moved or deleted (R3). The one recorded exception is D-026: on 2026-11-14 the tag `v0.1.0` was moved to the last Studio release commit and the tags `v0.2.0` to `v0.4.0` were deleted.
7. Go module consequence: the module `github.com/AlexGalhardo/stuarty` is an application; nobody imports it. Tags are release markers, not module versions, so there is no `go.mod` `retract` step (R3). If this changes, SDD-009 is revised.

## 5. Changelog and `cliff.toml`

### 5.1 Rules

1. `CHANGELOG.md` at the repository root is Keep a Changelog 1.1.0: the header text, a permanent `## [Unreleased]` section, one `## [X.Y.Z] - YYYY-MM-DD` section per release, comparison links at the bottom.
2. Sections are generated by `git cliff --config .config/cliff.toml` (section 8) from Conventional Commits. No human edits a released section. No human edits `[Unreleased]`: it is rewritten by the tool only (ADR-0025 item 3; this resolves VF-007, where vukem states the rule and does not always follow it).
3. A wrong released section is corrected by fixing the template or, for the next release, the commit; never by editing the published section.
4. The existing sections `0.1.0` to `0.4.0` moved from `web/CHANGELOG.md` and stay as they are; their comparison links keep the old repository name `vukem-studio`, which GitHub redirects. This is the only place the old name remains.
5. The mapping from commits to sections (D-027; it replaces the vukem mapping of `docs/tooling.md:90-98`). A release has only these five sections, in this order, and a section with no entry is left out:

| Commit | Section | Meaning |
| --- | --- | --- |
| `feat` | Added | something new |
| any commit marked breaking (`!` or a `BREAKING CHANGE:` footer) | Changed | the behaviour of an existing public interface changed (command, tool, HTTP or CLI contract, file format) |
| `revert`, or a `feat`/`refactor` with a `Removed:` footer | Removed | something was taken out |
| `fix` | Fixed | something was corrected |
| footer `Security:`, or the scope `security` | Security | a security fix or hardening |
| `perf`, `refactor` without `!`, `docs`, `test`, `ci`, `build`, `chore`, `style`, `chore(release)` | omitted | |

The first matching row wins in this order: Security, Changed, Removed, Added, Fixed. There is no Deprecated section: a deprecation is announced in the commit that removes the thing, under Removed, or as a breaking change under Changed.

6. An empty section is replaced by `### Changed` with `- Maintenance release (no user-facing changes).` by the release tool, as in vukem (`internal/tools/release/main.go:110-112`), so a release body is never empty (R7).
7. Unconventional commits are ignored by cliff (`filter_unconventional = true`). The old Studio history has such commits; cliff only looks at commits after `v0.4.0`, so they never appear.

### 5.2 `.config/cliff.toml`

Copied from `vukem/.config/cliff.toml`. Changed: the repository URL in lines 23, 31, 33 and 36 of the original, and nothing else. The Studio's `video/web/cliff.toml` differs from vukem's by an extra `docs(site|repo)` skip that is redundant (`docs` is already skipped by the `^(docs|test|...)` rule, `.config/cliff.toml:53`), so it is dropped.

```toml
# git-cliff configuration: Conventional Commits -> Keep a Changelog 1.1.0.
# https://git-cliff.org/docs/configuration

[changelog]
header = """
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
"""
body = """
{% if version %}\
## [{{ version | trim_start_matches(pat="v") }}] - {{ timestamp | date(format="%Y-%m-%d") }}
{% else %}\
## [Unreleased]
{% endif %}\
{% for group, commits in commits | group_by(attribute="group") %}
### {{ group | striptags | trim }}

{% for commit in commits %}\
- {% if commit.breaking %}**BREAKING:** {% endif %}{% if commit.scope %}**{{ commit.scope }}:** {% endif %}{{ commit.message | upper_first | replace(from="<", to="\\<") }} ([{{ commit.id | truncate(length=7, end="") }}](https://github.com/AlexGalhardo/stuarty/commit/{{ commit.id }}))
{% endfor %}\
{% endfor %}
"""
footer = """
{% for release in releases -%}
{% if release.version -%}
{% if release.previous.version -%}
[{{ release.version | trim_start_matches(pat="v") }}]: https://github.com/AlexGalhardo/stuarty/compare/{{ release.previous.version }}...{{ release.version }}
{% else -%}
[{{ release.version | trim_start_matches(pat="v") }}]: https://github.com/AlexGalhardo/stuarty/releases/tag/{{ release.version }}
{% endif -%}
{% else -%}
[Unreleased]: https://github.com/AlexGalhardo/stuarty/compare/{{ release.previous.version }}...HEAD
{% endif -%}
{% endfor %}
"""
trim = true

[git]
conventional_commits = true
filter_unconventional = true
split_commits = false
protect_breaking_commits = true
filter_commits = false
tag_pattern = '^v[0-9]+\.[0-9]+\.[0-9]+'
sort_commits = "oldest"
commit_parsers = [
  { message = '^chore\(release\)', skip = true },
  # Security is explicit: a "Security:" footer or the scope security.
  # Matching the word in the body filed unrelated commits (a feature whose
  # test is named TestSecurity_*) under Security.
  { footer = "^Security", group = "<!-- 4 -->Security" },
  { message = '^[a-z]+\(security\)', group = "<!-- 4 -->Security" },
  # Changed means a breaking change of a public interface (D-027).
  { field = "breaking", pattern = "true", group = "<!-- 1 -->Changed" },
  { footer = "^Removed", group = "<!-- 2 -->Removed" },
  { message = "^revert", group = "<!-- 2 -->Removed" },
  { message = "^feat", group = "<!-- 0 -->Added" },
  { message = "^fix", group = "<!-- 3 -->Fixed" },
  { message = "^(perf|refactor|docs|test|ci|build|chore|style)", skip = true },
]
```

Notes on behaviour that surprised the research and are kept on purpose:

- `tag_pattern` makes cliff treat only `vX.Y.Z` tags as releases, so the Studio tags `v0.1.0` to `v0.4.0` are valid previous versions and the `v0.5.0` section covers exactly the commits after `v0.4.0`.
- The footer `Deprecated:` rule comes before the "skip docs" rule (original line 51-53), so a deprecation recorded on a `docs` commit still lands in Deprecated. The `Security:` rule comes after the skip, so a security footer on a `docs` commit is dropped, as in vukem.

## 6. Commits and pull request titles

### 6.1 Format

Conventional Commits 1.0 in English (`docs/tooling.md:57-74`, vukem `.claude/rules/commits.md`):

```text
<type>(<scope>)!: <description>

[body]

[footer(s)]
```

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style`, `revert`.
- Description: imperative, starts with a lower-case letter, no trailing period, header at most 100 characters.
- Breaking change: `!` after the type or scope and a `BREAKING CHANGE:` footer describing the impact.
- Security change: a `Security: <one line on the risk closed>` footer on a `feat`, `fix`, `perf` or `refactor` commit.
- The ROADMAP task is in the body: `Refs: P1-ENG-003` (ADR-0025 item 8). The ROADMAP identifier scheme is the one the ROADMAP defines.
- One logical change per commit and per pull request.
- Merge commits, `Revert "..."` and local `fixup!`/`squash!` are tolerated by the local validator and are blocked on pushes to `main` by the ruleset (`required_linear_history`, no merge method for merge commits).

### 6.2 Allowed scopes

**Adapted:** vukem lists scopes in prose and does not enforce the list (`internal/tools/commitlint/lint.go:20,63-64` checks only the shape). Here the list is enforced twice: by the local validator and by the pull request title check. A scope is optional; when present it must be one of these **49** scopes. The list is derived from ADR-0001; the first eleven rows are the `core/`, `engine/` and `service/` packages. `release` is the scope of the release commit and the release pull request (`chore(release): vX.Y.Z`); without it the commit-msg hook and `Conventional title` would reject every release. `deps-dev` is what Dependabot writes for development dependencies (`include: scope`). `adr`, `sdd`, `go` and `security` were added on 2026-11-14 because the team already writes them; the first two name the documents of `docs/` that `docs` would hide, and `security` is the scope D-027 reads. Three commits of release/v0.3.0 (`0bb53009b`, `c83d5ab77`, `a2d2a6177`) were pushed with a header of 101 to 108 characters; `Conventional commits` skips them by hash, because a pushed commit cannot be reworded without a force push, and every later commit is checked for length before it is pushed. `image` and `workspace` were added on 2026-11-14 (the container image and `service/workspace`), and a numbered `sdd-NNN` is accepted as a spelling of `sdd` (one commit of P1-SRV-003 used it). `installers` was added with P1-REL-010 for the installer packaging of `.config/installers/`.

| Scope | Covers |
| --- | --- |
| `core` | `core/` as a whole, `core/timeval`, `core/schema`, cross-cutting core changes |
| `timeline` | `core/timeline` |
| `schema` | `core/schema`, `internal/tools/contractgen` (the generated contracts, P1-SCH-003) |
| `command` | `core/command` |
| `plan` | `core/plan` |
| `lint` | `core/lint` (the editor lint, not the code linters) |
| `engine` | `engine/` as a whole, `engine/ffmpeg`, `engine/store` |
| `raster` | `engine/rasterize`, `renderer/fonts` and the bundled fonts |
| `ingest` | `engine/ingest` |
| `export` | `engine/export` |
| `image` | the container image: `.config/docker/`, `image.yml`, `scripts/image-smoke.sh` |
| `transcribe` | `engine/transcribe` |
| `installers` | `.config/installers/`, `internal/tools/installericon`, the installer scripts and jobs (P1-REL-010) |
| `service` | `service/` |
| `workspace` | `service/workspace` (the workspace directory, lock, runtime.json, token) |
| `web` | the vukem application in `web/` that is none of `api`, `mcp`, `cli`, `ui` (persistence, jobs, auth, SSE) |
| `api` | HTTP and SSE endpoints, OpenAPI |
| `mcp` | the MCP server and its tool descriptions |
| `cli` | the `stuarty` commands |
| `ui` | `web/resources/js` |
| `site` | `site/` |
| `docs` | `docs/`, README and other repository documents |
| `repo` | repository layout, root files, git configuration, the public `AGENTS.md` for contributors |
| `ci` | `.github/`, `.config/`, `Taskfile.yml`, `.githooks/`, `internal/tools/` |
| `deps` | dependency bumps (Dependabot prefix `build(deps)` and `ci(deps)`) |
| `deps-dev` | development dependency bumps (Dependabot `build(deps-dev)`) |
| `release` | `chore(release): vX.Y.Z`, the release commit and pull request (section 7) |
| `adr` | `docs/adrs/` |
| `sdd` | `docs/sdds/` |
| `go` | Go-wide changes that fit no package scope: formatter or linter findings, toolchain bumps |
| `security` | a security fix or hardening; also files the commit under Security in the changelog (D-027) |
| `desktop` | `desktop/`, the desktop shell |
| `http` | `web/app/http`, the HTTP handlers and middleware |
| `ffmpeg` | `engine/ffmpeg`, the FFmpeg runner and its pins |
| `renderer` | `renderer/`, the Rust pixel renderer and its build |
| `docker` | `.config/docker/` and the image build |
| `library` | the media library panel of the editor |
| `events` | `web/app/events`, the event stream |
| `cmd` | `web/cmd/`, the program entry points |
| `contract` | `web/contract/`, the contract scenarios |
| `pathguard` | `service/pathguard` |
| `remote` | the remote client mode of the CLI |
| `timeval` | `core/timeval` |
| `store` | `engine/store` |
| `notices` | `internal/tools/notices`, the third-party notices |
| `projects` | the projects page and the project endpoints |
| `preview` | the preview component and its frame requests |
| `owner` | `web/app/owner`, the process that owns a workspace |
| `e2e` | `web/e2e/`, the browser tests |
| `upgrade` | the upgrade test of workspaces written by older releases (`scripts/upgrade-test.sh`, `upgrade.yml`) |
| `servicetest` | `web/app/servicetest`, the shared test helpers of the service |
| `auth` | `web/app/auth`, sessions, launch codes and network-mode users |

On 2026-11-16 the 18 scopes the project already used in its commits (`desktop`, `http`, `ffmpeg`, `renderer`, `docker`, `library`, `events`, `cmd`, `contract`, `pathguard`, `remote`, `timeval`, `store`, `notices`, `projects`, `preview`, `owner`, `e2e`) were added, because the commits were already on the release branch and cannot be reworded without a force push. For the same reason `upgrade`, `servicetest` and `auth` were added on 2026-11-16 for release v0.21.0. The subject length limit of 100 characters is unchanged; three older subjects are known exceptions (`af3252cc`, `5cf1eba3`, and `5144ea0d` of release v0.21.0).

### 6.3 Enforcement

| Where | How |
| --- | --- |
| Local, on every commit | `.githooks/commit-msg` runs `go run ./internal/tools/commitlint "$1"`. The tool is vukem's `internal/tools/commitlint` copied with one change: an allow-list of the scopes above and the type list (`REL-TOOLS`) |
| Pull request | `.github/workflows/pr-title.yml` (section 9.2) |
| Merge | `main` accepts squash merge and rebase merge; the pull request title becomes the squash commit subject and the pull request body its message, so the title is what cliff reads (R27). This holds only with the repository setting "Default commit message for squash merges: Pull request title and description" (section 16.1); GitHub defaults to the commit message for a single-commit pull request |

Superseded on 2026-11-14 by ADR-0028 (D-026): **task pull requests are rebase-merged**, every commit of the branch is a Conventional Commit that reaches `main` as it is and becomes its own changelog entry, and a footer (`BREAKING CHANGE:`, `Security:`) goes in the commit that carries the change. The pull request title is still checked (`pr-title.yml`) as the summary of the branch. The rest of this paragraph is the earlier decision, kept for the record. Earlier decision: **task pull requests are squash-merged**, so one pull request is one changelog entry and its title is the entry. The footers that cliff and the translation check read (`BREAKING CHANGE:`, `Security:`, `Translation:`) must be written in the **pull request body**, because the body becomes the squash commit message; a footer only in a branch commit is lost. vukem's `docs/tooling.md:112` says rebase-merge while `pr-title.yml` says the title becomes the commit; the two statements disagree (vukem friction 2). Rebase-merge is used only for the first pull request of section 18, to keep the pure-rename commit.

### 6.4 The commit that deletes the Studio

```text
refactor(repo)!: replace the Studio application with the Stuarty editor

Delete the Studio application, its skill kits, films, research and Railway
deployment files, exactly the paths of the first list of section 17. The editor
in web/, core/, engine/ and service/ replaces it. Everything removed stays in
the git history.

BREAKING CHANGE: the Studio (brief-to-film pipeline, its skills and its
Railway deployment) no longer exists in this repository. Use v0.4.0 for it.

Refs: <ROADMAP task id>
```

## 7. The release procedure

Who: only the orchestrator (the main session), and only after the author confirmed the version in the chat (AGENTS.md rule of D-020: only the orchestrator pushes, after confirmation). Agents prepare; they never tag or push.

The sequence is the vukem sequence (`docs/tooling.md:112-131`, `docs/maintenance.md:21-29`) with the changes of section 8. There are two pushes, as in vukem: the release branch (instead of `main`, because the ruleset forbids direct pushes) and the tag.

1. **Confirm.** The author says yes to the version `vX.Y.Z`. The orchestrator states the task group being closed, the previous tag and the phase.
2. **Preconditions** (the tool checks 2a to 2d and refuses otherwise):
   - 2a. `git fetch origin --tags`; the local `release/vX.Y.Z` equals `origin/release/vX.Y.Z` and contains `origin/main` (D-031: the release branch carries every commit of the release, so `prepare` runs on its pushed tip, not on `main`).
   - 2b. The tree is clean: `git status --porcelain --untracked-files=no` prints nothing.
   - 2c. The required checks are green on the tip of the release branch: `gh run list --branch release/vX.Y.Z --commit <sha>`.
   - 2d. No open issue labelled `ci-failure`: `gh issue list --label ci-failure --state open` is empty (R25).
   - 2e. Every task of the group is checked in the ROADMAP (workspace root, outside this repository, D-025) and its verification passed. This is checked by the orchestrator, not by the tool.
3. **Local gate.** `task release:preflight` (section 14): `task ci:full` minus the integration services, plus `task notices:check`, `task generate:check`, `task site:build`, `task image:smoke`. It must pass.
4. **Branch.** `git switch -c release/vX.Y.Z`.
5. **Prepare.** `go run ./internal/tools/release prepare vX.Y.Z`. The tool runs `git cliff`, inserts the section under `[Unreleased]`, rewrites the comparison links, and commits `chore(release): vX.Y.Z` (scope `release`, section 6.2). It writes no plan file (the plan status is outside the repository, D-025) and creates no tag.
6. **Doc dates.** Refresh `site/content/last-updated.json` so every page shows its real date: `npm run last-updated` in `site/` (vukem `docs/maintenance.md:43`). The pre-push hook does this and stops the first push; a second `git push` carries it.
7. **Push 1.** `git push -u origin release/vX.Y.Z`, then `gh pr create --title "chore(release): vX.Y.Z" --body "Refs: <task>"`. Wait for the required checks, squash-merge (`gh pr merge --squash`). The squash commit subject must start with `chore(release): vX.Y.Z`; with the squash default of section 16.1 it is the pull request title.
8. **Tag.** `git fetch origin && git switch main && git merge --ff-only origin/main`, then `go run ./internal/tools/release tag vX.Y.Z`. The tool checks that `HEAD` is `origin/main` and that its subject starts with `chore(release): vX.Y.Z`, creates the annotated tag, and runs `git push origin vX.Y.Z` (**push 2**, one tag per push).
9. **Workflow.** The tag starts `release.yml`: it creates the GitHub release with the changelog section as the body (empty body fails), builds and attaches the binaries, checksums, SBOMs, the cosign bundle and the provenance, then publishes the image to GHCR and Docker Hub and signs the digests. Watch it: `gh run watch --exit-status $(gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')`.
10. **Recovery.** If the push started no run: `gh workflow run release.yml -f version=vX.Y.Z` (R26). If the workflow failed in the middle: re-run the failed job (`gh run rerun <id> --failed`); every job is idempotent (the notes step edits an existing release, GoReleaser uploads with `keep-existing`, the image step re-pushes the same tags). A published tag is never moved; a wrong release is fixed by the next patch (R3).
11. **Verify** (within 15 minutes, R29). Each command is expected to succeed:

```bash
VERSION=vX.Y.Z
V="${VERSION#v}"
REPO=AlexGalhardo/stuarty
ARCHIVE="stuarty_${V}_linux_amd64.tar.gz"
gh release view "$VERSION" -R "$REPO" --json body,assets --jq "(.body | length > 0), (.assets | length)"   # true, 15
gh release download "$VERSION" -R "$REPO" -p checksums.txt -p checksums.txt.sigstore.json -p "$ARCHIVE" -p "$ARCHIVE.sbom.json"
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp "^https://github\.com/AlexGalhardo/stuarty/\.github/workflows/release\.yml@refs/tags/v" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
gh attestation verify "$ARCHIVE" -R "$REPO"
for image in ghcr.io/alexgalhardo/stuarty alexgalhardo/stuarty; do
  digest="$(docker buildx imagetools inspect "$image:$V" --format "{{.Manifest.Digest}}")"
  cosign verify "$image@$digest" \
    --certificate-identity-regexp "^https://github\.com/AlexGalhardo/stuarty/\.github/workflows/release\.yml@refs/tags/v" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com
  echo "$image $digest"
done
docker run --rm "ghcr.io/alexgalhardo/stuarty:$V" version
bash scripts/first-export-timing.sh "ghcr.io/alexgalhardo/stuarty:$V"   # first verified export, under 600 s (NFR-040)
```

   `scripts/first-export-timing.sh` follows the Docker page of the site on a fresh container and a fresh volume (API key, import folder, CLI inside the container, one clip, one export whose five checks must pass), prints the time of each step and the total, removes what it created and never removes the image; it exits 1 over 600 seconds (NFR-040, ALPHA exit criterion 7, TASK-0071). The number goes in the release notes of the orchestrator, not in the changelog. First measurement, 2026-11-16, `ghcr.io/alexgalhardo/stuarty:latest` (then v0.18.0), a Windows machine that had run the product before (a fresh container and volume is the closest clean environment there): pull 78 s (cold, 14 layers), `docker run` until `/up` 2.2 s, sign in and key 0.4 s, clip 2.6 s, project and import 6.3 s, edit 0.6 s, export and verification 8.8 s: 21 s without the pull, about 100 s with it. Following only the pages, the first attempt failed at three points (no import root, the CLI needs a key, `project:create --id` is refused in network mode), now written in `self-hosting/docker`.

   The image loop and the `docker run` apply from the first ALPHA release that publishes the image (not `v0.5.0`). The two `echo` lines must print the same digest (one build pushed to both registries). After the first image release only: set the GHCR package visibility to public (GitHub, profile, Packages, `stuarty`, Package settings), or an anonymous `docker pull` fails (`docs/maintenance.md:48`); and confirm the Docker Hub repository `alexgalhardo/stuarty` is public. This one-time step is listed as a repository setting in section 16.1, not as a step of every release; the manual steps of a release are the release pull request and the tag (NFR-044).
12. **Close.** The orchestrator marks the group released in the ROADMAP and updates the status in the workspace root (outside this repository, so no pull request is needed). Delete the branch `release/vX.Y.Z` (it is also deleted automatically on merge, section 16.1).

## 8. The release tool

`internal/tools/release` is vukem's tool (`internal/tools/release/main.go`, `changelog.go`, `bump.go`) copied and adapted. `internal/tools/` is in the tree of ADR-0001 (the Go module root is the repository root, so it is reachable by every package; vukem has the same folder). It is Go, standard library only, with `git`, `git cliff` and `gh` as external commands.

| Command | What it does |
| --- | --- |
| `release prepare vX.Y.Z` | Preconditions 2a to 2d, on a branch named `release/vX.Y.Z` (it refuses `main`). Previous tag: `git describe --tags --abbrev=0 --match "v[0-9]*" HEAD`. Section: `git cliff --config .config/cliff.toml <previous>..HEAD --tag vX.Y.Z --strip all`. If the section has no `### ` heading it adds the maintenance entry. Inserts the section below `[Unreleased]`, rewrites the footer links, `git add CHANGELOG.md`, commits `chore(release): vX.Y.Z` |
| `release tag vX.Y.Z` | `HEAD` is `origin/main`, clean tree, subject starts with `chore(release): vX.Y.Z`, the tag does not exist locally or remotely. `git tag -a vX.Y.Z -m vX.Y.Z`, then `git push origin vX.Y.Z` |
| `release notes vX.Y.Z` | Prints the body that `release.yml` will publish (the same awk extraction), for review in the pull request. Exit 1 when empty |
| `release dispatch vX.Y.Z` | `gh workflow run release.yml -f version=vX.Y.Z` (recovery, R26) |

Changes from vukem, each with the reason:

- No `-push`, no `gh release create`, no `-modules`, no `bumpNestedModules`: those exist for nested modules and for a maintainer that publishes notes. The workflow publishes the notes (NFR-044).
- `prepare` works on a release branch and `tag` runs after the merge. vukem's own documentation (`docs/maintenance.md:23-29`) says "until the release tool opens the PR itself" and prescribes deleting the local tag and moving the commit; splitting the command removes that manual sequence.
- No status table: vukem's tool rewrites a `docs/PLAN.md` status row; here the plan lives outside the repository (D-025), so the tool touches only `CHANGELOG.md` and the orchestrator updates the plan status after the release (section 7 step 12).
- The tool does not mark `v0.x` as pre-release. vukem's code does (`internal/tools/release/main.go`, `--prerelease` for `v0.`) while every release from `v0.5.0` on is published as a normal release (research note `vukem-practices-and-releases` section 2, VF-005); we follow what was observed. Only a tag containing `-` is marked pre-release, in the workflow.
- Tests: table-driven tests for the section insertion, footer rewriting and tag preconditions, as vukem's `changelog_test.go` and `bump_test.go`. The tool has no network access in tests.

## 9. Workflows

Common rules for every file in `.github/workflows/` (vukem `docs/tooling.md:151`, `.claude/rules/workflows.md`):

- `permissions: contents: read` at the top; a job widens permissions only for what it writes.
- `persist-credentials: false` on checkout when the job does not push.
- `concurrency` with `cancel-in-progress: true` on workflows that run on pull requests (never on `release.yml`).
- Every job has `timeout-minutes`. Jobs that gate a pull request have 20 at most (NFR-042). The release jobs and the weekly jobs state their own, truthfully: `release.yml` job `goreleaser` 30, job `image` 45, `fuzz.yml` 90, `long-verification.yml` 90 (crash and SIGKILL) and 100 (soak), other weekly jobs 20.
- Pinned actions, copied from vukem at the cited line (table 9.0).
- The repository variable `SITE_URL` is optional (SDD-010).

### 9.0 Pinned actions

All SHAs are copied from vukem workflows at `v0.43.0`; `oven-sh/setup-bun` (not used in vukem) is copied from the Studio's `ci.yml` (a pin only, the Studio workflow itself is not copied) and `actions/dependency-review-action` is read from its own repository.

| Action | SHA | Version | Source |
| --- | --- | --- | --- |
| `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1` | v7.0.1 | `release.yml:61` |
| `actions/setup-go` | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` | v7.0.0 | `release.yml:66` |
| `actions/setup-node` | `949feb2413d6458794dcd2491c4babbbce0c15c1` | v7.1.0 | `site.yml:34` |
| `oven-sh/setup-bun` | `0c5077e51419868618aeaa5fe8019c62421857d6` | v2.2.0 | Studio `ci.yml` |
| `sigstore/cosign-installer` | `6f9f17788090df1f26f669e9d70d6ae9567deba6` | v4.1.2 (cosign v3.1.3) | `release.yml:69-71` |
| `anchore/sbom-action/download-syft` | `66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c` | v0.24.3 (syft v1.54.1) | `release.yml:72-74` |
| `goreleaser/goreleaser-action` | `a4f02ab5355a589de60d917a659ce617ed19b3de` | v7.3.0 (GoReleaser v2.18.2) | `release.yml:75-78` |
| `actions/attest-build-provenance` | `4d101475d8b20a2381f78447822ac1eab6504dd8` | v4.2.2 | `release.yml:87` |
| `docker/setup-qemu-action` | `99012661954931238ded8c8b007157a8430204e1` | v4.4.0 | `release.yml:133` |
| `docker/setup-buildx-action` | `f87e5991a6d7451dcb8d9637bfbc97413f497069` | v4.4.1 | `release.yml:134` |
| `docker/login-action` | `8aa5831fb8d48d8f76594c7ddde61ae58019f550` | v4.8.0 | `release.yml:135` |
| `docker/build-push-action` | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` | v7.4.0 | `release.yml:140` |
| `golangci/golangci-lint-action` | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` | v9.3.0 (golangci-lint v2.14) | `ci.yml:25-27` |
| `DavidAnson/markdownlint-cli2-action` | `696eb3f2509270d7b882d2aa68347bc676e7af34` | v25.0.0 | `ci.yml:103` |
| `actions/upload-artifact` | `cf430e030ddbb5b0abf93d22962f4752f3646cd9` | v7.0.2 | `ci.yml:142` |
| `amannn/action-semantic-pull-request` | `48f256284bd46cdaab1048c3721360e808335d50` | v6.1.1 | `pr-title.yml:16` |
| `actions/upload-pages-artifact` | `fc324d3547104276b827a68afc52ff2a11cc49c9` | v5.0.0 | `site.yml:46-50` |
| `actions/deploy-pages` | `368f82528645a54fb793d4d04e342629a3f51346` | v5.0.1 | `site.yml:73-75` |
| `actions/dependency-review-action` | `a1d282b36b6f3519aa1f3fc636f609c47dddb294` | v5.0.0 | not in vukem; tag `v5.0.0` of `actions/dependency-review-action` read with `gh api repos/actions/dependency-review-action/git/ref/tags/v5.0.0` on 2026-11-14, latest stable release (SDD-007 section 12). `REL-CI` re-reads it before use |
| `ossf/scorecard-action` | `fa7117db83c52d787ae5a89f350999701505d78e` | v2.5.0 | `scorecard.yml:29` |
| `github/codeql-action/init`, `analyze`, `upload-sarif` | `9b3254085494b55a0f7c4c2b3d2d52cc712ce370` | v4.39.1 | `codeql.yml`, `scorecard.yml:39` |

(`site.yml`, `pr-title.yml`, `scorecard.yml` line numbers are relative to each file.) **No action is left to pin.** The Codecov action of vukem (`ci.yml:151`) is not used: Decision of this SDD, because the covergate is the required check and Codecov needs an external account (vukem's own comment says its upload is allowed to fail, `ci.yml:146-148`).

Tools pinned by version in workflows: `govulncheck@v1.9.0` (`ci.yml:166`), Betterleaks `1.9.0` with the sha256 `f8b185a39ffcece2a1ca82bf3a4e7435cd81963ffd16b7a9128daf75f35f6de7` of `betterleaks_1.9.0_linux_x64.tar.gz` (`secrets.yml:18` and `secrets.yml:20`), Bun `1.4.4` (Studio `ci.yml`), Node `24` (`site.yml:35-37`), markdownlint-cli2 `0.24.1` for local runs (`bunx markdownlint-cli2@0.24.1`, the newest stable on 2026-11-14; `REL-TOOLS` re-reads it), GoReleaser `v2.18.2`, cosign `v3.1.3`, syft `v1.54.1`. git-cliff runs only on the maintainer's machine; its exact version is recorded in the required-tools table of `.github/CONTRIBUTING.md` by `REL-TOOLS`.

### 9.1 Workflow inventory

| File | Name | Trigger | Gates |
| --- | --- | --- | --- |
| `ci.yml` | CI | push to `main`, pull request, manual | Merge (required checks) |
| `pr-title.yml` | PR title | pull request opened, edited, synchronize, reopened | Merge (required) |
| `release.yml` | release | tag `v*`, manual with `version` | The release itself |
| `desktop.yml` | desktop | called by `release.yml`, pull request touching `desktop/**`, manual | The desktop installers of the release (section 10.6); not required on a pull request |
| `image.yml` | image | pull request touching the image inputs, manual | Merge when the image inputs change (not required; path-filtered) |
| `site.yml` | Site | push to `main` and pull request touching `site/**`, `CHANGELOG.md` or the workflow; manual | The site deploy |
| `docs-last-updated.yml` | Docs last updated | every push to `main`, every pull request, manual | Merge (required) |
| `secrets.yml` | Secrets | push to `main`, pull request, weekly, manual | Merge (required) |
| `codeql.yml` | CodeQL | push to `main`, pull request, weekly | Informational, `ci-failure` issue |
| `vuln.yml` | Vulnerabilities (weekly) | weekly, manual | `ci-failure` issue |
| `scorecard.yml` | Scorecard | branch protection rule, push to `main`, weekly, manual | `ci-failure` issue |
| `fuzz.yml` | Fuzz | weekly (cron `17 3 * * 0`), manual with `fuzztime` and `long_fuzztime` | `ci-failure` issue (the long run; the 30 s pull request run is the `Fuzz (30 s)` job of `ci.yml`) |
| `long-verification.yml` | Long verification | manual only, with `crash_count` (default 100), `kill_minutes` (default 10) and `soak_duration` (default `1h`); never on a schedule, a push or a pull request | The result of the run (TASK-0144, TASK-0146); logs are artifacts. Jobs: `crash` on ubuntu, windows and macos runs `go test -tags crashtest ./web/app/servicetest/ -run TestCrash -count=100 -timeout 60m`, then repeats `-run TestKill` (the in-process kill and the real `serve` killed during concurrent HTTP commits; `SIGKILL`, and `TerminateProcess` on Windows) for `kill_minutes`; `soak` on ubuntu runs `go test -tags soak -run TestSoak -timeout 90m ./web/app/servicetest/` with `STUARTY_SOAK_DURATION`. Command: `gh workflow run long-verification.yml` |
| `agent-harness.yml` | Agent harness | manual only, with `scenario`, `runs` (default 10) and `model` (default `claude-sonnet-5-5`); never on a schedule, a push or a pull request | The release checklist (live agent run and vendor token count of `tools/list`); not required; uploads the transcripts as an artifact; fails first, naming it, when the secret `ANTHROPIC_API_KEY` is missing |
| `ffmpeg-freshness.yml` | FFmpeg freshness | weekly, manual | `ci-failure` issue |
| `ci-failure.yml` | CI failure issue | `workflow_run` of the weekly workflows failing | Blocks the next release (R25) |

vukem's `bench.yml`, `downstream.yml` and `starter-deps.yml` are not copied: no benchmarks, downstream apps or scaffold pins exist here. `fuzz.yml` is vukem's (`fuzz.yml`, weekly, `fuzztime` input) with our targets from SDD-007 section 12 (`FuzzCanonicalizeImportPath`, `FuzzSniffMedia`, `FuzzUntrustedClean`, `FuzzHostCheck`, `FuzzFontHeader`, `FuzzTemplateArchive`, `FuzzFFArgsCheck`); the task that adds a fuzz target adds it to the matrix of `fuzz.yml` and of the `Fuzz (30 s)` job. `ci-failure.yml` lists `Vulnerabilities (weekly)`, `Scorecard`, `Secrets`, `CodeQL`, `Fuzz` and `FFmpeg freshness`.

`ffmpeg-freshness.yml` is new (SDD-007 section 12): a weekly job that compares the FFmpeg release pinned in `.config/ffmpeg.env` with the newest patch release of its branch (`gh api` on the FFmpeg tags), opens or updates one issue labelled `ffmpeg-freshness` when a newer patch exists or the pin is older than 90 days, and fails the run in the second case. The decision is `internal/tools/ffmpegfresh`, which reads no network (the workflow passes the tags in).

### 9.2 `pr-title.yml`

Adapted from `vukem/.github/workflows/pr-title.yml` (lines 1-33): the same action and types, plus the `scopes` list and a job name that can be a required check.

```yaml
name: PR title

on:
  pull_request:
    types: [opened, edited, synchronize, reopened]

permissions:
  contents: read

jobs:
  conventional:
    name: Conventional title
    runs-on: ubuntu-latest
    timeout-minutes: 5
    permissions:
      pull-requests: read
    steps:
      - uses: amannn/action-semantic-pull-request@48f256284bd46cdaab1048c3721360e808335d50 # v6.1.1
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        with:
          types: |
            build
            chore
            ci
            docs
            feat
            fix
            perf
            refactor
            revert
            style
            test
          scopes: |
            core
            timeline
            schema
            command
            plan
            lint
            engine
            raster
            ingest
            export
            transcribe
            service
            web
            api
            mcp
            cli
            ui
            site
            docs
            repo
            ci
            deps
            deps-dev
            release
            adr
            sdd
            go
            security
            desktop
            http
            ffmpeg
            renderer
            docker
            library
            events
            cmd
            contract
            pathguard
            remote
            timeval
            store
            notices
            projects
            preview
            owner
            e2e
          requireScope: false
          subjectPattern: ^[^A-Z].*[^.]$
          subjectPatternError: Description must start lowercase and must not end with a period.
```

A second job of the same file, `Conventional commits` (**Decision of REL-CI**, because pull requests are rebase-merged and every commit reaches `main`, ADR-0028), checks out with `fetch-depth: 0` and tests every subject of `git log base..head` with the rules of section 6.1 and the 31 scopes of section 6.2 (merge commits and `Revert "..."` subjects are skipped; `fixup!` and `squash!` fail). Its header is at most 100 characters. It needs no tool, so it does not wait for `commitlint` of `REL-TOOLS`.

Gate: merge. Secrets: none beyond `GITHUB_TOKEN`.

### 9.3 `release.yml` (full)

Adapted from `vukem/.github/workflows/release.yml` (R14 to R24). Changes, with reasons:

1. Step "Install and build the web assets" and `oven-sh/setup-bun`: the binary embeds the React build (RF-200); vukem's binary has no front end.
2. Step "Notices are current": RF-215, NFR-043.
3. Step "Publish the release notes": NFR-044, R6, R7. Placed before GoReleaser so `keep-existing` finds the release with its notes.
4. Provenance bundle file name `stuarty.intoto.jsonl`; a step that asserts the asset count (15 at the Prototype; 26 with the installers and the desktop application, 29 from v0.18.0 with the launcher artifacts, 30 with the feed, see section 10.6).
5. The `image` job pushes to two registries and signs and attests both digests (R22); a first step fails early and clearly when the Docker Hub secrets are missing (ADR-0025 consequences).
6. The image build context is the local checkout (`context: .`), exactly as vukem, so `.git` is in the context and the binary reports the tag (see section 11).
7. The `image` job builds and smoke tests the amd64 image with `load: true` before the multi-architecture push, so a broken image never reaches `latest`. At `v0.43.0` vukem pushes directly; the build-then-test-then-push order is present on vukem main after v0.43.0 and is copied from there. The smoke test is the script `scripts/image-smoke.sh`, shared with `image.yml` and `task image:smoke` (section 9.5).
8. The hook "assert the web build output" of `.config/goreleaser.yaml` (section 10) fails the release job when the web build produced no asset, so a binary without the UI is never published.
9. The desktop application (D-033, section 10.6): the job `desktop` (reusable workflow `desktop.yml`) builds the installers, `goreleaser` attaches them with one SBOM to the same release and to `checksums.txt` (26 assets, not 21; 29 from v0.18.0, with the three launcher artifacts of section 10.6), there is no signing or notarization step and no code signing secret. For a stable tag the job `preflight` checks the secret `UPDATE_FEED_SIGNING_KEY` and the variable `UPDATE_FEED_PUBLIC_KEY` first, the job `feed` signs and attaches `feed.json` (27 assets, 30 from v0.18.0), and the job `pages` starts `site.yml`, which publishes the feed and the installers on GitHub Pages.

```yaml
name: release

# Builds the stuarty binaries and the container image when a version tag
# is pushed. The release and its notes are created here from CHANGELOG.md
# (empty notes fail the run); GoReleaser only attaches the archives, checksums,
# SBOMs (syft) and the cosign keyless bundle of checksums.txt. The image job
# publishes to GHCR and Docker Hub for stable tags only.
#
# workflow_dispatch takes the version to build, because a tag push does not
# always start a run: GitHub creates no push event when more than three tags
# go up in a single push. Version tags are pushed one at a time, and a tag
# that went up without a run is recovered by dispatching that version here
# instead of moving the tag.
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
    timeout-minutes: 30
    permissions:
      contents: write # create the release and upload assets
      id-token: write # Sigstore OIDC identity for cosign and attestations
      attestations: write # build provenance attestations
    outputs:
      tag: ${{ steps.tag.outputs.tag }}
      stable: ${{ steps.tag.outputs.stable }}
    steps:
      - name: Resolve the tag
        id: tag
        env:
          INPUT_VERSION: ${{ inputs.version }}
          REF_NAME: ${{ github.ref_name }}
        run: |
          tag="${INPUT_VERSION:-$REF_NAME}"
          case "$tag" in
            v[0-9]*.[0-9]*.[0-9]*) ;;
            *)
              echo "::error::not a version tag: $tag"
              exit 1
              ;;
          esac
          echo "tag=$tag" >>"$GITHUB_OUTPUT"
          # Only vX.Y.Z moves the image's latest tag; vX.Y.Z-rc.1 and friends do not.
          if [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "stable=true" >>"$GITHUB_OUTPUT"
          else
            echo "stable=false" >>"$GITHUB_OUTPUT"
          fi
      # GoReleaser reads the version from HEAD, so a dispatched run checks out
      # the tag itself rather than the branch the dispatch came from.
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          ref: refs/tags/${{ steps.tag.outputs.tag }}
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
      - uses: oven-sh/setup-bun@0c5077e51419868618aeaa5fe8019c62421857d6 # v2.2.0
        with:
          bun-version: 1.4.4
      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.3
      - uses: anchore/sbom-action/download-syft@66cbf4bc1f1c0d2edc94016e65bc221b6bb0ad6c # v0.24.3
        with:
          syft-version: v1.54.1
      # The notes come from the changelog section of the tagged commit. The
      # heading match is literal: a regex with \[ never matches in awk.
      - name: Publish the release notes
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ steps.tag.outputs.tag }}
        run: |
          set -euo pipefail
          awk -v v="${TAG#v}" '
            index($0, "## [" v "]") == 1 { found = 1; next }
            found && index($0, "## [") == 1 { exit }
            found && /^\[[^]]+\]: / { exit }
            found
          ' CHANGELOG.md | sed '/./,$!d' >"$RUNNER_TEMP/notes.raw"
          printf '%s\n' "$(cat "$RUNNER_TEMP/notes.raw")" >"$RUNNER_TEMP/notes.md"
          if ! grep -q '[^[:space:]]' "$RUNNER_TEMP/notes.raw"; then
            echo "::error::CHANGELOG.md has no entries for ${TAG#v}"
            exit 1
          fi
          flags=()
          case "$TAG" in *-*) flags+=(--prerelease) ;; esac
          if gh release view "$TAG" >/dev/null 2>&1; then
            gh release edit "$TAG" --title "$TAG" --notes-file "$RUNNER_TEMP/notes.md" "${flags[@]}"
          else
            gh release create "$TAG" --title "$TAG" --notes-file "$RUNNER_TEMP/notes.md" --verify-tag "${flags[@]}"
          fi
      - name: Install the web dependencies
        working-directory: web
        run: bun install --frozen-lockfile
      - name: Notices are current
        run: go run ./internal/tools/notices -check
      - uses: goreleaser/goreleaser-action@a4f02ab5355a589de60d917a659ce617ed19b3de # v7.3.0
        with:
          version: "v2.18.2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      - name: Verify the checksums signature
        run: |
          cosign verify-blob dist/checksums.txt \
            --bundle dist/checksums.txt.sigstore.json \
            --certificate-identity "https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release.yml@${GITHUB_REF}" \
            --certificate-oidc-issuer https://token.actions.githubusercontent.com
      - uses: actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2
        id: attest
        with:
          subject-checksums: ./dist/checksums.txt
      # Also attach the provenance bundle to the release, so tools that read
      # release assets (OpenSSF Scorecard) see it next to the archives.
      - name: Upload the provenance bundle
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          BUNDLE: ${{ steps.attest.outputs.bundle-path }}
          TAG: ${{ steps.tag.outputs.tag }}
        run: |
          cp "$BUNDLE" stuarty.intoto.jsonl
          gh release upload "$TAG" stuarty.intoto.jsonl --clobber --repo "$GITHUB_REPOSITORY"
      # 6 archives + 6 SBOMs + checksums.txt + its bundle + the provenance.
      - name: Check the release assets
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAG: ${{ steps.tag.outputs.tag }}
        run: |
          count="$(gh release view "$TAG" --repo "$GITHUB_REPOSITORY" --json assets --jq ".assets | length")"
          body="$(gh release view "$TAG" --repo "$GITHUB_REPOSITORY" --json body --jq ".body | length")"
          echo "assets=$count body_length=$body"
          [ "$count" -eq 15 ] && [ "$body" -gt 0 ]

  image:
    needs: goreleaser
    if: needs.goreleaser.outputs.stable == 'true'
    runs-on: ubuntu-latest
    timeout-minutes: 45
    env:
      GHCR_IMAGE: ghcr.io/alexgalhardo/stuarty
      HUB_IMAGE: alexgalhardo/stuarty
      TAG: ${{ needs.goreleaser.outputs.tag }}
    permissions:
      contents: read
      packages: write # push the image to GHCR
      id-token: write # Sigstore OIDC identity for cosign and attestations
      attestations: write # build provenance attestation of the image
    steps:
      - name: Check the Docker Hub credentials
        env:
          HUB_USER: ${{ vars.DOCKERHUB_USERNAME }}
          HUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}
        run: |
          if [ -z "$HUB_USER" ] || [ -z "$HUB_TOKEN" ]; then
            echo "::error::add the repository variable DOCKERHUB_USERNAME and the secret DOCKERHUB_TOKEN (see SDD-009 section 16)"
            exit 1
          fi
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          ref: refs/tags/${{ env.TAG }}
          fetch-depth: 0
          persist-credentials: false
      - name: Resolve the image tags
        id: meta
        run: |
          version="${TAG#v}"
          # latest moves only for the highest stable tag of the repository: a run of an
          # older tag (a workflow_dispatch re-run of v0.18.0 after v0.20.0 did this on
          # 2026-11-16) must not take latest back.
          highest="$(git ls-remote --tags --refs "https://github.com/${GITHUB_REPOSITORY}.git" 'v*'             | sed 's#.*refs/tags/v##' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)"
          echo "highest stable tag: v${highest}, this run: v${version}"
          {
            echo "version=$version"
            echo "revision=$(git rev-parse HEAD)"
            echo "tags<<EOF"
            for image in "$GHCR_IMAGE" "$HUB_IMAGE"; do
              echo "$image:$version"
              echo "$image:${version%.*}"
              if [ "$version" = "$highest" ]; then
                echo "$image:latest"
              fi
            done
            echo "EOF"
          } >>"$GITHUB_OUTPUT"
      - uses: docker/setup-qemu-action@99012661954931238ded8c8b007157a8430204e1 # v4.4.0
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/login-action@8aa5831fb8d48d8f76594c7ddde61ae58019f550 # v4.8.0
        with:
          username: ${{ vars.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}
      # Build and test the amd64 image locally first (load: true); nothing is
      # pushed, and so latest does not move, until the smoke test passes.
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          file: .config/docker/stuarty.Dockerfile
          platforms: linux/amd64
          load: true
          tags: stuarty:release-smoke
          build-args: |
            VERSION=${{ steps.meta.outputs.version }}
            REVISION=${{ steps.meta.outputs.revision }}
      - name: Smoke test the image before publishing
        run: bash scripts/image-smoke.sh stuarty:release-smoke
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        id: build
        with:
          context: .
          file: .config/docker/stuarty.Dockerfile
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          build-args: |
            VERSION=${{ steps.meta.outputs.version }}
            REVISION=${{ steps.meta.outputs.revision }}
          sbom: true
          provenance: mode=max
      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.3
      - name: Sign the image digest in both registries
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
        run: |
          cosign sign --yes "$GHCR_IMAGE@$DIGEST"
          cosign sign --yes "$HUB_IMAGE@$DIGEST"
      - uses: actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2
        with:
          subject-name: ghcr.io/alexgalhardo/stuarty
          subject-digest: ${{ steps.build.outputs.digest }}
          push-to-registry: true
      - uses: actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2
        with:
          subject-name: index.docker.io/alexgalhardo/stuarty
          subject-digest: ${{ steps.build.outputs.digest }}
          push-to-registry: true
      - name: Verify the signatures
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
        run: |
          for image in "$GHCR_IMAGE" "$HUB_IMAGE"; do
            cosign verify "$image@$DIGEST" \
              --certificate-identity "https://github.com/${GITHUB_REPOSITORY}/.github/workflows/release.yml@${GITHUB_REF}" \
              --certificate-oidc-issuer https://token.actions.githubusercontent.com
          done
```

Gate: nothing after it; it is the release. Secrets: `GITHUB_TOKEN` (automatic), `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` (section 16). Permissions: shown per job.

Known property inherited from vukem (`docs/tooling.md:178`): a `workflow_dispatch` recovery run signs with the identity `@refs/heads/<branch>`, not `@refs/tags/v...`; the regexp in section 7 step 11 then does not match. Say so on the release and verify with `release\.yml@refs/heads/main$`.

### 9.4 `ci.yml`

Adapted from `vukem/.github/workflows/ci.yml`. The header is vukem's (`ci.yml:3-14`): triggers `push` to `main`, `pull_request`, `workflow_dispatch`; `permissions: contents: read`; `concurrency` group `${{ github.workflow }}-${{ github.ref }}` with `cancel-in-progress: true`. Every job has `timeout-minutes: 20`. Every job starts with `actions/checkout` (SHA of table 9.0, `persist-credentials: false`, `fetch-depth: 0` only where noted) and, for Go jobs, `actions/setup-go` with `go-version-file: go.mod` and `cache-dependency-path: go.sum`.

**What `REL-CI` wrote (the code differs from the table in these points, and this paragraph is the contract).** `ci.yml` holds jobs 1, 2, 3, 4, 8 and 10 now. The others are added by the tasks that create their tools, each with the display name below. Jobs 1 and 2 call the task runner (`task lint:go`, `task vet`, `task tidy:check`, `task lint:md`) after installing go-task `v3.54.0` with `go install` and golangci-lint `v2.15.0` with `golangci/golangci-lint-action` in `install-only` mode, so CI and a developer run the same commands; job 2 therefore runs `markdownlint-cli2` `0.24.1` through `bunx` and not the DavidAnson action. Job 3 has no `bun run test` step until a unit test runner is approved. Job 4 does not call `scripts/install-ffmpeg.sh` until a test needs FFmpeg, its covergate step is `continue-on-error: true` while the skeleton packages are below the floors of section 13.2 (the step prints them), and the `Test` job names are `Test (ubuntu-latest)`, `Test (windows-latest)` and `Test (macos-latest)` as the ruleset needs. Job 8 is named `Vulnerabilities`. Every job has `persist-credentials: false` and `timeout-minutes: 20`.

The jobs below are listed by their display name, which is the string the ruleset requires (section 16). The vukem jobs for drivers, contrib modules, `apidiff` and `openapi golden` do not exist here (one module, one application); `apidiff` returns when a Go package is meant to be imported.

| # | Job (display name) | Runs on | Steps, in order, exact commands | Notes |
| --- | --- | --- | --- | --- |
| 1 | `Lint (Go)` | ubuntu | `golangci/golangci-lint-action` with `version: v2.14`; `go vet ./...`; `go fix -diff ./... > fix.diff \|\| true` and a `::warning` when non-empty (vukem `ci.yml:30-33`); the `//nolint:gocognit` budget check of `ci.yml:44-55` with `GOCOGNIT_BUDGET: "0"` | gofumpt, gci, golines, gosec, `forbidigo` (SDD-007 section 10.2) and the depguard import rules of ADR-0001 run inside golangci-lint (section 15.3) |
| 2 | `Lint (Markdown)` | ubuntu | `DavidAnson/markdownlint-cli2-action` with `globs: "**/*.md"` | config `.markdownlint-cli2.jsonc` |
| 3 | `Types and lint (web)` | ubuntu | `oven-sh/setup-bun` `bun-version: 1.4.4`; in `web/`: `bun install --frozen-lockfile`; `bun run typecheck`; `bun run lint`; `bun run test --if-present` | Studio `ci.yml` jobs `types`; `bun run lint` is `biome ci` |
| 4 | `Go tests (${{ matrix.shard }}, ${{ matrix.os }})` and `Test (${{ matrix.os }})` | `Go tests`: an `include` list per system (section 13.4), `fail-fast: false`: `export-K` (every Nth test of `./engine/export/`, by sorted name, N = 3 on Linux, 5 on Windows, 1 on macOS), `engine` (the rest of `./engine/...`), `rest` (everything else) and, on Windows only, `contract` (`./web/contract/...`, taken out of `rest`); `Test`: the three systems on `ubuntu-latest`, needs every shard and checks that the number of shards of its system (5, 8, 3) passed | `Go tests`: `bash scripts/install-ffmpeg.sh`; `go test -race -count=1 -timeout=17m -coverpkg=./... -coverprofile=coverage.out <shard packages>` with `shell: bash`; the export benchmark in the `export-1` shard; on Linux, `actions/upload-artifact` of the profile (`coverage-<shard>`). `Test`: checks through the jobs API that the five shards of its own system passed (a skipped required check counts as a pass, so the job runs with `!cancelled()` and fails on anything but success); on Linux only, merges the profiles, runs `go run ./internal/tools/covergate -v coverage.out` and uploads the merged `coverage.out` (name `coverage`) | `-race` needs cgo, present on all three runners (vukem `ci.yml:134`). The Windows step prints the CPU first as vukem does (`ci.yml:128-130`) because of golang/go#81238. `fetch-depth: 0` is not needed. On Windows `scripts/install-ffmpeg.sh` unpacks the BtbN `.zip` with `tar -xf` (bsdtar ships with Windows) or `powershell -Command Expand-Archive`, because Git Bash has no `unzip` |
| 5 | `Browser tests (Playwright)` | ubuntu | setup-go, setup-bun; `bash scripts/install-ffmpeg.sh`; in `web/`: `bun install --frozen-lockfile`, `bun run build`, `bunx playwright install --with-deps chromium`, `bun run e2e -- --project=chromium --project=failing-encoder --project=desktop-detect`; on failure the traces of `web/e2e/test-results/` are uploaded | Studio `ci.yml` job `e2e`; Firefox and WebKit run in job 21 |
| 6 | `Generated files` | ubuntu | setup-go; `actions/setup-node` (pinned, `node-version: 24`, npm cache on `site/package-lock.json`); `go run ./internal/tools/contractgen` (writes `core/schema/` and `web/resources/js/generated/`); `go run ./web/cmd/gen-agent-docs --check` (SDD-003 section 7); `cd site && npm ci && npm run reference`; `git diff --exit-code -- core/schema web/resources/js/generated site/generated site/content/generated` | the generated-schema check of ADR-0014 item 2 and the reference check of ADR-0024 item 5. `npm run reference` runs the four generators with the flags of SDD-010 section 8.2 (`stuarty command:help --json`, `stuarty openapi:export -prefix /api/v1`, `go run ./internal/tools/configref -json`, `go run ./internal/tools/contractgen -errors`), so the flags are defined once. Node 24 is needed because `site/scripts` run with Node type stripping. Prints the diff on failure |
| 7 | `Notices` | ubuntu | setup-go, setup-bun; `bun install --frozen-lockfile` in `web/`; `go run ./internal/tools/notices -check` | section 12 |
| 8 | `Vulnerabilities` | ubuntu | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | vukem `ci.yml:166` |
| 9 | `Release build (snapshot)` | ubuntu | setup-go, setup-bun, syft installer; `bun install --frozen-lockfile` in `web/`; `goreleaser/goreleaser-action` with `version: "v2.18.2"` and `args: release --snapshot --clean --skip=sign,publish`; for every archive, `for f in dist/stuarty_*.tar.gz dist/stuarty_*.zip; do [ "$(stat -c %s "$f")" -lt 60000000 ] \|\| exit 1; done` | NFR-041: every archive is under 60,000,000 bytes (60 MB, the unit of the requirement). This is the dry run that catches a broken `.config/goreleaser.yaml` before a tag |
| 10 | `Workflow pins` | ubuntu | `actions/checkout`; the pin check of section 13.1 (an `if` block, so a pinned tree passes and an unpinned line fails) | enforces R23 mechanically; required |
| 11 | `Security tests (${{ matrix.os }})` | matrix `ubuntu-latest`, `windows-latest`, `macos-latest`, `fail-fast: false` | setup-go; `bash scripts/install-ffmpeg.sh`; `go test -count=1 -v -timeout=17m -run TestSecurity_ ./... \| tee security.log` (no race detector, as in the export shards; the full suite of job 4 keeps it for core, service and web/app); then the skip check of section 9.4.1 (on macOS, where only Homebrew's GPL FFmpeg exists, the skips are listed in the job summary, not failed, risk R6) | dedicated job of SDD-007 section 12, in addition to the full suite of job 4; fails on a skipped security test that is not marked with its platform in the name |
| 12 | `Threat coverage` | ubuntu | setup-go; `go run ./internal/tools/threatcheck` | SDD-007 section 12: a threat row without its named test, a `TestSecurity_T..` test no row names, a threat without a mitigation, or an operation missing from the SDD-007 section 5 table fails |
| 13 | `Vulnerabilities (UI)` | ubuntu | setup-bun `1.4.4`; in `web/`: `bun audit --audit-level=high` | SDD-007 section 12; high or critical fails. Also runs weekly in `vuln.yml` |
| 14 | `Dependency review` | ubuntu, `pull_request` only | `actions/dependency-review-action` (SHA of table 9.0) with `fail-on-severity: high` and `allow-licenses` equal to the allow-list of `internal/tools/notices/spdx.go` (section 12.2 item 4: MIT, BSD-2-Clause, BSD-3-Clause, ISC, Apache-2.0, MPL-2.0, 0BSD, Unlicense, OFL-1.1, LGPL-3.0-or-later, blessing, Zlib, WTFPL) | SDD-007 section 12 and NFR-043: a new dependency with a high or critical advisory or a licence outside the allow-list (the GPL family included) fails. Built in `ci.yml`; changing the allow-list means editing both places. The repository needs the dependency graph enabled, which only the first pull request shows |
| 15 | `Fuzz (30 s)` | ubuntu | setup-go; `bash scripts/fuzz-changed.sh 30s` | runs each target of SDD-007 section 12 whose package changed in the pull request for 30 seconds with `go test -run ^$ -fuzz <Target> -fuzztime 30s`; passes with no target when none changed. The long weekly run is `fuzz.yml` (section 9.1) |
| 16 | `No egress` | ubuntu | setup-go; `sudo apt-get install -y strace`; `bash scripts/install-ffmpeg.sh` (puts the pinned FFmpeg on `PATH`); `bash scripts/no-egress.sh` | SDD-007 section 10.2 item 3: the script builds the binary and runs `serve`, a CLI session (create, import, edit, export, view), the BETA scenario of TASK-0149 (every export format `mp4`, `gif`, `wav`, `aac` and `png`; OTIO export and import; bundle export and import; media and font import; a template saved, written and imported; the brand kit with a logo, colours and a font; `tools:install`, which must stop at its question and is refused with `--yes` outside a person's session, so no helper is downloaded) and an MCP stdio session, each under `strace -f --seccomp-bpf -e trace=connect,sendto,sendmsg,sendmmsg`, so FFmpeg is traced as a child of the owner; any socket address that is not loopback or a Unix socket fails, and so does a trace with no loopback call. The verdict is `scripts/no-egress-scan.sh`, tested on fixtures by `TestSecurity_T10_EgressScanner`. The desktop binary has no headless mode and is not run. The helper download is the one allowed outbound path and is not part of the job. About 1 minute once the binary is built (27 traced processes), far inside the 20 minute budget. Built in `ci.yml`; the script also runs in WSL or on any Linux host with `strace` and FFmpeg |
| 17 | `Sandbox (bwrap)` | ubuntu | setup-go; `sudo apt-get install -y bubblewrap`; `go test -count=1 -run TestSecurity_T16_BwrapHasNoNetwork ./...` | SDD-007 section 12. If the runner restricts unprivileged user namespaces, the job first sets `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` |
| 18 | `Contract and parity` | ubuntu | setup-go; `bash scripts/install-ffmpeg.sh`; `go test -count=1 ./web/contract/...` | the `contract` job of SDD-003 section 7 (check 26): the contract scenarios on the six drivers, `TestSchemasMatch`, `TestSummariesMatch` (NFR-025, RF-104) |
| 19 | `OpenAPI (Redocly)` | ubuntu | setup-go; `go run ./web/cmd/app openapi:export -prefix /api/v1 > "$RUNNER_TEMP/openapi.json"`; `bunx @redocly/cli@2.78.0 lint "$RUNNER_TEMP/openapi.json"` | SDD-003 check 22: the OpenAPI document is valid 3.1. The Redocly version is the newest stable on 2026-11-14; `REL-CI` re-reads it |
| 20 | `Golden cases (PATH FFmpeg)` | ubuntu | setup-go; `sudo apt-get install -y ffmpeg`; `STUARTY_FFMPEG=$(command -v ffmpeg) go test -count=1 -run Golden ./engine/export/...` | SDD-004 section 11: the golden cases run with the FFmpeg found on the path, PSNR at least 50 dB per frame against the references; `continue-on-error: true`, reported and not blocking. The distribution package is used for this test only and is never shipped (NFR-043) |
| 21 | `Browser tests (${{ matrix.browser }})` | matrix `firefox`, `webkit`, `fail-fast: false`, ubuntu, `timeout-minutes: 20` | as job 5, but `bun run build:e2e`, `bunx playwright install --with-deps "$BROWSER"` and `bunx playwright test -c e2e/playwright.config.ts --project="$BROWSER"`; the specs tagged `CHROMIUM_ONLY` are skipped by the config; on failure the same traces are uploaded | SDD-012 section 21.1, ADR-0023 item 5 (P3-REL-005); parallel with job 5 |
| 22 | `Bundle export (${{ matrix.os }})` | matrix `ubuntu-latest`, `windows-latest`, `macos-latest`, `fail-fast: false` | setup-go; `bash scripts/install-ffmpeg.sh`; `bash scripts/bundle-matrix.sh export "$RUNNER_TEMP/bundle-<OS>.vvbundle"`; upload the file as artifact `bundle-<OS>` | BETA exit criterion 5: the bundle of the reference project, made with fixed ids and bit-exact sample audio |
| 23 | `Bundle matrix (${{ matrix.os }})` | the same matrix, `needs` job 22 | setup-go; `bash scripts/install-ffmpeg.sh`; download the three artifacts; `bash scripts/bundle-matrix.sh verify <the three files>` | each system imports the three bundles (two made elsewhere, one its own: six cross imports and three local), exports each again and compares the document hash and the media hashes (`internal/tools/bundlehash`) with those of the source and with the other bundles |
| 24 | `Renderer (ubuntu-24.04)` | `runs-on: ubuntu-24.04` (pinned, no matrix), `timeout-minutes: 30` | the toolchain pinned in `rust-toolchain.toml` (rustup installs it on the first `cargo` call); `cargo install --locked cargo-deny` at the version of `CARGO_DENY_VERSION` (no wasm-bindgen-cli: the module has no glue); in `renderer/`: `cargo fmt --check`, `cargo clippy --locked --all-targets -- -D warnings`, `cargo test --locked`, `cargo deny check`; then `bash scripts/build-renderer.sh check` | ADR-0029 and `.agents/rules/rust.md`: the Rust gate of `renderer/` and a WebAssembly module that builds twice from clean to the same SHA-256 and is the one embedded in `engine/renderhost/`. `cargo deny` uses the allow-list of `internal/tools/notices/spdx.go`, with the per-crate exceptions of `renderer/deny.toml` (`unicode-ident`, `yoke`, `yoke-derive`, `zerofrom` and `zerofrom-derive`, Unicode-3.0) (also written in `internal/tools/notices/spdx.go`); the crates compiled into the module are also covered by `go run ./internal/tools/notices -check` |
| 25 | `Determinism (${{ matrix.os }})` and `Determinism (compare)` | matrix `ubuntu-latest`, `windows-latest`, `macos-latest`, `fail-fast: false`; the compare job on ubuntu, `needs` the matrix | setup-go; `VUKEM_DETERMINISM_OUT=<file> go test -count=1 -run TestCanonicalHashesAreStable ./core/command/` writes one sha256 per golden case and seeded batch, uploaded as `hashes-<os>`; the compare job diffs the three files and fails naming the cases that differ | |

**Benchmark regression guard (TASK-0136).** `scripts/bench-gate.sh <pr|nightly> .github/bench-limits.txt <output>` reads the output of `go test -bench`, compares each metric with its limit (`min` or `max`, one line per limit) and exits 1 on a regression; a limit with no result in the output fails too, so a renamed or skipped benchmark cannot leave its guard silent. The `Go tests` job runs the `pr` profile (compositor, caption raster and commit round trip, `-benchtime` 20x, 3x or 300 ms, about 40 s) in the `export-1` shard on `ubuntu-latest` only, after the `BenchmarkExportCommonCase` step, which keeps its 1.5 floor; the `nightly` workflow checks every limit on the output of `-bench .`. The limits, the measures they come from and the margin (20 percent, SDD-008 section 7) are in SDD-004 section 12. The `Benchmarks` and `The 60 s export` steps of `nightly.yml` now run with `shell: bash`: the default shell of a step has no `pipefail`, so `| tee` hid the exit code of `go test` and the run of 2026-11-16 (`BenchmarkExportCommonCase` at 1.43 times real time against a floor of 1.5) was green.

**What P2-REL-005 added to `ci.yml`.** `Generated files` (job 6; it runs `task codegen` and checks `core/schema/`, `web/resources/js/generated/` and, when they exist, `site/generated/` and `site/content/generated/`; the same step moved out of `Lint (Go)`, and `gen-agent-docs --check` joins when that command exists), `Browser tests (Playwright)` (5), `Release build (snapshot)` (9, with `--skip=sign,publish`; SBOMs are built, the six archives are checked for size and count), `Sandbox (bwrap)` (17; it runs `TestSecurity_T16_` over `./engine/...`, fails on a skip, requires a PASS and, once `TestSecurity_T16_BwrapHasNoNetwork` exists, requires that test to pass) and `Golden cases (PATH FFmpeg)` (20, not blocking). The benchmark gate is not a job: the `Test` job runs `BenchmarkExportCommonCase` without the race detector and with `STUARTY_BENCH_MIN_RT=1.5` on the three runners (it skips where only a GPL FFmpeg exists, risk R6) and writes the numbers to the job summary. `nightly.yml` (daily, and on demand) runs the long benchmarks, the 60 s export test and the tests named `Soak` or `Fixture5Min`; `ci-failure.yml` watches it. `scripts/ci-time.sh [n]` prints the wall time and the slowest job of each of the last `n` finished (not cancelled) pull request runs of `CI`, their median and maximum, and the largest tracked file, and exits 1 when the median is 15 minutes or more or a run exceeds 20. `Security tests` (11, `security` in `ci.yml`, three operating systems) was added by TASK-0030. Still missing: jobs 13 to 16, 18 and 19, which wait for their tools (threatcheck, fuzz-changed, no-egress, contract, OpenAPI export).

#### 9.4.1 The security jobs

The skip check of job 11 fails when a security test was skipped on a platform its name does not mark. A test that is meant to run only on one platform carries the platform as the last name part (`_Linux`, `_Windows`, `_Darwin`); the check is:

```bash
if grep -E -- "--- SKIP: TestSecurity_" security.log | grep -vE "TestSecurity_[A-Za-z0-9_]*_(Linux|Windows|Darwin)"; then
  echo "::error::a security test was skipped without a platform marker"
  exit 1
fi
```

`scripts/fuzz-changed.sh`, `scripts/no-egress.sh` and `scripts/image-smoke.sh` are new scripts of `REL-CI` (the contents of the last two follow SDD-007 section 10.2 and section 9.5 below). Jobs 10 to 19 are required checks (section 16.1); job 20 is not (reported, not blocking).

Time budget (NFR-042, under 20 minutes): the jobs run in parallel, so the pull request time is that of the slowest job. Every job has `timeout-minutes: 20`; a job that hits the limit fails the check, which is the enforcement. The expected slowest job is the Windows test (vukem notes its test job needs up to about 7 minutes on a slow Windows runner, `ci.yml:133`). Acceptance check 6 measures it.

### 9.5 `image.yml`

Adapted from `vukem/.github/workflows/image.yml` (lines 1-41): builds the image on pull requests that change what goes into it, pushes nothing, then smoke tests it.

```yaml
name: image

# Builds the image on pull requests that change what goes into it, so a
# broken Dockerfile, a bad FFmpeg pin or a binary that no longer builds there
# is caught before a release. Nothing is pushed; release.yml publishes.
on:
  pull_request:
    paths:
      - ".config/docker/**"
      - ".config/ffmpeg.env"
      - ".github/workflows/image.yml"
      - "go.mod"
      - "go.sum"
      - "web/**"
      - "engine/**"
      - "core/**"
      - "service/**"
      - "THIRD_PARTY_NOTICES.md"
      - "licenses/**"
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  build:
    name: Image build
    runs-on: ubuntu-latest
    timeout-minutes: 20
    steps:
      # Full history and a local tag: the Dockerfile stamps the version from the
      # tag at HEAD and fails when it differs from VERSION. A dummy tag lets a
      # pull request run the same check a release runs, so a dirty build context
      # (+dirty) is caught here and not at the first tag. The tag is never pushed.
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - name: Create the local dummy tag
        run: git tag v0.0.0-ci
      - uses: docker/setup-buildx-action@f87e5991a6d7451dcb8d9637bfbc97413f497069 # v4.4.1
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          file: .config/docker/stuarty.Dockerfile
          load: true
          tags: stuarty:ci
          build-args: |
            VERSION=0.0.0-ci
      - name: Smoke test
        run: bash scripts/image-smoke.sh stuarty:ci
```

`scripts/image-smoke.sh` is shared by `image.yml`, the `image` job of `release.yml` and `task image:smoke`. It contains no apostrophes and no `latest`:

```bash
#!/usr/bin/env bash
# Smoke test of a built image: bash scripts/image-smoke.sh <image>
set -euo pipefail
img="${1:?usage: image-smoke.sh <image>}"
docker run --rm "$img" version
docker run --rm --entrypoint sh "$img" -c "id -u | grep -qx 10001"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -version | head -n 1"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -encoders | grep -q libopenh264"
docker run --rm --entrypoint sh "$img" -c "ffmpeg -hide_banner -f lavfi -i testsrc=duration=1:size=320x240:rate=25 -c:v libopenh264 -pix_fmt yuv420p /tmp/t.mp4 && ffprobe -v error -show_entries stream=codec_name -of csv=p=0 /tmp/t.mp4 | grep -qx h264"
docker run --rm --entrypoint sh "$img" -c "if ffmpeg -version | grep -q -e --enable-gpl -e --enable-nonfree; then exit 1; fi"
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/stuarty/THIRD_PARTY_NOTICES.md && test -s /usr/share/doc/stuarty/ffmpeg/LICENSE.txt && test -s /usr/share/doc/stuarty/ffmpeg/configure.txt && test -s /usr/share/doc/stuarty/ffmpeg/SOURCE.txt"
docker image inspect --format "{{json .Config.Healthcheck.Test}}" "$img" | grep -q 7878
```

The smoke test is the executable form of NFR-043 for the image: the FFmpeg inside has no `--enable-gpl` and no `--enable-nonfree`, can encode H.264 with the encoder that works in a container (OpenH264), the notices are present and the health check is declared. Because it runs `ffmpeg -version` and the reference export inside the image on the Debian runtime, it also verifies that the BtbN build runs there; proving that is the first subtask of `REL-DOCKER` (section 11.1).

### 9.6 `site.yml` and `docs-last-updated.yml`

`site.yml` is vukem's (`site.yml:1-75`) with these changes, specified in detail in SDD-010 section 12: the added steps `npm run reference:check`, `npm run check:built`; `SITE_URL` from the repository variable; `actions/upload-pages-artifact` path `site/out`. Trigger: push to `main` and pull request touching `site/**`, `CHANGELOG.md`, `core/schema/**`, `.github/workflows/site.yml`; manual. The step lists are identical in SDD-010 section 12.2: job `build` (`actions/checkout`, `actions/setup-go` with `go-version-file: go.mod` because `reference:check` builds the binary, `actions/setup-node` with Node 24 and the npm cache on `site/package-lock.json`, `npm ci`, `npm run test:site` (which includes the base path test, `test:base-path`), `npm run lint`, `npm run check:mdx`, `npm run reference:check`, `npm run build`, `npm run check:built`, upload artifact on `main` or manual), job `last-updated` (`fetch-depth: 0`, Node 24, `npm run test:scripts`, `npm run check:last-updated`), job `deploy` (only on `main` or manual, needs both, `environment: github-pages`, `actions/deploy-pages`). Permissions as vukem: `contents: read`; `build` also `actions: write` and `pages: write`; `deploy` `pages: write`, `id-token: write`. Gate: the deploy.

`docs-last-updated.yml` is vukem's (`docs-last-updated.yml:1-34`) unchanged except the name of the job: `Docs last updated` (so it can be a required check without clashing with the job `Last updated` of `site.yml`). It runs on every pull request so that a missing date fails even when `site/` is untouched.

### 9.7 `secrets.yml`, `codeql.yml`, `vuln.yml`, `scorecard.yml`, `ci-failure.yml`

| File | Content |
| --- | --- |
| `secrets.yml` | Copied from vukem (`secrets.yml:1-51`): Betterleaks `1.9.0` downloaded by exact URL and checked against `f8b185a39ffcece2a1ca82bf3a4e7435cd81963ffd16b7a9128daf75f35f6de7`, scanning `$PR_BASE..$HEAD_SHA` on pull requests, `$PUSH_BEFORE..$HEAD_SHA` on pushes, the full history weekly (cron `41 5 * * 1`). Job display name `Betterleaks`. Config `.betterleaks.toml` (moved from `web/`, section 17), extended with the rules of SDD-007 section 12: the key prefixes `vvl_` and `vvk_`, the `APP_KEY` value and the transcription key. The job `secrets` of the Studio's `ci.yml` is removed: it duplicates this workflow |
| `codeql.yml` | Copied from vukem: `languages: go`, `build-mode: autobuild`, push to `main`, pull request, cron `17 4 * * 1`; `security-events: write` on the job. JavaScript is added with the second matrix entry `languages: javascript-typescript` when the UI exists (task `REL-CI`) |
| `vuln.yml` | Reduced from vukem (`vuln.yml`): weekly (cron `17 6 * * 1`) and manual `govulncheck@v1.9.0 ./...` plus `bun audit --audit-level=high` in `web/`, no module matrix because there is one module |
| `scorecard.yml` | Copied from vukem (`scorecard.yml:1-41`): branch protection rule, push to `main`, cron `23 5 * * 1`, manual; `ossf/scorecard-action` with `publish_results: true`; upload SARIF with `actions/upload-artifact` and `github/codeql-action/upload-sarif`. The job must stay minimal (vukem's comment, lines 3-4 of the file) |
| `ci-failure.yml` | Copied from vukem: on `workflow_run` completed with `failure` of `Vulnerabilities (weekly)`, `Scorecard`, `Secrets`, `CodeQL`, `Fuzz`, `FFmpeg freshness`; opens or comments on an issue labelled `ci-failure` and `priority:p1` with `gh`. `issues: write`. The labels exist (section 16.3). An open issue blocks the next release (R25) |

What P1-REL-011 built, where it differs from the rows above and from sections 14 and 16:

- `scorecard.yml` is vukem's unchanged. `ci-failure.yml` is vukem's with a different filter: it watches `CI`, `Site`, `Fuzz`, `Vulnerabilities (weekly)`, `Scorecard`, `Secrets` and `CodeQL`, and opens or updates its issue only for a failure of a scheduled or manual run, or of a push to `main`; a pull request run never opens one (vukem lists only scheduled workflows).
- `fuzz.yml` has two jobs. `fuzz` is a matrix of one `ubuntu-latest` job per fuzz target of SDD-007 section 12 and of the BETA, 15 in all: `FuzzApplyBatch` and `FuzzApplyArgs` (`./core/command`), `FuzzImport` (`./core/otio`), `FuzzDecode` (`./core/timeline`), `FuzzParse` (`./core/timeval`), `FuzzCubeParse` (`./engine/lut`, the `.cube` lookup tables of SDD-004 section 3.6), `FuzzSniffMedia` and `FuzzFFArgsCheck` (`./engine/ffmpeg`), `FuzzFontHeader` (`./engine/raster`), `FuzzRenderInput` (`./engine/renderhost`), `FuzzTemplateArchive` (`./service`), `FuzzCanonicalizeImportPath` (`./service/pathguard`), `FuzzUntrustedClean` (`./service/untrusted`), `FuzzHostCheck` (`./web/app/http`) and `FuzzBundleArchive` (`./web/app/servicetest`); each runs for the input `fuzztime` (30 s by default, `30m` for the campaign of TASK-0151) with `timeout-minutes: 90`. Job `fuzz-long` runs `FuzzApplyBatch`, `FuzzSniffMedia`, `FuzzFFArgsCheck` and `FuzzFontHeader` for the input `long_fuzztime` (30 minutes by default) on `windows-latest` and `macos-latest` (the ubuntu run is the `fuzz` job), with `timeout-minutes: 90` (P4-ENG-001, P4-RND-002, TASK-0129). Both upload `testdata/fuzz/<Target>/` as the artifact `crashers-<Target>` (with `-<os>` for `fuzz-long`) when a run fails. The campaign is `gh workflow run fuzz.yml -f fuzztime=30m -f long_fuzztime=30m`; weekly (cron `17 3 * * 0`) the ubuntu run of each target is 30 s. The 30 s run on pull requests that change a target is the `Fuzz (30 s)` job of `ci.yml`.
- `.github/labels.txt` is the list of section 16.3. `internal/tools/labelsync` is vukem's tool with its test rewritten without `go-cmp` (not a dependency of this module); a second test compares the `area:` labels with the scopes of `pr-title.yml` and with the table of section 6.2, so the three lists cannot drift. `task labels:sync` runs it.
- `lint:site` (part of `task lint`) runs `npm ci`, `npm run lint`, `npm run build` and `npm run check:built` in `site/`, and is skipped while `site/package.json` does not exist.

## 10. GoReleaser

`.config/goreleaser.yaml`, adapted from `vukem/.config/goreleaser.yaml` (lines 1-53). Changes: names; `main: ./web/cmd/app`; a `before` hook that installs and builds the web assets, because the binary embeds them (RF-200); the archive file list adds the notices (RF-215).

```yaml
# GoReleaser builds the stuarty binaries for a tag and attaches them to
# the GitHub release that release.yml created from CHANGELOG.md (so GoReleaser
# keeps the notes). See docs/sdds/009-releases-and-ci-cd.md.
version: 2

project_name: stuarty

before:
  hooks:
    - cmd: bun install --frozen-lockfile
      dir: web
    - cmd: bun run build
      dir: web
    # Assert the web build output: without it the binary compiles and ships
    # without the UI (the embed directive also matches public/robots.txt).
    - cmd: bash scripts/assert-web-build.sh

builds:
  - id: stuarty
    main: ./web/cmd/app
    binary: stuarty
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    flags: [-trimpath]
    ldflags: ["-s -w"]
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - name_template: "stuarty_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE.md
      - README.md
      - CHANGELOG.md
      - THIRD_PARTY_NOTICES.md
      - licenses/**/*
      # the generated material for agents (SDD-003 section 8), at agent/ inside the archive
      - src: web/app/agentdocs/out/SKILL.md
        dst: agent/skills/stuarty
        strip_parent: true
      - src: web/app/agentdocs/out/AGENTS.snippet.md
        dst: agent
        strip_parent: true
      - src: web/app/agentdocs/out/mcp-instructions.txt
        dst: agent
        strip_parent: true

checksum:
  name_template: checksums.txt

# One SPDX SBOM per archive (<archive>.sbom.json); needs syft on PATH.
sboms:
  - artifacts: archive

# Keyless: cosign gets its identity from the GitHub Actions OIDC token, so this
# only works in release.yml (task release:binaries passes --skip=sign). Signing
# checksums.txt covers every archive and SBOM it lists.
signs:
  - cmd: cosign
    artifacts: checksum
    signature: "${artifact}.sigstore.json"
    args: ["sign-blob", "--yes", "--bundle=${signature}", "${artifact}"]
    output: true

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  disable: true

release:
  mode: keep-existing
  prerelease: auto
```

Facts that follow from this file:

- Six targets: linux, darwin and windows, each on amd64 and arm64. Assets of a release: six archives named `stuarty_X.Y.Z_<os>_<arch>.tar.gz` (`.zip` for Windows), six `<archive>.sbom.json`, `checksums.txt`, `checksums.txt.sigstore.json`, `stuarty.intoto.jsonl`: 15 files, the same count as vukem's `v0.43.0`.
- The archives contain no FFmpeg (NFR-041, under 60 MB). The binary finds FFmpeg on the path or at `STUARTY_FFMPEG` (RF-204). `THIRD_PARTY_NOTICES.md` still lists FFmpeg, stating that it is used when present and shipped only in the image.
- The binary must be built after `bun run build` because `web/` embeds `web/resources/public` with `go:embed` (the current directive is `//go:embed views all:public lang` in `web/resources/resources.go`; SDD-012 owns the final one). A build without the web build does **not** fail to compile: `public/robots.txt` is tracked, so the directive matches and the binary silently ships without the UI. That is why the hook `scripts/assert-web-build.sh` runs after the web build and before the compile: it fails when `find web/resources/public/build -type f -name "*.js"` finds nothing. The Dockerfile asserts the same after its web build stage (section 11.3).

What P1-REL-008 built (the tooling; the release itself is cut by the orchestrator with the author), where it differs from sections 5, 8, 9.3 and 10:

- **`.config/cliff.toml`** is the file of section 5.2 unchanged. Checked with git-cliff 2.14.2: `git cliff --unreleased` on `main` files the existing commits under `Added`, `Changed` and `Fixed` only.
- **`internal/tools/release`** has the four commands of section 8 plus the flag `-skip-remote-checks`, for a dry run in a clone: it skips the checks that need `origin` (HEAD equal to `origin/main`, green runs for that commit, no open `ci-failure` issue, no remote tag) and `tag` then does not push. Beyond section 8, `prepare` also refuses a version that is not newer than the previous tag, a branch that is not `release/<version>`, and a commit with no workflow run; `tag` refuses to tag when the changelog section is empty. The footer links of older releases are kept as written (the Studio links keep `vukem-studio`); only `[Unreleased]` and the new version's link are written. The tool has no `gh release create`: `release.yml` publishes the notes.
- **`CHANGELOG.md` still has the Studio sections `0.2.0`, `0.3.0`, `0.3.1` and `0.4.0`** although the tags and releases were folded into `v0.1.0` (D-026). `prepare v0.2.0` refuses ("already has a section") until those sections are folded into `0.1.0` or renumbered in the release pull request: a decision for the author, not made by the tool.
- **`.config/goreleaser.yaml`:** `before.hooks` entries are plain commands (GoReleaser has no `dir` key there), so the web build runs as `bash -c "cd web && bun install --frozen-lockfile && bun run build"` followed by `scripts/assert-web-build.sh`, which this task adds. The archive `files` list is `LICENSE.md`, `README.md` and `CHANGELOG.md`: `THIRD_PARTY_NOTICES.md`, `licenses/` and the agent documents do not exist yet, and GoReleaser fails on a pattern that matches nothing; the tasks that create them add the lines. A local `goreleaser release --snapshot --clean --skip=sign` (GoReleaser v2.18.2, syft v1.54.1, Windows 11) builds the six archives and six SBOMs in about two minutes; each archive is 7.4 to 8.1 MB, so NFR-041 (60 MB without FFmpeg) holds with a wide margin. `task release:snapshot` skips the SBOMs as well, so it needs no syft.
- **`release.yml`** is section 9.3 without the step "Notices are current" and without the `image` job, which join with RF-215 and P1-REL-009; the installers of P1-REL-010 raise the asset count it asserts to 21 (section 10.4). cosign v3.1.3 is installed by the workflow only; the keyless signature and `cosign verify-blob` run for the first time on the first tag.

### 10.4 Installers of the local server (D-029, P1-REL-010)

The release of the Prototype attaches installers next to the archives. Each installs `stuarty` and a shortcut that runs `stuarty open`; none carries paid code signing, and none deletes the workspace.

| System | File | Tool (pinned) | Built by |
| --- | --- | --- | --- |
| Windows 11 | `stuarty-X-Y-Z.exe` (0.28.0 is `stuarty-0-28-0.exe`) | Inno Setup 7.1.1, fixed URL and SHA-256 in `.config/installers/tools.env` | `installers.yml`, on `windows-latest` |
| Debian and Ubuntu | `stuarty_X.Y.Z_linux_amd64.deb`, `..._arm64.deb` | nfpm, the library inside GoReleaser v2.18.2 (the `nfpms` section of `.config/goreleaser.yaml`) | GoReleaser, in `release.yml` |
| Any Linux | `stuarty-X-Y-Z.AppImage` (0.28.0 is `stuarty-0-28-0.AppImage`) | appimagetool 1.9.1, fixed URL and SHA-256 in `tools.env` | `installers.yml`, on `ubuntu-latest` |
| macOS | `stuarty_X.Y.Z_darwin_amd64.pkg`, `..._arm64.pkg` | `pkgbuild`, `sips` and `iconutil` of macOS | `installers.yml`, on `macos-latest` |

Inno Setup and appimagetool are downloaded by `scripts/fetch-installer-tool.sh`, which verifies the SHA-256 and stops with both digests on a mismatch (the same rule as the FFmpeg pin of section 11). Bump them with a pull request that changes `tools.env` and passes `installers.yml`. There is no Windows ARM or Linux ARM AppImage: Windows 11 on ARM runs the amd64 installer, and the arm64 Linux user has the `.deb` and the archive.

**What each installer does** (`.config/installers/`):

- **Windows.** Per user by default (no administrator prompt, program in `%LOCALAPPDATA%\Programs\Stuarty`; the first page lets the user choose an all-users install). A task, on by default, adds the folder to the PATH. A Start menu shortcut runs `stuarty.exe open` minimized; a desktop shortcut is optional. The icon is `.config/installers/assets/stuarty.ico`. Installing over a running server stops it first (`taskkill`), and the uninstaller stops it too, removes the program, the shortcuts and the PATH entry, and leaves `%USERPROFILE%\.stuarty` (or `STUARTY_HOME`).
- **Debian and Ubuntu.** `/usr/bin/stuarty`, `/usr/share/applications/stuarty.desktop` (`Exec=stuarty open`, `Terminal=false`) and the icon. `apt remove` leaves the workspace in the home folder, which a package never touches.
- **AppImage.** Run with no argument it runs `stuarty open`; with arguments it is the program. It needs no installation. The server it starts outlives the AppImage process (the smoke test checks it).
- **macOS.** `/usr/local/bin/stuarty`, `/Applications/Stuarty.app` (a launcher whose executable runs `stuarty open`) and `/usr/local/share/stuarty/uninstall.sh`, which removes all three and keeps the workspace (run it with `sudo`). The package is built with `pkgbuild` only, with no `productbuild` wrapper, because there is no signature, license page or choice to add.

**`stuarty open`** (`web/cmd/app/open.go`, SDD-003 section 5.4, SDD-007 section 3.4). It reads the address (`STUARTY_ADDR`, else `HOST` and `PORT`, else `127.0.0.1:7878`) and refuses a non-loopback host. If `GET /up` does not answer, it starts `stuarty serve` detached, with its output in `<workspace>/logs/server.log`, and waits up to 30 s; then it opens the default browser with argv (`rundll32.exe url.dll,FileProtocolHandler`, `/usr/bin/open`, `xdg-open`), never through a shell. `STUARTY_NO_BROWSER=1` prints the address instead, which the smoke tests use. Two deviations from the specifications: it opens the plain address and does not ask for the launch code of SDD-007 section 3.4 (a code asked for with the bearer token alone gives a session without the browser-only privileges, section 3.4 item 7), and, temporary, it runs before the application is built and gives the server it starts a key kept in `<workspace>/app.key`, because every command of the application needs `APP_KEY` today (`vukem-feedback.md` VF-030, task P1-SRV-001).

**`stuarty-render` (TASK-0222, SDD-013 section 4.9, SDD-007 T-37).** The GPU render child process of the export is built by `cargo build --locked --release --bin stuarty-render` in `renderer/`, or by `bash scripts/build-renderer.sh host`, which copies it to `renderer/dist/` with its SHA-256. SDD-013 requires it in every archive and installer that carries FFmpeg, next to the server executable or in `libexec/stuarty/` (the server looks next to the executable first, as for FFmpeg). Today none does: the archives and installers carry no FFmpeg (NFR-041), so none carries `stuarty-render` and the export draws on the CPU path unless the user places both next to the executable. A user who wants the GPU path builds it with `bash scripts/build-renderer.sh host` (it needs the Rust toolchain of `rust-toolchain.toml`), copies `renderer/dist/stuarty-render` (`.exe` on Windows) next to the server executable or into `../libexec/stuarty/`, and starts the server with `STUARTY_EXPORT_RENDERER=gpu`; the site says so in the export guide (`site/content/docs/en/guide/exporting.mdx`), and a missing helper falls back to the CPU path with no error. The image carries FFmpeg and does not carry `stuarty-render`: a container has no graphics device or driver, and a runner inside `bwrap` never starts it (SDD-007 6.10), so it would only add a Rust build stage and a binary that always falls back. The day an archive or installer starts to carry FFmpeg, it carries `stuarty-render` built for its platform on a native runner, its digest goes into `checksums.txt` like the other files (signed by the same cosign step), the macOS package signs it with the same identity as the server, and `THIRD_PARTY_NOTICES.md` already lists its crates (section 12.2).

**In the release.** `installers.yml` is a reusable workflow that `release.yml` calls with the tag. It builds the binaries with `goreleaser build`, then builds each installer from those binaries, installs it on its own system, runs its shortcut command and checks that the server answers `GET /up` and that the uninstall keeps a workspace it was given. The same workflow runs for a pull request that touches `.config/installers/`, the GoReleaser file or `web/cmd/app/`, and by hand, with a snapshot version. After GoReleaser has built the archives and the `.deb` files, `release.yml`: (1) `scripts/installer-binaries.sh check` proves that the binaries in the installers are the ones of the release (GoReleaser builds them twice; they are reproducible at the same commit); (2) appends the SHA-256 of the three kinds of installer to `checksums.txt`; (3) signs it again with cosign keyless; (4) uploads the installers and the new `checksums.txt` and bundle with `--clobber`. The release then has 21 assets: the 15 of section 10 plus two `.deb`, the Windows installer, the AppImage and two macOS packages. `cosign verify-blob` on `checksums.txt` covers every one of them.

**SmartScreen and Gatekeeper.** The installers are not signed with a paid certificate. Windows SmartScreen shows "Windows protected your PC" for the `.exe`: choose "More info" then "Run anyway". macOS Gatekeeper says the package cannot be opened because Apple cannot check it: control-click it and choose Open, or allow it in System Settings, Privacy & Security. To check a download first, verify `checksums.txt` with cosign (SECURITY.md) and compare the file's SHA-256. `README.md` and the site repeat this (P1-REL-007, P1-DOC-001).

### 10.5 Desktop updates (P3-DSK-005, RF-213, ADR-0023)

The packaged desktop application can replace itself with a newer release. This is the only update channel of the product: the server binary, the archives and the container image never check for a version (SDD-007 section 10). The client is the package `desktop/update`, which uses the standard library only. The shell implements the launcher and the install step; the release workflow produces the feed (P3-REL-004).

**Decisions (D-033, 2026-11-15).** The feed host is GitHub Pages of the product repository (`https://video.vukem.dev/updates/feed.json` unless the repository variable `SITE_URL` says otherwise; until v0.20.0 it was `https://alexgalhardo.github.io/stuarty/updates/feed.json`), and the BETA has no paid code signing, so the only key is the one below. The feed URL and the public key are still build-time configuration: a build without them (a local one, a pull request build, or a pre-release made while the repository variable `UPDATE_FEED_PUBLIC_KEY` is unset) has the placeholder (`https://updates.example.invalid/stuarty/feed.json`, and no key), the client refuses to run while either is a placeholder (`ErrNotConfigured`), and the tests use a key that lives in `desktop/update/testdata/` only. Section 10.6 says how the release builds, signs and publishes all of it.

**Note: the host change of v0.21.0 (TASK-0263).** The feed URL is baked into each build, and the client follows no redirect, so a desktop application up to v0.20.0 keeps asking `https://alexgalhardo.github.io/stuarty/updates/feed.json`. Once the custom domain is configured, GitHub Pages answers the old address with a redirect (301) to `video.vukem.dev`, which the client refuses, so those applications stop finding updates. One manual install of v0.21.0 or later fixes it; the user documentation says so (the page "The desktop application", section Updates). No code change can reach those applications.

**The key.** One Ed25519 key pair signs the feed and every artifact it lists. The private key is a secret of the release workflow: never in the repository, never on a developer machine. The public key is embedded in the desktop binary at build time, together with the feed URL:

```text
-X github.com/AlexGalhardo/stuarty/desktop/update.FeedURL=https://<host>/<path>/feed.json
-X github.com/AlexGalhardo/stuarty/desktop/update.PublicKeyHex=<64 hex characters>
```

Rotating the key needs a new desktop release signed by the old key; there is no remote key delivery. The feed and the artifact are signed over differently labelled messages (domain separation), so a signature of one is never valid for the other.

**The feed** is one JSON document, `feed.json`, served over HTTPS from the single feed host:

```json
{
  "payload": "<base64 (standard, padded) of the payload JSON below>",
  "signature": "<base64 Ed25519 signature>"
}
```

The signature is over the bytes `vukem-video-update-feed-v1\n` followed by the decoded payload bytes. Signing the bytes, not a re-serialized object, means no canonical JSON is needed. The payload:

```json
{
  "schema": 1,
  "version": "0.15.0",
  "published_at": "2026-11-16T12:00:00Z",
  "expires": "2027-05-15T12:00:00Z",
  "notes": "Optional plain text summary of the release.",
  "platforms": {
    "windows-amd64": {
      "url": "https://<feed host>/stuarty/0.15.0/stuarty-desktop-0-15-0.exe",
      "sha256": "<64 lowercase hex>",
      "size": 12345678,
      "signature": "<base64 Ed25519 signature>"
    }
  }
}
```

- `published_at` and `expires` are RFC 3339 instants in UTC inside the signed payload (PLAN-019, T-35). The tool signs `expires` 180 days after `published_at` by default (`-valid-for`, at most 400 days). They make a replayed or frozen feed detectable: a replayed older feed is already refused as not newer, but an attacker who keeps serving the current feed would hide a security update for ever, and now cannot do it past `expires`. They stay in `schema` 1 on purpose: a client already installed reads the payload with a tolerant JSON decoder and ignores both, so installed copies keep updating and the first feed with the fields does not brick them.
- `notes` is optional, plain text, at most 4096 bytes; a client that does not know it ignores it and one that finds it absent shows no notes. It is inside the signed payload, so it cannot be changed without the key.
- `version` is `MAJOR.MINOR.PATCH`, a stable release; a pre-release suffix is never in the feed (P3-REL-004).
- The platform keys are `<GOOS>-<GOARCH>` (`windows-amd64`, `darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`).
- `url` is `https` on the feed host; any other host is refused. The installers are served by the same host as the feed (GitHub Pages, `<site>/updates/<file>`), never by the release page: a release asset URL answers a redirect to another host, and the client follows no redirect and talks to one host only. The client is therefore unchanged (section 10.6).
- The artifact signature is over `vukem-video-update-artifact-v1\n<version>\n<platform>\n<sha256>` (UTF-8, `\n` separators, no trailing newline). It binds the digest to the version and the platform, so a valid older artifact, or the artifact of another platform, cannot be offered as the new one.
- `size` is positive and at most 1 GiB, the ceiling of the client.
- `notes` (optional, TASK-0260) is a short plain text summary of the release: the bullets of the release notes (headings dropped, links reduced to their text, Markdown emphasis removed), at most 10 lines and 1200 bytes. It is part of the signed payload, so a launcher can show it in the update dialog knowing that it is the text of the release. `updatefeed sign -notes-file <file>` makes it from the release body (`gh release view --json body`); with no file the field is absent. Treat it as untrusted text in the UI (plain text, never HTML).
- `launchers` (optional, TASK-0260) lists the artifacts the desktop launcher replaces itself with, keyed by platform like `platforms` and with the same entry fields: `stuarty-desktop_<version>_<goos>_<goarch>.AppImage` (Linux), `.app.zip` (macOS, a zipped `Stuarty.app`) and `_portable.zip` (Windows). The artifact signature of a launcher entry is over the same message with the platform `launcher:<goos>-<goarch>` (for example `launcher:linux-amd64`), so a signature made for an installer is never valid for a launcher artifact and the other way round. `updatefeed sign` puts every file of the release that matches these names in `launchers`; the field is absent while the release has none. Both fields are additive: the schema stays 1 and a client that does not know them ignores them. `updatefeed verify` and `update-feed.sh pages` treat the launcher files like the installers: each digest must be in `checksums.txt`, each signature must verify, and each file is copied to Pages after its digest is checked.

**Client rules.**

1. **Consent first, then a schedule (D-039).** The setting `update.check` has three states: absent (the person was not asked yet), `true` and `false`. While it is absent the client makes no connection at all: the launcher first shows the question of the launcher flow below. With `false` the client makes no connection, at start-up or later; a check happens only when the person chooses "Check for updates" (one check, for that action). With `true` it checks once at every start and then every 24 hours while the application is open. The agent surfaces (MCP, CLI, HTTP API) cannot trigger a check or an install: it is a desktop menu action only (SDD-007 T-10).
2. **One GET to one host.** The check is one HTTPS `GET` of the feed URL, with the client built by `outbound.DesktopUpdate` (purpose `desktop_update`, SDD-007 section 10.1 path 6): `https` only, TLS verified, no proxy, no redirect followed, the single host of the feed URL for the feed and for the artifact, a response cap of 1 MiB for the feed. The request carries no cookie, identifier or body; the host learns the connection and the `User-Agent` `stuarty-desktop/<version>` only.
3. **Verify the feed first.** The envelope is verified against the embedded key before its content is parsed or used. A bad signature, bad base64, an unknown `schema`, a missing platform entry or a malformed field ends the check with an error and no download. **Freshness** (PLAN-019): the signed payload must carry `published_at` and `expires`, with `expires` after `published_at` by at most 400 days and `published_at` not more than 10 minutes in the future of the local clock. A feed without them is `ErrBadFeed` (the **transition rule**: the client that requires the fields ships in the same release as the tool that writes them, and the feed holds only the latest release, so no feed that client meets lacks them, while an old signed feed replayed by an attacker does; clients installed before the change ignore the fields and are unaffected). A feed whose `expires` is not after the local clock is `ErrStaleFeed`: the check fails with the message that the update server has not been refreshed, the shell shows it as the reason of the failed check, and nothing is offered or applied. The release workflow signs a new feed at every stable release, so a feed older than 180 days means no release for that long; the maintainers refresh it by cutting one. The client keeps no memory of the last `published_at`, so a replay of the same version stays harmless only because of the strictly-newer rule.
4. **Newer only.** The offered `version` must be strictly greater than the running version in SemVer order, else `ErrNotNewer`: an equal version is "up to date", an older one is a rollback and is refused. A running version that is not `MAJOR.MINOR.PATCH` (a development build) does not check.
5. **Download, then verify, then install.** After the person accepts the offer, the artifact is downloaded into a private staging directory (mode 0700, outside the workspace) with a hard cap at the declared `size`: fewer bytes (an interrupted download), more bytes or a failed read is an error. The SHA-256 is computed while writing and must equal the digest of the feed; then the artifact signature is verified with the embedded key. Only when both pass is the staged file made read-only and passed to the installer. On any failure the staged file is deleted and the installer is never called.
6. **The install step is an interface**, `Installer.Install(ctx, Artifact) error`, which the shell implements with the platform mechanism (run the installer, restart). The client never executes an artifact itself, and a tampered artifact never reaches the interface (the tests assert it).
7. **A failed install is safe.** If the installer returns an error the running version is untouched and usable; the client returns `ErrInstallFailed` with the reason, and the shell shows it. The update client never reads or writes the workspace (`STUARTY_HOME`), and the installers of section 10.4 never delete it.

**The shell wiring** (`desktop/shell`, `desktop/shell/app`). The setting `update.check` is stored in `desktop.json` of the user config directory (`os.UserConfigDir()/stuarty/`), never in the workspace. A missing file, a file without the key and a damaged file all mean "not asked yet": the question is shown and nothing connects until it is answered (a damaged file never turns the network on). The Help menu has "Check for updates" (one `CheckNow`, then a dialog: up to date, a newer version with an install question, or why the check failed) and the checkbox "Check for updates automatically", which takes effect at once (it starts or stops the schedule). A build without the feed URL and key has no client, never asks the question, and the menu shows that message, with the release page, instead of failing. The texts are in the catalogue of `desktop/shell/strings.go` (en and pt-BR, following the system language).

**The launcher flow (EPIC-55, D-039).** `shell.Launcher` joins the client, the settings and the native dialogs; every dependency (clock, client, dialogs, installer, process starter) is injected, so the flow is tested without a network or a window.

1. **First start.** With the setting absent the launcher shows the native question "Check for updates automatically?" with Yes preselected and No. The answer is saved to `update.check` before anything else happens. Closing the dialog is No for this start and is not saved, so the question is asked again at the next start. Nothing connects before the answer.
2. **Schedule (answer Yes or the setting `true`).** One check at once, then one every 24 hours while the application is open. The machine being offline is not probed separately (a probe would be one more connection): a check that fails on the network is silent, with no dialog, and is retried after 1 hour (24 hours after a check that worked). Turning the setting off in the Help menu stops the schedule at once, and the setting is saved.
3. **A newer stable version.** The launcher shows the version, the release notes and the buttons "Install and restart" and "Not now". The notes are the optional string `notes` of the signed feed payload (plain text, at most 4096 bytes, cut on a character boundary; the feed host never sends markup that is shown): absent means nothing is shown. After "Not now" the same version is not offered again by the schedule until the next start; the menu action still offers it. A version whose installation was rolled back (below) is not offered by the schedule either, only by the menu action.
4. **Download.** The client downloads in the background into the staging directory with the rules of items 5 and 6, and reports `done` and `total` bytes to a small progress window (a bar and a percentage); the editor stays usable. The verification (size, SHA-256, signature) is unchanged and runs before anything is installed.
5. **Install and restart.** The installer of the system takes the verified file. Windows starts the setup program and quits (unchanged). macOS and Linux replace the application in place and restart it, as follows.
6. **Install on macOS and Linux.** The client prefers the entry of `launchers` for its platform (macOS and Linux only) and verifies its signature with the platform `launcher:<goos>-<goarch>`; with no launcher entry (Windows, whose setup is in `platforms`, or a feed that has no `launchers`) it uses `platforms` with the plain platform. A signature made for an installer therefore never verifies as a launcher artifact, and the other way round. The artifact of a platform is: `darwin-*` a zip that holds one `Stuarty.app` bundle; `linux-*` the AppImage (the `.deb` is not updated by the application, because it needs root: the person is sent to the release page). The installer works only when the running application is itself an installed bundle (`.../X.app/Contents/MacOS/<exe>`) or an AppImage (`$APPIMAGE`), and its parent directory is writable; else it returns `ErrInstallUnsupported`, which points to the release page. The steps: (a) take the lock `update/swap.lock` (one swap at a time); (b) extract the zip, or copy the AppImage, into a hidden staging name next to the target (same directory, so the final rename is atomic), refusing names that leave the staging directory, absolute or escaping symlinks, any entry below a symlink, special files, and archives of more than 100000 entries or 2 GiB; (c) rename the target to `<target>.bak` (the backup) and the staged copy to the target; if the second rename fails the first is undone; (d) write `update/pending.json` (new version, target, backup, executable); (e) start the watchdog, which is the executable of the backup run as `stuarty-desktop --update-watch <pending.json>` (the code that is known to work), detached, and quit.
7. **Health check and rollback.** The watchdog starts the new application and waits up to 60 seconds for the file `update/confirmed-<version>`, which the new application writes (`shell.ConfirmStart`) once its server is up and its window is created. When it appears, the watchdog deletes the backup and `pending.json`. When the new application exits first, cannot be started, or the time runs out, the watchdog deletes the new target, renames the backup back, writes `update/failed.json` with the version, starts the old application again and exits; the old application tells the person at its next start that the update was undone, once. The workspace is never touched.

   **A watchdog that dies.** While it works the watchdog rewrites `update/watch.lock` at every poll; it counts as alive when that file or `pending.json` is less than 15 s old. At every start, before anything else, the application calls `shell.RecoverPending`: when `pending.json` is left over, its version differs from the running one, there is no `confirmed-<version>` and no live watchdog, it takes `swap.lock` (a second start at the same time gives up, so the recovery runs once), deletes the staging directory, puts `<target>.bak` back as the watchdog does, deletes `pending.json` and writes `failed.json`, so the person is told once. With `confirmed-<version>` present it only deletes the backup and the markers; with no backup it only drops the marker and never deletes the target. When the running copy is the new version, `shell.ConfirmStart` finishes the update (deletes the backup, `pending.json` and the confirmation) if no watchdog is alive.
8. **What the client never does.** It never installs without the person pressing "Install and restart" (the download is background, the install is not), and never follows a redirect to another host.

**Out of scope here.** Delta updates, release channels, silent installs, and an update of the server binary or the container image. The generation of the feed is section 10.6.

### 10.6 Desktop installers and the feed in the release (P3-REL-003, P3-REL-004, D-033)

**No paid signing.** The BETA desktop installers carry no Apple Developer ID, no notarization and no Windows certificate (D-033), so no workflow step signs or notarizes them and no secret for it exists. Their integrity is the SHA-256 in `checksums.txt`, which cosign signs keyless, and a provenance attestation of each file. Windows SmartScreen and macOS Gatekeeper warn at first open, as for the installers of section 10.4; `site/content/docs/en/self-hosting/verify-a-release.mdx` and the install page say how to open them and how to verify the download. Paid signing is decided again before 1.0.

**The installers** are built by `desktop.yml`, a reusable workflow that `release.yml` calls after `installers.yml` (it reuses its `binaries` artifact, the server binaries of the release) and that also runs for a pull request that touches `desktop/**`. The application is `desktop/shell/app` (Wails v3), built with `-tags purego` and no cgo on Linux and macOS, so one runner builds both macOS architectures; the packaging is in `desktop/packaging/`:

| System | File | Tool | Contents |
| --- | --- | --- | --- |
| Windows 11 | `stuarty-desktop-X-Y-Z.exe` (0.28.0 is `stuarty-desktop-0-28-0.exe`) | Inno Setup (`tools.env`) | `stuarty-desktop.exe` and `stuarty.exe` in one folder, a Start menu shortcut; the uninstall keeps the workspace |
| Debian and Ubuntu | `stuarty-desktop_X.Y.Z_linux_amd64.deb` | `dpkg-deb` | both programs in `/usr/lib/stuarty-desktop/`, a link in `/usr/bin`, a desktop entry; depends on `libgtk-3-0` and `libwebkit2gtk-4.1-0`; owns no `/usr/bin/stuarty`, so it installs next to the server package |
| macOS | `stuarty-desktop-X-Y-Z-amd64.dmg`, `stuarty-desktop-X-Y-Z-arm64.dmg` (and the `.app.zip` launcher artifacts, below) | `hdiutil` | `Stuarty.app` with the server next to the application and a link to `/Applications`; first open: control-click and Open, or `xattr -d com.apple.quarantine` |

The application finds the server next to its own executable (`desktop/shell`). Each installer is installed or mounted by a smoke test that checks the files, runs `stuarty version` and, for Windows, the uninstall; none starts the window, because a runner has no screen (the window itself is the manual run of P3-DSK-003). The release attaches the four installers and one SBOM of the application (`stuarty-desktop_X.Y.Z.sbom.json`, syft over `desktop/shell/app`) to the same release; `release.yml` lists them in `checksums.txt` and signs it again, and the attestation of `checksums.txt` covers them. A release has 29 assets from v0.18.0 (26 before, without the launcher artifacts below), and a stable one has 30 with `feed.json`.

**The launcher artifacts** (EPIC-55, D-039, TASK-0269) are the files the launcher of section 10.5 replaces itself with. `desktop.yml` builds them next to the installers, with the names the feed tool `updatefeed` recognizes and lists under `launchers` (section 10.5, `<goos>-<goarch>` keys): the AppImage `stuarty-desktop-X-Y-Z.AppImage` (job `linux`, `desktop/packaging/linux/build-appimage.sh`, AppDir with `stuarty-desktop` and `stuarty` in `usr/bin`, built with the `appimagetool` pinned by version and SHA-256 in `.config/installers/tools.env` and verified by `scripts/fetch-installer-tool.sh` before it runs; the job only extracts it, because a runner has no FUSE or screen) and `stuarty-desktop_X.Y.Z_darwin_<amd64|arm64>.app.zip` (job `macos`, `build-dmg.sh` runs `ditto -c -k --keepParent`; the job checks that the zip holds exactly one top-level `Stuarty.app` with both programs). Windows needs no extra file: its launcher artifact is the setup of the table above, listed under `platforms`; the `_portable.zip` name of the feed tool is not produced. The `.dmg` and `.deb` stay, for a person installing by hand. The feed `notes` is the optional top-level string the client reads (4096 bytes at most on the client, 1200 on the feed tool).

**The feed** (`internal/tools/updatefeed`, `scripts/update-feed.sh`, job `feed` of `release.yml`). For a stable tag only (`vX.Y.Z`; a suffix such as `-rc.1` never reaches the feed, and the tool refuses it), the job signs `feed.json` over the desktop installers of the run with the Ed25519 seed of the repository secret `UPDATE_FEED_SIGNING_KEY` (64 hex characters), proves that the signature and each artifact signature verify and that every digest equals `checksums.txt`, and attaches `feed.json` to the release. The public key is the repository variable `UPDATE_FEED_PUBLIC_KEY` (not a secret): `desktop.yml` builds it, and the feed URL, into the application with `-X`. The job `preflight` of `release.yml` runs first for a stable tag and fails within minutes, naming the missing secret or variable, and when the variable is not the public half of the secret. A pre-release or a pull request build with the variable unset has no client, and its update menu says so.

**Continuity across the rename (ADR-0044).** An installed Vukem Video must find Stuarty as its update, so what its client checks does not change. The feed stays at the same address and under the same signing key and labels (`vukem-video-update-feed-v1`, `vukem-video-update-artifact-v1`; SDD-000 section 13.4); the file names of the artifacts are not checked by the client, which reads their URLs from the feed. The macOS launcher of Vukem Video installs a zip that holds exactly one `.app` and starts it again from the path of the program it ran, `Contents/MacOS/vukem-video-desktop`, so `build-dmg.sh` adds a one-line shim with that name beside `stuarty-desktop`, and `desktop.yml` and the macOS smoke test check for it. The Windows installers keep their `AppId`, so the setup upgrades the installed folder in place; the macOS package identifier and bundle identifiers keep their names; the Linux `.deb` packages `replace` and `conflict` with `vukem-video` and `vukem-video-desktop`. The bundle that an old launcher updates keeps the folder name `Vukem Video.app` until the person installs the disk image again. Not tested here: the update itself from a real installed Vukem Video on each system.

**Where the feed and the installers are served.** The feed client allows one host and follows no redirect (section 10.5), and a GitHub release asset URL redirects to `objects.githubusercontent.com`, so the release page cannot serve the installers. The decision is to keep the client as it is and serve both from GitHub Pages: `site.yml` runs `scripts/update-feed.sh pages`, which copies `feed.json` and the installers it lists from the latest stable release into `<site>/updates/` after checking each against the digest of the feed, then deploys the site. `release.yml` starts `site.yml` on `main` after it attaches the feed (job `pages`), and every later deploy of the docs repeats the copy, so a docs change never removes them. Only the latest version is on Pages, which is all the feed offers; a person on an older version finds it through the feed, and the older installers stay on the release page. The Pages copy of the four installers is about 200 MB, inside the 1 GB limit of a Pages site.

**Verification.** `scripts/release-verify.sh vX.Y.Z` downloads the desktop installers and runs `gh attestation verify` for each, and for a stable release fetches `feed.json` and runs `scripts/update-feed.sh verify` (the signature, every artifact signature and each digest against `checksums.txt`), then compares the release feed with the one GitHub Pages serves (`--no-pages` skips that, because Pages is deployed minutes after the release). `scripts/desktop-verify.sh` (to be run by a person who downloaded an installer) checks the SHA-256 against `checksums.txt`, the cosign signature of `checksums.txt` and the attestation; it uses neither `codesign` nor `signtool`.

## 11. The container image

### 11.1 Decisions

- The Go binary serves HTTP and the UI itself (RF-200), so **no Caddy or any other web server is needed inside the image**. The author's Caddy rule is for images that serve static files or proxy; this one does neither. A reverse proxy for TLS is the operator's concern and is documented in the self-hosting guide (SDD-010).
- Base images, all with fixed version tags: `oven/bun:1.4.4-alpine` (asset build stage only, same as the Studio's `Dockerfile.app`; it never reaches the runtime image, so the Alpine in this stage is not a runtime base), `golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d` (build, the Debian image vukem pins by digest, `.config/docker/vukem.Dockerfile:11`; it has `git`, which the version stamp needs), `debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` (runtime; tag and digest read from Docker Hub on 2026-11-14, `REL-DOCKER` re-reads them and records the values it pins).
- The runtime base is a fixed-tag `debian:13-slim` pinned by digest. This reverses the first draft, which chose Alpine on the claim that the BtbN builds are static and run on musl: the BtbN Linux builds are linked against glibc, so they do not run on Alpine. Debian is also what vukem uses (`.config/docker/vukem.Dockerfile:9-10`). The first subtask of `REL-DOCKER` is a smoke test that runs `ffmpeg -version` and the reference export inside the image on the Debian runtime, before any other work on the image; if it fails the task stops and reports. No Alpine image is the runtime anywhere in this document.
- FFmpeg is the BtbN `lgpl` variant on a release branch, downloaded by exact URL and verified by SHA-256 (ADR-0011 item 2). Its single source of truth is `.config/ffmpeg.env`, sourced by the Dockerfile and by `scripts/install-ffmpeg.sh` (CI). The Dockerfile copies the file into the build and fails if a value is missing or a hash does not match.
- The own LGPL FFmpeg build (D-017, P3-REL-001) has its inputs in `.config/ffmpeg-build/` and is made by `scripts/ffmpeg-build.sh`. It has no `--enable-libopenh264` and links no OpenH264. Decision of the author (2026-11-15), the Fedora model: one patch, `.config/ffmpeg-build/patches/0001-libopenh264-dlopen.patch`, pinned by SHA-256 in `sources.env` like every source (the script refuses a different file and applies no other), makes the `libopenh264` encoder load Cisco's library at run time with `dlopen` or `LoadLibrary` from `VUKEM_VIDEO_OPENH264_DIR` and refuse a version other than the pinned 2.6.0. The build downloads only the four Cisco API headers (BSD-2-Clause) from `raw.githubusercontent.com`, pinned to the commit of the `v2.6.0` tag (`652bdb7719f30b52b08e506645a7322ff1b2cc6f`) with one SHA-256 per file in `sources.env` (an auto-generated tag archive is not byte-stable, so none is pinned). `scripts/ffmpeg-verify.sh` and the configure-line tests (`ffmpegconf`, `TestSecurity_T27_OwnBuild*`) accept exactly this and nothing broader: no GPL or nonfree flag, no `--enable-libopenh264`, no OpenH264 in the linker listing, and the encoder present. Reproducibility on macOS: the two rounds of `ffmpeg-9.0.2-2` differed only in the 16 bytes of the `LC_UUID` load command (and, on arm64, in the ad hoc code signature that hashes them); the linker derives that UUID from its inputs and it varied once the static LAME, libogg, libvorbis and libwebp archives were linked, so after the final link the script rewrites the UUID with `internal/tools/machouuid` (the first 16 bytes of the SHA-256 of the binary with the UUID and the code signature zeroed, marked as a version 5 UUID) and signs the binaries again ad hoc (`codesign --force -s -`). Dropping the load command with `-Wl,-no_uuid` is not possible: dyld refuses an arm64 executable without `LC_UUID`, so every configure test program aborts. SDD-011 section 8.4 specifies the loader.
- The product bundles and links no GPL component: no `x264`, `x265`, no `libfdk_aac`, no Debian `ffmpeg` package (research note `licensing` section 3), and the smoke test greps the configure line for `--enable-gpl` and `--enable-nonfree`. The base image is an operating system: its packages (bash, coreutils, dpkg and others) carry GPL licences and are not part of the product. They are listed with their licences: the Dockerfile writes the installed package list (`dpkg-query -W`) to `/usr/share/doc/stuarty/base-image-packages.txt`, the licence of each package is the `copyright` file Debian keeps in `/usr/share/doc/<package>/` of the slim image, and `THIRD_PARTY_NOTICES.md` has a group "Base image packages (Debian)" that says so. The architect rewords NFR-043 to "no GPL component is bundled or linked by the product; base image packages are listed with their licences". A base with no GPL userland is not required.
- Non-root user `app`, uid 10001 (the Studio's `Dockerfile.app` choice), a `/data` volume as the workspace (SDD-000 section 9).
- The version stamp (VV-046): the one version of the binary is `agentdocs.Version`; the build passes `-X github.com/AlexGalhardo/stuarty/web/app/agentdocs.Version=$VERSION` and fails when `stuarty version` does not print `stuarty $VERSION`. vukem stamps the tag from VCS and fails from a `git worktree`; the `-X` stamp does not need `.git`, so it works there.
- The build context must be clean, or Go stamps `+dirty` and the version check fails at the first real release. The `.dockerignore` therefore lists only untracked or generated paths (`**/node_modules`, `**/dist`, `.env`, `.env.*`, build outputs), as vukem's does, and never a tracked path such as `site/`, `docs/`, `spikes/` or `desktop/`. `image.yml` passes a dummy `VERSION` and a local dummy tag and runs the same version check on every pull request (section 9.5), so a violation is caught before a release.

### 11.2 `.config/ffmpeg.env`

```bash
# The one pin of the FFmpeg that Stuarty distributes (ADR-0011).
# Sourced by .config/docker/stuarty.Dockerfile and scripts/install-ffmpeg.sh.
# Values are recorded by task REL-FFMPEG-PIN from the release of
# https://github.com/BtbN/FFmpeg-Builds named by FFMPEG_RELEASE_TAG:
# the asset names from the release page, each SHA-256 computed locally with
# sha256sum after downloading the asset, the source commits from the release
# notes and from `ffmpeg -version`.
# Bump only through a pull request that re-runs the image smoke test.
FFMPEG_RELEASE_TAG="<autobuild tag recorded by REL-FFMPEG-PIN>"
FFMPEG_VERSION="<version line printed by ffmpeg -version, recorded by REL-FFMPEG-PIN, release branch n9.0>"
FFMPEG_BUILDS_COMMIT="<BtbN/FFmpeg-Builds commit of that autobuild, recorded by REL-FFMPEG-PIN>"
FFMPEG_SOURCE_URL="<URL of the FFmpeg source tree for FFMPEG_VERSION, recorded by REL-FFMPEG-PIN>"
FFMPEG_ASSET_LINUX_AMD64="<asset name ending in -linux64-lgpl-9.0.tar.xz>"
FFMPEG_ASSET_LINUX_ARM64="<asset name ending in -linuxarm64-lgpl-9.0.tar.xz>"
FFMPEG_ASSET_WINDOWS_AMD64="<asset name ending in -win64-lgpl-9.0.zip>"
FFMPEG_SHA256_LINUX_AMD64="<64 hex characters recorded by REL-FFMPEG-PIN>"
FFMPEG_SHA256_LINUX_ARM64="<64 hex characters recorded by REL-FFMPEG-PIN>"
FFMPEG_SHA256_WINDOWS_AMD64="<64 hex characters recorded by REL-FFMPEG-PIN>"
```

The download URL is `https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_RELEASE_TAG}/${FFMPEG_ASSET_...}`. The rolling `latest` tag of BtbN is never used: it moves. The `lgpl` variant and the release branch (`-lgpl-9.0`) are the ones the spike measured (`n9.0.2-57`, ADR-0011 evidence). The recorded pin is the monthly autobuild `autobuild-2026-10-31-12-59` (`n9.0.2-39-g7adfbe40dc-20261031`, same branch and patch release as the spike), not the daily one of the spike: BtbN deletes daily autobuilds after about two weeks and keeps one per month, so a daily tag would break the pin within weeks. `FFMPEG_BUILDS_COMMIT` is the commit the autobuild tag points to; `FFMPEG_SOURCE_URL` is the FFmpeg source archive of the full commit in `ffmpeg -version`. The build was checked on Windows against the encoders, filters and muxers of SDD-004 and SDD-011; `eq` and `boxblur` are absent and `engine/` never uses them (SDD-004 section 1, item 3). macOS has no BtbN build; CI on macOS uses the runner's own FFmpeg for tests only (section 13.3), which is never distributed.

**The own build block (D-017, TASK-0109 to TASK-0113).** The file also holds a second block, `FFMPEG_OWN_*`, for the LGPL build of `.github/workflows/ffmpeg-build.yml` (inputs in `.config/ffmpeg-build/`), and `FFMPEG_SOURCE` (`btbn` or `own`) says which block is in use. The Dockerfile (build argument `FFMPEG_SOURCE`, empty takes the file), `scripts/install-ffmpeg.sh` (environment variable `FFMPEG_SOURCE`, else the file) and `internal/tools/ffmpegpins` (which writes the `doctor --verify` digests from the block in use) all read that one variable, so a switch is one edit of one file. The own block has the tag (`FFMPEG_OWN_TAG`, the release `ffmpeg-<version>-<n>` of the repository, base URL `https://github.com/AlexGalhardo/stuarty/releases/download`), the version, the source URL and its SHA-256, and for each of `LINUX_AMD64`, `LINUX_ARM64`, `WINDOWS_AMD64`, `DARWIN_ARM64` and `DARWIN_AMD64` the asset name, the archive SHA-256 and the SHA-256 of the `ffmpeg` and `ffprobe` inside it. Nobody types those values: `bash scripts/ffmpeg-pin.sh print` downloads the assets of `FFMPEG_BUILD_TAG`, checks each archive against its `.sha256` file and prints the block from the bytes it hashed, and `bash scripts/ffmpeg-pin.sh check` fails when `.config/ffmpeg.env` differs from it. The switch is one pull request after a published build: dispatch the workflow with `publish` on, run `ffmpeg-pin.sh print`, paste the block, set `FFMPEG_SOURCE="own"`, run `go generate ./internal/ffmpegpins`, and update the FFmpeg and OpenH264 rows of `.config/third-party.json` (`task notices`). Signatures and attestations are checked with `bash scripts/ffmpeg-verify.sh signatures <dir>` (`cosign verify-blob` with the identity of `ffmpeg-build.yml` at the ref that built it, and `gh attestation verify`); set `GITHUB_REF` to that ref when it is not `refs/heads/main`.

### 11.3 `.config/docker/stuarty.Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1

# The Stuarty image: the stuarty binary (server, CLI, MCP server and the
# UI assets) and a pinned LGPL FFmpeg. Published to ghcr.io/alexgalhardo/stuarty
# and alexgalhardo/stuarty (Docker Hub) by release.yml; see
# docs/sdds/009-releases-and-ci-cd.md. Build from the repository root:
#
#   docker build -f .config/docker/stuarty.Dockerfile -t stuarty .
#
# No web server runs here: the Go binary serves HTTP. The product bundles and
# links no GPL component: .config/ffmpeg.env pins the BtbN lgpl build (ADR-0011);
# the packages of the Debian base are listed with their licences in the notices.
ARG BUN_IMAGE=oven/bun:1.4.4-alpine@sha256:4cf82f5686b196504fe119db3fcd9185b191f82ece82c40680e597228ec6070a
ARG GO_IMAGE=golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d
ARG RUNTIME_IMAGE=debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

# Web assets. Platform independent, so built once on the build machine.
FROM --platform=$BUILDPLATFORM ${BUN_IMAGE} AS assets
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
# The lockfile is a required build input: the image installs exactly the
# versions the repository tracks.
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build \
	&& find resources/public/build -type f -name "*.js" | grep -q .

# The static binary, cross compiled for the target platform.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=devel
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/web/resources/public/build web/resources/public/build
# The version `stuarty version` prints is agentdocs.Version, set with -X from
# VERSION. A release passes VERSION and the build fails if the printed version
# does not match; VERSION=devel keeps the version written in the source.
RUN stamp=""; \
	if [ "$VERSION" != devel ]; then stamp="-X github.com/AlexGalhardo/stuarty/web/app/agentdocs.Version=$VERSION"; fi \
	&& GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -buildvcs=false -trimpath -ldflags="-s -w $stamp" -o /out/stuarty ./web/cmd/app \
	&& if [ "$VERSION" != devel ] && [ "$TARGETARCH" = "$(go env GOHOSTARCH)" ]; then [ "$(/out/stuarty version)" = "stuarty $VERSION" ]; fi

# The pinned FFmpeg, downloaded by exact URL and verified by SHA-256.
FROM --platform=$BUILDPLATFORM ${RUNTIME_IMAGE} AS ffmpeg
ARG TARGETARCH
COPY .config/ffmpeg.env /tmp/ffmpeg.env
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
	&& rm -rf /var/lib/apt/lists/* \
	&& . /tmp/ffmpeg.env \
	&& case "$TARGETARCH" in \
		amd64) asset="$FFMPEG_ASSET_LINUX_AMD64"; sum="$FFMPEG_SHA256_LINUX_AMD64" ;; \
		arm64) asset="$FFMPEG_ASSET_LINUX_ARM64"; sum="$FFMPEG_SHA256_LINUX_ARM64" ;; \
		*) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
	esac \
	&& curl -fsSL -o /tmp/ffmpeg.tar.xz "https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_RELEASE_TAG}/${asset}" \
	&& echo "${sum}  /tmp/ffmpeg.tar.xz" | sha256sum -c - \
	&& mkdir -p /opt/ffmpeg \
	&& tar -xJf /tmp/ffmpeg.tar.xz -C /opt/ffmpeg --strip-components=1 \
	&& test -x /opt/ffmpeg/bin/ffmpeg && test -x /opt/ffmpeg/bin/ffprobe && test -s /opt/ffmpeg/LICENSE.txt

FROM ${RUNTIME_IMAGE}
ARG VERSION=devel
ARG REVISION=unknown

LABEL org.opencontainers.image.title="stuarty" \
	org.opencontainers.image.description="Stuarty: a minimal video editor operated by AI agents and by a human UI" \
	org.opencontainers.image.source="https://github.com/AlexGalhardo/stuarty" \
	org.opencontainers.image.url="https://github.com/AlexGalhardo/stuarty" \
	org.opencontainers.image.documentation="https://github.com/AlexGalhardo/stuarty#readme" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${REVISION}"

# curl is the health check client; tzdata and ca-certificates are the runtime
# data the binary needs. Debian slim keeps /usr/share/doc/<package>/copyright.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl tzdata \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --system --no-create-home --uid 10001 --user-group app \
	&& mkdir -p /data /usr/share/doc/stuarty/ffmpeg \
	&& chown app:app /data \
	&& dpkg-query -W > /usr/share/doc/stuarty/base-image-packages.txt

COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/LICENSE.txt /usr/share/doc/stuarty/ffmpeg/LICENSE.txt
COPY .config/ffmpeg.env /tmp/ffmpeg.env
# The licence obligations of the LGPL build (ADR-0011 item 2): the licence text,
# the configure line and where the corresponding source is.
RUN . /tmp/ffmpeg.env \
	&& /opt/ffmpeg/bin/ffmpeg -version | sed -n "s/^configuration: //p" > /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& test -s /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& ! grep -q -e "--enable-gpl" -e "--enable-nonfree" /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& printf '%s\n%s\n%s\n' \
		"FFmpeg ${FFMPEG_VERSION} (LGPL build) from https://github.com/BtbN/FFmpeg-Builds/releases/tag/${FFMPEG_RELEASE_TAG}" \
		"Build scripts: https://github.com/BtbN/FFmpeg-Builds/tree/${FFMPEG_BUILDS_COMMIT}" \
		"Corresponding FFmpeg source: ${FFMPEG_SOURCE_URL}" > /usr/share/doc/stuarty/ffmpeg/SOURCE.txt \
	&& rm /tmp/ffmpeg.env

COPY LICENSE.md THIRD_PARTY_NOTICES.md /usr/share/doc/stuarty/
COPY licenses/ /usr/share/doc/stuarty/licenses/
COPY --from=build /out/stuarty /usr/local/bin/stuarty

# The fonts (Inter, Montserrat, Anton, Bebas Neue, Playfair Display, JetBrains
# Mono, D-022) are embedded in the binary (renderer/fonts, go:embed), so
# the image needs no font files; their licences are in licenses/fonts/ above.
#
# A published port needs a non-loopback bind, and a non-loopback bind requires
# network mode (SDD-000 section 10), where login is mandatory. The first
# administrator is created as SDD-007 describes.
ENV STUARTY_HOME=/data \
	STUARTY_ADDR=0.0.0.0:7878 \
	STUARTY_MODE=network \
	STUARTY_FFMPEG=/opt/ffmpeg/bin/ffmpeg \
	STUARTY_FFPROBE=/opt/ffmpeg/bin/ffprobe \
	PATH=/opt/ffmpeg/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
VOLUME ["/data"]
USER app
WORKDIR /data
EXPOSE 7878
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
	CMD curl -fsS http://127.0.0.1:7878/up || exit 1
ENTRYPOINT ["stuarty"]
CMD ["serve"]
```

Notes on the file:

- Every `FROM` line resolves to a fixed tag (or a digest plus tag); there is no `latest`.
- In the `build` stage the version check runs only when the build machine and the target architecture are the same (the arm64 binary cannot run on the amd64 builder). The amd64 build is checked and the arm64 build uses the same source; this is a limitation of cross building without emulation and is accepted.
- The `ffmpeg` stage runs on the build platform because it only downloads and unpacks; the `RUN ffmpeg -version` in the last stage runs on the target platform (QEMU for arm64, `docker/setup-qemu-action`).
- `ENV` follows SDD-000 section 10 names exactly. `STUARTY_MODE=network` is forced by the bind address rule of that section, so a container started as `docker run -p 7878:7878 -v stuarty:/data ghcr.io/alexgalhardo/stuarty:X.Y.Z` always requires login. A user who wants local mode without login runs the binary on the host.
- `HEALTHCHECK` calls `GET /up` on `127.0.0.1:7878`, the vukem health probe that SDD-000 section 1 lists among the routes that are not service operations. `curl` is installed in the runtime stage only for this. SDD-007 §3.7 item 6 states the rule: `/up` needs no credential, and a request from a loopback peer is exempt from the `Host` check, which is what the health check needs in network mode.
- `VOLUME /data` plus `chown app:app /data` makes the workspace writable by uid 10001 on a named volume. A bind mount must be writable by uid 10001; the self-hosting guide says so.

### 11.4 `.config/docker/stuarty.Dockerfile.dockerignore`

BuildKit reads `<Dockerfile>.dockerignore` next to the Dockerfile (vukem's file of the same shape is `.config/docker/vukem.Dockerfile.dockerignore`). The `.git` directory is deliberately not ignored (section 11.1). Only untracked or generated paths are listed, every one of them git-ignored by the root `.gitignore`, so the tree in the build context is exactly the committed tree and the version stamp has no `+dirty`. A tracked path (`site/`, `docs/`, `spikes/`, `desktop/`) is never listed here: dropping it from the context makes git see it as deleted.

```text
**/node_modules
**/dist
web/resources/public/build
web/resources/public/hot
web/bootstrap/ssr
site/out
.env
.env.*
!.env.example
coverage*
*.log
```

### 11.5 What P1-REL-009 built, and where it differs from sections 9.5 and 11.1 to 11.4

The files are `.config/docker/stuarty.Dockerfile`, its `.dockerignore`, `scripts/image-smoke.sh`, `.github/workflows/image.yml` and the tasks `task image:build` and `task image:smoke`. The image was built and smoke tested on Windows 11 with Docker 29.9.2 (571 MB; amd64 only: the arm64 build runs in `release.yml`).

- **How to run it (D-029, local only).** `docker run --rm -p 127.0.0.1:7878:7878 -v stuarty:/data ghcr.io/alexgalhardo/stuarty:<version>`, then open `http://127.0.0.1:7878`. The server inside listens on every interface of the container (`HOST=0.0.0.0`), which is what a published port needs; the `127.0.0.1:` in `-p` is what keeps it reachable from the host only. Checked: `curl http://127.0.0.1:7878/up` answers `{"status":"ok"}` and the same port on the host's other interface (`172.25.80.1`) is refused. Do not publish it with `-p 7878:7878`.
- **Boot settings until P1-SRV-005.** The binary does not read the `STUARTY_*` variables of SDD-000 section 10 yet; it reads vukem's `HOST` and `PORT`. The image sets `HOST=0.0.0.0`, `PORT=7878` and an explicit `APP_ENV=development`, because vukem refuses to boot without `APP_KEY` unless the environment is set and not production (VF-030). This is a stopgap: P1-SRV-005 settles local mode and the line goes. The workspace variables (`STUARTY_HOME`, `STUARTY_FFMPEG`, `STUARTY_FFPROBE`) are set as section 11.3 has them, and `STUARTY_MODE=network` is not set (it contradicts D-029).
- **Not in the Dockerfile yet.** No `# syntax=docker/dockerfile:1` line (a floating tag); no `version` check and no `VERSION` stamp assertion, because `stuarty version` does not exist (the `VERSION` and `REVISION` build arguments still set the image labels); no `COPY THIRD_PARTY_NOTICES.md` or `licenses/`, which do not exist until RF-215. The image carries the FFmpeg licence text, configure line and source pointer, and `base-image-packages.txt`.
- **Git worktrees.** The build stage checks that git works in the context and builds without VCS stamping when it does not (a worktree `.git` file points outside the context), instead of failing as section 11.1 predicted.
- **Smoke test.** `image-smoke.sh` has no `version` run and no notices check; it adds a boot test: it starts the container published on a free loopback port and waits up to 30 s for `/up`, then removes the container. `image.yml` has no dummy tag and no `VERSION` argument, for the same reason; it builds on pull requests that touch what goes into the image and pushes nothing.
- **P2-REL-002 finished it.** `stuarty version` exists (a CLI command that needs no workspace; it prints `stuarty <agentdocs.Version>` since VV-046, before that the VCS stamp), so the build stage takes `ARG VERSION`, stamps it with `-X` and fails when the build does not print `stuarty $VERSION`; `image.yml` checks out with full history, creates the local tag `v0.0.0-ci` and passes `VERSION=0.0.0-ci`, and `release.yml` passes the release version. The runtime stage installs `util-linux` (`prlimit`, P2-RND-005) and copies `LICENSE.md`, `THIRD_PARTY_NOTICES.md` and `licenses/` to `/usr/share/doc/stuarty/`. `image-smoke.sh <image> [<version>]` adds the version check, `STUARTY_HOME=/data` and the `/data` volume, the notices, `prlimit --version` and the no-egress run (`--network none`, CLI create, edit, lint, history in a named volume, which is the part of the Prototype scenario that needs no media; import and export join when they exist). The musl question is closed: the runtime stays Debian (section 11.1).
- **Secrets for publishing.** `release.yml` will need the repository variable `DOCKERHUB_USERNAME` and the secret `DOCKERHUB_TOKEN` (a Docker Hub access token with write scope for `alexgalhardo/stuarty`), created by the author before the first release; GHCR uses the automatic `GITHUB_TOKEN`, and the package must be made public once after the first push (section 7 step 11).

## 12. Third-party notices

RF-215 and NFR-043 require notices for everything shipped, and ADR-0011 requires the FFmpeg licence text, configure line and source link in every artifact that carries FFmpeg.

### 12.1 Files

| File | Content | Generated | Shipped in |
| --- | --- | --- | --- |
| `LICENSE.md` | MIT, copyright of the author | no | archive, image, repository |
| `THIRD_PARTY_NOTICES.md` | One table per group (Go modules, npm packages, fonts, external programs), each row: name, version, SPDX licence expression, copyright line, path of the licence text under `licenses/` | yes | archive, image, repository |
| `licenses/<group>/<name>/<file>` | The verbatim licence and `NOTICE` files of each component | yes (copied) | archive, image, repository |
| `licenses/crates/<crate>@<version>/<file>` | The licence files of the Rust crates of `renderer/` (copied like the Go modules) | yes (copied) | archive, image, repository |
| `licenses/crate-texts/<crate>/<file>` | The licence text of a crate whose package ships none (`profiling`, `spirv`, the `objc2` family): the file of its upstream repository, which the generator copies into `licenses/crates/` as if the package had shipped it | no | repository (input) |
| `.config/third-party.json` | Hand-maintained inventory of components that no package manager lists: FFmpeg, the fonts, whisper.cpp and its models, OpenH264 | no | repository (input) |
| `/usr/share/doc/stuarty/ffmpeg/{LICENSE.txt,configure.txt,SOURCE.txt}` | FFmpeg licence text, the configure line as printed by the binary, the source locations | by the Dockerfile (section 11.3) | image only |
| `/usr/share/doc/stuarty/base-image-packages.txt` | the packages of the Debian runtime base (`dpkg-query -W`); each package licence is the `copyright` file in `/usr/share/doc/<package>/` of the image. `THIRD_PARTY_NOTICES.md` has a group "Base image packages (Debian)" that states this; the product bundles and links no GPL component, and these packages are an operating system, not part of the product | by the Dockerfile (section 11.3) | image only |

### 12.2 The generator

`internal/tools/notices` (Go, standard library only; new, not in vukem; vukem has `internal/tools/*` tools of the same kind, for example `configref` with its `-check`, `docs/cli.md`). It must not add a third-party Go dependency (rule: no new dependency without asking).

Inputs and algorithm:

1. **Go modules.** `go list -deps -f '{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}' ./web/cmd/app`, deduplicated, excluding the main module. For each, find the licence files in `{{.Dir}}` by name (`LICENSE*`, `LICENCE*`, `COPYING*`, `NOTICE*`, case-insensitive) and detect the SPDX id by matching the licence text against a built-in table of the allowed licences. No match fails the run.
2. **npm packages.** The production dependency closure of `web/package.json`, read from `web/node_modules` after `bun install --frozen-lockfile` (only `dependencies`, transitively, never `devDependencies`). The licence is the `license` field of each `package.json`, and the licence files are found as for Go modules.
2b. **Rust crates.** `cargo metadata --locked --filter-platform <target>` over `renderer/Cargo.toml`, the normal dependencies (never dev or build ones, never proc macros and what only they need) of two roots: the core crate `stuarty-renderer` for `wasm32-unknown-unknown` (the module the Go export embeds), and the GPU host crate `stuarty-renderer-gpu`, the binary `stuarty-render` (SDD-013 section 4.9), for each platform of the archives (`x86_64` and `aarch64` for Linux, Windows and macOS), joined, so a crate only one platform links (the `windows` crates, the `objc2` bindings of Metal) is listed. The licence files are found as for Go modules; when the package has none, the files of `licenses/crate-texts/<crate>/` stand in, and when neither exists the run fails with the crate name. The legacy `A/B` of old manifests reads as `A OR B`. `profiling` and `spirv` ship no licence text, and the `objc2` crates ship none either (the texts kept are `LICENSE-MIT.txt` and `LICENSE-ZLIB.txt` of its repository). The Apple SDK note in the `LICENSE.md` of `objc2` (the crates derive from SDK headers, whose redistribution that licence calls unclear) concerns macOS builds of `stuarty-render` only.
3. **Inventory.** `.config/third-party.json`, an array of `{name, version, spdx, copyright, files[], note, shipped_in[]}`. It contains at least: the FFmpeg LGPL build (`LGPL-3.0-or-later`, version from `.config/ffmpeg.env`, `shipped_in: ["image"]`, note that archives do not carry it), the six fonts of D-022 with the licence file each ships with (`OFL-1.1`; the task that bundles a font verifies the licence, D-022), whisper.cpp (`MIT`, `shipped_in: []`, downloaded on first use, ADR-0021) and the Whisper model (`MIT`, same), OpenH264 (`BSD-2-Clause`, inside the BtbN build; note about the Cisco binary, D-017).
4. **Allow-list.** `MIT`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `Apache-2.0`, `MPL-2.0`, `0BSD`, `Unlicense`, `OFL-1.1` (fonts only), `LGPL-3.0-or-later` (FFmpeg only, a separate process). `blessing`, the SQLite public-domain dedication (`modernc.org/sqlite`), was added by P2-REL-001 and confirmed by the orchestrator on 2026-11-14; `OFL-1.1` is also accepted for `@fontsource*` npm packages. The shadcn CLI is not a dependency, and `shadcn/tailwind.css` is not imported. Its `BlueOak-1.0.0` and `CC-BY-4.0` packages are not shipped and not allowed. Anything else, and any GPL, AGPL, SSPL, "Commons Clause" or unknown id, fails the run with the module name (NFR-043). MPL-2.0 rows carry the note "file-level copyleft: publish changes to its files" (research note `licensing` section 7).
5. **Output.** Writes `THIRD_PARTY_NOTICES.md` and `licenses/` deterministically: sorted by group then name then version, LF endings, no timestamps. `-check` regenerates in memory and exits 1 with a unified diff if the committed files differ.

### 12.3 Where it is checked

- Locally: `task notices` writes, `task notices:check` checks; `task release:preflight` runs the check.
- CI: the `Notices` job of `ci.yml` (required).
- Release: the step "Notices are current" of `release.yml`, so no artifact is built from stale notices.
- Dependabot pull requests that change `go.mod`, `go.sum` or `web/bun.lock` fail `Notices` until a maintainer runs `task notices` in the same branch. The pull request template reminds of it.

## 13. CI gates, coverage and time budget

### 13.1 The gates (ADR-0025 item 7)

| Gate | Command | Job or workflow |
| --- | --- | --- |
| Conventional Commit pull request title with allowed scopes | `amannn/action-semantic-pull-request` (section 9.2) | PR title / `Conventional title` |
| Go format | `golangci-lint run` with the `formatters` section (gofumpt, gci, golines); local fix `golangci-lint fmt ./...` | `Lint (Go)` |
| Go lint | `golangci-lint run ./...` (v2.14) | `Lint (Go)` |
| Go vet | `go vet ./...` | `Lint (Go)` |
| Import rules of ADR-0001 | depguard rules in `.golangci.yml` (section 15.3) | `Lint (Go)` |
| Tests on three operating systems, with the race detector except in the `engine/export` shards | `go test -race -count=1 -timeout=17m -coverpkg=./... -coverprofile=coverage.out <shard packages>` (`-covermode=atomic` without `-race` for the `export-K` shards), in parallel shards per system (section 13.4) | `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)` |
| Coverage gate | `go run ./internal/tools/covergate -v coverage.out` on the merged profile of the shards (ubuntu only, section 13.2) | `Test (ubuntu-latest)` |
| Front-end types | `bun run typecheck` in `web/` | `Types and lint (web)` |
| Front-end lint | `bun run lint` in `web/` (`biome ci`) | `Types and lint (web)` |
| Playwright end to end | `bun run e2e` in `web/` | `Browser tests (Playwright)`, `Browser tests (firefox)`, `Browser tests (webkit)` |
| Markdown lint | `markdownlint-cli2 "**/*.md"` (CI: the pinned action; local: `bunx markdownlint-cli2@0.24.1`) | `Lint (Markdown)` |
| Secret scan | `betterleaks git . --config .betterleaks.toml --no-banner --redact --log-opts=<range>` (before the move of section 17 the config is `web/.betterleaks.toml`) | `Secrets / Betterleaks` |
| govulncheck | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | `Vulnerabilities` |
| Generated-schema check (ADR-0014) and generated reference (ADR-0024) | regenerate, then `git diff --exit-code` over the generated paths (section 9.4 job 6) | `Generated files` |
| Third-party notices | `go run ./internal/tools/notices -check` | `Notices` |
| Release build dry run and archive size | GoReleaser snapshot, every archive under 60,000,000 bytes | `Release build (snapshot)` |
| Image build and smoke test | section 9.5 | `image / Image build` (path filtered) |
| Site build, MDX, links, markdown twin, last updated, reference, built-output checks | SDD-010 section 12 | `Site` (path filtered), `Docs last updated` (always) |
| Every action pinned by SHA | `scorecard.yml` and the job `Workflow pins` (below) | `Workflow pins` |
| Security tests on three operating systems | `go test -count=1 -v -run TestSecurity_ ./...` and the skip check of section 9.4.1 | `Security tests (ubuntu-latest)`, `Security tests (windows-latest)`, `Security tests (macos-latest)` |
| Threat coverage | `go run ./internal/tools/threatcheck` | `Threat coverage` |
| Static analysis for security | `gosec` and `forbidigo` (SDD-007 section 10.2) inside golangci-lint | `Lint (Go)` |
| Vulnerable dependencies, UI | `bun audit --audit-level=high` in `web/` | `Vulnerabilities (UI)` |
| Dependency review | `actions/dependency-review-action` (pinned in 9.0) | `Dependency review` |
| Fuzzing | 30 seconds on changed targets per pull request; weekly long run | `Fuzz (30 s)`, `Fuzz` (`fuzz.yml`) |
| No egress | `scripts/no-egress.sh` under `strace` | `No egress` |
| Sandbox | `TestSecurity_T16_BwrapHasNoNetwork` with `bwrap` installed | `Sandbox (bwrap)` |
| FFmpeg freshness | weekly job comparing the pin with the newest patch release | `FFmpeg freshness` (`ffmpeg-freshness.yml`) |
| Secret rules for `vvl_`, `vvk_`, `APP_KEY` and the transcription key | Betterleaks rules in `.betterleaks.toml` (SDD-007 section 12) | `Secrets / Betterleaks` |
| Contract and parity tests | `go test -count=1 ./web/contract/...` (SDD-003 section 7) | `Contract and parity` |
| OpenAPI validation | `openapi:export` then `bunx @redocly/cli@2.78.0 lint` (SDD-003 check 22) | `OpenAPI (Redocly)` |
| Golden cases with the FFmpeg on the path | SDD-004 section 11, reported, not blocking | `Golden cases (PATH FFmpeg)` |

`Workflow pins`: a small job in `ci.yml` (ubuntu, a checkout of the sources) that runs the block below and fails when any unpinned line is printed; `docker://` and local `./` actions are not used. This enforces R23 mechanically; vukem relies on review and Scorecard (`docs/tooling.md:151`, `scorecard.yml`). It is a required check. The pattern is anchored at the start of the line, otherwise the job matches its own text in `ci.yml` and always fails. The check is an `if` block because a bare `grep ... | grep -v ...` pipeline exits 0 when it finds an unpinned line, which is the inverse of what a check needs:

```bash
if grep -RnE "^[[:space:]]*(- )?uses: [^@ ]+@" .github/workflows | grep -vE "@[0-9a-f]{40} # v[0-9]"; then
  echo "::error::an action is not pinned by a full commit SHA with a version comment"
  exit 1
fi
```

### 13.2 Coverage gate

`internal/tools/covergate` is vukem's tool (`internal/tools/covergate/gate.go`) copied with the package table replaced. Decision of this SDD (ADR-0025 names the gate and no number): the default floor is 80 percent per package, the packages under `core/` have a floor of 90 percent (they are pure and cheap to cover, ADR-0001), and excluded from the gate are `web/cmd/...`, `internal/tools/...`, `spikes/...` and `desktop/...`. A package missing from the profile fails the gate, as in vukem, so a package without tests cannot hide. The tool asks `go list ./...` for the packages and parses their non-test files: a package with no statement in any function body (the `doc.go` of a skeleton) does not appear in a profile and is not counted as missing. The tool depends on the standard library only (the vukem tests used `go-cmp`, which this module does not require). Floors may be raised by a pull request that raises them and lowered only by a revision of this SDD.

The gate blocks since P1-REL-005 closed its last gap (the step has no `continue-on-error`). Two details beyond the text above: the CI test run passes `-coverpkg=./...`, so a package is credited with the statements any test of the repository executes in it (the wiring packages `web/bootstrap`, `web/routes`, `web/app/listeners` and `web/database/seeders` have no tests of their own and are exercised by `web/tests`); and the tool also leaves out any package under a `node_modules` directory (Go files that npm packages ship, such as `site/node_modules`). The gate runs on Linux only, from the merged profiles of the five Go test shards (`covergate` already counts a block once and covered when any profile hit it, so the profiles are concatenated). It needs every test to run, and the tests that drive FFmpeg skip where there is no pinned LGPL build: macOS has none (section 13.3, risk R6), so `Test (macos-latest)` runs `go test` and does not gate, and Windows runs the tests without gating. From release v0.21.0 (the BETA phase) the Go suite runs on all three systems again: the pause of 2026-11-15, taken because the export tests outran 20 minutes in one job, ended with the split of the suite into shards. No package was excluded to make the gate pass: the packages that were under their floor got tests (`core/timeline` schema, `service`, `service/workspace`, `engine/store`, `web/database/migrations`, `web/resources`).

### 13.3 Operating systems

| OS | Runner | Notes |
| --- | --- | --- |
| Linux | `ubuntu-latest` | FFmpeg: pinned BtbN LGPL via `scripts/install-ffmpeg.sh`; the only system that runs the coverage gate (it has the pinned build and every test runs) |
| Windows | `windows-latest` (Windows Server 2025) | FFmpeg: pinned BtbN LGPL zip via the same script under Git Bash. GitHub offers no hosted x64 Windows 11 runner; CI therefore runs Windows Server; NFR-045 names Windows 11, so the orchestrator runs `task ci` on its Windows 11 machine before each release (a line of the `task release:preflight` checklist) |
| macOS | `macos-latest` | With `FFMPEG_SOURCE="own"` (section 11.2) the script installs the own `darwin-arm64` build of the release, verified by SHA-256 like the other two, so every FFmpeg test runs on macOS and risk R6 is closed; the coverage gate stays on Linux, which merges the shard profiles. While `FFMPEG_SOURCE="btbn"` the rest of this row applies: BtbN has no macOS build, and the script installs `ffmpeg` with Homebrew for tests only; it is never distributed. Reference tests compare composed frames before encoding (ADR-0011 item 7), so the encoder difference does not matter. The tests that need the pinned build skip, so the coverage of `engine/export`, `engine/ingest` and others stays under the floors: macOS runs `go test` and no coverage gate (risk R6) |

`scripts/install-ffmpeg.sh` (bash, runs on all three): reads `.config/ffmpeg.env`, downloads the asset for the OS and architecture by exact URL, verifies the SHA-256, unpacks to `$RUNNER_TEMP/ffmpeg` (or `$TEMP`; `tar -xJf` for the Linux archives and PowerShell `Expand-Archive` for the Windows zip, because Git Bash has neither `unzip` nor a `tar` that reads zip), checks that `bin/ffmpeg`, `bin/ffprobe` and `LICENSE.txt` exist and that `ffmpeg -version` has neither `--enable-gpl` nor `--enable-nonfree`, prints the version to stderr and the `bin` folder on the last line of stdout, and adds `bin` to `$GITHUB_PATH`; on macOS runs `brew install ffmpeg` and says it is for tests only. A digest mismatch deletes the download and exits 1 with both digests in the message. `scripts/install-ffmpeg.ps1` does the same for Windows without Git Bash (it reads the `KEY="value"` lines of `.config/ffmpeg.env` without running them). `FFMPEG_ENV_FILE` and `FFMPEG_INSTALL_DIR` override the pin file and the install folder (used to test the mismatch).

### 13.4 Time budget (NFR-042)

| Job | Budget | Basis |
| --- | --- | --- |
| `Lint (Go)`, `Lint (Markdown)`, `Workflow pins`, `Notices`, `Vulnerabilities`, `Vulnerabilities (UI)`, `Dependency review`, `Threat coverage`, `Conventional title` | 5 minutes each | vukem runs the same jobs in this range |
| `Security tests (...)`, `Contract and parity`, `OpenAPI (Redocly)`, `No egress`, `Sandbox (bwrap)`, `Golden cases (PATH FFmpeg)` | 10 minutes each | a Go test run on one runner |
| `Fuzz (30 s)` | 5 minutes | 30 seconds per changed target plus the build |
| `Types and lint (web)` | 5 minutes | Studio job |
| `Browser tests (Playwright)` | 12 minutes | Studio job plus FFmpeg |
| `Generated files` | 8 minutes | Go build plus `npm ci` of the site |
| `Release build (snapshot)` | 12 minutes | six targets and the web build |
| `Go tests (<shard>, <os>)`: `export-K`, `engine`, `rest`, and `contract` on Windows | 18 minutes each (`go test -timeout=17m`), target 10 | the shards of one system run in parallel; layout below |
| `Browser tests (<firefox or webkit>, <i>/2)` | 12 minutes each | Playwright `--shard=i/2` of each engine; not required checks |
| `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)` | 5 minutes each | the required checks: they wait for the shards, check them, and on Linux merge the profiles and run the gate |

The Go suite is sharded because one job cannot hold it. Measured on 2026-11-16, without the race detector and with the pinned LGPL FFmpeg, on a Windows machine shared with other work: `go test -count=1 ./...` took 19 minutes 50 seconds, of which `engine/export` alone 18 minutes 56 seconds (80 tests); then `web/contract` 10.6 minutes, `web/app/servicetest` 5.4, `web/app/cli`, `web/cmd/app`, `engine/ffmpeg`, `engine/ingest` and `engine/frame` about 4 to 4.5 each, `core/command` 2.4. The macOS job, which skips the FFmpeg tests, already took 13 minutes with `-race` (run of 2026-11-16). The `engine/export` tests are therefore split into three shards by test name; `engine` and `rest` keep one shard each. The first pull request run (run 43924786343) measured it: under the race detector each `export` shard ran past 17 minutes on Linux and Windows, so the `export` shards run without `-race` (`-covermode=atomic`, so their profiles still merge with the others). That matches the race scope of the project, which is `core`, `service` and `web/app` (`task test:race`); the `engine` and `rest` shards keep the race detector. If an `export` shard still passes 15 minutes, the split becomes `k of N` with a larger `N`.

Measured on 2026-11-16 with `bash scripts/ci-time.sh` (first job start to last job end of the latest attempt, cancelled runs skipped, TASK-0176): the last 10 finished pull request runs of `CI`, none of them green, are 13.9, 12.0, 10.2, 10.6, 13.0, 19.2, 17.2, 23.9, 25.0 and 34.8 minutes (oldest first), median 15.5 minutes, maximum 34.8. The first four predate the sharded workflow; the six since then have median 21.6 minutes, so NFR-042 (median under 15, none over 20) is not met yet. The slowest jobs of those six: `Go tests (engine, macos-latest)` 34.2 minutes (cancelled, past its 20-minute timeout; the run of 19:11), `Bundle export (macos-latest)` 20.3, `Go tests (export-1, windows-latest)` 19.4, `Browser tests (firefox)` 15.4 to 16.9 (twice). Most runs of that day were cancelled by a newer push of the release branch, so these are the failed runs; no pull request has had a green `CI` yet, which acceptance check 6 needs. Other budgets at `v0.20.0` and the tip of `release/v0.22.0`: the largest release asset is `stuarty-desktop_0.20.0_darwin_amd64.dmg` with 29,903,106 bytes and the largest archive of the server is `stuarty_0.20.0_windows_amd64.zip` with 21,073,914 bytes (NFR-041: under 60,000,000); the largest tracked file is `renderer/fonts/Inter-Variable.ttf` with 876,576 bytes (`scripts/ci-time.sh` prints it); `bash scripts/release-audit.sh` prints the largest archive and checks it against 60,000,000.

Second measurement (2026-11-16, jobs of the runs on `release/v0.21.0` and `release/v0.22.0`, step times of each job against the time it waited for a runner). The slow jobs of the first measurement were not slow to run: `Go tests (engine, macos-latest)` (34.2 minutes), `Go tests (export-2, macos-latest)` (31) and `Bundle export (macos-latest)` (20.3) spent that time queued, then ran `engine` in 7 minutes, `export-K` in 0 to 2 (the tests skip without the pinned LGPL FFmpeg, section 13.3) and the bundle export in 2 to 3. A public repository runs five macOS jobs at a time and one run started 10 of them, so two release branches in flight (and the retries) queued each other for 10 to 32 minutes. The Linux and Windows jobs also waited 5 to 11 minutes when several runs overlapped. The jobs that really ran long: `Go tests (rest, windows-latest)` 13 to 16 minutes (`web/contract` is 10 of them), `export-1` and `export-3` on Windows 13 to 19, `Browser tests (firefox)` 14 to 15 and `(webkit)` 11 to 16 (the shard of the first measurement includes the Playwright install, about 2 minutes).

The layout that follows from it: macOS runs three Go jobs instead of five (`export-1` holds all of `engine/export`, `engine`, `rest`), which takes 2 of the 10 macOS slots of a run; Windows has five `export-K` shards (N = 5, so one shard is about a fifth of the 27 minutes of `export` tests measured there, unevenly because the split is by test name) and `contract` apart from `rest` (about 6 and 8 minutes); Linux keeps three `export-K` shards (4 to 6 minutes each when not queued); Firefox and WebKit each run in two Playwright shards (`--shard=i/2`, expected 7 to 9 minutes each). No test, the race detector (`engine`, `rest`, `contract`) or the coverage gate was removed: the Linux profiles still come from every package. If macOS gets the own FFmpeg build (`FFMPEG_SOURCE="own"`), its `export-K` tests stop skipping and it needs `export_n: 3` and the shards `export-2` and `export-3` (and `shards: 5`) in the matrices. `cancel-in-progress: true` stays: a push to a branch cancels the older run of the same branch, and the cancelled runs are skipped by `scripts/ci-time.sh`. What only the next runs can confirm: the median wall time, which depends on the macOS queue when two release branches run together, and the balance of the Windows `export-K` shards.

The jobs are parallel, `timeout-minutes: 20` is set on every pull request job, and acceptance check 6 measures the median of the last 10 pull request runs. If the median passes 15 minutes, the slowest job is split before more tests are added.

## 14. Local developer commands

`Taskfile.yml` at the repository root (go-task v3), with the names of vukem's Taskfile (`Taskfile.yml:17-259`). Targets marked "new" do not exist in vukem.

| Target | Does | Vukem line |
| --- | --- | --- |
| `task` (default) | `task --list` | 18-20 |
| `task setup` | prints tool versions (`go`, `golangci-lint`, `bun`, `git-cliff`, `gh`), runs `task hooks`, `git worktree prune` | 22-30 |
| `task hooks` | `git config core.hooksPath .githooks` (adapted: vukem runs `lefthook install`, line 32-35) | 32-35 |
| `task fmt` | `golangci-lint fmt ./...` and `bun run format` in `web/` (`biome format --write`) | 37-41 |
| `task lint` | `lint:go`, `lint:md`, `lint:web` | 43-47 |
| `task lint:go` | `golangci-lint run --allow-parallel-runners ./...` | 49-53 |
| `task lint:md` | `bunx markdownlint-cli2@0.24.1 "**/*.md"` (adapted: vukem runs an unpinned `npx --yes`, `Taskfile.yml:58`; ours pins the version) | 55-58 |
| `task lint:web` (new) | in `web/`: `bun run typecheck`, `bun run lint` | Studio `package.json` |
| `task test` | `go test -short -count=1 -timeout 20m ./...` | 60-64 |
| `task test:e2e` | in `web/`: `bun run e2e`; plus `go test -count=1 -timeout 30m ./...` without `-short` | 66-70 |
| `task test:race` | `CGO_ENABLED=1 go test -race -count=1 ./...` | 72-78 |
| `task test:linux` | the vukem Docker run with the pinned `golang:1.27.4@sha256:5e8afd...` image, `go test -race` | 213-216 |
| `task cover` | `go test -coverprofile=coverage.out ./...`, `go tool cover -func`, covergate | 94-99 |
| `task vet` | `go vet ./...`, `go fix -diff ./...` | 101-106 |
| `task vuln` | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | 118-121 |
| `task secrets:scan` | Betterleaks `1.9.0` over the history | 128-132 |
| `task tidy`, `task tidy:check` | `go mod tidy`, `go mod tidy -diff` | 145-149, 161-165 |
| `task generate` (new) | `go run ./internal/tools/contractgen`, `go run ./web/cmd/gen-agent-docs`, `npm run reference` in `site/` (which runs `command:help --json`, `openapi:export -prefix /api/v1`, `configref -json` and `contractgen -errors`, SDD-010 section 8.2), `bun run generate:types` if SDD-001 defines it separately | |
| `task generate:check` (new) | `task generate` then `git diff --exit-code` over the generated paths | |
| `task notices`, `task notices:check` (new) | section 12 | |
| `task ffmpeg:install` (new) | `bash scripts/install-ffmpeg.sh` | |
| `task image:build` (new) | `docker build -f .config/docker/stuarty.Dockerfile -t stuarty:local .` | |
| `task image:smoke` (new) | `bash scripts/image-smoke.sh stuarty:local` (section 9.5) | |
| `task site:dev`, `task site:build` | in `site/`: `npm run dev`; `npm ci`, `npm run test:site`, `npm run lint`, `npm run check:mdx`, `npm run reference:check`, `npm run build`, `npm run check:built` (the build steps of SDD-010 section 12.2) | 167-181 |
| `task site:reference` (new) | `npm run reference` in `site/` (SDD-010 section 8) | |
| `task site:dates` (new) | `npm run last-updated` in `site/` | |
| `task deps:web`, `task deps:images` (new) | `bash scripts/bump-web-deps.sh` (only when Dependabot has no `bun` ecosystem) and `bash scripts/bump-image-args.sh` (section 16.4) | |
| `task changelog` | `git cliff --config .config/cliff.toml --unreleased --strip all` (preview, writes nothing) | 183-186 |
| `task labels:sync` | `go run ./internal/tools/labelsync .github/labels.txt` (needs `gh auth`; vukem's tool copied) | 108-111 |
| `task release:preflight` (new) | step 3 of section 7: `ci:full` without services, `notices:check`, `generate:check`, `site:build`, `image:smoke` | adapted from `release`, 193-197 |
| `task release -- vX.Y.Z` | runs `release:preflight` then prints the procedure of section 7 and runs `go run ./internal/tools/release prepare vX.Y.Z` (adapted: vukem pushes, ours does not) | 193-197 |
| `task release:binaries` | GoReleaser in Docker, `SNAPSHOT=1` builds without publishing, `--skip=sign` | 204-211 |
| `task ci` | `lint`, `vet`, `test`, `tidy:check`, `notices:check` | 228-236 |
| `task ci:full` | `ci` plus `test:e2e`, `test:linux`, `vuln`, `site:build`, `cover`, `generate:check` | 238-259 |

The Windows 11 developer machine runs `task ci` with Git Bash on the path, as vukem notes (`docs/tooling.md:302-304`).

## 15. Git hooks, editor and lint configuration

### 15.1 Hooks

Decision of this SDD: **`.githooks/` with bash scripts, activated by `git config core.hooksPath .githooks`**, not lefthook. ADR-0001 lists `.githooks/` in the tree and D-021 reuses the Studio's hooks; vukem uses lefthook (`lefthook.yml`). The jobs are the same, so a contributor of one knows the other. The mapping from `lefthook.yml`:

| Hook | vukem job (`lefthook.yml`) | Ours |
| --- | --- | --- |
| `pre-commit` | `golangci-lint fmt {staged_files}` (7-13), `golangci-lint run --new-from-rev=HEAD ./...` (14-16), `task tidy:check` on `go.mod` or `go.sum` changes (17-19), `markdownlint-cli2` on staged `.md` (20-24) | the same, plus `bun run lint` in `web/` when `web/` files are staged |
| `commit-msg` | `go run ./internal/tools/commitlint {1}` (26-29) | the same (replaces the Studio's regex hook) |
| `pre-push` | `go vet ./...`, `govulncheck` (33-39), `scripts/refresh-last-updated.sh` when `site/content/**` changed (40-42) | the same; `scripts/refresh-last-updated.sh` is vukem's script (it commits the refreshed dates and stops the push; the next `git push` carries them) |

Hooks never run the test suite (vukem: "Tests, the race detector and the OS matrix run in GitHub Actions", `lefthook.yml:31-32`). `--no-verify` is not used (the global rules forbid it).

### 15.2 `.editorconfig`

Decision of this SDD, **adapted**: the author's rule is tab indentation of width 4 everywhere except formats that need spaces; vukem uses two spaces by default and tabs only for Go, Makefiles and templates (`.editorconfig:5-6`). Ours:

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = tab
indent_size = 4
tab_width = 4

# Formats that forbid tabs or that tools rewrite with spaces.
[*.{yml,yaml}]
indent_style = space
indent_size = 2

# Lists in Markdown render inconsistently with tabs, and markdownlint MD007 and
# MD010 (.markdownlint-cli2.jsonc) assume spaces.
[*.{md,mdx}]
indent_style = space
indent_size = 2

# JSON files that bun, npm and the shadcn CLI write with two spaces.
# Hand-written JSON (biome.json, ruleset, glossary.json) and TOML use tabs.
[{package.json,package-lock.json,bun.lock,tsconfig*.json,components.json}]
indent_style = space
indent_size = 2

[*.{ps1,psm1,bat,cmd}]
end_of_line = crlf
```

The exceptions are only these three: YAML (the format requires spaces), Markdown (list continuation is counted in columns) and JSON written by tools (`package.json`, lockfiles, `tsconfig*.json`, `components.json`). Everything else, TOML included, uses tabs of width 4. The copied `.config/cliff.toml` of section 5.2 shows vukem's two-space array indentation; `REL-LINT` reindents it with tabs and acceptance 1 compares the file ignoring indentation.

Consequences: the Studio's React sources and `biome.json` use two spaces; `REL-LINT` sets Biome's formatter to tabs (`"indentStyle": "tab", "indentWidth": 4`) and reformats `web/resources/js` and `web/e2e` in one isolated `style(web)` commit so later diffs stay readable. The same task reformats the copied `site/` TypeScript, TSX and Astro files to tabs in a second isolated commit, `style(site)`, made after `SITE-COPY` (SDD-010 section 1.2), so that `site/` follows the same rule as `web/` and no commit mixes formatting with content. `.gitattributes` is vukem's (`* text=auto eol=lf`, CRLF for `*.bat|cmd|ps1`, `go.sum linguist-generated`), required because of `core.autocrlf=true` on Windows (`docs/tooling.md:22`).

### 15.3 `.golangci.yml`

Copied from `vukem/.golangci.yml` (174 lines): `version: "2"`, formatters gofumpt, gci, golines (120 columns, tab length 4), the same linters, `nolintlint` with required explanation, revive `exported` and `package-comments`, `gocognit` at 30. Changes:

- `gofumpt.module-path`, `gci` `prefix(...)` and `run.relative-path-mode: gomod`: `github.com/AlexGalhardo/stuarty` (the vukem prefix `github.com/AlexGalhardo/vukem` is also a gci section so framework imports group together).
- `formatters.exclusions.paths` and `linters.exclusions.paths`: `site/`, `testdata/`, `spikes/`, `web/resources/` (not Go).
- The vukem `depguard` rules about drivers and contrib are replaced by the import rules of ADR-0001:

```yaml
    depguard:
      rules:
        core-is-pure:
          list-mode: lax
          files: ["**/core/**"]
          deny:
            - pkg: github.com/AlexGalhardo/stuarty/engine
              desc: core never imports engine (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/stuarty/service
              desc: core never imports service (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/stuarty/web
              desc: core never imports web (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/vukem
              desc: core never imports vukem (ADR-0001 rule 1)
        engine-is-not-web:
          list-mode: lax
          files: ["**/engine/**"]
          deny:
            - pkg: github.com/AlexGalhardo/stuarty/service
              desc: engine never imports service (ADR-0001 rule 2)
            - pkg: github.com/AlexGalhardo/stuarty/web
              desc: engine never imports web (ADR-0001 rule 2)
        service-is-not-vukem:
          list-mode: lax
          files: ["**/service/**"]
          deny:
            - pkg: github.com/AlexGalhardo/stuarty/web
              desc: service defines interfaces that web implements (ADR-0001 rule 3)
            - pkg: github.com/AlexGalhardo/vukem
              desc: only web imports vukem application packages (ADR-0001 rule 4)
        tests-use-stdlib:
          list-mode: lax
          files: ["$all"]
          deny:
            - pkg: github.com/stretchr/testify
              desc: use the standard library and go-cmp
```

  The exception "engine may use vukem's `contrib/engine`" (ADR-0001 rule 2) is expressed by not denying `github.com/AlexGalhardo/vukem/contrib/engine` for `engine/`: the `engine-is-not-web` rule denies only `service` and `web`. Rule 1 and the vukem prefix deny use `github.com/AlexGalhardo/vukem` as a prefix, so the vukem packages that `engine/` may need are limited to what the rule omits; if `engine/` needs a vukem package other than `contrib/engine`, the rule is changed by a pull request that cites ADR-0001.

- The `crypto-randomness` rule of vukem is kept for `**/web/**/auth/**` and `**/service/**` (key and token generation, ADR-0019).
- `forbidigo` is added (SDD-007 section 10.2 and section 12; vukem's `.golangci.yml` has `gosec` and no `forbidigo`, so it is not copied but written here). It forbids the direct outbound calls outside `internal/outbound/`:

```yaml
    forbidigo:
      forbid:
        - pattern: ^net\.(Dial|DialTimeout|DialTCP|DialUDP|Dialer)$
          msg: outbound connections go through internal/outbound (SDD-007 section 10.2)
        - pattern: ^http\.(Get|Post|PostForm|Head|DefaultClient|DefaultTransport|Client)$
          msg: outbound HTTP goes through internal/outbound (SDD-007 section 10.2)
      exclude-godoc-examples: true
```

  with `forbidigo` added to `linters.enable` and `internal/outbound/` excluded from this one linter in `linters.exclusions.rules`. `gosec` keeps the rules G201, G202, G204, G304 and G306 enabled.

- `GOCOGNIT_BUDGET` starts at 0 (no suppression exists yet).

### 15.4 `.markdownlint-cli2.jsonc`

Copied from `vukem/.markdownlint-cli2.jsonc` (lines 1-24 of that file) with `ignores` replaced by `["**/node_modules/**", "**/testdata/**", "site/out/**", "web/resources/public/**", "CREATE.md", ".claude/skills/**"]`. `MD013` off, `MD024` siblings only, `MD004` dash, `MD046` fenced, `MD033` allowed elements as vukem. `MD010` is `{ "code_blocks": false }` (vukem lists only Go and Makefile fences; ours also has tab-indented TOML, JSON and Dockerfile fences because the `.editorconfig` uses tabs for them). `docs/` is linted like everything else.

### 15.5 What P1-REL-004 built, and where it differs from sections 14 and 15

The files exist at the root: `.editorconfig`, `.golangci.yml`, `.markdownlint-cli2.jsonc`, `Taskfile.yml`, `.githooks/pre-commit` and an adapted `.githooks/pre-push`. This list is the contract where it differs from the text above.

- **Biome keeps `indentWidth` 2 with `indentStyle` `tab`** (section 15.2 says 4). Biome uses `indentWidth` only to count columns for the 80-column line limit. With 4, the reformat re-wraps lines, so it is no longer whitespace only (`git show -w --stat` must be empty); with 2 the line breaks are identical to the old two-space output. Editors still show tabs at width 4 (`.editorconfig`). The linter uses `"preset": "recommended"` (Biome 2.6 deprecates `"recommended": true`), and `security/noDangerouslySetInnerHtml` is an error. `web/package.json` gains the script `format` (`biome format --write resources/js e2e`) that `task fmt` calls.
- **`gosec` runs every rule.** Section 15.3 says it "keeps G201, G202, G204, G304 and G306 enabled"; an `includes` list would turn all the others off, so none is written and the five are part of the default set.
- **`gci` has a fourth section**, `prefix(github.com/AlexGalhardo/stuarty)`, after the vukem prefix, so product imports group last.
- **`.markdownlint-cli2.jsonc` switches off four rules** that the existing documents break on purpose: `MD029` (a numbered list continues its numbers after a code block or table and the text cites items by number; 74 findings), `MD036` (bold labels such as `**Arguments**`; 68), `MD038` (significant spaces in code spans such as `` `### ` ``; 22) and `MD014` (a `$` prompt in examples; 3). Section 15.4 listed none of them. All other findings (688 in total before the fix) were repaired in the documents without changing wording: bare URLs wrapped in `<>`, blank lines around lists, headings and fences, `|` escaped inside table cells, two heading levels, one inline-HTML placeholder put in backticks, one merged table row split, `MD025` disabled by a comment in research note `notes/engine-postmortems` (its Parts are level-one headings).
- **`task lint:go`** is `golangci-lint run`, which in version 2 also reports unformatted files (`gofumpt`, `gci`, `golines`), so a space-indented line fails it. `task fmt` rewrites them.
- **Hooks.** `task hooks` runs `git config core.hooksPath .githooks`. `pre-commit` only checks (formatting diff of staged Go files, `golangci-lint run --new-from-rev=HEAD`, `go mod tidy -diff` when `go.mod` or `go.sum` is staged, markdownlint on staged Markdown, Biome on staged web files). `pre-push` now runs `go vet ./...` and skips the `site/` date refresh while `site/` does not exist; `govulncheck` and `commit-msg` still come with `REL-CI`.
- **`task ci`** is `lint`, `vet`, `test`, `tidy:check`; `notices:check` joins it with section 12's generator. Tools used: golangci-lint 2.15.0, go-task 3.54.0, markdownlint-cli2 0.24.1, Biome 2.6.6 (the pinned `@biomejs/biome` of `web/package.json`).

## 16. Repository protection, labels, secrets and Dependabot

### 16.1 Ruleset `protect-main`

`.config/ruleset-protect-main.json`, adapted from `vukem/.config/ruleset-protect-main.json` (lines 1-42): the same rules (no deletion, no force push, linear history, pull request with zero required approvals, stale reviews dismissed, review threads resolved, branch up to date, no bypass actors), our check names and an explicit merge-method list.

```json
{
  "name": "protect-main",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "include": ["~DEFAULT_BRANCH"],
      "exclude": []
    }
  },
  "bypass_actors": [],
  "rules": [
    { "type": "deletion" },
    { "type": "non_fast_forward" },
    { "type": "required_linear_history" },
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": true,
        "allowed_merge_methods": ["squash", "rebase"]
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "required_status_checks": [
          { "context": "Conventional title" },
          { "context": "Conventional commits" },
          { "context": "Lint (Go)" },
          { "context": "Lint (Markdown)" },
          { "context": "Types and lint (web)" },
          { "context": "Test (ubuntu-latest)" },
          { "context": "Test (windows-latest)" },
          { "context": "Test (macos-latest)" },
          { "context": "Browser tests (Playwright)" },
          { "context": "Generated files" },
          { "context": "Notices" },
          { "context": "Vulnerabilities" },
          { "context": "Workflow pins" },
          { "context": "Release build (snapshot)" },
          { "context": "Security tests (ubuntu-latest)" },
          { "context": "Security tests (windows-latest)" },
          { "context": "Security tests (macos-latest)" },
          { "context": "Threat coverage" },
          { "context": "Vulnerabilities (UI)" },
          { "context": "Dependency review" },
          { "context": "Fuzz (30 s)" },
          { "context": "No egress" },
          { "context": "Sandbox (bwrap)" },
          { "context": "Contract and parity" },
          { "context": "OpenAPI (Redocly)" },
          { "context": "Betterleaks" },
          { "context": "Docs last updated" }
        ]
      }
    }
  ]
}
```

The path-filtered checks (`Site`, `Image build`) are not required, because a required check that never runs blocks the merge forever (vukem leaves its path-filtered `Site` and `image` checks out of its ruleset for the same reason). Apply it with `gh api -X PUT repos/AlexGalhardo/stuarty/rulesets/<id> --input .config/ruleset-protect-main.json` (the id is read with `gh api repos/AlexGalhardo/stuarty/rulesets`); only the author runs this, because it is a settings change.

Other repository settings, set by the author once (vukem `docs/tooling.md:180`, Scorecard checks): Dependabot alerts and security updates on; the dependency graph on (the `Dependency review` check needs it); private vulnerability reporting on; GitHub Pages source "GitHub Actions"; default branch `main`; "Automatically delete head branches" on; merge commits off; **"Default commit message for squash merges: Pull request title and description"** (without it the squash commit keeps the commit message of a single-commit pull request, and the title the changelog reads is lost); the Docker Hub secrets of section 16.2; and, once after the first image release, the GHCR package `stuarty` set to public (section 7 step 11). The footers `BREAKING CHANGE:` and `Security:` are written in the pull request body, which becomes the squash commit message (section 6.3, SDD-010 section 6.2).

Section 18 step 8 removes the required status checks from this ruleset, by the author, for the first pull request only, and `REL-REPO` restores them with the job names above in the pull request that adds the new workflows.

### 16.2 Secrets and permissions

| Secret | Used by | Who creates it |
| --- | --- | --- |
| `GITHUB_TOKEN` | all workflows | automatic |
| `DOCKERHUB_USERNAME` | `release.yml` job `image` | the author, repository variable, value `alexgalhardo` |
| `DOCKERHUB_TOKEN` | `release.yml` job `image` | the author: a Docker Hub access token with read and write scope on `alexgalhardo/stuarty`, stored as a repository secret. It is the one manual prerequisite named in ADR-0025 |
| none | `site.yml` deploy | Pages uses OIDC (`id-token: write`) |

No signing key exists: cosign is keyless (`docs/tooling.md:176`). Workflows never print secrets; a secret check step (section 9.3) fails fast with a clear message. A secret is never passed to a pull request workflow from a fork (the default).

### 16.3 Labels

`.github/labels.txt` in vukem's format `name|color|description` (`vukem/.github/labels.txt`), created by `task labels:sync` (never deletes). The `area:` labels are the commit scopes of section 6.2, so the two lists cannot drift: a test in `internal/tools/labelsync` fails when the lists differ.

```text
# type
bug|d73a4a|Something does not work as documented
enhancement|a2eeef|New feature or improvement
documentation|0075ca|Docs are wrong, missing or unclear
task|5319e7|Planned work with a done criterion
tracking|0E8A16|Long-lived tracking issue
ci-failure|B60205|A scheduled or main-branch workflow failed
# state
needs-triage|fbca04|Not yet reviewed by a maintainer
needs-repro|fbca04|Waiting for a minimal reproduction
blocked|000000|Cannot proceed until something else lands
blocker|b60205|Blocks the release: fix it before the tag
breaking|b60205|Changes or removes public behaviour
# priority
priority:p1|b60205|Fix before the next release
priority:p2|d93f0b|Fix soon
priority:p3|fef2c0|Nice to have
# phase
phase:prototype|bfd4f2|Belongs to the Prototype
phase:alpha|d4c5f9|Belongs to ALPHA
phase:beta|c2e0c6|Belongs to BETA
# area (same list as the commit scopes in SDD-009 section 6.2)
area:core|c5def5|core
area:timeline|c5def5|timeline
area:schema|c5def5|schema
area:command|c5def5|command
area:plan|c5def5|plan
area:lint|c5def5|lint
area:engine|c5def5|engine
area:raster|c5def5|raster
area:ingest|c5def5|ingest
area:export|c5def5|export
area:transcribe|c5def5|transcribe
area:service|c5def5|service
area:web|c5def5|web
area:api|c5def5|api
area:mcp|c5def5|mcp
area:cli|c5def5|cli
area:ui|c5def5|ui
area:site|c5def5|site
area:docs|c5def5|docs
area:repo|c5def5|repo
area:ci|c5def5|ci
area:deps|c5def5|deps
area:deps-dev|c5def5|deps-dev
area:release|c5def5|release
area:adr|c5def5|adr
area:sdd|c5def5|sdd
area:go|c5def5|go
area:security|c5def5|security
```

### 16.4 Dependabot, templates and ownership

`.github/dependabot.yml`, adapted from vukem's (`dependabot.yml`): weekly; `gomod` at `/` grouped as `go-deps`, prefix `build`, `include: scope`; `github-actions` at `/` grouped as `actions`, prefix `ci`; `docker` at `/.config/docker` prefix `build`; `npm` at `/site` grouped `site-deps`; and for `web/`, which has only `bun.lock` and no `package-lock.json` (an `npm` entry would not update the lockfile, and `bun install --frozen-lockfile` would then fail on every such pull request), the `bun` ecosystem grouped `web-deps` if GitHub supports `package-ecosystem: bun` at the time of `REL-REPO` (the task checks the Dependabot documentation and records the answer; **answer of 2026-11-14: it does, for Bun 1.1.39 and later and for the text `bun.lock`, so `.github/dependabot.yml` uses the `bun` ecosystem and `scripts/bump-web-deps.sh` is not needed**), else a weekly script `scripts/bump-web-deps.sh` run by the maintainer (`task deps:web`) that runs `bun update` in `web/`, `task notices` and opens the pull request. Both web and site entries use prefix `build` and `include: scope`. `REL-CI` wrote the `gomod`, `github-actions` and `bun` entries; the `docker` and `npm` entries are added by the tasks that create `/.config/docker` and `/site`, because Dependabot fails on a path that does not exist. The commit messages are then valid Conventional Commits with the scope `deps` for runtime dependencies and `deps-dev` for development dependencies, both in the list of section 6.2.

Dependabot's `docker` ecosystem does not see an image given through an `ARG` default, which is how the Dockerfile of section 11.3 names its three bases (`BUN_IMAGE`, `GO_IMAGE`, `RUNTIME_IMAGE`). A weekly script `scripts/bump-image-args.sh` (task `task deps:images`, same shape as vukem's `starterdeps` pinning of the toolchain image digest) reads the three `ARG` lines, finds the newest stable tag and digest of each, rewrites the lines and opens a pull request `build(deps): bump the image bases`, which must pass `Image build`. The BtbN FFmpeg pin in `.config/ffmpeg.env` is not seen by Dependabot; `REL-FFMPEG-PIN` documents the manual bump (a pull request that changes the file and passes the image smoke test) in `CONTRIBUTING.md`.

Files in `.github/` (vukem has all of them): `CONTRIBUTING.md` (tools, commit rules, release flow in short), `SECURITY.md` (how to report privately, how to verify a release: the commands of section 7 step 11, how to verify the image by digest), `CODE_OF_CONDUCT.md`, `CODEOWNERS` (`* @AlexGalhardo`), `pull_request_template.md`, `ISSUE_TEMPLATE/{bug_report,docs,feature_request,task}.yml` and `config.yml`, `labels.txt`, `dependabot.yml`. Decision of this SDD: these live in `.github/` as in vukem, not at the root where the Studio kept them (ADR-0001 rule 6 lists only workflow-related files at the root).

The pull request template is vukem's (`pull_request_template.md`) with these checklist lines:

```markdown
## Summary

<!-- What does this change and why? End with the ROADMAP task ("Refs: P1-ENG-003") or the issue it closes ("Fixes #123"). -->

## Checklist

- [ ] PR title is a Conventional Commit with an allowed scope; body has `Refs: <task>`
- [ ] `task ci` passes locally
- [ ] Changed contracts: `task generate` run and the generated files committed
- [ ] Changed dependencies: `task notices` run and the notices committed
- [ ] New user-facing behaviour has its page in English and in pt-BR (SDD-010), or the page is listed in the PR as pending
- [ ] No new `//nolint` without a one-line reason; no new dependency without a decision
- [ ] Workflows: actions pinned by full SHA with a `# vX.Y.Z` comment; minimal `permissions`
- [ ] `CHANGELOG.md` untouched (generated from commits by `task release`)
- [ ] Breaking changes use `!` and a `BREAKING CHANGE:` footer in this pull request body
- [ ] `Security:` footer, when it applies, are in this pull request body (it becomes the squash commit message)
```

## 17. Files that move from `web/` to the root

Paths are relative to the repository root `video/`. "Today" is the layout after the pure move of section 18 (everything of the old tree under `web/`). Actions: **move** (git mv, content kept), **move and edit**, **replace** (new content from another source; the old file is deleted in the commit that adds the replacement), **delete** (removed by the deletion commit of section 6.4). The application paths under `web/` that are not config or governance files are not in this table: sections 17.1 to 17.3 list them exactly, from the real tree of `C:\dev\stuarty\video\web\` read on 2026-11-14 (dependency and runtime folders `node_modules/`, `storage/` content and `.vukem/` excluded, as they are git-ignored).

| Path today | Path after | Action | Reason |
| --- | --- | --- | --- |
| `web/.agents/` | none | delete | D-021: 3,177 files in Portuguese, Studio skills |
| `web/.betterleaks.toml` | `.betterleaks.toml` | move and edit | secret scan config; Studio-specific allowlists reviewed; keep fingerprints out; the rules for `vvl_`, `vvk_`, `APP_KEY` and the transcription key are added (SDD-007 section 12). Until this move the scan of section 18 step 7 reads it at `web/.betterleaks.toml` |
| `web/.dockerignore` | `.config/docker/stuarty.Dockerfile.dockerignore` | replace | section 11.4 |
| `web/.editorconfig` | `.editorconfig` | replace | section 15.2 |
| `web/.env.example` | none | delete | Studio variables (Google sign-in, Railway); workspace configuration is `STUARTY_*` (SDD-000 section 10) |
| `web/.gitattributes` | `.gitattributes` | move and edit | merged with vukem's (section 15.2) |
| `web/.githooks/commit-msg` | `.githooks/commit-msg` | replace | calls `commitlint` (section 15.1) |
| `web/.githooks/pre-push` | `.githooks/pre-push` | move and edit | still refreshes `site/content/last-updated.json`; adds vet and govulncheck |
| `web/.github/dependabot.yml` | `.github/dependabot.yml` | replace | section 16.4 |
| `web/.github/workflows/ci.yml` | `.github/workflows/ci.yml` | replace | section 9.4 (the Studio file is the source of jobs 3 and 5) |
| `web/.github/workflows/pr-title.yml` | `.github/workflows/pr-title.yml` | replace | section 9.2 |
| `web/.github/workflows/release.yml` | `.github/workflows/release.yml` | replace | section 9.3 (the Studio file contributed the notes extraction) |
| `web/.github/workflows/scorecard.yml` | `.github/workflows/scorecard.yml` | replace | vukem's file (section 9.7); the Studio copy is a fork of it |
| `web/.github/workflows/security-audit.yml` | none | delete | audits the Studio skills and agent files |
| `web/.github/workflows/site.yml` | `.github/workflows/site.yml` | replace | section 9.6 and SDD-010 |
| `web/.gitignore` | `.gitignore` and `web/.gitignore` | move and edit | the root file ignores `vukem-feedback.md` (the file itself lives in the workspace root, outside the repository, D-025; the line is a safety net), `node_modules/`, `dist/`, `coverage*`, `*.log`, `.env`, `.env.*` (keeping `!.env.example`), editor folders, `site/out/`, `.cache/`, large media in `spikes/`; `web/.gitignore` keeps `/bin/`, `.vukem/`, `storage/*` with `!storage/.gitkeep`, **and the Studio build lines** `/resources/public/build/`, `/resources/public/hot`, `/bootstrap/ssr/`, `test-results/`, `playwright-report/`, or built assets and test output become stageable |
| `web/.markdownlint-cli2.jsonc` | `.markdownlint-cli2.jsonc` | replace | section 15.4 |
| `web/.railway/`, `web/.railwayignore`, `web/railway.toml`, `web/railway.app.toml` | none | delete | D-015 |
| `web/CHANGELOG.md` | `CHANGELOG.md` | move | keeps `0.1.0` to `0.4.0` (section 5.1) |
| `web/CODE_OF_CONDUCT.md`, `web/CONTRIBUTING.md`, `web/SECURITY.md` | `.github/CODE_OF_CONDUCT.md`, `.github/CONTRIBUTING.md`, `.github/SECURITY.md` | move and edit | rewritten for the editor (section 16.4) |
| `web/Dockerfile` | none | delete | the Studio docs-site image for Railway (D-015) |
| `web/Dockerfile.app` | `.config/docker/stuarty.Dockerfile` | replace | ideas kept (Bun then Go then small non-root runtime); the distribution `ffmpeg` package (a GPL build) removed (ADR-0011); the runtime is Debian slim (section 11.1) |
| `web/LICENSE.md` | `LICENSE.md` | move | MIT, unchanged |
| `web/README.md` | `README.md` | replace | describes the editor |
| `web/ROADMAP.md`, `web/prompt.md` | none | delete | D-021; the new `ROADMAP.md` is not part of this repository: it lives in the workspace root (D-025) |
| `web/cliff.toml` | `.config/cliff.toml` | replace | section 5.2 (vukem's location, `.config/`, as ADR-0001 rule 6 says) |
| `web/docs/` | none | delete | D-021, Studio research; this is the folder that collides with the new `docs/` (section 18) |
| `web/films/` | none | delete | D-021 |
| `web/scripts/film/` | none | delete | Studio film scripts; the new `scripts/` holds `refresh-last-updated.sh`, `install-ffmpeg.sh` |
| `web/site/` | `site/` | replace | SDD-010 section 1: the new site is copied from the current vukem `site/`, not moved from the Studio fork |
| `web/go.mod`, `web/go.sum` | `go.mod`, `go.sum` | move and edit | module path becomes `github.com/AlexGalhardo/stuarty` (ADR-0001); `go mod tidy` |
| `web/package.json`, `web/bun.lock` | same | kept (17.2) and edited | `name` becomes `stuarty`, `version` `0.0.0` (section 4); the `check` script (`bun --cwd site run check`, the Studio site) is removed |

New files at the root created by the tasks of section 19: `Taskfile.yml`, `.golangci.yml`, `AGENTS.md` (the public file for contributors; the working `AGENTS.md`, `ROADMAP.md`, `STATUS.md` and `vukem-feedback.md` stay in the workspace root, outside the repository, D-025), `CLAUDE.md`, `THIRD_PARTY_NOTICES.md`, `licenses/`, `scripts/`, `internal/tools/`, `core/`, `engine/`, `service/`, `desktop/`.

What P1-REL-002 did with the table (the code differs from the rows above in these points, and this paragraph is the contract):

- **Moved with `git mv`** (history follows): `.betterleaks.toml`, `.gitattributes` (merged with vukem's, with `*.cmd` and a `.githooks/*` eol rule added), `.githooks/pre-push`, `CHANGELOG.md`, `LICENSE.md`, `.github/{CODE_OF_CONDUCT,CONTRIBUTING,SECURITY}.md`. `web/.gitignore` was cut down to the Vukem app and web build lines; the root `.gitignore` already held the rest (step 7 of section 18) and needed no change. `web/package.json` is renamed `stuarty` 0.0.0 and loses the `check` script.
- **Moved, edit left to the owning task** (the edit needs files that do not exist yet): `.betterleaks.toml` keeps the Studio rules and the `.env.example` allowlist; the `vvl_`, `vvk_`, `APP_KEY` and transcription key rules are written by `REL-CI` (section 19, which owns the SDD-007 section 12 rules). The example-id allowlist is added by P1-REL-002 (section 18 step 7). `.githooks/pre-push` still only refreshes `site/content/last-updated.json`; vet and govulncheck come with `REL-CI`, because there is no Go module yet. The three `.github/*.md` files still carry the Studio text; `REL-REPO` rewrites them.
- **Replace rows not applied** (the replacement content is a separate task; the old Studio file is only deleted, by the deletion commit): `.dockerignore`, `Dockerfile.app` (`REL-DOCKER`), `.editorconfig`, `.markdownlint-cli2.jsonc` (`REL-LINT`), `.githooks/commit-msg`, `.github/dependabot.yml` and the five replaced workflows (`REL-CI`, `REL-REPO`), `cliff.toml` (`REL-CI`), `README.md` (written with the first public documentation), `site/` (`SITE-COPY`, SDD-010 section 1). Until those tasks land, the repository has no root `.editorconfig`, workflows, commit-msg hook, `.config/cliff.toml`, `README.md` or `site/`.
- **`web/go.mod` and `web/go.sum` stay in `web/`**: the packages of section 17.3 are still undecided and need them, and the module is created at the root by P1-REL-003 (ADR-0001), which then decides the fate of the 17.3 paths. Until then the Go code left in `web/` does not build (it imported the deleted `app/jobs`).

### 17.1 Paths deleted by the deletion commit (section 6.4)

Every Studio application path and every Studio leftover, read from the real tree. Paths are relative to `video/` and are under `web/` at that point.

| Path | What it is |
| --- | --- |
| `web/app/studio/` | the brief-to-film pipeline (audio, brief, captions, compile, errors, exec, llm, pcm, plan, publish, qa, render, run, skills and their tests) |
| `web/app/jobs/` | Studio job handlers (budget, hooks, limits, registry, render, root, save) |
| `web/app/controllers/job_controller.go` | Studio jobs controller |
| `web/app/models/job.go` | Studio job model |
| `web/app/requests/brief.go` | Studio brief form request |
| `web/routes/jobs.go` | Studio job routes |
| `web/database/migrations/20261105120000_create_jobs_table.go`, `20261107000000_add_job_limits.go`, `20261107120000_add_render_slots.go`, `20261107130000_add_model_budget.go` | Studio job tables |
| `web/tests/jobs_budget_test.go`, `jobs_limits_test.go`, `jobs_model_test.go`, `jobs_save_test.go`, `jobs_test.go`, `railway_config_test.go`, `voice_upload_test.go` | tests of the Studio jobs, Railway files and voice upload |
| `web/resources/js/components/BriefForm.tsx`, `FieldTip.tsx`, `Tips.tsx` | Studio brief form pieces (SDD-012 section 3.1 "files removed") |
| `web/resources/js/pages/Jobs/` (`Create.tsx`, `Index.tsx`, `Show.tsx`), `web/resources/js/pages/Dashboard.tsx` | Studio pages (SDD-012 section 3.1 "files removed") |
| `web/e2e/specs/model.spec.ts`, `ok.spec.ts`, `render.spec.ts` | Studio end-to-end flows (the Playwright setup itself is kept) |
| `web/.agents/` (`rules/studio.md`, `skills/`, 47 skill kits) | D-021 |
| `web/films/` (`vukem-launch/`) | D-021 |
| `web/docs/` (`brainstorming.md`, `recommend-tools.md`, `security-audit-2026-11-07.*`, `video-studio-kit.md`, `web-research.md`) | D-021; collides with the new `docs/` |
| `web/prompt.md`, `web/ROADMAP.md` | D-021 |
| `web/scripts/film/` | Studio film scripts |
| `web/.railway/`, `web/.railwayignore`, `web/railway.toml`, `web/railway.app.toml`, `web/Dockerfile`, `web/.env.example` | D-015 and the Studio variables |
| `web/.github/workflows/security-audit.yml` | audits the Studio skills |
| `web/site/` | the Studio docs-site fork, replaced by the new root `site/` (SDD-010 section 1) |

(The rows marked move, move and edit or replace above delete their old file in the commit that creates the new one, so the deletion commit never finds a file that a later commit still needs. 19 rows, 39 paths.)

### 17.2 Paths kept

What `docs/sdds/012-ui-architecture.md` section 3.1 says the editor reuses, plus the build and test setup it depends on.

| Path | Status |
| --- | --- |
| `web/resources/js/components/ui/` (shared primitives, Base UI and UI SDK) | kept (Biome-excluded) |
| `web/resources/js/lib/cn.ts`, `web/resources/js/hooks/use-mobile.ts` | kept as is |
| `web/resources/css/app.css` | kept (tokens extended, SDD-012 section 17) |
| `web/resources/vite/vukem.js`, `web/resources/vite/vukem.d.ts` | kept as is |
| `web/resources/resources.go` | kept as is |
| `web/resources/js/components/ThemeToggle.tsx`, `web/resources/js/pages/Error.tsx`, `web/resources/js/ssr.tsx` | kept as is |
| `web/biome.json`, `web/components.json`, `web/tsconfig.json` | kept (`biome.json` formatter set to tabs by `REL-LINT`) |
| `web/vite.config.ts` | kept and changed (second input, SDD-012) |
| `web/package.json`, `web/bun.lock` | kept; dependency changes only by the owner's approval |
| `web/e2e/playwright.config.ts` | kept: the Playwright setup |
| `web/resources/js/lib/i18n.ts`, `web/resources/js/components/LocaleSwitcher.tsx` | kept and changed (SDD-012 section 16.3) |
| `web/resources/js/layouts/AppLayout.tsx`, `web/resources/views/app.html`, `web/resources/js/app.tsx` | kept and changed (navigation reduced to Projects and Settings, theme default) |
| `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json` | kept and changed (Studio keys removed, editor keys added) |
| `web/storage/.gitkeep` | kept (the rest of `storage/`, `node_modules/` and `.vukem/` are git-ignored) |

(14 rows, 25 paths.)

### 17.3 To decide in the task

A path in neither list above is a finding. These are neither Studio-only nor named by SDD-012 section 3.1; the task that owns the area (named after each) decides keep or delete and records it in its report. None is deleted by the deletion commit until decided.

| Path | Why it is undecided | Owner |
| --- | --- | --- |
| `web/app/accounts/` (`accounts.go`, `messages.go`, `passwords.go`, `social.go`, `users.go`, `lang/en/auth.json`, `lang/en/social.json`) | accounts and login code; SDD-007 section 4 wants operator-created users and no social login, so `social.go` and `social.json` likely go, the rest may stay | SDD-007 task |
| `web/app/controllers/account/account.go`, `web/app/models/user.go`, `web/app/models/social_account.go` | account controller and models, same question | SDD-007 task |
| `web/app/controllers/doc.go`, `web/app/models/doc.go`, `web/app/requests/doc.go`, `web/app/listeners/registry.go` | empty package stubs of the vukem skeleton | SDD-005 task |
| `web/bootstrap/app.go`, `commands.go`, `providers.go` | application wiring that imports the Studio packages above | SDD-005 task |
| `web/cmd/app/main.go` | the entry point that `.config/goreleaser.yaml` builds (`./web/cmd/app`) | SDD-003 task |
| `web/routes/web.go`, `auth.go`, `registry.go`, `doc.go` | route files of the skeleton and of login | SDD-003 and SDD-007 tasks |
| `web/routes/social.go`, `web/routes/allowlist.go` | social login and the sign-in email allowlist (no social login, SDD-007 section 4) | SDD-007 task |
| `web/routes/schedule.go` | scheduled tasks declared for the Studio | SDD-006 task |
| `web/database/migrations/20261105010355_create_users_table.go`, `20261105010356_create_auth_tables.go`, `20261105010357_create_social_accounts_table.go`, `registry.go` | users and auth tables, social accounts table | SDD-005 task |
| `web/database/migrations/20260101000000_create_posts_table.go`, `web/database/seeders/registry.go` | vukem skeleton samples | SDD-005 task |
| `web/tests/auth_test.go`, `auth_messages_test.go`, `social_test.go`, `locale_test.go`, `home_test.go`, `shell_test.go`, `e2e_server_test.go`, `fixtures_test.go` | tests of the skeleton, login and locale that follow whatever is decided above | the task that decides the code they test |
| `web/resources/js/pages/Auth/Login.tsx` | SDD-012 section 3.2 lists `Auth/Login` as a page of the editor, section 3.1 does not list it as kept | SDD-012 task |
| `web/resources/js/types.ts` | SDD-012 section 3.1 removes "the Studio types in `types.ts`"; the file may stay with new types or go | SDD-012 task |
| `web/e2e/specs/helpers.ts` | shared helpers of the specs; kept if any new spec uses them | SDD-012 task |
| `web/resources/public/robots.txt` | the file that makes `go:embed public` match when the web build is missing (section 10); SDD-012 owns the embed directive | SDD-012 task |

(15 rows, 37 paths.)

Order of the moves (ADR-0001: large commits isolated): (1) the pure rename under `web/` (section 18, commit 1); (2) the root `.gitignore`, the planning documents, the spikes and the desktop placeholder (section 18, commits 2 to 4); (3) the reusable files to their final places and the new workflows, one commit per row group, in the pull request that restores the required checks (`REL-REPO`, section 18 step 8); (4) the single deletion commit of section 6.4, after every reusable file has moved and after the entries of 17.3 are decided. The Studio workflows are not copied anywhere: the Studio is deleted and its checks have no value.

## 18. Wiring the local repository to the remote history

Situation (D-005, D-020): `C:\dev\stuarty\video\` is a git repository with no commits and the remote `origin` (`https://github.com/AlexGalhardo/stuarty.git`) set. The working tree holds `docs/`, `spikes/`, `web/` and an empty `desktop/` directory (git does not list an empty directory, so only `docs/`, `spikes/` and `web/` appear as untracked). `web/` has the same files as the tip of remote `main` (the old tree was moved there). The remote `main` has the old application at the repository root, including its own `docs/` folder, and carries the tags `v0.1.0` to `v0.4.0`.

Goal: a branch whose history continues `origin/main`; the old root files appear as renamed under `web/`; the new `docs/` is added; nothing in the working tree is overwritten; no force-push.

Why the obvious ways are wrong, so nobody "simplifies" the procedure:

- A pull, a checkout of `origin/main`, a `switch -c x origin/main`, a merge of `origin/main`, a hard reset to `origin/main`, a stash and a clone into the folder all write the remote tree into the working directory. The remote root `docs/` would overwrite the author's decision log and every document of the plan, which exist only on disk (D-020 risk). None is used.
- A fresh commit on an unrelated root would need a force-push over the protected `main` and would destroy tags and releases (D-005).

The method: point the new branch at the remote commit with `git update-ref` (changes a ref, touches no file), load the remote tree into the index **under the prefix `web/`** with `git read-tree --prefix=web/` and without `-u` (changes the index, touches no file), so the index equals the working tree for `web/` and git sees every old root file as renamed.

Every command runs in Git Bash from `C:\dev\stuarty\video\`. The orchestrator runs it; the author confirms at the marked point. Do not paste an apostrophe into these commands. Every placeholder in angle brackets (`<task id>`, `<ROADMAP task id of the git wiring>`, `<id>`, `<the step 6 commit id>`) is replaced by the real value before the command runs; a commit with a placeholder left in it is not pushed. Each step below gives its command, its reason, its check and its rollback.

**Step 0. Back up what exists only on disk.** Nothing below deletes a file, but the plan documents have no other copy.

```bash
cd /c/dev/stuarty/video
git status
mkdir -p ../backup-video-2026-11-14
cp -a docs spikes desktop ../backup-video-2026-11-14/
ls ../backup-video-2026-11-14
```

Check: `git status` prints `No commits yet` and lists `docs/`, `spikes/`, `web/` as untracked (`desktop/` is an empty directory and git does not list an empty directory, so it is absent from the list; it is created as a tracked placeholder in step 7); the backup folder has the same three folders and `diff -rq docs ../backup-video-2026-11-14/docs` prints nothing.

Rollback: none needed, the backup is only a copy. It stays until the merge of step 11 is verified; it is removed afterwards only with the author's yes.

**Step 1. Local settings that avoid false differences.**

```bash
git config core.autocrlf false
git config diff.renameLimit 20000
git config merge.renameLimit 20000
git remote -v
```

Reason: with `core.autocrlf=true` a Windows working tree can differ from the remote blobs only by line endings (the remote `.gitattributes` has `eol=lf`). Rename detection must be allowed to cover the thousands of files of `web/.agents`. Check: `origin` points to `AlexGalhardo/stuarty`.

Rollback: `git config --unset core.autocrlf`, `git config --unset diff.renameLimit`, `git config --unset merge.renameLimit` (local settings only; no file is touched).

**Step 2. Fetch without touching the working tree.**

```bash
git fetch origin --tags --prune
git rev-parse origin/main
git tag -l "v0.*"
git ls-tree -r origin/main --name-only | wc -l
```

Reason: `fetch` writes only `refs/remotes/origin/*`, tags and objects. Check: `origin/main` resolves; tags `v0.1.0`, `v0.2.0`, `v0.3.0`, `v0.3.1`, `v0.4.0` exist; `git status` is unchanged from step 0. Record the commit id and the file count; they are compared again in steps 4 and 5.

Rollback: none needed; fetch adds only remote-tracking refs, tags and objects. A fetched tag that is wrong is removed locally with `git tag -d <name>` (the remote is not touched).

**Step 3. Prove that `web/` equals the remote tip, using a throwaway index.**

```bash
export GIT_DIR="$PWD/.git" GIT_WORK_TREE="$PWD/web" GIT_INDEX_FILE="$TEMP/vv-check-index"
git read-tree origin/main
git diff --stat --ignore-cr-at-eol
git ls-files --others --exclude-standard
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
rm -f "$TEMP/vv-check-index"
```

Reason: the whole method assumes `web/` is exactly the old root. The throwaway index lets git compare `origin/main` against the folder `web/` without creating or changing the real index. Check: both commands print nothing. If a file differs or is extra, **stop**: the old tree was edited after the move (or the remote moved on). Decide with the author whether the difference is wanted; do not continue until `web/` equals the remote tip or the difference is understood and recorded.

Rollback: none needed; the throwaway index is deleted by the last command and the real index is never used. If the commands were interrupted, run `unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE` and `rm -f "$TEMP/vv-check-index"`.

**Step 4. Create the branch at the remote commit, without checking anything out.**

```bash
git symbolic-ref HEAD refs/heads/feat/stuarty-layout
git update-ref refs/heads/feat/stuarty-layout origin/main
git rev-parse HEAD origin/main
git status --short | cut -c1-2 | sort | uniq -c
```

Reason: `symbolic-ref` points HEAD at a new unborn branch name (never `main`: the local `main` is created only after the merge, step 12), `update-ref` makes that branch the remote commit. The index is still empty and the working tree is untouched. Check: both ids are equal; `git status` shows every old root file as a staged deletion (`D `) and `docs/`, `spikes/`, `web/` as untracked (`??`; `desktop/` is an empty directory and is not listed). That is expected and no file on disk changed: `ls docs | head` still shows the planning documents.

Rollback (the repository returns to "No commits yet"; no file is touched): `git symbolic-ref HEAD refs/heads/main` then `git update-ref -d refs/heads/feat/stuarty-layout`.

**Step 5. Load the old tree into the index under `web/`.**

```bash
git read-tree --prefix=web/ origin/main
git status --short | cut -c1-2 | sort | uniq -c
git diff --cached --name-status -M100% | cut -c1 | sort | uniq -c
git diff --name-only | wc -l
```

Never add `-u`: it would write files into the working tree. Reason: now the index holds the old files at `web/<path>`, whose blobs are the same as the old root files, so git pairs each staged deletion with an addition. Check: `git status` shows `R ` (renamed) for every old file and `??` only for `docs/`, `spikes/`; the number of `R` lines equals the file count recorded in step 2; `git diff --name-only` (index against working tree) prints `0`, meaning no file in `web/` differs from what is staged. This count of step 5 is the one reused later (step 11 does not recount through the API). A file that shows as ` D` in the work-tree column is missing from `web/` (ignore rules never hide a tracked file): stop and report, do not force-add.

Rollback (index only; no file is touched): `rm -f .git/index`, which returns the index to the empty state of step 4.

**Step 6. Commit the pure move.**

```bash
git commit -m "refactor(repo): move the Studio tree under web/" -m "Refs: <ROADMAP task id of the git wiring>"
git show --stat -M --format=%H%n%s HEAD | tail -n 3
git log --oneline -3
git status --short
```

Reason: this commit changes nothing but paths; review shows only renames. Check: the commit has parent `origin/main`; `git show --stat -M` reports renames with no content changes (`git diff --stat -M HEAD~1 HEAD` lists no `+`/`-` lines apart from renames); `git status` lists only `docs/`, `spikes/` as untracked. `git diff HEAD --stat` is empty (the working tree equals the commit for `web/`).

Rollback (the branch returns to the state after step 4; no file is touched): `git update-ref refs/heads/feat/stuarty-layout origin/main`, then `rm -f .git/index`; repeat step 5 if the move is to be redone. To return to "No commits yet", follow the rollback of step 4 afterwards. After step 7 the same two commands also drop the later commits (they stay reachable through `git reflog` until the author decides).

**Step 7. Add the root `.gitignore`, the planning documents, and the spikes with the desktop placeholder, in three commits.** The branch then has four commits over `origin/main`: (1) the pure rename of step 6, (2) `chore(repo): add the root .gitignore`, (3) `docs: add the Stuarty planning documents`, (4) `chore(repo): add the spikes and the desktop placeholder`.

```bash
git add -n docs | wc -l
find spikes -type f -size +5M
git ls-files --others --exclude-standard spikes | head -n 50
```

Create the root `.gitignore` first (section 17, row `web/.gitignore`) so that `spikes/**/node_modules`, downloaded FFmpeg binaries, `*.exe`, and generated media are not staged; re-run the dry run until nothing larger than 5 MiB and no binary appears. Then:

```bash
git add .gitignore
git commit -m "chore(repo): add the root .gitignore" -m "Refs: <task id>"
git add docs
git commit -m "docs: add the Stuarty planning documents" -m "Refs: <task id>"
touch desktop/.gitkeep
git add spikes desktop/.gitkeep
git commit -m "chore(repo): add the spikes and the desktop placeholder" -m "Refs: <task id>"
git status --short
git ls-tree -r HEAD --name-only | grep -c "^docs/"
```

Reason: the new `docs/` shares paths with the old root `docs/` only in the history; in the index the old files are under `web/docs/`, so there is no collision. `desktop/` is an empty directory that git does not list, so the placeholder file `desktop/.gitkeep` is what makes it exist in the repository. Check: `git status` prints nothing; the new the author's decision log equals the backup (`diff docs/brainstorming.md ../backup-video-2026-11-14/docs/brainstorming.md` prints nothing); `web/docs/brainstorming.md` is the old Studio document; `git log --oneline origin/main..HEAD` lists exactly the four commits above.

The secret scan runs on these commits before the push. Betterleaks of the version of section 9.0 (`1.9.0`) must be installed first: `go install github.com/betterleaks/betterleaks@v1.9.0` and `betterleaks version` (CI downloads the Linux archive and checks the sha256 of section 9.0; on a local machine the Go module checksum database verifies the source instead). At this point the config file is still at `web/.betterleaks.toml` (it moves to the root in section 17), so the command uses that path: `betterleaks git . --config web/.betterleaks.toml --no-banner --redact --log-opts="origin/main..HEAD"` must report nothing.

False positives of the example ids. The specifications in `docs/sdds/` show example actor ids (`key:key_<10 characters>`, SDD-000 section 2), and the default `generic-api-key` rule matches them. The allowlist that silences them did not exist yet at step 7, because it lives in the root `.betterleaks.toml`, which only arrives in section 17. So at step 7 the scan reported 7 findings (`docs/sdds/005` line 1280, `006` lines 142, 486, 492, 493 and 561, `012` line 502); the orchestrator reviewed each one, all are the documented example ids `key:key_h8v2n5q7w3` and `key:key_ab12cd34ef`, and the author approved the push with them. The allowlist is added with the move of the config file (P1-REL-002): the pattern `key:key_[a-z0-9]{10}`, and only in `docs/sdds/*.md` (both conditions must hold, so the same string anywhere else is still reported). From then on `betterleaks git . --config .betterleaks.toml --no-banner --redact` and `betterleaks dir . --config .betterleaks.toml` report nothing.

Rollback: `git update-ref refs/heads/feat/stuarty-layout <the step 6 commit id>`, then `rm -f .git/index` and run `git read-tree HEAD` (no `-u`) so the index matches the step 6 commit again (no file is touched; `desktop/.gitkeep` is the only file this step creates and may stay or be removed by hand).

**Step 8. Required status checks: read the ruleset, then the author removes them for this one pull request.** Remote `main` is protected and its required status checks name jobs of the old workflows. After step 6 those workflows live in `web/.github/workflows/` and GitHub ignores them, so the pull request would wait for checks that never run and could not merge. The Studio is about to be deleted and its checks have no value, so its workflows are **not** copied to the root (a copy cannot work anyway: `defaults.run` does not reach `uses:` steps or Docker build contexts, and the workflows read `site/`, `CHANGELOG.md` and `.agents/` at the old root). Instead:

```bash
gh api repos/AlexGalhardo/stuarty/rulesets --jq ".[] | {id, name, enforcement}"
gh api repos/AlexGalhardo/stuarty/rulesets/<id> --jq ".rules[] | select(.type == \"required_status_checks\") | .parameters.required_status_checks[].context"
```

(a) Both commands are reads: the first finds the ruleset of `main`, the second lists its required status checks. (b) The author, and only the author, removes the required status checks from the ruleset of `main` for this pull request in the repository settings (Settings, Rules, the ruleset of `main`, delete the "Require status checks to pass" rule). The pull request rule and the linear history rule stay. This is a reversible settings change, not a force-push, and the orchestrator never makes it. (c) The orchestrator records the removed check names, exactly as read in (a), in the task, so that `REL-REPO` restores required checks with the new job names of section 16.1 in the pull request that adds the new workflows (the ruleset file `.config/ruleset-protect-main.json` is applied with the command of section 16.1). The pull request of step 11 then has no required check and is merged after the author looked at the diff summary.

Reason: the only alternatives are to carry workflow copies that do not run, or to leave an unmergeable pull request. Check: the second command printed the names; after (b) it prints nothing; the pull request rule and `required_linear_history` are still listed in the ruleset (`gh api repos/AlexGalhardo/stuarty/rulesets/<id> --jq ".rules[].type"` prints `pull_request` and `required_linear_history`).

Rollback: the author re-adds the required status checks with the names recorded in (c) (or applies `.config/ruleset-protect-main.json` once it exists), in the same settings page. Until then `main` has no required check, so no merge happens apart from step 11.

Case found on the real remote (2026-11-14): the first command printed nothing, because the repository had no ruleset, and it had no classic branch protection either (`gh api repos/AlexGalhardo/stuarty/branches/main/protection` answered 404 "Branch not protected"). So there were no required status checks, nothing was removed in (b), and (c) records an empty list. The pull request of step 11 is not blocked by checks (the one check that ran was `conventional`, the title check of a `pull_request_target` workflow of the old `main`, and it passed). `REL-REPO` does not restore anything: it creates the protection from scratch by applying `.config/ruleset-protect-main.json` (section 16.1) in the pull request that adds the new workflows, and the author confirms it.

**Step 9. Author confirmation before any push.** The orchestrator shows, in the chat: the output of `git log --oneline origin/main..HEAD` (exactly the four commits of steps 6 and 7), `git diff --stat -M origin/main HEAD | tail -n 5`, the step 7 secret scan result, the check names recorded in step 8 and the confirmation that the author removed them. The author answers yes in the chat. Without that yes nothing below runs (D-020). Reason: step 10 is the first action that leaves the machine. Check: the five items above are on screen and the yes is in the chat before the next command.

Rollback: nothing has left the machine; stop, and use the rollbacks of steps 4 to 7 if the branch is to be dropped.

**Step 10. Push the branch (a plain push, a fast-forward of a new branch).**

```bash
git merge-base --is-ancestor origin/main HEAD && echo "descends from origin/main"
git push -u origin feat/stuarty-layout
```

Reason: the branch contains `origin/main`, so this is an ordinary push of a new ref; no `--force`, no `--tags`. Check: the first command prints the sentence; the push reports a new branch.

Rollback: `git push origin --delete feat/stuarty-layout`, only with the author's confirmation in the chat; it removes a branch that nothing else uses, and neither `main` nor tags are touched. The local branch and the local files stay.

**Step 11. Open the pull request and merge it with rebase-merge.**

```bash
gh pr create --base main --head feat/stuarty-layout --title "refactor(repo): move the Studio tree under web/ and add the planning documents" --body "Refs: <task id>. Pure rename of the old tree under web/, the root gitignore, the planning documents, spikes and the desktop placeholder. No content of web/ changed."
gh pr view --json mergeable,mergeStateStatus
```

The pull request has no required status check (step 8). The orchestrator shows the author the diff summary of step 9 again (the rename count recorded in step 5, `git diff --stat -M origin/main HEAD | tail -n 5`; no file count through `gh pr diff`, which hits the API file caps on thousands of renames). Then **a separate, explicit yes of the author in the chat is required before `gh pr merge --rebase`**: merging into the protected `main` is the one irreversible action of this procedure. Only after that yes: `gh pr merge --rebase`. Reason for rebase-merge only here: it keeps the pure-rename commit as its own commit; a squash would still show renames in `git log -M` but would fold the move and the documents into one entry. The PR title check applies to the title above (type `refactor`, scope `repo`). Check: `gh pr view --json state,mergeCommit` prints `MERGED`.

Rollback: before the merge, close the pull request (`gh pr close`) and use the rollback of step 10. After the merge there is none that rewrites history: only a revert pull request, which restores the old tree through a new commit and is its own decision of the author.

**Step 12. Return to `main` safely after the merge.** A rebase-merge rewrites the commit ids, so the merged `main` has the same tree as the branch but different commits, and the safe check is a tree comparison.

```bash
git fetch origin --tags
git diff --stat origin/main feat/stuarty-layout
git status --short
git switch -c main --track origin/main
git status
git log --oneline -5
git branch -D feat/stuarty-layout
```

Reason: the merged `main` and the branch must hold the same files before the branch is dropped. Check: `git diff --stat origin/main feat/stuarty-layout` must print nothing (same tree). If it prints anything, for example because `main` moved for another reason after the merge, **stop and report**; do not switch. `git status --short` must also print nothing (every file is tracked and identical); if it prints anything, stop and compare against the backup. Only then `git switch -c main --track origin/main`, which changes no file. After the switch `git status` says `Your branch is up to date with 'origin/main'`. Then `git branch -D feat/stuarty-layout`; `-D` and not `-d` because the rewritten commit ids make `-d` refuse a branch whose content is merged. Afterwards run `git describe --tags --abbrev=0` and expect `v0.4.0`: this is what lets `git cliff` start the `v0.5.0` section after `v0.4.0` (section 5.2).

Rollback: the switch changes no file. Before the last command, `git switch feat/stuarty-layout` and then `git branch -D main` undo it; that is why the feature branch is deleted last and only after the tree comparison printed nothing. After the deletion its commits stay reachable through `git reflog`.

Stop conditions, in one place: any difference in step 3; any `D` or `M` in the work-tree column in step 5; a file above 5 MiB or a binary in step 7; the required-check names of step 8 not recorded, or the author's removal of them not confirmed; no explicit yes in step 9; no separate explicit yes before the merge of step 11; any output of the tree comparison in step 12. In each case nothing irreversible happened before the merge and the local files are intact; the backup of step 0 stays until the merge of step 11 is verified.

## 19. Implementation tasks owned by this SDD

These identifiers are the ones the ROADMAP must use; the ROADMAP author assigns phase and order. All are `senior-d` unless noted.

| Id | Task | Records or produces |
| --- | --- | --- |
| `REL-GIT` | Execute section 18 (orchestrator runs it, author confirms the push) | branch, pull request, `main` descending from `v0.4.0` |
| `REL-FFMPEG-PIN` | Pick the BtbN `lgpl` autobuild of the release branch `9.0` (the spike used `n9.0.2-57`), download the three assets, compute their SHA-256 locally, read the version, the BtbN commit and the source URL, write `.config/ffmpeg.env`; verify the archive layout (`bin/ffmpeg`, `bin/ffprobe`, `LICENSE.txt`); verify `ffmpeg -version` has no `--enable-gpl` and no `--enable-nonfree` | `.config/ffmpeg.env` (every `<recorded by REL-FFMPEG-PIN>` value), a line in the task report with the three digests |
| `REL-DOCKER` | (ALPHA) **First subtask:** prove on the `debian:13-slim` runtime that the pinned BtbN FFmpeg runs, with `ffmpeg -version` and the reference export inside the image (section 11.1); stop and report if it does not. Then add the Dockerfile, the dockerignore, `image.yml`, `scripts/image-smoke.sh` and `scripts/assert-web-build.sh`; re-read the Debian tag and digest; the first digest-pinned `golang` reference stays as in vukem | `.config/docker/*`, `.github/workflows/image.yml`, the two scripts |
| `REL-TOOLS` | Copy and adapt `internal/tools/{release,commitlint,covergate,labelsync}`; write `internal/tools/notices` and `internal/tools/configref` and `contractgen` hooks as needed; record the git-cliff version in `CONTRIBUTING.md` | `internal/tools/*` with tests |
| `REL-CI` | Write `.github/workflows/*` of section 9 (including the jobs of section 9.4 numbered 10 to 20, `fuzz.yml` and `ffmpeg-freshness.yml`), `Taskfile.yml`, `.golangci.yml` (with `forbidigo`), `.githooks/`, `scripts/*` (`install-ffmpeg.sh`, `refresh-last-updated.sh`, `fuzz-changed.sh`, `no-egress.sh`, `image-smoke.sh`, `assert-web-build.sh`); `.config/goreleaser.yaml`; `.config/cliff.toml`; the Betterleaks rules of SDD-007 section 12 in `.betterleaks.toml`; the `actions/setup-node` steps | the files |
| `REL-LINT` | `.editorconfig`, Biome to tabs and the isolated reformat commits, `.markdownlint-cli2.jsonc`, `.gitattributes`, `.gitignore` | the files, one `style(web)` commit and one `style(site)` commit (after `SITE-COPY`) |
| `REL-NOTICES` | (ALPHA) Create `.config/third-party.json`, run the generator, commit `THIRD_PARTY_NOTICES.md` and `licenses/`; verify the licence of each of the six fonts when it is bundled (D-022) | the files |
| `REL-REPO` | `.github/` governance files, labels, templates; Dependabot (`bun` ecosystem check, the image bump script); the author applies the ruleset, **restoring the required status checks removed in section 18 step 8 with the job names of section 16.1 in the pull request that adds the new workflows**, sets the squash default message and adds the Docker Hub secrets (the orchestrator asks, the author does it) | the files, the ruleset applied, the secrets present, the settings of section 16.1 |
| `REL-FIRST` | Cut `v0.5.0` following section 7 and verify it with the archive checks of section 7 step 11 (the image checks and making the GHCR package public belong to the first ALPHA release that carries the image) | release `v0.5.0` |

Decisions of this SDD that other documents must reflect: `internal/tools/` and `scripts/` exist at the root (ADR-0001 tree); `docs:export` is not used (SDD-010 reads three sources of the binary); `.config/cliff.toml` and `.github/{CONTRIBUTING,SECURITY,CODE_OF_CONDUCT}.md` live where vukem puts them; the scopes list (31 scopes, with `release`, `deps-dev`, `adr`, `sdd`, `go` and `security`) is enforced; task pull requests are squash-merged with the pull request title and description as the default message; the coverage floors of section 13.2; the Debian slim runtime and the `ffmpeg.env` single pin; NFR-043, NFR-044 and NFR-045 are reworded (section 11.1, section 7 step 11, section 13.3); `.githooks/` instead of lefthook; tabs in `.editorconfig` with the listed exceptions; `release.yml` publishes the notes.

## 20. Acceptance

Phases: `v0.5.0`, the end of the Prototype, ships signed archives only. The checks about the container image (the `image` job in item 4, items 9 and 10) and about the third-party notices (item 11, and the notices file inside each archive in item 5) are accepted on the first ALPHA release that carries them (tasks `REL-NOTICES` and `REL-DOCKER`; in the ROADMAP, P2-REL-001, P2-REL-002 and P2-REL-004). Every other item is accepted on `v0.5.0`.

Each check is verifiable by someone who has only this document and the repository. A check passes when the stated command or observation matches.

1. `.config/cliff.toml` equals section 5.2: `git cliff --config .config/cliff.toml --unreleased --strip all` runs and prints an `## [Unreleased]` section; run against a temporary repository with commits `feat(core): add x`, `fix(ingest): y`, `refactor(repo)!: z`, `docs: d`, `ci: c`, `feat(api): s` plus a `Security: closes a risk` footer, the sections are Added, Changed, Fixed, Security in that order, the `!` commit carries `**BREAKING:**`, and `docs` and `ci` do not appear.
2. A temporary commit `feat(foo): bar` is rejected by `go run ./internal/tools/commitlint <file>` (unknown scope), and a pull request titled `feat(foo): bar` fails `Conventional title`; titles with each of the 31 scopes pass, including `chore(release): v9.9.9`.
3. `go run ./internal/tools/release prepare v9.9.9` on a scratch branch inserts a section under `[Unreleased]` and a `chore(release): v9.9.9` commit and refuses to run on `main`; `release tag` refuses when `HEAD` is not `origin/main` (tested with a temporary bare remote, no network).
4. `release.yml` passes a review of section 9.3: top-level `permissions: contents: read`; every `uses:` line has a 40-hex SHA and a version comment; the `image` job's `if` requires `stable == 'true'`; it pushes six tags (`X.Y.Z`, `X.Y`, `latest`, on `ghcr.io/alexgalhardo/stuarty` and `alexgalhardo/stuarty`). The `Workflow pins` job passes on the repository and fails on a scratch branch that adds a `uses:` line without a SHA; the `image` job builds and smoke tests the image before the push step.
5. Dry run of the release build: `task release:binaries SNAPSHOT=1` produces six archives and six SBOM files in `dist/` plus `checksums.txt`; every archive is under 60,000,000 bytes; each archive contains `LICENSE.md`, `README.md` and `CHANGELOG.md`, and, from the first ALPHA release (RF-215), also `THIRD_PARTY_NOTICES.md` and `licenses/`; `stuarty version` of the extracted binary runs.
6. `CI` on 10 consecutive pull requests: every required check green; the median total wall time of the workflow is under 15 minutes and no run exceeds 20 minutes (`gh run list --workflow CI --json startedAt,updatedAt`).
7. The `Test` job passes on `ubuntu-latest`, `windows-latest` and `macos-latest`, with `-race`; the coverage gate fails when a `core/` package is below 90 percent (tested by lowering coverage in a scratch branch) and passes at the tip of `main`.
8. `Generated files` fails when any of `core/schema/`, `web/resources/js/generated/`, `site/generated/` or `site/content/generated/` is edited by hand, and passes after `task generate`.
9. The image builds with `task image:build`; `task image:smoke` passes: `ffmpeg -version` has neither `--enable-gpl` nor `--enable-nonfree`, `libopenh264` encodes a one-second test clip, the process runs as uid 10001, `/usr/share/doc/stuarty/` holds `THIRD_PARTY_NOTICES.md` and `ffmpeg/{LICENSE.txt,configure.txt,SOURCE.txt}`, `docker image inspect` shows `STUARTY_HOME=/data`, a volume at `/data` and a health check on `127.0.0.1:7878/up`; the version stamp check of the Dockerfile passes in `image.yml` with the dummy `VERSION` (no `+dirty`), and a scratch change that lists a tracked path in the dockerignore makes it fail.
10. `grep -nE "FROM|latest" .config/docker/stuarty.Dockerfile` shows only fixed tags and no `latest`, and the runtime base is `debian:13...-slim@sha256:...`; `.config/ffmpeg.env` contains no `<recorded by` text; the SHA-256 in it matches `sha256sum` of a fresh download of each asset.
11. `task notices:check` passes; adding a dependency licensed GPL-3.0 to a scratch branch fails it with the module name; `THIRD_PARTY_NOTICES.md` lists FFmpeg, the six fonts, whisper.cpp and every Go and npm production dependency.
12. The `protect-main` ruleset (section 16.1) is applied: a direct `git push origin HEAD:main` is rejected; a pull request whose `Lint (Go)` fails cannot merge; a merge commit cannot be created.
13. Section 18 executed on the real repository: `git log --oneline origin/main..main` after the merge shows the four commits of steps 6 and 7 (the rewritten ids after the rebase-merge, with the same subjects); `git merge-base main v0.4.0` equals `v0.4.0`; `git show -M --stat <step 6 commit>` shows only `rename` entries (100 percent similarity) and no content change; `diff -r docs ../backup-video-2026-11-14/docs` prints nothing; no `--force` appears in the shell history or in the remote's push events (`gh api repos/AlexGalhardo/stuarty/events`).
14. Release `v0.5.0` (`REL-FIRST`): `gh release view v0.5.0 --json name,body,isPrerelease,assets` returns `name` `v0.5.0`, a non-empty body that starts with `### ` and equals the section of `CHANGELOG.md` without its heading, `isPrerelease` false, 15 assets; every command of section 7 step 11 that concerns the archives (checksums, cosign, SBOM, attestation) succeeds; `git cat-file -t v0.5.0` prints `tag`. The image commands of step 11 and "both registries serve `X.Y.Z`, `X.Y` and `latest` with one digest" are checked for the first ALPHA release that publishes the image (RF-210), with `THIRD_PARTY_NOTICES.md` in every archive from that release.
15. An attempt to push a tag `v0.5.0` again, or to delete it, is documented in `CONTRIBUTING.md` as forbidden and the release tool refuses a version that exists (`release tag v0.5.0` exits 1).
16. The hooks work: `git config core.hooksPath` prints `.githooks`; a commit with the header `Fix Things.` is rejected by `commit-msg`; a push that changes `site/content/**` without refreshed dates is stopped by `pre-push` with the committed refresh.
17. `.editorconfig` matches section 15.2: `editorconfig-checker` (or the editor) reports no violation on `core/`, `engine/`, `service/`, `web/resources/js`, `web/e2e` and `site/` (tabs, including the TypeScript, TSX and Astro files of `site/`) and on `*.yml`, `*.md` (two spaces); the only space-indented files outside YAML and Markdown are `package.json`, lockfiles, `tsconfig*.json` and `components.json`.
18. Every statement of section 2 can be followed from its cited vukem file and line: spot check R6, R15, R22 and R27 against `v0.43.0:.github/workflows/release.yml:120-164`, `v0.43.0:.config/goreleaser.yaml:8-18` and `v0.43.0:.config/ruleset-protect-main.json:12-40`, read with `git -C C:\dev\stuarty\vukem show v0.43.0:<path>`.
19. Section 18 is safe step by step: each of steps 0 to 12 has a command, a reason, a check and a rollback; no step uses a stash, a hard reset, a checkout of paths or a force push; no step pushes before the confirmation of step 9, and the merge of step 11 needs its own explicit yes.
20. The supply-chain audit of a release (RF-215, NFR-041, NFR-043): `bash scripts/release-audit.sh [vX.Y.Z | --dist DIR] [--image REF | --no-image] [--strict]` exits 0 on a published release and on a local `dist/`. It opens every archive and installer (7z for `.deb`, `.pkg` and AppImage, `innoextract` for the Inno Setup `.exe`, both CLIs on the runner and not dependencies; an artifact it cannot open is "NOT INSPECTED", a failure only under `--strict`) and requires `THIRD_PARTY_NOTICES.md` and `licenses/` in each of them: the archives, the `.deb`, `.pkg` and AppImage of the server, the server and desktop Inno Setup installers, the desktop `.deb` and the `.dmg` (the nfpm `contents` of `goreleaser.yaml` and the build scripts of each installer copy both). It also checks the largest archive against 60,000,000 bytes and the image. `release.yml` runs it in the job `audit` after the release, with `innoextract` installed.
    - The judges are `go run ./internal/tools/notices -audit-tree DIR`, which matches every text under `DIR/licenses` with the signatures of the generator and applies its allow-list, and `-audit-spdx`, which reads one `name<TAB>SPDX expression` per line on stdin (the SBOM packages) and applies the same allow-list: one source of truth (section 12.2). A verdict is `deny` (GPL, AGPL, SSPL, LGPL-2.x or LGPL-3.0-only), `notallowed` or `unknown`; any of them is a finding, except that an SBOM `unknown` (`LicenseRef-*` and `NOASSERTION`, which the SBOM scanner prints when it cannot identify a text) is a "NOTE" that fails only under `--strict`, because the `licenses/` scan of the same archive is the judge of the text.
    - A scanner can claim a licence the shipped text does not show. The decision is a reviewed waiver in `.config/sbom-waivers.json`, read by `-audit-spdx -waivers FILE -waiver-root DIR`. Each entry has `package`, `claimed` (the licence the scanner reports), `shipped` (the licence the text shows), `path` (the text, relative to the root of the archive of that SBOM), `sha256` of that text, and a one-line `reason`; every field is required and an unknown field is an error. A waiver applies only to the exact `package` and `claimed` pair and only while the text at `path` still has the `sha256`: when the text changes, or `path` disappears (a dependency upgrade moves it), the line is printed as `expired` and is a finding again, so the waiver has to be reviewed again. A waived line is printed as `WAIVED` and is not a finding. The file starts with three waivers: `modernc.org/libc` (GPL-2.0-only claimed from the comments of generated header translations of subpackages that are not linked; the module text is BSD-3-Clause), the product module (LGPL-3.0-only claimed from the aggregate of `licenses/`, which holds the text of the separate FFmpeg program; the product is MIT) and `github.com/modelcontextprotocol/go-sdk` (CC-BY-4.0 claimed; the text says the code is Apache-2.0 or MIT and CC-BY-4.0 covers only documentation).
    - `scripts/release-verify.sh` expects 26 assets from `v0.16.0` and 29 from `v0.18.0` (one more for a stable release, with `feed.json`) and 21 for `v0.15.0`, which has no desktop application or feed.
