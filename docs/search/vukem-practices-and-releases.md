# vukem practices and releases: the pattern this project follows

> Stage 1, research. All sources accessed 2026-11-14.
> Marking: **FACT** (verified at the source), **INFERENCE**, **OPINION** (the architect's judgement).
> Evidence, with the full text of real release notes and workflow excerpts: [`notes/vukem.md`](notes/vukem.md). Sources: the vukem repository at `v0.43.0` (commit `f8fefef`), `gh release list/view -R AlexGalhardo/vukem`, and the existing Studio application in `web/`.
> Friction found while studying vukem is recorded as VF-001 to VF-010 in `vukem-feedback.md` at the workspace root.

## Contents

1. [What "follow the vukem release pattern" means, concretely](#1-what-follow-the-vukem-release-pattern-means-concretely)
2. [Observed release pattern](#2-observed-release-pattern)
3. [A real release note](#3-a-real-release-note)
4. [Tooling behind a release](#4-tooling-behind-a-release)
5. [Repository conventions](#5-repository-conventions)
6. [How vukem plans work](#6-how-vukem-plans-work)
7. [The documentation site model](#7-the-documentation-site-model)
8. [Where the briefing and the repository disagree](#8-where-the-briefing-and-the-repository-disagree)
9. [Gaps](#9-gaps)

## 1. What "follow the vukem release pattern" means, concretely

| Rule | Source |
| --- | --- |
| Semantic Versioning, tag `vX.Y.Z`, annotated; a published tag is never moved or deleted | `docs/tooling.md`; `gh api .../git/ref/tags/v0.43.0` returns an annotated tag |
| Before 1.0, a minor version may break and a patch never does | `docs/tooling.md#versioning` |
| One release per task group of the plan, never several groups in one | `docs/tooling.md#release-process` |
| The release title is exactly the tag | every recent release |
| The release body is the changelog section of that version, copied, without the `## [x]` heading; English | release `v0.43.0` |
| Sections in fixed order: Added, Changed, Deprecated, Removed, Fixed, Security | `.config/cliff.toml` |
| Each entry: `- **<scope>:** Message ([abc1234](commit URL))`, with `**BREAKING:**` in front when it breaks; commit links, no pull request links | release `v0.43.0` |
| `CHANGELOG.md` follows Keep a Changelog 1.1.0, has a permanent `[Unreleased]` section and comparison links at the bottom; released sections are never edited by hand | `CHANGELOG.md`, `.github/CONTRIBUTING.md` |
| Version sections are generated from Conventional Commits by git-cliff; `docs`, `test`, `ci`, `build`, `chore`, `style` commits are left out | `.config/cliff.toml` |
| A release with an empty body is refused | `docs/tooling.md`, line 118 |
| Binaries by GoReleaser: static, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`, for linux, darwin and windows on amd64 and arm64 | `.config/goreleaser.yaml`, assets of `v0.43.0` |
| `checksums.txt` signed with cosign keyless; an SBOM per archive; build provenance attestation | assets of `v0.43.0`, `release.yml` |
| Docker image tags `X.Y.Z`, `X.Y` and `latest`, multi-architecture, signed by digest; a pre-release tag publishes no image | `release.yml`, job `image` |
| Every GitHub Action pinned by full commit SHA with a version comment | `release.yml` |

## 2. Observed release pattern

- **FACT.** 58 core releases between 2026-10-16 and 2026-11-12, the latest `v0.43.0`. 32 releases on the first day; later about one or two a week; `v0.38` to `v0.43` in six days.
- **FACT.** Only `v0.0.1` to `v0.2.0` (14 releases) are flagged as pre-release. Everything after is a normal release, although the documentation says `v0.x` releases are marked pre-release (VF-005).
- **FACT.** Assets of `v0.43.0`, 15 files:

```text
checksums.txt
checksums.txt.sigstore.json
vukem.intoto.jsonl
vukem_0.43.0_{darwin,linux}_{amd64,arm64}.tar.gz
vukem_0.43.0_windows_{amd64,arm64}.zip
<each archive>.sbom.json
```

- **FACT.** Each archive contains `LICENSE`, `README.md` and `CHANGELOG.md`.
- **FACT.** Module releases use the tag `<dir>/vX.Y.Z` (for example `drivers/s3/v0.3.0`), have their own body format and no assets. GitHub Releases for modules stop on 2026-10-18 while tags continue (VF-008).
- **FACT.** The image is published only to `ghcr.io/alexgalhardo/vukem` (VF-010).

## 3. A real release note

`v0.43.0`, published 2026-11-12, title `v0.43.0`. Verbatim, from `gh release view v0.43.0`:

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

## 4. Tooling behind a release

- **FACT.** `task release -- vX.Y.Z` runs a Go tool (`internal/tools/release`) that requires a clean tree on `main`, generates the version section under `[Unreleased]`, commits `chore(release): vX.Y.Z`, creates the annotated tag, pushes `main` and the tag in two separate pushes, and publishes the GitHub release from the changelog section.
- **FACT.** GoReleaser only attaches binaries, checksums, SBOMs and signatures (`release.mode: keep-existing`, `changelog.disable: true`). It never writes the notes.
- **FACT.** `release.yml` triggers on a pushed `v*` tag and on `workflow_dispatch` with a version input, because a push of more than three tags does not fire the event. It verifies the cosign signature of `checksums.txt` right after producing it.
- **FACT.** The Studio's own `release.yml` is the lean variant for an application with no binary yet: it extracts the changelog section with `awk` and runs `gh release create "$TAG" --title "$TAG" --notes-file ...`, failing if the section is empty. Its comment says the binary pipeline is "added when the application ships a binary".

**INFERENCE for this project.** The editor ships a binary and an image, so it needs the full vukem pipeline, adapted in two places: the image goes to GHCR and to Docker Hub (the author's standing rule, which vukem itself does not follow), and the image contains a pinned LGPL FFmpeg with its licence notices. The version line continues the repository: the first editor release is `v0.5.0` (D-005).

## 5. Repository conventions

| Area | Convention | Source |
| --- | --- | --- |
| Go | `go 1.27.4`; no cgo; Windows 11 is a first-class target | `go.mod`, `.claude/rules/go.md` |
| Task runner | `Taskfile.yml` (go-task): `ci`, `fmt`, `lint`, `test`, `test:e2e`, `test:race`, `vuln`, `release` | `Taskfile.yml` |
| Go lint and format | golangci-lint v2 with gofumpt, gci and golines; `go vet`; a budget of `//nolint:gocognit` that never rises | `.golangci.yml`, `ci.yml` |
| Markdown | markdownlint-cli2; `MD013` off; dash lists | `.markdownlint-cli2.jsonc` |
| EditorConfig | two spaces by default; tabs for Go, `go.mod`, Makefile; LF | `.editorconfig` |
| Front end | Biome, `tsc --noEmit`, Bun | Studio `package.json` |
| Hooks | vukem uses lefthook; the Studio uses `.githooks/commit-msg` and `pre-push` | `lefthook.yml`, `web/.githooks/` |
| Commits | Conventional Commits 1.0, English, `type(scope): description`, lowercase start, no trailing period, header up to 100 characters; breaking change with `!` and a `BREAKING CHANGE:` footer; a `Security:` footer lists the commit under Security; task reference in the body (`Refs: P.3.2`) | `.claude/rules/commits.md` |
| Branches | `feat/<name>`, `fix/<name>`, `docs/<name>` | `docs/tooling.md` |
| Pull requests | `main` accepts only pull requests with green checks and linear history; the PR title is checked as a Conventional Commit and becomes the squash commit | `pr-title.yml`, ruleset |
| Tests | test first; standard library plus go-cmp; table-driven; no network in unit tests; `-race` in CI; Linux, macOS and Windows; a coverage gate | `AGENTS.md`, `docs/testing.md` |
| Security | secure by default, fail closed; every security default has a `TestSecurity_*` test; secret scan with Betterleaks; govulncheck; CodeQL; OpenSSF Scorecard | `.claude/rules/secure-coding.md`, workflows |
| Agent files | `AGENTS.md` of about 50 lines (start here, commands, rules, where things live, when stuck); `CLAUDE.md` only points to it | repository root |
| Decisions | "Decisions are final unless `docs/brainstorming.md` is updated first" | `AGENTS.md` |
| Everything in English | code, comments, docs | `AGENTS.md`; adopted here by D-008 |
| Framework feedback | downstream applications log findings in the single `vukem-feedback.md` at the workspace root, never in a copy inside an application | `AGENTS.md` |

## 6. How vukem plans work

- **FACT.** There is no `plans/` folder in vukem. The plan is one file, `docs/PLAN.md`, described as the "single source of truth for what to build next" (VF-009).
- **FACT.** Its structure: how to use the file; a status table (current phase, next task, updated) rewritten by the release tool; design references; a definition of done valid for every task; a phase overview (phase, version, goal, exit criteria); then phases, groups and tasks.
- **FACT.** Task format:

```markdown
### P.1 Config & environment — needs: P.0
- [x] **P.0.1** Initialise module ... (`go 1.27.0`) ...
  → verify: `go build ./...` succeeds.
```

  Each task has an identifier `<phase>.<group>.<task>`, a statement and one executable `→ verify:` line. Groups may declare `needs:`.

- **FACT.** The Studio's `ROADMAP.md` uses `[x]` done, `[/]` partial, `[ ]` not started, phases with numbered subsections and a final `#### Audit` box whose items are verifiable (commands, URLs, CI run identifiers), with the rule "A phase is done only when every audit box under it is `[x]`".

**INFERENCE for this project.** The briefing's ROADMAP format (owner, dependencies, files, references, asserts) is a superset of vukem's. The two rules worth copying exactly are: every task has a verification that can be executed, and one release per task group.

## 7. The documentation site model

- **FACT.** vukem.dev is built from `site/` with Astro 7 as a static site, React islands, Tailwind 4 and MDX. Content lives in `site/content/docs/<category>/<page>.mdx`; navigation is a typed array in `site/content/navigation.ts`. There are 104 pages in 12 categories.
- **FACT.** Page style: "The idea", "Quick start" with steps, "Common mistakes", "Next steps", "You will learn".
- **FACT.** For agents: `llms.txt`, `llms-full.txt`, a Markdown twin of every page, and an "Agents" link in the top bar.
- **FACT.** "Last updated" per page comes from git through a generated `content/last-updated.json`, checked in CI and in a pre-push hook.
- **FACT.** Search is an index generated at build time with a dialog loaded on first use. Deployment is GitHub Pages on push to `main`. The changelog page renders `CHANGELOG.md`. Go snippets in the documentation are type-checked in CI.
- **FACT.** The site has no i18n: English only. A snapshot of the 0.20 documentation is kept under `content/versions/0.20`.
- **FACT.** The Studio's `web/site` is an older fork of that shell: it lacks the Markdown twin, the anchor check and the trailing-slash tests, and is served from a container on Railway.
- **FACT.** FreeCut's documentation is a route inside its React application: every path returns the same 1,408-byte shell, with no sitemap, `robots.txt` or `llms.txt`. Rendered with a browser it has 30 pages in 5 sections: Start, Core editing, Creative tools, Output, Reference.

**INFERENCE for this project.** Start from the current vukem `site/`, not from the Studio fork, and add what vukem does not have: a second locale (pt-BR). The FreeCut section structure is a good outline for the user guide, and its implementation is the thing to avoid, because an agent cannot read it without running JavaScript. A product whose main users are agents needs `llms.txt` and Markdown twins from the first day.

## 8. Where the briefing and the repository disagree

| Briefing says | Repository shows | Resolution |
| --- | --- | --- |
| vukem organises plans in `plans/*.md` | No such folder; the plan is `docs/PLAN.md` | Follow the real practice: one `ROADMAP.md` (VF-009) |
| Documents in pt-BR | vukem and the Studio keep everything in English | D-008: everything in English |
| `vukem-feedback.md` at the repository root | vukem's `AGENTS.md`: one file at the workspace root, never inside an application | The file stays at the workspace root |
| Images go to GHCR and Docker Hub (author's standing rule) | vukem publishes only to GHCR | Publish to both; adapt the workflow (VF-010) |

## 9. Gaps

- The full body of only 6 of the 92 releases was read; the rest by name, date and flag.
- vukem was not built or run; capabilities come from its code and documentation.
- `docs/architecture.md`, `docs/security.md` and `docs/testing.md` were not read in full.
- The version selector and theme toggle of vukem.dev were not confirmed in a browser.
