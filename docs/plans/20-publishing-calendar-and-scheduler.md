# 20: Publishing: calendar, approval and scheduler

## Goal

A creator plans the week in one calendar: which piece goes to which network and when. Each post waits for their yes, goes out at its time, and shows afterwards where it landed, or exactly why it did not.

## Context and related decisions

- ADR-0046 (the publishing scheduler), ADR-0032, ADR-0039 (a publication is an item of the project).
- Author decisions D-052, D-078. PRD-004 RF-402, RF-403, RF-404, RF-405, RF-406, RF-408.
- Research note `web/02-redes-sociais`.

## Scope

- Modules: `core/publish/` (the publication, its targets, the schedule rules, what happens to a missed time), `service/publish/` (new: the scheduler and the publication operations), `web/app/` (controllers, CLI, MCP), `web/resources/js/pages/Publish/` (calendar and list), `web/database/migrations/`, SDD-020 (the publication part), SDD-005, SDD-000.
- The scheduler runs inside the server process. Keeping it alive with the window closed is epic 21.

## Out of scope

- New networks (19, 22, 44) and blog destinations (23).
- Metrics (45). Recurring posts and "best time" suggestions.

## Dependencies

06, 07, 18.

## Technical design

- **Model**: a `publication` is a project item with a title, the output or image it publishes, the text (a caption of epic 17 when merged, otherwise its own field), and one **target** per connected account. Each target has its own per-network fields, its scheduled time with a time zone, its state and its result (remote id, URL, error).
- **Rules in `core/publish`**: target states (`draft`, `waiting_approval`, `approved`, `scheduled`, `publishing`, `published`, `failed`, `missed`, `cancelled`); `Due(targets, now)`; **missed time** (RF-406): a target whose time passed while the product was not running becomes `missed` and is never sent late on its own: the person chooses to send now or reschedule, unless they turned on "send late up to N minutes"; time zone and daylight-saving handling with tests.
- **Approval** (RF-404): scheduling a target creates an approval request (epic 05) that shows the final text, the file, the account and the time. An edit after approval returns the target to `waiting_approval`.
- **Scheduler** in `service/publish`: one loop that wakes at the next due time, claims a target with a database lease (no double publish across restarts or two processes), calls the connector, records the result, and retries only errors the connector marks as transient, with backoff and a ceiling.
- **Cost** (RF-408): a target on a network that charges per post shows its cost in the approval and counts against the ceilings.
- **Service**: `PublicationCreate`, `PublicationGet`, `PublicationList` (by range, state, project), `PublicationUpdate`, `PublicationSchedule`, `PublicationCancel`, `PublicationRetry`, `PublicationSendNow`; `publish` domain. An agent can create and schedule; approval stays with a person or a rule.
- **UI**: month and week calendar with drag to reschedule, a list view with filters, the editor of a publication with a preview per network, the state and the result link of each target.
- **Events**: `publication.updated` on SSE.

## Tasks

- [x] 1. Write the publication part of SDD-020, amend SDD-005 and SDD-000. Verify: `task lint:md` exits 0.
- [x] 2. Implement the states, `Due`, the missed-time rule and the time-zone rules with table tests. Verify: `go test ./core/publish/...` passes.
- [x] 3. Add the tables and the service operations. Verify: `go test ./service/publish/... -run TestPublication` passes.
- [x] 4. Implement the scheduler with leases and an injectable clock. Verify: `go test -race ./service/publish/... -run TestScheduler` passes (two schedulers, one publish; restart mid-publish; missed time).
- [x] 5. Wire approval and the cost of a target. Verify: `go test ./service/publish/... -run "TestApproval|TestCost"` passes.
- [x] 6. Expose on HTTP, CLI and MCP with parity. Verify: `go test ./web/contract/...` passes.
- [x] 7. Build the calendar, the list and the publication editor in both languages, keyboard operable. Verify: `task test:web` and `web/e2e/specs/publish-calendar.spec.ts` pass with the fake connector.
- [x] 8. Add the publication to the project overview and to `ProjectDescribe`. Verify: `go test ./service/... -run TestProjectDescribe` passes.

## Test strategy

- Unit: every state transition; time zones and daylight saving; missed time with and without the late window.
- Concurrency: leases under the race detector with two schedulers.
- Integration: schedule, approve, publish with the fake connector; edit after approval; transient and permanent failures.
- End to end: plan a post on the calendar, approve it, see it published.

## Acceptance criteria

1. A publication with two targets scheduled at different times publishes each at its time, once, after approval.
2. Two server processes on the same workspace publish a target exactly once (test).
3. A target whose time passed while the product was stopped becomes `missed` and is not sent until a person decides, unless the late window is on.
4. Editing an approved target requires approval again.
5. Each target shows its remote URL when published, or an actionable error when failed, and can be retried.
6. The calendar is operable by keyboard and available in both languages.
7. `go test -race ./core/publish/... ./service/publish/...`, `go test ./web/contract/...` and `task test:web` pass.

## Risks and mitigation

- **A double post.** Mitigation: leases, idempotency keys where a network supports them, and the concurrency tests.
- **A late post nobody wanted.** Mitigation: `missed` by default.
- **Clock and time-zone mistakes.** Mitigation: store instants in UTC with the zone name; table tests around the changes of hour.

## Size

L

## Hand-off for review

The diff of `epic/20-publishing-calendar-and-scheduler`; the state table; the outputs of the acceptance commands; a walk-through of planning, approving and publishing with the fake connector.
