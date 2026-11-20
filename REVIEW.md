# Review rules

What a reviewer (a person or an agent) checks in a change of Stuarty: an epic branch reviewed by the specialist (ADR-0045), or a pull request. The author owns this file. It is read from the default branch, never from the pull request under review, and a change to it needs the author's review (`.github/CODEOWNERS`).

## Severity

Every finding has one severity, cites `file:line`, and is fixed or answered with a reason before merge.

- **Important:** a bug, a regression, a security issue, a broken rule of `AGENTS.md` section 3, or a missing test for a behaviour change.
- **Nit:** naming, wording or small style points the linters do not catch. At most five per review; skip them when there are Important findings.
- **Pre-existing:** a problem the change did not introduce. Report it, but it does not block the merge.

## What is a failure here

1. A rule of a domain outside `core/`, or the UI computing an edit result (ADR-0002, ADR-0031, SDD-012 §1).
2. A name, an operation, an error code or a table that is not in `docs/sdds/000-shared-definitions.md`.
3. Code that differs from its SDD without the SDD fixed in the same pull request.
4. A new dependency, or a version that is not exact, without the author's approval.
5. An outbound connection to anything but a destination the user connected, a publication or a costly generation that skips the approval queue, a credential outside the vault or returned by a tool, or any telemetry (ADR-0032, ADR-0033).
6. A skipped, focused, retried or sleeping test (`.agents/skills/verify/` section 5). An environment skip needs `verify:env-skip <reason>`.
7. Generated files edited by hand, or out of date with their generator.
8. A commit header that is not a Conventional Commit of at most 100 characters.
9. A change to the checks themselves made in the same pull request as the code they judge. That covers `.agents/skills/verify/`, this file, `.agents/rules/`, `.github/workflows/`, `scripts/gate.sh`, `scripts/smoke-e2e.sh` and the test configs. Split it, and have the author review the checks.
10. An acceptance criterion of the epic with no command and result as evidence, or checks that were not run again by the reviewer on the head commit. While CI is not a gate (ADR-0045) the evidence is the local checks of `.github/CONTRIBUTING.md`; `verify.sh --full` is required again when releases return.
11. A change that leaves the scope of its epic without saying so.

## Paths to skip

Generated folders (`site/content/generated/`, `site/generated/`, `web/resources/js/generated/`, `web/app/agentdocs/out/`, golden files under `testdata/`): check only that they match their generator.

## Who approves

- An agent's approval never counts as a person's.
- An approval covers the commit it was given on.
- Changes to auth, the update feed, CI or release workflows, or the checks listed above need the author's review.
