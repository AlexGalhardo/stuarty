# Vukem Video: planning documents

Everything needed to build Vukem Video, written so that an engineer who was not in the planning conversation can work from it. Read in this order: the PRDs (what and why), the ADR list (how, in one page each), then the SDD of the area you are about to touch.

Work is driven by the plans of `plans/` (index `plans/README.md`): one plan is one unit of work that an agent executes and delivers, with its steps, acceptance commands and status (rule `.agents/rules/plans.md`). The other folders hold what defines the product. Names and numbering of the documents follow `.agents/rules/naming.md`: `prds/NNN-<slug>.md`, `adrs/NNNN-<slug>.md`, `sdds/NNN-<slug>.md`, decisions of the author `D-NNN` and research notes are kept by the maintainers outside version control and are cited by id or name only; a document is never renumbered or moved, and each starts with its title and a short paragraph that says what it answers.

## Reading map

| You want to | Read |
| --- | --- |
| know what the product is and what is in or out of scope | `prds/001-editor-mvp.md` |
| know what to build next, in what order, and how to verify it | `plans/README.md`, then the plan you were given |
| know how an agent uses it | `prds/002-agent-operability.md`, then `sdds/003-agent-interfaces.md` |
| use a name (operation, table, error code, variable) | `sdds/000-shared-definitions.md` |
| change the document format | `sdds/001-timeline-schema.md` |
| add or change a command | `sdds/002-command-catalogue.md` |
| understand why preview and export agree | `adrs/0003-render-one-plan-two-executors.md`, `sdds/004-render-pipeline.md` |
| know what the Rust renderer reads, writes and may never do (Phase 5) | `adrs/0029-*.md`, then `sdds/013-renderer.md` |
| know why a decision was taken | `adrs/`; the decisions of the author (`D-NNN`) they cite are kept privately by the maintainers |
| check that a requirement is covered | `traceability.md` |
| see how well small and large AI agents use the product, round by round | `agent-benchmark.md` |
| design or review a screen of the web UI or the desktop shell (tokens, components, content style, accessibility) | `design-system.md`, then `sdds/012-ui-architecture.md` |

## Requirements (`prds/`)

| File | Covers |
| --- | --- |
| `001-editor-mvp.md` | the editor: projects, timeline, picture, text and captions, audio, templates, preview and export, UI; RF-001 to RF-075, NFR-001 to NFR-013 |
| `002-agent-operability.md` | MCP, CLI, HTTP API, text views, verification, safety; RF-100 to RF-134, NFR-020 to NFR-025 |
| `003-distribution-and-docs.md` | installation, local and network modes, releases, container image, documentation site; RF-200 to RF-215, NFR-040 to NFR-045 |
| `004-agent-studio.md` | the agent studio outside the editor: production folders, skills, memory; RF-300 to RF-306, NFR-060 to NFR-061 |

Each requirement has a phase: `P` Prototype, `A` ALPHA, `B` BETA.

## Decisions of the author (`D-NNN`)

Each decision has a date, alternatives, reason and impact. They are the input of the ADRs and are not reopened without the author. The log itself is private (since D-037); the ADRs, PRDs and SDDs carry what a contributor needs.

## Architecture decisions (`adrs/`)

