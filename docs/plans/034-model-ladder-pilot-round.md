# Plan 034: Pilot round of the model ladder campaign

This plan answers whether the model ladder campaign (Haiku, Sonnet and Opus as first-class users of the product through the agent interfaces) is calibrated before the first full round. When delivered, a short pilot has run with only Haiku and only the L0 and V01 scripts, its report follows the final report format, and every adjustment it asks for is made in the script, the briefing or the verifier and noted in the protocol.

- Owner: orchestrator
- Effort: M
- Depends on: the author asking for the round (a round only runs when the author asks; each round takes hours); the agent harness and the verification script and test assets already prepared
- Origin: EPIC-57: TASK-0284
- Status: TODO
- Written against: 8787714a

## 1. Why

Author request of 2026-11-16: the three models read only documentation, `--help`, MCP schemas and the OpenAPI, and each gap between Haiku and Opus becomes a `VV-NNN` issue with its cause in the product. The public output is `docs/agent-benchmark.md`. A pilot avoids spending a long round on a broken script.

## 2. Current state (drift check)

```text
ls docs/agent-benchmark.md
git log --oneline -n 5 -- docs/agent-benchmark.md
```

Already true:

- The protocol, the standard request ("run the QA plan") and its 10 sub-sections are written in the maintainers' private QA plan, and `docs/agent-benchmark.md` is in `docs/README.md` (5a9c736a and 7f7a984d, release v0.24.0).
- The verification script exists (stdlib only; profiles `api-basic`, `cli-basic` and `v01..v21`; common and specific asserts; JSON and Markdown output; `--self-test` with 23 checks ok), but it has not run against real exports yet; ducking is checked from the document only.
- The test assets: 40 aliases checked; 10 new (A-LUT, A-SCREEN, A-SRT, A-VTT, A-PODCAST-1/2, A-PHOTOS, A-STICKER, A-BRAND, A-FONT) with their licence recorded; the beats of A-MUSIC-2 were extracted (180 beats, about 91 BPM). Pending: one sfx-riser row has shifted columns and the Mixkit licence of A-MUSIC-2 is not verified.

## 3. Steps

### Step 1: Clear the pending asset items

Owner: estagiario-1. Files: the private test assets and their licence list (kept out of git).

Acceptance:

- [ ] The sfx-riser row has its columns fixed and the licence of A-MUSIC-2 is verified, or the asset is replaced by a verified one.

### Step 2: Run the pilot with only Haiku

Owner: orchestrator. Files: none in the repository.

Run the 13 steps of section 0 of the private QA plan with only Haiku and only the L0 and V01 scripts (TV-LAD-001 and TV-LAD-101).

Acceptance:

- [ ] The 13 steps of section 0 are run with only Haiku and only L0 and V01, and the report follows the format of section 10.10 of the private QA plan.

### Step 3: Apply the adjustments

Owner: orchestrator. Files: the private QA plan and verifier.

Acceptance:

- [ ] Every adjustment the pilot asks for in the script, the briefing or the verifier is made before the first full round and noted in section 10.1 of the private QA plan.
- [ ] Any product gap found is opened as a `VV-NNN` feedback issue with its cause in the product.

## 4. Out of scope

- The full rounds with Sonnet and Opus (they run only when the author asks).
- Product performance measurement (only at the end of a campaign, with the machine idle).
- Changing product code to help a model.

## 5. STOP conditions

- The author has not asked for the round: stop; do not start it.
- The agent under test is given anything beyond docs, `--help`, MCP schemas and the OpenAPI: stop; the result is void.
- The verifier fails its own `--self-test`: stop and fix it first.

## 6. Maintenance

Update the private QA plan and `docs/agent-benchmark.md` after each round; re-run the pilot when the verifier or the briefing changes in a way that affects scoring.
