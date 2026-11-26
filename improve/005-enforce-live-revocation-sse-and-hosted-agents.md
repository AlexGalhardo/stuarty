# Plan 005: Enforce live revocation on SSE `Revalidate` ticks and hosted agent sessions in network mode

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- web/app/http/api.go web/app/http/eventstream.go web/app/http/publications.go web/app/http/chat.go web/app/http/inbox.go web/app/auth/admin_users.go web/app/auth/network.go web/app/http/users_controller.go service/agenthost/host.go service/agenthost/tokens.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

In network mode, two credential-revocation boundaries fail to terminate already-active connections:

1. **SSE streams never re-verify credentials**: All 6 SSE endpoints (`eventsStream`, `imagesStream`, `approvalsStream`, `publicationsStream`, `chatStream`, `inboxStream`) run periodic `Revalidate` callbacks (`DefaultRevalidate = 60s`), but every callback simply reuses `c.Context()`, which holds the immutable `service.Caller` struct parsed once at handshake time. Revoking an API key, logging out, changing a password, disabling a user, or demoting an admin to member never closes open SSE streams — a demoted admin subscribed to workspace-wide `/approvals/events` or `/publications/events` keeps receiving all other users' events indefinitely.
2. **Hosted agent `stc_` tokens outlive user disablement / password reset**: `authenticateNetwork` trusts in-memory `agenthost.Tokens.Lookup` (valid up to 8 hours) without checking whether the owning user was disabled or had their password reset, and neither `auth.Admin.Update` nor `Network.ChangePassword` stops that user's live hosted agent sessions.

## Current state

- `web/app/http/eventstream.go:37-47, 66-76, 108-121`:

```go
	check := func() error {
		_, err := h.Service.ProjectGet(c.Context(), service.ProjectGetRequest{ProjectID: in.ID})

		return err
	}
	...
	return noIn{}, h.Events.Serve(c, in.ID, since, func() bool { return check() == nil })
```

  Same pattern in `imagesStream` (lines 66–76), `approvalsStream` (lines 108–121), `web/app/http/publications.go:146-150`, `web/app/http/chat.go:182-188`, and `web/app/http/inbox.go:35-39`.

- `web/app/http/api.go:419-424`:

```go
		if g, found := h.agentGrant(secret, true); found {
			next.ServeHTTP(w, r.WithContext(service.WithCaller(r.Context(), auth.AgentCaller(g, true))))
			return
		}

		caller, err := h.Auth.KeyCaller(r.Context(), secret)
```

- `web/app/auth/admin_users.go:77-136, 231-249` and `web/app/auth/network.go:130-141`: disable/demote users and change passwords by incrementing `session_version`, setting `disabled_ms`, and revoking API keys, without notifying `agenthost.Host` to terminate live sessions or revoke `stc_` grants.
- Conventions:
  - Read-only revalidation during SSE MUST NOT write `Set-Cookie` headers to an already-flushed response writer.
  - Follow existing `web/app/http/security_test.go` and `web/app/events/` test patterns.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| HTTP & events tests | `go test -race -count=1 ./web/app/http/... ./web/app/events/... ./web/app/auth/...` | `ok`, exit 0 |
| Agenthost tests | `go test -race -count=1 ./service/agenthost/...` | `ok`, exit 0 |
| Security suite | `go test -count=1 -run TestSecurity_ ./...` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `web/app/http/api.go`
- `web/app/http/eventstream.go`
- `web/app/http/publications.go`
- `web/app/http/chat.go`
- `web/app/http/inbox.go`
- `web/app/http/users_controller.go`
- `web/app/auth/network.go`
- `web/app/auth/admin_users.go`
- `service/agenthost/host.go` (and `service/agenthost/host_test.go`)
- `web/app/http/sse_revocation_test.go` (create)

**Out of scope** (do NOT touch):

