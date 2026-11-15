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
4. Check the diff against the briefing with `git diff --stat`, and revert what is outside the agent's area unless it is a minimal compile fix the report names.
5. Run `skills/ci-preflight/`, at least the package tests, the cross-target lint and the generators.
6. Push the line, mark the ROADMAP, and reply to the agent with the next task in the same message, so it never waits idle.

## Things that went wrong before

- A test passed only in the agent's worktree because an environment variable was unset there: the sessions tests built `$VUKEM_VIDEO_TEST_FFMPEG_DIR/ffmpeg` without `.exe` on Windows. Run delivered tests from a clean worktree with the variables CI sets.
- An agent's single report can be refused when it reports twice. If a notification says the report was refused, read the commits in its worktree (`git -C <worktree> log`).
- A security review fires on pushed commits. Send each finding to the owner with the fix it needs (owner scoping, fail closed on lookup errors, one transaction for read-then-act), and integrate only the fixed version.
- Timing tests flake when several agents build at once. A failure under load is re-run alone before it is called a defect, and is fixed only if it also fails alone.
