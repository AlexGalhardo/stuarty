# ADR-0031: Stuarty is one product with several domains over one service

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0001 (the product name), ADR-0002 and ADR-0008 (their scope grows from one editing domain to several)
- Links: author decisions D-047, D-048, D-049; PRD-004; ADR-0002, ADR-0007, ADR-0008, ADR-0043

## Context

The product was a video editor, Vukem Video, built on one rule: every editing rule lives in a pure command core, and the UI, the MCP server, the CLI and the HTTP API are thin adapters over `service.Service`. The author now wants a different product on the same foundation: Stuarty, where a creator plans, creates, edits, manages and publishes content in one place, self-hosted, with AI agents as first-class users.

The market research found that agent-driven video editing is already crowded with large open source projects, and that nobody offers the whole cycle (project, assets, script, edit, thumbnail, publication, memory) as one local product (research note `web/04-ferramentas-similares`). The most documented pain of creators is the number of tools they switch between (research note `web/05-rotina-criadores`).

## Decision

1. **The product is Stuarty.** It covers these domains: video editing, image editing, assets, writing (scripts, copy, blog posts), publishing to social networks and blogs, AI providers, and memory, skills and prompts.
2. **One service, one model for every domain.** Each domain that changes user data follows the model the video editor proved: a catalogue of operations with a JSON Schema, batches applied atomically as one revision, undo per actor, a lint, a text view with a token budget, and the same result on the UI, the MCP server, the CLI and the HTTP API. A domain whose rules do not fit a document (a publication, a provider call) still goes through `service.Service` and is still reachable from the four surfaces.
3. **Pure rules stay pure.** The rules of a domain live in `core/` (Go, no I/O); anything that touches the network, the disk or a process lives in `engine/` or `service/`. The UI computes no result (ADR-0007).
4. **Work is a queue of epics, with no phases.** There is no Prototype, ALPHA, BETA, MVP or 1.0 milestone. The first complete flow to deliver is a short vertical video with captions and a thumbnail, from the briefing to the publication (ADR-0045 fixes the process).

## Alternatives

- **Keep the editor as the product and build the rest as an agent-side kit.** That was the direction of author decision D-041. Rejected by the author: the creator would still assemble the cycle by hand.
- **A separate application per domain.** Rejected: the value is the shared state between the steps of a production.

## Consequences

Positive: a new domain reuses jobs, events, the media store, templates, the brand kit, the agent documentation generator and the parity tests.

Negative: `service.Service` grows; ADR-0043 keeps the agent surface small. The specifications SDD-000 to SDD-013 describe only the video domain; each new domain gets its own specification from the epic that builds it.

Negative: the former name is still in the Go module path, the binary, the environment variables and the workspace folder until the epic of ADR-0044.
