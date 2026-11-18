# Git and branches

The process is ADR-0045: one integration branch, one pull request, no release while the epics are implemented.

1. Never commit or push to `main`. Without the author: no merge to `main`, no marking the pull request ready, no release, no tag.
2. **One integration branch carries the single draft pull request.** Every epic is a branch from it, `epic/NN-<slug>`, and a fix of the feedback queue is `fix/VV-NNN-<slug>`. Epic branches are never pushed and never get pull requests of their own.
3. **Only the orchestrator pushes, and only the integration branch**, after merging an epic the specialist approved, so that the draft pull request shows what is finished. The work-session prompt carries the author's authorization; without it, do not push. Never push a branch whose local checks failed.
4. **Each agent works in its own worktree outside the repository folder**, in the sibling folder `stuarty-worktrees/<agent>`, created by the orchestrator from the integration branch. Parallel agents and worktrees need the author's explicit yes; the work-session prompt carries it.
5. The orchestrator merges an approved epic into the integration branch with a merge that keeps its commits (`git merge --no-ff epic/NN-<slug>`), then removes the worktree and the local branch. Shared files (`rules/plans.md` section 4) are merged one epic at a time.
6. Before starting, an epic branch is created from the tip of the integration branch. A long epic is updated by merging the integration branch into it, never by rebasing commits another agent may have based work on.
7. Never `git stash`, `git reset --hard`, `git checkout -- .`. `git reset --soft` to squash your own unpushed commits is fine. No force push.
8. Chain with `&&`. Never pipe a check into `tail` or `head` before a push: the pipe hides the exit code. Write the output to a file, test `$?`, then push.
9. When the epics are done and the QA campaign ended (`rules/plans.md` section 6), the integration pull request is split into releases (`v0.30.0`, `v0.31.0` and so on). GitHub refuses a rebase-merge of more than 100 commits ("This branch can't be rebased"), and it counts one more commit than `git rev-list --count origin/main..HEAD` sometimes does: keep every release pull request at 97 commits or fewer. The rules of that split are written, with the author, when it happens.
10. Old worktrees and merged branches are removed when their epic is done. A branch that was deleted on 2026-11-18 can be recovered from the maintainers' backup bundle; ask the author.
