---
name: qa-campaign
description: Run the end-to-end QA campaign after the epics - five seniors test the whole product in parallel, each on its own surface, every problem becomes a VV entry, the problems are fixed and reviewed, and a new round starts until a full round finds no problem.
---

# QA campaign

The rule is `rules/plans.md` section 6 (ADR-0045 item 8). It starts when every epic of `docs/plans/00-roadmap.md` is `DONE` or `BLOCKED`, and it is run by the orchestrator.

## 1. The loop

```text
round n:
    build once, from the tip of the integration branch
    prepare five isolated environments
    five seniors test in parallel, each on its surface: they record, they do not fix
    the orchestrator consolidates the problems into the feedback queue
    if the round found no problem (or only ones the author already accepted): the campaign is over
    fix phase: each problem is fixed like a small epic, reviewed by the specialist, merged, pushed
round n+1: from the beginning, on the new tip, everything again
```

A fix can break something else, so a round always tests the whole product again, not only what was fixed.

## 2. Preparing a round

1. Round `R<n>` is the largest round recorded in `development/tests/PLAN.md` plus one. Record the commit of the integration branch under test.
2. Build the binary and the web UI once, from a clean worktree at that commit, into `qa-build/R<n>/bin/` (git-ignored). Every senior tests the same build.
3. Give each senior its own folder `qa-build/R<n>/<agent>/` holding its workspace, and its own port; never the default port and never the author's own workspace. Start nothing on the author's behalf that reads their real accounts: providers and networks are the fake servers of the test suites, unless the author connected a real account for the round.
4. Refresh the inventories that say what "everything" is, from the build under test, never from memory:
   - `web/contract/testdata/surface.txt` (every CLI command, HTTP route and MCP tool);
   - the catalogue of every domain (`task codegen` output and the registry snapshot);
   - the UI inventory of `development/tests/inventory/`, section by section;
   - the reference scenario of PRD-001 and the first complete flow of PRD-004 (`scripts/smoke-e2e.sh`, `scripts/first-flow.sh`).
5. Check the test media of `development/tests/assets/` against its `LICENSES.md`.
6. Create `development/tests/results/R<n>/<agent>/` for the evidence of each senior.

## 3. The five surfaces

Nothing is tested twice and nothing is left out: every line of an inventory belongs to exactly one senior.

| Senior | Surface | Counted from |
| --- | --- | --- |
| `senior-a` | every CLI command and flag, exit codes, `--json`, help texts, the installation and first start, the documentation commands | the CLI part of the surface list |
| `senior-b` | every route of the HTTP API, the OpenAPI document against the behaviour, the event stream, authentication modes, rate limits, scopes | the HTTP part of the surface list and the OpenAPI document |
| `senior-c` | every MCP tool and every operation of every catalogue through MCP, the discovery calls and their token budgets, the approval queue and the vault from an agent's side | the MCP part of the surface list and the catalogues |
| `senior-d` | every control of the web UI of the editors: video editor and image editor, in both languages and both themes, with the keyboard as well as the pointer | the UI inventory, editor sections |
| `senior-e` | every control of the rest of the UI (dashboard, projects, assets, writing, publishing calendar, connections, memory, skills, prompts, chat, settings), the desktop application, the Docker installation, and the complete flows across domains | the UI inventory, platform sections; the two flows |

Adjust the table when a surface is much larger than another, and write the split used into the round's record.

## 4. What a senior does in a round

- It tests and records. **It does not fix**, and it does not read the source to explain a defect away: what a user or an agent sees is what counts.
- For each inventory line: do it, compare with what the specification and the documentation say, and mark it ok, problem, broken, or not tested with the reason.
- A problem is recorded only after it was reproduced twice. An intermittent one says how often (`x/y`).
- Each problem is one `VV` block in the format of `development/feedbacks/vukem-video.md`: the steps with the exact commands, what was expected and where that expectation comes from, what happened with the error quoted, the evidence under `development/tests/results/R<n>/<agent>/`, the frequency, the severity.
- It writes its coverage (every inventory line with its mark) and its evidence only in its own results folder, and ends with: lines covered out of lines owned, problems by severity, what it could not test and why.
- It leaves no process running and deletes the media and the projects it created when the round is consolidated.

Severity: **Critical** loses work or data, crashes, exports or publishes something wrong without a warning, leaks a secret, or publishes without approval. **High** stops a common task with no reasonable way around. **Medium** has a way around that costs time or quality. **Low** is cosmetic or rare.

## 5. Consolidating

1. Deduplicate the problems of the five seniors; one problem seen on two surfaces is one `VV` that names both.
2. Reproduce every Critical and High yourself before it is accepted.
3. Give each a final id, in ascending order from the largest id ever used, and say where it is fixed (the queue; or an epic, when it is a missing feature that an existing epic owns).
4. Write the round into `development/tests/PLAN.md` (date, commit, coverage per surface, problems by severity) and into `development/STATE.md`.

## 6. The fix phase

As `rules/plans.md` section 5: one branch `fix/VV-NNN-<slug>` from the integration branch per problem, or per group of problems in the same module; a test that reproduces the problem first, then the fix; the specialist reviews with no context (the `VV` block as the task, its repro steps as the acceptance criteria, the diff); the orchestrator merges, pushes the integration branch, and removes the block from the queue. Critical and High first. Two fixes that touch the same module are not in progress together. A fix that turns out to be a redesign goes back to the orchestrator and becomes an epic.

## 7. The end

The campaign ends when a full round, with every inventory line covered, finds no problem, or only problems the author accepted as known in `development/QUESTIONS.md`. A round whose coverage is incomplete does not end the campaign, whatever it found. Then the orchestrator writes the final record and stops: splitting the pull request into releases and restoring CI are planned with the author (`rules/git-and-branches.md` item 9).
