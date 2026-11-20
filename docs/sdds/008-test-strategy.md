# SDD-008: Test strategy

- Status: Accepted (2026-11-14). Written by the architect after the other SDDs, so that it names their tests instead of inventing new ones.
- Depends on: every SDD (each has an Acceptance section that this document organises), ADR-0003 (tolerance), ADR-0006, ADR-0014, ADR-0025
- Implements: the verification side of NFR-001 to NFR-013, NFR-020 to NFR-025, NFR-040 to NFR-045; RF-104 and NFR-025 (parity)
- Owner area: each engineer owns the tests of the code they own. Cross-area suites: senior-d (CLI, MCP, contract, CI wiring), senior-e (browser), senior-b (reference scenes, performance record), senior-c (security coordination, crash tests).

## 1. Purpose

Say which kinds of test exist, what each one proves, where it lives, what it is compared against, and when it runs. A task of the ROADMAP is done when the tests it names pass; this document is where an engineer finds what "a golden case", "a reference scene" or "the reference scenario" means, so that nobody defines them twice.

Three rules shape everything:

1. **The core is tested without media.** `core/` is pure, so its tests need no FFmpeg, no database and no browser, and run in milliseconds. Most behaviour of the product is proved there.
2. **One scenario, every door.** Whatever an agent can do through MCP, the CLI or HTTP, and a person through the UI, is proved by driving the same scenario through each and comparing documents (RF-104, NFR-025).
3. **Numbers are measured, recorded and guarded.** Every non-functional requirement has a named benchmark or test, a limit, and a place where the measured value is written.

## 2. The kinds of test

| Kind | Proves | Lives in | Compared against | Runs |
| --- | --- | --- | --- | --- |
| Unit and table tests | one function or type | next to the code (`*_test.go`, `*.test.ts`) | expected values in the test | every pull request, three operating systems |
| Golden cases of commands | each command's result, inverse, diff and errors | `core/command/testdata/<command>/<case>/` | files of the case (section 3) | every pull request |
| Property tests | undo restores the document; invariants always hold; summaries equal the document | `core/command/`, `service/` | the property itself, on random input with a fixed seed | every pull request |
| Schema and contract generation | Go types, JSON Schema, TypeScript types and the registry snapshot agree | `internal/tools/contractgen/` and the "Generated files" job | regeneration leaves the tree unchanged | every pull request |
| Golden text | the texts written for agents (views, diff, lint, history) | `service/testdata/text/`, `web/app/mcp/testdata/` | text files, byte for byte | every pull request |
| Golden plans | `core/plan` output for the reference scenes | `core/plan/testdata/` | JSON, byte for byte, on three operating systems | every pull request |
| Golden frames | pixels composed by the export executor | `engine/export/testdata/scenes/` | hashes of composed frames, and SSIM where an encoder is involved (section 4) | every pull request |
| Preview against export | the browser preview stays within tolerance | `web/e2e/specs/preview-vs-export.spec.ts` | the frame from `ViewFrame` (section 4) | every pull request, Chromium |
| Contract and parity | the four ways of reaching `Service` behave the same | `web/contract/` | byte-equal documents, equal error codes | every pull request |
| End-to-end scenarios | the product does what a user came for | `web/e2e/` | exported files verified by the core | every pull request |
| Crash injection | no committed change is lost, no half state | `service/crashtest/` | invariants after reopening the database | every pull request (build tag `crashtest`) |
| Security tests | each mitigation of SDD-007 | `TestSecurity_T<nn>_*` in the package that owns the code | the behaviour stated in the threat row | every pull request, dedicated job |
| Fuzzing | parsers do not panic or hang | `Fuzz*` functions | no crash | 30 s per changed target on pull requests; weekly long run |
| Benchmarks | the non-functional requirements | `Benchmark*` functions and `internal/tools/perfreport/` | limits of section 7 | every pull request (regression), release (absolute) |
| Soak | memory and stability over time | `web/e2e/specs/soak-5min.spec.ts` | limits of NFR-007 and NFR-008 | nightly |
| Agent run | a real agent completes the reference tasks using only the shipped material | `internal/tools/agentharness/`, run by `.github/workflows/agent-harness.yml` | exported file verified; number of tool calls recorded; transcripts saved | by hand before each release, `workflow_dispatch` only (it costs money, needs the secret `ANTHROPIC_API_KEY` and is not deterministic) |

