# Epics

The work lives in `docs/plans/` as numbered epics, with the index `docs/plans/00-roadmap.md`. **An epic is one unit of work: one senior takes it from the first task to the hand-off, and the specialist reviews it** (ADR-0045). If it does not fit one senior from start to review, split it. There are no phases and no milestones. Fixes of bugs found by using and testing the product are a separate queue (section 5).

## 1. The epic

An epic is `docs/plans/NN-<slug>.md`, numbered in the order of execution, in English, with this layout:

```markdown
# NN: <name of the epic>

## Goal
<one paragraph: the value for the user>

## Context and related decisions
<links to ADRs, PRD requirements (RF-, NFR-), author decisions as "author decision D-NNN", research notes by name>

## Scope
## Out of scope
## Dependencies
<epics that must be done first>

## Technical design
<architecture, modules, contracts of the API, the CLI and MCP, data models>

## Tasks
<an ordered checklist; every task ends with "Verify:" and the command, with its expected result, that proves the task>

## Test strategy
<unit, integration, end to end, media fixtures>

## Acceptance criteria
<verifiable by the specialist without asking the senior anything>

## Risks and mitigation
## Size
<S, M, L or XL>

## Hand-off for review
<what the specialist receives: the diff, how to run, how to test>
```

- Every acceptance criterion says how to verify it: a command and its expected output, a number with its limit, a behaviour someone can observe. "Works" is never an acceptance criterion.
- An epic names the modules it touches, so that the roadmap can tell which epics run side by side. Two epics that touch the same modules are never in progress together.
- A task is small enough to be done and verified in one sitting, and it names the command that verifies it (a test, a lint, a build, a script) with the result to expect. A task with no verification command is not a task yet.
- A task is delivered as one small commit (`rules/commits.md`) that cites the epic: `Refs: epic NN task N`.
- An epic of a new domain writes the specification of that domain (an SDD, with names first in `docs/sdds/000-shared-definitions.md`) as its first task, before code.
- An epic states its STOP conditions in "Risks and mitigation": what makes the senior stop and report instead of improvising.
- Epics are public: never write a path inside `development/` (rule `development-folder.md` 1); cite decisions as "author decision D-NNN".
- `docs/plans/00-roadmap.md` is the index: the order, the dependency graph, what can run in parallel between the five seniors, the status of each epic and the table that maps every requirement to at least one epic. It changes in the same commit as the status of an epic.

## 2. Before starting an epic

1. Read the whole epic, `AGENTS.md` and the documents it links.
2. Check that what the epic says about the code is still true. If a cited file, function or contract no longer matches, stop and report; the orchestrator refreshes the epic first.
3. The orchestrator sets the status of the epic to `IN PROGRESS (<agent>, <date>)` in the roadmap.

## 3. The loop and the completion protocol

The senior works the epic in a fixed loop (ADR-0045 item 3):

```text
read the epic and what it links
for each task, in order:
    do the task
    run its "Verify:" command
    while it fails: find the cause, fix it, run it again      (never skip, retry or weaken the check)
    commit the task (Refs: epic NN task N)
    tick the task in the epic
run every acceptance criterion; fix and repeat until all pass
write the hand-off and give the epic to the orchestrator
```

A task that still fails after three honest attempts at its cause is a STOP: report what was tried and what was seen, and wait for the orchestrator. Never mark a task done with a failing or missing verification.

