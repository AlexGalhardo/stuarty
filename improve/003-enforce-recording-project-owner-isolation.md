# Plan 003: Enforce project-owner tenant isolation across the Recording subsystem (SDD-007 T-06)

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- service/recording_ops.go service/recording_agent.go service/recording_upload.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

In network mode, Stuarty enforces multi-user project isolation (SDD-007 threat T-06): a non-admin member may only access projects they own (`visible(c, head.OwnerUserID)`), and unauthorized IDs must return the exact same not-found error as non-existent IDs to prevent enumeration. Currently, the entire Recording subsystem omits `visible(c, head.OwnerUserID)` checks. Any authenticated member in network mode can list, read, inspect pointer/click logs, start/prepare/upload recordings into, mark/propose/reject cuts on, or stop/delete (`os.RemoveAll`) recordings belonging to another user's project.

## Current state

- `service/recording_ops.go:759-778` (`recordingFor`): checks `s.covers(ctx, c, r.ProjectID)` (project-limited API key check only) and `recording.VisibleTo`, but never loads the owning production to check `visible(c, head.OwnerUserID)`:

```go
func (s *svc) recordingFor(ctx context.Context, c Caller, id string) (recordings.Row, error) {
	if !timeline.ValidID("rec", id) {
		return recordings.Row{}, recordingNotFound()
	}

	r, err := s.store.Recordings().Get(ctx, id)
	if err != nil {
		if errors.Is(err, recordings.ErrNotFound) {
			return r, recordingNotFound()
		}

		return r, asError(err)
	}

	if !s.covers(ctx, c, r.ProjectID) || !recording.VisibleTo(recording.Review(r.Review), isPerson(c)) {
		return r, recordingNotFound()
	}

	return r, nil
}
```

- `service/recording_ops.go:1102-1122` (`RecordingList`): calls `s.authorizeProject` (scope + project-limited key only) and returns `ListByProject(ctx, req.ProjectID)` without loading the project or checking `visible(c, head.OwnerUserID)`.
- `service/recording_ops.go:407-413` (`RecordingStart`), `service/recording_agent.go:157-163` (`RecordingPrepare`), and `service/recording_upload.go:232-238` (`RecordingUploadBegin`): all load `Productions().Get(ctx, req.ProjectID)` and check `gerr != nil || head.State == ProjectTrashed`, omitting `!visible(c, head.OwnerUserID)`.
- Exemplar pattern in this repo: inspect `service/chat_ops.go:50-75` and `service/keys_ops.go:130-150` to match the exact `visible(c, head.OwnerUserID)` and `projectNotFound()` conventions.
- Note on local mode vs network mode: in local mode, `Caller.OwnerUserID` is `""` (or `visible` returns `true` for admins and local callers — verify `visible(...)` definition in `service/` before editing).

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Recording servicetests | `go test -race -count=1 -run 'Recording' ./web/app/servicetest/` | `ok`, exit 0 |
| Security test suite | `go test -count=1 -run TestSecurity_ ./...` | `ok`, exit 0 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify or create):

- `service/recording_ops.go`
- `service/recording_agent.go`
- `service/recording_upload.go`
- `web/app/servicetest/security_recording_test.go` (create — or add to an existing `security_*_test.go` in `web/app/servicetest/` matching repo naming)

**Out of scope** (do NOT touch):

- Recording state-machine concurrency bugs (`uploadDiscard`, `settleUpload`, `RecordingCancel` `CanDelete` inside tx — handled in `improve/008-fix-recording-agenthost-capture-and-cache-concurrency-bugs.md`).
- HTTP route definitions or UI code (authorization lives strictly in `service/` per AGENTS.md rule 1).

## Git workflow

- Branch: `fix/security-recording-tenant-isolation` cut from `release/v0.60.0`.
- Commit message must follow Conventional Commits with `Security:` footer, e.g.:
  `fix(security): enforce project ownership checks across recording operations`
  with footer `Security: prevents cross-user recording access and deletion in network mode (SDD-007 T-06)`.
- Do NOT push or merge to `main`.

## Steps

### Step 1: Add a shared `recordingProject` ownership helper and enforce it in `recordingFor`

1. Read `visible(...)` in `service/` (grep `func visible(` under `service/`) to confirm its exact signature and behavior for local callers, admins, and members.
2. Read `service/recording_ops.go:395-430`, `service/recording_agent.go:145-185`, and `service/recording_upload.go:220-260` to see every place a recording operation resolves `ProjectID`.
3. Add a helper on `*svc` in `service/recording_ops.go`:

```go
// recordingProject loads a non-trashed production visible to c, returning projectNotFound when missing,
// trashed, or owned by another user (SDD-007 T-06).
func (s *svc) recordingProject(ctx context.Context, c Caller, projectID string) (ProductionRow, error)
```

   Also add a read-only visibility helper `s.recordingProjectVisible(ctx, c, projectID)` (allowing trashed projects if `RecordingGet`/`RecordingList` permit them — check existing tests to confirm whether `RecordingGet`/`RecordingList` allow trashed projects, and preserve that behavior for the owner while refusing other users).
