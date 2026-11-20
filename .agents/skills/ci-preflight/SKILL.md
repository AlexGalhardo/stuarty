---
name: ci-preflight
description: Run, before every push, the checks that the pull-request pipeline runs and that a Windows machine does not reveal by itself (cross-target lint, commit headers, generated files, web checks).
---

# CI preflight

Run every step from the repository root. Stop at the first failure and fix it before you push.

## 1. Commits

Run `bash .agents/skills/ci-preflight/scripts/check-commits.sh > out.txt; echo $?`. It replays the "Conventional commits" job on `origin/main..HEAD`: the type, the scope list, at most 100 characters, and the skip lists. Test the exit code. Never pipe it to `tail` before a push. The same holds for any check you shorten with a pipe: start with `set -o pipefail`, or the exit code of `tail` hides the failure (it did twice in v0.26.0).

## 2. Go lint on the three targets

```sh
GOOS=linux   golangci-lint run ./...
GOOS=darwin  golangci-lint run ./...
GOOS=windows golangci-lint run ./...
```

- Run one lint at a time, or pass `--allow-serial-runners --timeout 25m` so a second run waits instead of failing with "parallel golangci-lint is running". That error does not mean the code is clean.
- `golangci-lint fmt <packages>` rewrites what `golines` and `gofumpt` report; commit the result as `style(<scope>)`.
- Lints that only one target shows:
  - `unconvert` when a field type differs between operating systems;
  - `nolintlint` when a `//nolint` is needed on one target only. Split the file by build tag instead, as `disk_linux.go` and `disk_other.go` do;
  - `golines` on long argument lists.

## 3. Go tests

- `go test -count=1` on the packages you touched.
- `go test -race` on packages with concurrency.
- Tests that need FFmpeg read `STUARTY_TEST_FFMPEG_DIR`. `bash scripts/install-ffmpeg.sh` prints the directory of the pinned LGPL build. Point it at that `bin` folder: any other FFmpeg (a scoop or system one) makes those tests skip with "not the pinned LGPL build", and an export spec of the e2e suite fails for a reason that is not in the code. If `ffprobe` disappears from the folder, run the script again.
- When the full suite runs while other agents build, three signatures are the machine, not the code: "paging file too small" (`O arquivo de paginação é muito pequeno`), "failed to create new OS thread", and `[build failed]` on a package that builds alone. Rerun the package alone before calling it a defect. Tests known to be sensitive to load: `TestViewFrameColdP95`, `TestScaleProjectViewGates`, `TestEventsSlowReaderIsDropped`, `TestEventsLatencyFromCommitToClient`, `TestClientModeAgainstARunningOwner`. A test that also fails alone is a defect: `TestCancelTimeoutFailsTheJob` looked like load until it failed alone; it was a race in the test that the suspended start of a Windows child (Job Object, VUKEM VF-049) exposed.
- Performance budgets (NFR-001, NFR-004, the 200-item timeline, event latency, the export speeds) are measured only on a quiet machine. Sample the load first (`powershell -c "(Get-CimInstance Win32_Processor).LoadPercentage"` a few times); above about 20% the numbers are not evidence. Stop every agent and wait for an idle machine; record the load with each number. With several agents running the CPU sat at 85 to 100% and every timing budget failed for no code reason.
- Many `TIME_WAIT` sockets (`netstat -ano | grep -c TIME_WAIT` in the thousands) make `web/contract` answer "the server did not answer" and Playwright fail with `EADDRINUSE`: run one e2e project at a time with `--workers=2` at most.
- A full disk fails builds in odd places and can corrupt the Go build cache. `go clean -cache` frees tens of gigabytes; check `df -h` first when errors make no sense.
- Keep the gate as one script that runs each step, records its exit code and prints only the failing steps, so a truncated log never hides a failure (`TestDriversCoverService` was missed once by cutting a `grep` short).

## 4. Generated files

Run the generators of `skills/release-pr/` step 2.4. After that, `git status` must be clean; commit whatever changed. The site's "Last updated" check compares `site/content/last-updated.json` with the date of the last commit of each page, so run `bun run last-updated` after the commits that touch the pages.

Hand-kept lists that a new feature must update in the same change:

- A new browser page route (an Inertia page such as `/settings/brand-kit`) goes into the `builtin` list of `TestSecurity_T05_EveryOperationHasScope` in `web/app/http/security_test.go`.
- A new threat row in SDD-007 changes the count in `TestTheRealModelParses` (`internal/tools/threatcheck/check_test.go`).
- An error code a command can return goes into its `Errors` in the registry, because agents read that list. When only the service raises it before `Apply` (a font hash missing from the store), the core has no golden case for it: add the command and code to `serviceOnlyCodes` in `core/command/cases_test.go`, cover it in a service test, and say so in the SDD-002 error table.
- `core/command` never imports `core/plan` (`internal/tools/importcheck`). A test that needs both lives in `core/plan`.

## 5. Web and site

- In `web/`: `bun install --frozen-lockfile`, then `bun run lint`, `bun run typecheck`, `bun run check:i18n`, `bun run check:thin-client`, `bun run test`.
- `check:i18n` runs with Node: the TypeScript 7 API it uses (`typescript/unstable/sync`) reads a Node child-process internal that Bun does not have. Do not move it back to Bun, and do not use `Bun.*` APIs in it.
- In `site/`, which uses Bun: `bun run lint`, `bun run test:scripts`.
- Markdown: `bunx markdownlint-cli2@0.24.1 "**/*.md"`.

## 6. Security and notices

- `go test -run TestSecurity_ ./...`.
- `go run ./internal/tools/notices -check` after any dependency change.
- Expect CodeQL findings on new code. Common ones: `go/incorrect-integer-conversion` (use `strconv.Atoi` for an `int`), and `js/incomplete-sanitization` (escape the backslash before the character you escape).
- Secret scan: `betterleaks git . --config .betterleaks.toml --no-banner --redact --log-opts="origin/main..HEAD"`. It scans every commit of the range, so a false positive cannot be fixed by editing the file afterwards; add a narrow allowlist to `.betterleaks.toml` (path AND line shape) and say why in a comment. Run with `--report-format json --report-path <file>` to see the rule, file and line, which the CI log hides.
- Rate limits apply in local mode too. A test suite that starts many exports or imports a minute sets `STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE` high in its server environment (see `web/e2e/playwright.config.ts`).
- Never pipe a test command into `grep` or `tail` inside a chain that ends in `git push`: the pipe hides the exit code and the push goes out red. Run the tests, check `$?` (or use `set -o pipefail`), and push in a separate step only after a clean result.
