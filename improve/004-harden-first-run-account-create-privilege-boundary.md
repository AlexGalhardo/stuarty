# Plan 004: Harden first-run `POST /auth/account/create` against bearer-token privilege escalation (VV-378)

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- web/app/http/account_controller.go web/app/http/session.go web/app/http/api.go web/app/http/account_test.go web/app/auth/session.go web/app/cli/open.go desktop/shell/app/main.go docs/sdds/007-security-model.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

`POST /auth/account/create` is explicitly designed (VV-378, SDD-007 §3.9 item 7) to refuse bearer-token callers so a local AI agent holding `~/.stuarty/token` cannot initialize the workspace account under an attacker-chosen password, receive the one-time recovery key, and immediately sign in as a privileged `Browser: true` person.

Today that boundary is bypassable: `accountCreate` checks only `caller.Actor == "local:"+auth.UILabel` and `ScopeAdmin`, yet `POST /api/v1/system/launch` issues a launch code to ANY local bearer token, and redeeming that code at `GET /auth/launch` mints a session cookie whose authenticated caller has `Actor: "local:ui"` and `Scopes: service.AllScopes`. That cookie passes `accountCreate`, allowing a non-browser bearer-token caller to seal the workspace, lock out the user, and escalate to a `Browser: true` person session via `POST /auth/account/login`.

## Current state

- `web/app/http/account_controller.go:136-145`:

```go
	// Only the person's own browser creates the account: the one that `stuarty open` or the desktop shell opened,
	// which holds the session cookie. A bearer caller (the local token of an agent, a key, a hosted agent) never
	// does, or it would set the password and sign in as a person (VV-378, SDD-007 section 3.9 item 7).
	if caller, ok := service.CallerFrom(c.Context()); !ok || caller.Actor != "local:"+auth.UILabel ||
		!slices.Contains(caller.Scopes, service.ScopeAdmin) {
		return accountCreateOut{}, &service.Error{
			Code: service.CodeForbidden, Message: "only a person opening Stuarty can create the account.",
			Hint: "run stuarty open, or use the desktop window, and create the account there",
		}
	}
```

- `web/app/http/session.go:26-44, 74-87`:

```go
	privileged, ok := h.Launch.Redeem(c.Query("code"))

	// With an account, only the password makes a person: a launch code, whoever asked for it, does not
	// (ADR-0052, SDD-007 section 3.9 item 5).
	if h.Account != nil {
		privileged = false
	}
...
	id, err := h.Sessions.Create(privileged)
```

```go
func (h *apiHandler) systemLaunch(c *web.Ctx, _ noIn) (launchOut, error) {
	if err := requireJSONOrEmptyBody(c.Request()); err != nil {
		return launchOut{}, err
	}

	privileged := h.Account == nil && auth.IsLauncher(h.LauncherSecret, c.Request().Header.Get(auth.LauncherHeader))

	code, err := h.Launch.Issue(privileged)
	if err != nil {
		return launchOut{}, err
	}

	return launchOut{Code: code, URL: "http://127.0.0.1:" + listenPort(c.Request()) + "/auth/launch?code=" + code}, nil
}
```

- `web/app/http/account_test.go:121`: read this test and `web/e2e/specs/account-first-run.spec.ts` to see how both Go tests and Playwright obtain the first-run session before calling `POST /auth/account/create`.
- `web/app/cli/open.go`, `web/cmd/app/`, and `desktop/shell/`: read how `stuarty serve`, `stuarty open`, and the desktop shell exchange `STUARTY_LAUNCHER_SECRET` / launch codes today.
- SDD contract: `docs/sdds/007-security-model.md` §3.9 item 7 (AGENTS.md rule 4: update the SDD in the same change).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| HTTP account & session tests | `go test -race -count=1 -run 'Account\|Launch\|Session' ./web/app/http/...` | `ok`, exit 0 |
| Auth package tests | `go test -race -count=1 ./web/app/auth/...` | `ok`, exit 0 |
| Security suite | `go test -count=1 -run TestSecurity_ ./...` | `ok`, exit 0 |
| Account E2E spec | `cd web && bunx playwright test -c e2e/playwright.config.ts --project=account` | all pass |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):

