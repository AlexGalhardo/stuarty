# ADR-0032: Outbound connections go only to connected destinations, and publishing waits for a person

- Status: Accepted (2026-11-18)
- Deciders: author
- Supersedes: ADR-0020 item 1 (no outbound capability)
- Amends: ADR-0020 item 2 (publishing is an irreversible command), PRD-002 RF-130, PRD-003 RF-206, SDD-007
- Links: author decisions D-052, D-016, D-011; ADR-0019, ADR-0033, ADR-0035, ADR-0036, ADR-0046

## Context

ADR-0020 broke the "lethal trifecta" (private data, untrusted content, external communication) by removing external communication: no publish, no webhook, no upload, no import by URL. That let agents act without confirmation. ADR-0020 itself says that publishing to social networks needs it revised.

Stuarty publishes to social networks and blogs and calls AI providers. Publishing cannot be undone, and generated video costs money (research note `web/03-integracao-ia`). Untrusted text is still everywhere: transcripts, file names, metadata, and now the output of providers.

## Decision

1. **Outbound is allowed only to connected destinations.** A destination is an address the user connected on purpose: a social account, an AI provider, a WordPress site, a webhook URL. No operation sends data to an address chosen by the caller of that operation. With no connection configured, the product still opens no outbound connection.
2. **Each connection is visible and revocable.** The Connections section lists every destination, what it may receive and when it was last used. Removing a connection removes its credentials (ADR-0033).
3. **Publishing and costly generation wait for a person by default.** An agent or a schedule can prepare a publication or a generation request; it becomes an item of the approval queue, with what will be sent, where, and the estimated cost. A person approves or rejects it.
4. **Automatic approval is an explicit rule.** The user can create a rule that approves a kind of action without asking (for example, publications of one project to one account, or generations under a cost ceiling per day). A rule is created only from a privileged browser session, never by an agent, and every use of it is audited.
5. **Costs are bounded.** Every provider call has an estimate before and a recorded cost after; ceilings per day and per project stop further calls with an actionable error.
6. **The rest of ADR-0020 stays.** Deletes go to a trash, everything inside the product is undoable per actor, untrusted text is marked and truncated, paths are confined, FFmpeg sees only files we chose, and no tool returns a secret. Text returned by a provider or read from a network is untrusted text.
7. **No telemetry** (author decision D-016): the product never sends data to its own authors.

## Alternatives

- **Autonomous agents bounded only by ceilings.** Rejected by the author: one injected instruction becomes a public post.
- **Approval always, with no rule.** Rejected: it forbids the unattended use the product is built for.
- **Detect injected instructions.** Unreliable, as ADR-0020 already recorded.

## Consequences

Positive: a successful prompt injection can prepare a publication but cannot publish it, unless the user wrote a rule that covers it.

Negative: the threat model of SDD-007 must be rewritten for outbound destinations, approval rules and provider output before the first connector ships.

Negative: an approval queue and its rules are a new surface to build and to test in every publishing and generation epic.
