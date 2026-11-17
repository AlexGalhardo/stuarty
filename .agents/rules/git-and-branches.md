# Git and branches

1. `main` is protected by the ruleset `protect-main`. Never commit or push to it. Every change reaches `main` through a release pull request that is rebase-merged.
2. One pull request per release: `release/vX.Y.Z`, titled `chore(release): vX.Y.Z updates`. Task branches never get pull requests; their commits are cherry-picked into the release line.
3. GitHub refuses a rebase-merge of more than 100 commits ("This branch can't be rebased"), and it counts one more commit than `git rev-list --count origin/main..HEAD` sometimes does. Keep every release pull request at 97 commits or fewer, the `chore(release)` commit included. Move the excess to the next release.
4. A force push is allowed only on a release branch that the author named for it, only as `git push --force-with-lease=<branch>:<sha you inspected>`. `main` is never force pushed.
5. Before a force push, look at what is on the remote. A CodeQL autofix ("Potential fix for ...") may have been committed to the branch from the GitHub interface. Keep its change, or confirm that your branch already supersedes it.
6. Never `git stash`, `git reset --hard`, `git checkout -- .`. `git reset --soft` to squash your own unpushed commits is fine.
7. Chain with `&&`. Never pipe a check into `tail` or `head` before a push: the pipe hides the exit code. Write the output to a file, test `$?`, then push.
8. Rewording a pushed commit changes its hash. The skip list by hash in `.github/workflows/pr-title.yml` then no longer covers it; shorten an over-long header instead of relying on the list.
9. No pull request is opened and no branch with a pull request is pushed without `bash .agents/skills/verify/scripts/verify.sh --full` green on that commit (`VERIFY_OK`). A change to the checks themselves (`VERIFY_NEEDS_HUMAN`) goes in its own pull request and waits for the author's review (`.github/CODEOWNERS`, `REVIEW.md`).