| ADR | Decision |
| --- | --- |
| 0001 | one repository, one Go module: `core/`, `engine/`, `service/`, `web/`, `site/`, `desktop/` |
| 0002 | the command core is Go, pure, the only place editing logic lives |
| 0003 | render: one plan, two executors, truth on pause; pixels composed in Go (items 3 and 4 amended by ADR-0029) |
| 0004 | time is integer frames at a rational rate; times cross the boundary as strings with a unit |
| 0005 | timeline data model: tracks, items with absolute position, captions with words, keyframes |
| 0006 | operation log, revisions, conflicts per object, undo per actor by compensating batch |
| 0007 | the UI is a thin client |
| 0008 | one service boundary; one owner process per workspace |
| 0009 | persistence: SQLite, the document and an append-only log |
| 0010 | content-addressed media store; ingest produces normalized media, proxy, thumbnails, peaks |
| 0011 | FFmpeg: separate process, LGPL build only, encoder selection, argument policy |
| 0012 | text and shapes rasterized by Go code with bundled open fonts |
| 0013 | browser stack: React, a canvas compositor behind an interface, `<video>` proxies |
| 0014 | contracts in JSON Schema, generated to TypeScript and to the reference |
| 0015 | MCP server: sixteen tools named by intent, stateless |
| 0016 | CLI on vukem commands, JSON output, exit codes 0, 1, 2 |
| 0017 | HTTP API under `/api/v1` and server-sent events |
| 0018 | jobs: own table over the vukem queue, with progress and cancellation |
| 0019 | authentication: local trust on loopback, login and scoped keys on the network |
| 0020 | agent safety: no outbound capability, trash, limits, marked untrusted text |
| 0021 | transcription behind a provider interface; whisper.cpp local by default |
| 0022 | templates are parameterized projects |
| 0023 | desktop shell deferred to BETA |
| 0024 | documentation site: static, two languages, generated reference |
| 0025 | releases and CI in the vukem pattern |
| 0026 | observability without telemetry |
| 0027 | items of the stack draft that were cut or deferred, each with its reason |
| 0028 | one legacy release, a minor about every 90 commits, rebase-merged release pull requests |
| 0029 | one frame plan (Go), one renderer (Rust in WebAssembly), two hosts; WebCodecs and WebGPU in the preview (Phase 5) |
| 0030 | desktop editor is a Gio window over `service.Service`; the browser UI stays the web client |
| 0031 | the agent studio lives outside the editor: generation, engine choice, production folders and memory are agent-side |

## Specifications (`sdds/`)

| SDD | Specifies | Main owner |
| --- | --- | --- |
| 000 | shared definitions: the names everything uses | architect |
| 001 | timeline schema, invariants, examples, versioning | senior-a |
| 002 | command catalogue: the model (Part A), the 48 commands (Part B), each command in detail with its golden cases (Part C) | senior-a |
| 003 | agent interfaces: the `Service` interface, text formats, MCP tools, CLI, HTTP API, parity | senior-d |
| 004 | render pipeline: frame plan, rasterizer, compositor, audio, export, frames, preview executor, tolerance, measurements | senior-b |
| 005 | data model and persistence: tables, the commit transaction, undo, trash, jobs, workspace ownership | senior-c |
| 006 | real-time protocol: events, ordering, replay, reconciliation, conflicts between a human and an agent | senior-c |
| 007 | security model: threats, local and network trust, scopes, process policy, paths, untrusted text, limits | senior-c (coordination) |
| 008 | test strategy: kinds of test, golden cases, golden frames and tolerance, scenarios, parity, performance | every owner |
| 009 | releases and CI/CD in the vukem pattern, repository files, the git wiring procedure | senior-d |
| 010 | documentation site and its two languages | senior-d |
| 011 | ingest and media: import, probe, profiles, artifacts, analysis, transcription | senior-b |
| 012 | editor UI architecture: thin-client rules, state, timeline, preview, command coverage | senior-e |
| 013 | the renderer: the plan as input, the pixel contract, the module and its hosts, frames from WebCodecs and FFmpeg, the proxy chunk index, errors and limits (Phase 5) | senior-b |
| 014 | the studio kit: plugin layout, skills, production folder, hand-off manifest and memory | senior-d |

Every SDD except SDD-000 (names only) ends with an Acceptance section; the acceptance of SDD-002 is its golden cases (§A.11 and Part C).

## Research notes

The evidence behind the decisions is kept privately by the maintainers (since D-037); documents cite a note as "research note `<slug>`". Where a research note and an ADR disagree, the ADR prevails: notes record what was found before decisions were taken.

## Rules for changing these documents

1. A name changes in SDD-000 first.
2. Code that must differ from an SDD changes the SDD in the same pull request.
3. A decision of the author changes only with the author; add a new `D-NNN`, do not rewrite an old one.
4. An ADR is not edited to reverse it: write a new ADR that supersedes it and mark the old one.
5. Examples inside the SDDs are extracted and validated by tests (SDD-008 §6), so keep them valid.
