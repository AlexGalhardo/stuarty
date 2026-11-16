<!--
Vukem Video gathers its work in one pull request per release (D-031):
title `chore(release): vX.Y.Z updates`, branch `release/vX.Y.Z`, about 90
microcommits, rebase-merged. Refresh the body with `task release:pr-body`;
it writes the sections below from the commits, each linked by its hash.
-->

Release pull request for **vX.Y.Z**: every microcommit since the previous tag lands on `main` as it is (rebase-merge, ADR-0028). After the merge, the tag `vX.Y.Z` is pushed with the author's yes.

- Commits since the previous tag: N (target: about 90 per release, D-026)
- Tasks: <!-- ids of the task cards (TASK-NNNN) this release closes -->
- Docs page: <!-- path of the guide page under site/content/docs/ that a feat commit changes or adds; "none" with the reason when no page applies -->

### Added

- **scope:** What is new ([abc1234](https://github.com/AlexGalhardo/vukem-video/commit/abc1234))

### Changed

- **scope:** A breaking change of a public interface ([abc1234](https://github.com/AlexGalhardo/vukem-video/commit/abc1234))

### Removed

- **scope:** What was taken out ([abc1234](https://github.com/AlexGalhardo/vukem-video/commit/abc1234))

### Fixed

- **scope:** What was corrected ([abc1234](https://github.com/AlexGalhardo/vukem-video/commit/abc1234))

### Security

- **scope:** A security fix or hardening ([abc1234](https://github.com/AlexGalhardo/vukem-video/commit/abc1234))

## Checklist

- [ ] Title is `chore(release): vX.Y.Z updates`; every commit is a Conventional Commit with an allowed scope and a `Refs:` line
- [ ] The body was refreshed with `task release:pr-body` after the last push
- [ ] All required checks are green on the three runners
- [ ] The asserts of every task card named above were run, and the commands with their exit codes are in this description or in the `Status` line of each card
- [ ] Thin-client rule: no editing rule or edit result is computed in `web/`, a handler or a tool
- [ ] Changed contracts: `task codegen` run and the generated files committed
- [ ] New dependencies: approved by the author, exact versions; `task notices` run and `THIRD_PARTY_NOTICES.md` and `licenses/` committed (a Dependabot update that changes `go.mod`, `go.sum` or `web/bun.lock` fails `Notices` until a maintainer does this on its branch)
- [ ] Every specification the code now differs from (SDD, ADR) is fixed in this release
- [ ] Friction with the vukem framework is recorded in `vukem-feedback.md`
- [ ] `CHANGELOG.md` untouched by hand (the release tool writes it on `release:prepare`)
