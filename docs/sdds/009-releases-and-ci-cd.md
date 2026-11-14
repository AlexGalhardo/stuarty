# SDD-009: Releases and CI/CD in the vukem pattern

- Status: Accepted (2026-11-14). Drafted by a delegated engineer, audited independently, corrected under the architect's decisions.
- Depends on: SDD-000, SDD-007, ADR-0001, ADR-0011, ADR-0014, ADR-0024, ADR-0025, ADR-0026, brainstorming D-005, D-006, D-008, D-015, D-016, D-017, D-020, D-021, D-025
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

- The vukem repository at `C:\dev\vukem-video\vukem\`, read at the tag `v0.43.0`. Every citation of a vukem file in this document, written `path:line` or `file:line`, means `v0.43.0:<path>:<line>` and is read with `git -C C:\dev\vukem-video\vukem show v0.43.0:<path>`, never from the working tree (the working tree is ahead of the tag). Lines were rechecked against the tag on 2026-11-14.
- Where the vukem working tree already has what we need beyond the tag, this document says "present on vukem main after v0.43.0" (the Docker Hub publishing, commit `bb1fffa0`); it is not called missing.
- The Studio application's CI in `video/web/.github/`, `video/web/.githooks/`, `video/web/cliff.toml`, `video/web/Dockerfile.app`. The Studio is about to be deleted; its workflows are sources of ideas only and are never copied into the new pipeline.
- The workspace root `C:\dev\vukem-video\` holds `ROADMAP.md`, `AGENTS.md`, `STATUS.md` and `vukem-feedback.md` OUTSIDE this repository (D-025). The repository has no ROADMAP and no STATUS file; it has a public `AGENTS.md` for contributors.
- `docs/search/vukem-practices-and-releases.md` and `docs/search/notes/vukem.md`.

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
| R5 | The release title is exactly the tag | `internal/tools/release/main.go` (`--title version`); `docs/search/vukem-practices-and-releases.md` section 1 | The workflow creates the release with `--title "$TAG"` (`release.yml` below, step "Publish the release notes") |
| R6 | The body is the changelog section of that version, copied, without the `## [x]` heading, starting at `### Added` | `docs/tooling.md:100`; Studio `release.yml` awk extraction; `docs/tooling.md:159-172` | Same, extracted by the workflow with the literal-match awk of `docs/tooling.md:164-168`, **adapted**: one extra stop line at the first comparison link (`[x.y.z]: ...`) so the last section does not swallow the link footer |
| R7 | A release with an empty body is refused | `docs/tooling.md:118`; `internal/tools/release/main.go` (`refusing to publish ... empty release body`) | The workflow fails when the extracted section has no non-blank character |
| R8 | Sections in fixed order: Added, Changed, Deprecated, Removed, Fixed, Security | `.config/cliff.toml:50-62` (`<!-- N -->` group prefixes) | Same, section 5 |
| R9 | Entry format `- **<scope>:** Message ([abc1234](commit URL))`, `**BREAKING:**` in front when it breaks, commit links and no pull request links | `.config/cliff.toml:23` | Same, only the repository URL changes |
| R10 | `CHANGELOG.md` is Keep a Changelog 1.1.0 with a permanent `[Unreleased]` section and comparison links at the bottom; released sections are never edited by hand | `.config/cliff.toml:4-6, 27-40`; `docs/tooling.md:100` | Same. The `[Unreleased]` section is not written by hand either (ADR-0025 item 3) |
| R11 | Version sections are generated by git-cliff from Conventional Commits; `docs`, `test`, `ci`, `build`, `chore`, `style` are omitted | `.config/cliff.toml:53-62`; `docs/tooling.md:90-98` | Same |
| R12 | `Security:` footer on a `feat`, `fix`, `perf` or `refactor` commit files it under Security; a body that only mentions security does not | `.config/cliff.toml:57-60` | Same |
| R13 | The release commit is `chore(release): vX.Y.Z`, skipped by cliff | `internal/tools/release/main.go:139`; `.config/cliff.toml:51` | Same |
| R14 | Tag push triggers `release.yml`; `workflow_dispatch` with a version input recovers a tag whose push started no run; version tags are pushed one at a time | `release.yml:15-22`; `docs/tooling.md:153-157` | Same |
| R15 | Binaries by GoReleaser: static, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`, linux, darwin, windows on amd64 and arm64 | `.config/goreleaser.yaml:8-18` | Same, with binary `vukem-video` from `./web/cmd/app` |
| R16 | Archives `tar.gz`, `zip` on Windows, containing the licence, README and CHANGELOG | `.config/goreleaser.yaml:20-26` | Same plus the third-party notices (RF-215) |
| R17 | GoReleaser never writes the notes: `release.mode: keep-existing`, `changelog.disable: true` | `.config/goreleaser.yaml:48-53` | Same |
| R18 | `checksums.txt` signed with cosign keyless, verified right after signing | `.config/goreleaser.yaml:38-43`; `release.yml:81-86` | Same |
| R19 | One SPDX SBOM per archive by syft | `.config/goreleaser.yaml:31-33`; `release.yml:72-74` | Same |
| R20 | Build provenance attestation of `checksums.txt`, and the bundle attached to the release as `<name>.intoto.jsonl` | `release.yml:87-100` | Same, file `vukem-video.intoto.jsonl` |
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

Release `v0.43.0` of `AlexGalhardo/vukem`, published 2026-11-12, title `v0.43.0`. Verbatim from `gh release view v0.43.0` (copied from `docs/search/vukem-practices-and-releases.md` section 3):

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

- **core:** Add the timeline document types and their JSON Schema ([6854f17](https://github.com/AlexGalhardo/vukem-video/commit/6854f171519cb928540077d9d397d1dd4d5365c4))
- **command:** Add the command registry with apply, inverse and actionable errors ([57794e6](https://github.com/AlexGalhardo/vukem-video/commit/57794e637edd94041d38d0906528571347c5fb34))
- **plan:** Add the frame plan that says what is on screen at frame t ([1b02f1e](https://github.com/AlexGalhardo/vukem-video/commit/1b02f1e13c761440bc229328cd4029b5d775210c))
- **lint:** Add structural checks that return plain text findings ([5d85df6](https://github.com/AlexGalhardo/vukem-video/commit/5d85df6543399001774ff7fc740f1f2991e6a5cb))
- **ingest:** Add normalized media, proxy and thumbnail generation ([4f2e65e](https://github.com/AlexGalhardo/vukem-video/commit/4f2e65eb7fb03ec4c56952d555add8d2f9e6e1fe))
- **service:** Add the service boundary with revisions, undo and redo ([1b2c6ad](https://github.com/AlexGalhardo/vukem-video/commit/1b2c6adf6a782f69d7ccc9a1b905b49d71cfe4f7))
- **api:** Add the HTTP API under /api/v1 and the OpenAPI 3.1 document ([eecac20](https://github.com/AlexGalhardo/vukem-video/commit/eecac20f3712524487c5bc5a8df0cfdd524fc248))
- **mcp:** Add the MCP server with the edit, view and lint tools ([b329252](https://github.com/AlexGalhardo/vukem-video/commit/b32925249de66da3389d6199cffb1cfb12b6009b))
- **cli:** Add project:create, edit, view and export to vukem-video ([9d0bb76](https://github.com/AlexGalhardo/vukem-video/commit/9d0bb769ee1fc748395d66ebbcfaf76c07c2f2ed))
- **ui:** Add the browser editor with live preview and the four basic operations ([d621c7c](https://github.com/AlexGalhardo/vukem-video/commit/d621c7cf0329903518fbeb542bd331a0c6745111))
- **site:** Add the documentation site in English and Brazilian Portuguese ([2028dc6](https://github.com/AlexGalhardo/vukem-video/commit/2028dc6aff0695d610f083554d15fd99afb16e70))

### Changed

- **BREAKING:** **repo:** Replace the Studio application with the Vukem Video editor ([079a3e6](https://github.com/AlexGalhardo/vukem-video/commit/079a3e6ce84328842b8cafae9dd27a99f937c9bd))
- **repo:** Move the Studio tree under web/ ([1ec9385](https://github.com/AlexGalhardo/vukem-video/commit/1ec9385a0cf834ba9f68a702bb35ee06cc38ba35))
- **timeline:** Store item times as whole frames on the project frame rate ([e8cdff8](https://github.com/AlexGalhardo/vukem-video/commit/e8cdff8b5324d58e74f6112b3237fd938425ee4f))

### Fixed

- **ingest:** Apply the display rotation of phone videos before normalizing ([2a69fb7](https://github.com/AlexGalhardo/vukem-video/commit/2a69fb78dc49152ee3bfbcb16d924b8ef30bce36))
- **export:** Keep every output dimension even for 4:2:0 ([93b6bcd](https://github.com/AlexGalhardo/vukem-video/commit/93b6bcdcf5cc9d22d7f1bea0946de2e486def032))

### Security

- **api:** Require the local token and check Host and Origin on every request ([bb7aec9](https://github.com/AlexGalhardo/vukem-video/commit/bb7aec9ed40f05a334f45748d5443dd44d1f4376))
```

How each visible property is produced:

