# 24: Optional Google account and login on a server

## Goal

A creator who runs Stuarty on their own server signs in with a password, as today, or with their Google account if they prefer, using a Google project they own. On the desktop nothing changes: no login is asked.

## Context and related decisions

- ADR-0034 (local login by default, an optional Google account with the user's own client), ADR-0019 (authentication and local trust), ADR-0020, ADR-0033.
- Author decision D-057. PRD-003 RF-217; PRD-004 NFR-060.
- Research note `web/07-stack-desktop-web-self-hosted` (Google lets a project whose owner is its only user skip verification; what that mode limits).

## Scope

- Modules: `web/app/auth/` (or the current authentication package of the server), `service/connections/oauth` (reused), `web/app/controllers/` (login pages and callback), `web/resources/js/pages/Auth/` and `Settings/`, `web/database/migrations/` (linked identities), SDD-007 (network mode), a new section in SDD-014.
- The Google client is the same one the user created for YouTube (epic 18) when they want; the guide says which extra redirect address and scopes it needs.

## Out of scope

- A Google client owned by the project, or any central account service.
- Other identity providers, single sign-on for teams, multi-tenant accounts.
- Login on the desktop in local mode.

## Dependencies

04, 18.

## Technical design

- **Linking, not replacing**: an existing local user links a Google identity from their settings (a privileged session, password confirmed). Afterwards "Sign in with Google" signs in that user. There is no self-registration: a Google identity that is not linked is refused with a clear message. The password login always remains, so losing the Google account never locks the user out.
- **Flow**: OpenID Connect authorization code with PKCE through `service/connections/oauth`; the ID token is verified (issuer, audience, expiry, nonce, signature against Google's published keys, fetched through the outbound gate with the Google connection). Only the subject identifier and the e-mail are stored.
- **Sessions**: the session created is the same kind as a password session, with the same cookie flags, expiry and rate limits of the existing network mode.
- **Admin controls**: the server owner can turn Google sign-in off, and can restrict it to a list of e-mail addresses.
- **Outbound**: Google sign-in is a connection like any other; with it off, the server opens no connection to Google.
- **Review of the existing network mode**: the checklist of SDD-007 for network mode is run again with the new routes (rate limit on login and callback, CSRF on linking, session fixation on callback).

## Tasks

- [x] 1. Amend SDD-007 and SDD-014 with the flow, the stored fields and new threat rows (an unlinked identity signing in, account takeover by e-mail match, callback CSRF, session fixation). Verify: `task lint:md` exits 0.
- [x] 2. Implement ID token verification with a fake issuer in tests. Verify: `go test ./web/app/... -run TestIDToken` passes (wrong audience, expired, bad signature, wrong nonce).
- [x] 3. Implement linking and unlinking from settings. Verify: `go test ./web/app/... -run TestLinkIdentity` passes.
- [x] 4. Implement sign-in with a linked identity and the refusal of an unlinked one. Verify: `go test ./web/app/... -run TestGoogleSignIn` passes.
- [x] 5. Implement the admin controls. Verify: `go test ./web/app/... -run TestGoogleSignInPolicy` passes.
- [x] 6. Build the login button, the settings and the guide, in both languages. Verify: `task test:web` and `web/e2e/specs/google-sign-in.spec.ts` pass with the fake issuer.
- [x] 7. Security tests of the new rows. Verify: `go test -run TestSecurity_ ./...` and `go run ./internal/tools/threatcheck` exit 0.
- [x] 8. One real sign-in by the author on a Docker deployment, or marked as not verified. Verify: the result or the note is in the hand-off.

## Test strategy

- Unit: token verification; the linking rules.
- Security: one `TestSecurity_` test per new row; the network-mode checklist.
- End to end: link, sign out, sign in with the fake issuer; an unlinked identity is refused.

## Acceptance criteria

1. In network mode a linked user signs in with Google and gets the same session as with a password.
2. A Google identity that was never linked cannot sign in, even when its e-mail equals a local user's.
3. The password login keeps working after linking and after unlinking.
3a. Unlinking ends every other session of the user and keeps the current one (D-104; `TestUnlinkEndsTheOtherSessionsAndKeepsTheCurrentOne`).
4. With Google sign-in off, `bash scripts/no-egress.sh` passes on a server with no other connection.
5. In local mode on the desktop no login is shown.
6. `go test ./web/app/...`, `go test -run TestSecurity_ ./...` and `task test:web` pass with no network access.

## Risks and mitigation

- **Account takeover through an e-mail match.** Mitigation: linking is explicit and by subject identifier, never by e-mail.
- **The user's Google project in testing mode expires tokens quickly.** Mitigation: the guide explains it; sign-in needs no long-lived token.

## Size

M

## Hand-off for review

The diff of `epic/24-google-account-and-server-login`; the outputs of the acceptance commands; whether a real sign-in was verified.
