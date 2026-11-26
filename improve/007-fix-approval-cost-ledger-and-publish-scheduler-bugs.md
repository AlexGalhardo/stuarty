# Plan 007: Fix approval cost-ledger, publish scheduler lease-loss, and OAuth refresh-rotation bugs

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- service/approval_jobs.go service/jobrun.go service/publish/scheduler.go service/publish/manager.go service/social/extras.go service/social/tiktok/auth.go service/social/x/auth.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

Four verified backend correctness bugs affect financial safety (daily AI/publishing cost ceilings) and scheduled publishing reliability:

1. **Transient DB errors bypass cost ceilings and drop spent cost (`service/approval_jobs.go`)**: `ensureReserved`, `recordInterrupted`, and `recordActual` swallow ANY `err != nil` from `RequestOfJob` instead of only `approval.ErrNotFound`, letting paid jobs run unbudgeted or losing real spend records on SQLite busy errors.
2. **Early exits in `RunJob` leak daily cost reservations (`service/jobrun.go`)**: if the studio locks (`job.InputLocked`), a handler panics, or post-claim `Jobs().Get` fails, `RunJob` marks the job `JobFailed` without calling `s.finishCost`, permanently locking that estimate out of the user's daily ceiling for the rest of the UTC day.
3. **Lease loss permanently fails scheduled posts (`service/publish/scheduler.go`)**: `Scheduler.send` checks `ctx.Err() != nil` (parent context) instead of `sendCtx.Err() != nil` after `m.Send` returns, so lease-heartbeat cancellation wraps `context.Canceled` into a non-transient `"internal"` error and permanently fails the target (or clobbers a new owner's lease) instead of leaving it for `ActionResume`.
4. **3-way concurrent TikTok/X publishes can reuse a spent OAuth refresh token (`service/social/extras.go`)**: `Rotation.last` stores only one `{from, to}` pair per connection, so with `DefaultParallel = 3`, a 3rd concurrent send holding `R1` after `R0 -> R1 -> R2` misses `Current` (`R0 != R1`) and presents the already-spent `R1`, risking OAuth token-family revocation.

## Current state

- `service/approval_jobs.go:381-384, 468, 491-494, 505-508`:

```go
		req, err := st.RequestOfJob(ctx, job.ID)
		if err != nil || req.Estimate.Micros == 0 {
			return nil //nolint:nilerr // a job with no request or no cost reserves nothing
		}
```

  Same `if err != nil { return nil //nolint:nilerr }` pattern in `recordInterrupted` (line 491) and `recordActual` (line 505), while `finishCost` (line 468) discards `_ = s.store.Write(...)`. Note `web/app/store/approvals.go:87-98` specifically returns `approval.ErrNotFound` for `sql.ErrNoRows`.

- `service/jobrun.go:174-206, 254-265`: panic recovery (line 182), `gerr != nil` (line 192), and `job.InputLocked` (line 205) call `s.finishFailed(...)` and return without calling `s.finishCost`.
- `service/publish/scheduler.go:284-319`:

```go
	sent, err := s.m.Send(sendCtx, t, nil)

	close(done)

	// A shutdown interrupts the send: the target stays claimed, its lease runs out and the next process
	// resumes it with the same upload key.
	if ctx.Err() != nil {
		return
	}
```

  Meanwhile `heartbeat` (lines 340–351) cancels `sendCtx` (NOT `ctx`) when lease renewal fails.

- `service/social/extras.go:88-94, 119-139`:

```go
type Rotation struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	last  map[string]rotated
}

type rotated struct{ from, to string }
```

  `Current` returns `got.to` ONLY when `got.from == inSecret`.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Publish scheduler tests | `go test -race -count=1 ./service/publish/...` | `ok`, exit 0 |
| Social rotation tests | `go test -race -count=1 ./service/social/...` | `ok`, exit 0 |
| Approval & job tests | `go test -race -count=1 -run 'Approval\|Cost\|Job\|Locked' ./web/app/servicetest/` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `service/approval_jobs.go`
- `service/jobrun.go`
- `service/publish/scheduler.go`
- `service/publish/scheduler_test.go`
- `service/publish/manager.go`
- `service/social/extras.go`
- `service/social/extras_test.go` (or existing test file in `service/social/`)
- `web/app/servicetest/approval_cost_bugs_test.go` (create, or add to existing `approval_*_test.go` in `web/app/servicetest/`)

**Out of scope** (do NOT touch):

- `service/recording_*.go`, `service/agenthost/`, `engine/` (handled in Plan 008).
- Approval rules/ceiling evaluation math in `core/approval/`.

## Git workflow

- Branch: `fix/cost-ledger-scheduler-and-rotation-bugs` cut from `release/v0.60.0`.
- Commit per logical fix (Conventional Commits), e.g.:
  `fix(service): propagate store errors in approval cost checks and settle reservations on RunJob early exits`
  `fix(service): leave publish target for resume when sendCtx is cancelled on lease loss`
  `fix(service): retain chained refresh-token rotations in social.Rotation`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Distinguish `approval.ErrNotFound` from store errors in `service/approval_jobs.go`

1. In `ensureReserved` (`service/approval_jobs.go:381-384`):
   - Check `if errors.Is(err, approval.ErrNotFound) { return nil }`, `if err != nil { return err }`, and `if req.Estimate.Micros == 0 { return nil }`.
   - Remove the `//nolint:nilerr` comment (recall `.golangci.yml` enables `nolintlint` requiring no unused nolint directives!).
2. Apply the exact same `errors.Is(err, approval.ErrNotFound)` distinction in `recordInterrupted` (lines 491–494) and `recordActual` (lines 505–508), removing their `//nolint:nilerr` directives.
3. In `finishCost` (`service/approval_jobs.go:463-485`): log any error from `s.store.Jobs().Get` or `s.store.Write` via `s.logJob(ctx, slog.LevelError, ...)` instead of silently discarding it.

**Verify**: `go test -race -count=1 -run 'Approval|Cost' ./web/app/servicetest/ && task lint:go` -> `PASS`, exit 0.

### Step 2: Settle approval cost reservations on early exits in `service/jobrun.go`

1. In `RunJob` (`service/jobrun.go:174-206`):
   - In the `job.InputLocked` branch (lines 201–206): after `ferr := s.finishFailed(fctx, &job, JobFailed, lockedError())`, call `if s.kindOf(job.Kind) != nil { s.finishCost(fctx, &job, nil) }` before returning `ferr`.
   - In the post-claim `gerr != nil` branch (lines 188–195): after `s.finishFailed(...)`, call `if s.kindOf(job.Kind) != nil { s.finishCost(fctx, &job, nil) }`.
   - In the panic-recovery `defer` (lines 174–185): after `s.finishFailed(...)`, call `if s.kindOf(job.Kind) != nil { s.finishCost(fctx, &job, nil) }` (guarding against a second panic if needed, or keeping `finishCost` safe).
   - Verify whether `job.Kind` is already populated on `job` before line 187 (read lines 135–186 of `service/jobrun.go` to confirm!).

**Verify**: `go test -race -count=1 -run 'Approval|Cost|Job|Locked' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 3: Check `sendCtx.Err()` on lease loss in `service/publish/scheduler.go`

1. Read `service/publish/scheduler.go:270-355` and `service/publish/manager.go:1295-1330` (`Finish`).
2. At `service/publish/scheduler.go:298`, change `if ctx.Err() != nil { return }` to `if sendCtx.Err() != nil { return }` (and update the comment above it to explain both shutdown and lease loss leave the target for `ActionResume`, matching line 288).
3. In `service/publish/manager.go:1306-1309`, inspect whether `Finish` should also verify `cur.LeaseUntil > 0 && !s.clock.Now().After(...)` or whether `LeaseOwner == owner` is intentional for delayed finishes — read `service/publish/manager_test.go` and `scheduler_test.go` first so we do not break legitimate slow-finish tests; if `sendCtx.Err() != nil` already prevents calling `Finish` on lease loss, add a unit test in `scheduler_test.go` proving a lost lease leaves the target claimed/resumable rather than `TargetFailed`.

**Verify**: `go test -race -count=1 ./service/publish/...` -> `PASS`, exit 0.

### Step 4: Retain chained refresh-token rotations in `service/social/extras.go`

1. Read `service/social/extras.go:83-139`, `service/social/tiktok/auth.go:25-55`, and `service/social/x/auth.go:35-65`.
2. Update `Rotation` in `service/social/extras.go`:
   - For each `connID`, track `newest string` and a bounded `seen map[string]struct{}` (or slice capped at e.g. 16 prior tokens so memory stays bounded) of superseded `from` tokens that rotated into `newest`.
   - When `Remember(connID, from, to)` is called: if `from == "" || to == "" || from == to`, return; record `from` (and the previous `newest`, if different from `to`) into `seen`, and set `newest = to` (removing `to` from `seen` in case of reuse).
   - When `Current(connID, inSecret)` is called: if `inSecret` is in `seen` (any token in the superseded chain), return `newest`; otherwise return `inSecret`.
   - Confirm: in the 3-way scenario (`R0 -> R1`, then `R0 -> R2`, then Send C arrives with `R1`), `Remember(id, R0, R1)` records `seen={R0}, newest=R1`; `Remember(id, R0, R2)` records `seen={R0, R1}, newest=R2`; `Current(id, R1)` finds `R1` in `seen` and returns `R2`!
3. Add a unit test in `service/social/extras_test.go` exercising the exact 3-way `R0 -> R1 -> R2` concurrent-send chain.

**Verify**: `go test -race -count=1 ./service/social/...` -> `PASS`, exit 0.

### Step 5: Add regression tests for approval cost errors and `RunJob` reservation release

1. Add tests in `web/app/servicetest/approval_cost_bugs_test.go`:
   - **Locked-studio reservation release**: approve a paid generation job (reserving its estimate against a tight daily ceiling), lock the studio before running the job so `RunJob` hits `job.InputLocked`, verify the job fails with `E_LOCKED`, unlock the studio, and assert the daily cost reservation was released (a second job of the same estimate now fits within the daily ceiling).
   - **Panic reservation release**: trigger a panic in a test approvable handler, verify the job fails and its reservation is released.
   - **Store error propagation**: verify `ensureReserved` / `recordActual` returns a non-`ErrNotFound` error rather than `nil` (either via a focused unit test in `service/` with a failing `approval.Store` stub, or in `servicetest`).

**Verify**: `go test -race -count=1 ./service/publish/... ./service/social/... && go test -race -count=1 -run 'Approval|Cost|Locked' ./web/app/servicetest/ && task lint:go && task vet` -> all exit 0.

## Test plan

- `service/publish/scheduler_test.go`: lease-loss cancellation test proving `Send` cancellation from `heartbeat` does NOT call `Finish` with `TargetFailed` and leaves the target resumable.
- `service/social/extras_test.go`: 3-way refresh chain test (`R0 -> R1 -> R2`, `Current(id, R1) == R2`), plus cap-bounding test.
- `web/app/servicetest/approval_cost_bugs_test.go` + `service/` unit test: `InputLocked` reservation release, panic reservation release, and non-`ErrNotFound` error propagation in `ensureReserved`/`recordActual`/`recordInterrupted`.

## Done criteria

- [ ] `grep -n 'nolint:nilerr' service/approval_jobs.go` returns no matches
- [ ] `go test -race -count=1 ./service/publish/...` exits 0 including lease-loss test
- [ ] `go test -race -count=1 ./service/social/...` exits 0 including 3-way rotation test
- [ ] `go test -race -count=1 -run 'Approval|Cost|Locked' ./web/app/servicetest/` exits 0 including new reservation-release tests
- [ ] `task lint:go && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `job` before `s.store.Jobs().Get(ctx, jobID)` at `service/jobrun.go:187` does not yet have `job.Kind` populated (inspect lines 135–175 of `service/jobrun.go`; if `job.Kind` is only populated after `Get`, handle the `gerr != nil` path carefully without skipping release).
- Any existing `scheduler_test.go` test explicitly expects lease-loss cancellation to mark a target `TargetFailed` — stop and report the test name and assertion.

## Maintenance notes

- Never use `//nolint:nilerr` on store lookups that distinguish `ErrNotFound` from transient DB errors; always branch explicitly on `errors.Is(err, ...ErrNotFound)`.
- Reviewer focus: confirm `Rotation` in `service/social/extras.go` stays bounded in memory (never grows unbounded across months of uptime) and never logs or persists raw refresh token values.
