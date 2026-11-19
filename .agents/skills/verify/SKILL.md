---
name: verify
description: Verify a change end to end before it is committed for a pull request, before a pull request is opened and before a release branch is pushed - boots the product and smoke-tests it, runs the lint, build and tests of what changed, and stops for a human after three failed fix rounds.
allowed-tools: [Bash, Read, Grep, Glob, Edit]
---

# Verify

A change is done when this skill says so, not when the code looks right. It is the smoke-test harness of the repository: keep it rock solid, and fix it on the default branch only.

## 1. When

| Moment | Command |
| --- | --- |
| After each logical step of a task, before its commit | `bash .agents/skills/verify/scripts/verify.sh --quick` |
| Before a large merge into the integration branch, and before any release branch when releases return | `bash .agents/skills/verify/scripts/verify.sh --full` |
| A senior's delivery under review by the specialist (`rules/plans.md` section 3) | the acceptance criteria of the epic and the checks of what changed; `--full` is optional while releases are deferred (ADR-0045) |

Run it from the repository or worktree root, with `STUARTY_TEST_FFMPEG_DIR` set to the LGPL FFmpeg `bin` folder so the export tests and the smoke test do not skip or pick another FFmpeg.

## 2. What it runs

- **Always:**
  - the commit headers (`skills/ci-preflight/scripts/check-commits.sh`);
  - the flaky-pattern check (`scripts/check-flaky-patterns.sh`, section 5).
- **Go files changed:** `gofmt`, `go build ./...`, then `go vet`, `go test -count=1` and `golangci-lint` on the touched packages.
- **`web/app`, `core/schema` or agent docs changed:** `go run ./web/cmd/gen-agent-docs -check`.
- **`web/` UI changed:** `bun run lint`, `typecheck`, `check:thin-client`, `check:i18n`, `bunx vitest run`.
- **`site/` or the reference changed:** `check:last-updated` and `reference:check`.
- **`--full` adds two steps:**
  - `scripts/gate.sh`, the whole local release gate;
  - `scripts/smoke-e2e.sh`, which boots the product end to end. It builds the binary and starts `stuarty serve` with a private workspace. It then creates a project, imports a generated clip, adds a clip and a title, lints and exports with `--wait`. Finally `ffprobe` checks the MP4 (video, audio, 4 s), and the server is stopped by its pid.

The last line is `VERIFY_OK` (exit 0), `VERIFY_FAIL=<n>` (exit 1) or `VERIFY_NEEDS_HUMAN` (exit 3). Logs are in `$TEMP/verify-<time>/`.

## 3. The checks come from the default branch

`verify.sh` reads its own scripts, `check-commits.sh`, `gate.sh` and `smoke-e2e.sh` from `origin/main` with `git show` and runs those against the working tree, so a change can never loosen the checks that judge it.

A change that edits a check is reported as `CHECKS_CHANGED` and ends with `VERIFY_NEEDS_HUMAN`. The checks are this skill, `REVIEW.md`, `.agents/rules/`, `.github/workflows/`, `.github/CODEOWNERS`, `scripts/gate.sh`, `scripts/smoke-e2e.sh`, and the Playwright and Vitest configs. Stop, and ask the author to review those files. An agent never approves a change to its own checks, and CODEOWNERS plus the `protect-main` ruleset require the author's review on GitHub.

## 4. The fix loop: three rounds, then a human

1. Read the failing step's log. Report each failure as `file:line`, the command, and the first error lines.
2. Find the cause, fix it, and run the same `verify.sh` mode again. That is one round.
3. Before each new round, reread what the earlier rounds tried and why it did not work. Do not repeat a fix that already failed.
4. **After three rounds and a failure that is still there, stop.** Ask the author with the question tool and stop the work on that failure. Say what failed (`file:line`, command, error), the three attempts, the hypotheses you ruled out, and your recommended next step. A senior agent stops and reports the same to the orchestrator, which asks the author.

## 5. Flaky tests

A test that fails sometimes is a defect of the code or of the test (`rules/ci.md`).

- **Never** skip it, mark it `.only`/`fixme`, add a retry (`retries:`, a rerun loop, `-count=N` to get a green run) or add a sleep (`time.Sleep`, `waitForTimeout`, `setTimeout`). `check-flaky-patterns.sh` refuses those in the lines a change adds.
- Find the cause: load, CPU count, SIMD, ordering, a shared port or folder, time. Fix it, and wait on the condition, never on a duration.
- If no cause is found, delete the test in its own commit whose body says why and names the issue that tracks the gap. Never leave it skipped.
- A skip that depends on the environment (an OS, a privilege, a missing tool), never on flakiness, carries `verify:env-skip <reason>` on the same line.

## 6. Evidence

The verification is valid for the commit it ran on. In the pull request body, or in the report of a delegated task, paste:

- the `verify.sh` mode;
- the commit hash;
- the step lines;
- the last line.

A new commit needs a new run.