Naming: Go tests are `Test<Subject>_<Behaviour>`; security tests `TestSecurity_T<nn>_<Behaviour>`; benchmarks `Benchmark<Subject>`; Playwright specs are named after the scenario (`prototype-reference`, `overlap-snapback`). A test named in an SDD keeps that exact name, because the ROADMAP and the threat check refer to it.

## 3. Golden cases of the command core

SDD-002 §A.11 defines the unit: a directory per case.

```text
core/command/testdata/item.split/ok_split_in_the_middle/
├── before.json      the document before, canonical
├── op.json          {"op": "...", "args": {...}} as a caller would send it
├── after.json       the document after (success cases)
├── inverse.json     the inverse primitives
├── diff.json        the diff
├── effective.json   the effective values
└── error.json       instead of after, inverse, diff, effective (error cases)
```

Rules:

1. Case names come from SDD-002 Part C: every section lists its golden cases, `ok_*` for success and `err_<code>_*` for each error. `TestGoldenCasesComplete` fails when a name of Part C has no directory, or a directory has no name in Part C.
2. The runner applies `op.json` to `before.json` and compares every output file byte for byte. For success cases it then applies the inverse and requires `before.json` again (undo is exact).
3. `error.json` is compared on `code`, `path`, `op_index` and the keys of `details`; the message is compared as text, because messages are part of the contract for agents.
4. Updating a golden file is a reviewed change: `go test ./core/command/... -update` rewrites them, and the pull request shows the difference. A golden file never changes in a pull request that does not explain why.
5. Analysis-dependent commands (`clip.remove_silences`, `caption.from_transcript`, `caption.retime_from_clip`) carry `resolved` in `op.json`, so the case needs no media.
6. Identifiers in cases are supplied, never generated, so files are stable.

Property tests over the registry, seed fixed and printed on failure: apply then inverse is the identity on canonical bytes; every result satisfies `timeline.Validate`; `Apply` does not mutate its input; applying a batch equals applying its operations one by one.

## 4. Pictures: golden frames and the tolerance

ADR-0003 and SDD-004 §11 define two different comparisons, and they must not be confused.

| Comparison | What is compared | Rule | Why |
| --- | --- | --- | --- |
| **Exact** | frames composed by the Go compositor, before encoding, against stored hashes | SHA-256 equal, on Linux, macOS and Windows | our own code produced every pixel from decoded frames of generated media; nothing may differ (NFR-010) |
| **Within tolerance** | (a) a frame decoded from the exported file against the composed frame; (b) the browser preview during playback against `ViewFrame` | (a) SSIM of at least 0.95 on luma (SDD-004 §11); (b) SSIM of at least 0.97 on luma, and no layer displaced by more than 1 pixel at preview resolution (NFR-011) | an encoder and a browser are not ours to make bit-exact |

**Reference scenes.** A scene is a project document plus generated media, under `engine/export/testdata/scenes/<name>/`. Media is generated by `scripts/gen-fixtures.sh` with the pinned FFmpeg (`testsrc2`, sine tones, a rotated clip, a variable-rate clip); no media file is committed. There are three first scenes (a clip with a caption; two clips with a cut; a clip in contain mode with a text item); later scenes add one per feature that draws something new (transition, keyframed transform, colour adjustment, shape, word-highlight caption, speed change, crossfade), and the three starter templates instantiated.

For each scene the test stores the hash of 30 frames spread over the timeline. A scene is added in the same pull request as the feature it covers. When a hash changes, the pull request must say which rule of SDD-004 changed; the comparison tool writes the before and after images and a difference image as artifacts of the job.

**Displacement** is measured, not eyeballed: each scene has markers (a high-contrast corner pattern on each layer) and the test locates them in both images.

**Decoded input.** Exact comparison needs identical decoded frames on every machine. The fixtures are encoded once per run with the pinned FFmpeg and decoded by the same build on Linux and Windows. On macOS the FFmpeg comes from Homebrew (risk R6 of the ROADMAP), so the exact comparison there uses frames decoded on Linux and stored as an artifact of the workflow; the macOS job proves the compositor, not the decoder.

## 5. Scenarios end to end

Two scenarios are named and used as exit criteria of phases.

