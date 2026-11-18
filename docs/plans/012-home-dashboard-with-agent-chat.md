# Plan 012: A home dashboard with a left sidebar and a centred agent chat

This plan answers what the first screen of the web app (and of the desktop shell, which shows the same pages) becomes: a modern dashboard in the style of the ChatGPT and Claude apps, with navigation in a left sidebar (Projects, Asset library, Templates, Prompts, Skills, Memory, Brand kit, Settings) and a chat in the centre where the person writes what video they want. When it is delivered, `/` opens that dashboard instead of redirecting to `/projects`, and a request typed in the chat reaches an agent and its answer comes back on the same screen, without the editor gaining any intelligence or outbound connection.

- Owner: designer (step 2), orchestrator (steps 1 and 3), senior-c (steps 4-5), senior-d (step 6), senior-e (steps 7-9); one branch, owners in sequence
- Effort: L
- Depends on: the author's choice in step 1; plan 002 (the studio plugin) for the Prompts and Skills pages; coordinate with the plan that finishes the CapCut-style editor UI (the former EPIC-53) because both touch `web/resources/js/layouts/`
- Origin: author request of 2026-11-18
- Status: TODO (step 1 is an author decision)
- Written against: `8787714a`

## 1. Why

Today `/` redirects to `/projects` (`web/routes/pages.go:58`), and the person must know the editor to start. The studio direction (author decisions D-041 to D-044) makes "describe the video you want" the natural first action, and the chat-first layout of the ChatGPT and Claude apps is what people already know. The constraint: the product has no intelligence and no outbound capability (ADR-0020, author decision D-041). The chat therefore cannot call a model itself: it must hand the request to an agent the person already runs, or execute editor commands directly.

## 2. Current state (drift check)

```text
grep -n 'r.Get("/{$}"' web/routes/pages.go             # redirect to /projects
ls web/resources/js/pages web/resources/js/layouts       # Auth, Error.tsx, Projects, Settings; AppLayout.tsx
grep -n "type LibraryAsset struct" service/asset_library.go   # the workspace asset list exists (AssetList)
ls web/resources/js/components/UntrustedText.tsx          # the component for text that came from outside
```

## 3. Step 1: Choose how the chat gets its answers (author decision)

| Option | How it works | For | Against |
| --- | --- | --- | --- |
| **A. Agent inbox (recommended)** | The chat stores each message in the workspace (a conversation table). An agent the person runs (Claude Code with the studio plugin, or any MCP client) reads new messages through a new MCP tool `inbox`, works with the studio skills and the editor tools, and posts replies and links to projects or exports; the page shows them live over the existing server-sent events | no model in the product, no outbound connection, works with any agent; the same conversation is visible to the person and the agent | the person must have an agent running and connected; a new MCP tool raises the tools/list budget (NFR-020) |
| B. Command chat | The chat accepts only editor commands in plain words with a fixed grammar ("new 9:16 project", "import ...", "export") and runs them through the service | works with no agent at all; deterministic | not a real conversation; duplicates the CLI |
| C. Model in the browser | The page calls a model provider directly with a key the person types | the most "ChatGPT-like" | puts intelligence and provider keys into the product; contradicts author decision D-041; rejected |

- Owner: orchestrator. Record the choice as an author decision. The steps below assume option A; with option B, steps 4 to 6 are replaced by a command parser that calls existing service operations, specified in SDD-012 first.

## 4. Steps

### Step 2: Wireframes and visual design

- Owner: designer (OpenPencil). Two or three versions of the dashboard at desktop and at 390 px width, light and dark, following `docs/design-system.md`: a collapsible left sidebar (logo, "New video" button, conversation list, navigation links, the release mark at the bottom left as today), the centred chat (an empty state with three example prompts per use case, the message list, a composer with attach-file and a project picker), and a right-hand panel closed by default for the project or export a reply links to. The editor layout of the CapCut-style plan is unchanged.
- Acceptance:
  - [ ] the author picks one version; its specification (sizes, tokens, states, keyboard and screen reader behaviour) is written into SDD-012 as a new section.

### Step 3: Specify the conversation model and the surfaces

- Owner: orchestrator. Files: SDD-000 (names: tables `conversations`, `messages`; operations `ConversationList`, `ConversationGet`, `MessagePost`, `InboxRead`, `InboxReply`; event `message.posted`), SDD-005 (tables), SDD-006 (event), SDD-003 (MCP tool `inbox` with actions `read`, `reply`; HTTP endpoints; no CLI beyond `inbox` for parity), SDD-007 (messages from agents and people are untrusted text: stored, shown with `UntrustedText`, budgeted, never executed).
- Commit: `docs(sdd): specify the home conversation and the agent inbox`.
- Acceptance:
  - [ ] `task lint` clean; the MCP tool's description measured against NFR-020 (raise it only with the author's approval).

