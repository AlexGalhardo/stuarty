# Stuarty: planning documents

Everything needed to build Stuarty, written so that an engineer who was not in the planning conversation can work from it. Read in this order: the four PRDs (what and why), the ADR list (how, in one page each), the roadmap, then the epic you were given and the SDD of the area you are about to touch.

Work is driven by the epics of `plans/` (index `plans/00-roadmap.md`): one epic is one unit of work that one senior engineer takes from its first task to its review (rule `.agents/rules/plans.md`, process ADR-0045). The other folders hold what defines the product. Names and numbering of the documents follow `.agents/rules/naming.md`: `prds/NNN-<slug>.md`, `adrs/NNNN-<slug>.md`, `sdds/NNN-<slug>.md`, `plans/NN-<slug>.md`. Decisions of the author (`D-NNN`) and research notes are kept by the maintainers outside version control and are cited by id or name only.

## Status of these documents

The product was a video editor called Vukem Video until 2026-11-18, when it became Stuarty (ADR-0031). What that means for a reader:

- **The PRDs and the ADRs are current.** ADR-0031 to ADR-0048 record the turn; the status line of every older ADR says what amended or superseded it.
- **The SDDs specify the video editor as built, up to release 0.29.0.** Each one that the turn affects starts with an "Amended" line that names the ADRs to read with it. A new domain (image, publishing, providers and the others) gets its specification from the epic that builds it, before code.
- **The former name** is in SDD-000 section 13, with the old variables, folder and identifiers that are still read (ADR-0044). Four SDDs (SDD-000, SDD-002, SDD-003, SDD-007) are read by tests and by the site generator.
- **`security-review-1.0.md` and `agent-benchmark.md` are records** of a review and of benchmark rounds of the editor; they are not rewritten.
- **`security-review-stuarty.md` and `quality/` belong to the quality campaigns** of epic 42 (SDD-008 section 13): the security review of Stuarty, kept open until its findings are closed, and the dated reports of the fuzz, soak and other campaigns.

## Reading map

| You want to | Read |
| --- | --- |
| know what Stuarty is and what is in or out of scope | `prds/004-creator-platform.md`, then `adrs/0031-*.md` |
| know what the video editor must do | `prds/001-video-editor.md` |
| know what to build next, in what order, and how to verify it | `plans/00-roadmap.md`, then the epic you were given |
| know how an agent uses the product | `prds/002-agent-operability.md`, then `sdds/003-agent-interfaces.md` |
| know what may leave the machine, and what waits for a person | `adrs/0032-*.md`, then `sdds/007-security-model.md` |
| know what must be true of the accessibility of every section, and the script of the screen reader pass | `sdds/012-ui-architecture.md` section 18.10, `accessibility-screen-reader-script.md` |
| use a name (operation, table, error code, variable) | `sdds/000-shared-definitions.md` |
| change the timeline document format | `sdds/001-timeline-schema.md` |
| add or change a command of the video editor | `sdds/002-command-catalogue.md` |
| understand why preview and export agree | `adrs/0003-*.md`, `adrs/0029-*.md`, `sdds/004-render-pipeline.md`, `sdds/013-renderer.md` |
| know why a decision was taken | `adrs/` |
| follow the recipe of a product commercial or of a long talking video in Portuguese | `guides/pt-BR/recipe-product-commercial.md`, `guides/pt-BR/recipe-long-talking-video.md` (the English pages are in `site/content/docs/en/guide/`) |
| design or review a screen (tokens, components, content style, accessibility) | `design-system.md`, then `sdds/012-ui-architecture.md` |
| see how well small and large AI agents use the editor, round by round | `agent-benchmark.md` |

## Requirements (`prds/`)

