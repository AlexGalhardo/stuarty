# ADR-0048: The release train: linear release pull requests of about 90 commits, rebase-merged by the orchestrator

- Status: Accepted (2026-11-20)
- Deciders: author
- Amends: ADR-0045 (the single draft integration pull request and the deferred releases); restores ADR-0028 items 3 and 4 (a release about every 90 commits, rebase-merged)
- Links: ADR-0025, ADR-0028, ADR-0045, SDD-009, `.agents/rules/git-and-branches.md`, `.agents/skills/release-pr/`

## Context

Under ADR-0045 every commit of the epics went to one integration branch, `release/0.30.0`, with one draft pull request (#106). It gathered more than 1100 commits and 200 merge commits, many with resolved conflicts. On 2026-11-20 the author asked to release it: GitHub rebase-merges at most 100 commits and refuses a pull request that needs a merge commit, so the history was cut into thirteen linear releases (v0.30.0 to v0.42.0, at most 96 commits each, every tree checked against the integration branch) and rebase-merged one by one. The author then asked for that flow to become the normal way of working, fully unattended.

## Decision

1. **The open release pull request is the integration line.** One release branch is open at a time, `release/vX.Y.Z` from `main`, with its pull request against `main`. It is linear: no merge commit.
2. **Approved work joins it as linear commits.** The orchestrator integrates an epic or fix branch the specialist approved by cherry-picking its own commits, in order (`release-train.sh integrate`), never with a merge.
3. **A release is cut at about 90 commits**, by the orchestrator, without asking the author: at 88 commits or more, and always before 96 (GitHub counts one more than `git rev-list` sometimes does). The cut writes the CHANGELOG section of the version, linked to every commit, commits `chore(release): vX.Y.Z`, rebase-merges the pull request into `main`, and starts the next release branch from the new `main`.
4. **`main` changes only through the train.** No direct push or commit to `main`, no force push. Pushing the open release branch and merging its pull request with `--admin` are part of the orchestrator's job.
5. **CI stays off and no tag is pushed** until the author turns CI back on: a pushed tag starts the publishing workflow, which needs secrets. The tags then go on the `chore(release)` commits of `main`.
6. The former integration branch `release/0.30.0` and its draft pull request #106 are frozen as the backup of the history before the train.

## Consequences

- Epic and fix branches start from the tip of the open release branch. A branch that started from a release since merged is integrated the same way: only its own commits are picked, and its merge commits stay on it.
- A rebase-merge gives the commits new ids, so the next release cannot be prepared on top of an unmerged one; it always starts from the new `main`.
- The CHANGELOG links of a release point at the commits of its release branch, which stay reachable through the pull request.
- The commits `chore(release)` of v0.30.0 and v0.31.0 carry the subject of the previous commit, by an error of the first split; they are the release commits of those versions all the same.
