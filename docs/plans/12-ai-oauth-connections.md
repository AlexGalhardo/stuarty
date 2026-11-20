# 12: Sign in with ChatGPT and OpenRouter

## Goal

A creator who already pays for ChatGPT, or has an OpenRouter account, connects it with a sign-in in the browser instead of copying an API key, and Stuarty uses it within the limits that provider allows.

## Context and related decisions

- ADR-0036 items on the four ways to connect, ADR-0033 (tokens in the vault), ADR-0032.
- Author decision D-059. PRD-004 RF-360 (the two OAuth flows), RF-365.
- Research note `web/03-integracao-ia`: OpenAI documents plan usage through "Sign in with ChatGPT" for open source and local applications; OpenRouter documents an OAuth flow with PKCE that yields a key of the user. Both pages were read on 2026-11-18 and must be read again before code.

## Scope

- Modules: `service/connections/oauth/` (new: a generic authorization-code flow with PKCE, reused by epics 18 and 24), `service/providers/openai/` and `service/providers/openrouter/` (credential kind `oauth`), `web/app/controllers/` (the callback route), provider cards on the Connections page, SDD-014, SDD-016, SDD-007 rows.

## Out of scope

- Any sign-in with a Claude.ai subscription: the provider forbids it in third-party applications (RF-365). The user's local agent is the supported path (epic 14).
- Social networks (epic 18) and the Google account (epic 24): they reuse the flow of this epic.

## Dependencies

09.

## Technical design

- **Generic flow** in `service/connections/oauth`: `Begin(provider) -> authorization URL` with a `state` and a PKCE verifier kept server-side for a few minutes; `Complete(state, code)` exchanges the code, stores access and refresh tokens in the vault and creates or updates the connection. `state` is single-use and bound to the browser session that began the flow.
- **Redirect**: a loopback address on the desktop and on a local server; on a network deployment the public base URL of the server. The callback route accepts only a known `state`.
- **Refresh**: tokens are refreshed before expiry under a per-connection lock; a refresh failure marks the connection as "needs sign-in" and never deletes user data.
- **OpenAI**: implement exactly the documented flow for open source applications, including any identifier of the host the provider asks for; only the models and limits the plan grants. If the published terms no longer cover a self-hosted application, STOP.
- **OpenRouter**: the PKCE flow returns a key bound to the user's account; it is stored like an API key and used by the adapter of the same name (model list from its catalogue, prices from its catalogue).
- **Only privileged sessions begin a flow**: an agent can read that a connection exists and cannot start, complete or remove one.

## Tasks

- [x] 1. Read the two official pages again; record the flow, the endpoints, the limits and the date in SDD-016; STOP if either changed in a way that forbids this use. Verify: the links and the date are in SDD-016 and in the hand-off.
- [x] 2. Add SDD-007 rows (stolen `state`, code interception, token in a log, refresh race) and the flow to SDD-014. Verify: `task lint:md` exits 0.
- [x] 3. Implement the generic flow with a fake authorization server in tests. Verify: `go test ./service/connections/oauth/...` passes (happy path, replayed state, wrong verifier, expired state).
- [x] 4. Implement refresh with the lock and the "needs sign-in" state. Verify: `go test ./service/connections/oauth/... -run TestRefresh` passes with the race detector.
- [x] 5. Implement the OpenRouter credential kind and adapter. Verify: `go test ./service/providers/openrouter/...` passes against recorded responses.
- [ ] 6. Implement the OpenAI sign-in credential kind. Verify: `go test ./service/providers/openai/...` passes against recorded responses.
- [ ] 7. Add the sign-in buttons and states to the provider cards, in both languages. Verify: `task test:web` and `web/e2e/specs/provider-sign-in.spec.ts` pass against the fake authorization server.
- [ ] 8. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.

## Test strategy

- Unit: PKCE, state handling, token storage, refresh.
- Contract: each provider's token and model endpoints from recorded responses.
- Security: one `TestSecurity_` test per new row.
- Manual: one real sign-in per provider by the author, or marked as not verified.

## Acceptance criteria

1. A sign-in begun from the Connections page ends with a connection whose tokens are only in the vault.
2. A replayed or foreign `state` is refused and creates nothing.
3. `TextGenerate` works through a connection made by sign-in, with the models the account grants.
4. An expired token is refreshed without the user noticing; a failed refresh shows "needs sign-in".
5. An agent credential cannot begin, complete or remove a sign-in on any surface.
6. `go test ./service/connections/... ./service/providers/...` and `go test -run TestSecurity_ ./...` pass with no network access.

## Risks and mitigation

- **The provider's terms change.** Mitigation: task 1 before code; the adapter is one package that can be switched off.
- **A real sign-in cannot be automated.** Mitigation: a fake authorization server in tests; the manual check is recorded honestly.
- **STOP** at task 1 when the terms do not cover this use.

## Size

M

## Hand-off for review

The diff of `epic/12-ai-oauth-connections`; the two official pages with the date read; the outputs of the acceptance commands; whether the real sign-ins were verified.