### Step 4: Tables and service operations

- Owner: senior-c. Files: a migration in `web/database/migrations/`, `service/` operations and tests (post, list, read since a cursor, reply, mark handled; attachments are references to project ids and job ids only).
- Commit: `feat(service): store home conversations and the agent inbox`.
- Acceptance:
  - [ ] `go test ./service/... ./web/database/...` passes; `task test:race` passes for `service/`.

### Step 5: HTTP endpoints and the event

- Owner: senior-c. Files: `web/app/http/endpoints.go`, handlers, the SSE event, OpenAPI fixtures, `web/contract/` parity; route `/` renders the new page instead of redirecting (`web/routes/pages.go`).
- Commit: `feat(http): serve the home conversation and its events`.
- Acceptance:
  - [ ] `go test ./web/app/http/... ./web/contract/... ./web/routes/...` passes.

### Step 6: The `inbox` MCP tool and its studio skill

- Owner: senior-d. Files: `web/app/mcp/definitions/inbox.json`, handler, tests, regenerated agent docs; `studio/skills/home-inbox/SKILL.md` (loop: read new messages, route to `video-studio`, reply with progress and the final link).
- Commit: `feat(mcp): add the inbox tool for the home chat`.
- Acceptance:
  - [ ] `go test ./web/app/mcp/...` passes, budget tests included; `go run ./web/cmd/gen-agent-docs --check` and `go run ./internal/tools/skillcheck studio` exit 0.

### Step 7: The dashboard layout and the sidebar

- Owner: senior-e. Files: `web/resources/js/layouts/` (a `DashboardLayout` beside `AppLayout`), `web/resources/js/pages/Home/`, both locale files. Before writing UI, follow the `modern-web-guidance` skill (layout, accessibility, reduced motion) and the existing Base UI primitives in `web/resources/js/components/ui/`.
- Sidebar links: Home (chat), Projects, Asset library, Templates, Prompts, Skills, Memory, Brand kit, Settings. Pages that do not exist yet show an empty state that says which plan brings them, not a dead link.
- Commit: `feat(ui): add the dashboard layout with the left sidebar`.
- Acceptance:
  - [ ] `task test:web`, `bun run check:thin-client`, `bun run check:i18n` pass; keyboard: the sidebar and composer are reachable with Tab and the sidebar collapses with a shortcut listed in the shortcuts dialog.

### Step 8: The chat

- Owner: senior-e. Files: `web/resources/js/pages/Home/` components, tests, an end-to-end spec in `web/e2e/specs/`.
- Behaviour: post a message; replies appear live; links to projects open the editor; an "agent not connected" state appears when no `inbox` read happened in the last minute, with the command to start one; every agent text renders through `UntrustedText`.
- Commit: `feat(ui): chat with a connected agent from the home page`.
- Acceptance:
  - [ ] an end-to-end spec posts a message, a test MCP client replies, and the reply appears without a reload.

### Step 9: Asset library, Prompts, Skills and Memory pages (the delivery of this plan)

- Owner: senior-e (pages), senior-c (read-only service operations).
- Asset library lists `AssetList` with filters by kind. Prompts, Skills and Memory are read-only views of the studio plugin folder and of `VUKEM_STUDIO_HOME` (local reads only, configured path, confined like import roots in SDD-007); each item has "Use in chat", which pre-fills the composer.
- Commit: `feat(ui): browse assets, prompts, skills and memory from the sidebar`.
- Acceptance:
  - [ ] `task test:web` and the security suite (`go test -run TestSecurity_ ./...`) pass, with a new test that a path outside the configured studio home is refused;
  - [ ] a walk-through in the running app (headless browser screenshots of each page at desktop and 390 px, light and dark) is attached to the report.

## 5. Out of scope

- Any model call, provider key or outbound connection from the server or the page.
- Changing the editor's own layout.

## 6. STOP conditions

- The author picks option C: stop; it needs a new ADR that supersedes part of ADR-0020 and author decision D-041.
- The `inbox` tool cannot fit NFR-020 even after trimming descriptions: stop and report the measured tokens.

## 7. Maintenance

The sidebar becomes the place every new top-level page is linked from; add a link only with its page, never ahead of it.
