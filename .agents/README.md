# Agent skills and rules

This folder is for the people and agents who work on this repository. It holds what earlier sessions learned the hard way, so that the same mistakes are not made twice.

- `rules/`: workflow rules that always apply. Read them before you start.
- `skills/`: step-by-step procedures, one per folder, each with a `SKILL.md`. Load the one that matches the task.

| Skill | Use it when |
| --- | --- |
| `skills/release-pr/` | cutting a release, rebuilding a stacked release branch, getting a release PR merged |
| `skills/ci-preflight/` | before every push: the checks that CI runs and that a Windows machine does not show by itself |
| `skills/integrate-agent-work/` | taking commits from a delegated branch into the integration line |
| `skills/deterministic-media/` | anything that runs FFmpeg and whose bytes or pixels are compared with a golden |
| `skills/cross-platform-go/` | Go code or tests that touch paths, file systems, processes or timing |

The documents that define the product stay in `docs/` (PRDs, ADRs, SDDs); this folder never overrides them.
