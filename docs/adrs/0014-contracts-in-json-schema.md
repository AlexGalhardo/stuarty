# ADR-0014: Contracts are JSON Schema, generated from the Go types

- Status: Accepted (2026-11-14)
- Deciders: architect
- Links: [stack trade-offs §2](../search/stack-tradeoffs.md), [agents and MCP §4](../search/agents-mcp.md), ADR-0002, SDD-001, SDD-002, SDD-003

## Context

The stack draft asked for a shared schema, "JSON Schema or Protobuf", and listed gRPC and buf for the desktop. Three consumers need machine-readable contracts: MCP tools (`inputSchema` and `outputSchema` are JSON Schema 2020-12), the REST API (OpenAPI 3.1 embeds JSON Schema) and the UI's TypeScript types. FilmCraft documents its commands with an example string instead of a schema, which the research notes as a weakness.

## Decision

1. The contracts are: the timeline document schema, one argument schema per command, the result and error envelopes, and the event envelope. They are JSON Schema 2020-12.
2. The Go types in `core/` are the single source. Schemas are generated from them by a small generator in the repository and committed under `core/schema/`. A CI check regenerates them and fails on any difference.
3. Arguments of a command are validated against its schema before `apply` runs, with `santhosh-tekuri/jsonschema`. Validation errors are translated into the actionable error format (SDD-002).
4. TypeScript types for the UI are generated from the same schemas and committed.
5. A contract is **frozen** when its schema file is merged with the marker `"x-status": "frozen"`. Changing a frozen schema needs a revision of the SDD that owns it. Adding an optional field is compatible; anything else bumps the document `schema_version` or the command's version.
6. No Protobuf, no gRPC, no buf.

## Alternatives

- **Protobuf with buf.** Compact and strict, but none of the three consumers reads it; it would add a toolchain and a translation layer to JSON Schema anyway.
- **Hand-written schemas as the source, Go types generated.** Schemas first is attractive for a contract-driven project, but generated Go types are awkward for a domain model with invariants.
- **OpenAPI as the only source.** It does not describe the timeline document or MCP tools well.

## Consequences

Positive: one definition feeds validation, MCP, OpenAPI, CLI help and the UI types; an agent can ask for the schema of any command.

Negative: a generator to write and maintain; struct tags must carry descriptions, since tool descriptions are prompts.

Negative: JSON Schema cannot express every invariant (for example, non-overlap on a track). Those live in `apply` and are documented per command.