**Reference scenario 1.** Create a 9:16 project; import the generated portrait clip; add it to the timeline; split it and delete one part; add a caption; lint; view a frame; export; wait; verify the file. Driven three ways: an MCP client over stdio, the CLI, and Playwright in Chromium. Checks: each export passes the verification of SDD-004 §8; the three documents are byte-equal after identifiers are normalized; with the UI open while the CLI drives, every change is visible within 200 ms at the 95th percentile.

**Reference scenario 2.** From a generated talking-head recording with pauses (speech synthesized offline and committed as text plus a script that renders it, or the test transcription provider), an agent transcribes, removes silences, creates captions from the transcript, applies a caption style, and exports; then instantiates each of the three starter templates with the same media and exports them. Checks: verification of each export; captions inside the safe area (lint clean); the caption words line up with the audio within 2 frames of the reference timings.

Transcription in CI uses a **test provider** that returns a stored transcript for a known audio hash, so that CI needs no model download; the real whisper.cpp path is exercised by a separate job that installs the helper explicitly and runs on Linux only.

Browser scenarios are the list of SDD-012 §20.1, by name. Each one runs against a real server started by the test with a temporary workspace (`STUARTY_HOME`), never against mocks of the API: the UI is a thin client, and a mocked core would prove nothing.

## 6. Parity and contracts

`web/contract/` holds scenarios as data (a list of operations with supplied identifiers). A scenario runs through: the in-process `Service`, the HTTP client `Service`, the CLI as a subprocess with `--json`, and the MCP server over stdio. Assertions: the final documents are byte-equal; the diffs are equal; failure scenarios return the same code through every door; the exit code of the CLI follows SDD-000 §8.

The parity table of SDD-003 §7.1 is itself a test: every operation of SDD-000 §1 must be reachable through each surface that the table lists for it. Removing a tool, a command or a route fails the test.

Generated contracts (ADR-0014) are tested by regeneration: `task codegen && git diff --exit-code`. The OpenAPI document is validated and compared with the router. The examples inside the SDDs are extracted from the Markdown and validated against the schemas (SDD-006 §11 check 2), so a document cannot drift from the code without a red job.

## 7. Performance: what is measured and where

The **reference runner** is the GitHub-hosted `ubuntu-latest` runner; absolute limits in CI are calibrated on it by P1-QA-002 and written into this table. The **reference machine** is the one of the spikes (Ryzen 7 5700X3D, 32 GB, Windows 11), used for the numbers quoted in the SDDs and for the checks the orchestrator runs before a release.