- Frontend `EventSource` reconnect UI behavior (`web/resources/js/`).
- Local-mode account creation flow (handled in Plan 004).
- SSE wire format / event payload schemas (`docs/sdds/006-real-time-protocol.md`).

## Git workflow

- Branch: `fix/security-sse-and-agent-revocation` cut from `release/v0.60.0`.
- Commit per logical step with `Security:` footer, e.g.:
  `fix(security): re-verify caller credentials and role on SSE revalidation ticks`
  `fix(security): terminate hosted agent sessions when a network user is disabled or resets password`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Add a read-only `revalidateCaller(r *http.Request) (service.Caller, bool)` helper on `apiHandler`

1. Read `web/app/http/api.go` (both local-mode and network-mode auth middleware paths) and `web/app/auth/network.go` (how `SessionUser` validates cookie sessions and `session_version`/`disabled_ms`/`role`).
2. Add a read-only session lookup method on `auth.Network` (if `SessionUser` currently refreshes cookies or writes headers, add `LookupSessionCaller(ctx, r)` that only reads the session + user row, checks `disabled_ms == 0`, checks `session_version`, and returns the fresh `service.Caller` without touching `http.ResponseWriter`).
3. Add `func (h *apiHandler) revalidateCaller(r *http.Request) (service.Caller, bool)` in `web/app/http/api.go`:
   - In **network mode**:
     - If `Authorization` header is present: re-check `h.agentGrant(secret, true)` (and verify owning user is not disabled in `h.Auth.Users`) or `h.Auth.KeyCaller(r.Context(), secret)` (which checks key revocation + user disabled + current role/scopes).
     - Otherwise: re-check the session cookie via the read-only lookup (`disabled_ms`, `session_version`, current `role`/`Scopes`).
   - In **local mode**:
     - If `Authorization` header is present: re-check `h.agentGrant(secret, false)` or local bearer token (`h.Token`) and verify studio lock state (`h.Account == nil || !h.Account.Locked()`).
     - Otherwise: re-check `h.Sessions.Lookup(cookie)` and studio lock state.

**Verify**: `go test -race -count=1 ./web/app/auth/... ./web/app/http/...` -> `PASS`, exit 0.

### Step 2: Wire `revalidateCaller` into all 6 SSE handlers

1. Update the `authorized` callback passed to `h.Events.Serve*` in all six SSE handlers:
   - `web/app/http/eventstream.go`: `eventsStream` (`Serve`), `imagesStream` (`ServeImage`), `approvalsStream` (`ServeApprovals`)
   - `web/app/http/publications.go`: `publicationsStream` (`ServePublications`)
   - `web/app/http/chat.go`: `chatStream` (`ServeChat`)
   - `web/app/http/inbox.go`: `inboxStream` (`ServeInbox`)
2. In each callback:
   - Call `fresh, ok := h.revalidateCaller(c.Request())`; if `!ok`, return `false`.
   - For streams subscribed with an `owner` filter (`approvalsStream`, `publicationsStream`, `inboxStream` — read each handler carefully!), verify that the effective subscription scope did not shrink: if the stream subscribed with `owner == ""` (admin workspace-wide view) in network mode, return `false` if `!slices.Contains(fresh.Scopes, service.ScopeAdmin)`.
   - Build `ctx := service.WithCaller(c.Context(), fresh)` and invoke the handler's authorization check (`ProjectGet`, `ImageDescribe`, `ApprovalList`, `PublicationList`, `ChatSession`, `InboxList`) with `ctx` instead of the stale `c.Context()`.

**Verify**: `go test -race -count=1 ./web/app/http/... ./web/app/events/...` -> `PASS`, exit 0.

### Step 3: Terminate hosted agent sessions and reject `stc_` grants on user disablement, role change, or password reset

