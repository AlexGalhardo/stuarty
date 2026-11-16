# AGENTS.md: how the team works on Vukem Video

Read this file first, then the current state at the end of `development/PROMPT.md` and the open cards of `development/TODO.md`. Together with `docs/` and `.agents/rules/` they are enough to start working without asking the author anything.

## 1. What this is

**Vukem Video** is a minimalist, open source (MIT) video editor, web first and desktop later, built to be operated by AI agents (through MCP, a CLI and an HTTP API) as first-class users. A person edits the same project in a browser UI. Both go through one command core, so a human and an agent see the same result. Two use cases drive every priority: short vertical videos with captions for social networks, and product commercials.

The product is built in public releases (`CHANGELOG.md`). What defines it: research (`development/search/`, private) and decisions of the author (`development/brainstorming.md`, `D-NNN`, private; D-037), requirements (`docs/prds/`), architecture decisions (`docs/adrs/`), specifications (`docs/sdds/`). What is left to do: new work in the epics and task cards of `development/TODO.md`, and fixes in the issue queue of `development/feedbacks/vukem-video.md` (rule `.agents/rules/task-cards.md`).

## 2. Where things are

```text
C:/dev/vukem-video/                 THE PRODUCT REPOSITORY (remote AlexGalhardo/vukem-video)
├── AGENTS.md                       this file
├── CLAUDE.md                       points here
├── .agents/                        rules/ (always apply) and skills/ (procedures); start with .agents/README.md
├── development/                    local work files only, git-ignored, in Portuguese (D-036, rule development-folder.md)
│   ├── README.md                   the map of the folder
│   ├── TODO.md                     new work: epics and task cards (rule task-cards.md)
│   ├── PROMPT.md                   the prompt that starts a work session; its last section is the current state
│   ├── QUESTIONS.md                questions for the author asked while they were away, with the decision taken
│   ├── feedbacks/                  vukem-video.md (product issues VV-NNN), vukem.md (framework frictions VF-NNN)
│   ├── tests/                      PLAN.md (QA campaign and benchmark history), inventory/, assets/, bench/, results/
│   └── scripts/                    traceability.py, build-todo.py
├── plans/                          advisor plans of the improve skill, git-ignored
├── docs/                           prds/, adrs/, sdds/, README.md (index), traceability.md, design-system.md
├── scripts/                        versioned scripts, gate.sh (the full local release gate: task gate)
├── spikes/                         disposable experiments, never promoted to product code
└── web/, core/, engine/, service/, site/, desktop/ ...  the product
```

- Paths inside task cards are relative to the repository root.
- `development/` is git-ignored (D-036, which replaces D-025): nothing in it is published, and nothing outside it may reference it. Its layout, names and languages are fixed by `.agents/rules/development-folder.md`; names elsewhere by `.agents/rules/naming.md`.
- A clone of the vukem framework, if kept, lives outside this repository; never commit it. Parallel worktrees live in `C:/dev/vukem-video-worktrees/`.
- After task P1-REL-003 the product has `core/`, `engine/`, `service/`, `web/`, `site/` (ADR-0001).

## 3. The rules that are never traded

