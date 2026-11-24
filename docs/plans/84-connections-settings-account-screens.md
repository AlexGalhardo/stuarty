# 84: Connections, settings, account and error screens

## Goal

The screens that hold accounts, keys and preferences are plain and trustworthy: a creator always knows what is connected, what stays local and how to recover their account.

## Context and related decisions

- Author decision D-120 and epic 57 (local account), ADR-0032, ADR-0033, SDD-014, SDD-022. Frames 12, 15, 16, 18, 20 and 21 of the final redesign file held by the author, with their state frames.
- Code: `web/resources/js/pages/Connections/`, `web/resources/js/pages/Settings/`, `web/resources/js/pages/Auth/`, `web/resources/js/pages/Webhooks/`, `web/resources/js/pages/Providers/`, `web/resources/js/pages/Social/`, `web/resources/js/pages/Error.tsx`.

## User stories

- As a creator, I want to see what is connected and where my keys live.
- As a creator, I want to create my account, keep my recovery key and unlock the app with clear steps.
- As a creator, I want an error page that tells me what to do next.

## Scope

- Modules: the pages above, except `Settings/BrandKit.tsx` (epic 79), and their language keys.
- Must not touch: `core/`, `service/`, the vault, the auth flow.

## Out of scope

Changing any account or recovery rule, and Google sign-in, which epic 57 removes.

## Dependencies

57, 74 and 75.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Connections, Settings, API keys, Webhooks, account, unlock, error |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new.

## Technical design

- Account screens follow epic 57: create account, recovery key shown once with a confirmation, sign in, locked state.
- Secrets are never shown after they are saved. Fields show only what the server returns.
- The error page keeps the core's error text verbatim and adds the next step.

## Tasks

- [x] 1. Connections and the setup guides. Verify: `cd web && bun run test -- Connections SocialSection BlogSection ProviderCards` passes.
- [x] 2. Settings and its sections. Verify: `cd web && bun run test -- Settings` passes.
- [x] 3. API keys. Verify: `cd web && bun run test -- Keys` passes.
- [x] 4. Webhooks. Verify: `cd web && bun run typecheck && bun run e2e -- webhooks` pass.
- [x] 5. Create account, recovery key, sign in and unlock. Verify: the first-run and logout specs of epic 57 pass.
- [x] 6. The error page. Verify: `cd web && bun run test -- Error` passes.
- [ ] 7. Check three widths in both themes. Verify: `cd web && bun run e2e -- a11y` passes.

## Test strategy

Existing page tests and the first-run specs, axe at three widths.

## Acceptance criteria

1. **Given** a saved credential, **when** its page opens, **then** the value is not in the DOM.
2. **Given** first run, **when** the account is created, **then** the recovery key shows once and needs a confirmation.
3. **Given** a locked workspace, **when** any page opens, **then** only the unlock screen shows.
4. **Given** a core error, **when** the error page shows, **then** its text is verbatim.
5. `bun run typecheck`, `bun run check:i18n`, `bun run lint`, `task test:web` and `go test -run TestSecurity_ ./...` pass.

## Risks and mitigation

- These screens hold secrets: no value moves to client state that was not there before.
- **STOP** if a change would need a new credential path or a change to auth.

## Size

L: many small pages with security weight. About 7 commits.

## Priority

Should. Connections and account are in the first-run flow.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of first run in both themes.
