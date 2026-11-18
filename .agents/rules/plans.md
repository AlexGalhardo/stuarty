# Plans

New work lives in `docs/plans/` as numbered plans; fixes of errors, bugs and problems found by testing the product are a separate queue (section 5). **A plan is one unit of work: an agent (or a sequence of owners on one branch) executes it from start to delivery.** This rule replaces the task cards of the private work list (retired on 2026-11-18, author decision D-046); their open items were converted into plans 011 and 020 onwards, each listing the former `TASK-NNNN` ids it absorbed in its `Origin` line.

## 1. The plan

A plan is `docs/plans/NNN-<slug>.md` (next free number, never renumbered), in English, with this layout:

```markdown
# Plan NNN: <title>

<one paragraph: what it answers and the result when delivered>

- Owner: <senior-a..e | estagiario-1/2 | orchestrator | designer, per step>
- Effort: S | M | L
- Depends on: <plan NNN, an external condition, or —>
- Origin: <author decision D-NNN, author request with date, finding, or former epic and TASK ids>
- Status: TODO | IN PROGRESS (<agent>, <date>) | BLOCKED (<reason>) | DONE (<date>, <commits>) | REJECTED (<reason>)
- Written against: <commit>

## 1. Why
## 2. Current state (drift check)
## 3. Steps        (### Step N: owner, files, what, commit message, Acceptance as "- [ ]" items)
## 4. Out of scope
## 5. STOP conditions
## 6. Maintenance
```

- Every acceptance item says how to verify it: a command and its expected output, a number with its limit, a behaviour someone can observe. "Works" is never an acceptance item.
- A step belongs to one owner (the folder table of `AGENTS.md` section 4). A plan with several owners is one branch on which they work one after the other.
- A step is delivered as one small commit (`rules/commits.md`) that cites the plan: `Refs: plan NNN step N`.
- `docs/plans/README.md` is the index: execution order, dependencies, status. It changes in the same commit as a plan's `Status` line.
- Plans are public: never write a path inside `development/` (rule `development-folder.md` 1); cite decisions as "author decision D-NNN".

## 2. Before starting a plan

1. Read the whole plan, `AGENTS.md` and the files it names.
2. Run its drift check. If a cited file or line no longer matches, stop and report; the orchestrator refreshes the plan first.
3. Set `Status: IN PROGRESS (<agent>, <date>)` in the plan and the index.

## 3. Completion protocol

1. Only the orchestrator marks a plan or a step done. An engineer delivers; the orchestrator reads the diff, runs every acceptance item, fixes what is wrong (or sends it back), and only then ticks the items and writes `Status: DONE (<date>, <commits>)`. A report is a claim, not evidence.
2. A plan that cannot meet an acceptance item is not marked and not changed silently: write `Status: BLOCKED (<reason>)`, then fix the specification (the SDD first) or split the plan into a new one.
3. Commits reach `main` through the release line (ADR-0028), unless the author says otherwise for a given change.
4. No editing logic outside `core/` (ADR-0002, ADR-0007). Names come from `docs/sdds/000-shared-definitions.md`, changed first.
5. Every friction with the vukem framework goes to `development/feedbacks/vukem.md` as a `VF-NNN`; every product problem found while working or testing goes to `development/feedbacks/vukem-video.md` as a `VV-NNN` (section 5), not into a plan.
6. The author confirms before anything that cannot be undone or that costs money: a push to a protected branch, a release tag, repository settings, secrets, a new dependency.
7. When a plan adds or changes requirements (`RF-`, `NFR-`), update `docs/traceability.md` for the requirements it touches.

## 4. Shared files

Touched by one plan at a time, never in parallel: `go.mod`, `web/routes/`, `web/resources/lang/*.json`, `.github/workflows/ci.yml`, `docs/sdds/000-shared-definitions.md`, `docs/plans/README.md`. Generated folders (`core/schema/`, `web/resources/js/generated/`, `site/generated/`, `site/content/generated/`, `studio/skills/vukem-video/`) are written only by their generators.

## 5. The feedback queue

Errors, bugs, crashes and the problems found by practical tests (QA rounds, real use) are fixed from `development/feedbacks/vukem-video.md`, not from a plan.

1. One dedicated agent per session works the queue, in parallel with the agents of the plans: the orchestrator names one senior "the feedback agent" in its briefing. It takes the open `VV` with the lowest number and works one issue at a time, from the reproducing test to the delivery.
2. The feedback agent may touch the folders the fix needs; the orchestrator does not dispatch at the same time a plan whose files intersect the fix in progress. A fix that turns out large, or that changes a rule of `core/`, stops and goes back to the orchestrator, who turns it into a plan.
3. Delivery: one microcommit per `VV` (or one per area when the fix crosses areas), `fix(<scope>): <english summary>` (`feat` for a missing feature) with `Refs: VV-NNN`.
4. The last line of each `VV` block is its status: `aberto`, `em andamento (<agent>, <date>)`, `bloqueado (<reason>)` or `corrigido em <date>, commit <hash>, evidência: <command and result>`. Only the orchestrator writes `corrigido`, after running the reproduction steps and the test of the issue. An issue is never deleted.