1. Read `service/agenthost/host.go` and `service/agenthost/tokens.go` to see how `EndAll` / `BeforeLock` terminates live sessions and calls each session's token `revoke()` closure.
2. Add `func (h *Host) EndByOwner(ownerUserID string)` on `service/agenthost.Host` (and wire it through a small optional hook or interface if `auth` does not import `agenthost` directly — check package dependencies so `depguard` stays clean!). It ends every live session owned by `ownerUserID` and revokes its `stc_` token immediately.
3. Call that hook when:
   - An admin disables a user or changes a user's role (`web/app/auth/admin_users.go` / `web/app/http/users_controller.go`).
   - A user changes or resets their password (`web/app/auth/network.go:ChangePassword`).
4. As defense-in-depth in `web/app/http/api.go:419-422` (`authenticateNetwork`), after `h.agentGrant(secret, true)` succeeds, verify the owning user (`g.OwnerUserID`) is still active (`disabled_ms == 0`) via `h.Auth.Users` (rejecting with 401 if disabled or missing).
5. Add unit tests in `service/agenthost/host_test.go` for `EndByOwner`.

**Verify**: `go test -race -count=1 ./service/agenthost/... ./web/app/auth/... ./web/app/http/...` -> `PASS`, exit 0.

### Step 4: Add network-mode revocation integration tests

1. Create `web/app/http/sse_revocation_test.go` (setting a short `Events.Revalidate` interval, e.g., `25 * time.Millisecond`, matching existing tests in `web/app/events/`) covering:
   - Revoking an API key closes an open `/projects/{id}/events` SSE stream on the next revalidation tick.
   - Disabling a user closes their open SSE stream and rejects requests bearing their active `stc_` hosted agent token with HTTP 401.
   - Changing a user's password closes their open session-cookie SSE stream and terminates/revokes their active hosted agent session.
   - Demoting an admin to member closes their workspace-wide `/approvals/events` and `/publications/events` streams on the next tick.

**Verify**: `go test -race -count=1 -run 'TestSecurity_.*SSE|TestSecurity_.*Revok|TestEndByOwner' ./web/app/http/... ./service/agenthost/... && go test -count=1 -run TestSecurity_ ./... && task lint:go` -> all exit 0.

## Test plan

- `service/agenthost/host_test.go`: `TestEndByOwner` verifying only the targeted user's live sessions and `stc_` tokens are ended/revoked while other users' sessions stay live.
- `web/app/http/sse_revocation_test.go`: 4 network-mode integration tests covering API-key revocation, user disablement, password reset, and admin demotion across SSE streams and `stc_` hosted agent tokens.
- Full security suite: `go test -count=1 -run TestSecurity_ ./...`.

## Done criteria

- [ ] All 6 SSE handlers (`events`, `images`, `approvals`, `publications`, `chat`, `inbox`) call `revalidateCaller` on every revalidation tick
- [ ] Workspace-wide admin SSE subscriptions close when the caller loses `ScopeAdmin`
- [ ] Disabling/demoting a user or changing a password terminates their live hosted agent sessions and invalidates `stc_` tokens immediately
- [ ] `go test -race -count=1 ./web/app/http/... ./web/app/events/... ./web/app/auth/... ./service/agenthost/...` exits 0
- [ ] `go test -count=1 -run TestSecurity_ ./...` exits 0
- [ ] `task lint:go && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Importing `service/agenthost` from `web/app/auth` violates a `.golangci.yml` `depguard` rule — use a callback/hook wired in `web/app/http` or `web/bootstrap` instead.
- Any SSE test in `web/app/events/` or `web/contract/` uses a mock `Caller` injected directly into `context.Context` without a real header/cookie (if so, check how those tests set up `apiHandler` before breaking them).

## Maintenance notes

- Any new SSE stream added to `web/app/http/` MUST wrap its `authorized` callback with `h.revalidateCaller(c.Request())` rather than reading the stale initial `c.Context()`.
- Reviewer focus: confirm `revalidateCaller` never writes response headers/cookies and never triggers side effects like updating `last_used_at` excessively if avoidable.
