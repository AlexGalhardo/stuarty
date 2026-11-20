---
name: release-pr
description: Run the release train of Stuarty (ADR-0048) - integrate approved work as linear commits on the open release pull request, cut it at about 90 commits, rebase-merge it into main and open the next one, without waiting for the author.
---

# Release pull request

The release train (ADR-0048, author's decision of 2026-11-20) replaces the single integration pull request of ADR-0045. Read `rules/git-and-branches.md` first. The tool is `bash .agents/skills/release-pr/scripts/release-train.sh`.

## 0. The train

1. **One open release at a time.** The open release branch is the integration line: `release-train.sh status` prints the branch, its version, its commit count and its pull request. It is linear and starts at `main`.
2. **Approved work joins it** with `release-train.sh integrate <branch>`: the branch's own commits, in order, cherry-picked with `-x`; merge commits and commits already on `main`, on a release branch or on the line are left out. A conflict stops the tool; resolve it, `git cherry-pick --continue`, and run `integrate` again. The tool pushes the release branch and opens its pull request when the branch has its first commit.
3. **The cut.** At 88 commits or more, `release-train.sh cut`; `integrate` cuts by itself when the line reaches 95. The cut writes the CHANGELOG section of the version from every commit since the last `chore(release)` on `main` (it replaces a section of the same version an earlier cut wrote), commits `chore(release): vX.Y.Z`, pushes, rebase-merges the pull request (`gh pr merge --rebase --admin`), waits until `main` has the same tree, and starts `release/v<next minor>` from the new `main`. The orchestrator does this on its own, as part of the loop, without asking the author.
4. **Limits.** GitHub refuses a rebase-merge above 100 commits, and one that needs a merge commit; the tool refuses to cut above 99 or with a merge commit, and never opens a pull request with an empty commit. A rebase-merge gives every commit a new id, so a branch stacked on an unmerged release cannot be prepared in advance: the next release always starts from the new `main`.
5. **GitHub release.** After the merge the cut publishes the GitHub release `vX.Y.Z` on the release commit of `main`, with the changelog section as notes, and pauses the `release` workflow meanwhile (`gh workflow disable release`, then `enable`), so the tag builds and publishes nothing while CI is off. No tag is pushed with git. The commits of v0.30.0 and v0.31.0 carry the subject of the commit before them, by an error of the first split; they are still the release commits of those versions.
6. A name already spent on the remote (a branch pushed by mistake) is not reused: the release goes to `release/vX.Y.Z-r2`, because rewriting a pushed branch needs a force push.

The sections below are the flow of ADR-0028 until release 0.29.0. The release tool (`go run ./internal/tools/release`) finds the previous release by its tag, so it is used again once the tags exist.

## 1. Size

- At most 97 commits per release pull request, the `chore(release)` commit included. GitHub refuses a rebase-merge of more than 100 commits.
- Count with `git rev-list --count origin/main..HEAD` and compare with `gh pr view N --json commits --jq '.commits|length'`.
- Count the two commits the ritual adds (the agent-docs stamp and `chore(release)`) before you decide: close the line at 95 integrated commits at most.
- When a fix pushes the count over, move the newest feature commits to the next release. On an unpushed line, split it without losing anything: `git branch release/v<next> HEAD`, then `git checkout -B release/vX.Y.Z <last commit to keep>`. Prefer a cut that also leaves out work which cannot release yet (in v0.15.0, the update feed that needed a repository secret). You can also squash your own fix commits of the same scope with `git reset --soft`.

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

1. Name the local branch `release/vX.Y.Z` and set it to track the remote. `prepare` refuses any other name. Before `prepare`, stamp the agent docs: set `Version` in `web/app/agentdocs/agentdocs.go` (the one version of the binary: `version`, `doctor` and the agent docs read it, and every release build also stamps it with `-X .../agentdocs.Version`), run the generators of step 2.4 plus `UPDATE_GOLDEN=1 go test ./web/app/mcp/ -run TestSecurity_T31_ToolDefinitionsMatchTheirGolden`, and commit `docs(cli): stamp the agent docs with version X.Y.Z`.
2. `go run ./internal/tools/release -skip-remote-checks prepare vX.Y.Z` writes the CHANGELOG section and commits `chore(release): vX.Y.Z`. Use `-skip-remote-checks` only when the tip was just rebuilt and CI will run on the push anyway.
3. `prepare` refuses while an issue labelled `ci-failure` is open. Triage it: fix it, or close it with a comment that names the commits that fixed it.
4. Push, then:
   1. `bash scripts/release-pr-body.sh vX.Y.Z`. It edits the pull request itself; never pipe its output into `gh pr edit`.
   2. `gh pr ready N`
   3. `gh pr merge N --rebase --auto`
   - While the epics are implemented the pull-request workflows are disabled by the author's decision (2026-11-15), so nothing would fire auto-merge: after the full local gate passes, merge with `gh pr merge N --rebase --admin`.
   - "This branch can't be rebased" right after a push can be GitHub still computing mergeability. Check `gh pr view N --json mergeable,mergeStateStatus` and look for an empty commit (`git diff-tree --no-commit-id -r <sha>` prints nothing); if `mergeable` is `MERGEABLE` and no commit is empty, retry the merge once.
5. If a fix is needed after `prepare`, branch from `HEAD~1`, commit the fix, move `release/vX.Y.Z` there, push with lease, and run `prepare` again. The CHANGELOG must be written last.
6. Auto-merge sometimes does not fire on a clean pull request. When every check is green, run `gh pr merge N --rebase` yourself.

## 4. Tag

1. `git switch main && git merge --ff-only origin/main`.
2. `go run ./internal/tools/release tag vX.Y.Z`: it creates the annotated tag and pushes it, and `release.yml` publishes.
3. Check the run: `gh run list --workflow release.yml --limit 2`, and `gh release view vX.Y.Z --json assets` for the expected asset count (`scripts/release-verify.sh` knows it).
4. From v0.16.0 a stable tag needs the repository secret `UPDATE_FEED_SIGNING_KEY` and the variable `UPDATE_FEED_PUBLIC_KEY` (the desktop update feed, SDD-009 §10.5); the `preflight` job fails early, naming the missing one. A stable release then has 27 assets (desktop installers, their SBOM and `feed.json` included).
5. A release the author wants to see before it is cut can exist as a GitHub draft: `gh release create vX.Y.Z --draft --target release/vX.Y.Z --notes-file <notes>`, with the notes from `git cliff --config .config/cliff.toml --unreleased --strip all`. A draft creates no tag. Refresh its notes with `gh release edit vX.Y.Z --notes-file <notes>` as the line grows; the release tool's `tag` step publishes the real release later.
6. The repository also holds tags that are not product versions (`ffmpeg-9.0.2-2`, the FFmpeg pin). Every tool that looks for the previous release must match `v[0-9]*` only: `scripts/release-pr-body.sh` uses `git describe --tags --abbrev=0 --match 'v[0-9]*'`, and `.config/goreleaser.yaml` lists `ffmpeg-*` under `git.ignore_tags`. A new script that walks tags follows the same rule.
7. Keep the line at 95 integrated commits or fewer: v0.15.0 and v0.16.0 were both cut early, and the deliveries that arrived after the cut went to the next line, which was rebuilt on `main` after the merge (section 2).