| Requirement | Limit | Test or benchmark | Owner |
| --- | --- | --- | --- |
| NFR-001 command round trip | 50 ms p95, 200 items | `BenchmarkCommit200Items` (store share 25 ms), and `vv:edit` measured in the browser | senior-c, senior-e |
| NFR-002 change reaches another UI | 200 ms p95 | the observer test of reference scenario 1; event latency 20 ms p95 (SDD-006 §11 check 5) | senior-c, senior-e |
| NFR-003 proxy frame on seek | 100 ms p95 | Playwright `scrub-latency` | senior-e |
| NFR-004 exact frame after pause | 500 ms p95 | Playwright `play-pause-truth` (SDD-012) | senior-e |
| NFR-005 ingest of 60 s 1080p | 30 s | `BenchmarkIngest60s` | senior-b |
| NFR-006 export of 60 s vertical | 60 s with a hardware or system encoder | `BenchmarkExport60s` on the reference machine; `BenchmarkExportCommonCase` at least 1.5 times real time on the reference runner | senior-b |
| NFR-007 Go process memory | under 500 MB during an export | the export test samples resident memory | senior-b |
| NFR-008 browser tab memory | under 1 GB with a 5 minute project | `soak-5min` (nightly) | senior-e |
| NFR-009 the large project | all of the above on 5 minutes, 200 items, 2,000 words | the `project-200` fixture used by every benchmark above | each owner |
| NFR-010 determinism | identical composed frames on two runs | golden frames (section 4) and `TestCompositorBandsEqual` | senior-b |
| NFR-011 preview tolerance | SSIM 0.97, 1 pixel | `preview-vs-export` | senior-e |
| NFR-012 old projects open | every released schema version | fixtures per version (section 9) | senior-a |
| NFR-013 no committed change lost | crash suite | `service/crashtest` | senior-c |
| NFR-020 tool definitions and discovery | 7,300 tokens of `tools/list` (23,400 bytes), 2,650 tokens for the first discovery call | `go run ./internal/tools/surfacebudget` and `TestToolDefinitionBudget` (SDD-003) | senior-d |
| NFR-021 concise view | pages of at most 4,000 tokens; 200 items in at most two pages | `TestTextBudgets`, `TestPaging` (SDD-003) | senior-d |
| NFR-022 image cost | 800 and 1,600 image tokens | `TestImageSizeTable` (SDD-003) | senior-b |
| NFR-023 frame and contact sheet for an agent | frame under 1 s, contact sheet under 2 s, median | `BenchmarkViewFrame`, `BenchmarkViewSheet9` (SDD-003) | senior-b |
| NFR-024 CLI read with a server | median under 300 ms | `BenchmarkCLIReadViaServer` (SDD-003) | senior-d |
| NFR-025 same document through every door | byte-equal | contract tests (section 6) | senior-d |
| NFR-061 image document | 4,000 by 4,000 pixels with 30 layers: composed in 100 ms, exported in 2 s | `BenchmarkImageReference` (`compose_ms`, `s_export`; the browser half is the Playwright spec of the image editor) | senior-b |
| NFR-040 first export in 10 minutes | 10 minutes | the getting-started page timed by the author (SDD-010 acceptance 19) | senior-d |
| NFR-041 archive size | 60 MB without FFmpeg | the release job | senior-d |
| NFR-042 pull-request pipeline | 20 minutes | measured on a real pull request and recorded in the ROADMAP task | senior-d |
| NFR-043 no GPL component | none | the notices and image smoke test (SDD-009 §7 and §11) | senior-d |
| NFR-044 release by a tag | only the release pull request and the tag are manual | the release procedure (SDD-009 §7) | senior-d |
| NFR-045 three operating systems | every pull request on Linux, macOS and Windows | the CI matrix (SDD-009 §13.3) | senior-d |
| PRD-001 success metric 4 | a 60 s vertical video with automatic captions, from import to exported file, in under 5 minutes | manual check of the release checklist, timed by the orchestrator | orchestrator |

`internal/tools/perfreport` runs the Go benchmarks, writes one table per platform (`docs/perf/<date>-<goos>.md`, with the values in a comment for the comparison) and compares it with the last committed table of the same platform: a regression of more than 20% fails the job (`-compare`). Absolute limits are checked on the reference runner with `-absolute`. The benchmarks it runs are `BenchmarkCommit200Items` (NFR-001), `BenchmarkIngest60s` (NFR-005), `BenchmarkExport60s` (NFR-006 and NFR-007, which reports `peak_MB`), `BenchmarkExportCommonCase` (the 1.5 times real time floor), and `BenchmarkViewFrame` and `BenchmarkViewSheet9` (NFR-023); the list is `Specs` in the tool. The comparison of composed frames writes the frame, and with `STUARTY_TEST_GOLDEN_BEFORE_DIR` the frame before and a difference image, into `STUARTY_TEST_ARTIFACTS_DIR` for the job to upload; `STUARTY_TEST_OTHER_FFMPEG_DIR` runs the PSNR comparison against another FFmpeg build (reported, not blocking). Token counts are estimated as characters divided by 3.2 (SDD-003 §3.10) and are confirmed once per release with the real tokenizer through the agent run.

## 8. Fixtures

1. **No binary media in the repository.** Media is generated by `scripts/gen-fixtures.sh` into a cache directory keyed by the script's hash and the FFmpeg version; CI caches it. The script is deterministic (fixed seeds, fixed encoder settings).
2. **Documents** used as fixtures are canonical JSON files with supplied identifiers. The standard ones: `empty`, `one-clip`, `reference-scene-1..3`, `project-200` (5 minutes, 200 items, 2,000 caption words), one per schema version under `core/timeline/testdata/versions/`.
3. **Hostile files** for the security tests (a playlist renamed to `.mp4`, an archive with a traversal entry, a truncated font) are generated by the tests themselves, never committed as opaque blobs, so that a reviewer can read what they are.
4. A fixture is shared when two areas need it; then it lives under `testdata/shared/` at the repository root and has one owner named in a `README.md` there.

## 9. Compatibility and migration

