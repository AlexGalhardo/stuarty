# ADR-0028: One legacy release, a minor release about every 90 commits, rebase-merged pull requests

- Status: Suspended (2026-11-18) by [ADR-0045](0045-epics-one-pull-request-ci-and-releases-deferred.md): no release is cut while the epics are implemented. Accepted on 2026-11-14; its rules return, revised, when the single pull request is split into releases
- Deciders: author (D-026), orchestrator
- Supersedes: ADR-0025 items 1 and 2 (the version line starting at `v0.5.0` and one release per task group); the merge method of SDD-009 section 6.3
- Links: author decision D-026, ADR-0025, SDD-009

## Context

ADR-0025 kept the Studio releases `v0.1.0` to `v0.4.0` and started the editor at `v0.5.0`, with one release per ROADMAP task group and squash-merged task pull requests. On 2026-11-14 the author asked for one legacy release and for releases that each gather a substantial amount of work.

## Decision

1. **Legacy.** One release, `v0.1.0`, holds the whole Studio history. Its annotated tag points to the last Studio release commit (`6a420b6`). The notes are the notes of the five former releases, newest first. The tags `v0.2.0`, `v0.3.0`, `v0.3.1` and `v0.4.0` and their releases no longer exist.
2. **Version line.** The editor continues with sequential minors, `v0.2.0`, `v0.3.0`, ..., until `v1.0.0`. Patch releases (`v0.N.1`) stay for bug fixes only (SDD-009 section 4 item 4).
3. **Cadence.** A release is cut when `main` has gathered about 90 commits since the previous tag (`git rev-list --count <last tag>..main`), as an average across releases, at a commit where the release pipeline is green. A phase exit (Prototype, ALPHA, BETA) is always a release, whatever the count.
4. **Merge method.** Task pull requests are rebase-merged: each commit of the branch is a Conventional Commit and reaches `main` as it is, so the changelog lists every microcommit. Footers (`BREAKING CHANGE:`, `Security:`) go in the commit that carries the change, not in the pull request body.
5. Everything else in ADR-0025 stands: annotated tags, one tag per push, a tag is never moved after this ADR (the move of `v0.1.0` is the one recorded exception), release notes from the changelog, signed artifacts.

## Consequences

- Each commit on a branch must be small, self-contained and pass the commit lint; fixup commits are folded before the merge.
- `git describe --tags --abbrev=0` on `main` prints `v0.1.0` until the first editor release.
- The first editor release is `v0.2.0`, cut by P1-REL-008 at the end of the Prototype, or earlier if about 90 commits have gathered and the release tooling exists.
