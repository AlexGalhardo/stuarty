---
name: epic-loop
description: Work one epic of docs/plans/ from its first task to the hand-off, as the senior that owns it - one task, its Verify command, fix until it passes, commit, next task - and stop correctly at a STOP condition instead of improvising.
---

# Epic loop

For the senior that received an epic. The rule is `rules/plans.md` section 3; this is the procedure, step by step. The same loop is used for a fix of the feedback queue (`rules/plans.md` section 5), where the `VV` block takes the place of the epic and its repro steps take the place of the acceptance criteria.

## 1. Before the first task

1. Work only in the worktree and on the branch the briefing names. Check both: `git rev-parse --show-toplevel` and `git branch --show-current`.
2. Read the whole epic, `AGENTS.md`, and every document the epic links (ADRs, PRD requirements, SDDs). The briefing carries what the epic may not cite: the decisions of the author and the product issues (`VV`) the epic closes.
3. **Check that the epic is still true.** Open the files, functions and commands it names. If one no longer matches the code, stop and report: the orchestrator refreshes the epic first. Do not adapt the plan silently.
4. Prepare the worktree once: `bun install --frozen-lockfile` in `web/` when the epic touches `web/`; `bash scripts/build-renderer.sh check` when it touches `renderer/`. Set the test FFmpeg folder the briefing gives (`STUARTY_TEST_FFMPEG_DIR`), or the export tests skip.
5. Record the starting point: `git rev-parse HEAD`. The hand-off lists the commits after it.

## 2. One task

```text
do the task, and nothing beyond it
run its "Verify:" command, exactly as written
while it fails:
    read the output; find the cause; fix the cause; run it again
    after three honest attempts at the cause: STOP (section 4)
run the checks of what changed:
    bash .agents/skills/verify/scripts/verify.sh --quick --base <integration branch>
commit the task: one small commit, explicit paths, Conventional Commits, "Refs: epic NN task N"
tick the task in the epic file ("- [x]") in the same commit
```

- **Pass `--base` with the integration branch the briefing names.** The default base is `origin/main`, and against it every epic would be reported as changing the checks, because the integration branch already carries changes to `.agents/rules/`.
- `VERIFY_OK` is the only result that lets a task be committed without a note. `VERIFY_FAIL=<n>`: fix and run again. `VERIFY_NEEDS_HUMAN` with `CHECKS_CHANGED`: the epic touched a check (`.agents/rules/`, the verify and ci-preflight skills, `REVIEW.md`, the workflows, the gate and smoke scripts, the test configs). If the epic's scope names those files, list them in the hand-off for the author's review and continue; if it does not, revert the edit. Never edit a check to make it pass.
- Never skip, retry, sleep around or weaken a failing verification, and never mark a task done with a failing or missing one. A flaky test is a defect: find the cause.
- A task whose `Verify:` names a test, a spec or a script that does not exist yet is asking you to write it: the task is not done until that command exists and passes.
- Stage explicit paths. Never `git add -A` or `git add .`: the worktree may hold build output.
- Leave no process running between tasks. A server you start for a check runs on the port and with the workspace folder the briefing gives you, never on the default port and never on the author's own workspace, and you stop it by its process id when the check ends.

## 3. After the last task

1. Run **every** acceptance criterion of the epic, in order, and keep the command and its result for each. Fix and repeat until all pass.
2. Run the Definition of Done: the tests of what you changed; `task lint`; the type checks and the build (`bun run typecheck`, `bun run check:thin-client` and `task test:web` when `web/` changed; the Rust gate of `rules/rust.md` when `renderer/` changed); the documents the epic touches are updated (the SDD of its domain, the user documentation when behaviour changed).
3. Check your own diff against the integration branch: `git diff --stat <integration branch>...HEAD`. Revert anything outside the modules the epic names, unless it is the minimal edit that keeps the build compiling, and say so.
4. Write the hand-off (section 5) and end. Do not push, do not merge, do not touch the roadmap.

## 4. A STOP

Stop and report, instead of improvising, when:

- a STOP condition written in the epic's "Risks and mitigation" is true;
- a task still fails after three honest attempts at its cause;
- an acceptance criterion cannot be met;
- the work needs a new dependency, something that costs money, a real account, a download you are not allowed to make, or a decision that contradicts an ADR or changes the scope of the epic;
- the epic contradicts the code or a specification.

What to do: commit what is finished and verified; add the entry to `development/QUESTIONS.md` in the format of that file (the question with enough context, the options with the recommended one first, the exact command of a manual action, what you did meanwhile, what it blocks); then either continue with the tasks that do not depend on the answer, or end with a hand-off whose first line says `STOPPED` and why. A stop that is reported correctly is a correct result.

## 5. The hand-off

```markdown
## Hand-off: epic NN <name>   (or: STOPPED at task N: <reason>)

- Branch and worktree: <branch>, <path>
- Commits: <first>..<last>, <count> (git log --oneline <integration branch>..HEAD)
- Tasks: <done>/<total>; not done: <numbers and why>
- Acceptance criteria:
  1. <criterion> — MET | NOT MET — `<command>` → <result>
  2. ...
- Checks run: <command> → <result>, for each
- Not verified: <what you could not run, and why>   (never leave this out)
- Outside the scope: <files changed outside the epic's modules, and why>
- Specification changes: <SDD, ADR or PRD lines changed, and why>
- Recorded: <VV-NNN and VF-NNN added, questions added to QUESTIONS.md>
- How to run and test it: <the commands a reviewer needs, from a clean checkout>
```

A report is a claim, not evidence: the specialist runs everything again from a clean checkout, without reading this hand-off. Write it for the orchestrator.

## 6. Recording while you work

- A friction with the vukem framework, or an idea to improve it: a `VF-NNN` in `development/feedbacks/vukem.md`, at once, checked against the vukem source. Never work around the framework silently (ADR-0047).
- A product defect outside the scope of the epic: a `VV-NNN` at the end of `development/feedbacks/vukem-video.md`. Do not fix it in this epic.
- Nothing else in `development/` is yours to edit (`rules/development-folder.md` section 4).
