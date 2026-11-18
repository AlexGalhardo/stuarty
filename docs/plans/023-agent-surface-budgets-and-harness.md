# Plan 023: Agent surface budgets and the agent harness runs with a real model

This plan answers whether a real model completes the prototype scenario and the template scenario from text feedback alone, and whether the agent surface meets its token, image and time budgets with the final catalogue. When delivered, the acceptance table of SDD-003 (checks 1 to 29) is re-run with the numbers attached, 20 harness transcripts are attached to the release, and the vendor token count of `tools/list` is at most 6,000.

- Owner: senior-d
- Effort: M
- Depends on: a paid model API key (`ANTHROPIC_API_KEY`) and the author's approval to spend it; plan 021 for the performance numbers of NFR-023
- Origin: EPIC-08 (P2-AGT-007): TASK-0041; EPIC-09 (P2-AGT-009): TASK-0045; EPIC-18 (P3-AGT-002): TASK-0087, TASK-0088; EPIC-33 (P4-AGT-001): TASK-0158, TASK-0159, TASK-0160
- Status: BLOCKED (needs a real CI run, a paid API key, a clean machine or another system, or the author)
- Written against: 8787714a

## 1. Why

PRD-002 success metrics 1 and 2 and SDD-003 check 29 require that an agent finishes each scenario in at least 9 of 10 runs with text feedback only. SDD-003 section 9.1 states the vendor token count was never measured and check 29 was never run, so "29 of 29" cannot be attached to the 1.0 notes.

## 2. Current state (drift check)

```text
go test ./web/app/mcp/... ./web/contract/... ./web/app/agentdocs/...
go test ./web/app/cli/...
ls scripts/agent-harness
git diff --stat -- web/contract
```

Already true (from the former card statuses):

- 13 lines of the budget card were met; 8 lines of the recipes card; 10 lines of the bundle and parity card; 7 lines of the freeze card.
- `go test ./web/app/...` was run on 2026-11-16: every package ok except `web/app/http` `TestEventsSlowReaderIsDropped` (a p95 commit-time assertion, 41 to 70 ms against a baseline of 24 to 46 ms; failed twice on the loaded machine); `./web/contract/...` ok.
- `-race` on the three operating systems was not run (Windows only, as SDD-003 section 9.1 says).
- Equality of the archive `agent/` files with `agent:docs --print` was not verified.
- No 1.0 snapshot difference is recorded.
- The path `scripts/agent-harness/` named by the former cards did not exist in the tree when this plan was written; check it first (see STOP conditions).

## 3. Steps

### Step 1: Count the vendor tokens of `tools/list`

Owner: senior-d. Files: `web/app/mcp/`, `web/app/mcp/testdata/budgets/`.

Run `TestToolListVendorTokens` with the paid key.

Commit: `test(cli): <english summary>` or none if only measurements.

Acceptance:

- [ ] The vendor token count of `tools/list` is at most 6,000 (NFR-020); the number is recorded in the release notes.

### Step 2: Re-run the SDD-003 acceptance table

Owner: senior-d. Files: `web/app/mcp/`, `web/contract/`, `web/app/agentdocs/`, `docs/sdds/003-agent-interfaces.md`.

Re-run checks 1 to 29 and attach the numbers.

Commit: `docs(sdd): record the agent acceptance numbers`.

Acceptance:

- [ ] `go test ./web/app/mcp/... ./web/contract/... ./web/app/agentdocs/...` passes.
- [ ] `go test ./web/app/... ./web/contract/...` passes with `-race` on the three operating systems.
- [ ] The concise view of the 200-item project fits its page; a frame costs at most 800 image tokens and a sheet at most 1,600; a sheet takes under 2 s; a CLI read takes under 300 ms (NFR-020 to NFR-024, numbers recorded in the release notes).

### Step 3: Re-run the agent harness for the prototype scenario

Owner: senior-d. Files: `scripts/agent-harness/`, `web/app/cli/`, `web/contract/`, `web/app/agentdocs/`.

Acceptance:

- [ ] The agent completes the prototype scenario in at least 9 of 10 runs (PRD-002 metric 2), with text feedback only.
- [ ] `go test ./web/app/cli/... ./web/contract/... ./web/app/agentdocs/...` passes.

### Step 4: Run the harness for the template scenario and attach the transcripts

Owner: senior-d. Files: `web/contract/scenarios/`, `web/app/agentdocs/recipes/`, `scripts/agent-harness/`.

Acceptance:

- [ ] The agent completes the prototype scenario in at least 9 of 10 runs and the template scenario in at least 9 of 10 (PRD-002 metrics 1 and 2; SDD-003 section 9 check 29).
- [ ] The 20 transcripts are attached to the release.
- [ ] The files of the archive `agent/` folder equal `agent:docs --print`.

### Step 5: Close the documents

Owner: senior-d. Files: `docs/sdds/003-agent-interfaces.md`.

Acceptance:

- [ ] SDD-003 section 9.1 no longer says check 29 and the vendor token count were not run, and "29 of 29" is attached.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- Changing a budget to make a number pass.
- The model ladder campaign (plan 034).
- The agent studio skills.

## 5. STOP conditions

- `scripts/agent-harness/` does not exist or cannot run the 20 scenarios: stop and ask the orchestrator where the harness lives (the former cards name that path, the tree did not have it).
- Fewer than 9 of 10 runs pass: stop; the cause is in the documentation or the tool surface, and fixing it is a new piece of work for the orchestrator to approve.
- No paid key is available: stop; do not substitute a different model without the author.
- The flaky `TestEventsSlowReaderIsDropped` fails on an idle machine: report it as a regression candidate against the previous tag in a detached worktree before fixing.

## 6. Maintenance

Re-run the table at every release candidate and after any change to the tool catalogue, schemas or recipes. The token budgets are release-checklist items; record the new numbers in each release's notes.
