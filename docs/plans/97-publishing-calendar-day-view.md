# 97: Publishing calendar: a day of its own, notes, quiet days and recurring slots

## Goal

A creator opens one day of the publishing calendar and plans it there: creates a post at an hour, adjusts the posts of that day, writes notes and a theme for the day, and marks a day with no posts. Recurring slots, duplicating a post to another day, a mini calendar and an `.ics` export make planning a week fast. Nothing is published without a person's approval, and an agent works inside the same rules.

## Context and related decisions

- Today (SDD-020 §30) the page `/publish` has a month view, a week view and a list. A target can be dragged, or moved with the keyboard, to another day or hour. There is no view of one day, nowhere to note a plan, and no way to say "no posts that day".
- Author decisions D-146 (2026-11-24, from an interview with the author):
  1. The day view does three things together. It creates a publication at an hour (a click on an empty hour opens the editor with the date and time filled in). It adjusts the publications of the day: time, text and target, in place or in the editor. And it plans the day with notes and a theme with a colour.
  2. A day marked "no posts" (a quiet day) is a rule, not a hint. Scheduling on that day by a caller that is not a person is refused. A person is asked to confirm before scheduling on it, on every surface.
  3. Recurring slots per network ("every Tuesday 18:00 on Instagram") appear as empty slots to fill. Duplicating a publication to another day or hour makes a new publication that waits for approval. There is a mini month with "today" and "go to date", and the calendar is exported as `.ics`, read only.
  4. An agent reads everything and creates and edits notes, themes and recurring slots. Only a person creates or removes a quiet day, and an agent respects it when it schedules.
- AGENTS.md rules 1 to 3 and 6. The rules of notes, quiet days and slots live in `core/`. The UI draws what the service answers. Names come from SDD-000. Publishing waits for a person (ADR-0032). D-145: a caller that is not a person does not reschedule what a person approved. D-134: a publication can be text only.
- The MCP `tools/list` budget (NFR-020) has 13 bytes of margin after D-143. New operations extend the existing `publication` tool's actions. Any raise is the measured size, recorded in SDD-003 §4.1 as D-143 did, never more.

## User stories

- As a creator, I want to open one day and create a post at an hour by clicking it.
- As a creator, I want to change the time, text or target of a post of that day without leaving the day.
- As a creator, I want a note and a coloured theme on a day ("long video day") and to mark a day with no posts.
- As a creator, I want fixed weekly slots per network, so the empty slot reminds me what to fill.
- As a creator, I want to copy a post to another day, jump to a date, and see my plan in another calendar app.
- As an agent, I want to read the plan, write notes and slots, and know when a day is closed to posts.

## Scope

- Modules: `core/publish` (or the calendar part of the publishing domain): day notes, themes, quiet days, recurring slots, duplication, the `.ics` writer. `service/publish`, the HTTP routes, the CLI and the MCP `publication` tool actions. A migration for the new tables. `web/resources/js/pages/Publish/`, i18n. SDD-020 (a new section after §30), SDD-000, SDD-003, SDD-005, SDD-007 (threats), the user guide `publishing-calendar.mdx`.
- Must not touch: the connectors and the sending of a publication, `renderer/`, `capture/`, the recording UI.

## Out of scope

Guests and attendees, reminders by email or any outbound notification, two-way sync with Google or Outlook, importing `.ics`, events that are not publications, and "best time to post" suggestions.

## Dependencies

20 and 82 (the calendar and its redesign), and VV-282 (the `publication` MCP tool) integrated with D-145.

## Surfaces

| Surface | Change |
| --- | --- |
| App (web and desktop) | The day view, notes and themes, quiet days, recurring slots, duplicate, the mini month and go to date, `.ics` download |
| HTTP | Routes for day notes, quiet days, slots, duplicate and the `.ics` export |
| CLI | The same operations under the publication commands |
| MCP | New actions on the `publication` tool for notes, slots, duplicate and reading the plan of a day. No action creates or removes a quiet day |

## Core commands and permissions

- Day note and theme (per day, per workspace): create, edit, delete. Scope `edit` for any actor.
- Quiet day: create and remove by a person only (`E_PRIVILEGED_SESSION` or the matching code for anyone else). Reading it is open to every scope that reads publications. Scheduling on a quiet day: refused for a caller that is not a person. A person must confirm, and the operation takes an explicit flag, so the UI and the CLI ask first.
- Recurring slot (network, weekday, local time, zone, optional connection): create, edit, delete with scope `edit`. A slot is a placeholder and never sends anything.
- Duplicate: creates a new publication in draft with the same fields and a new time, with scope `edit`. Scheduling it follows the normal approval.
- `.ics` export: read only, scope `read`. Titles and texts pass through the untrusted-text rules. No credential, URL token or internal id that grants access is in the file.