1. **The senior delivers.** Tests written and passing, linters and type checks clean, documents updated, and the hand-off written in the format the epic asks for: the commits, each acceptance criterion as done or not done with the reason, the commands run and their result, how to run and how to test.
2. **The specialist reviews with no prior context.** The orchestrator opens a new specialist session and gives it only the epic, its acceptance criteria and the diff of the epic branch against the integration branch. Never the senior's report, never an opinion. The specialist runs every acceptance criterion and the checks again, reads the diff against `REVIEW.md`, fixes what it finds in new commits, and approves, or sends back what is too large to fix in review.
3. **Only the orchestrator marks an epic done**, after the specialist approved: it merges the epic branch into the integration branch, pushes the integration branch so that the draft pull request shows the epic (`rules/git-and-branches.md` item 3), writes `DONE (<date>, <commits>)` in the roadmap and stops the agents. A report is a claim, not evidence.
4. An epic that cannot meet an acceptance criterion is not marked and not changed silently: write `BLOCKED (<reason>)`, then fix the specification first or split the epic.
5. A delivery that fails review twice stops: the orchestrator writes `BLOCKED (<reason>)`, records the question for the author with the options and a recommendation, and dispatches the next unblocked epic. In an unattended session nobody waits idle for an answer.
6. No rule of a domain outside `core/` (ADR-0002, ADR-0031). Names come from `docs/sdds/000-shared-definitions.md`, changed first.
7. Every friction with the vukem framework goes to `development/feedbacks/vukem.md` as a `VF-NNN` (ADR-0047); every product problem found while working or testing goes to `development/feedbacks/vukem-video.md` as a `VV-NNN` (section 5), not into an epic.
8. The author confirms before anything that cannot be undone or that costs money: a push, a merge to `main`, a release or a tag, repository settings, secrets, a new dependency, a decision that contradicts an ADR, a change of the scope of an epic.

## 4. Shared files

Touched by one epic at a time, never in parallel: `go.mod`, `web/routes/`, `web/resources/lang/*.json`, `.github/workflows/ci.yml`, `docs/sdds/000-shared-definitions.md`, `docs/plans/00-roadmap.md`. When two epics in progress both need one of them, the orchestrator orders the two merges. Generated folders (`core/schema/`, `web/resources/js/generated/`, `site/generated/`, `site/content/generated/`) are written only by their generators.

## 5. The feedback queue

Errors, bugs, crashes and the problems found by practical tests (QA rounds, real use) are recorded in `development/feedbacks/vukem-video.md`, one `VV` block per problem, in the format that file gives. Each block says where the problem is to be fixed: in an epic (it then goes into that epic's briefing and is closed with it) or in the queue (it is worked on its own, as below).

1. An important fix is worked like a small epic: the orchestrator gives one open `VV` to a free senior, on its own branch from the integration branch, and the specialist reviews it the same way. The orchestrator does not dispatch at the same time an epic whose modules intersect the fix.
2. A fix that turns out large, or that changes a rule of `core/`, stops and goes back to the orchestrator, who turns it into an epic.
3. Delivery: one commit per `VV` (or one per area when the fix crosses areas), `fix(<scope>): <english summary>` (`feat` for a missing feature) with `Refs: VV-NNN`. Write the test that reproduces the problem first.
4. The last line of each `VV` block is its status: `aberto`, `em andamento (<agent>, <date>)`, `bloqueado (<reason>)` or `corrigido em <date>, commit <hash>, evidência: <command and result>`. Only the orchestrator writes `corrigido`, after the specialist approved, and then moves the whole block to `development/feedbacks/vukem-video-resolvidas.md`, where the QA campaign checks it again. An issue is never deleted, and an id is never reused: a problem that comes back gets a new `VV` that cites the old one.

## 6. The QA campaign

When every epic of the roadmap is done or blocked, the work becomes a campaign of rounds (ADR-0045 item 8).

1. **A round tests the whole product end to end**, with the five seniors in parallel, each on its own surface so that nothing is tested twice and nothing is left out: every command of the CLI; every route of the HTTP API; every MCP tool and every operation of every catalogue; every control of the web UI, section by section; the desktop application, the installation in Docker and the complete flows across domains (the first complete flow of PRD-004 and the reference scenario of PRD-001). The coverage is counted from the inventories (the surface list of `web/contract/testdata/surface.txt`, the catalogues, the UI inventory), not from memory.
2. **A senior that tests does not fix.** It records each problem as a `VV` entry with the steps to reproduce, what was expected, what happened and the evidence. Each senior works in its own workspace, port and worktree.
3. **After the round, the problems are fixed** as in section 5: one branch per `VV` or per group in the same module, a reproducing test first, then the fix, then the specialist's review, then the merge and the push.
4. **Then a new round starts from the beginning**, on the new tip of the integration branch. Fixed problems are tested again; the whole product is tested again, because a fix can break something else.
5. **The campaign ends when a full round finds no problem**, or only problems the author already accepted as known. The orchestrator writes the result of each round (what was covered, how many problems, which are open) where the session state is kept.