| File | Covers |
| --- | --- |
| `001-video-editor.md` | the video editor: projects, timeline, picture, text and captions, audio, templates, preview and export, UI, and the motion, audio and export scope of ADR-0042; RF-001 to RF-089, NFR-001 to NFR-013 |
| `002-agent-operability.md` | MCP, CLI, HTTP API, text views, verification, revert and checkpoints, safety and approval; RF-100 to RF-137, NFR-020 to NFR-025 |
| `003-distribution-and-docs.md` | installation, local and network modes, the secret vault, the VPS package, the desktop application, the documentation site; RF-200 to RF-221, NFR-040 to NFR-046 |
| `004-creator-platform.md` | the production project, the dashboard, assets, the image editor, writing, AI providers, the agent chat, memory, skills and prompts, connections and publishing, blog; RF-300 to RF-423, NFR-060 to NFR-066 |

Which epic delivers each requirement is in the traceability table of `plans/00-roadmap.md`.

## Architecture decisions (`adrs/`)

| ADR | Decision |
| --- | --- |
| 0001 | one repository, one Go module: `core/`, `engine/`, `service/`, `web/`, `site/`, `desktop/` (name amended by 0031 and 0044) |
| 0002 | the command core is Go, pure, the only place editing logic lives |
| 0003 | render: one plan, two executors, truth on pause (items 3 and 4 amended by 0029) |
| 0004 | time is integer frames at a rational rate; times cross the boundary as strings with a unit |
| 0005 | timeline data model: tracks, items with absolute position, captions with words, keyframes (the project around it: 0039) |
| 0006 | operation log, revisions, conflicts per object, undo per actor by compensating batch |
| 0007 | the UI is a thin client |
| 0008 | one service boundary; one owner process per workspace (amended by 0046) |
| 0009 | persistence: SQLite, the document and an append-only log (amended by 0039) |
| 0010 | content-addressed media store; ingest produces normalized media, proxy, thumbnails, peaks |
| 0011 | FFmpeg: separate process, LGPL build only, encoder selection, argument policy |
| 0012 | text and shapes rasterized with bundled open fonts |
| 0013 | browser stack: React, a canvas compositor behind an interface, `<video>` proxies |
| 0014 | contracts in JSON Schema, generated to TypeScript and to the reference |
| 0015 | MCP server: tools named by intent, stateless (extended by 0043) |
| 0016 | CLI on vukem commands, JSON output, exit codes 0, 1, 2 |
| 0017 | HTTP API under `/api/v1` and server-sent events |
| 0018 | jobs: own table over the vukem queue, with progress and cancellation (amended by 0046) |
| 0019 | authentication: local trust on loopback, login and scoped keys on the network (amended by 0034) |
| 0020 | agent safety: trash, limits, marked untrusted text (item 1, no outbound capability, superseded by 0032) |
| 0021 | transcription behind a provider interface; whisper.cpp local by default (part of 0036) |
| 0022 | templates are parameterized projects |
| 0023 | the desktop is a shell around the same binary and the same UI: Wails v3 (confirmed by 0041) |
| 0024 | documentation site: static, two languages, generated reference |
| 0025 | releases and CI in the vukem pattern (deferred by 0045) |
| 0026 | observability without telemetry |
| 0027 | items of the stack draft that were cut or deferred, each with its reason (amended on 2026-11-18) |
| 0028 | release cadence and rebase-merged release pull requests (**suspended** by 0045) |
| 0029 | one frame plan (Go), one renderer (Rust in WebAssembly), two hosts; WebCodecs and WebGPU in the preview |
| 0030 | desktop editor as a Gio window (**superseded** by 0041) |
| 0031 | Stuarty is one product with several domains over one service; work is a queue of epics |
| 0032 | outbound only to connected destinations; publishing and costly generation wait for a person |
| 0033 | secrets in an encrypted file opened by a master key |
| 0034 | login stays local; a Google account is an optional connection with the user's own client |
| 0035 | social networks through the user's own developer apps; the order of the networks |
| 0036 | one AI provider layer with four ways to connect; what is never done |
| 0037 | the chat hosts the user's own agent over ACP; no built-in agent |
| 0038 | image editing is a second document type on the same core and the same renderer |
| 0039 | a project is the container of a whole production |
| 0040 | memory, skills and prompts are files in the workspace, with an index |
| 0041 | one web UI; the desktop is the Wails shell; the Gio window is removed |
| 0042 | the video editor's scope for motion, audio and export |
| 0043 | the MCP server keeps a few tools named by intent, with a domain |
| 0044 | the code is renamed to Stuarty in one epic, with a workspace migration |
| 0045 | work is epics in a loop on one draft pull request; CI and releases are deferred |
| 0046 | publishing is a scheduled, approved job; blog posts have four destinations |
| 0047 | the vukem framework stays, confined to the web layer and the process runner |
| 0048 | the release train: linear release pull requests of about 90 commits, rebase-merged by the orchestrator |

