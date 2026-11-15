---
name: ci-preflight
description: Run, before every push, the checks that the pull-request pipeline runs and that a Windows machine does not reveal by itself (cross-target lint, commit headers, generated files, web checks).
---

# CI preflight

Run every step from the repository root. Stop at the first failure and fix it before you push.

## 1. Commits

Run `bash .agents/skills/ci-preflight/scripts/check-commits.sh > out.txt; echo $?`. It replays the "Conventional commits" job on `origin/main..HEAD`: the type, the scope list, at most 100 characters, and the skip lists. Test the exit code. Never pipe it to `tail` before a push.

## 2. Go lint on the three targets

```sh
GOOS=linux   golangci-lint run ./...
GOOS=darwin  golangci-lint run ./...
GOOS=windows golangci-lint run ./...
```

- Run one lint at a time. A second run fails with "parallel golangci-lint is running". Wait and retry; that error does not mean the code is clean.
- Lints that only one target shows:
  - `unconvert` when a field type differs between operating systems;
  - `nolintlint` when a `//nolint` is needed on one target only. Split the file by build tag instead, as `disk_linux.go` and `disk_other.go` do;
  - `golines` on long argument lists.

## 3. Go tests

- `go test -count=1` on the packages you touched.
- `go test -race` on packages with concurrency.
- Tests that need FFmpeg read `VUKEM_VIDEO_TEST_FFMPEG_DIR`. `bash scripts/install-ffmpeg.sh` prints the directory of the pinned LGPL build.

## 4. Generated files

Run the generators of `skills/release-pr/` step 2.4. After that, `git status` must be clean; commit whatever changed. The site's "Last updated" check compares `site/content/last-updated.json` with the date of the last commit of each page, so run `npm run last-updated` after the commits that touch the pages.

## 5. Web and site

- In `web/`: `bun install --frozen-lockfile`, then `bun run lint`, `bun run typecheck`, `bun run check:i18n`, `bun run check:thin-client`, `bun run test`.
- `check:i18n` runs with Node: the TypeScript 7 API it uses (`typescript/unstable/sync`) reads a Node child-process internal that Bun does not have. Do not move it back to Bun, and do not use `Bun.*` APIs in it.
- In `site/`, which uses npm: `npm run lint`, `npm run test:scripts`.
- Markdown: `bunx markdownlint-cli2@0.24.1 "**/*.md"`.

## 6. Security and notices

- `go test -run TestSecurity_ ./...`.
- `go run ./internal/tools/notices -check` after any dependency change.
- Expect CodeQL findings on new code. Common ones: `go/incorrect-integer-conversion` (use `strconv.Atoi` for an `int`), and `js/incomplete-sanitization` (escape the backslash before the character you escape).