- `web/app/auth/` (session/launch setup flag — read `web/app/auth/session.go` and `web/app/auth/launch.go` first)
- `web/app/http/account_controller.go`
- `web/app/http/session.go`
- `web/app/http/api.go`
- `web/app/http/account_test.go`
- `web/app/cli/serve.go` / `web/app/cli/open.go` (if first-run setup link/secret is emitted by `serve` or passed by `open`)
- `desktop/shell/` (only if launcher header wiring needs adjustment — verify `STUARTY_LAUNCHER_SECRET` is already sent on `POST /api/v1/system/launch`)
- `web/e2e/playwright.config.ts` and `web/e2e/specs/account-first-run.spec.ts` (update first-run setup handshake in E2E)
- `docs/sdds/007-security-model.md` (update §3.9 item 7 + threat table)

**Out of scope** (do NOT touch):

- Network-mode user creation (`POST /users`, `STUARTY_ADMIN_EMAIL`).
- Password login (`accountLogin`) or recovery (`accountRecover`) cryptographic logic in `service/account/`.
- Adding any new external dependency.

## Git workflow

- Branch: `fix/security-account-create-boundary` cut from `release/v0.60.0`.
- Commit with Conventional Commits + `Security:` footer, e.g.:
  `fix(security): require launcher or setup-code proof for first-run account creation`
  `Security: blocks local bearer-token agents from creating the workspace account via /api/v1/system/launch (VV-378)`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Audit all callers of `POST /api/v1/system/launch` and `POST /auth/account/create`

1. Read `web/app/auth/launch.go`, `web/app/auth/session.go`, `web/app/cli/open.go`, `web/app/cli/serve.go`, `desktop/shell/launcher.go` (or wherever `STUARTY_LAUNCHER_SECRET` is sent), `web/app/http/account_test.go`, and `web/e2e/specs/account-first-run.spec.ts`.
2. Verify how the two legitimate human onboarding paths reach `/account/create`:
   - **Desktop shell**: spawns the server with `STUARTY_LAUNCHER_SECRET` in env and sends `Stuarty-Launcher: <secret>` on `POST /api/v1/system/launch`.
   - **CLI (`stuarty serve` / `stuarty open`)**: check how `stuarty serve` prints its startup URL or how `stuarty open` obtains a launch URL. Notice that if `stuarty open` only holds `~/.stuarty/token`, ANY local process that can read `~/.stuarty/token` can execute the exact same HTTP request as `stuarty open`!
3. Choose the clean out-of-band proof mechanism:
   - Track a boolean `canCreateAccount` (or `setupVerified`) on the launch code and local session (separate from `privileged` / `Browser`, which MUST remain `false` when `h.Account != nil` per ADR-0052!).
   - Set `canCreateAccount = true` ONLY when either:
     - (a) `POST /api/v1/system/launch` carries a valid `Stuarty-Launcher` header matching `h.LauncherSecret` (desktop shell path — `h.LauncherSecret` is an in-memory/env secret never written to `~/.stuarty/token`), OR
     - (b) The browser redeems a one-time first-run setup launch code generated by `stuarty serve` at startup when `Account.State() == StateNone` and printed ONLY to `serve`'s interactive stdout/stderr (never persisted to disk and never returned by `/api/v1/system/launch` to a bearer-token caller).
   - **Important**: If `stuarty open` needs to open the setup page in a browser when `StateNone`, verify how `stuarty open` interacts with `serve` before deciding; if `stuarty open` must NOT bypass the agent boundary, have `stuarty open` print the hint to use the one-time setup URL printed by `stuarty serve` (or require a one-time setup code passed by the person). If changing `stuarty open` UX breaks existing E2E or CLI expectations, check STOP conditions below.

**Verify**: Design confirmed against every caller of `/api/v1/system/launch` and `/auth/account/create` (grep across repo).

### Step 2: Enforce `canCreateAccount` on launch redemption and `POST /auth/account/create`

1. Extend `auth.Launch` and `auth.Sessions` so a launch code and session record an explicit `canCreateAccount bool` flag (independent of `privileged`), defaulting to `false`.
2. In `web/app/http/session.go`:
   - In `systemLaunch`, compute `launcherOK := auth.IsLauncher(h.LauncherSecret, c.Request().Header.Get(auth.LauncherHeader))`.
   - Issue `canCreateAccount = launcherOK` (so a bare local bearer token WITHOUT `Stuarty-Launcher` gets `canCreateAccount = false`).
   - In `launch`, propagate `canCreateAccount` from `h.Launch.Redeem` into `h.Sessions.CreateWithSetup(privileged, canCreateAccount)` while continuing to force `privileged = false` when `h.Account != nil`.
