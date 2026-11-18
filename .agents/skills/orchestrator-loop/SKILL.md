---
name: orchestrator-loop
description: Run the work session as the orchestrator in a loop that survives hours unattended - pick the next unblocked epics, dispatch seniors in worktrees, send each delivery to a specialist with no context, merge and push what was approved, keep the roadmap and the state true, and never wait idle.
---

# Orchestrator loop

For the main session. The rules are `rules/orchestration.md`, `rules/plans.md` section 3 and `rules/git-and-branches.md`; this is the procedure. Load it at the start of every work session, together with `skills/epic-loop/` (what a senior does, so that the briefing and the hand-off are judged in the right format) and `skills/qa-campaign/` (where the loop ends), and read it again after every context compaction. The integration branch and its draft pull request are named in `rules/git-and-branches.md` item 2. Parallel agents, worktrees and pushing the integration branch need the author's authorization: the work-session prompt carries it, and without it this loop does not start.

## 1. One turn of the loop

```text
read development/STATE.md and development/QUESTIONS.md        (always first; after a compaction too)
for every agent that finished:        handle its result        (sections 3 and 4)
for every answer the author gave:     apply it                 (section 6)
while a senior is free and an epic is unblocked:   dispatch it (section 2)
if nothing can be dispatched and nobody is running:
    every epic done or blocked  -> start or continue the QA campaign (skills/qa-campaign/)
    otherwise                    -> the loop is blocked on the author: write it in STATE.md and end the turn
while agents run: prepare the next briefings, check the tip of the integration branch, keep the roadmap true
rewrite development/STATE.md                                   (after every event, never from memory)
```

An epic is **unblocked** when every epic in its "Dependencies" is `DONE` in `docs/plans/00-roadmap.md`, it is not `BLOCKED`, and no epic in progress names one of its modules or one of the shared files of `rules/plans.md` section 4. At most five seniors at once; the foundation epics run one at a time.

## 2. Dispatching an epic

1. Mark it `IN PROGRESS (<agent>, <date>)` in the roadmap and in `development/STATE.md`.
2. Create the branch and the worktree from the tip of the integration branch, in the sibling folder:

   ```bash
   git worktree add -b epic/NN-<slug> ../stuarty-worktrees/<agent> <integration branch>
   ```