## Technical design

- A day is a local date in the zone of the person who planned it. Notes, themes and quiet days store the date and the zone. Recurring slots store the weekday, the local time and the zone, and expand to instants for a range on read.
- Read model of a day: its publications, its note and theme, whether it is quiet, and its slots with the publication that fills each one, if any. Range-limited and paginated like the list.
- Quiet-day check inside `Schedule` (and send-now, if it applies), in the service, on every surface.
- The UI: the day view opens from a day cell of the month or week view. Hours form a column, empty hours are buttons, existing targets are draggable, and the keyboard parity of §30 applies. The mini month and "go to date" sit in the page header. Theme colours come from the design tokens with checked contrast.
- `.ics`: one `VEVENT` per scheduled target with its instant in UTC, folded lines, escaped text and a stable UID that reveals nothing.

## Tasks

- [x] 1. Specify: a new section of SDD-020, the names in SDD-000, the routes and tool actions in SDD-003 with the measured budget, the tables in SDD-005, and the threats in SDD-007 (a quiet day bypassed by an agent, a slot that sends, a leak in `.ics`). Verify: `go run ./internal/tools/threatcheck`.
- [x] 2. Core and store: notes and themes, quiet days, slots and their expansion, duplicate, the `.ics` writer, the migration. Verify: `go test ./core/... ./web/database/...`.
- [x] 3. Service and adapters: the operations on HTTP, CLI and MCP with parity, and the quiet-day rule in scheduling. Verify: `go test ./service/... ./web/app/...`, the parity tests, `go test -run TestSecurity_ ./...` and `go run ./internal/tools/surfacebudget`.
- [x] 4. The day view: create at an hour, adjust in place, notes and theme, quiet day with confirmation. Verify: `task test:web`, `bun run check:thin-client`, `bun run check:i18n` and an e2e of the day.
- [x] 5. Recurring slots, duplicate, the mini month and go to date, the `.ics` download. Verify: unit tests and e2e on Chromium, and the `.ics` checked by a parser test.
- [x] 6. The user guide and the generated references. Verify: `task codegen` and, in `site/`, `bun run reference` and `bun run last-updated` with no diff.

## Test strategy

Core unit tests (slot expansion across DST changes, quiet-day rule, `.ics` escaping), service tests per actor, parity across the four surfaces, security tests for each new threat, UI unit tests and e2e of the day view with the keyboard.

## Acceptance criteria

1. **Given** a day of the month or week view, **when** it is opened, **then** the day view shows its hours, its publications, its note and theme, its slots and whether it is quiet. A click on an empty hour opens the editor with that date and time.
2. **Given** a publication of that day, **when** its time, text or target is changed in the day view, **then** the service applies the change and the view shows the server's answer. Approval works as before, and D-145 still holds.
3. **Given** a quiet day, **when** an agent schedules on it on any surface, **then** it is refused with a hint. **When** a person schedules on it, **then** the person is asked to confirm, and only the confirmed call schedules. Only a person creates or removes a quiet day.
4. **Given** a weekly slot, **when** the calendar shows a range, **then** the slot appears on each matching day at its local time, also across a DST change, and it never sends anything.
5. **Given** a publication, **when** it is duplicated to another day, **then** a new draft publication exists at that time and the original is unchanged.
6. **Given** the `.ics` export, **when** it is parsed, **then** it lists the scheduled targets with UTC times, escaped text, and no credential or access token.
7. **Given** the MCP tool list, **when** measured, **then** it passes `surfacebudget`, and any raise equals the measured size and is recorded.
8. **Given** the checks, **when** run, **then** `task lint`, `task test`, `task test:web`, `bun run typecheck`, `check:thin-client`, `check:i18n`, the security suite and the e2e of the calendar pass.

## Risks and mitigation

- Time zones and DST: store local date and zone, expand on read, and test the DST edges.
- MCP budget: extend actions, not tools. Measure. If a raise is needed, record the exact size, as D-143 did.
- A quiet day bypassed through an old path (send now, retry): the rule lives in the service for every scheduling path, with a test for each.
