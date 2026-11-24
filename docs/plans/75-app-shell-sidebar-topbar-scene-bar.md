# 75: App shell: sidebar, top bar, scene bar and notification center

## Goal

Wherever a creator is, the app shows where they are, what the agent is doing and what waits for them. This epic builds that frame around every page, reporting only what the server knows today and saying so when it knows nothing.

## Context and related decisions

- Author decisions D-120, D-134, D-137, and the standing requirement that the skill, its state, the tool, the workflow and approval notifications are always explicit. Frames 01, 23 and the state frames of the final redesign file held by the author.
- SDD-012 section 23 (the dashboard frame), SDD-017 (chat), SDD-015 (approvals). Code: `web/resources/js/layouts/DashboardLayout.tsx`, `web/resources/js/dashboard/Sidebar.tsx`, `web/resources/js/dashboard/sections.ts`, `web/resources/js/dashboard/chat/useChat.ts`, `web/resources/js/dashboard/approval/useApprovalQueue.ts`, `web/resources/js/components/ReleaseMark.tsx`.

## User stories

- As a creator, I want a navigation grouped by what I am doing so that I find a section without reading all of them.
- As a creator, I want to see from any page what the agent is doing so that it never works behind my back.
- As a creator, I want one counter of what waits for me so that no request is forgotten.

## Scope

- Modules: `web/resources/js/layouts/`, `web/resources/js/dashboard/Sidebar.tsx`, `web/resources/js/dashboard/sections.ts`, new `web/resources/js/shell/`, the shell keys of `web/resources/lang/*.json`, SDD-012 section 23.
- Must not touch: page bodies, `core/`, `service/`, the editors (epics 85 and 86 mount the shell parts there).

## Out of scope

New backend data (epics 76 to 78), the body of Approvals and Chat (epics 82 and 83), and any decision made from a new code path.

## Dependencies

57 (it changes the sidebar footer and the layouts) and 74.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | New sidebar, top bar, scene bar, bell and notification center |
| HTTP, CLI, MCP | None: it reads existing endpoints |

## Core commands and permissions

None. A decision goes through the existing `ChatAnswerPermission` and `ApprovalDecide`, under their current checks.

## Technical design

- **Sidebar.** Groups: Create (Projects, Write, Edit video, Edit image), Assets (Video, Visual, Sound, Brand kit), Agent (Skills, Prompts, Memory), Publish (Approvals, Publish), Connect (Connections, Webhooks, API keys). Home sits above them; Settings and Log out in the footer, as epic 57 leaves them. Record and Local models appear only when epics 61 and 70 add their routes. `SECTION_KEYS` and the Go test `TestDashboardSectionsMatchTheSidebar` keep working; groups are a separate list over those keys, plus links to routes that already exist.
- **Forms of the sidebar:** full, icon rail at 1280 px, drawer at phone width.
- **Top bar:** breadcrumb, bell with its count, Chat button.
- **`SceneBar`** at the bottom of each page, 56 px, with the variants `none`, `idle`, `working`, `waiting`, `denied`, `done` and `failed`. It shows the agent profile, the state from `chat.State`, the tool title of the last `tool_call` event as untrusted text, and Allow and Deny for an open `permission_request`.
- **Honest fields.** Until epic 76 lands, the skill slot says "Not reported". Nothing presumes a skill, a next step or a cost. "Always allow" is never offered, even when the agent offers that option.
- **Bell and center.** The count and the list read the open chat permission of the sessions the client knows and `ApprovalList`, each row linking to its own flow. Epic 77 replaces this with one read model.
- **Version.** `ReleaseMark` moves into the sidebar footer, next to the mode.
- **Text scale.** The layout stops setting a scale per page once the tokens of epic 74 are the default.

## Tasks

- [x] 1. Write the shell section of SDD-012. Verify: `bunx markdownlint-cli2@0.24.1 docs/sdds/012-ui-architecture.md` exits 0.
- [x] 2. The grouped sidebar with its three forms, keeping the keys. Verify: `go test ./web/routes/...` and `cd web && bun run test -- Sidebar sections` pass.
- [x] 3. The top bar and breadcrumb. Verify: `cd web && bun run typecheck && bun run test` pass.
- [x] 4. `SceneBar` and its mobile form, from the chat state, with every variant. Verify: a Vitest per variant, and `bun run check:i18n` exits 0.
- [ ] 5. Allow and Deny on the bar through the existing endpoint, with no "always" option. Verify: `cd web && bun run e2e -- agent-chat` and a new spec `scene-bar` pass.
- [ ] 6. The bell and the notification center over the existing reads. Verify: new spec `notification-center` passes, and `cd web && bun run e2e -- approval` passes.
- [ ] 7. Mount the shell in `DashboardLayout`, with focus order and the skip link. Verify: `cd web && bun run e2e -- a11y` passes.
- [ ] 8. Check 1440, 1280 and 390 px in both themes. Verify: the screenshots are in the hand-off and `task test:web` passes.

## Test strategy

Unit tests per bar variant, Playwright specs for a real permission through the fake agent, the axe specs at three widths.

## Acceptance criteria

1. **Given** any dashboard page, **when** it opens, **then** the sidebar, the top bar and the scene bar are present (spec `scene-bar`).
2. **Given** a running fake agent, **when** it calls a tool, **then** the bar shows the session state and the tool title as plain text.
3. **Given** an open permission, **when** a person answers on the bar, **then** the request ends through `chat.sessions.answer` and the conversation shows the same answer.
4. **Given** a request whose options include `allow_always`, **when** it is shown, **then** no such action exists on the bar, the center or the bell.
5. **Given** no agent session, **when** a page opens, **then** the bar says so and invents no skill, tool or step.
6. **Given** a pending approval, **when** the center opens, **then** it is listed and its action leads to its own flow.
7. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run lint`, `task test:web` and `go test ./web/routes/...` pass.

## Risks and mitigation

- A second answer path could weaken the privileged check: the bar calls the same endpoint, and `TestSecurity_` stays green.
- The bar takes 56 px on every page: the main region scrolls above it, and specs check that no control is covered.
- **STOP** if a field of the design has no server data and cannot be shown as "Not reported", or if a decision would need a new endpoint.

## Size

L: the frame of every page and a new live component. About 8 commits.

## Priority

Must. Explicit agent state is a standing requirement of the author.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of the bar in every variant.