Every released schema version keeps a set of documents under `core/timeline/testdata/versions/v<N>/`. `TestMigrateAllVersions` opens each with the current code and requires a valid document; SDD-005 §18 check 26 does the same through a workspace. From the first release on, the release checklist adds the current fixtures to the version set. Before 1.0, a workspace created by each released version is kept as an archive attached to that release and opened by the release candidate (ROADMAP, Release 1.0 exit criterion 2).

## 10. Flaky tests and time

- Tests never sleep to wait for something: they wait on an event, a file, a row, or use an injected clock (`app.Now()` in Go, fake timers in the browser).
- A test that fails without a code change is a bug with priority: it is fixed or quarantined within the same day by its owner, with an issue; a quarantined test does not count as coverage and blocks the next release until it is back.
- **Absolute time budgets run only on an idle machine.** A test whose assertion is a number of milliseconds on the wall clock (a p95, a cold start, a slow-reader latency) is a measurement, not a functional check: under the parallel suite or with other programs running it measures the machine. Such a test is split in two. The functional assertion (the result is right, the work was done, the cost does not grow with the input measured as a count of operations or as a ratio with a wide margin) runs always. The absolute budget runs only when `STUARTY_PERF=1` is set, through the helper `perfgate.Require(t)` (package `internal/perfgate`), which skips the test with the reason otherwise. The measurement is not lost: section 13.1 makes the idle run with `STUARTY_PERF=1` a mandatory step of the gate, by the orchestrator, with every agent stopped.
- A test that races a timer against a real process (a watchdog, an encoder that dies, a slow reader) waits on the event it asserts (a channel, a file, a state change) and never on a deadline that load can overrun. When the code under test reads the clock, the test injects it.
- Retries are allowed only in Playwright, at most one, and a test that passed on retry is listed in the job summary.
- Port numbers come from the operating system (port 0); workspaces are temporary directories; no test touches `~/.stuarty`.
- Coverage floors (SDD-009 §13.2): 90% for each package of `core/`, 80% elsewhere, enforced by `covergate`. Coverage is a floor, not a goal: the golden cases and the scenarios are what prove behaviour.

## 11. What runs when

| Moment | What runs | Budget |
| --- | --- | --- |
| Before a commit (git hook) | format, lint of changed files, `go vet`, unit tests of changed packages | under 30 s |
| Every pull request | everything of section 2 marked "every pull request", on Linux, macOS and Windows; browser tests in Chromium | under 20 minutes (NFR-042) |
| Nightly | soak, long fuzzing, the whisper.cpp job, the FFmpeg freshness check | no limit |
| Before a release | `task ci` on Windows 11 by the orchestrator; absolute performance limits on the reference machine; the agent run; the verification commands of SDD-007 §13.2 on the artifacts | by hand, recorded in the release checklist |

## 12. Acceptance

1. Every test named in an Acceptance section of an SDD exists with that name, or the SDD was corrected (`go test -list` and the Playwright list are compared with the names by a script, `internal/tools/testnames`).
2. `TestGoldenCasesComplete` passes: Part C of SDD-002 and `core/command/testdata/` list the same cases.
3. The three reference scenes exist with generated media and 30 hashed frames each, equal on three operating systems.
4. The contract tests drive at least the reference scenario and five failure scenarios through the four doors.
5. `perfreport` writes its table and the pipeline fails on a regression of more than 20% (shown once with a deliberately slowed function).
6. No media file larger than 100 KB is tracked (`git ls-files` checked by a script in CI).
7. The pull-request pipeline finishes in under 20 minutes with all of the above.

## 13. Quality campaigns

Epic 42 settles the numbers and the hard cases before the product is tested by hand. Each campaign is a repeatable command with a pass rule; the measuring ones are run by the orchestrator with every agent stopped and the machine idle (`AGENTS.md` section 9), and the report records what else was running.

### 13.1 Performance budgets

