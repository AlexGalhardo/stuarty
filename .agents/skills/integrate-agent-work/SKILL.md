---
name: integrate-agent-work
description: Take the commits of a delegated engineer into the release integration line, verify them, and send the next briefing, without trusting the report as evidence.
---

# Integrate agent work

The integration line is the newest open release branch, for example `release/v0.7.0`. Every senior branches from it.

## Steps

1. List what is new: `git log --oneline origin/<line>..<agent branch>`. An agent that was based on an older line may repeat commits that are already integrated. Cherry-pick only the new ones, and use `git cherry` to find duplicates.
2. `git cherry-pick <commits>` in order. When the agent says the tree is green only at the end of its series, take the whole series together.
3. Conflicts:
   - In generated files, take either side and regenerate.
   - An empty cherry-pick ("The previous cherry-pick is now empty") means the change is already there: `git cherry-pick --skip`.
   - Two seniors can fix the same defect in parallel (the rotation handle of the preview was fixed twice). Keep the version already on the line when it has the test hooks other specs use (`data-inside`), and `git cherry-pick --skip` the other; check that no duplicate declaration survived the merge.
   - A conflict in an SDD table row (the threat table of SDD-007) usually means both sides added a test name to the same row: keep the union, then run `go run ./internal/tools/threatcheck`. Restore a row from the original commit with `git show <sha>:<file> | grep '^| T-26 |'` rather than retyping it.
   - A commit whose conflict you resolved gets a new patch id, so `git cherry` lists it again on the next pass and the cherry-pick conflicts a second time. When a line is rebuilt in passes, skip the commits whose subject is already on the line (`git log --format=%s origin/main..HEAD`), and abort a repeated attempt with `git cherry-pick --abort`.
   - Two seniors shrinking the same list (the `KNOWN_DIVERGENCES` of `preview-vs-export`, each removing the cases it fixed) conflict on every entry: the result is the intersection, often an empty object. Run the web lint afterwards; Biome wants `{}` on one line.
4. Check the diff against the briefing with `git diff --stat`, and revert what is outside the agent's area unless it is a minimal compile fix the report names.
5. Run `go build ./...` after every integrated series, not only at the end: a type one senior changes in its area breaks another senior's surface (the OpenTimelineIO result moved from `otio.Lost` to `service.OtioLost` and broke `web/app/cli`). Then run `skills/ci-preflight/`, at least the package tests, the cross-target lint and the generators.
6. Push the line, mark the card in `development/TODO.md`, and reply to the agent with the next card in the same message, so it never waits idle.
7. When a test fails on the line, decide regression or environment before you route it: run the same test at the previous tag from a throwaway `git worktree add --detach <dir> vX.Y.Z` (remove it after), and, for a timing test, interleave runs of `main` and the line under the same load. `TestCancelTimeoutFailsTheJob` passed 3/3 at v0.14.0 and failed 3/3 later, which pointed at the v0.15.0 Job Object start; `TestEventsLatencyFromCommitToClient` failed on `main` and on the line alike, which made it load.
8. Mark `development/TODO.md` from the asserts you ran, never from memory (rule `rules/task-cards.md` section 3): `[x]` on the card, its checklists removed, and `Status: feito em <date>, commit <hash>, evidência: <command and result>`. A card whose remaining assert needs CI, another system, a quiet machine or the author stays open with `Status: bloqueado (<reason>)`.

## Things that went wrong before

- A test passed only in the agent's worktree because an environment variable was unset there: the sessions tests built `$VUKEM_VIDEO_TEST_FFMPEG_DIR/ffmpeg` without `.exe` on Windows. Run delivered tests from a clean worktree with the variables CI sets.
- An agent's single report can be refused when it reports twice. If a notification says the report was refused, read the commits in its worktree (`git -C <worktree> log`).
- A security review fires on pushed commits. Send each finding to the owner with the fix it needs (owner scoping, fail closed on lookup errors, one transaction for read-then-act), and integrate only the fixed version.
- Timing tests flake when several agents build at once. A failure under load is re-run alone before it is called a defect, and is fixed only if it also fails alone.
- Agents ran `python3 - <<'EOF'` despite the rule against it; an empty heredoc left a process waiting on stdin as a background task. Read each report for it, stop the leftover task through the session's task list (never by image name), and repeat the rule in the next briefing.
- Read a report's "not verified" list as the real state. "Fails for a reason outside my area, probably the machine" is not a result: send it back with the exact rerun (pinned FFmpeg, private home and port) and ask for the job record's code and reason.
- A briefing names the files an agent owns. When an agent edits a file outside them because a task assert names it (an SDD status line), accept it if the report says so; otherwise revert it.