3. Dispatch the named agent (`senior-a` to `senior-e`; never a name that is still alive) with a briefing that contains, and nothing it must guess:
   - the path of the epic, and the instruction to read it whole, with what it links;
   - the skills to read before the first task, by file path: always `skills/epic-loop/SKILL.md` and `skills/verify/SKILL.md`; `skills/renderer-parity/` and `rules/rust.md` when the epic touches `renderer/`, the preview executor or the export compositor; `skills/deterministic-media/` when bytes or pixels are compared with a golden; `skills/cross-platform-go/` when Go code touches paths, processes, file systems or timing. A subagent has no skill tool: it reads the file;
   - the goal in one sentence and why it matters to a user;
   - the worktree path and the branch; the integration branch to pass as `--base`; the modules it owns and what it must not touch;
   - its own port and workspace folder for any server it starts (one per agent, never the default port, never the author's workspace), and the test FFmpeg folder;
   - the acceptance criteria as commands (copied from the epic);
   - from `development/`: the text of every `VV` the epic closes and every decision `D-NNN` that applies (an epic never cites that folder);
   - what was already ruled out;
   - the standing rules: no push, no merge, no new dependency, no forbidden git command, no process left running, record `VF` and `VV`, report what was not run, stop and ask at a STOP condition;
   - the hand-off format of `skills/epic-loop/` section 5.
4. Rewrite `development/STATE.md`.

Use the **intern** for mechanical, checkable work that does not need the epic's judgement: running a suite and reporting, building fixtures, i18n keys, a lookup with sources, reading an official page again and reporting what changed. It writes no product code and works in the worktree of the epic it serves, or in the main checkout for read-only work.

## 3. A hand-off arrived

1. It is a claim. Look at the branch yourself: `git log --oneline <integration branch>..epic/NN-<slug>` and `git diff --stat <integration branch>...epic/NN-<slug>`. Changes outside the epic's modules go back to the senior, unless the hand-off names them as the minimal compile fix.
2. If the first line says `STOPPED`: section 5.
3. Stop the senior. Open a **new** specialist session and give it only:
   - the path of the epic;
   - the acceptance criteria;
   - the diff: the branch name, the worktree path and the command `git diff <integration branch>...epic/NN-<slug>`;
   - the integration branch to pass as `--base`, its own port and workspace folder, the test FFmpeg folder.

   Never the senior's hand-off, never your reading of it, never the conversation.
4. Rewrite `development/STATE.md`.

## 4. A verdict arrived

- **APPROVED or APPROVED AFTER FIXES:**
  1. Merge with `skills/integrate-agent-work/`: `git merge --no-ff epic/NN-<slug>` on the integration branch, shared files one epic at a time.
  2. Check the merged tip, writing the output to a file and testing the exit code (never through a pipe): `go build ./...`, the tests of what the epic changed, and `bash .agents/skills/verify/scripts/verify.sh --quick --base origin/<integration branch>`.
  3. Mark the roadmap `DONE (<date>, <first>..<last>)` and commit it on the integration branch (`docs(repo): mark epic NN done`).
  4. Push the integration branch, and only it: `git push origin <integration branch>`. A failed check means no push: fix it first.
  5. Remove the worktree and the local branch (`git worktree remove`, `git branch -d`), stop the agents and any process they left.
  6. Close in `development/feedbacks/vukem-video.md` the `VV` blocks the epic fixed (the block leaves the file), and rewrite `development/STATE.md`.
- **CHANGES REQUIRED:** send the findings back to a senior on the same branch (a new session with the epic, the findings and the hand-off format), then a new specialist session. **A delivery that fails review twice stops:** mark the epic `BLOCKED (<reason>)`, record the question for the author with the options and a recommendation, keep the branch and the worktree for the author, and dispatch the next unblocked epic.
- Every security finding is fixed before the merge, or its reason for not applying is written down.

## 5. A STOP arrived

1. Check that the question is in `development/QUESTIONS.md`, complete: context, options with the recommended one first, the exact command of a manual action, what was done meanwhile, what it blocks. Fix it if it is not.
2. If the rest of the epic does not depend on the answer, let the senior continue. Otherwise mark the epic `BLOCKED (<reason>)` in the roadmap, keep its branch and worktree, stop the agent, and dispatch the next unblocked epic. Nobody waits idle for an answer.
3. A question that is cheap to undo is not a STOP: take the simplest reversible option, record it in the specification, and continue.

## 6. An answer arrived

Record a decision as the next `D-NNN` in `development/brainstorming.md`; fix the epic and the public documents it changes, in one commit on the integration branch; remove the entry from `development/QUESTIONS.md`; set the epic back to `TODO` or dispatch it again on its kept branch.

## 7. What never happens

- No push, merge or commit to `main`; no marking the pull request ready; no release, no tag; no repository setting, no secret; no force push; no `git stash`, `git reset --hard`, `git checkout -- .`.
- No push of an epic or fix branch. No push of the integration branch with a failed check.
- No new dependency, no paid call, no real account without the author.
- No epic marked done from a report. No specialist that saw the senior's report.
- No check edited in the same change as the code it judges.
- No performance number measured while agents run.

## 8. Surviving a long run

- `development/STATE.md` is rewritten after every event: what each agent is doing and since when, epics done with their commits, epics blocked with the reason, what waits for the author. It describes now.
- Before a context compaction, rewrite it first. After one, read it, then `docs/plans/00-roadmap.md`, then `git worktree list` and `git status`, and continue. Never start an epic again from memory.
- An agent that returns nothing, or dies, is not a result: check its worktree (`git status`, `git log`), keep the commits that are there, and dispatch a new session on the same branch with what remains.
- A worktree that is left over with no agent is listed in `STATE.md` with its branch, never deleted while it holds commits that are not merged.
- When a usage limit or an error stops the session, the state file and the roadmap are what the next session resumes from: keep them true at every step, not at the end.