3. In `web/cmd/app` / `web/app/cli/serve.go` (when `Account != nil && Account.State() == StateNone`):
   - Pre-issue a one-time launch code with `canCreateAccount = true` (valid e.g. 15 minutes or until redeemed) and print the setup URL once to `serve`'s terminal output (and support `STUARTY_LAUNCHER_SECRET` in `web/e2e/playwright.config.ts` for the `account` server so the desktop/launcher setup flow is tested end-to-end).
4. In `web/app/http/account_controller.go:136-145`:
   - Verify the request holds a valid local session cookie whose session has `canCreateAccount == true` (in addition to `caller.Actor == "local:"+auth.UILabel` and `ScopeAdmin`).
   - After `m.Create(...)` succeeds, clear `canCreateAccount` on the session (and invalidate any unused setup launch codes).

**Verify**: `go test -race -count=1 ./web/app/auth/... && go test -race -count=1 -run 'Account|Launch|Session' ./web/app/http/...` -> all pass, exit 0.

### Step 3: Add regression tests and update SDD-007

1. In `web/app/http/account_test.go`:
   - Update legitimate test helpers to obtain their setup session via `Stuarty-Launcher` (or the one-time setup launch code).
   - Add `TestSecurity_T117_UnprivilegedLaunchCannotCreateAccount` proving:
     1. A local bearer token calls `POST /api/v1/system/launch` (without `Stuarty-Launcher`), receives a launch code, and redeems it at `GET /auth/launch` for a `local:ui` session cookie.
     2. Calling `POST /auth/account/create` with that cookie returns HTTP 403 `E_FORBIDDEN`, leaving `Account.State() == StateNone`.
     3. Calling `POST /api/v1/system/launch` WITH valid `Stuarty-Launcher`, redeeming it, and calling `POST /auth/account/create` succeeds and returns the recovery key.
2. Update `web/e2e/playwright.config.ts` (`accountPort` server env) and `web/e2e/specs/account-first-run.spec.ts` so the E2E flow authenticates via the launcher/setup flow.
3. Update `docs/sdds/007-security-model.md` §3.9 item 7 to document the exact proof required by `POST /auth/account/create` (and check `go test ./internal/tools/threatcheck/...` if any threat IDs are added).

**Verify**: `go test -count=1 -run TestSecurity_ ./... && task lint:go && task lint:md` -> all pass, exit 0.

## Test plan

- `TestSecurity_T117_UnprivilegedLaunchCannotCreateAccount` in `web/app/http/account_test.go` (negative test for bearer-token `/system/launch` -> `/auth/launch` -> `/auth/account/create` escalation, plus positive tests for launcher-verified and terminal-setup-code flows).
- Existing `web/app/http/account_test.go`, `web/app/auth/...`, and `web/e2e/specs/account-first-run.spec.ts`.

## Done criteria

- [ ] `go test -race -count=1 -run 'TestSecurity_T117|Account|Launch|Session' ./web/app/http/...` exits 0
- [ ] `go test -race -count=1 ./web/app/auth/...` exits 0
- [ ] `go test -count=1 -run TestSecurity_ ./...` exits 0
- [ ] `cd web && bunx playwright test -c e2e/playwright.config.ts --project=account` exits 0 (when Playwright/browsers are installed)
- [ ] `docs/sdds/007-security-model.md` §3.9 item 7 matches the implemented rule
- [ ] `task lint:go && task lint:md && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `stuarty open` is required by PRD/SDD-007 to create an account purely via `~/.stuarty/token` with no terminal interaction or launcher secret (that would represent a fundamental contradiction with VV-378 because any local agent can read `~/.stuarty/token` — stop and present the trade-off to the author).
- `account-first-run.spec.ts` visits `/` directly without a launch code (check how `account-first-run.spec.ts` currently gets its cookie before modifying it!).
- Changing `auth.Sessions` breaks session serialization or existing contract tests in `web/contract/`.

## Maintenance notes

- Never equate `Actor == "local:ui"` with "proven human browser" — any local bearer-token caller can mint a `local:ui` cookie via `/api/v1/system/launch`. Any future privileged-before-account route must check an out-of-band proof (`canCreateAccount` / `Stuarty-Launcher`), never `Actor` alone.
- Reviewer focus: confirm `privileged` (`Browser: true`) still stays strictly `false` on launch redemption whenever `h.Account != nil` (ADR-0052).
