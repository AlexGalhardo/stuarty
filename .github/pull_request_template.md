<!--
Stuarty gathers its work on one integration branch with one pull request while
the epics of docs/plans/ are implemented (ADR-0045). An epic is a branch from the
integration branch; a contribution from outside targets the integration branch.
-->

## What this delivers

- Epic: <!-- number and title of the epic of docs/plans/, or "none" with the issue -->
- Summary: <!-- what a user or an agent can do now that they could not before -->
- Docs page: <!-- path of the guide page under site/content/docs/ that this changes or adds; "none" with the reason -->

## Acceptance criteria

<!-- Copy the acceptance criteria of the epic. For each: the command run and its result. "Works" is not a result. -->

- [ ] ...

## Checks run on this commit

- [ ] `task lint`
- [ ] `task test` and the tests of what changed (name them)
- [ ] `bun run typecheck`, `bun run check:thin-client` and `task test:web` when `web/` changed
- [ ] The Rust gate of `.agents/rules/rust.md` when `renderer/` changed
- [ ] `bash .agents/skills/verify/scripts/verify.sh --quick`

## Checklist

- [ ] Every commit is a Conventional Commit with an allowed scope and a `Refs:` line
- [ ] No rule of a domain and no edit result is computed in `web/`, a handler or a tool
- [ ] Changed contracts: `task codegen` run and the generated files committed
- [ ] New dependencies: approved by the author, exact versions; `task notices` run and `THIRD_PARTY_NOTICES.md` and `licenses/` committed
- [ ] Every specification the code now differs from (SDD, ADR) is fixed in this change
- [ ] Anything that opens a network connection goes through a connection and, where ADR-0032 says so, through the approval queue; it has a regression test and a `Security:` footer
- [ ] Friction with the vukem framework is recorded (ADR-0047)
- [ ] `CHANGELOG.md` untouched by hand