1. **One command core.** All editing logic lives in `core/` (Go, pure, no I/O). The UI, the MCP server, the CLI and the HTTP API are thin adapters over `service.Service`. A rule about timelines written in a handler, a tool or in TypeScript is rejected in review, even "for now" (ADR-0002, ADR-0007, ADR-0008).
2. **The UI never computes an edit result.** It draws what the core says and sends commands. `bun run check:thin-client` enforces it (SDD-012 §1).
3. **Names come from one place.** Operations, tables, error codes, variables: `docs/sdds/000-shared-definitions.md`. Change it first, then everything else.
4. **A specification is the contract.** When the code must differ from an SDD, fix the SDD in the same pull request and say why. Never let them drift.
5. **Only LGPL FFmpeg, as a separate process; pixels are composed by one renderer** (ADR-0003, ADR-0011, ADR-0029). Today that is the Go compositor; Phase 5 replaces it with the Rust renderer in `renderer/`, compiled to WebAssembly and run by the browser preview and, through `wazero` (no cgo), by the Go export. Rust lives only in `renderer/`; the plan (`core/plan`) stays in Go and the renderer decides nothing. No GPL component in anything we publish (`cargo-deny` uses the same allowlist as `internal/tools/notices`).
6. **No outbound network connection by default, no telemetry** (D-016, RF-206). Agents cannot make the product send data anywhere (ADR-0020).
7. **Releases follow the vukem pattern strictly** (SDD-009): SemVer tags, Conventional Commits, changelog generated from commits, signed artifacts. The legacy Studio is the single release `v0.1.0`; the editor continues `v0.2.0`, `v0.3.0` ..., one minor about every 90 commits on `main` and at every phase exit (D-026, ADR-0028). Release notes have only five sections: **Added** (`feat`), **Changed** (a breaking change of a public interface), **Removed**, **Fixed** (`fix`), **Security**; no introduction, nothing else (D-027).
8. **Everything in the repository is in English** (D-008); the git-ignored `development/` is in Portuguese, for the author (D-036). The conversation with the author is in Brazilian Portuguese.
9. **No new dependency without the author's approval.** JavaScript uses Bun in `web/`; the documentation site keeps npm (it already uses it). Exact versions, never `latest`, `next`, `rc`, `beta`, `alpha`. Docker images use fixed tags; any HTTP server in an image is Caddy. Rust (D-035): the stable toolchain pinned in `rust-toolchain.toml`, `Cargo.lock` committed; approved crates and tools are `wgpu`, `wasm-bindgen` (and its CLI), `bytemuck`, `cargo-deny`, and `wazero` on the Go side; any other crate needs the author's approval.
10. **vukem frictions are recorded when found**, in `development/feedbacks/vukem.md`, format `VF-NNN`, each checked against the vukem source.

## 4. Roles

| Role | Who | Does | Model |
| --- | --- | --- | --- |
| Orchestrator | the main session | plans the session, writes the briefings, reviews and audits every delivery, fixes the last defects itself, runs git wiring, merges, pushes and releases, marks `[x]` on the cards of `development/TODO.md` | the model chosen with the author at session start |
| Senior | `senior-a` to `senior-e` | implements: features, fixes, migrations, tests | Sonnet |
| Intern | `estagiario-1`, `estagiario-2` | mechanical, checkable work: translations, catalogue keys, running a suite and reporting, building fixtures, gathering data | Haiku |

Each senior has a stable area, so that two never edit the same folder:

| Senior | Area | Folders |
| --- | --- | --- |
| senior-a | command core and schema | `core/` |
| senior-b | render, export, ingest, transcription, the Rust renderer | `engine/`, `renderer/` |
| senior-c | service, persistence, HTTP, events, jobs, auth | `service/`, `web/app/` (except `mcp`, `cli`, `agentdocs`), `web/routes/`, `web/database/`, `web/cmd/` (except `gen-agent-docs`), `internal/outbound/` |
| senior-d | agent surfaces, documentation site, releases and CI | `web/app/mcp/`, `web/app/cli/`, `web/app/agentdocs/`, `web/cmd/gen-agent-docs/`, `web/contract/`, `site/`, `.github/`, `.config/`, `internal/tools/`, `scripts/`, `licenses/`, root files |
| senior-e | editor UI, desktop shell | `web/resources/`, `web/scripts/`, `web/package.json`, `web/biome.json`, `web/e2e/`, `desktop/` |

Generated folders are written only by the generators. A task with more than one owner is one branch on which the owners work one after the other. Interns write no code: scripts, tests and scenarios belong to the senior named in the card. A card that crosses two areas is split into one card per owner (`.agents/rules/task-cards.md` section 2).

Dispatch by the named type (`senior-a`, never the generic `senior`), and never reuse a name while an agent with that name is alive. Stop an agent as soon as its delivery is reviewed, together with any process it left running.

## 5. Starting a session

1. Read the current state at the end of `development/PROMPT.md`, this file, `.agents/rules/` and the open cards of `development/TODO.md`, highest priority first (`P0` before `P1`).
2. Follow the author's global instructions for the session start (model and effort are chosen with the author; questions go through the question tool, one at a time, with a recommended option first).
3. Pick the next cards whose `Depende de` are marked and whose `Arquivos` do not intersect. One agent per card. In parallel, one senior is the feedback agent: it fixes the open issues of `development/feedbacks/vukem-video.md` one at a time, lowest number first (`.agents/rules/task-cards.md` section 5). Parallel agents and worktrees need the author's explicit yes.
4. For each card, write a briefing (section 6), dispatch, and keep working on what does not depend on the answer.
5. At the end, or before a context compaction, refresh the last section of `development/PROMPT.md` (the current state): cards done, commands that validate, decisions taken, what is open.

## 6. A briefing

A delegated engineer knows only what the briefing says. It contains:

