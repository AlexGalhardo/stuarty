---
name: release-pr
description: Cut a release of Vukem Video through one rebase-merged pull request, including stacked release branches that must be rebuilt on main after the previous release merges.
---

# Release pull request

Read `rules/git-and-branches.md` first. The release tool is `go run ./internal/tools/release` (SDD-009).

## 1. Size

- At most 97 commits per release pull request, the `chore(release)` commit included. GitHub refuses a rebase-merge of more than 100 commits.
- Count with `git rev-list --count origin/main..HEAD` and compare with `gh pr view N --json commits --jq '.commits|length'`.
- When a fix pushes the count over, move the newest feature commits to the next release. You can also squash your own fix commits of the same scope with `git reset --soft`.

## 2. Rebuild a stacked branch on main

After release N is merged, its commits get new hashes on `main`, and branch N+1, which was built on top of the old ones, must be rebuilt:

1. `git fetch origin && git switch -c rebuild/vX.Y.Z origin/main`.
2. Cherry-pick the commits of the old branch that are not on `main`. Filter by patch with `git cherry`, then by subject: a commit whose conflict was resolved has another patch id.
3. Conflicts in generated files (`site/content/last-updated.json`, `site/content/generated/**`, `web/app/agentdocs/out/**`, `site/generated/openapi.json`): take either side, then regenerate at the end (step 4). Any other conflict is resolved by hand.
4. Regenerate everything derived and commit what changes:
   - `go run ./internal/tools/contractgen`
   - `go run ./internal/tools/ffmpegpins`
   - `cd web && bun run codegen:ts`
   - `UPDATE_OPENAPI=1 go test ./web/contract/ -run OpenAPI`
   - `cd site && npm run reference && npm run last-updated`
   - `go run ./web/cmd/gen-agent-docs`
   - `go run ./internal/tools/notices`
5. Run `skills/ci-preflight/`.
6. Push with `git push --force-with-lease=release/vX.Y.Z:<remote sha you inspected> origin rebuild/vX.Y.Z:release/vX.Y.Z`, and only on a branch the author allowed.
7. `gh pr edit N --base main`.

## 3. Prepare and merge

1. Name the local branch `release/vX.Y.Z` and set it to track the remote. `prepare` refuses any other name.
2. `go run ./internal/tools/release -skip-remote-checks prepare vX.Y.Z` writes the CHANGELOG section and commits `chore(release): vX.Y.Z`. Use `-skip-remote-checks` only when the tip was just rebuilt and CI will run on the push anyway.
3. `prepare` refuses while an issue labelled `ci-failure` is open. Triage it: fix it, or close it with a comment that names the commits that fixed it.
4. Push, then:
   1. `bash scripts/release-pr-body.sh vX.Y.Z`
   2. `gh pr ready N`
   3. `gh pr merge N --rebase --auto`
5. If a fix is needed after `prepare`, branch from `HEAD~1`, commit the fix, move `release/vX.Y.Z` there, push with lease, and run `prepare` again. The CHANGELOG must be written last.
6. Auto-merge sometimes does not fire on a clean pull request. When every check is green, run `gh pr merge N --rebase` yourself.

## 4. Tag

1. `git switch main && git merge --ff-only origin/main`.
2. `go run ./internal/tools/release tag vX.Y.Z`: it creates the annotated tag and pushes it, and `release.yml` publishes.
3. Check the run: `gh run list --workflow release.yml --limit 2`.