4. In `recordingFor` (`service/recording_ops.go:759-778`): after verifying `s.covers(ctx, c, r.ProjectID)` and `recording.VisibleTo(...)`, load the project via `s.store.Productions().Get(ctx, r.ProjectID)` and return `recordingNotFound()` if missing or `!visible(c, head.OwnerUserID)`. Never distinguish "not found" from "owned by another user" (preserve T-06/T-20 non-enumeration).

**Verify**: `go test -race -count=1 -run 'Recording' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 2: Enforce `visible(c, head.OwnerUserID)` in `RecordingList`, `RecordingStart`, `RecordingPrepare`, and `RecordingUploadBegin`

1. `RecordingList` (`service/recording_ops.go:1102-1122`): after `s.authorizeProject`, verify the project exists and is visible to `c` (`visible(c, head.OwnerUserID)`); return `projectNotFound(req.ProjectID)` (matching `ProjectGet` / other `List` methods in `service/`) when missing or owned by another user.
2. `RecordingStart` (`service/recording_ops.go:407-413`): check `!visible(c, head.OwnerUserID)` alongside `gerr != nil || head.State == ProjectTrashed`.
3. `RecordingPrepare` (`service/recording_agent.go:157-163`): check `!visible(c, head.OwnerUserID)` alongside `gerr != nil || head.State == ProjectTrashed` (before validating `req.ScriptID`, so another user's `script_id` cannot be probed).
4. `RecordingUploadBegin` (`service/recording_upload.go:232-238`): check `!visible(c, head.OwnerUserID)` alongside `gerr != nil || head.State == ProjectTrashed`.
5. Audit ALL remaining exported `Recording*` methods in `service/recording_*.go` (grep `func (s *svc) Recording` across `service/`) to confirm every single one goes through `recordingFor` or `recordingProject`.

**Verify**: `go test -race -count=1 -run 'Recording' ./web/app/servicetest/` -> `PASS`, exit 0.

### Step 3: Add `TestSecurity_T06_RecordingTenantIsolation` tests

1. Read an existing `TestSecurity_T06_*` test in `web/app/servicetest/` (grep `TestSecurity_T06` in `web/app/servicetest/`) to follow its exact setup for two non-admin network users (`alice` and `bob`) and an `admin` user.
2. Create `web/app/servicetest/security_recording_test.go` testing:
   - `alice` creates a project and a recording (both prepared/uploaded and active/pending states).
   - `bob` (another non-admin member, with `Browser: true` person session) is refused with `CodeNotFound` (identical to a non-existent ID) on: `RecordingList`, `RecordingGet`, `RecordingEvents`, `RecordingStart`, `RecordingPrepare`, `RecordingUploadBegin`, `RecordingUploadChunk`, `RecordingUploadFinish`, `RecordingMark`, `RecordingProposeCuts`, `RecordingProposalDecide`, `RecordingStop`, `RecordingPause`, `RecordingResume`, `RecordingCancel`, `RecordingConfirm`, `RecordingDismiss`.
   - `bob`'s `RecordingCancel` does NOT delete `alice`'s recording directory on disk.
   - `admin` and `alice` continue to succeed normally.

**Verify**: `go test -race -count=1 -run 'TestSecurity_T06_Recording|Recording' ./web/app/servicetest/ && go test -count=1 -run TestSecurity_ ./...` -> all pass, exit 0.

## Test plan

- New `TestSecurity_T06_RecordingTenantIsolation` in `web/app/servicetest/security_recording_test.go` covering every `Recording*` service method across owner, non-owner member, and admin callers, plus verifying non-enumeration (same error code/message as unknown ID) and filesystem preservation on rejected `RecordingCancel`.
- Full security suite: `go test -count=1 -run TestSecurity_ ./...`.

## Done criteria

- [ ] Every exported `Recording*` method on `*svc` enforces `visible(c, head.OwnerUserID)`
- [ ] `go test -race -count=1 -run 'TestSecurity_T06_Recording' ./web/app/servicetest/` exits 0
- [ ] `go test -race -count=1 -run 'Recording' ./web/app/servicetest/` exits 0
- [ ] `go test -count=1 -run TestSecurity_ ./...` exits 0
- [ ] `task lint:go && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any existing recording test creates a recording whose `ProjectID` is actually a timeline ID rather than a production ID (check `Productions().Get` vs `ProjectOfTimeline` before rejecting — if timeline IDs are accepted anywhere in recording APIs, stop and clarify).
- Any existing test expects `RecordingList` on an unknown project ID to return an empty list instead of `CodeNotFound` (if so, verify whether other list endpoints return empty vs `CodeNotFound` and preserve owner behavior while hiding other users' projects).

## Maintenance notes

- Every new project-scoped domain added to `service/` must enforce `visible(c, head.OwnerUserID)` in its `Get`/`List`/mutation helpers — `s.covers(...)` only checks project-limited API keys, NOT multi-user ownership.
- Reviewer focus: verify `RecordingPrepare` checks project ownership BEFORE inspecting `req.ScriptID` so script IDs cannot be probed across tenants.
