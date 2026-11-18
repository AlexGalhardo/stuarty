# Plans: the work of Vukem Video

This folder answers "what do we build next, in what order, and how does an agent execute and verify it". **One plan is one unit of work**: an agent (or a sequence of owners on one branch) executes it from start to delivery. Each plan is self-contained: an executor that reads only the plan, `AGENTS.md` and the files it names can do the work. The rule is `.agents/rules/plans.md`; it replaced the private task cards on 2026-11-18 (author decision D-046), whose open items became plans 011 and 020 to 034, each listing the former `TASK-NNNN` ids in its `Origin` line.

Written against commit `8787714a` (after release v0.29.0).

## 1. The direction in one page

- **Vukem Video stays an editing toolkit.** It has no intelligence and no integration with any outside service. It gives agents (MCP, CLI, HTTP API) and people the same editing operations over one command core. ADR-0020 (no outbound capability) does not change (author decision D-041).
- **There is one UI.** The desktop application (Wails v3, `desktop/shell/app/`) is a native window over the same web UI (Inertia and React, no HTMX) that the same binary serves (ADR-0023). A page is written once in `web/resources/` and appears in the browser and in the desktop app; only native menus, file dialogs, drops and updates belong to the shell.
- **Agents own everything "smart".** They generate images, video, voice, music and sound effects with the providers' own MCP servers or CLIs, download clips from links, choose the engine, and write the results as files.
- **The engine is the simplest one that does the job.** If a request can be done with Remotion or HyperFrames alone, the agent does it there. The editor is used to cut, caption, mix or adjust.
- **Every video has a production folder**; reusable short clips live in an evergreen clip library; learnings are Markdown files with an index (author decisions D-042, D-043, D-045).
- **The studio kit is a plugin in this repository** (`studio/`), in the Agent Skills format, versioned with the product.
- **Agent mistakes are recoverable in one step**: revert to a revision, then named checkpoints and a diff (author decision D-044).
- **Phase 5 continues**: the Rust renderer is finished and becomes the only renderer (plan 011).

## 2. Execution order and status

