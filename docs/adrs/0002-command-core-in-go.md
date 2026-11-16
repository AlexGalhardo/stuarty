# ADR-0002: A single command core in Go

- Status: Accepted (2026-11-14)
- Deciders: author (D-001), architect
- Links: author decision D-001, post-mortems §12, agents and MCP §2, engine §4, SDD-002

## Context

The product's differential is that agents operate the editor through MCP, CLI and API, and a human UI is a second door to the same logic. The research shows what happens otherwise: OpenCut's editor had non-serializable command classes tied to the UI and was archived for a rewrite; FreeCut exposes agents a second dispatcher that duplicates the UI's operations; Omniclip's MCP needs a browser tab open. FilmCraft, which started from one command registry dispatched by UI, CLI and MCP, is the counter-example.

## Decision

All editing logic lives in `core/command`, written in Go.

1. A **command** is a named, versioned, serializable operation with a JSON Schema for its arguments, declared preconditions, a pure `apply` function and an inverse computed at apply time.
2. A **registry** lists every command with its schema, description, example, error codes, scope and phase. Every surface is generated from this registry: MCP tool schemas, CLI subcommands and help, the REST endpoint's validation, the OpenAPI document, the agent skill file and the UI's command palette.
3. `apply(document, operation) -> (document', inverse, diff)` is deterministic: same inputs, same outputs, no clock, no randomness, no I/O. Identifiers and timestamps are inputs supplied by the caller.
4. Nothing outside `core/command` changes a timeline document. The browser never computes a resulting timeline (ADR-0007).

## Alternatives

- **Core and renderer in TypeScript, export through headless Chrome.** Rejected by the author (D-001): CLI and server would need Chromium and the Go side would be a shell.
- **Core in Rust, native and WebAssembly.** Rejected by the author (D-001): a third language for no stated requirement.
- **Commands as Go methods without a registry.** Rejected: each surface would then describe the commands by hand, which is how descriptions drift.

## Consequences

Positive: one implementation to test; golden tests of the form "document + operation = document" cover every surface at once; a new command appears in MCP, CLI, API and the UI palette without surface code.

Positive: determinism makes the operation log replayable and auditable (ADR-0006).

Negative: every user-facing edit, including drag interactions in the UI, costs a round trip to the Go process. Acceptable because the process is local or self-hosted (D-003); see ADR-0007.

Negative: the registry format is a contract frozen early. Changing a command's arguments after release is a breaking change with a version bump of that command.
