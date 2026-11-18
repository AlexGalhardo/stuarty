# Plan 033: Close the CapCut feature gaps in the command core, starting with animated crop and mask

This plan answers which CapCut-class features the command core still lacks, up to four per round, and records the open decision on crop and mask keyframes. When delivered, each gap taken in a round has a core command, a test and an SDD update, and the decision on animating crop and mask is written down.

- Owner: senior-a (core); senior-b (engine goldens)
- Effort: M
- Depends on: an author decision to make crop and mask keyframeable (SDD-001 section 3.5 says they are not animatable)
- Origin: EPIC-54 (author decisions of 2026-11-16, answer Q13): TASK-0254
- Status: BLOCKED (crop and mask animation needs a decision from the author: make crop and mask keyframeable in the core)
- Written against: 8787714a

## 1. Why

The author asked for CapCut feature gaps in the core to be closed, at most four per round. Round 5 found that grade, colour keyframes and LUT were already in the engine, and fixed the spin, shrink and grow transitions by goldens. What is left cannot be done without changing a documented rule.

## 2. Current state (drift check)

```text
go test ./core/command/... ./core/plan/... ./core/lint/...
rg -n "animat" docs/sdds/001-timeline-schema.md
git log --oneline -n 10 -- core
```

Already true (round 5, 2026-11-17, senior-b): grade, colour keyframes and LUT were already in the engine (goldens `grade-*`, `color-keyframes`, `lut-*`); the transitions spin, shrink and grow are pinned by goldens (dc862e37). Missing: animated crop and mask; SDD-001 section 3.5 says they are not animatable.

## 3. Steps

### Step 1: Record the author's decision on crop and mask keyframes

Owner: orchestrator. Files: `docs/sdds/001-timeline-schema.md` (section 3.5), `docs/sdds/000-shared-definitions.md`.

Ask the author (one question, recommended option first) whether crop and mask become keyframeable; record the answer in the SDD.

Commit: `docs(sdd): decide whether crop and mask are animatable`.

Acceptance:

- [ ] SDD-001 section 3.5 states the decision and its reason; names come from SDD-000, which is changed first.

### Step 2: Make crop and mask keyframeable in the core (only if decided yes)

Owner: senior-a. Files: `core/`, SDD-001, SDD-002, SDD-003, SDD-004.

Commit: `feat(core): <english summary>`.

Acceptance:

- [ ] `go test ./core/command/... ./core/plan/... ./core/lint/...` passes; the change has a test that fails before and passes after.
- [ ] SDD-001, SDD-002, SDD-003 and SDD-004 describe the new commands and the plan fields.

### Step 3: Render and pin the new behaviour

Owner: senior-b. Files: `engine/` (compositor and goldens).

Acceptance:

- [ ] A golden per animated crop and mask case exists and `go test ./engine/...` passes.

### Step 4: Pick the next gaps of the round

Owner: orchestrator with senior-a. Files: none.

Name up to four further gaps from the research of the reference application's features, each with its command.

Acceptance:

- [ ] At most four gaps are named for the round, each with the command that closes it and its SDD section.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- UI for the new commands (plan 032 wires shortcuts when commands exist).
- Gaps beyond four in one round.
- The in-progress author-decision items of the same epic that already have commits (they are not part of this plan).

## 5. STOP conditions

- The author has not answered the crop and mask question: stop before Step 2.
- A new command would let the UI compute an edit result: stop; the core owns it.
- The Rust renderer needs the same keyframe fields: stop and coordinate with the renderer epics through the orchestrator.

## 6. Maintenance

Every command added to the core is registered in SDD-000 first, then SDD-002, and gets a golden if it changes pixels or sound.
