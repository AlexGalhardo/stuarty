# 13: MCP tools by domain and generated agent documentation

## Goal

An agent that connects to Stuarty finds a short list of tools that covers every domain, learns in a few hundred tokens what it can do, and keeps working as the product grows from video to images, writing and publishing: with no tool list that explodes.

## Context and related decisions

- ADR-0043 (few tools by intent, with a domain), ADR-0015 (the MCP server), ADR-0016 (the CLI), ADR-0017 (the HTTP API), ADR-0014 (contracts in JSON Schema).
- Author decision D-069. PRD-002 RF-100 to RF-119, NFR-020 (token budget, revised once here), NFR-021 to NFR-025.
- Absorbs the earlier plan on agent surface budgets and the harness.
- Author decision D-088: `project_get` keeps accepting `detail: "document"` over MCP, like the CLI and the HTTP API (SDD-003 lists decision S-1 as withdrawn). The document is unbounded, so the revised budget of this epic states that this one call is outside the per-call ceiling and that the tool description says what it costs.

## Scope

- Modules: `core/command/registry.go` and `core/schema/` (a `Domain` on every operation), `web/app/mcp/`, `web/app/cli/` (commands grouped by domain), `internal/tools/` (the generators of agent documents and the budget check), `docs/sdds/003-agent-interfaces.md`, SDD-000, the generated agent documents under `web/` and `site/`.
- The mechanism only: the video domain and the project domain (epic 06) are moved onto it. Every later epic registers its own operations with its domain.

## Out of scope

- New operations of any domain.
- The hosted chat (epic 14) and the skills (epic 15).

## Dependencies

06.

## Technical design

- **Registry**: each operation declares `Domain` (`project`, `video`, `image`, `writing`, `asset`, `publish`, `provider`, `memory`, `workspace`). The registry snapshot test and SDD-000 gain the column.
- **Tools by intent**: the existing sixteen tools stay under their names (the frozen surface). The intent tools (describe, list, apply commands, validate, export, job control) accept `domain` and route through the registry; a missing `domain` means `video`, so existing agents keep working. A new tool is added only when no existing intent fits, and the reason is written in SDD-003.
- **Discovery**: one tool call returns the domains, and for one domain the operations with their one-line summaries; the full schema of an operation is fetched on demand (progressive disclosure).
- **Budget**: NFR-020 is revised once in this epic: a budget for the tool list plus the first discovery call, and a budget per domain description; `internal/tools/surfacebudget` measures both with the repository's tokenizer approximation and fails over budget.
- **Generated documents**: `task codegen` writes, from the registry, the agent guide (one page per domain), the CLI reference and the MCP reference; `task generate:check` fails when they are stale.
- **CLI**: commands are grouped as `stuarty <domain> <verb>`; the former top-level video commands remain as aliases.
- **Parity**: the contract test iterates the registry, so an operation with no HTTP route, CLI command or MCP path fails the build, whatever its domain.

## Tasks

- [x] 1. Amend SDD-003 and SDD-000: domains, the routing rule, the revised budget and how it is measured. Verify: `task lint:md` exits 0 and the tests that read SDD-003 pass.
- [x] 2. Add `Domain` to the registry and classify every existing operation. Verify: `go test ./core/command/... ./core/schema/...` passes with the updated snapshot.
- [x] 3. Route the intent tools by domain, defaulting to `video`. Verify: `go test ./web/app/mcp/...` passes and `TestSurfaceFrozen` passes with additions only.
- [x] 4. Add the discovery call with progressive disclosure. Verify: `go test ./web/app/mcp/... -run TestDiscovery` passes.
- [x] 5. Group the CLI by domain with aliases. Verify: `go test ./web/app/cli/...` passes and `bash scripts/smoke-e2e.sh` still exports.
- [x] 6. Make the parity test registry-driven for every domain. Verify: `go test ./web/contract/...` passes, and removing one route in a scratch change makes it fail (described in the hand-off).
- [x] 7. Implement the budget check and wire it into `task lint`. Verify: `go run ./internal/tools/surfacebudget` exits 0 and prints the measured numbers.
- [ ] 8. Generate the agent documents per domain. Verify: `task generate:check` exits 0.

## Test strategy

- Unit: routing by domain; the default; unknown domain error.
- Contract: registry-driven parity; frozen surface.
- Budget: measured in CI-less mode by `task lint`.
- Scenario: the reference scenario through MCP with the tool list of this epic.

## Acceptance criteria

1. The MCP tool list has no more tools than before plus those justified in SDD-003, and it covers the `project` and `video` domains.
2. An agent script written for release 0.29.0 runs unchanged (`bash scripts/smoke-e2e.sh` exits 0; `TestSurfaceFrozen` passes).
3. `go run ./internal/tools/surfacebudget` exits 0 with the tool list and the discovery call inside the revised NFR-020.
4. An operation registered without one of its three surfaces fails `go test ./web/contract/...`.
5. `task generate:check` exits 0 and the generated guide has one page per domain.

## Risks and mitigation

- **A domain parameter confuses small models.** Mitigation: the default; the pilot of epic 38 measures it with a small model.
- **Later epics forget the domain.** Mitigation: the registry refuses an operation with no domain.
- **STOP** if the frozen surface cannot hold without a breaking change.

## Size

M

## Hand-off for review

The diff of `epic/13-mcp-domains-and-agent-docs`; the measured budgets; the outputs of the acceptance commands; how a later epic registers a domain operation (a short recipe added to SDD-003).
