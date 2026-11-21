# ADR-0001: Repository layout and Go module

- Status: Accepted (2026-11-14); the product name is amended by [ADR-0031](0031-stuarty-product-scope-and-domains.md) and the module path and binary name by [ADR-0044](0044-rename-to-stuarty.md) (2026-11-18); rule 1 admits vukem's `z` package by author decision D-124 (2026-11-20)
- Deciders: author (D-002, D-005, D-007, D-008, D-020, D-021), architect
- Links: author decision D-002, D-005, D-007, vukem practices, existing Studio

## Context

The repository `AlexGalhardo/vukem-video` holds the old Studio application at its root. The author moved that tree under `web/`, wants `web/`, `desktop/` and `docs/` in one repository, and decided that the editor replaces the Studio (D-002) and continues the git history (D-005). The briefing asks whether a shared code folder such as `core/` or `engine/` should exist. Restriction 1 of the briefing requires a single command core reused by UI, MCP, CLI and API.

## Decision

One repository, one Go module at the repository root, module path `github.com/AlexGalhardo/vukem-video`.

```text
video/                       repository root (product root)
├── core/                    pure domain, no I/O, no vukem import
│   ├── timeline/            document types, JSON Schema, migrations
│   ├── timeval/             time parsing and frame arithmetic
│   ├── command/             command registry, apply, inverse, errors
│   ├── plan/                frame plan: what is where at frame t
│   └── lint/                structural checks that return text
├── engine/                  media work, calls external processes
│   ├── ffmpeg/              probe, graph builder, runner
│   ├── raster/              text and shape rasterizer, bundled fonts
│   ├── ingest/              normalized media, proxy, thumbnails, peaks
│   ├── export/              render a project to a file
│   ├── transcribe/          whisper.cpp and API providers
│   └── store/               content-addressed files on disk
├── service/                 the application boundary (ADR-0008)
├── web/                     the vukem application: HTTP, SSE, persistence, jobs, MCP, CLI, React UI
├── desktop/                 desktop shell (BETA)
├── site/                    documentation site
├── docs/                    planning documents (this folder)
├── spikes/                  disposable experiments
├── internal/tools/          build-time tools: release, commitlint, covergate, contractgen, importcheck, threatcheck
├── scripts/                 shell scripts for CI and development (FFmpeg install, fixtures)
├── README.md, LICENSE.md, CHANGELOG.md, AGENTS.md (for contributors)
└── .github/, .config/ (cliff.toml, goreleaser, FFmpeg pin, Docker files), .githooks/
```

Rules:

1. `core/` imports only the Go standard library and small pure libraries. It never imports `engine/`, `service/`, `web/` or vukem, with one exception: vukem's validation package `z`, which imports only the standard library (author decision D-124, 2026-11-20; `internal/tools/importcheck` lists it in `CoreLibraries`).
2. `engine/` may import `core/`. It never imports `web/` or vukem's HTTP packages. It may use vukem's `contrib/engine` to run processes.
3. `service/` imports `core/` and `engine/` and defines interfaces for persistence that `web/` implements.
4. `web/` is the only place that imports vukem's application packages.
5. The binary is built from `web/cmd/app` and named `vukem-video`.
6. Repository-level files that the Studio kept at its root (`.github/`, `.githooks/`, `CHANGELOG.md`, `LICENSE.md`, `site/`) live at the repository root, because GitHub only runs workflows from the root `.github/`; tool configuration such as `cliff.toml` lives in `.config/`, where vukem keeps it (SDD-009).
7. The work contract of the agent team (`ROADMAP.md`), its protocol (`AGENTS.md` of the workspace) and its session memory are not in this repository: they live in the workspace root, outside git (D-025).

## Alternatives

- **No shared folder; all logic inside the vukem application.** Rejected: the CLI, the MCP server and a future desktop shell would import an HTTP application, and the rule "the editor is decoupled from Inertia" would rest on discipline alone.
- **Several Go modules joined by `go.work`** (as vukem does for its drivers). Rejected for now: the code is one product with one version; several modules add version bookkeeping with no consumer. It is the fallback if the vukem tooling refuses to run with `go.mod` one level above the application.
- **Separate repositories for core and application.** Rejected: contracts and their consumers would change in different pull requests.

## Consequences

Positive: the import rules make "single command core" a property the compiler and a lint rule can check; the core is testable without a database, FFmpeg or a browser.

Negative: vukem's tooling (`vukem serve`, `vukem build`, `make:*`) is written for an application whose `go.mod` is in its own folder. Verified on 2026-11-14 with vukem v0.43.0 (P1-REL-003), running from `web/` with the module root one level up: `serve`, `route:list`, `migrate` and `about` work, because the CLI runs `go build ./cmd/app` and the Go tool finds the `go.mod` above. The `make:*` generators refuse (`no go.mod here`) and would generate import paths without the `web/` segment. This is a convenience only (the files are written by hand), so the single-module layout stays and the fallback below is not applied; the friction is recorded in `vukem-feedback.md` as VF-029. The fallback remains available if the author wants the generators.

The import rules are enforced by `internal/tools/importcheck` (a Go test over every package of the module). ADR-0001 rule 1 does not list the "small pure libraries" of `core/`; the test holds the list, and it contains only the JSON Schema validator of ADR-0014. A library is added to the list together with the author's approval of the dependency. Rule 2 allows `engine/` one vukem import, `github.com/AlexGalhardo/vukem/contrib/engine`; any other vukem import outside `web/` fails the test.

Negative: moving the Studio tree and deleting it are large commits; they are isolated in the first task group so later history stays readable.

Scope of the removal (D-021): everything of the Studio leaves the tree, including its non-code content (the skill kits under `web/.agents/skills`, `web/films`, `web/docs`, `prompt.md`, its `ROADMAP.md`). What is reused (CI workflows, git hooks, `cliff.toml`, lint configurations, the documentation site shell) is copied to its new place first. Everything removed stays in the git history.
