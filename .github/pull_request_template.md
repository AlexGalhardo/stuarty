## Summary

<!-- What does this change and why? End with the ROADMAP task ("Refs: P1-ENG-003") or the issue it closes ("Fixes #123"). -->

## Checklist

- [ ] PR title is a Conventional Commit with an allowed scope; body has `Refs: <task>`
- [ ] Task id: <!-- e.g. P1-ENG-003 -->; the asserts of the task were run and their results are pasted below
- [ ] `task ci` passes locally (until the task runner exists: `go build ./...`, `go vet ./...`, `go test ./...` and the `web/` scripts)
- [ ] Thin-client rule: no editing rule or edit result is computed in `web/`, a handler or a tool; `bun run check:thin-client` passes when `web/` changed
- [ ] Friction with the vukem framework, if any, is recorded in `vukem-feedback.md` (maintainers) or described here
- [ ] Changed contracts: `task generate` run and the generated files committed
- [ ] Changed dependencies: approved by the maintainer, exact versions, `task notices` run
- [ ] A specification that the code now differs from (SDD, ADR) is fixed in this pull request
- [ ] New user-facing behaviour has its documentation page, or the page is listed here as pending
- [ ] No new `//nolint` without a one-line reason
- [ ] Workflows: actions pinned by full SHA with a `# vX.Y.Z` comment; minimal `permissions`
- [ ] `CHANGELOG.md` untouched (generated from commits)
- [ ] Breaking changes use `!` and a `BREAKING CHANGE:` footer in this pull request body
- [ ] `Security:` footer, when it applies, is in this pull request body (it becomes the squash commit message)

## Asserts run

<!-- Command and result, one per line. -->
