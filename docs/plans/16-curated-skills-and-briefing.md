# 16: Curated skills and the guided briefing

## Goal

A creator starts a production by answering a few questions, and their agent already knows how to take that brief to a finished short video: because Stuarty ships a small set of skills that were written for it and tested with it.

## Context and related decisions

- ADR-0040, ADR-0039 (the brief is an item of the project), ADR-0031 (the first complete flow).
- Author decisions D-048, D-064, D-071 (two third-party skill sets adopted as sources, with attribution). PRD-004 RF-301, RF-302, RF-389.
- Research notes `web/06-skills-open-source` and `web/05-rotina-criadores`.
- Absorbs the earlier plans on the router skill, the recall and retro skills, and the agent-side `fetch-clip` skill.

## Scope

- Modules: `web/resources/skills/` (the built-in skills, embedded in the binary and installed into the library as `built-in`), `core/project/` (the briefing questions and the brief schema), `service/` (briefing operations), `web/resources/js/pages/Projects/` (the briefing form), `licenses/` (attribution of adapted material), SDD-018, SDD-005.
- The curated set for the first flow: `start-here` (a router: which skill for which request), `brief-a-production`, `short-vertical-video`, `captions-for-shorts`, `thumbnail`, `recall-learnings`, `production-retro`, `fetch-clip` (agent-side download, then import with origin and rights note), and the writing skills adapted from the adopted marketing set that the first flow needs (hook, short script, caption per network).

## Out of scope

- Recipes for commercials and long videos (epic 39).
- Skills that drive an external motion engine (epic 33).
- Any skill that asks for a credential or tells an agent to bypass the approval queue.

## Dependencies

06, 15.

## Technical design

- **Briefing** (RF-301): the questions are data in `core/project` (id, text, kind of answer, whether required), not UI code: audience, goal, platform and format, duration, tone, references, what must appear, what must not, deadline. `BriefingQuestions` returns them; `BriefingAnswer` validates and stores answers; `BriefSet` of epic 06 holds the resulting Markdown. An agent can ask the same questions in the chat and store the same structure.
- **Built-in skills**: Markdown in the repository, embedded with `go:embed`, installed or updated in the library at start with origin `built-in` and the product version; a user's edited copy is never overwritten (it becomes `user`, and the built-in stays available).
- **Every skill names real operations**: a test parses each built-in skill, extracts the operation and tool names it mentions and checks them against the registry, so a renamed operation breaks the build instead of the agent.
- **Adapted material** (D-071): each adapted skill records its source and licence in its front matter; the licence texts and a notice file go in `licenses/`; nothing is copied from a source whose licence was not read.
- **Agent-side only**: `fetch-clip` tells the agent to download with its own tools on its own side and to import the file with a source and a rights note; the product gains no downloader.
- **Recall and retro**: `recall-learnings` reads memory at the start of a production; `production-retro` writes what was learned at the end (marked as agent-written until kept).

## Tasks

- [x] 1. Amend SDD-018 (built-in skills, update rule, the registry check) and SDD-005 (the brief schema). Verify: `task lint:md` exits 0.
- [x] 2. Implement the briefing questions and answers in `core/project` and the service, on the three surfaces. Verify: `go test ./core/project/... ./service/... -run TestBriefing` and `go test ./web/contract/...` pass.
- [x] 3. Build the briefing form on the project overview, in both languages. Verify: `task test:web` and `web/e2e/specs/briefing.spec.ts` pass.
- [ ] 4. Implement embedding, install and update of built-in skills. Verify: `go test ./service/library/... -run TestBuiltIn` passes (a user's edit survives an update).
- [ ] 5. Write the registry check for skills. Verify: `go test ./web/resources/skills/...` passes, and fails when a skill names an unknown operation (described in the hand-off).
- [ ] 6. Write `start-here`, `brief-a-production`, `recall-learnings`, `production-retro` and `fetch-clip`. Verify: `SkillLint` reports nothing for each (`go test ./web/resources/skills/...`).
- [ ] 7. Write `short-vertical-video`, `captions-for-shorts` and `thumbnail` against the operations that exist when this epic runs; mark the steps that depend on epics not yet merged. Verify: the registry check passes.
- [ ] 8. Read the licences of the two adopted sources, adapt the writing skills, add attribution to `licenses/`. Verify: each adapted skill names a source whose licence text is in `licenses/` (a test in `web/resources/skills/` checks it) and `task notices:check` exits 0.
- [ ] 9. Run the briefing and `short-vertical-video` once by hand with a real agent, or mark as not verified. Verify: the transcript or the note is in the hand-off.

## Test strategy

- Unit: briefing validation; built-in install and update.
- Static: the skill lint and the registry check on every built-in skill.
- Scenario: a scripted agent follows `short-vertical-video` up to the steps available.
- The measured run with a small model belongs to epic 38.

## Acceptance criteria

1. A new project offers the briefing; the answers become the project's brief, on HTTP, CLI and MCP as well.
2. A fresh workspace lists the built-in skills with origin `built-in`, each passing `SkillLint`.
3. Every operation or tool a built-in skill names exists in the registry (`go test ./web/resources/skills/...`).
4. Adapted skills carry source and licence, and `licenses/` holds the notices.
5. Updating the binary updates untouched built-in skills and keeps edited copies.
6. No built-in skill instructs an agent to approve, publish without approval, or reveal a credential (lint rule).

## Risks and mitigation

- **Skills written before their operations exist.** Mitigation: the registry check; dependent steps are marked and finished by the epic that adds the operation.
- **A licence that forbids adaptation.** Mitigation: task 8 reads it first. **STOP** if either source's licence is not what the research note recorded.

## Size

M

## Hand-off for review

The diff of `epic/16-curated-skills-and-briefing`; the list of skills with their sources; the outputs of the acceptance commands; which steps wait for later epics.
