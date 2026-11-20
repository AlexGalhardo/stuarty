# 38: Pilot of the first complete flow

## Goal

Prove, by doing it, that a creator and their agent can go from an idea to a published short vertical video with captions and a thumbnail without leaving Stuarty: and fix everything the proof trips on.

## Context and related decisions

- ADR-0031 (the first complete flow), ADR-0043, ADR-0045.
- Author decision D-048. PRD-004 section 3, NFR-066 (an agent on a small model completes the flow), RF-389.
- Absorbs the earlier plans on the pilot with a small model and the model ladder. The harness that runs a model through the CLI exists in `internal/tools/agentharness`.

## Scope

- Modules: `scripts/first-flow.sh` (new: the scenario, scripted, with fake provider and fake connector), `internal/tools/agentharness/` (the flow as a harness task), `web/resources/skills/` (fixes to the built-in skills), `web/e2e/specs/first-flow.spec.ts` (the same flow through the UI), `docs/agent-benchmark.md` (the results), and small fixes anywhere the pilot finds a defect, each with a test.
- The flow: create a project; answer the briefing; draft and edit the script; import the footage; cut to a vertical short; captions from the transcript; a thumbnail from a template; export with a platform preset; create the publication with a caption; schedule; approve; publish; see the result on the calendar.

## Out of scope

- New features. A missing capability found by the pilot becomes an entry in the feedback queue and, if it blocks the flow, a STOP for the orchestrator.
- Commercials and long videos (epic 39).

## Dependencies

16, 19, 20, 28, 35 (and through them 04 to 10, 13, 15, 17, 18, 27).

## Technical design

- **Three runs of the same flow**, from the cheapest to the most real:
  1. **Scripted** (`scripts/first-flow.sh`): the exact CLI and MCP calls, with a fake AI provider and a fake connector, no network. It runs in the local checks from now on and is the regression test of the whole product.
  2. **Agent on a small model** (NFR-066): the harness gives a small model only the brief ("make a short from this footage about X and schedule it") and the built-in skills, with the fake provider and connector behind Stuarty. Measured: completion, number of tool calls, errors recovered, tokens. This run calls a paid API with the author's key: the harness's estimate is shown and the author approves the budget first (STOP).
  3. **By a person in the UI** (`first-flow.spec.ts` automated; and once by hand by the author with a real private upload to YouTube, if they wish).
- **Findings**: every defect, friction or confusing answer becomes a `VV` entry with the run that showed it; skills are corrected where the model went wrong because of an instruction; product defects are fixed with a reproducing test when small, or sent to the orchestrator when large.
- **Budgets**: the token cost of the discovery call and of each skill is measured again against the revised NFR-020.
- **Exit rule**: the pilot is done when run 1 and the UI spec pass, and run 2 completes the flow in at least four of five attempts on the small model, or the author accepts the measured result.

## Tasks

- [x] 1. Write `scripts/first-flow.sh` step by step, fixing what blocks it. Verify: `bash scripts/first-flow.sh` exits 0 and prints the URL the fake connector returned.
- [ ] 2. Write `web/e2e/specs/first-flow.spec.ts`. Verify: it passes in Chromium.
- [ ] 3. Add the flow as a harness task with its success check (an approved, published target exists and the exported file passes verification). Verify: `go test ./internal/tools/agentharness/...` passes with the fake model script.
- [ ] 4. Show the harness estimate and STOP for the author's budget; then run five attempts on the small model. Verify: the results table is in `docs/agent-benchmark.md`.
- [ ] 5. Triage the findings: fix skills and small defects with tests; file the rest. Verify: `go test ./web/resources/skills/...`, `task test` and `task test:web` pass; each fixed finding names its test.
- [ ] 6. Run the five attempts again after the fixes. Verify: at least four complete, or the author's acceptance is recorded.
- [ ] 7. Add `first-flow.sh` to `verify.sh --full`. Verify: `bash .agents/skills/verify/scripts/verify.sh --full` runs it.

## Test strategy

- The scripted flow as a permanent regression test.
- The UI spec.
- Harness unit tests with a fake model; real runs measured and recorded, never asserted in the test suite.

## Acceptance criteria

1. `bash scripts/first-flow.sh` exits 0 on Windows and in the Linux Docker test, with no network.
2. `first-flow.spec.ts` passes.
3. The small-model runs are recorded with completion rate, tool calls and tokens; the rate meets the exit rule or the author accepted it.
4. Every finding is a `VV` entry, fixed with a test or filed with a reason.
5. The publish step never happens without approval in any run.
6. `task lint`, `task test` and `task test:web` pass.

## Risks and mitigation

- **Real model runs cost money and vary.** Mitigation: the estimate and the author's approval; five attempts, not one; results recorded as measurements.
- **The pilot uncovers a large gap.** Mitigation: STOP and hand it to the orchestrator as a new or reopened epic instead of patching around it.

## Size

M

## Hand-off for review

The diff of `epic/38-first-complete-flow-pilot`; the results table; the list of findings with their state; the outputs of the acceptance commands.
