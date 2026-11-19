# 03: Drop the phase labels from the registry and the specifications

## Goal

Nobody reading the code or the specifications meets a schedule that no longer exists. A command is in the catalogue or it is not; "Prototype", "ALPHA", "BETA" and "1.0" stop appearing as if they still decided what is built.

## Context and related decisions

- ADR-0045 (no phases and no milestones), ADR-0031. Author decision D-049.
- The command registry carries a phase per command (`core/command/registry.go`: `Phase`, `Prototype`, `Alpha`, `Beta`), `TestPartCSectionsMatchTheRegistry` compares it with SDD-002, and `internal/tools/threatcheck` has a `-phase` flag and a `beta.go` allowlist.

## Scope

- Modules: `core/command` (the registry and every command registration), `core/schema` (the generated registry snapshot), `internal/tools/threatcheck`, the tests that read a phase, `web/resources/js/generated/`, `site/` reference pages generated from the registry.
- Documents: SDD-000 to SDD-013 (phase columns, "(phase A)" in headings, "frozen for ALPHA", "until the BETA exit"), `docs/design-system.md`, `docs/README.md` (remove the paragraph that explains why the labels are still there).

## Out of scope

- Renaming or removing any command, argument, error code or route.
- `docs/security-review-1.0.md`, `docs/agent-benchmark.md` and `CHANGELOG.md`: they are records of the past.
- The ADRs: their text is history; only a status line may be touched.

## Dependencies

02.

## Technical design

- Remove the `Phase` type and field from the registry and from the JSON the registry snapshot emits. If a consumer filters by phase (an "exposed build is BETA" rule), every registered command becomes exposed; state it in the hand-off.
- SDD-002 Part B tables lose their phase column; Part C headings become `### C.N \`name\``. The test that parsed the phase now checks only that every Part C section has a registry entry and the reverse, with the same count.
- `threatcheck`: remove `-phase`; fold the allowlist of `beta.go` into the main check or delete it when it is empty of meaning. The threat rows of SDD-007 keep their ids.
- Prose: replace a phase by what it meant ("built", "planned by epic NN", or nothing). Do not invent dates.

## Tasks

- [x] 1. Remove the phase from the registry and from the snapshot; update the generators. Verify: `task codegen` leaves no diff after a second run; `go build ./...` exits 0.
- [ ] 2. Rewrite the tests that read a phase. Verify: `go test ./core/command/... ./core/schema/...` passes and the count of Part C sections is asserted.
- [ ] 3. Remove the phase column and labels from SDD-002 and the other SDDs. Verify: `git grep -nE "\b(ALPHA|BETA|Prototype)\b|phase [PAB]\b" -- docs/sdds docs/design-system.md` prints nothing.
- [ ] 4. Simplify `threatcheck`. Verify: `go test ./internal/tools/threatcheck/...` passes and `go run ./internal/tools/threatcheck` prints that every threat has its tests.
- [ ] 5. Regenerate the TypeScript types and the site reference; fix UI or site text that shows a phase. Verify: `bun run typecheck` in `web/` and `task site:build` exit 0.
- [ ] 6. Remove the explanatory notes from `docs/README.md` and the "Amended" lines of SDD-002 and SDD-009 that only spoke of phases. Verify: `task lint:md` exits 0.

## Test strategy

- The registry tests keep proving that documentation and code list the same commands.
- The surface test (`TestSurfaceFrozen`) and the parity tests must pass unchanged: no behaviour changes.

## Acceptance criteria

1. `git grep -n "Phase" -- core/command` prints nothing that refers to a product phase.
2. `git grep -nE "\b(ALPHA|BETA|Prototype|MVP)\b" -- docs/sdds docs/prds docs/README.md docs/design-system.md .agents AGENTS.md README.md` prints nothing.
3. `task test`, `go run ./internal/tools/threatcheck`, `task generate:check`, `task lint` exit 0.
4. `web/contract/testdata/surface.txt` is unchanged.

## Risks and mitigation

- **A command hidden by its phase becomes visible.** Mitigation: list in the hand-off every command whose exposure changed; if any was hidden on purpose, STOP and ask.
- **Mechanical edits damage a specification that tests read.** Mitigation: run the reading tests after each SDD.

## Size

S

## Hand-off for review

The diff of `epic/03-drop-the-phase-labels`; the list of commands whose exposure changed (or "none"); the outputs of the acceptance commands.