- `go run ./internal/tools/perfreport -write -absolute -gated` measures every row of section 7 that a benchmark can measure, on the reference projects (video: `project-200`; image: the reference document of NFR-061, a 2,000 by 2,000 pixel document with 20 layers), and fails when a limit is missed. The table is `docs/perf/<date>-<goos>.md`.
- The absolute budgets that live in tests (section 10) are listed in `Gates` of the tool; `-gated` runs them one package at a time with `STUARTY_PERF=1` and writes their result into the same table. A budget that is skipped or missing counts as failed, so the gate cannot pass without having measured. Today they are `TestViewFrameColdP95` and `TestScaleProjectViewTimeBudgets` (NFR-004), `TestEventsSlowReaderDoesNotSlowCommits` (NFR-002) and `TestTransitionEmptyLintTimeBudget`.
- A budget that fails is fixed, or, when the requirement was wrong, taken to the author with the measurement; the requirement is never edited silently.

### 13.2 Fuzzing

Every parser of untrusted input and the command core of both document kinds has a `Fuzz*` target. A target runs for a fixed budget (30 s on a pull request, 10 minutes in the long run) with `go test -run '^$' -fuzz '^<Target>$' -fuzztime <budget> <package>`. Pass rule: no panic, no hang, the invariants hold (a batch is valid or rejected whole; undo returns the previous document byte for byte; a parser returns an error or a value that validates). A crash is minimised by the fuzzer into `testdata/fuzz/<Target>/` and stays there as a regression case that `go test` runs on every pull request. `scripts/fuzz.sh` runs the whole list with the budget as its argument.

### 13.3 Crash recovery

A child process is ended by a hard kill at a named kill point and what it left is reopened. The kill points are named by `service/crashpoint` (`crashpoint.Hit`); with the `crashtest` build tag the point named by `STUARTY_CRASH_AT` ends the process with exit code 137, as a SIGKILL would. `bash scripts/crash.sh` runs every one: the tests of `web/app/servicetest` (tag `crashtest`) kill the commit path (mid-batch, 10 points, for an edit, an undo and a redo), the bundle export and import, the migration and the emptying of the trash, and a real `serve` in the middle of concurrent commits; `internal/tools/crashharness` kills the vault in the middle of a write (before and after the rename) and holds `TestEveryCrashPointHasARecoveryTest`, which fails when a `crashpoint.Hit` in the code has no listed test that kills it, so a point cannot be added without its recovery test.

Pass rule: the database opens and is consistent; the document is the revision before or the revision after the batch, never between; a half-written file is never published; the vault opens and holds every secret that was acknowledged, and the one being written is there only if its rename happened; a job of the killed process is resumed or ends `failed` with a stable code, none stays `running`. A publication is sent at most once: a post that was being sent when the process died is ended as interrupted when its lease runs out and is never sent again by the next scheduler (`TestSchedulerRestartMidPublishResumes` and `TestSchedulerStopsAPostThatKeepsInterrupting` in `service/publish`, on the store of the scheduler tests).

### 13.4 Soak

`go run ./internal/tools/soak -duration 4h -report docs/quality/<date>-soak.md` drives the Service in process, on a real store in a fresh workspace, with a seeded script: three video projects (text items added and deleted, undo and redo, reads) and two image documents (a layer added and undone), about 90% edits. Every 30 s it forces a collection and samples the heap in use, the goroutines, the open handles (`GetProcessHandleCount` on Windows, the descriptors of the process elsewhere), the size of the database and its log, and the jobs that are not final. A step that the rules refuse is counted; an internal error ends the run. Pass rule, after a baseline taken once the warm-up is over (a quarter of the duration, at most 10 minutes): the heap in use at the end is within 25% of the baseline, goroutines and handles within 10 of it, the database grows at most 2 KB per commit after the baseline, and no job is queued or running at the end. `TestSmokeRunStaysBounded` runs the same script for 6 s in `go test`. Generation with a fake provider and the scheduler are not in the script yet: their lifecycle is held by the scenario and crash tests, and a soak of them needs the provider and connector fakes to leave the test packages.

### 13.5 Security acceptance

Every row of SDD-007 has a passing `TestSecurity_` test (`threatcheck`); `scripts/no-egress.sh` passes with no connection configured; a specialist session with only SDD-007 and the code reviews the outbound paths, the vault, the approval queue, the hosted agent and the sign-in flows, and writes `docs/security-review-stuarty.md` with each finding and its fix. Pass rule: no open finding of high severity.

### 13.6 Agent surface

`go run ./internal/tools/surfacebudget` and `go test ./web/contract/...` are run again on the final surface; the budgets of NFR-020 and NFR-021 hold or carry the author's decision.
