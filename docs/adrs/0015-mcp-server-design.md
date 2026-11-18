# ADR-0015: MCP server: sixteen tools by intent, stateless, text first

- Status: Accepted (2026-11-14); extended to several domains by [ADR-0043](0043-mcp-tools-by-intent-with-a-domain.md) (2026-11-18)
- Deciders: architect; author (D-011)
- Links: agents and MCP, ADR-0002, ADR-0008, ADR-0014, ADR-0020, SDD-003

## Context

Tool selection degrades beyond 30 to 50 tools (Anthropic) and OpenAI advises fewer than 20 at the start of a turn. The DaVinci Resolve MCP recommends its 37 compound tools over 390 granular ones; the Premiere MCP announces 5 of 283; FilmCraft exposes 16 meta-tools over 650 commands. MCP revision `2026-07-28` is stateless. Client support is uneven: tools are universal; resources, prompts, elicitation and tasks vary. In Claude Code a tool result is capped at 25,000 tokens. vukem has no MCP server for applications (VF-003).

## Decision

1. The server is built with the official Go SDK (`modelcontextprotocol/go-sdk`), targeting revision `2026-07-28` and negotiating older revisions when the SDK allows.
2. Two transports: **stdio** (`vukem-video mcp`) and **Streamable HTTP** at `/mcp` on the server. Both are adapters over `Service` (ADR-0008).
3. The server keeps no per-connection state. Every tool takes `project_id` explicitly.
4. A fixed catalogue of sixteen tools named by intent: `project_list`, `project_create`, `project_get`, `project_trash`, `edit`, `command_help`, `history`, `media_import`, `media_inspect`, `view`, `lint`, `job_start`, `job_get`, `job_cancel`, `template`, `font_import`. The list is fixed in SDD-000 §1 and the schemas are in SDD-003. Sixteen stays under the guidance of fewer than 20 tools.
5. All timeline changes go through `edit`, which takes `base_revision`, an array of typed operations from the command catalogue, an optional idempotency key and `dry_run`. Adding a command never adds a tool.
6. Every tool returns text plus `structuredContent` with an `outputSchema`. Read tools accept `detail: "concise" | "full"`, concise by default. No result exceeds a fixed token budget; longer content is paged with a cursor.
7. Every write returns the new revision, a text diff of what changed and any new lint finding.
8. `view` returns images produced by the export executor, at a reduced default resolution, single frame or contact sheet with the time printed in each cell.
9. Long work returns a job identifier; `job_get` accepts `wait_ms`. Where the client supports the MCP tasks extension it is used in addition, never instead.
10. Nothing depends on sampling, roots, prompts, resources or elicitation.
11. Tool descriptions, the server `instructions` text (first 512 characters self-sufficient) and the skill file are generated from the command registry.
12. There is no tool that sends data out of the system and no tool that deletes irreversibly (ADR-0020).

## Alternatives

- **One tool per command.** Simple mapping, but the catalogue is expected to pass 40 commands, beyond the guidance, and grows with every feature.
- **Pure meta-tools (`command_list`, `command_run`).** Constant size, but the model learns nothing from the tool list and every edit needs a discovery step.
- **MCP only, no CLI.** Chat clients are served, coding agents pay more tokens per task.

## Consequences

Positive: a constant, small tool list; the lowest common denominator of clients can do everything.

Negative: `edit` has a large input schema (a union over operations). SDD-003 keeps it small by listing operation names in the schema and serving each operation's full schema through `command_help`.

Negative: the SDK's support for the tasks extension and for parts of the `2026-07-28` revision is unverified; a Prototype task checks it before the MCP contract is frozen.
