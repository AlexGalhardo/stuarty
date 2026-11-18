# ADR-0043: The MCP server keeps a few tools named by intent, with a domain

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0015 (sixteen tools for one domain), PRD-002 NFR-020 (the token budget of the tool definitions)
- Links: author decision D-069; ADR-0014, ADR-0015, ADR-0016, ADR-0017, ADR-0031; SDD-003

## Context

The MCP server has sixteen tools named by intent (`edit`, `view`, `lint`, `history`, `command_help`, the job and media tools and others), stateless, within a budget of 6,500 tokens for all their definitions. Stuarty adds image editing, publishing, writing, assets, memory and providers. One tool per operation would mean hundreds of tools and would fill an agent's context before it did anything. The current MCP specification is stateless, which matches ADR-0015 (research note `web/03-integracao-ia`).

## Decision

1. **The tools stay few and named by intent.** A tool that applies to several domains takes a `domain` argument (video, image, publishing, writing, assets, memory, providers); the same `edit`, `view`, `lint` and `history` serve a timeline and an image document.
2. **The catalogue is discovered at run time.** `command_help` lists the operations of a domain with their arguments, an example and their error codes; nothing about an operation is repeated in a tool definition.
3. **A tool is added only for an intent that no existing tool expresses** (for example, to submit an item to the approval queue). Each addition states its cost in tokens.
4. **The token budget is revised once**, by the epic that adds the first new domain, and written into PRD-002; it stays a hard limit checked by a test.
5. **The CLI and the HTTP API follow the same catalogue** (ADR-0016, ADR-0017), and parity between the surfaces stays a tested property.

## Alternatives

- **One MCP server per domain.** Each small, but the agent's configuration grows and a production that crosses domains needs several.
- **One tool per operation.** Rejected: the context of every agent would be spent on definitions.

## Consequences

Positive: a small model can still hold the whole tool list.

Negative: one more step of discovery for the agent; the generated skill of each domain must teach it.
