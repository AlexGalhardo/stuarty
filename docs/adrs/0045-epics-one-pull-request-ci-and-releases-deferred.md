# ADR-0045: Work is epics in a loop on one draft pull request; CI and releases are deferred

- Status: Accepted (2026-11-18)
- Deciders: author
- Suspends: ADR-0028 (release cadence and one pull request per release)
- Amends: ADR-0025 (releases and CI), SDD-009
- Links: author decisions D-049, D-056, D-070, D-072, D-073, D-080, D-082, D-083, D-084; ADR-0031

## Context

The editor shipped 28 releases in five days under the rule of one minor release about every 90 small commits (ADR-0028). The default branch has been red for several commits and its protection ruleset is disabled. The author wants to move fast on a much larger scope, with five engineers working at once and for long unattended stretches, and decided to defer release and CI work.

## Decision

1. **The unit of work is an epic**: one document in `docs/plans/`, sized to fit one senior engineer from start to review, with its acceptance criteria. The roadmap is the ordered list of epics and their dependencies. There are no phases and no milestones (ADR-0031).
2. **Two tracks run in parallel**: the editor (two seniors) and the platform (three seniors), after a short sequential foundation.
3. **An epic is worked in a fixed loop.** The senior reads the epic, does task 1, tests task 1, fixes it if a test fails, and only then moves to task 2. Every task states the command that verifies it. When every task is done and every acceptance criterion passes, the senior hands the epic to the orchestrator.
4. **Review is independent.** The orchestrator gives the epic to a specialist with no prior context: only the epic, its acceptance criteria and the diff. The specialist reviews, audits, fixes what it finds and returns the epic approved, or sends it back.
5. **One integration branch carries one draft pull request.** Each epic is a branch from it, in its own worktree. The orchestrator merges an approved epic into the integration branch and pushes it, so the draft pull request always shows the finished epics. The work-session prompt carries the author's standing authorization for that push and for five parallel agents with worktrees.
6. **`main` is never touched without the author**: no push or merge to it, no marking the pull request ready, no release and no tag.
7. **CI is not a gate for now.** The Definition of Done is local: the tests of what changed, the linters, the type check and the build, run by the engineer and run again by the specialist.
8. **When every epic is done, a QA campaign runs in a loop.** The five seniors test the whole product in parallel, end to end: every CLI command, the HTTP API, the MCP tools and every control of the UI. Each problem is recorded, fixed with a reproducing test and reviewed like an epic; then the campaign starts again. It ends when a full round finds no problem.
9. **No release is cut during the implementation.** After the QA campaign, the single pull request is split into releases (v0.30.0, v0.31.0 and so on, because GitHub refuses a rebase-merge of more than 100 commits), and CI and the branch protection are restored release by release.
10. **Conventional Commits stay**, and so do the forbidden git commands of `AGENTS.md`.

## Alternatives

- **Make `main` green and protected first.** Recommended by the orchestrator and rejected by the author: it delays every feature.
- **One release per group of epics.** Rejected by the author for now, for the same reason.
- **Ask the author before every push.** Rejected by the author: an unattended session of two days cannot wait for it, and a draft pull request is not `main`.

## Consequences

Positive: no release or CI overhead while the scope is built, and the work can run unattended with a visible, pushed record of what is finished.

Negative, accepted by the author: a regression between epics is seen only by a local check, by the QA campaign or at the end, when CI returns; the split into releases will be a large piece of work on its own.

Negative: a loop with no person in it needs hard stop conditions (an epic that fails review twice, a decision that contradicts an ADR, a new dependency, anything paid or irreversible) and a written state that survives a context compaction; both are part of the work-session prompt and of the rules in `.agents/`.

Negative: the release tooling and the skills written for the former flow (`.agents/skills/release-pr`, `.agents/skills/ci-preflight`) are unused until then and may drift.
