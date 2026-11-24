# 82: Publishing and approvals screens

## Goal

A creator sees what goes out and when, and decides each request knowing what it does, where it goes and what it costs. A blog post without media is a normal path.

## Context and related decisions

- Author decisions D-134 and D-137, ADR-0032, SDD-015, SDD-020. Frames 13, 14 (month, week, list), 33f to 33h, and the state and responsive frames of the final redesign file held by the author.
- Code: `web/resources/js/pages/Publish/`, `web/resources/js/pages/Approvals/`, `web/resources/js/dashboard/approval/`.

## User stories

- As a creator, I want a calendar in month, week and list so that I can plan.
- As a creator, I want a text post to publish with no media so that a blog is simple.
- As a creator, I want to see why a destination cannot take my content.

## Scope

- Modules: the Publish and Approvals pages, `web/resources/js/dashboard/approval/`, their language keys.
- Must not touch: `core/`, `service/`, the shell, the connectors.

## Out of scope

New destinations, publishing a video to a blog, and approval rule changes.

## Dependencies

74, 75, 77 and 78.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Publish, Approvals, publication editor |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new. `ApprovalDecide` and the ceilings keep their privileged checks.

## Technical design

- `ApprovalItem` uses the rows of epic 77: what happens, item, destination, cost, undo, Approve and Reject at the same size.
- Text items show excerpt, words and address, with no media preview.
- A destination that cannot take the content is shown disabled, with its reason from the server's connector capabilities. If the server does not expose that, the UI does not compute it, and the epic stops.
- Cost and limits show the server's numbers, including the member-scoped view.

## Tasks

- [ ] 1. `ApprovalItem`, cost and rules panels. Verify: `cd web && bun run test -- ApprovalQueue RulesPanel` passes.
- [ ] 2. The Approvals page and states. Verify: `cd web && bun run e2e -- approval` passes.
- [ ] 3. Calendar in month, week and list. Verify: `cd web && bun run e2e -- publish-calendar` passes.
- [ ] 4. The publication editor with destinations and reasons. Verify: `cd web && bun run test -- Publish` passes.
- [ ] 5. The text path: destination, approval, published. Verify: new spec `publish-text-post` passes with the local blog destination.
- [ ] 6. Empty, loading, error and agent states. Verify: `bun run check:i18n` exits 0.
- [ ] 7. Check 1440, 1280 and 390 px in both themes. Verify: `cd web && bun run e2e -- a11y` passes.

## Test strategy

The existing approval and calendar specs, a new end-to-end spec of a text post with a fake destination.

## Acceptance criteria

1. **Given** a text post, **when** scheduled to a blog and approved by a person, **then** it is sent with no media (`publish-text-post`).
2. **Given** a destination that needs video and a text item, **when** the choice opens, **then** it is disabled and states the reason.
3. **Given** a request, **when** shown, **then** what happens, item, destination, cost and undo are visible, or read "Not reported".
4. **Given** an agent session, **when** it tries to approve, **then** it is refused as before (`go test -run TestSecurity_ ./...`).
5. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- A misleading approval card is a safety problem: its fields come from the server only.
- **STOP** if destination capability is not available from the server, or if a change would touch a decision path.

## Size

L: two connected areas with a safety role. About 7 commits.

## Priority

Must. Human approval is a rule of the product.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of the text path.
