# 64: Agents in the recording flow

## Goal

An agent prepares a recording, marks takes and mistakes, and proposes cuts, while the person stays in control: only a person starts a capture, and the capture is always visibly on.

## Context and related decisions

- Author decision (2026-11-23): an agent never starts a capture alone, and the recording indicator is always visible.
- ADR-0051 (agent consent for capture), SDD-015 (approval), RF-135, RF-136, NFR-020.

## User stories

- As an agent, I want to prepare a recording with its sources and script so that the person only has to press record.
- As an agent, I want to mark a take as good or bad so that the edit starts from that.
- As an agent, I want to propose cuts so that the person can accept them before anything changes.
- As a creator, I want to see at any time that a capture is on so that nothing records me unnoticed.

## Scope

- Modules: `core/recording/`, `service/` recording and approvals, the adapters, the chat and the recording UI, a curated skill in `web/resources/skills/`, SDD-023, SDD-007.
- Must not touch: `capture/` except the indicator signal, the publishing domain.

## Out of scope

Any way for an agent to start or resume capture, standing approval rules for capture, automatic application of a proposal.

## Dependencies

61 and 62.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop and web | Prepared recording card, indicator, proposal review |
| HTTP, CLI | Every operation below |
| MCP | Through existing tools. **No new tool unless the author raises the budget of NFR-020** |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Scope | Who |
| --- | --- | --- |
| `RecordingPrepare` | `media` | Agent or person. Creates a `prepared` recording and starts nothing |
| `RecordingMark` | `edit` | Agent or person. Adds a take or mistake marker |
| `RecordingProposeCuts` | `edit` | Agent or person. Creates a proposal and changes no timeline |
| `RecordingProposalDecide` | `edit` | **A person in a privileged browser session only** |
| `RecordingStart` | `media` | Unchanged: a person only, and approval rules never apply to it |

## Technical design

- A prepared recording stores sources, script id and notes. A person presses record on it.
- An approval rule cannot match `RecordingStart`, and a test proves it.
- **Indicator:** the desktop shell shows a system-level indicator from the moment the helper starts until it exits, driven by the host and not by the UI, so a closed window cannot hide it. The web relies on the browser's indicator plus a page banner.
- A proposal is a list of ranges with a reason. On acceptance, the server applies it as one undoable batch through existing commands.

## Tasks

- [ ] 1. SDD-023, SDD-000, SDD-007 and the agent docs. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Prepare, mark and the proposal rules in the core. Verify: `go test ./core/recording/...` passes.
- [ ] 3. Service operations and the person rule, including approval rules. Verify: `go test ./service/...` and `go test -run TestSecurity_ ./...` pass.
- [ ] 4. The host-driven indicator on Windows. Verify: a test with a fake helper that the indicator signal starts and ends with the process, and a manual check recorded in the hand-off.
- [ ] 5. HTTP, CLI and MCP within the budget. Verify: `go test ./web/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 6. The UI: prepared card, proposal review with accept and reject per range. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `task test:web` and `bun run e2e` pass.
- [ ] 7. A curated skill for the flow. Verify: the skill lint of the repository passes through `task lint`.

## Test strategy

Security tests over every agent surface; a test walking all approval rule shapes; Playwright for the review.

## Acceptance criteria

1. **Given** an agent over MCP, CLI or HTTP, **when** it tries by any operation to start or resume capture, **then** no helper process starts (`go test -run TestSecurity_ ./...`).
2. **Given** an approval rule of any shape, **when** evaluated for `RecordingStart`, **then** it does not authorize it.
3. **Given** a capture running with the window closed, **when** the desktop is observed, **then** the indicator is still visible.
4. **Given** an agent's proposal, **when** it is created, **then** the timeline revision does not change.
5. **Given** a person accepts it, **when** it is applied, **then** one batch changes the timeline and one undo restores it.
6. `task lint`, `task test`, `task test:web` pass and `surfacebudget` exits 0.

## Risks and mitigation

- A hijacked agent trying to record the user: enforced in the service, independent of every adapter.
- MCP budget: operations go through existing tools.
- **STOP** if the budget of NFR-020 must rise, which is the author's decision, or if the indicator cannot be guaranteed on a system.

## Size

L: a security boundary across all surfaces.

## Priority

Must. It is the differentiator of Stuarty and the safety rule of the feature.

## Hand-off for review

The commits, the security tests, the manual indicator check, each criterion with its command and result.