## Specifications (`sdds/`)

| SDD | Specifies | Amended on 2026-11-18 by |
| --- | --- | --- |
| 000 | shared definitions: the names everything uses | ADR-0031, ADR-0044 |
| 001 | timeline schema, invariants, examples, versioning | — |
| 002 | command catalogue of the video domain: the model, the commands, each command in detail with its golden cases | ADR-0031, ADR-0045 |
| 003 | agent interfaces: the `Service` interface, text formats, MCP tools, CLI, HTTP API, parity | ADR-0037, ADR-0043 |
| 004 | render pipeline: frame plan, rasterizer, compositor, audio, export, frames, preview executor, tolerance | — |
| 005 | data model and persistence: tables, the commit transaction, undo, trash, jobs, workspace ownership | ADR-0033, ADR-0039, ADR-0046 |
| 006 | real-time protocol: events, ordering, replay, reconciliation, conflicts between a human and an agent | — |
| 007 | security model: threats, local and network trust, scopes, process policy, paths, untrusted text, limits | ADR-0032 to ADR-0037, ADR-0046 |
| 008 | test strategy: kinds of test, golden cases, golden frames and tolerance, scenarios, parity, performance | — |
| 009 | releases and CI/CD, repository files | ADR-0044, ADR-0045 |
| 010 | documentation site and its two languages | — |
| 011 | ingest and media: import, probe, profiles, artifacts, analysis, transcription | — |
| 012 | editor UI architecture: thin-client rules, state, timeline, preview, command coverage, the desktop shell | ADR-0041 |
| 013 | the renderer: the plan as input, the pixel contract, the module and its hosts | ADR-0038 |
| 014 | connections and the secret vault: the record, the encrypted file, the master key, the outbound gate | ADR-0032, ADR-0033 |
| 015 | the approval queue, rules and cost ceilings: the decision, the requests, the jobs that wait, the ledger | ADR-0032, ADR-0018 |
| 020 | publishing: the connector frame, the sign-in with the user's own app and the YouTube connector | ADR-0032, ADR-0035 |
| 021 | image documents: the document, the catalogue of layer operations, the plan, export, templates, lint, storage | ADR-0038 |
| 022 | outbound webhooks: the record, the events and their metadata payload, signature, delivery and retries, the address rules | ADR-0032, ADR-0033 |
| 018 | memory, skills and prompts: the files, front matter, review, the skill lint, import and export, the index and search | ADR-0040 |
| 017 | the hosted agent chat: the protocol, profiles, the process, sessions, permission requests, streaming, transcripts | ADR-0037, ADR-0036 |

## Rules for changing these documents

1. A name changes in SDD-000 first.
2. Code that must differ from an SDD changes the SDD in the same change.
3. A decision of the author changes only with the author; a new `D-NNN` is added, an old one is not rewritten.
4. An ADR is not edited to reverse it: write a new ADR that supersedes or amends it and say so in the status line of the old one.
5. Examples inside the SDDs are extracted and validated by tests (SDD-008 §6), and SDD-000, SDD-002, SDD-003 and SDD-007 are read by tests and tools: keep them valid, and run those tests after any edit.
6. An epic of a new domain writes that domain's specification as its first task.
