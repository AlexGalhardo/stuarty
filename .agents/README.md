# Agent skills and rules

This folder is for the people and agents who work on this repository. It holds what earlier sessions learned the hard way, so that the same mistakes are not made twice.

- `rules/`: workflow rules that always apply. Read them before you start: `orchestration.md`, `task-cards.md` (the work list and its completion protocol), `development-folder.md` (the git-ignored local work folder), `naming.md` (folders, files and ids), `commits.md`, `git-and-branches.md`, `ci.md`.
- `skills/`: step-by-step procedures, one per folder, each with a `SKILL.md`. Load the one that matches the task.

| Skill | Use it when |
| --- | --- |
| `skills/verify/` | before every commit (`--quick`) and before opening or updating a pull request (`--full`): boots the product end to end, runs the checks of what changed, stops for the author after three failed fix rounds |
| `skills/release-pr/` | cutting a release, rebuilding a stacked release branch, getting a release PR merged |
| `skills/ci-preflight/` | before every push: the checks that CI runs and that a Windows machine does not show by itself |
| `skills/integrate-agent-work/` | taking commits from a delegated branch into the integration line |
| `skills/deterministic-media/` | anything that runs FFmpeg and whose bytes or pixels are compared with a golden |
| `skills/cross-platform-go/` | Go code or tests that touch paths, file systems, processes or timing |

Review rules are in `REVIEW.md` at the root; they and the checks are read from the default branch and owned by the author (`.github/CODEOWNERS`).

The documents that define the product stay in `docs/` (PRDs, ADRs, SDDs); this folder never overrides them.
