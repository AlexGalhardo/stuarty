# AGENTS.md: how the team works on Stuarty

Read this file first, then the roadmap in `docs/plans/00-roadmap.md` and the rules in `.agents/rules/`. Together with `docs/` they are enough to start working without asking the author anything.

## 1. What this is

**Stuarty: create, edit, manage and publish your content in one place.** An open source (MIT), self-hosted studio, web and desktop, for independent creators, with AI agents as first-class users: everything a person does in the UI is possible through the HTTP API, the CLI and MCP. Files, tokens and credentials stay on the user's machine or on the user's own server.

It grew out of a video editor called Vukem Video. The editor (a command core in Go, a renderer in Rust, MCP, CLI and HTTP API with tested parity) is what exists; the other domains (projects as productions, the dashboard, image editing, writing, AI providers, the agent chat, publishing, memory, skills and prompts) are built by the epics of `docs/plans/`. The first complete flow to deliver is a short vertical video with captions and a thumbnail, from the briefing to the publication (ADR-0031, PRD-004 section 3).

What defines the product: requirements (`docs/prds/`), architecture decisions (`docs/adrs/`), specifications (`docs/sdds/`), and the decisions of the author (`D-NNN`, private, cited by id). What is left to do: the epics of `docs/plans/`, in the order of `docs/plans/00-roadmap.md`. There are no phases and no milestones (ADR-0045).

**The name in the code.** The product was renamed in one epic (ADR-0044): the Go module, the binary, the variables (`STUARTY_*`) and the workspace folder (`~/.stuarty`) say `stuarty`. The former names are still read for a deprecation period (SDD-000 section 13). The specifications SDD-000, SDD-002, SDD-003 and SDD-007 are read by tests and by the site generator: their names change only together with the code.

## 2. Where things are

```text
the repository (remote AlexGalhardo/stuarty)
├── AGENTS.md                       this file
├── CLAUDE.md                       points here
├── .agents/                        rules/ (always apply) and skills/ (procedures); start with .agents/README.md
├── .claude/agents/                 the seven agents of the team (section 4)
├── development/                    local work files only, git-ignored, in Portuguese (rule development-folder.md)
├── docs/                           prds/, adrs/, sdds/, plans/ (the epics and 00-roadmap.md), README.md (index), design-system.md
├── core/                           the pure rules of every domain (Go, no I/O)
├── engine/                         media: FFmpeg, ingest, export, transcription, the renderer hosts
├── renderer/                       the pixel renderer (Rust, compiled to WebAssembly)
├── service/                        the Service interface and its implementation
├── web/                            the server (vukem), the MCP server, the CLI, the web UI in web/resources/
├── desktop/                        the Wails shell, packaging, the update feed
├── site/                           the documentation site
├── internal/, scripts/, licenses/  build tools, scripts, third-party licences
└── spikes/                         disposable experiments, never promoted to product code
```

- The work: `docs/plans/NN-<slug>.md`, one file per epic, in English, with the index `docs/plans/00-roadmap.md` (rule `.agents/rules/plans.md`).
- `development/` is git-ignored: nothing in it is published, and nothing outside it may reference it, except this file, `CLAUDE.md` and `.agents/`. The current state of the work is in `development/STATE.md` (read it first); questions waiting for the author in `development/QUESTIONS.md`; decisions of the author in `development/brainstorming.md`; the open product issues in `development/feedbacks/vukem-video.md`; research notes in `development/search/`; the state of the code on 2026-11-18 in `development/context/estado-atual.md`. How the folder is used, and who may write what in it, is the rule `.agents/rules/development-folder.md`; the folder is kept in step with the project, so what a change makes stale there is updated or deleted in the same change.
- There is one UI: the desktop application is the Wails shell over the same web UI (ADR-0041). A page is written once in `web/resources/`.

## 3. The rules that are never traded