- The `**BREAKING:**` entry comes from `refactor(repo)!: replace the Studio application with the Vukem Video editor` with a `BREAKING CHANGE:` footer (section 6.4). It is under Changed because `refactor` maps to Changed (`.config/cliff.toml:61`). There is no Removed section: cliff fills Removed only from `revert` commits (`.config/cliff.toml:61`), so a deletion is reported as a breaking change. This is vukem friction 1 in the final answer.
- The `Move the Studio tree under web/` entry comes from the pure-rename commit of section 18.
- The Security entry comes from a `feat(api)` or `fix(api)` commit with a `Security: <risk closed>` footer.
- `ci`, `docs`, `build`, `test`, `chore` and `style` commits (for example every commit of this SDD's workflows) never appear.

## 4. Versioning, phases and tags

1. Versions follow SemVer 2.0.0 (R1). The single source of the version is the git tag; there is no version field to keep in step, except `web/package.json` `version`, which the release commit does not touch (Decision of this SDD: it stays `0.0.0` and `private: true`, because the Studio's `0.4.0` there was a second source of truth that drifted).
2. Phase mapping (ADR-0025 item 2), consistent with vukem's `docs/tooling.md:106-110`:

| Phase | Tags | One release per | Ends when |
| --- | --- | --- | --- |
| Prototype | `v0.5.0`, then `v0.5.1`, `v0.5.2` ... (patch bump per task group) | ROADMAP task group | the Prototype exit criteria hold |
| ALPHA | `v0.6.0`, `v0.7.0` ... (minor bump per task group) | ROADMAP task group | the ALPHA exit criteria hold |
| BETA | the `v0.N.0` line continues (minor bump per task group) | ROADMAP task group | the BETA exit criteria hold, including the own LGPL FFmpeg build (D-017) |
| 1.0 | `v1.0.0` directly, no release candidates | the BETA exit | |

3. The first tag is `v0.5.0`. The version after `v0.4.0` (the Studio's last tag) is `v0.5.0`, so the line continues without a collision (D-005). `v0.5.0` is cut when the Studio replacement is merged and the whole pipeline of section 9 is green on `main`; it may be small. Cutting it early is deliberate: it is the first run of `release.yml` and of signing, and should happen while the content at risk is small. `v0.5.0` publishes the signed archives with checksums, SBOMs and provenance. The container image (RF-210, two registries) and the third-party notices (RF-215) are ALPHA: they first ship in the first ALPHA release, and only from that release do the archives carry `THIRD_PARTY_NOTICES.md` and the registries serve the image tags.
4. A patch release is a bug fix only and never carries a breaking change (R2). A breaking change before 1.0 is a minor bump and is marked `!` (section 6).
5. Pre-release suffixes (`-rc.1`, `-beta.1`) are allowed by the workflow (they build binaries, publish no image, never move `latest`, R21) but the project does not use them (ADR-0025 item 2).
6. A tag is annotated (`git tag -a vX.Y.Z -m vX.Y.Z`), is created only by the release tool on the squash-merged release commit, is pushed alone (`git push origin vX.Y.Z`, one tag per push, `docs/tooling.md:155`) and is never moved or deleted (R3). The tags `v0.1.0` to `v0.4.0` already exist on the remote and stay.
7. Go module consequence: the module `github.com/AlexGalhardo/vukem-video` is an application; nobody imports it. Tags are release markers, not module versions, so there is no `go.mod` `retract` step (R3). If this changes, SDD-009 is revised.

## 5. Changelog and `cliff.toml`

### 5.1 Rules

1. `CHANGELOG.md` at the repository root is Keep a Changelog 1.1.0: the header text, a permanent `## [Unreleased]` section, one `## [X.Y.Z] - YYYY-MM-DD` section per release, comparison links at the bottom.
2. Sections are generated by `git cliff --config .config/cliff.toml` (section 8) from Conventional Commits. No human edits a released section. No human edits `[Unreleased]`: it is rewritten by the tool only (ADR-0025 item 3; this resolves VF-007, where vukem states the rule and does not always follow it).
3. A wrong released section is corrected by fixing the template or, for the next release, the commit; never by editing the published section.
4. The existing sections `0.1.0` to `0.4.0` moved from `web/CHANGELOG.md` and stay as they are; their comparison links keep the old repository name `vukem-studio`, which GitHub redirects. This is the only place the old name remains.
5. The mapping from commits to sections is the one of vukem (`docs/tooling.md:90-98`):

| Commit | Section |
| --- | --- |
| `feat` | Added |
| `perf`, `refactor` | Changed |
| footer `Deprecated:` on any type | Deprecated |
| `revert` | Removed |
| `fix` | Fixed |
| footer `Security:` on `feat`, `fix`, `perf` or `refactor` | Security |
| `docs`, `test`, `ci`, `build`, `chore`, `style`, `chore(release)` | omitted |

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
- {% if commit.breaking %}**BREAKING:** {% endif %}{% if commit.scope %}**{{ commit.scope }}:** {% endif %}{{ commit.message | upper_first | replace(from="<", to="\\<") }} ([{{ commit.id | truncate(length=7, end="") }}](https://github.com/AlexGalhardo/vukem-video/commit/{{ commit.id }}))
{% endfor %}\
{% endfor %}
"""
footer = """
{% for release in releases -%}
{% if release.version -%}
{% if release.previous.version -%}
[{{ release.version | trim_start_matches(pat="v") }}]: https://github.com/AlexGalhardo/vukem-video/compare/{{ release.previous.version }}...{{ release.version }}
{% else -%}
[{{ release.version | trim_start_matches(pat="v") }}]: https://github.com/AlexGalhardo/vukem-video/releases/tag/{{ release.version }}
{% endif -%}
{% else -%}
[Unreleased]: https://github.com/AlexGalhardo/vukem-video/compare/{{ release.previous.version }}...HEAD
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
  { footer = "^Deprecated", group = "<!-- 2 -->Deprecated" },
  { message = "^(docs|test|ci|build|chore|style)", skip = true },
  # Security is explicit: a "Security:" footer on a feat, fix, perf or
  # refactor commit. Matching the word in the body filed unrelated commits
  # (a feature whose test is named TestSecurity_*) under Security.
  { footer = "^Security", group = "<!-- 5 -->Security" },
  { message = "^feat", group = "<!-- 0 -->Added" },
  { message = "^(perf|refactor)", group = "<!-- 1 -->Changed" },
  { message = "^revert", group = "<!-- 3 -->Removed" },
  { message = "^fix", group = "<!-- 4 -->Fixed" },
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

**Adapted:** vukem lists scopes in prose and does not enforce the list (`internal/tools/commitlint/lint.go:20,63-64` checks only the shape). Here the list is enforced twice: by the local validator and by the pull request title check. A scope is optional; when present it must be one of these **23** scopes. The list is derived from ADR-0001; the first eleven rows are the `core/`, `engine/` and `service/` packages. `release` is the scope of the release commit and the release pull request (`chore(release): vX.Y.Z`); without it the commit-msg hook and `Conventional title` would reject every release. `deps-dev` is what Dependabot writes for development dependencies (`include: scope`).

| Scope | Covers |
| --- | --- |
| `core` | `core/` as a whole, `core/timeval`, `core/schema`, cross-cutting core changes |
| `timeline` | `core/timeline` |
| `command` | `core/command` |
| `plan` | `core/plan` |
| `lint` | `core/lint` (the editor lint, not the code linters) |
| `engine` | `engine/` as a whole, `engine/ffmpeg`, `engine/store` |
| `raster` | `engine/raster` and the bundled fonts |
| `ingest` | `engine/ingest` |
| `export` | `engine/export` |
| `transcribe` | `engine/transcribe` |
| `service` | `service/` |
| `web` | the vukem application in `web/` that is none of `api`, `mcp`, `cli`, `ui` (persistence, jobs, auth, SSE) |
| `api` | HTTP and SSE endpoints, OpenAPI |
| `mcp` | the MCP server and its tool descriptions |
| `cli` | the `vukem-video` commands |
| `ui` | `web/resources/js` |
| `site` | `site/` |
| `docs` | `docs/`, README and other repository documents |
| `repo` | repository layout, root files, git configuration, the public `AGENTS.md` for contributors |
| `ci` | `.github/`, `.config/`, `Taskfile.yml`, `.githooks/`, `internal/tools/` |
| `deps` | dependency bumps (Dependabot prefix `build(deps)` and `ci(deps)`) |
| `deps-dev` | development dependency bumps (Dependabot `build(deps-dev)`) |
| `release` | `chore(release): vX.Y.Z`, the release commit and pull request (section 7) |

`desktop/` has no scope until BETA; the task that starts `desktop/` adds `desktop` to this table, to the validator and to the labels in the same pull request (a change of this SDD).

### 6.3 Enforcement

| Where | How |
| --- | --- |
| Local, on every commit | `.githooks/commit-msg` runs `go run ./internal/tools/commitlint "$1"`. The tool is vukem's `internal/tools/commitlint` copied with one change: an allow-list of the scopes above and the type list (`REL-TOOLS`) |
| Pull request | `.github/workflows/pr-title.yml` (section 9.2) |
| Merge | `main` accepts squash merge and rebase merge; the pull request title becomes the squash commit subject and the pull request body its message, so the title is what cliff reads (R27). This holds only with the repository setting "Default commit message for squash merges: Pull request title and description" (section 16.1); GitHub defaults to the commit message for a single-commit pull request |

Decision of this SDD: **task pull requests are squash-merged**, so one pull request is one changelog entry and its title is the entry. The footers that cliff and the translation check read (`BREAKING CHANGE:`, `Security:`, `Translation:`) must be written in the **pull request body**, because the body becomes the squash commit message; a footer only in a branch commit is lost. vukem's `docs/tooling.md:112` says rebase-merge while `pr-title.yml` says the title becomes the commit; the two statements disagree (vukem friction 2). Rebase-merge is used only for the first pull request of section 18, to keep the pure-rename commit.

### 6.4 The commit that deletes the Studio

```text
refactor(repo)!: replace the Studio application with the Vukem Video editor

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
   - 2a. `git fetch origin --tags` and the local `main` equals `origin/main`: `git switch main && git merge --ff-only origin/main`.
   - 2b. The tree is clean: `git status --porcelain --untracked-files=no` prints nothing.
   - 2c. The required checks are green on the `origin/main` commit: `gh run list --branch main --limit 10`.
   - 2d. No open issue labelled `ci-failure`: `gh issue list --label ci-failure --state open` is empty (R25).
   - 2e. Every task of the group is checked in the ROADMAP (workspace root, outside this repository, D-025) and its verification passed. This is checked by the orchestrator, not by the tool.
3. **Local gate.** `task release:preflight` (section 14): `task ci:full` minus the integration services, plus `task notices:check`, `task generate:check`, `task site:build`, `task site:translations`, `task image:smoke`. It must pass.
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
REPO=AlexGalhardo/vukem-video
ARCHIVE="vukem-video_${V}_linux_amd64.tar.gz"
gh release view "$VERSION" -R "$REPO" --json body,assets --jq "(.body | length > 0), (.assets | length)"   # true, 15
gh release download "$VERSION" -R "$REPO" -p checksums.txt -p checksums.txt.sigstore.json -p "$ARCHIVE" -p "$ARCHIVE.sbom.json"
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp "^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@refs/tags/v" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
gh attestation verify "$ARCHIVE" -R "$REPO"
for image in ghcr.io/alexgalhardo/vukem-video alexgalhardo/vukem-video; do
  digest="$(docker buildx imagetools inspect "$image:$V" --format "{{.Manifest.Digest}}")"
  cosign verify "$image@$digest" \
    --certificate-identity-regexp "^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@refs/tags/v" \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com
  echo "$image $digest"
done
docker run --rm "ghcr.io/alexgalhardo/vukem-video:$V" version
```

   The image loop and the `docker run` apply from the first ALPHA release that publishes the image (not `v0.5.0`). The two `echo` lines must print the same digest (one build pushed to both registries). After the first image release only: set the GHCR package visibility to public (GitHub, profile, Packages, `vukem-video`, Package settings), or an anonymous `docker pull` fails (`docs/maintenance.md:48`); and confirm the Docker Hub repository `alexgalhardo/vukem-video` is public. This one-time step is listed as a repository setting in section 16.1, not as a step of every release; the manual steps of a release are the release pull request and the tag (NFR-044).
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
- The tool does not mark `v0.x` as pre-release. vukem's code does (`internal/tools/release/main.go`, `--prerelease` for `v0.`) while every release from `v0.5.0` on is published as a normal release (`docs/search/vukem-practices-and-releases.md` section 2, VF-005); we follow what was observed. Only a tag containing `-` is marked pre-release, in the workflow.
- Tests: table-driven tests for the section insertion, footer rewriting and tag preconditions, as vukem's `changelog_test.go` and `bump_test.go`. The tool has no network access in tests.

## 9. Workflows

Common rules for every file in `.github/workflows/` (vukem `docs/tooling.md:151`, `.claude/rules/workflows.md`):

- `permissions: contents: read` at the top; a job widens permissions only for what it writes.
- `persist-credentials: false` on checkout when the job does not push.
- `concurrency` with `cancel-in-progress: true` on workflows that run on pull requests (never on `release.yml`).
- Every job has `timeout-minutes`. Jobs that gate a pull request have 20 at most (NFR-042). The release jobs and the weekly jobs state their own, truthfully: `release.yml` job `goreleaser` 30, job `image` 45, `fuzz.yml` 90, other weekly jobs 20.
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
| `image.yml` | image | pull request touching the image inputs, manual | Merge when the image inputs change (not required; path-filtered) |
| `site.yml` | Site | push to `main` and pull request touching `site/**`, `CHANGELOG.md` or the workflow; manual | The site deploy |
| `docs-last-updated.yml` | Docs last updated | every push to `main`, every pull request, manual | Merge (required) |
| `secrets.yml` | Secrets | push to `main`, pull request, weekly, manual | Merge (required) |
| `codeql.yml` | CodeQL | push to `main`, pull request, weekly | Informational, `ci-failure` issue |
| `vuln.yml` | Vulnerabilities (weekly) | weekly, manual | `ci-failure` issue |
| `scorecard.yml` | Scorecard | branch protection rule, push to `main`, weekly, manual | `ci-failure` issue |
| `fuzz.yml` | Fuzz | weekly (cron `17 3 * * 0`), manual with `fuzztime` | `ci-failure` issue (the long run; the 30 s pull request run is the `Fuzz (30 s)` job of `ci.yml`) |
| `ffmpeg-freshness.yml` | FFmpeg freshness | weekly, manual | `ci-failure` issue |
| `ci-failure.yml` | CI failure issue | `workflow_run` of the weekly workflows failing | Blocks the next release (R25) |

vukem's `bench.yml`, `downstream.yml` and `starter-deps.yml` are not copied: no benchmarks, downstream apps or scaffold pins exist here. `fuzz.yml` is vukem's (`fuzz.yml`, weekly, `fuzztime` input) with our targets from SDD-007 section 12 (`FuzzCanonicalizeImportPath`, `FuzzSniffMedia`, `FuzzUntrustedClean`, `FuzzHostCheck`, `FuzzFontHeader`, `FuzzTemplateArchive`, `FuzzFFArgsCheck`); the task that adds a fuzz target adds it to the matrix of `fuzz.yml` and of the `Fuzz (30 s)` job. `ci-failure.yml` lists `Vulnerabilities (weekly)`, `Scorecard`, `Secrets`, `CodeQL`, `Fuzz` and `FFmpeg freshness`.

`ffmpeg-freshness.yml` is new (SDD-007 section 12): a weekly job that compares the FFmpeg release pinned in `.config/ffmpeg.env` with the newest patch release of its branch (`gh api` on the FFmpeg tags), opens an issue when a newer patch exists, and fails when the pin is older than 90 days.

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
          requireScope: false
          subjectPattern: ^[^A-Z].*[^.]$
          subjectPatternError: Description must start lowercase and must not end with a period.
```

Gate: merge. Secrets: none beyond `GITHUB_TOKEN`.

### 9.3 `release.yml` (full)

Adapted from `vukem/.github/workflows/release.yml` (R14 to R24). Changes, with reasons:

1. Step "Install and build the web assets" and `oven-sh/setup-bun`: the binary embeds the React build (RF-200); vukem's binary has no front end.
2. Step "Notices are current": RF-215, NFR-043.
3. Step "Publish the release notes": NFR-044, R6, R7. Placed before GoReleaser so `keep-existing` finds the release with its notes.
4. Provenance bundle file name `vukem-video.intoto.jsonl`; a step that asserts the asset count (15).
5. The `image` job pushes to two registries and signs and attests both digests (R22); a first step fails early and clearly when the Docker Hub secrets are missing (ADR-0025 consequences).
6. The image build context is the local checkout (`context: .`), exactly as vukem, so `.git` is in the context and the binary reports the tag (see section 11).
7. The `image` job builds and smoke tests the amd64 image with `load: true` before the multi-architecture push, so a broken image never reaches `latest`. At `v0.43.0` vukem pushes directly; the build-then-test-then-push order is present on vukem main after v0.43.0 and is copied from there. The smoke test is the script `scripts/image-smoke.sh`, shared with `image.yml` and `task image:smoke` (section 9.5).
8. The hook "assert the web build output" of `.config/goreleaser.yaml` (section 10) fails the release job when the web build produced no asset, so a binary without the UI is never published.

```yaml
name: release

# Builds the vukem-video binaries and the container image when a version tag
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
          cp "$BUNDLE" vukem-video.intoto.jsonl
          gh release upload "$TAG" vukem-video.intoto.jsonl --clobber --repo "$GITHUB_REPOSITORY"
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
      GHCR_IMAGE: ghcr.io/alexgalhardo/vukem-video
      HUB_IMAGE: alexgalhardo/vukem-video
      TAG: ${{ needs.goreleaser.outputs.tag }}
    permissions:
      contents: read
      packages: write # push the image to GHCR
      id-token: write # Sigstore OIDC identity for cosign and attestations
      attestations: write # build provenance attestation of the image
    steps:
      - name: Check the Docker Hub credentials
        env:
          HUB_USER: ${{ secrets.DOCKERHUB_USERNAME }}
          HUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}
        run: |
          if [ -z "$HUB_USER" ] || [ -z "$HUB_TOKEN" ]; then
            echo "::error::add the repository secrets DOCKERHUB_USERNAME and DOCKERHUB_TOKEN (see SDD-009 section 16)"
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
          {
            echo "version=$version"
            echo "revision=$(git rev-parse HEAD)"
            echo "tags<<EOF"
            for image in "$GHCR_IMAGE" "$HUB_IMAGE"; do
              echo "$image:$version"
              echo "$image:${version%.*}"
              echo "$image:latest"
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
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}
      # Build and test the amd64 image locally first (load: true); nothing is
      # pushed, and so latest does not move, until the smoke test passes.
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        with:
          context: .
          file: .config/docker/vukem-video.Dockerfile
          platforms: linux/amd64
          load: true
          tags: vukem-video:release-smoke
          build-args: |
            VERSION=${{ steps.meta.outputs.version }}
            REVISION=${{ steps.meta.outputs.revision }}
      - name: Smoke test the image before publishing
        run: bash scripts/image-smoke.sh vukem-video:release-smoke
      - uses: docker/build-push-action@c3c9e263c25d99ce0380d002d59b67737d91b0dc # v7.4.0
        id: build
        with:
          context: .
          file: .config/docker/vukem-video.Dockerfile
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
          subject-name: ghcr.io/alexgalhardo/vukem-video
          subject-digest: ${{ steps.build.outputs.digest }}
          push-to-registry: true
      - uses: actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8 # v4.2.2
        with:
          subject-name: index.docker.io/alexgalhardo/vukem-video
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

The jobs below are listed by their display name, which is the string the ruleset requires (section 16). The vukem jobs for drivers, contrib modules, `apidiff` and `openapi golden` do not exist here (one module, one application); `apidiff` returns when a Go package is meant to be imported.

| # | Job (display name) | Runs on | Steps, in order, exact commands | Notes |
| --- | --- | --- | --- | --- |
| 1 | `Lint (Go)` | ubuntu | `golangci/golangci-lint-action` with `version: v2.14`; `go vet ./...`; `go fix -diff ./... > fix.diff \|\| true` and a `::warning` when non-empty (vukem `ci.yml:30-33`); the `//nolint:gocognit` budget check of `ci.yml:44-55` with `GOCOGNIT_BUDGET: "0"` | gofumpt, gci, golines, gosec, `forbidigo` (SDD-007 section 10.2) and the depguard import rules of ADR-0001 run inside golangci-lint (section 15.3) |
| 2 | `Lint (Markdown)` | ubuntu | `DavidAnson/markdownlint-cli2-action` with `globs: "**/*.md"` | config `.markdownlint-cli2.jsonc` |
| 3 | `Types and lint (web)` | ubuntu | `oven-sh/setup-bun` `bun-version: 1.4.4`; in `web/`: `bun install --frozen-lockfile`; `bun run typecheck`; `bun run lint`; `bun run test --if-present` | Studio `ci.yml` jobs `types`; `bun run lint` is `biome ci` |
| 4 | `Test (${{ matrix.os }})` | matrix `ubuntu-latest`, `windows-latest`, `macos-latest`, `fail-fast: false` | `bash scripts/install-ffmpeg.sh`; `go test -race -count=1 -timeout=20m -coverprofile=coverage.out ./...` with `shell: bash`; on ubuntu only: `go run ./internal/tools/covergate -v coverage.out` and `actions/upload-artifact` of `coverage.out` (name `coverage`) | `-race` needs cgo, present on all three runners (vukem `ci.yml:134`). The Windows step prints the CPU first as vukem does (`ci.yml:128-130`) because of golang/go#81238. `fetch-depth: 0` is not needed. On Windows `scripts/install-ffmpeg.sh` unpacks the BtbN `.zip` with `tar -xf` (bsdtar ships with Windows) or `powershell -Command Expand-Archive`, because Git Bash has no `unzip` |
| 5 | `Browser tests (Playwright)` | ubuntu | setup-go, setup-bun; `bash scripts/install-ffmpeg.sh`; in `web/`: `bun install --frozen-lockfile`, `bun run build`, `bunx playwright install --with-deps chromium`, `bun run e2e` | Studio `ci.yml` job `e2e` |
| 6 | `Generated files` | ubuntu | setup-go; `actions/setup-node` (pinned, `node-version: 24`, npm cache on `site/package-lock.json`); `go run ./internal/tools/contractgen` (writes `core/schema/` and `web/resources/js/generated/`); `go run ./web/cmd/gen-agent-docs --check` (SDD-003 section 7); `cd site && npm ci && npm run reference`; `git diff --exit-code -- core/schema web/resources/js/generated site/generated site/content/generated` | the generated-schema check of ADR-0014 item 2 and the reference check of ADR-0024 item 5. `npm run reference` runs the four generators with the flags of SDD-010 section 8.2 (`vukem-video command:help --json`, `vukem-video openapi:export -prefix /api/v1`, `go run ./internal/tools/configref -json`, `go run ./internal/tools/contractgen -errors`), so the flags are defined once. Node 24 is needed because `site/scripts` run with Node type stripping. Prints the diff on failure |
| 7 | `Notices` | ubuntu | setup-go, setup-bun; `bun install --frozen-lockfile` in `web/`; `go run ./internal/tools/notices -check` | section 12 |
| 8 | `Vulnerabilities` | ubuntu | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | vukem `ci.yml:166` |
| 9 | `Release build (snapshot)` | ubuntu | setup-go, setup-bun, syft installer; `bun install --frozen-lockfile` in `web/`; `goreleaser/goreleaser-action` with `version: "v2.18.2"` and `args: release --snapshot --clean --skip=sign,publish`; for every archive, `for f in dist/vukem-video_*.tar.gz dist/vukem-video_*.zip; do [ "$(stat -c %s "$f")" -lt 60000000 ] || exit 1; done` | NFR-041: every archive is under 60,000,000 bytes (60 MB, the unit of the requirement). This is the dry run that catches a broken `.config/goreleaser.yaml` before a tag |
| 10 | `Workflow pins` | ubuntu | `actions/checkout`; the pin check of section 13.1 (an `if` block, so a pinned tree passes and an unpinned line fails) | enforces R23 mechanically; required |
| 11 | `Security tests (${{ matrix.os }})` | matrix `ubuntu-latest`, `windows-latest`, `macos-latest`, `fail-fast: false` | setup-go; `bash scripts/install-ffmpeg.sh`; `go test -race -count=1 -v -run TestSecurity_ ./... \| tee security.log`; then the skip check of section 9.4.1 | dedicated job of SDD-007 section 12, in addition to the full suite of job 4; fails on a skipped security test that is not marked with its platform in the name |
| 12 | `Threat coverage` | ubuntu | setup-go; `go run ./internal/tools/threatcheck` | SDD-007 section 12: a threat row without its named test, a `TestSecurity_T..` test no row names, a threat without a mitigation, or an operation missing from the SDD-007 section 5 table fails |
| 13 | `Vulnerabilities (UI)` | ubuntu | setup-bun `1.4.4`; in `web/`: `bun audit --audit-level=high` | SDD-007 section 12; high or critical fails. Also runs weekly in `vuln.yml` |
| 14 | `Dependency review` | ubuntu, `pull_request` only | `actions/dependency-review-action` (SHA of table 9.0) with `fail-on-severity: high` and `deny-licenses: GPL-2.0-only, GPL-2.0-or-later, GPL-3.0-only, GPL-3.0-or-later, AGPL-3.0-only, AGPL-3.0-or-later` | SDD-007 section 12 and NFR-043: a new dependency with a high or critical advisory or a GPL-family licence fails |
| 15 | `Fuzz (30 s)` | ubuntu | setup-go; `bash scripts/fuzz-changed.sh 30s` | runs each target of SDD-007 section 12 whose package changed in the pull request for 30 seconds with `go test -run ^$ -fuzz <Target> -fuzztime 30s`; passes with no target when none changed. The long weekly run is `fuzz.yml` (section 9.1) |
| 16 | `No egress` | ubuntu | setup-go; `bash scripts/install-ffmpeg.sh`; `sudo apt-get install -y strace`; `bash scripts/no-egress.sh` | SDD-007 section 10.2 item 3: the real binary and the real FFmpeg run the scenario under `strace -f -e trace=connect,sendto,sendmsg`; any `connect` to an address that is not loopback or a Unix socket fails |
| 17 | `Sandbox (bwrap)` | ubuntu | setup-go; `sudo apt-get install -y bubblewrap`; `go test -count=1 -run TestSecurity_T16_BwrapHasNoNetwork ./...` | SDD-007 section 12. If the runner restricts unprivileged user namespaces, the job first sets `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` |
| 18 | `Contract and parity` | ubuntu | setup-go; `bash scripts/install-ffmpeg.sh`; `go test -count=1 ./web/contract/...` | the `contract` job of SDD-003 section 7 (check 26): the contract scenarios on the six drivers, `TestSchemasMatch`, `TestSummariesMatch` (NFR-025, RF-104) |
| 19 | `OpenAPI (Redocly)` | ubuntu | setup-go; `go run ./web/cmd/app openapi:export -prefix /api/v1 > "$RUNNER_TEMP/openapi.json"`; `bunx @redocly/cli@2.78.0 lint "$RUNNER_TEMP/openapi.json"` | SDD-003 check 22: the OpenAPI document is valid 3.1. The Redocly version is the newest stable on 2026-11-14; `REL-CI` re-reads it |
| 20 | `Golden cases (PATH FFmpeg)` | ubuntu | setup-go; `sudo apt-get install -y ffmpeg`; `VUKEM_VIDEO_FFMPEG=$(command -v ffmpeg) go test -count=1 -run Golden ./engine/export/...` | SDD-004 section 11: the golden cases run with the FFmpeg found on the path, PSNR at least 50 dB per frame against the references; `continue-on-error: true`, reported and not blocking. The distribution package is used for this test only and is never shipped (NFR-043) |

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
          file: .config/docker/vukem-video.Dockerfile
          load: true
          tags: vukem-video:ci
          build-args: |
            VERSION=0.0.0-ci
      - name: Smoke test
        run: bash scripts/image-smoke.sh vukem-video:ci
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
docker run --rm --entrypoint sh "$img" -c "test -s /usr/share/doc/vukem-video/THIRD_PARTY_NOTICES.md && test -s /usr/share/doc/vukem-video/ffmpeg/LICENSE.txt && test -s /usr/share/doc/vukem-video/ffmpeg/configure.txt && test -s /usr/share/doc/vukem-video/ffmpeg/SOURCE.txt"
docker image inspect --format "{{json .Config.Healthcheck.Test}}" "$img" | grep -q 7878
```

The smoke test is the executable form of NFR-043 for the image: the FFmpeg inside has no `--enable-gpl` and no `--enable-nonfree`, can encode H.264 with the encoder that works in a container (OpenH264), the notices are present and the health check is declared. Because it runs `ffmpeg -version` and the reference export inside the image on the Debian runtime, it also verifies that the BtbN build runs there; proving that is the first subtask of `REL-DOCKER` (section 11.1).

### 9.6 `site.yml` and `docs-last-updated.yml`

`site.yml` is vukem's (`site.yml:1-75`) with these changes, specified in detail in SDD-010 section 12: the added steps `npm run reference:check`, `npm run check:built`, `npm run check:translations`; `SITE_URL` from the repository variable; `actions/upload-pages-artifact` path `site/out`. Trigger: push to `main` and pull request touching `site/**`, `CHANGELOG.md`, `core/schema/**`, `.github/workflows/site.yml`; manual. The step lists are identical in SDD-010 section 12.2: job `build` (`actions/checkout`, `actions/setup-go` with `go-version-file: go.mod` because `reference:check` builds the binary, `actions/setup-node` with Node 24 and the npm cache on `site/package-lock.json`, `npm ci`, `npm run test:site` (which includes the base path test, `test:base-path`), `npm run lint`, `npm run check:mdx`, `npm run reference:check`, `npm run build`, `npm run check:built`, upload artifact on `main` or manual), job `last-updated` (`fetch-depth: 0`, Node 24, `npm run test:scripts`, `npm run check:last-updated`, `npm run check:translations`), job `deploy` (only on `main` or manual, needs both, `environment: github-pages`, `actions/deploy-pages`). Permissions as vukem: `contents: read`; `build` also `actions: write` and `pages: write`; `deploy` `pages: write`, `id-token: write`. Gate: the deploy.

`docs-last-updated.yml` is vukem's (`docs-last-updated.yml:1-34`) unchanged except the name of the job: `Docs last updated` (so it can be a required check without clashing with the job `Last updated` of `site.yml`). It runs on every pull request so that a missing date fails even when `site/` is untouched.

### 9.7 `secrets.yml`, `codeql.yml`, `vuln.yml`, `scorecard.yml`, `ci-failure.yml`

| File | Content |
| --- | --- |
| `secrets.yml` | Copied from vukem (`secrets.yml:1-51`): Betterleaks `1.9.0` downloaded by exact URL and checked against `f8b185a39ffcece2a1ca82bf3a4e7435cd81963ffd16b7a9128daf75f35f6de7`, scanning `$PR_BASE..$HEAD_SHA` on pull requests, `$PUSH_BEFORE..$HEAD_SHA` on pushes, the full history weekly (cron `41 5 * * 1`). Job display name `Betterleaks`. Config `.betterleaks.toml` (moved from `web/`, section 17), extended with the rules of SDD-007 section 12: the key prefixes `vvl_` and `vvk_`, the `APP_KEY` value and the transcription key. The job `secrets` of the Studio's `ci.yml` is removed: it duplicates this workflow |
| `codeql.yml` | Copied from vukem: `languages: go`, `build-mode: autobuild`, push to `main`, pull request, cron `17 4 * * 1`; `security-events: write` on the job. JavaScript is added with the second matrix entry `languages: javascript-typescript` when the UI exists (task `REL-CI`) |
| `vuln.yml` | Reduced from vukem (`vuln.yml`): weekly (cron `17 6 * * 1`) and manual `govulncheck@v1.9.0 ./...` plus `bun audit --audit-level=high` in `web/`, no module matrix because there is one module |
| `scorecard.yml` | Copied from vukem (`scorecard.yml:1-41`): branch protection rule, push to `main`, cron `23 5 * * 1`, manual; `ossf/scorecard-action` with `publish_results: true`; upload SARIF with `actions/upload-artifact` and `github/codeql-action/upload-sarif`. The job must stay minimal (vukem's comment, lines 3-4 of the file) |
| `ci-failure.yml` | Copied from vukem: on `workflow_run` completed with `failure` of `Vulnerabilities (weekly)`, `Scorecard`, `Secrets`, `CodeQL`, `Fuzz`, `FFmpeg freshness`; opens or comments on an issue labelled `ci-failure` and `priority:p1` with `gh`. `issues: write`. The labels exist (section 16.3). An open issue blocks the next release (R25) |

## 10. GoReleaser

`.config/goreleaser.yaml`, adapted from `vukem/.config/goreleaser.yaml` (lines 1-53). Changes: names; `main: ./web/cmd/app`; a `before` hook that installs and builds the web assets, because the binary embeds them (RF-200); the archive file list adds the notices (RF-215).

```yaml
# GoReleaser builds the vukem-video binaries for a tag and attaches them to
# the GitHub release that release.yml created from CHANGELOG.md (so GoReleaser
# keeps the notes). See docs/sdds/009-releases-and-ci-cd.md.
version: 2

project_name: vukem-video

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
  - id: vukem-video
    main: ./web/cmd/app
    binary: vukem-video
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    flags: [-trimpath]
    ldflags: ["-s -w"]
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - name_template: "vukem-video_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
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
        dst: agent/skills/vukem-video
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

- Six targets: linux, darwin and windows, each on amd64 and arm64. Assets of a release: six archives named `vukem-video_X.Y.Z_<os>_<arch>.tar.gz` (`.zip` for Windows), six `<archive>.sbom.json`, `checksums.txt`, `checksums.txt.sigstore.json`, `vukem-video.intoto.jsonl`: 15 files, the same count as vukem's `v0.43.0`.
- The archives contain no FFmpeg (NFR-041, under 60 MB). The binary finds FFmpeg on the path or at `VUKEM_VIDEO_FFMPEG` (RF-204). `THIRD_PARTY_NOTICES.md` still lists FFmpeg, stating that it is used when present and shipped only in the image.
- The binary must be built after `bun run build` because `web/` embeds `web/resources/public` with `go:embed` (the current directive is `//go:embed views all:public lang` in `web/resources/resources.go`; SDD-012 owns the final one). A build without the web build does **not** fail to compile: `public/robots.txt` is tracked, so the directive matches and the binary silently ships without the UI. That is why the hook `scripts/assert-web-build.sh` runs after the web build and before the compile: it fails when `find web/resources/public/build -type f -name "*.js"` finds nothing. The Dockerfile asserts the same after its web build stage (section 11.3).

## 11. The container image

### 11.1 Decisions

- The Go binary serves HTTP and the UI itself (RF-200), so **no Caddy or any other web server is needed inside the image**. The author's Caddy rule is for images that serve static files or proxy; this one does neither. A reverse proxy for TLS is the operator's concern and is documented in the self-hosting guide (SDD-010).
- Base images, all with fixed version tags: `oven/bun:1.4.4-alpine` (asset build stage only, same as the Studio's `Dockerfile.app`; it never reaches the runtime image, so the Alpine in this stage is not a runtime base), `golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d` (build, the Debian image vukem pins by digest, `.config/docker/vukem.Dockerfile:11`; it has `git`, which the version stamp needs), `debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2` (runtime; tag and digest read from Docker Hub on 2026-11-14, `REL-DOCKER` re-reads them and records the values it pins).
- The runtime base is a fixed-tag `debian:13-slim` pinned by digest. This reverses the first draft, which chose Alpine on the claim that the BtbN builds are static and run on musl: the BtbN Linux builds are linked against glibc, so they do not run on Alpine. Debian is also what vukem uses (`.config/docker/vukem.Dockerfile:9-10`). The first subtask of `REL-DOCKER` is a smoke test that runs `ffmpeg -version` and the reference export inside the image on the Debian runtime, before any other work on the image; if it fails the task stops and reports. No Alpine image is the runtime anywhere in this document.
- FFmpeg is the BtbN `lgpl` variant on a release branch, downloaded by exact URL and verified by SHA-256 (ADR-0011 item 2). Its single source of truth is `.config/ffmpeg.env`, sourced by the Dockerfile and by `scripts/install-ffmpeg.sh` (CI). The Dockerfile copies the file into the build and fails if a value is missing or a hash does not match.
- The product bundles and links no GPL component: no `x264`, `x265`, no `libfdk_aac`, no Debian `ffmpeg` package (`docs/search/licensing.md` section 3), and the smoke test greps the configure line for `--enable-gpl` and `--enable-nonfree`. The base image is an operating system: its packages (bash, coreutils, dpkg and others) carry GPL licences and are not part of the product. They are listed with their licences: the Dockerfile writes the installed package list (`dpkg-query -W`) to `/usr/share/doc/vukem-video/base-image-packages.txt`, the licence of each package is the `copyright` file Debian keeps in `/usr/share/doc/<package>/` of the slim image, and `THIRD_PARTY_NOTICES.md` has a group "Base image packages (Debian)" that says so. The architect rewords NFR-043 to "no GPL component is bundled or linked by the product; base image packages are listed with their licences". A base with no GPL userland is not required.
- Non-root user `app`, uid 10001 (the Studio's `Dockerfile.app` choice), a `/data` volume as the workspace (SDD-000 section 9).
- The version stamp follows vukem: `.git` travels in the build context (`context: .`) and the build fails when `vukem-video version` does not print the tag (`.config/docker/vukem.Dockerfile:20-27`). Friction inherited: it does not work from a `git worktree`, whose `.git` is a file pointing outside the context.
- The build context must be clean, or Go stamps `+dirty` and the version check fails at the first real release. The `.dockerignore` therefore lists only untracked or generated paths (`**/node_modules`, `**/dist`, `.env`, `.env.*`, build outputs), as vukem's does, and never a tracked path such as `site/`, `docs/`, `spikes/` or `desktop/`. `image.yml` passes a dummy `VERSION` and a local dummy tag and runs the same version check on every pull request (section 9.5), so a violation is caught before a release.

### 11.2 `.config/ffmpeg.env`

```bash
# The one pin of the FFmpeg that Vukem Video distributes (ADR-0011).
# Sourced by .config/docker/vukem-video.Dockerfile and scripts/install-ffmpeg.sh.
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

### 11.3 `.config/docker/vukem-video.Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1

# The Vukem Video image: the vukem-video binary (server, CLI, MCP server and the
# UI assets) and a pinned LGPL FFmpeg. Published to ghcr.io/alexgalhardo/vukem-video
# and alexgalhardo/vukem-video (Docker Hub) by release.yml; see
# docs/sdds/009-releases-and-ci-cd.md. Build from the repository root:
#
#   docker build -f .config/docker/vukem-video.Dockerfile -t vukem-video .
#
# No web server runs here: the Go binary serves HTTP. The product bundles and
# links no GPL component: .config/ffmpeg.env pins the BtbN lgpl build (ADR-0011);
# the packages of the Debian base are listed with their licences in the notices.
ARG BUN_IMAGE=oven/bun:1.4.4-alpine
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
# The .git directory travels in the context so the tag at HEAD becomes the
# version `vukem-video version` prints (Go stamps it from VCS). A release
# passes VERSION and the build fails if the stamped version does not match.
# Build from a clone, not from a git worktree: its .git file points outside
# the context.
RUN git config --global --add safe.directory /src \
	&& GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -ldflags="-s -w" -o /out/vukem-video ./web/cmd/app \
	&& if [ "$VERSION" != devel ] && [ "$TARGETARCH" = "$(go env GOHOSTARCH)" ]; then [ "$(/out/vukem-video version)" = "vukem-video v$VERSION" ]; fi

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

LABEL org.opencontainers.image.title="vukem-video" \
	org.opencontainers.image.description="Vukem Video: a minimal video editor operated by AI agents and by a human UI" \
	org.opencontainers.image.source="https://github.com/AlexGalhardo/vukem-video" \
	org.opencontainers.image.url="https://github.com/AlexGalhardo/vukem-video" \
	org.opencontainers.image.documentation="https://github.com/AlexGalhardo/vukem-video#readme" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${REVISION}"

# curl is the health check client; tzdata and ca-certificates are the runtime
# data the binary needs. Debian slim keeps /usr/share/doc/<package>/copyright.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl tzdata \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --system --no-create-home --uid 10001 --user-group app \
	&& mkdir -p /data /usr/share/doc/vukem-video/ffmpeg \
	&& chown app:app /data \
	&& dpkg-query -W > /usr/share/doc/vukem-video/base-image-packages.txt

COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/LICENSE.txt /usr/share/doc/vukem-video/ffmpeg/LICENSE.txt
COPY .config/ffmpeg.env /tmp/ffmpeg.env
# The licence obligations of the LGPL build (ADR-0011 item 2): the licence text,
# the configure line and where the corresponding source is.
RUN . /tmp/ffmpeg.env \
	&& /opt/ffmpeg/bin/ffmpeg -version | sed -n "s/^configuration: //p" > /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& test -s /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& ! grep -q -e "--enable-gpl" -e "--enable-nonfree" /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& printf '%s\n%s\n%s\n' \
		"FFmpeg ${FFMPEG_VERSION} (LGPL build) from https://github.com/BtbN/FFmpeg-Builds/releases/tag/${FFMPEG_RELEASE_TAG}" \
		"Build scripts: https://github.com/BtbN/FFmpeg-Builds/tree/${FFMPEG_BUILDS_COMMIT}" \
		"Corresponding FFmpeg source: ${FFMPEG_SOURCE_URL}" > /usr/share/doc/vukem-video/ffmpeg/SOURCE.txt \
	&& rm /tmp/ffmpeg.env

COPY LICENSE.md THIRD_PARTY_NOTICES.md /usr/share/doc/vukem-video/
COPY licenses/ /usr/share/doc/vukem-video/licenses/
COPY --from=build /out/vukem-video /usr/local/bin/vukem-video

# The fonts (Inter, Montserrat, Anton, Bebas Neue, Playfair Display, JetBrains
# Mono, D-022) are embedded in the binary (engine/raster/fonts, go:embed), so
# the image needs no font files; their licences are in licenses/fonts/ above.
#
# A published port needs a non-loopback bind, and a non-loopback bind requires
# network mode (SDD-000 section 10), where login is mandatory. The first
# administrator is created as SDD-007 describes.
ENV VUKEM_VIDEO_HOME=/data \
	VUKEM_VIDEO_ADDR=0.0.0.0:7878 \
	VUKEM_VIDEO_MODE=network \
	VUKEM_VIDEO_FFMPEG=/opt/ffmpeg/bin/ffmpeg \
	VUKEM_VIDEO_FFPROBE=/opt/ffmpeg/bin/ffprobe \
	PATH=/opt/ffmpeg/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
VOLUME ["/data"]
USER app
WORKDIR /data
EXPOSE 7878
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
	CMD curl -fsS http://127.0.0.1:7878/up || exit 1
ENTRYPOINT ["vukem-video"]
CMD ["serve"]
```

Notes on the file:

- Every `FROM` line resolves to a fixed tag (or a digest plus tag); there is no `latest`.
- In the `build` stage the version check runs only when the build machine and the target architecture are the same (the arm64 binary cannot run on the amd64 builder). The amd64 build is checked and the arm64 build uses the same source; this is a limitation of cross building without emulation and is accepted.
- The `ffmpeg` stage runs on the build platform because it only downloads and unpacks; the `RUN ffmpeg -version` in the last stage runs on the target platform (QEMU for arm64, `docker/setup-qemu-action`).
- `ENV` follows SDD-000 section 10 names exactly. `VUKEM_VIDEO_MODE=network` is forced by the bind address rule of that section, so a container started as `docker run -p 7878:7878 -v vukem-video:/data ghcr.io/alexgalhardo/vukem-video:X.Y.Z` always requires login. A user who wants local mode without login runs the binary on the host.
- `HEALTHCHECK` calls `GET /up` on `127.0.0.1:7878`, the vukem health probe that SDD-000 section 1 lists among the routes that are not service operations. `curl` is installed in the runtime stage only for this. SDD-007 §3.7 item 6 states the rule: `/up` needs no credential, and a request from a loopback peer is exempt from the `Host` check, which is what the health check needs in network mode.
- `VOLUME /data` plus `chown app:app /data` makes the workspace writable by uid 10001 on a named volume. A bind mount must be writable by uid 10001; the self-hosting guide says so.

### 11.4 `.config/docker/vukem-video.Dockerfile.dockerignore`

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

## 12. Third-party notices

RF-215 and NFR-043 require notices for everything shipped, and ADR-0011 requires the FFmpeg licence text, configure line and source link in every artifact that carries FFmpeg.

### 12.1 Files

| File | Content | Generated | Shipped in |
| --- | --- | --- | --- |
| `LICENSE.md` | MIT, copyright of the author | no | archive, image, repository |
| `THIRD_PARTY_NOTICES.md` | One table per group (Go modules, npm packages, fonts, external programs), each row: name, version, SPDX licence expression, copyright line, path of the licence text under `licenses/` | yes | archive, image, repository |
| `licenses/<group>/<name>/<file>` | The verbatim licence and `NOTICE` files of each component | yes (copied) | archive, image, repository |
| `.config/third-party.json` | Hand-maintained inventory of components that no package manager lists: FFmpeg, the fonts, whisper.cpp and its models, OpenH264 | no | repository (input) |
| `/usr/share/doc/vukem-video/ffmpeg/{LICENSE.txt,configure.txt,SOURCE.txt}` | FFmpeg licence text, the configure line as printed by the binary, the source locations | by the Dockerfile (section 11.3) | image only |
| `/usr/share/doc/vukem-video/base-image-packages.txt` | the packages of the Debian runtime base (`dpkg-query -W`); each package licence is the `copyright` file in `/usr/share/doc/<package>/` of the image. `THIRD_PARTY_NOTICES.md` has a group "Base image packages (Debian)" that states this; the product bundles and links no GPL component, and these packages are an operating system, not part of the product | by the Dockerfile (section 11.3) | image only |

### 12.2 The generator

`internal/tools/notices` (Go, standard library only; new, not in vukem; vukem has `internal/tools/*` tools of the same kind, for example `configref` with its `-check`, `docs/cli.md`). It must not add a third-party Go dependency (rule: no new dependency without asking).

Inputs and algorithm:

1. **Go modules.** `go list -deps -f '{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}' ./web/cmd/app`, deduplicated, excluding the main module. For each, find the licence files in `{{.Dir}}` by name (`LICENSE*`, `LICENCE*`, `COPYING*`, `NOTICE*`, case-insensitive) and detect the SPDX id by matching the licence text against a built-in table of the allowed licences. No match fails the run.
2. **npm packages.** The production dependency closure of `web/package.json`, read from `web/node_modules` after `bun install --frozen-lockfile` (only `dependencies`, transitively, never `devDependencies`). The licence is the `license` field of each `package.json`, and the licence files are found as for Go modules.
3. **Inventory.** `.config/third-party.json`, an array of `{name, version, spdx, copyright, files[], note, shipped_in[]}`. It contains at least: the FFmpeg LGPL build (`LGPL-3.0-or-later`, version from `.config/ffmpeg.env`, `shipped_in: ["image"]`, note that archives do not carry it), the six fonts of D-022 with the licence file each ships with (`OFL-1.1`; the task that bundles a font verifies the licence, D-022), whisper.cpp (`MIT`, `shipped_in: []`, downloaded on first use, ADR-0021) and the Whisper model (`MIT`, same), OpenH264 (`BSD-2-Clause`, inside the BtbN build; note about the Cisco binary, D-017).
4. **Allow-list.** `MIT`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `Apache-2.0`, `MPL-2.0`, `0BSD`, `Unlicense`, `OFL-1.1` (fonts only), `LGPL-3.0-or-later` (FFmpeg only, a separate process). Anything else, and any GPL, AGPL, SSPL, "Commons Clause" or unknown id, fails the run with the module name (NFR-043). MPL-2.0 rows carry the note "file-level copyleft: publish changes to its files" (`docs/search/licensing.md` section 7).
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
| Tests with the race detector on three operating systems | `go test -race -count=1 -timeout=20m -coverprofile=coverage.out ./...` | `Test (ubuntu-latest)`, `Test (windows-latest)`, `Test (macos-latest)` |
| Coverage gate | `go run ./internal/tools/covergate -v coverage.out` (ubuntu only) | `Test (ubuntu-latest)` |
| Front-end types | `bun run typecheck` in `web/` | `Types and lint (web)` |
| Front-end lint | `bun run lint` in `web/` (`biome ci`) | `Types and lint (web)` |
| Playwright end to end | `bun run e2e` in `web/` | `Browser tests (Playwright)` |
| Markdown lint | `markdownlint-cli2 "**/*.md"` (CI: the pinned action; local: `bunx markdownlint-cli2@0.24.1`) | `Lint (Markdown)` |
| Secret scan | `betterleaks git . --config .betterleaks.toml --no-banner --redact --log-opts=<range>` (before the move of section 17 the config is `web/.betterleaks.toml`) | `Secrets / Betterleaks` |
| govulncheck | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | `Vulnerabilities` |
| Generated-schema check (ADR-0014) and generated reference (ADR-0024) | regenerate, then `git diff --exit-code` over the generated paths (section 9.4 job 6) | `Generated files` |
| Third-party notices | `go run ./internal/tools/notices -check` | `Notices` |
| Release build dry run and archive size | GoReleaser snapshot, every archive under 60,000,000 bytes | `Release build (snapshot)` |
| Image build and smoke test | section 9.5 | `image / Image build` (path filtered) |
| Site build, MDX, links, markdown twin, last updated, translations report, reference, built-output checks | SDD-010 section 12 | `Site` (path filtered), `Docs last updated` (always) |
| Every action pinned by SHA | `scorecard.yml` and the job `Workflow pins` (below) | `Workflow pins` |
| Security tests on three operating systems | `go test -race -count=1 -v -run TestSecurity_ ./...` and the skip check of section 9.4.1 | `Security tests (ubuntu-latest)`, `Security tests (windows-latest)`, `Security tests (macos-latest)` |
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

`Workflow pins`: a small job in `ci.yml` (ubuntu, a checkout of the sources) that runs the block below and fails when any unpinned line is printed; `docker://` and local `./` actions are not used. This enforces R23 mechanically; vukem relies on review and Scorecard (`docs/tooling.md:151`, `scorecard.yml`). It is a required check. The check is an `if` block because a bare `grep ... | grep -v ...` pipeline exits 0 when it finds an unpinned line, which is the inverse of what a check needs:

```bash
if grep -RnE "uses: [^@ ]+@" .github/workflows | grep -vE "@[0-9a-f]{40} # v[0-9]"; then
  echo "::error::an action is not pinned by a full commit SHA with a version comment"
  exit 1
fi
```

### 13.2 Coverage gate

`internal/tools/covergate` is vukem's tool (`internal/tools/covergate/gate.go`) copied with the package table replaced. Decision of this SDD (ADR-0025 names the gate and no number): the default floor is 80 percent per package, the packages under `core/` have a floor of 90 percent (they are pure and cheap to cover, ADR-0001), and excluded from the gate are `web/cmd/...`, `internal/tools/...`, `spikes/...` and `desktop/...`. A package missing from the profile fails the gate, as in vukem, so a package without tests cannot hide. Floors may be raised by a pull request that raises them and lowered only by a revision of this SDD.

### 13.3 Operating systems

| OS | Runner | Notes |
| --- | --- | --- |
| Linux | `ubuntu-latest` | FFmpeg: pinned BtbN LGPL via `scripts/install-ffmpeg.sh` |
| Windows | `windows-latest` (Windows Server 2025) | FFmpeg: pinned BtbN LGPL zip via the same script under Git Bash. GitHub offers no hosted x64 Windows 11 runner; CI therefore runs Windows Server; NFR-045 names Windows 11, so the orchestrator runs `task ci` on its Windows 11 machine before each release (a line of the `task release:preflight` checklist) |
| macOS | `macos-latest` | BtbN has no macOS build. The script installs `ffmpeg` with Homebrew for tests only; it is never distributed. Reference tests compare composed frames before encoding (ADR-0011 item 7), so the encoder difference does not matter |

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
| `Test (windows-latest)` | 18 minutes | slowest; vukem's slow Windows runner needs about 7 minutes for a bigger suite (`ci.yml:133`) |

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
| `task image:build` (new) | `docker build -f .config/docker/vukem-video.Dockerfile -t vukem-video:local .` | |
| `task image:smoke` (new) | `bash scripts/image-smoke.sh vukem-video:local` (section 9.5) | |
| `task site:dev`, `task site:build` | in `site/`: `npm run dev`; `npm ci`, `npm run test:site`, `npm run lint`, `npm run check:mdx`, `npm run reference:check`, `npm run build`, `npm run check:built` (the build steps of SDD-010 section 12.2) | 167-181 |
| `task site:reference` (new) | `npm run reference` in `site/` (SDD-010 section 8) | |
| `task site:translations` (new) | `npm run check:translations` in `site/` (SDD-010 section 6) | || `task site:dates` (new) | `npm run last-updated` in `site/` | |
| `task deps:web`, `task deps:images` (new) | `bash scripts/bump-web-deps.sh` (only when Dependabot has no `bun` ecosystem) and `bash scripts/bump-image-args.sh` (section 16.4) | |
| `task changelog` | `git cliff --config .config/cliff.toml --unreleased --strip all` (preview, writes nothing) | 183-186 |
| `task labels:sync` | `go run ./internal/tools/labelsync .github/labels.txt` (needs `gh auth`; vukem's tool copied) | 108-111 |
| `task release:preflight` (new) | step 3 of section 7: `ci:full` without services, `notices:check`, `generate:check`, `site:build`, `site:translations`, `image:smoke` | adapted from `release`, 193-197 |
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

- `gofumpt.module-path`, `gci` `prefix(...)` and `run.relative-path-mode: gomod`: `github.com/AlexGalhardo/vukem-video` (the vukem prefix `github.com/AlexGalhardo/vukem` is also a gci section so framework imports group together).
- `formatters.exclusions.paths` and `linters.exclusions.paths`: `site/`, `testdata/`, `spikes/`, `web/resources/` (not Go).
- The vukem `depguard` rules about drivers and contrib are replaced by the import rules of ADR-0001:

```yaml
    depguard:
      rules:
        core-is-pure:
          list-mode: lax
          files: ["**/core/**"]
          deny:
            - pkg: github.com/AlexGalhardo/vukem-video/engine
              desc: core never imports engine (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/vukem-video/service
              desc: core never imports service (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/vukem-video/web
              desc: core never imports web (ADR-0001 rule 1)
            - pkg: github.com/AlexGalhardo/vukem
              desc: core never imports vukem (ADR-0001 rule 1)
        engine-is-not-web:
          list-mode: lax
          files: ["**/engine/**"]
          deny:
            - pkg: github.com/AlexGalhardo/vukem-video/service
              desc: engine never imports service (ADR-0001 rule 2)
            - pkg: github.com/AlexGalhardo/vukem-video/web
              desc: engine never imports web (ADR-0001 rule 2)
        service-is-not-vukem:
          list-mode: lax
          files: ["**/service/**"]
          deny:
            - pkg: github.com/AlexGalhardo/vukem-video/web
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

The path-filtered checks (`Site`, `Image build`) are not required, because a required check that never runs blocks the merge forever (vukem leaves its path-filtered `Site` and `image` checks out of its ruleset for the same reason). Apply it with `gh api -X PUT repos/AlexGalhardo/vukem-video/rulesets/<id> --input .config/ruleset-protect-main.json` (the id is read with `gh api repos/AlexGalhardo/vukem-video/rulesets`); only the author runs this, because it is a settings change.

Other repository settings, set by the author once (vukem `docs/tooling.md:180`, Scorecard checks): Dependabot alerts and security updates on; the dependency graph on (the `Dependency review` check needs it); private vulnerability reporting on; GitHub Pages source "GitHub Actions"; default branch `main`; "Automatically delete head branches" on; merge commits off; **"Default commit message for squash merges: Pull request title and description"** (without it the squash commit keeps the commit message of a single-commit pull request, and the title the changelog reads is lost); the Docker Hub secrets of section 16.2; and, once after the first image release, the GHCR package `vukem-video` set to public (section 7 step 11). The footers `BREAKING CHANGE:`, `Security:` and `Translation:` are written in the pull request body, which becomes the squash commit message (section 6.3, SDD-010 section 6.2).

Section 18 step 8 removes the required status checks from this ruleset, by the author, for the first pull request only, and `REL-REPO` restores them with the job names above in the pull request that adds the new workflows.

### 16.2 Secrets and permissions

| Secret | Used by | Who creates it |
| --- | --- | --- |
| `GITHUB_TOKEN` | all workflows | automatic |
| `DOCKERHUB_USERNAME` | `release.yml` job `image` | the author, repository secret, value `alexgalhardo` |
| `DOCKERHUB_TOKEN` | `release.yml` job `image` | the author: a Docker Hub access token with read and write scope on `alexgalhardo/vukem-video`, stored as a repository secret. It is the one manual prerequisite named in ADR-0025 |
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
```

### 16.4 Dependabot, templates and ownership

`.github/dependabot.yml`, adapted from vukem's (`dependabot.yml`): weekly; `gomod` at `/` grouped as `go-deps`, prefix `build`, `include: scope`; `github-actions` at `/` grouped as `actions`, prefix `ci`; `docker` at `/.config/docker` prefix `build`; `npm` at `/site` grouped `site-deps`; and for `web/`, which has only `bun.lock` and no `package-lock.json` (an `npm` entry would not update the lockfile, and `bun install --frozen-lockfile` would then fail on every such pull request), the `bun` ecosystem grouped `web-deps` if GitHub supports `package-ecosystem: bun` at the time of `REL-REPO` (the task checks the Dependabot documentation and records the answer), else a weekly script `scripts/bump-web-deps.sh` run by the maintainer (`task deps:web`) that runs `bun update` in `web/`, `task notices` and opens the pull request. Both web and site entries use prefix `build` and `include: scope`. The commit messages are then valid Conventional Commits with the scope `deps` for runtime dependencies and `deps-dev` for development dependencies, both in the list of section 6.2.

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
- [ ] `Security:` and `Translation: not-needed` footers, when they apply, are in this pull request body (it becomes the squash commit message)
```

## 17. Files that move from `web/` to the root

Paths are relative to the repository root `video/`. "Today" is the layout after the pure move of section 18 (everything of the old tree under `web/`). Actions: **move** (git mv, content kept), **move and edit**, **replace** (new content from another source; the old file is deleted in the commit that adds the replacement), **delete** (removed by the deletion commit of section 6.4). The application paths under `web/` that are not config or governance files are not in this table: sections 17.1 to 17.3 list them exactly, from the real tree of `C:\dev\vukem-video\video\web\` read on 2026-11-14 (dependency and runtime folders `node_modules/`, `storage/` content and `.vukem/` excluded, as they are git-ignored).

| Path today | Path after | Action | Reason |
| --- | --- | --- | --- |
| `web/.agents/` | none | delete | D-021: 3,177 files in Portuguese, Studio skills |
| `web/.betterleaks.toml` | `.betterleaks.toml` | move and edit | secret scan config; Studio-specific allowlists reviewed; keep fingerprints out; the rules for `vvl_`, `vvk_`, `APP_KEY` and the transcription key are added (SDD-007 section 12). Until this move the scan of section 18 step 7 reads it at `web/.betterleaks.toml` |
| `web/.dockerignore` | `.config/docker/vukem-video.Dockerfile.dockerignore` | replace | section 11.4 |
| `web/.editorconfig` | `.editorconfig` | replace | section 15.2 |
| `web/.env.example` | none | delete | Studio variables (Google sign-in, Railway); workspace configuration is `VUKEM_VIDEO_*` (SDD-000 section 10) |
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
| `web/Dockerfile.app` | `.config/docker/vukem-video.Dockerfile` | replace | ideas kept (Bun then Go then small non-root runtime); the distribution `ffmpeg` package (a GPL build) removed (ADR-0011); the runtime is Debian slim (section 11.1) |
| `web/LICENSE.md` | `LICENSE.md` | move | MIT, unchanged |
| `web/README.md` | `README.md` | replace | describes the editor |
| `web/ROADMAP.md`, `web/prompt.md` | none | delete | D-021; the new `ROADMAP.md` is not part of this repository: it lives in the workspace root (D-025) |
| `web/cliff.toml` | `.config/cliff.toml` | replace | section 5.2 (vukem's location, `.config/`, as ADR-0001 rule 6 says) |
| `web/docs/` | none | delete | D-021, Studio research; this is the folder that collides with the new `docs/` (section 18) |
| `web/films/` | none | delete | D-021 |
| `web/scripts/film/` | none | delete | Studio film scripts; the new `scripts/` holds `refresh-last-updated.sh`, `install-ffmpeg.sh` |
| `web/site/` | `site/` | replace | SDD-010 section 1: the new site is copied from the current vukem `site/`, not moved from the Studio fork |
| `web/go.mod`, `web/go.sum` | `go.mod`, `go.sum` | move and edit | module path becomes `github.com/AlexGalhardo/vukem-video` (ADR-0001); `go mod tidy` |
| `web/package.json`, `web/bun.lock` | same | kept (17.2) and edited | `name` becomes `vukem-video`, `version` `0.0.0` (section 4); the `check` script (`bun --cwd site run check`, the Studio site) is removed |

New files at the root created by the tasks of section 19: `Taskfile.yml`, `.golangci.yml`, `AGENTS.md` (the public file for contributors; the working `AGENTS.md`, `ROADMAP.md`, `STATUS.md` and `vukem-feedback.md` stay in the workspace root, outside the repository, D-025), `CLAUDE.md`, `THIRD_PARTY_NOTICES.md`, `licenses/`, `scripts/`, `internal/tools/`, `core/`, `engine/`, `service/`, `desktop/`.

What P1-REL-002 did with the table (the code differs from the rows above in these points, and this paragraph is the contract):

- **Moved with `git mv`** (history follows): `.betterleaks.toml`, `.gitattributes` (merged with vukem's, with `*.cmd` and a `.githooks/*` eol rule added), `.githooks/pre-push`, `CHANGELOG.md`, `LICENSE.md`, `.github/{CODE_OF_CONDUCT,CONTRIBUTING,SECURITY}.md`. `web/.gitignore` was cut down to the Vukem app and web build lines; the root `.gitignore` already held the rest (step 7 of section 18) and needed no change. `web/package.json` is renamed `vukem-video` 0.0.0 and loses the `check` script.
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
| `web/resources/js/components/ui/` (13 shadcn files) | kept as is (Biome-excluded) |
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

Situation (D-005, D-020): `C:\dev\vukem-video\video\` is a git repository with no commits and the remote `origin` (`https://github.com/AlexGalhardo/vukem-video.git`) set. The working tree holds `docs/`, `spikes/`, `web/` and an empty `desktop/` directory (git does not list an empty directory, so only `docs/`, `spikes/` and `web/` appear as untracked). `web/` has the same files as the tip of remote `main` (the old tree was moved there). The remote `main` has the old application at the repository root, including its own `docs/` folder, and carries the tags `v0.1.0` to `v0.4.0`.

Goal: a branch whose history continues `origin/main`; the old root files appear as renamed under `web/`; the new `docs/` is added; nothing in the working tree is overwritten; no force-push.

Why the obvious ways are wrong, so nobody "simplifies" the procedure:

- A pull, a checkout of `origin/main`, a `switch -c x origin/main`, a merge of `origin/main`, a hard reset to `origin/main`, a stash and a clone into the folder all write the remote tree into the working directory. The remote root `docs/` would overwrite `docs/brainstorming.md` and every document of the plan, which exist only on disk (D-020 risk). None is used.
- A fresh commit on an unrelated root would need a force-push over the protected `main` and would destroy tags and releases (D-005).

The method: point the new branch at the remote commit with `git update-ref` (changes a ref, touches no file), load the remote tree into the index **under the prefix `web/`** with `git read-tree --prefix=web/` and without `-u` (changes the index, touches no file), so the index equals the working tree for `web/` and git sees every old root file as renamed.

Every command runs in Git Bash from `C:\dev\vukem-video\video\`. The orchestrator runs it; the author confirms at the marked point. Do not paste an apostrophe into these commands. Every placeholder in angle brackets (`<task id>`, `<ROADMAP task id of the git wiring>`, `<id>`, `<the step 6 commit id>`) is replaced by the real value before the command runs; a commit with a placeholder left in it is not pushed. Each step below gives its command, its reason, its check and its rollback.

**Step 0. Back up what exists only on disk.** Nothing below deletes a file, but the plan documents have no other copy.

```bash
cd /c/dev/vukem-video/video
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

Reason: with `core.autocrlf=true` a Windows working tree can differ from the remote blobs only by line endings (the remote `.gitattributes` has `eol=lf`). Rename detection must be allowed to cover the thousands of files of `web/.agents`. Check: `origin` points to `AlexGalhardo/vukem-video`.

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
git symbolic-ref HEAD refs/heads/feat/vukem-video-layout
git update-ref refs/heads/feat/vukem-video-layout origin/main
git rev-parse HEAD origin/main
git status --short | cut -c1-2 | sort | uniq -c
```

Reason: `symbolic-ref` points HEAD at a new unborn branch name (never `main`: the local `main` is created only after the merge, step 12), `update-ref` makes that branch the remote commit. The index is still empty and the working tree is untouched. Check: both ids are equal; `git status` shows every old root file as a staged deletion (`D `) and `docs/`, `spikes/`, `web/` as untracked (`??`; `desktop/` is an empty directory and is not listed). That is expected and no file on disk changed: `ls docs | head` still shows the planning documents.

Rollback (the repository returns to "No commits yet"; no file is touched): `git symbolic-ref HEAD refs/heads/main` then `git update-ref -d refs/heads/feat/vukem-video-layout`.

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

Rollback (the branch returns to the state after step 4; no file is touched): `git update-ref refs/heads/feat/vukem-video-layout origin/main`, then `rm -f .git/index`; repeat step 5 if the move is to be redone. To return to "No commits yet", follow the rollback of step 4 afterwards. After step 7 the same two commands also drop the later commits (they stay reachable through `git reflog` until the author decides).

**Step 7. Add the root `.gitignore`, the planning documents, and the spikes with the desktop placeholder, in three commits.** The branch then has four commits over `origin/main`: (1) the pure rename of step 6, (2) `chore(repo): add the root .gitignore`, (3) `docs: add the Vukem Video planning documents`, (4) `chore(repo): add the spikes and the desktop placeholder`.

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
git commit -m "docs: add the Vukem Video planning documents" -m "Refs: <task id>"
touch desktop/.gitkeep
git add spikes desktop/.gitkeep
git commit -m "chore(repo): add the spikes and the desktop placeholder" -m "Refs: <task id>"
git status --short
git ls-tree -r HEAD --name-only | grep -c "^docs/"
```

Reason: the new `docs/` shares paths with the old root `docs/` only in the history; in the index the old files are under `web/docs/`, so there is no collision. `desktop/` is an empty directory that git does not list, so the placeholder file `desktop/.gitkeep` is what makes it exist in the repository. Check: `git status` prints nothing; the new `docs/brainstorming.md` equals the backup (`diff docs/brainstorming.md ../backup-video-2026-11-14/docs/brainstorming.md` prints nothing); `web/docs/brainstorming.md` is the old Studio document; `git log --oneline origin/main..HEAD` lists exactly the four commits above.

The secret scan runs on these commits before the push. Betterleaks of the version of section 9.0 (`1.9.0`) must be installed first: `go install github.com/betterleaks/betterleaks@v1.9.0` and `betterleaks version` (CI downloads the Linux archive and checks the sha256 of section 9.0; on a local machine the Go module checksum database verifies the source instead). At this point the config file is still at `web/.betterleaks.toml` (it moves to the root in section 17), so the command uses that path: `betterleaks git . --config web/.betterleaks.toml --no-banner --redact --log-opts="origin/main..HEAD"` must report nothing.

False positives of the example ids. The specifications in `docs/sdds/` show example actor ids (`key:key_<10 characters>`, SDD-000 section 2), and the default `generic-api-key` rule matches them. The allowlist that silences them did not exist yet at step 7, because it lives in the root `.betterleaks.toml`, which only arrives in section 17. So at step 7 the scan reported 7 findings (`docs/sdds/005` line 1280, `006` lines 142, 486, 492, 493 and 561, `012` line 502); the orchestrator reviewed each one, all are the documented example ids `key:key_h8v2n5q7w3` and `key:key_ab12cd34ef`, and the author approved the push with them. The allowlist is added with the move of the config file (P1-REL-002): the pattern `key:key_[a-z0-9]{10}`, and only in `docs/sdds/*.md` (both conditions must hold, so the same string anywhere else is still reported). From then on `betterleaks git . --config .betterleaks.toml --no-banner --redact` and `betterleaks dir . --config .betterleaks.toml` report nothing.

Rollback: `git update-ref refs/heads/feat/vukem-video-layout <the step 6 commit id>`, then `rm -f .git/index` and run `git read-tree HEAD` (no `-u`) so the index matches the step 6 commit again (no file is touched; `desktop/.gitkeep` is the only file this step creates and may stay or be removed by hand).

**Step 8. Required status checks: read the ruleset, then the author removes them for this one pull request.** Remote `main` is protected and its required status checks name jobs of the old workflows. After step 6 those workflows live in `web/.github/workflows/` and GitHub ignores them, so the pull request would wait for checks that never run and could not merge. The Studio is about to be deleted and its checks have no value, so its workflows are **not** copied to the root (a copy cannot work anyway: `defaults.run` does not reach `uses:` steps or Docker build contexts, and the workflows read `site/`, `CHANGELOG.md` and `.agents/` at the old root). Instead:

```bash
gh api repos/AlexGalhardo/vukem-video/rulesets --jq ".[] | {id, name, enforcement}"
gh api repos/AlexGalhardo/vukem-video/rulesets/<id> --jq ".rules[] | select(.type == \"required_status_checks\") | .parameters.required_status_checks[].context"
```

(a) Both commands are reads: the first finds the ruleset of `main`, the second lists its required status checks. (b) The author, and only the author, removes the required status checks from the ruleset of `main` for this pull request in the repository settings (Settings, Rules, the ruleset of `main`, delete the "Require status checks to pass" rule). The pull request rule and the linear history rule stay. This is a reversible settings change, not a force-push, and the orchestrator never makes it. (c) The orchestrator records the removed check names, exactly as read in (a), in the task, so that `REL-REPO` restores required checks with the new job names of section 16.1 in the pull request that adds the new workflows (the ruleset file `.config/ruleset-protect-main.json` is applied with the command of section 16.1). The pull request of step 11 then has no required check and is merged after the author looked at the diff summary.

Reason: the only alternatives are to carry workflow copies that do not run, or to leave an unmergeable pull request. Check: the second command printed the names; after (b) it prints nothing; the pull request rule and `required_linear_history` are still listed in the ruleset (`gh api repos/AlexGalhardo/vukem-video/rulesets/<id> --jq ".rules[].type"` prints `pull_request` and `required_linear_history`).

Rollback: the author re-adds the required status checks with the names recorded in (c) (or applies `.config/ruleset-protect-main.json` once it exists), in the same settings page. Until then `main` has no required check, so no merge happens apart from step 11.

Case found on the real remote (2026-11-14): the first command printed nothing, because the repository had no ruleset, and it had no classic branch protection either (`gh api repos/AlexGalhardo/vukem-video/branches/main/protection` answered 404 "Branch not protected"). So there were no required status checks, nothing was removed in (b), and (c) records an empty list. The pull request of step 11 is not blocked by checks (the one check that ran was `conventional`, the title check of a `pull_request_target` workflow of the old `main`, and it passed). `REL-REPO` does not restore anything: it creates the protection from scratch by applying `.config/ruleset-protect-main.json` (section 16.1) in the pull request that adds the new workflows, and the author confirms it.

**Step 9. Author confirmation before any push.** The orchestrator shows, in the chat: the output of `git log --oneline origin/main..HEAD` (exactly the four commits of steps 6 and 7), `git diff --stat -M origin/main HEAD | tail -n 5`, the step 7 secret scan result, the check names recorded in step 8 and the confirmation that the author removed them. The author answers yes in the chat. Without that yes nothing below runs (D-020). Reason: step 10 is the first action that leaves the machine. Check: the five items above are on screen and the yes is in the chat before the next command.

Rollback: nothing has left the machine; stop, and use the rollbacks of steps 4 to 7 if the branch is to be dropped.

**Step 10. Push the branch (a plain push, a fast-forward of a new branch).**

```bash
git merge-base --is-ancestor origin/main HEAD && echo "descends from origin/main"
git push -u origin feat/vukem-video-layout
```

Reason: the branch contains `origin/main`, so this is an ordinary push of a new ref; no `--force`, no `--tags`. Check: the first command prints the sentence; the push reports a new branch.

Rollback: `git push origin --delete feat/vukem-video-layout`, only with the author's confirmation in the chat; it removes a branch that nothing else uses, and neither `main` nor tags are touched. The local branch and the local files stay.

**Step 11. Open the pull request and merge it with rebase-merge.**

```bash
gh pr create --base main --head feat/vukem-video-layout --title "refactor(repo): move the Studio tree under web/ and add the planning documents" --body "Refs: <task id>. Pure rename of the old tree under web/, the root gitignore, the planning documents, spikes and the desktop placeholder. No content of web/ changed."
gh pr view --json mergeable,mergeStateStatus
```

The pull request has no required status check (step 8). The orchestrator shows the author the diff summary of step 9 again (the rename count recorded in step 5, `git diff --stat -M origin/main HEAD | tail -n 5`; no file count through `gh pr diff`, which hits the API file caps on thousands of renames). Then **a separate, explicit yes of the author in the chat is required before `gh pr merge --rebase`**: merging into the protected `main` is the one irreversible action of this procedure. Only after that yes: `gh pr merge --rebase`. Reason for rebase-merge only here: it keeps the pure-rename commit as its own commit; a squash would still show renames in `git log -M` but would fold the move and the documents into one entry. The PR title check applies to the title above (type `refactor`, scope `repo`). Check: `gh pr view --json state,mergeCommit` prints `MERGED`.

Rollback: before the merge, close the pull request (`gh pr close`) and use the rollback of step 10. After the merge there is none that rewrites history: only a revert pull request, which restores the old tree through a new commit and is its own decision of the author.

**Step 12. Return to `main` safely after the merge.** A rebase-merge rewrites the commit ids, so the merged `main` has the same tree as the branch but different commits, and the safe check is a tree comparison.

```bash
git fetch origin --tags
git diff --stat origin/main feat/vukem-video-layout
git status --short
git switch -c main --track origin/main
git status
git log --oneline -5
git branch -D feat/vukem-video-layout
```

Reason: the merged `main` and the branch must hold the same files before the branch is dropped. Check: `git diff --stat origin/main feat/vukem-video-layout` must print nothing (same tree). If it prints anything, for example because `main` moved for another reason after the merge, **stop and report**; do not switch. `git status --short` must also print nothing (every file is tracked and identical); if it prints anything, stop and compare against the backup. Only then `git switch -c main --track origin/main`, which changes no file. After the switch `git status` says `Your branch is up to date with 'origin/main'`. Then `git branch -D feat/vukem-video-layout`; `-D` and not `-d` because the rewritten commit ids make `-d` refuse a branch whose content is merged. Afterwards run `git describe --tags --abbrev=0` and expect `v0.4.0`: this is what lets `git cliff` start the `v0.5.0` section after `v0.4.0` (section 5.2).

Rollback: the switch changes no file. Before the last command, `git switch feat/vukem-video-layout` and then `git branch -D main` undo it; that is why the feature branch is deleted last and only after the tree comparison printed nothing. After the deletion its commits stay reachable through `git reflog`.

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

Decisions of this SDD that other documents must reflect: `internal/tools/` and `scripts/` exist at the root (ADR-0001 tree); `docs:export` is not used (SDD-010 reads three sources of the binary); `.config/cliff.toml` and `.github/{CONTRIBUTING,SECURITY,CODE_OF_CONDUCT}.md` live where vukem puts them; the scopes list (23 scopes, with `release` and `deps-dev`) is enforced; task pull requests are squash-merged with the pull request title and description as the default message; the coverage floors of section 13.2; the Debian slim runtime and the `ffmpeg.env` single pin; NFR-043, NFR-044 and NFR-045 are reworded (section 11.1, section 7 step 11, section 13.3); `.githooks/` instead of lefthook; tabs in `.editorconfig` with the listed exceptions; `release.yml` publishes the notes.

## 20. Acceptance

Phases: `v0.5.0`, the end of the Prototype, ships signed archives only. The checks about the container image (the `image` job in item 4, items 9 and 10) and about the third-party notices (item 11, and the notices file inside each archive in item 5) are accepted on the first ALPHA release that carries them (tasks `REL-NOTICES` and `REL-DOCKER`; in the ROADMAP, P2-REL-001, P2-REL-002 and P2-REL-004). Every other item is accepted on `v0.5.0`.

Each check is verifiable by someone who has only this document and the repository. A check passes when the stated command or observation matches.

1. `.config/cliff.toml` equals section 5.2: `git cliff --config .config/cliff.toml --unreleased --strip all` runs and prints an `## [Unreleased]` section; run against a temporary repository with commits `feat(core): add x`, `fix(ingest): y`, `refactor(repo)!: z`, `docs: d`, `ci: c`, `feat(api): s` plus a `Security: closes a risk` footer, the sections are Added, Changed, Fixed, Security in that order, the `!` commit carries `**BREAKING:**`, and `docs` and `ci` do not appear.
2. A temporary commit `feat(foo): bar` is rejected by `go run ./internal/tools/commitlint <file>` (unknown scope), and a pull request titled `feat(foo): bar` fails `Conventional title`; titles with each of the 23 scopes pass, including `chore(release): v9.9.9`.
3. `go run ./internal/tools/release prepare v9.9.9` on a scratch branch inserts a section under `[Unreleased]` and a `chore(release): v9.9.9` commit and refuses to run on `main`; `release tag` refuses when `HEAD` is not `origin/main` (tested with a temporary bare remote, no network).
4. `release.yml` passes a review of section 9.3: top-level `permissions: contents: read`; every `uses:` line has a 40-hex SHA and a version comment; the `image` job's `if` requires `stable == 'true'`; it pushes six tags (`X.Y.Z`, `X.Y`, `latest`, on `ghcr.io/alexgalhardo/vukem-video` and `alexgalhardo/vukem-video`). The `Workflow pins` job passes on the repository and fails on a scratch branch that adds a `uses:` line without a SHA; the `image` job builds and smoke tests the image before the push step.
5. Dry run of the release build: `task release:binaries SNAPSHOT=1` produces six archives and six SBOM files in `dist/` plus `checksums.txt`; every archive is under 60,000,000 bytes; each archive contains `LICENSE.md`, `README.md` and `CHANGELOG.md`, and, from the first ALPHA release (RF-215), also `THIRD_PARTY_NOTICES.md` and `licenses/`; `vukem-video version` of the extracted binary runs.
6. `CI` on 10 consecutive pull requests: every required check green; the median total wall time of the workflow is under 15 minutes and no run exceeds 20 minutes (`gh run list --workflow CI --json startedAt,updatedAt`).
7. The `Test` job passes on `ubuntu-latest`, `windows-latest` and `macos-latest`, with `-race`; the coverage gate fails when a `core/` package is below 90 percent (tested by lowering coverage in a scratch branch) and passes at the tip of `main`.
8. `Generated files` fails when any of `core/schema/`, `web/resources/js/generated/`, `site/generated/` or `site/content/generated/` is edited by hand, and passes after `task generate`.
9. The image builds with `task image:build`; `task image:smoke` passes: `ffmpeg -version` has neither `--enable-gpl` nor `--enable-nonfree`, `libopenh264` encodes a one-second test clip, the process runs as uid 10001, `/usr/share/doc/vukem-video/` holds `THIRD_PARTY_NOTICES.md` and `ffmpeg/{LICENSE.txt,configure.txt,SOURCE.txt}`, `docker image inspect` shows `VUKEM_VIDEO_HOME=/data`, a volume at `/data` and a health check on `127.0.0.1:7878/up`; the version stamp check of the Dockerfile passes in `image.yml` with the dummy `VERSION` (no `+dirty`), and a scratch change that lists a tracked path in the dockerignore makes it fail.
10. `grep -nE "FROM|latest" .config/docker/vukem-video.Dockerfile` shows only fixed tags and no `latest`, and the runtime base is `debian:13...-slim@sha256:...`; `.config/ffmpeg.env` contains no `<recorded by` text; the SHA-256 in it matches `sha256sum` of a fresh download of each asset.
11. `task notices:check` passes; adding a dependency licensed GPL-3.0 to a scratch branch fails it with the module name; `THIRD_PARTY_NOTICES.md` lists FFmpeg, the six fonts, whisper.cpp and every Go and npm production dependency.
12. The `protect-main` ruleset (section 16.1) is applied: a direct `git push origin HEAD:main` is rejected; a pull request whose `Lint (Go)` fails cannot merge; a merge commit cannot be created.
13. Section 18 executed on the real repository: `git log --oneline origin/main..main` after the merge shows the four commits of steps 6 and 7 (the rewritten ids after the rebase-merge, with the same subjects); `git merge-base main v0.4.0` equals `v0.4.0`; `git show -M --stat <step 6 commit>` shows only `rename` entries (100 percent similarity) and no content change; `diff -r docs ../backup-video-2026-11-14/docs` prints nothing; no `--force` appears in the shell history or in the remote's push events (`gh api repos/AlexGalhardo/vukem-video/events`).
14. Release `v0.5.0` (`REL-FIRST`): `gh release view v0.5.0 --json name,body,isPrerelease,assets` returns `name` `v0.5.0`, a non-empty body that starts with `### ` and equals the section of `CHANGELOG.md` without its heading, `isPrerelease` false, 15 assets; every command of section 7 step 11 that concerns the archives (checksums, cosign, SBOM, attestation) succeeds; `git cat-file -t v0.5.0` prints `tag`. The image commands of step 11 and "both registries serve `X.Y.Z`, `X.Y` and `latest` with one digest" are checked for the first ALPHA release that publishes the image (RF-210), with `THIRD_PARTY_NOTICES.md` in every archive from that release.
15. An attempt to push a tag `v0.5.0` again, or to delete it, is documented in `CONTRIBUTING.md` as forbidden and the release tool refuses a version that exists (`release tag v0.5.0` exits 1).
16. The hooks work: `git config core.hooksPath` prints `.githooks`; a commit with the header `Fix Things.` is rejected by `commit-msg`; a push that changes `site/content/**` without refreshed dates is stopped by `pre-push` with the committed refresh.
17. `.editorconfig` matches section 15.2: `editorconfig-checker` (or the editor) reports no violation on `core/`, `engine/`, `service/`, `web/resources/js`, `web/e2e` and `site/` (tabs, including the TypeScript, TSX and Astro files of `site/`) and on `*.yml`, `*.md` (two spaces); the only space-indented files outside YAML and Markdown are `package.json`, lockfiles, `tsconfig*.json` and `components.json`.
18. Every statement of section 2 can be followed from its cited vukem file and line: spot check R6, R15, R22 and R27 against `v0.43.0:.github/workflows/release.yml:120-164`, `v0.43.0:.config/goreleaser.yaml:8-18` and `v0.43.0:.config/ruleset-protect-main.json:12-40`, read with `git -C C:\dev\vukem-video\vukem show v0.43.0:<path>`.
19. Section 18 is safe step by step: each of steps 0 to 12 has a command, a reason, a check and a rollback; no step uses a stash, a hard reset, a checkout of paths or a force push; no step pushes before the confirmation of step 9, and the merge of step 11 needs its own explicit yes.
