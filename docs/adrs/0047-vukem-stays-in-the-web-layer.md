# ADR-0047: The vukem framework stays, confined to the web layer and the process runner

- Status: Accepted (2026-11-18)
- Deciders: author
- Confirms: ADR-0016, ADR-0018, ADR-0025 where they rely on vukem
- Links: author decision D-054; ADR-0001, ADR-0002, ADR-0008, ADR-0044

## Context

The project started as a way to evolve the author's Go framework, vukem. Stuarty is now an independent product, and the question was whether the framework stays. The coupling is narrow: `core/` and `service/` do not import vukem; it is used by `web/` (routing, sessions, authentication, the queue, migrations, the command runner) and by the process runner of `engine/` (`contrib/engine`). Replacing it would rewrite the server and its tests for no new value to a user.

## Decision

1. **vukem stays** as the framework of `web/` and as the process runner of `engine/`.
2. **The boundary is kept.** `core/` never imports vukem (ADR-0002). `service/` imports it only where it already does, and a new domain is written against `service.Service`, not against the framework.
3. **Every friction with the framework, and every idea to improve it, is recorded when found** in the maintainers' private friction log, checked against the vukem source, so that the framework evolves from real use.
4. **The framework keeps its own name and repository**; the rename of ADR-0044 does not touch it.

## Alternatives

- **Remove vukem** and use the standard library with small libraries. Rejected: months of rewrite of the server, authentication, queue, migrations and tests.
- **Decide again later.** Rejected by the author in favour of a clear answer now.

## Consequences

Positive: no rewrite, and the framework improves from a demanding application.

Negative: a need the framework does not cover may require a change in two repositories.