1. **One command core.** The rules of every domain live in `core/` (Go, pure, no I/O). The UI, the MCP server, the CLI and the HTTP API are thin adapters over `service.Service`. A rule written in a handler, a tool or in TypeScript is rejected in review, even "for now" (ADR-0002, ADR-0007, ADR-0008, ADR-0031).
2. **The UI never computes an edit result.** It draws what the core says and sends commands. `bun run check:thin-client` enforces it (SDD-012 §1).
3. **Names come from one place.** Operations, tables, error codes, variables: `docs/sdds/000-shared-definitions.md`. Change it first, then everything else.
4. **A specification is the contract.** When the code must differ from an SDD, fix the SDD in the same change and say why. A new domain gets its specification before its code.
5. **Only LGPL FFmpeg, as a separate process; pixels are composed by one renderer** (ADR-0003, ADR-0011, ADR-0029, ADR-0038). The Rust renderer in `renderer/`, compiled to WebAssembly, draws video frames and image documents in the browser and, through `wazero` (no cgo), in the export. Rust lives only in `renderer/`; the plan stays in Go and the renderer decides nothing. No GPL component in anything we publish.
6. **Outbound only to connected destinations; publishing waits for a person; no telemetry** (ADR-0032). With no connection configured, the product opens no outbound connection. An agent can prepare a publication or a costly generation and cannot run it without the approval queue or a rule the user wrote. Credentials live only in the vault and no tool returns one (ADR-0033).
7. **The user brings their own access.** Social networks through the user's own developer apps (ADR-0035); AI through an API key, Sign in with ChatGPT, OpenRouter or the user's local agent (ADR-0036). Never a subscription login a provider forbids, never automation of a service with no API, never a central Stuarty service.
8. **Everything in the repository is in English**; the git-ignored `development/` is in Portuguese. The conversation with the author is in Brazilian Portuguese.
9. **No new dependency without the author's approval.** JavaScript uses Bun everywhere (`web/` and `site/`); npm and Node only as a fallback where Bun has no native support or fails, each such place commented with the reason (D-115). Exact versions, never `latest`, `next`, `rc`, `beta`, `alpha`. Docker images use fixed tags; any HTTP server in an image is Caddy. Rust: the stable toolchain pinned in `rust-toolchain.toml`, `Cargo.lock` committed, approved crates listed in `.agents/rules/rust.md`.
10. **vukem stays, and its frictions are recorded when found** (ADR-0047). Every friction with the framework and every idea to improve it goes to `development/feedbacks/vukem.md`, format `VF-NNN`, checked against the vukem source. Never work around the framework silently.
11. **`main` changes only through the release train** (ADR-0048): approved work joins the open release pull request as linear commits, and the orchestrator rebase-merges each release when it holds about 90 commits, then opens the next one. Never a direct push or commit to `main`, never a force push, no tag pushed until CI is back. Only the orchestrator pushes, only the open release branch, only approved work.

## 4. Roles

Seven agents, defined in `.claude/agents/`.

| Role | Agent | Model and effort | Does |
| --- | --- | --- | --- |
| Orchestrator | the main session | chosen with the author | picks the next epics, writes the briefings, dispatches, consolidates, merges approved epics into the integration branch, marks the roadmap, decides when the author is needed |
| Senior | `senior-a` to `senior-e` | Claude Sonnet, medium | implements one epic at a time, from the first task to the hand-off: features, fixes, migrations, tests, the specification of its domain |
| Intern | `intern` | Claude Haiku | mechanical, checkable work: running tests and reporting, reports, small lookups, i18n, fixtures |
| Specialist | `specialist` | Claude Opus, medium | reviews each delivery with no prior context, fixes the bugs it finds and approves |

- **A senior owns one epic at a time**, not a folder. Two epics that touch the same modules are never in progress together; the roadmap says which epics can run side by side.
- **Two tracks run in parallel**: the editor (two seniors) and the platform (three seniors), after a short sequential foundation (ADR-0045).
- **The specialist starts from zero.** It receives only the epic, its acceptance criteria and the diff: never the senior's report, the orchestrator's opinion or the conversation. That is what keeps the review free of the implementer's bias.
- The intern writes no product code.
- Dispatch by the named type, and never reuse a name while an agent with that name is alive. Stop an agent as soon as its delivery is reviewed, together with any process it left running.

## 5. Starting a session

1. Read this file, `.agents/rules/`, `docs/plans/00-roadmap.md` and the current state in `development/STATE.md`, then `development/QUESTIONS.md` for what the author answered.
2. Follow the author's instructions for the session start (model and effort; questions through the question tool, one at a time, with a recommended option first).
3. Pick the next epics whose dependencies are done and whose modules do not intersect: at most five seniors at once. Parallel agents and worktrees need the author's explicit yes; the work-session prompt carries it.
4. For each epic, write a briefing (section 6), dispatch, and keep working on what does not depend on the answer.
5. After every event (an epic dispatched, handed off, reviewed, merged or blocked), at the end, and before a context compaction, rewrite `development/STATE.md`: what each agent is doing, epics done, commands that validate, decisions taken, what waits for the author.

## 6. A briefing

A delegated engineer knows only what the briefing says. It contains:

- the path of the epic in `docs/plans/`; the engineer reads the whole epic and the documents it links first;
- the goal in one sentence and why it matters to a user;
- the branch and the worktree to work in, the modules it owns, and what it must not touch;
- the acceptance criteria, as commands to run;
- what was already ruled out, and decisions of the author that apply;
- the standing rules: no push, no new dependency, no forbidden git command, no process left running, record vukem frictions, report what was not run, stop and ask instead of improvising when a STOP condition of the epic is true;
- the hand-off format of the epic: the commits, each acceptance criterion as done or not done with the reason, the commands run and their result, how to run and how to test, anything that contradicts a specification.

## 7. Review and completion

