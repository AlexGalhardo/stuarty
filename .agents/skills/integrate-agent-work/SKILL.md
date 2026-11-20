---
name: integrate-agent-work
description: Merge an epic that the specialist approved into the integration branch, verify the result, mark the roadmap and send the next briefing, without trusting a report as evidence.
---

# Integrate agent work

The integration line is the open release branch of the release train (ADR-0048), linear, with its pull request against `main`. Every epic is a branch from it (`epic/NN-<slug>`), in its own worktree (`rules/git-and-branches.md`). An epic is integrated only after the specialist approved it (`rules/plans.md` section 3).

An approved branch is integrated by cherry-picking its own commits, in order, onto the release branch: `bash .agents/skills/release-pr/scripts/release-train.sh integrate epic/NN-<slug>`. The release branch never gets a merge commit, because GitHub refuses to rebase-merge a pull request that has one.

## Steps

1. List what is new: `git log --oneline <line>..epic/NN-<slug>`. The specialist's fix commits are part of the epic. An epic based on an old tip of the line is first updated by merging the line into it, in its worktree, and its checks are run again.
2. Integrate it: `release-train.sh integrate epic/NN-<slug>` on the release branch. It picks the epic's commits that are not on `main`, on a release branch or already on the line by patch, skips merge commits, pushes, and cuts the release by itself when the line reaches 95 commits. Shared files (`rules/plans.md` section 4) are integrated one epic at a time.
3. Conflicts:
   - In generated files, take either side and regenerate.
   - An empty cherry-pick ("The previous cherry-pick is now empty") means the change is already there: `git cherry-pick --skip`.
   - Two seniors can fix the same defect in parallel (the rotation handle of the preview was fixed twice). Keep the version already on the line when it has the test hooks other specs use (`data-inside`), and `git cherry-pick --skip` the other; check that no duplicate declaration survived the merge.
   - A conflict in an SDD table row (the threat table of SDD-007) usually means both sides added a test name to the same row: keep the union, then run `go run ./internal/tools/threatcheck`. Restore a row from the original commit with `git show <sha>:<file> | grep '^| T-26 |'` rather than retyping it.
   - A commit whose conflict you resolved gets a new patch id, so `git cherry` lists it again on the next pass and the cherry-pick conflicts a second time. When a line is rebuilt in passes, skip the commits whose subject is already on the line (`git log --format=%s origin/main..HEAD`), and abort a repeated attempt with `git cherry-pick --abort`.
   - Two seniors shrinking the same list (the `KNOWN_DIVERGENCES` of `preview-vs-export`, each removing the cases it fixed) conflict on every entry: the result is the intersection, often an empty object. Run the web lint afterwards; Biome wants `{}` on one line.
4. Check the diff against the briefing with `git diff --stat`, and revert what is outside the agent's area unless it is a minimal compile fix the report names.
5. Run `go build ./...` after every integrated series, not only at the end: a type one senior changes in its area breaks another senior's surface (the OpenTimelineIO result moved from `otio.Lost` to `service.OtioLost` and broke `web/app/cli`). Then run `skills/ci-preflight/`, at least the package tests, the cross-target lint and the generators.
6. Mark the epic in `docs/plans/00-roadmap.md`, remove its worktree and its local branch, stop its agents, and dispatch the next unblocked epic to the free senior, so nobody waits idle. At 88 commits or more, cut the release (`release-train.sh cut`, `skills/release-pr/`).
7. When a test fails on the line, decide regression or environment before you route it: run the same test at the previous tag from a throwaway `git worktree add --detach <dir> vX.Y.Z` (remove it after), and, for a timing test, interleave runs of `main` and the line under the same load. `TestCancelTimeoutFailsTheJob` passed 3/3 at v0.14.0 and failed 3/3 later, which pointed at the v0.15.0 Job Object start; `TestEventsLatencyFromCommitToClient` failed on `main` and on the line alike, which made it load.
8. Mark the epic from the acceptance criteria the specialist ran, never from memory (rule `rules/plans.md` section 3): write `DONE (<date>, <commits>)` in the roadmap. An epic whose remaining criterion needs another system, a quiet machine or the author stays open with `BLOCKED (<reason>)`.

## Things that went wrong before

- A test passed only in the agent's worktree because an environment variable was unset there: the sessions tests built `$STUARTY_TEST_FFMPEG_DIR/ffmpeg` without `.exe` on Windows. Run delivered tests from a clean worktree with the variables CI sets.
- An agent's single report can be refused when it reports twice. If a notification says the report was refused, read the commits in its worktree (`git -C <worktree> log`).
- A security review fires on pushed commits. Send each finding to the owner with the fix it needs (owner scoping, fail closed on lookup errors, one transaction for read-then-act), and integrate only the fixed version.
- Timing tests flake when several agents build at once. A failure under load is re-run alone before it is called a defect, and is fixed only if it also fails alone.
- Agents ran `python3 - <<'EOF'` despite the rule against it; an empty heredoc left a process waiting on stdin as a background task. Read each report for it, stop the leftover task through the session's task list (never by image name), and repeat the rule in the next briefing.
- Read a report's "not verified" list as the real state. "Fails for a reason outside my area, probably the machine" is not a result: send it back with the exact rerun (pinned FFmpeg, private home and port) and ask for the job record's code and reason.
- A briefing names the files an agent owns. When an agent edits a file outside them because a task assert names it (an SDD status line), accept it if the report says so; otherwise revert it.
