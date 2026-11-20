# Agent skills and rules

This folder is for the people and agents who work on this repository. It holds what earlier sessions learned the hard way, so that the same mistakes are not made twice. The team itself (five seniors, an intern and a specialist) is defined in `.claude/agents/` and described in `AGENTS.md` section 4.

- `rules/`: workflow rules that always apply. Read them before you start: `orchestration.md` (how the orchestrator, the seniors, the intern and the specialist work together), `plans.md` (the epics of `docs/plans/`, the unit of work, and their completion protocol), `git-and-branches.md` (the release train, one branch per epic), `commits.md`, `ci.md`, `naming.md` (folders, files and ids), `development-folder.md` (the git-ignored local work folder), `rust.md` (the renderer).
- `skills/`: step-by-step procedures, one per folder, each with a `SKILL.md`. Load the one that matches the task.

| Skill | Use it when |
| --- | --- |
| `skills/verify/` | before every commit (`--quick --base <integration branch>`: without `--base` the change is compared with `main` and every epic is reported as changing the checks): runs the checks of what changed. `--full` boots the product end to end and runs the gate; it is not required while releases are deferred (ADR-0045) and is useful before a large merge |
| `skills/orchestrator-loop/` | running the work session as the orchestrator: the loop that picks, dispatches, reviews, merges, pushes and keeps the state, for hours unattended |
| `skills/epic-loop/` | working one epic (or one fix of the feedback queue) as its senior: one task, its `Verify:` command, fix until it passes, commit, next task, then the hand-off |
| `skills/qa-campaign/` | after the epics: the end-to-end rounds with five seniors, one surface each, until a full round finds no problem |
| `skills/integrate-agent-work/` | merging an approved epic into the integration branch |
| `skills/deterministic-media/` | anything that runs FFmpeg and whose bytes or pixels are compared with a golden |
| `skills/cross-platform-go/` | Go code or tests that touch paths, file systems, processes or timing |
| `skills/renderer-parity/` | any change of the renderer's output |
| `skills/ci-preflight/` | the checks that CI runs and that a Windows machine does not show by itself; run its cross-target lint for code that depends on the operating system. The rest waits for CI to be a gate again |
| `skills/release-pr/` | the release train (ADR-0048): integrating approved work as linear commits on the open release pull request, cutting it at about 90 commits, rebase-merging it into `main` and opening the next one (`scripts/release-train.sh`) |

Review rules are in `REVIEW.md` at the root; they are owned by the author (`.github/CODEOWNERS`).

The documents that define the product stay in `docs/` (PRDs, ADRs, SDDs); this folder never overrides them.