Ordered by value to the simplest use case first (short meme cuts of game footage for vertical social video), then the renderer, then the 1.0 backlog. Plans in different tracks may run side by side when their files do not intersect (parallel agents need the author's yes in each session).

### Track A: the agent studio

| Plan | Title | Owner | Effort | Depends on | Status |
| --- | --- | --- | --- | --- | --- |
| [001](001-studio-architecture-records.md) | Record the studio direction: ADR-0031, SDD-014, PRD-004 | orchestrator | S | — | DONE (2026-11-18) |
| [002](002-studio-plugin-skeleton.md) | The `studio/` plugin: layout, manifest, skill lint, editor skill, router skill | senior-d | M | 001 | TODO |
| [003](003-production-folder-and-memory.md) | Production folder and learnings memory as skills | senior-d, estagiario-1 | S-M | 002 | TODO |
| [004](004-meme-cut-pilot.md) | Pilot: a vertical meme cut from game footage, end to end | senior-d, estagiario-2, orchestrator | M | 003 | TODO |
| [005](005-agent-side-provider-skills.md) | Skills for agent-side generation: audio, images and video | senior-d, estagiario-1 | M | 003 | TODO |
| [006](006-clips-from-links-and-evergreen-library.md) | Clips from links (X, YouTube, Instagram, Reddit, TikTok) and an evergreen clip library | estagiario-1, orchestrator, senior-d | M | 003 | TODO |
| [007](007-revert-to-revision.md) | Revert a project to a revision in one batch | orchestrator, senior-c, senior-d, senior-e | M-L | 001 | TODO |
| [008](008-external-engines.md) | HyperFrames and Remotion as agent-side engines, and the hand-off into the editor | senior-d, orchestrator | M | 003, author approval of versions | TODO |
| [009](009-checkpoints-and-history-diff.md) | Named checkpoints and a diff between revisions | orchestrator, senior-c, senior-d, senior-e | M | 007 | TODO |
| [010](010-promo-and-authorial-recipes.md) | Recipes for product promos and authorial videos | senior-d, orchestrator | M | 004, 005, 006, 008 | TODO |
| [012](012-home-dashboard-with-agent-chat.md) | Home dashboard: left sidebar and a centred agent chat | designer, orchestrator, senior-c, senior-d, senior-e | L | author choice (step 1), 002 | TODO |

### Track B: the renderer

| Plan | Title | Owner | Effort | Depends on | Status |
| --- | --- | --- | --- | --- | --- |
| [011](011-finish-the-rust-renderer.md) | Finish the Rust renderer (Phase 5) and make it the only renderer | senior-b, senior-e, senior-d, orchestrator | L | Linux and macOS runners, WebGPU in browser runs, idle machine | TODO |

### Track C: the 1.0 backlog (former task cards)

| Plan | Title | Owner | Depends on | Status |
| --- | --- | --- | --- | --- |
| [032](032-keyboard-shortcuts-and-final-ui.md) | Finish the final editor UI and prove the shortcuts and layout in the running product | senior-e, orchestrator | author's go for screen automation (shortcut step) | TODO |
| [033](033-animated-crop-and-mask-in-core.md) | Close the CapCut feature gaps in the core, starting with animated crop and mask | senior-a, senior-b | author decision on keyframeable crop and mask | BLOCKED |
| [025](025-core-and-service-reliability-campaigns.md) | Command core fuzz, crash tests and the one-hour soak of the service | senior-a, senior-c | Linux and macOS runners, idle machine | TODO |
| [030](030-ci-gates-and-cross-os-jobs.md) | Prove the CI gates, the nightly workflow and the cross-OS jobs | senior-d | real CI | BLOCKED |
| [021](021-engine-goldens-and-performance-three-os.md) | Golden frames and export performance on three operating systems | senior-b | runners, idle machine | BLOCKED |
| [020](020-ui-performance-reference-machine.md) | Editor performance and preview budgets on the idle reference machine | senior-e | idle machine, 021 | BLOCKED |
| [022](022-security-acceptance-and-fuzz.md) | Security acceptance on three systems, the long fuzz campaign, the independent review | senior-c | real CI | BLOCKED |
| [023](023-agent-surface-budgets-and-harness.md) | Agent surface budgets and the harness runs with a real model | senior-d | paid API key approved by the author | BLOCKED |
| [024](024-first-export-timing-and-site-audit.md) | First export in under ten minutes, the reference scenario on three systems, the site audit | senior-d, senior-e, orchestrator | real CI, a clean machine | BLOCKED |
| [028](028-screen-reader-pass-and-accessibility.md) | Screen reader pass (NVDA, VoiceOver) and WCAG 2.2 AA conformance | senior-e | a person with NVDA and VoiceOver | BLOCKED |
| [029](029-desktop-real-machines-and-update-feed.md) | The desktop shell on real machines, the shell verdict and the update feed | estagiario-1, senior-e, senior-d, orchestrator | real macOS and Linux hardware | BLOCKED |
| [026](026-ffmpeg-build-pipeline-and-signed-assets.md) | The own FFmpeg build pipeline, published and signed assets | senior-d | author decision on OpenH264 | BLOCKED |
| [027](027-adopt-own-ffmpeg-build.md) | Adopt the own FFmpeg build in the engine, image, notices and security tests | senior-b, senior-c, senior-d | 026 | BLOCKED |
| [034](034-model-ladder-pilot-round.md) | Pilot round of the model ladder campaign | orchestrator | the author asking for the round | TODO |
| [031](031-release-1-0-0.md) | Cut release v1.0.0 and audit the supply chain | senior-d, orchestrator | 021-025, 028-030; the author | BLOCKED |

Numbers 013 to 019 are free for new plans of tracks A and B. Status values: TODO, IN PROGRESS (agent, date), BLOCKED (reason), DONE (date, commits), REJECTED (reason). The orchestrator updates this table in the same commit as a plan's `Status` line.

## 3. Dependency graph of track A

```text
001 records ──► 002 plugin ──► 003 folder + memory ──┬──► 004 meme pilot
     │              │                                 ├──► 005 provider skills ──┐
     │              │                                 ├──► 006 clips + library ──┼──► 010 promo + authorial
     │              │                                 └──► 008 engines ──────────┘
     │              └──────────────────────────────────────► 012 home dashboard (after the author's choice)
     └──────────► 007 revert ──► 009 checkpoints + diff
```

- 007 depends only on 001, so it can run beside 002 to 006: its files (`service/`, `web/`, `docs/sdds/`) do not intersect `studio/`.
- 004, 005 and 006 write different skill folders under `studio/skills/` and may run side by side.
- 012 and 032 both touch `web/resources/js/layouts/`: run them one after the other.

## 4. Checkpoints

| After | Check | How |
| --- | --- | --- |
| 001 | the direction is written down and consistent | `task lint` clean on `docs/`; ADR-0030, SDD-014 and PRD-004 listed in `docs/README.md` |
| 002 | the plugin installs and every skill passes the lint | `go run ./internal/tools/skillcheck studio` exits 0; `/plugin marketplace add` of the repository lists the plugin |
| 004 | the pilot works end to end with a cheap model | a Haiku-class agent, given only the plugin and a gameplay clip, exports a 9:16 MP4 that passes the checks of plan 004 |
| 006 | links become clips and clips are reused | the five-platform pilot of plan 006 |
| 007 | an agent undoes a bad session in one call | `go test ./service/... -run TestRevert` and the MCP, CLI and HTTP parity tests pass |
| 011 | one renderer | `engine/compositor` deleted; Phase 5 exit criteria hold on three systems |
| 010 | the three use cases have a recipe each | three pilot reports with learnings promoted to memory |

## 5. Risks

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Game footage, game music and downloaded clips are copyrighted; platforms remove or demonetize | High for the pilot | the skills forbid the game soundtrack, prefer short transformed clips, and record the source of every asset (plans 004, 005, 006) |
| Site terms and extractor breakage for downloads | Medium | `yt-dlp` pinned per skill, login only with the user's explicit consent per session, failures recorded as memory facts (plan 006) |
| Remotion is source-available, not open source | Medium | never bundled; the skill states the licence and asks the user to confirm eligibility once (plan 008) |
| Young engine CLIs change quickly | Medium | exact versions approved by the author; upgrades are reviewed changes |
| The studio, the renderer and the backlog compete for senior time | Medium | at most five seniors at once; track A uses senior-c and senior-d, track B senior-b and senior-e |
| Skills drift from the editor's real commands | Medium | the editor skill is generated from the command registry; the skill lint runs in `task lint` (plan 002) |
| A home chat that needs an agent running confuses people | Medium | the "agent not connected" state with the command to start one (plan 012) |
| `@ui-sdk/react` 0.1.0-beta.6 is a pre-release in the production UI | Low-Medium | replace it when a stable release exists; record it in the next UI plan |

## 6. How to execute a plan

1. Read the whole plan, then `AGENTS.md`, then the files it names.
2. Run its drift check: if a cited file or line no longer matches, stop and report.
3. Set its status to IN PROGRESS here and in the plan.
4. Do the steps in order, one microcommit per step, with `Refs: plan NNN step N`.
5. Run every acceptance command of the step and paste the result into the report.
6. Honour the STOP conditions: when one is true, stop and report instead of improvising.
7. No push, no pull request, no new dependency without the author's approval, no process left running.
