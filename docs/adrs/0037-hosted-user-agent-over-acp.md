# ADR-0037: The chat hosts the user's own agent; Stuarty has no agent of its own

- Status: Accepted (2026-11-18)
- Deciders: author
- Links: author decisions D-060, D-041 (replaced); ADR-0015, ADR-0032, ADR-0036, ADR-0043; PRD-004

## Context

The dashboard has an agent chat. Author decision D-041 kept all intelligence outside the product: agents ran in a terminal and handed finished files to the editor. The author now wants the conversation inside the product, without turning Stuarty into the maintainer of an agent.

The Agent Client Protocol (ACP) lets an application host a third-party agent as a child process inside its own interface; many coding agents support it and a registry of them exists (research note `web/03-integracao-ia`). Anthropic's terms allow running the unmodified Claude Code when each user authenticates themselves.

## Decision

1. **The chat panel hosts the agent the user already has** (Claude Code, Codex, OpenCode and others), started as a child process and spoken to over ACP. The agent authenticates as it always does, with the user's own account; Stuarty never sees those credentials.
2. **The agent works through the public surfaces.** It reaches the product through the MCP server (ADR-0015, ADR-0043), exactly like an agent in a terminal. There is no private channel for the hosted agent.
3. **Stuarty has no agent loop of its own.** It calls a provider directly only for a single bounded task that a person starts from the UI: generate an image, a voice, a text (ADR-0036).
4. **With no agent installed, the panel says so** and shows how to install or connect one. Everything else in the product keeps working.
5. **The agent's binary is never modified, bundled or renamed**, and vendors' names appear only as plain text.
6. **The hosted process runs under the process policy** of SDD-007: an explicit command, a working directory inside the workspace, and the same approval rules as any other actor (ADR-0032).

## Alternatives

- **An agent loop built into Stuarty**, calling model APIs with its own tools. Rejected: the project would maintain an agent harness, and usage would be paid by API key only.
- **No chat in the product.** Rejected: it contradicts the dashboard the author asked for.

## Consequences

Positive: the agent stays the user's, as D-041 wanted, and the conversation happens where the project is open.

Negative: the feature depends on a young protocol; the epic that builds it starts with a spike against two agents and records which ones work.

Negative: a hosted agent has the user's file access, so the limits come from the agent's own permission model plus ADR-0032, not from a sandbox of ours.