- the card copied whole from `development/TODO.md` (id, type, origin, context, files, commit, asserts); the engineer reads the `Contexto` sections first;
- the goal in one sentence and why it matters;
- the files and folders it owns, and the ones it must not touch;
- the acceptance: the asserts of the task, as commands to run;
- what was already ruled out, and decisions of the author that apply;
- the standing rules: no push, no new dependency, no forbidden git command, no process left running, record vukem frictions, report what was not run;
- the report format: the commit hash; each assert, done or not done with the reason; the commands run and their result; anything that contradicts a specification.

## 7. Review and completion

The protocol is in `.agents/rules/task-cards.md` section 3. In short: a report is a claim, not evidence. The orchestrator reads the diff (`git diff --stat` first, and reverts anything outside the briefing), runs the asserts, fixes or sends back, and only then marks the card `[x]` with its `Status` line (date, commit, evidence). One pull request per release (D-031): the branch `release/vX.Y.Z` collects the task branches (the orchestrator cherry-picks or fast-forwards them in, never force pushes), the pull request is titled `chore(release): vX.Y.Z updates`, holds about 90 microcommits, has its body refreshed with `task release:pr-body -- vX.Y.Z` (Added, Changed, Removed, Fixed, Security with linked commit hashes) and is rebase-merged so every microcommit reaches `main` (ADR-0028). Seniors branch from the open `release/vX.Y.Z` and never open pull requests; each commit is small and self-contained. Nobody else pushes. `main` is protected by the ruleset `protect-main`: no direct commit or push to `main`, ever, including releases: every change, the release commit of `v0.3.0` and later included, goes through a pull request whose required checks are green and which is up to date with `main`.

The author confirms before: the first push of the repository, a release tag, repository settings, secrets, anything paid, a new dependency.

## 8. Git and shell

- Never `git stash`, `git checkout -- .`, `git reset --hard`, or a force push.
- Chain with `&&`; never `;` before a push or a deploy.
- Edit versioned files with the edit tools, not with `sed` or redirection.
- Windows: expect CRLF, re-read a file before repeating a failed edit, keep apostrophes out of shell commands, use the session scratchpad or `$TEMP` and never `/tmp`.
- Do not read lockfiles, `node_modules/`, build outputs or generated files.

## 9. Commands

| Command | Does |
| --- | --- |
| `task lint` | Go, TypeScript and Markdown linters |
| `task test` | Go tests (short mode, no race detector) |
| `task test:race` | Go tests of core, service and web/app with the race detector |
| `task test:web` | the web unit tests (Vitest) |
| `task ci` | everything the pull-request pipeline runs |
| `task codegen` | regenerates schemas, TypeScript types and the registry snapshot |
| `bun run check:thin-client` (in `web/`) | the thin-client rules |
| `go test -run TestSecurity_ ./...` | the security suite |
| `task gate -- <tag>` (run inside the worktree) | the full local release gate (lint on three GOOS, tidy, notices, threatcheck, ffmpegpins, agent docs, Go tests, security suite, web, Markdown, leaks, site, commits); used instead of CI while the pull-request workflows are disabled until the BETA exit |
| `python development/scripts/traceability.py` | regenerates `docs/traceability.md` from the PRDs, the SDDs and the cards of `development/TODO.md` |
| `bash scripts/build-renderer.sh` (after P5-REL-001) | builds the Rust renderer natively and to WebAssembly, reproducibly |
| `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` (in `renderer/`) | the Rust gate (rule `.agents/rules/rust.md`) |

Phase 5 (Renderer evolution, ADR-0029): read `.agents/rules/rust.md` and `.agents/skills/renderer-parity/` before touching `renderer/`, the preview executor or the export compositor.

Working conventions learned in BETA (details in `.agents/skills/` and `.agents/rules/`): at most five seniors at once; performance budgets are measured only with every agent stopped and the machine idle; a test that fails on a line is checked against the previous tag in a detached worktree before it is called a regression; build inputs are pinned by digests measured from real downloads; a release can be prepared as a GitHub draft before it is cut.

## 10. When something is unclear

Look in this order: the SDD named in the task, `000-shared-definitions.md`, the ADR, `development/brainstorming.md`. If two documents disagree, the ADR wins over a research note, SDD-000 wins on names, and the more specific SDD wins on behaviour; fix the loser in the same pull request. If the answer is nowhere, it is a decision: take the simplest option that meets the requirement when it is cheap to undo, record it in the SDD, and ask the author only when it is irreversible or costs money.
