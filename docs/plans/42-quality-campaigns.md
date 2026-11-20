# 42: Quality campaigns: budgets, fuzz, crash, soak, security

## Goal

Before the product is tested by hand from end to end, the numbers and the hard cases are settled: it is fast enough on a creator's ordinary machine, it survives being killed at the worst moment, it does not leak after hours of work, and the threat model holds now that the product talks to the outside.

## Context and related decisions

- ADR-0045 (the checks are local while CI is deferred; the QA campaign follows the epics), ADR-0003, ADR-0020, ADR-0032.
- Author decisions D-073, D-084. PRD-001 NFR-001 to NFR-013; PRD-002 NFR-020 to NFR-025; PRD-003 NFR-040, NFR-041; PRD-004 NFR-061, NFR-062.
- SDD-007 (the threat model), SDD-008 (the test strategy).
- Absorbs the earlier plans on UI performance, goldens and export performance, fuzz, crash and soak, the security acceptance, and the agent surface budgets.
- Open product issues this epic closes (their text comes in the briefing): VV-224 (browser specs that fail under parallel load and pass alone) and VV-228 (Go timing tests that fail under load). A flaky test is fixed at its cause, never with a skip, a retry or a sleep.

## Scope

- Modules: `internal/tools/perfreport/`, `scripts/bench-gate.sh`, the fuzz targets of `core/` (both document kinds), new crash and soak harnesses under `internal/tools/`, the security suite, SDD-007, SDD-008, a new `docs/security-review-stuarty.md` next to the review of the editor (`docs/security-review-1.0.md`).
- Windows on the author's machine and Linux in Docker. macOS and real Linux hardware belong to epic 43.
- **Run by the orchestrator**: the measuring tasks need every agent stopped and the machine idle (a working convention of `AGENTS.md` section 9). Seniors prepare harnesses and fix findings; the orchestrator runs the measurements.

## Out of scope

- The end-to-end QA campaign by the five seniors (`.agents/rules/plans.md` section 6): it starts when this epic is done.
- CI, release gates, signing.

## Dependencies

Every epic above it in the roadmap that is not blocked.

## Technical design

- **Performance budgets**: each NFR with a number is measured with `perfreport` on the reference projects (video) and the reference document (image, NFR-061): UI responsiveness during editing, preview playback, time to first frame, export speed, memory ceiling, start-up. A budget that fails is either fixed or, when the requirement was wrong, brought to the author with the measurement (never edited silently).
- **Fuzz**: the command cores of both document kinds (random valid and invalid batches: no panic, invariants hold, undo returns to the previous document), the parsers that read untrusted input (subtitles, bundles, skill archives, OAuth callbacks, webhook responses), each for a fixed time budget; every crash becomes a regression case.
- **Crash recovery**: a harness kills the server at chosen points (mid-batch, mid-export, mid-migration, mid-upload to the fake connector, mid-publish by the scheduler, mid-vault write) and restarts it; asserted: no corrupted document, no half-applied batch, no double publish, no lost vault, jobs resumed or failed cleanly.
- **Soak**: several hours of scripted editing, generating with fake providers and scheduling; asserted: bounded memory, file handles and database size, no stuck job, no goroutine growth.
- **Security acceptance**: every row of SDD-007 has a passing `TestSecurity_` test (`threatcheck`); `no-egress` with no connection; a fresh review of the outbound paths, the vault, the approval queue, the hosted agent and the sign-in flows by a specialist session with only SDD-007 and the code; the review is written to `docs/security-review-stuarty.md` with findings and their fixes.
- **Agent surface**: the tool list and discovery budgets, and the parity of every operation, measured again on the final surface.
- **Findings**: each becomes a `VV` entry, fixed with a reproducing test and reviewed like any delivery.

## Tasks

- [x] 1. Amend SDD-008 with the campaigns, their harnesses and their pass rules. Verify: `task lint:md` exits 0.
- [x] 2. Extend `perfreport` to the image document and the new sections; prepare the reference projects. Verify: `go test ./internal/tools/perfreport/...` passes.
- [ ] 3. Orchestrator, machine idle: measure every budget. Verify: the report with each NFR, its number and pass or fail is committed under `docs/`.
- [ ] 4. Fix or escalate each failed budget. Verify: a second idle measurement, in the same report.
- [x] 5. Add the fuzz targets and run each for its time budget. Verify: `go test -fuzz` runs recorded with duration and result; new regression cases pass in `task test`.
- [x] 6. Build the crash harness and run every kill point. Verify: `go test ./internal/tools/crashharness/...` (new) passes.
- [ ] 7. Build and run the soak. Verify: the soak report with memory, handles and database size over time is committed; its assertions pass.
- [ ] 8. Run the security acceptance and the specialist review; fix the findings. Verify: `go test -run TestSecurity_ ./...`, `go run ./internal/tools/threatcheck` and `bash scripts/no-egress.sh` exit 0; `docs/security-review-stuarty.md` is written.
- [ ] 9. Measure the agent surface again. Verify: `go run ./internal/tools/surfacebudget` and `go test ./web/contract/...` exit 0.
- [ ] 10. Run the full local gate. Verify: `bash .agents/skills/verify/scripts/verify.sh --full` exits 0, or each failure is listed with its `VV` entry.

## Test strategy

This epic is the test strategy at the scale of the product: measured budgets, time-boxed fuzzing, kill-point recovery, a soak, and the security acceptance. Everything it adds stays as a repeatable command.

## Acceptance criteria

1. Every numeric NFR has a measurement taken on the idle reference machine, and passes or carries the author's decision.
2. Each fuzz target ran for its time budget with no unfixed crash.
3. Every kill point recovers with no corruption and no double publish.
4. The soak ends within its bounds.
5. Every row of SDD-007 is covered by a passing test, `no-egress` passes, and the specialist's review has no open high-severity finding.
6. `verify.sh --full` exits 0, or what remains is listed and filed.

## Risks and mitigation

- **Measurements taken on a busy machine.** Mitigation: only the orchestrator measures, with every agent stopped; the report records what was running.
- **A budget that cannot be met.** Mitigation: the measurement goes to the author; the requirement is changed only by the author. **STOP** for that requirement.
- **A long campaign that blocks everything.** Mitigation: harness work is parallel; only the measurements are serial.

## Size

L

## Hand-off for review

The diff of `epic/42-quality-campaigns`; the performance, soak and security reports; the list of findings with their state; the output of the full gate.