The protocol is `.agents/rules/plans.md` section 3, and its step-by-step procedures are three skills of `.agents/skills/`: `orchestrator-loop` (the session), `epic-loop` (one epic, for its senior) and `qa-campaign` (the rounds after the epics). In short:

1. **The senior works the epic in a loop**: read the epic; do task 1; run the command that verifies task 1; if it fails, fix and run it again; only when it passes, commit and move to task 2. When every task is done and every acceptance criterion passes, write the hand-off.
2. The orchestrator opens a **new** specialist session with only the epic, its acceptance criteria and the diff of the epic branch against the integration branch.
3. The specialist runs every acceptance criterion and the checks again, reads the diff against `REVIEW.md`, fixes what it finds in new commits, and approves or sends back what is too large to fix in review.
4. Only then the orchestrator integrates the epic branch into the open release branch (`release-train.sh integrate`, linear commits), pushes it so the release pull request shows the finished epic, ticks the epic in the roadmap with the date and the commits, and stops the agents. At about 90 commits it cuts the release: changelog, `chore(release)`, rebase-merge into `main`, next release branch (`skills/release-pr/`).
5. A delivery that fails review twice stops: the orchestrator marks the epic blocked, records why, and moves to the next unblocked epic; the author decides.

**When every epic is done, the QA campaign runs in a loop** (ADR-0045 item 8): the five seniors test the whole product in parallel, end to end, each on its own surface (every CLI command, the HTTP API, the MCP tools, every control of the UI, the desktop application, the installation); each problem becomes a `VV` entry, is fixed with a reproducing test and reviewed by the specialist; then a new round starts. The campaign ends when a full round finds no problem.

A report is a claim, not evidence. The Definition of Done of every epic: its tests pass, `task lint` and the type checks are clean, the documents it changes are updated (the SDD of its domain, the roadmap, the user documentation when behaviour changes), and every acceptance criterion was verified by the specialist. CI is not a gate while the epics are implemented (ADR-0045): the evidence is the local checks.

The author confirms before: a push or a merge to `main` outside the release train, a pushed tag, repository settings, secrets, anything paid, a new dependency, a decision that contradicts an ADR, a change of the scope of an epic. In an unattended session these are STOP conditions: record the question with the options and a recommendation, leave that epic blocked, and continue with work that does not depend on the answer.

## 8. Git and shell

- The integration branch is the open release branch, `release/vX.Y.Z`, with its pull request against `main` (`bash .agents/skills/release-pr/scripts/release-train.sh status`); it is linear and every approved commit is saved there. Each epic is a branch from it, in its own worktree outside the repository folder (`.agents/rules/git-and-branches.md`). The orchestrator integrates and pushes after each approved epic and cuts the release at about 90 commits; nobody else pushes, and nothing else is pushed. The former integration branch `release/0.30.0` (draft pull request #106) is a frozen backup.
- Conventional Commits, one small self-contained commit per task of the epic, with `Refs: epic NN task N`.
- Never `git stash`, `git checkout -- .`, `git reset --hard`, or a force push.
- Chain with `&&`; never `;` before a push.
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
| `task codegen` | regenerates schemas, TypeScript types and the registry snapshot |
| `bun run typecheck` and `bun run check:thin-client` (in `web/`) | the TypeScript types and the thin-client rules |
| `bun run e2e` (in `web/`) | the browser tests (Playwright) |
| `go test -run TestSecurity_ ./...` | the security suite |
| `bash .agents/skills/verify/scripts/verify.sh --quick --base release/0.30.0` | the checks of what changed against the integration branch, before a commit (without `--base` it compares with `main`) |
| `bash scripts/build-renderer.sh` | builds the Rust renderer natively and to WebAssembly, reproducibly |
| `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` (in `renderer/`) | the Rust gate (rule `.agents/rules/rust.md`) |
| `task ci`, `task gate -- <tag>`, `verify.sh --full` | the full gates; not required while releases are deferred (ADR-0045), useful before a large merge |

Read `.agents/rules/rust.md` and `.agents/skills/renderer-parity/` before touching `renderer/`, the preview executor or the export compositor.

Working conventions learned so far: at most five seniors at once (the author's machine runs out of CPU and memory above that); performance budgets are measured only with every agent stopped and the machine idle; a test that fails on a line is checked against the previous tag in a detached worktree before it is called a regression; build inputs are pinned by digests measured from real downloads.

## 10. When something is unclear

Look in this order: the epic, the SDD of the domain, `000-shared-definitions.md`, the ADR, the PRD. If two documents disagree, a newer ADR wins over an older one it amends, an ADR wins over a research note, SDD-000 wins on names, and the more specific SDD wins on behaviour; fix the loser in the same change. If the answer is nowhere, it is a decision: take the simplest option that meets the requirement when it is cheap to undo, record it in the specification, and ask the author only when it is irreversible, costs money, contradicts an ADR or changes the scope of the epic.
