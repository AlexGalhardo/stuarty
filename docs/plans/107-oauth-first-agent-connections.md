# 107: OAuth-first agent connections and secondary API key mode

## Goal

Connecting an AI agent in Stuarty no longer fails or forces the user to hunt for CLI executable paths on disk: the primary connection experience is one-click web/OAuth sign-in (matching official provider OAuth flows such as Sign in with ChatGPT, OpenRouter OAuth PKCE, and Google OAuth) with API key connection offered cleanly as a secondary option, while local CLI/ACP agents are auto-detected and remain an optional advanced mode without manual path entry.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #6**: Agent connection is still broken/unreliable (observed in v0.60.0: `gemini · long video Failed · it could not start`). Connecting an agent should ideally happen via **OAuth / web authentication** rather than asking the user to paste a binary path, with **API key connection** available as a secondary option.
- Architecture and policy alignment (CRITICAL — read carefully before task 1):
  - Today, `SDD-017` (`ProfileSetup.tsx` + `service/agenthost/`) requires the user to configure a local CLI executable path and arguments over ACP, which breaks on Windows PATH/shim differences, missing ACP flags, and CLI auth expiry.
  - Meanwhile, `SDD-016 §12` (epic 12) already implements official OAuth sign-in for **OpenAI ("Sign in with ChatGPT")** and **OpenRouter (OAuth PKCE)**, and `SDD-016` already implements API-key connections for **OpenAI**, **Anthropic**, **Google Gemini**, and **OpenRouter**.
  - **ADR-0037** originally stated "Stuarty has no agent loop of its own" (chat only hosts external ACP binaries). Enabling OAuth-first and API-key-secondary agents directly in the Chat panel requires **ADR-0054 (amending ADR-0037)** to allow a thin, bounded MCP tool-calling chat runner in `service/agenthost/` backed by any connected Stuarty AI provider (OAuth or API key), still invoking Stuarty exclusively through the public MCP/Service surface under the exact same project-scoped token, cost ceilings, and human approval queue (ADR-0032)!
  - **ADR-0036 item 3 MUST stay intact**: never automate or scrape a forbidden subscription login (e.g., Anthropic forbids third-party Claude.ai consumer web login; Claude connects via official Anthropic API key or auto-detected official Claude Code CLI, while OpenAI connects via official Sign in with ChatGPT OAuth or API key, OpenRouter via official OAuth PKCE or API key, and Google via official OAuth 2.0 PKCE with the user's own OAuth client or Gemini API key).

## Scope

- Modules owned:
  - `docs/adrs/0054-oauth-and-api-agent-chat-sessions.md` (new ADR amending ADR-0037, proposed in task 1 for author confirmation).
  - `docs/sdds/017-hosted-agent.md`, `docs/sdds/016-ai-providers.md`, `docs/sdds/007-security-model.md` (threats for provider-backed tool-calling sessions: token/turn/cost bounds, untrusted tool outputs, approval bypass prevention).
  - `core/chat/` (extend profile/session types with `mode: "oauth" | "api_key" | "local_acp"`, connection/model reference, turn/tool-call loop limits, and plain diagnostic error reasons).
  - `service/providers/google/` (add official OAuth 2.0 PKCE sign-in option alongside Gemini API key, using the user's own OAuth client per ADR-0034/ADR-0035 pattern).
  - `service/agenthost/` (add `provider_runner.go`: a bounded tool-calling session driver that speaks to the connected OAuth/API provider through `connections.Manager.With` and executes Stuarty MCP tools over the exact same project-bound `AgentTokens` session token, emitting the identical SSE `ChatEvent` stream; also fix Windows local ACP auto-discovery & shim handling so local CLI agents that are installed work out of the box without manual paths).
  - `service/chat_ops.go`, `service/provider_signin_ops.go` (one-click "Connect agent" flow that pairs an OAuth or API-key connection with a chat agent profile automatically).
  - `web/resources/js/dashboard/chat/ProfileSetup.tsx`, `web/resources/js/dashboard/chat/ChatDock.tsx`, `web/resources/js/pages/Connections/` (redesign agent connection UI: **1. Sign in with Web / OAuth** primary cards [ChatGPT/OpenAI, OpenRouter, Google/Gemini], **2. Connect with API Key** secondary accordion/tab [OpenAI, Anthropic Claude, Google Gemini, OpenRouter], **3. Local CLI agent (Advanced)** collapsed with one-click auto-detected buttons and zero required binary path input!).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `renderer/`, `engine/`, `desktop/`, `core/timeline/`, `core/image/`.

## Out of scope

- Any forbidden/unofficial subscription login or cookie scraping (ADR-0036 item 3 — e.g., no Claude.ai browser-cookie login).
- Adding image/video generation providers like Higgsfield or fal.ai (owned by epic 109).
- Third-party creative MCP servers like ArtCraft or OpenPencil (owned by epic 110).

## Dependencies

- Epics 12, 14, 83, 84, and 116 (OAuth sign-ins, hosted agent chat, chat and connections screens, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — update FigCraft frames for the OAuth-first agent connection UI).

## Technical design

1. **ADR-0054 (Amending ADR-0037)**:
   - Keeps every safety invariant of ADR-0037 and ADR-0032:
     - The provider-backed agent runner has **no private channel** to the core; it calls Stuarty's own MCP endpoint (`POST /mcp`) using the same project-bound `stc_` session token (`read`, `edit`, `media`, `render`, never `admin`, never privileged).
     - Every costly generation or publication still waits in the Approvals queue.
     - Every turn is bounded by configurable max tool-call steps (default 25), max wall time (15 min), and the workspace's cost ceilings (SDD-015).
     - All model output and tool arguments pass through `untrusted` sanitization and token scrubbing.

2. **Three-tier Agent Connection UX (`ProfileSetup.tsx` & `Connections/`)**:
   - **Primary (OAuth / Web Sign-In)**: large branded connection cards with one-click web OAuth buttons (`Sign in with ChatGPT`, `Sign in with OpenRouter`, `Sign in with Google`) -> completes OAuth PKCE in the browser -> automatically creates/activates a ready-to-chat agent profile and returns to the chat panel ready to send messages immediately.
   - **Secondary (API Key)**: clean secondary section ("Or connect with an API key") letting the user pick OpenAI, Anthropic (Claude), Google Gemini, or OpenRouter, paste their key (stored in the encrypted vault), pick a model from the live catalogue, and start chatting immediately.
   - **Advanced / Optional (Local CLI Agent via ACP)**: collapsed under "Local CLI agent (advanced)" with automatic PATH/AppData/Scoop/npm-global detection for `claude`, `codex`, `gemini`, `opencode` — fixing Windows `.cmd`/`.exe` shim invocation and clear health diagnostics ("Not signed in", "CLI not installed", "ACP flag mismatch") instead of an opaque `Failed · it could not start` banner.

3. **Provider-backed MCP runner (`service/agenthost/provider_runner.go`)**:
   - Uses standard-library HTTP through `connections.Manager.With` (zero new Go dependencies!).
   - Discovers tools dynamically from Stuarty's own in-process MCP handler (`tools/list`), translates them to the provider's tool-calling format (OpenAI Chat/Responses, Anthropic Messages, Google Gemini), streams assistant text and `tool_call` / `tool_call_update` events into the exact same `ChatEvent` SSE pipeline (`SDD-017 §8`), and records real token usage and cost in the cost ledger (`SDD-015`).

## Tasks

- [ ] 1. Write `docs/adrs/0054-oauth-and-api-agent-chat-sessions.md` (amending ADR-0037 while preserving ADR-0036 item 3) and amend SDD-017, SDD-016, and SDD-007 (new threats for bounded provider-backed chat loop). **STOP for author approval of ADR-0054 before writing code.** Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Extend `core/chat/` with `mode` (`oauth`, `api_key`, `local_acp`), connection/model binding, turn/step budgets, and structured failure reasons with table tests. Verify: `go test ./core/chat/...` passes.
- [ ] 3. Implement `service/agenthost/provider_runner.go` (bounded MCP tool-calling loop over OAuth & API-key connections for OpenAI, OpenRouter, Anthropic, and Google, emitting `ChatEvent` stream, recording ledger cost, scrubbing tokens) plus Windows `.cmd`/shim fix for local ACP agents. Verify: `go test ./service/agenthost/... ./service/... -run "Chat|Agent"` passes with recorded fake-provider contract tests and zero network access.
- [ ] 4. Add Google OAuth 2.0 PKCE sign-in support in `service/providers/google/` and `service/provider_signin_ops.go` (user's own OAuth client, vault storage, token refresh) with contract tests. Verify: `go test ./service/providers/... ./service/... -run "SignIn|OAuth"` passes.
- [ ] 5. Redesign `ProfileSetup.tsx`, `ChatDock.tsx`, and the Agent section of `Connections/Index.tsx` so OAuth/web sign-in is the primary hero action, API key is secondary, and local CLI binary path is collapsed under Advanced with one-click auto-detection; add `en.json` and `pt-BR.json` strings. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run resources/js/dashboard/chat/ resources/js/pages/Connections/`.
- [ ] 6. Add security tests (token scrubbing, project isolation, cost ceiling stop, approval queue not bypassed, no forbidden subscription auth) and Playwright E2E tests against a fake OAuth/API provider. Verify: `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, and the Playwright chat specs pass.

## Test strategy

- Unit tests: `core/chat` mode transitions, step limits, error reason mapping.
- Contract tests (no network): fake OAuth PKCE server + fake OpenAI/OpenRouter/Anthropic/Google tool-calling server verifying multi-step MCP tool execution, cost ledger entries, and cancellation.
- Security tests: verify the provider runner cannot call admin ops, cannot approve its own requests, cannot cross project boundaries (`E_FORBIDDEN`), and never leaks `stc_` tokens or provider tokens into transcripts or logs.
- E2E (Playwright): connect via fake OAuth button in Chat setup (zero binary paths typed) -> send a prompt -> agent calls MCP tool -> UI streams progress and final reply; switch to secondary API key flow -> works identically.

## Acceptance criteria

1. Opening the Agent Chat setup shows **OAuth / Web Sign-In** (ChatGPT, OpenRouter, Google) as the primary connection flow and **API Key** as the secondary option; no binary path input is shown unless the user explicitly expands "Local CLI agent (advanced)".
2. Completing an OAuth or API-key connection automatically makes the agent ready to chat in the panel, executes Stuarty MCP tools cleanly under project-scoped permissions, streams tool calls in the SceneBar and Chat panel, and enforces cost ceilings and human approvals.
3. No forbidden subscription login (ADR-0036 item 3) is offered, and all credentials stay sealed in the vault.
4. Existing local ACP agents on Windows resolve `.exe`/`.cmd` shims cleanly and report plain-language failure reasons if not installed or not signed in.
5. `task lint`, `task test`, `task test:web`, `go test -run TestSecurity_ ./...`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and the Playwright chat specs pass.

## Risks and mitigation

- **Contradicts ADR-0037 ("Stuarty has no agent loop of its own").** Mitigation: **STOP** at task 1 until the author approves ADR-0054 amending ADR-0037.
- **Runaway tool-call spend.** Mitigation: enforce strict per-turn tool-step cap (25), wall-clock cap, and pre-call cost ceiling check (SDD-015) before every model request inside a turn.
- **Provider OAuth policy changes.** Mitigation: each OAuth adapter is isolated behind `ProviderSignIn` with contract tests, and API key mode always works as an independent fallback.

## Size

L

## Hand-off for review

The commits of `epic/107-oauth-first-agent-connections`, approved ADR-0054, updated SDD-017/SDD-016/SDD-007, Go/security/Vitest/Playwright outputs, and screenshots of the redesigned OAuth-first agent connection flow.
