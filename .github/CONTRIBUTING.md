# Contributing to Vukem Video

Thank you for helping. Vukem Video is in the Prototype phase, so most of the code does not exist yet; the planning documents in [`docs/`](../docs/README.md) are the contract. Start with the requirements ([`docs/prds/`](../docs/prds)) and the specification of the area you will touch ([`docs/sdds/`](../docs/sdds)).

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities privately, as described in [SECURITY](SECURITY.md), never in a public issue.

## Rules that are never traded

1. **One command core.** Every editing rule lives in `core/` (Go, pure, no I/O). The UI, the MCP server, the CLI and the HTTP API are thin adapters over the service. A rule about timelines written in a handler, a tool or in TypeScript is rejected in review, even "for now".
2. **The UI never computes an edit result.** It draws what the core says and sends commands. `bun run check:thin-client` (in `web/`) enforces it.
3. **Names come from one place.** Operations, tables, error codes and variables are in [`docs/sdds/000-shared-definitions.md`](../docs/sdds/000-shared-definitions.md). Change it first, then everything else.
4. **A specification is the contract.** When the code must differ from an SDD, fix the SDD in the same pull request and say why.
5. **No outbound network connection by default, no telemetry.**
6. **No new dependency without the maintainer's approval.** Versions are exact, never `latest`, `next`, `rc`, `beta` or `alpha`.
7. **Everything in the repository is in English.**

## Commits and pull request titles

Conventional Commits 1.0 in English:

```text
<type>(<scope>)!: <description>

[body]

[footer(s)]
```

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `style`, `revert`.
- The description is imperative, starts with a lower-case letter, has no trailing period, and the header is at most 100 characters.
- The scope is optional. When present it is one of `core`, `timeline`, `command`, `plan`, `lint`, `engine`, `raster`, `ingest`, `export`, `transcribe`, `service`, `web`, `api`, `mcp`, `cli`, `ui`, `site`, `docs`, `repo`, `ci`, `deps`, `deps-dev`, `release`.
- A breaking change has `!` after the type or scope and a `BREAKING CHANGE:` footer that describes the impact.
- A security change on a `feat`, `fix`, `perf` or `refactor` has a `Security: <one line on the risk closed>` footer.
- The task or the issue goes in the body: `Refs: P1-ENG-003` or `Fixes #123`.
- One logical change per commit and per pull request.

Pull requests are rebase-merged (ADR-0028): every commit of the branch reaches `main` as it is and is what the changelog reads, so each commit must be a small, self-contained Conventional Commit, and the footers above go in the commit that carries the change. Fold fixup commits before asking for the merge. `CHANGELOG.md` is generated from the commits by the release tool and is not edited by hand.

## Pull requests and completion

1. One pull request per task or issue, titled as a Conventional Commit.
2. Fill in the pull request template: the task or issue, the checks you ran, the thin-client rule and, if you found friction with the vukem framework, its record.
3. A report that something works is a claim, not evidence. A change is done when its output exists and is tested, and every check of its task has been run and its result is visible in the pull request. A change that cannot meet a check is not silently changed: explain why in the pull request.
4. The maintainer reviews, fixes the last defects, rebase-merges and pushes. Nobody else pushes to `main`.

## Checks

The task runner (`task lint`, `task test`, `task ci`) and the pull request pipeline arrive with later tasks. Until then run what exists: `go build ./...`, `go vet ./...` and `go test ./...` at the root, and the scripts of `web/package.json` in `web/` (Bun, not npm). The same checks run on Linux, macOS and Windows.

## Editor settings

Indentation is tabs of width 4, except in files whose format needs spaces (YAML, Markdown lists and tables). Git expects LF endings in the repository.

## Security-sensitive changes

A change that touches authentication, path handling, the FFmpeg invocation policy or anything that opens a network connection needs a regression test and a `Security:` footer. Read [SDD-007](../docs/sdds/007-security-model.md) first.
