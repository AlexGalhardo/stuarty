# 45: Metrics of published posts

## Goal

A creator sees, next to each post in Stuarty, how it did: views, likes, comments. Their agent can read the same numbers and learn which hooks worked.

## Context and related decisions

- ADR-0035, ADR-0032 (outbound only to connected destinations), ADR-0040 (learnings in memory).
- Author decision D-079 (metrics come after publishing is solid). PRD-004 section on what comes later; the roadmap lists this epic as waiting.
- Research note `web/02-redes-sociais` (which metrics each official API exposes, and with which permissions).

## Scope

- Modules: `service/social/` (an optional `Metrics` capability on the connector interface), the adapters of YouTube, Instagram and Facebook, `core/publish/` (the metric model), `service/publish/` (the collection schedule), `web/app/` (controllers, CLI, MCP), the publication page and the calendar, SDD-020.
- Per published target: views or plays, likes, comments count, shares where the API offers it, with the time of collection.

## Out of scope

- Reading or answering comments, audience demographics, revenue.
- Dashboards across accounts, comparisons with other creators, "best time to post".
- X and TikTok until their access tiers allow reading metrics for the user's own posts without further cost or audit; recorded per network in task 1.

## Dependencies

19 and 20, **and the author's go** (author decision D-079).

## Technical design

- **Capability**: `Metrics(ctx, remote id) -> snapshot` per connector, optional; a connector without it shows "not available for this network".
- **Model**: `metric_snapshots` (target id, collected at, a small set of named counters). Snapshots are kept, so the UI can show growth; the counters are a closed list in `core/publish`.
- **Collection**: the scheduler of epic 20 collects with a decaying cadence (often in the first day, then daily, then weekly, then stops after a set age), within each API's quota, and only for connected accounts. A manual "refresh" is rate limited. Extra permissions are requested only when the user turns metrics on; without them nothing changes.
- **Agents**: `PublicationMetrics` returns the snapshots as text within the token budget; the `production-retro` skill gains a step that reads them before writing learnings.
- **UI**: counters on the publication page and on calendar entries, with the collection time; no charts library (a simple inline sparkline in SVG at most).

## Tasks

- [ ] 1. Read each network's official pages on metrics; record endpoints, permissions, quota and date in SDD-020; decide per network in or out. Verify: `task lint:md` exits 0.
- [ ] 2. Implement the model and the closed counter list. Verify: `go test ./core/publish/... -run TestMetrics` passes.
- [ ] 3. Implement the capability on the three adapters with contract fixtures. Verify: `go test ./service/social/...` passes with no network.
- [ ] 4. Implement collection with the decaying cadence and quota guard. Verify: `go test ./service/publish/... -run TestMetricsCollection` passes with an injected clock.
- [ ] 5. Expose on the three surfaces; update the retro skill. Verify: `go test ./web/contract/... ./web/resources/skills/...` passes.
- [ ] 6. Show the counters in the UI, both languages. Verify: `task test:web` and `web/e2e/specs/post-metrics.spec.ts` pass.

## Test strategy

- Unit: cadence and quota.
- Contract: each adapter's metrics endpoint, with a permission-missing error.
- End to end with a fake connector that returns growing numbers.

## Acceptance criteria

1. A published target on YouTube, Instagram or Facebook shows its counters with the time they were collected.
2. With metrics off, no extra permission is requested and no metrics call is made.
3. Collection stays within the configured quota and stops after the set age.
4. `PublicationMetrics` returns the snapshots on HTTP, CLI and MCP.
5. `go test ./core/publish/... ./service/social/... ./service/publish/...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Extra permissions trigger a review at a network.** Mitigation: task 1 checks; a network whose metrics need a review the user cannot pass alone is left out. **STOP** in that case.

## Size

M

## Hand-off for review

The diff of `epic/45-post-metrics`; the per-network decision table; fixtures; the outputs of the acceptance commands.
