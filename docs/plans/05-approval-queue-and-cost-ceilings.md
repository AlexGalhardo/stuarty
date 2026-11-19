# 05: Approval queue, rules and cost ceilings

## Goal

An agent can prepare a post or an expensive generation, and nothing is sent or spent until the creator says yes, or until a rule the creator wrote says yes for them. The creator always sees what will go out, where, and what it costs.

## Context and related decisions

- ADR-0032 items 3 to 5, ADR-0018, ADR-0046.
- Author decisions D-052, D-061. PRD-002 RF-132, RF-135, RF-136; PRD-004 RF-362, RF-363, RF-404, RF-408.

## Scope

- Modules: `core/approval/` (new, pure rules: what needs approval, rule matching, ceilings), `service/approval/` (new), `web/database/migrations/`, `web/app/` (controllers, CLI, MCP), the approval queue component in `web/resources/js/dashboard/approval/`, `docs/sdds/015-approval-and-cost.md` (new), SDD-007 rows.
- A generic "approvable action": any job kind can declare that it needs approval. Publication and generation use it in their own epics; a test job kind proves it here.

## Out of scope

- Any real provider or network.
- The calendar (epic 20).

## Dependencies

04.

## Technical design

- **Model**: `approval_requests` (id, actor, kind, summary text, destination connection, payload reference, estimated cost with currency, state `pending`, `approved`, `rejected`, `expired`, decided by, timestamps) and `approval_rules` (id, kind, connection, project, cost ceiling per item and per day, created by, enabled).
- **Rules live in `core/approval`**: `Decide(action, rules, spentToday) -> NeedsPerson | AutoApproved(ruleID) | Refused(reason)`. Pure and table-tested.
- **Service**: `ApprovalList`, `ApprovalGet`, `ApprovalDecide`, `ApprovalRuleList`, `ApprovalRuleSet`, `ApprovalRuleRemove`, `CostSummary`. `ApprovalDecide` and the rule operations require a **privileged browser session**, the same notion ADR-0020 item 2 uses for emptying the trash; the local token an agent holds is refused with a stable error.
- **Jobs**: a job whose kind needs approval is created in state `waiting_approval`; it starts only after `Decide` allows it. Cancelling it rejects the request.
- **Cost ledger**: every costly job records an estimate before and the real cost after, per connection, per project and per day; ceilings are configured in the workspace settings and enforced in `core/approval`.
- **Events**: `approval.requested` and `approval.decided` on the SSE stream, so the queue updates live.
- **Agents**: an agent submits through the existing job tools and receives "waiting for approval" with the request id; it can read the state, never decide.

## Tasks

- [x] 1. Write SDD-015 and the SDD-000 names; add SDD-007 rows (an agent approving its own request, a rule created by an agent, a cost ceiling bypass). Verify: `task lint:md` exits 0.
- [ ] 2. Implement `core/approval` with table tests. Verify: `go test ./core/approval/...` passes with every branch of `Decide` covered.
- [ ] 3. Add the tables and the service operations, with the privileged-session check. Verify: `go test ./service/approval/...` passes, including a test where a local-token caller is refused.
- [ ] 4. Wire jobs: the `waiting_approval` state, start on approval, reject on cancel. Verify: `go test ./service/... -run TestJobApproval` passes.
- [ ] 5. Implement the cost ledger and the ceilings. Verify: `go test ./service/approval/... -run TestCeiling` passes (per item, per day, per project).
- [ ] 6. Expose on HTTP, CLI and MCP (read and submit for agents; decide only for a privileged session) with parity tests. Verify: `go test ./web/contract/...` passes.
- [ ] 7. Build the approval queue component and the rules screen in both languages. Verify: `task test:web` and `web/e2e/specs/approval.spec.ts` pass.
- [ ] 8. Add the security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.

## Test strategy

- Unit: the decision table; ceilings at their boundaries; rule precedence.
- Integration: a test job kind that needs approval goes through pending, approved and rejected; an auto-approval rule lets it start and is audited.
- Security: an agent cannot decide or write a rule on any surface.
- End to end: approve and reject from the UI.

## Acceptance criteria

1. A job of an approvable kind submitted by an agent stays in `waiting_approval` and makes no outbound call until a person approves it.
2. `ApprovalDecide` and every rule operation called with the local token fail with a stable error on HTTP, CLI and MCP.
3. A rule created from a privileged session auto-approves a matching action, and the audit log names the rule.
4. An action whose estimate exceeds a ceiling is refused with an actionable error and nothing is spent.
5. The queue in the UI shows the summary, the destination, the cost and the actor, and updates without reloading.
6. `go test ./core/approval/... ./service/approval/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass.

## Risks and mitigation

- **A surface that lets an agent approve.** Mitigation: the check is in the service, not in the adapters, and a contract test calls every surface with an agent credential.
- **Estimates that are wrong.** Mitigation: the real cost is always recorded and the daily ceiling counts real costs.
- **STOP** if "privileged browser session" cannot be told apart from an agent on some surface.

## Size

M

## Hand-off for review

The diff of `epic/05-approval-queue-and-cost-ceilings`; the decision table; the outputs of the acceptance commands; how to see an agent's request wait and be approved.
