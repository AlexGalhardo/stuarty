# Plan 031: Cut release v1.0.0 and audit the supply chain

This plan answers how the 1.0.0 tag is prepared, cut and verified: the preflight passes with strict translations, the release audit reports no GPL, AGPL, SSPL or unknown licence, the tag push alone produces the release, and the public verification scripts succeed on the published artifacts. When delivered, `gh release view v1.0.0` shows the changelog section as its body, both registries serve `1.0.0`, `1.0` and `latest` with one digest, and no `blocker` issue is open.

- Owner: senior-d (preflight, audit, verification); orchestrator (confirms the version with the author and cuts the tag)
- Effort: M
- Depends on: plans 021, 022, 023, 024, 025, 028, 029 and 030 (the 1.0 exit criteria); the author confirming the version and scope (SDD-009 section 7)
- Origin: EPIC-37 (P4-REL-003): TASK-0174, TASK-0175, TASK-0177
- Status: BLOCKED (release 1.0: the author confirms the version and scope, SDD-009 section 7)
- Written against: 8787714a

## 1. Why

The last audit of release v0.20.0 found 0 licence findings and a largest asset of 21 MB, but two checks fail today: the median CI time is 15.5 min (maximum 34.8) against NFR-042, and NFR-045 fails. The v1.0.0 tag does not exist, so every tag-time check is untested.

## 2. Current state (drift check)

```text
git tag --list "v1.*"
task release:preflight
bash scripts/release-audit.sh --help
gh issue list --label blocker --state open
```

Already true (from the former card statuses):

- Audit of v0.20.0: 0 findings (the 2 `.exe` files were not inspected); the largest asset is 21 MB, below 60 MB.
- `task release:preflight` needs Docker and has not been run for 1.0; `CHANGELOG.md` is written by the release tool at the tag.
- No 1.0 contract snapshot difference is recorded.

## 3. Steps

### Step 1: Preflight and confirmation

Owner: senior-d; orchestrator confirms with the author. Files: `CHANGELOG.md`, `scripts/release-audit.sh`, `.github/workflows/release.yml`.

Run `task release:preflight` with `--strict` translations; the orchestrator confirms the version with the author and then runs the procedure of SDD-009 section 7.

Commit: `ci(cli): <english summary>` for any fix the preflight demands.

Acceptance:

- [ ] `task release:preflight` exits 0, with a test covering any change (fails before, passes after).
- [ ] On the commit that receives the tag, `gh issue list --label blocker --state open` returns nothing (1.0 exit criterion 1).

### Step 2: Cut the tag

Owner: orchestrator. Files: none.

Acceptance:

- [ ] The release comes out of a single tag push with no other manual step (NFR-044); `git cat-file -t v1.0.0` prints `tag`.

### Step 3: Verify after the tag

Owner: senior-d. Files: `scripts/release-verify.sh`, `scripts/desktop-verify.sh`.

After the tag run `scripts/release-verify.sh` and `scripts/desktop-verify.sh`; make the packages public if needed.

Commit: `ci(cli): <english summary>` if a script needs a fix.

Acceptance:

- [ ] Both scripts succeed on the published artifacts; a test covers any script change.
- [ ] `gh release view v1.0.0 --json name,body,isPrerelease,assets` returns `name` `v1.0.0`, a body equal to the changelog section, `isPrerelease` false; both registries serve `1.0.0`, `1.0` and `latest` with a single digest.

### Step 4: Audit licences, size, CI time and operating systems

Owner: senior-d. Files: `scripts/release-audit.sh`, `CHANGELOG.md`.

Acceptance:

- [ ] `scripts/release-audit.sh` reports no GPL, AGPL, SSPL or unknown licence, and every artifact contains `THIRD_PARTY_NOTICES.md` (NFR-043, RF-215).
- [ ] The `linux_amd64` archive is under 60 MiB without FFmpeg (NFR-041).
- [ ] The median CI time of the last 10 pull requests is under 15 minutes and none exceeds 20 (NFR-042).
- [ ] Windows 11, macOS and Linux were green on all those pull requests (NFR-045).
- [ ] `task release:preflight` exits 0; `CHANGELOG.md` is updated by the release tool; `task lint` reports no findings.

## 4. Out of scope

- Any feature or fix not required by an exit criterion.
- The security review itself (plan 022) and the site timing (plan 024).
- Moving the release to a minor-per-90-commits cadence.

## 5. STOP conditions

- The author has not confirmed the version and scope: stop; do not tag.
- NFR-042 or NFR-045 fails: stop; the plan 030 evidence is incomplete.
- Any licence finding or an open `blocker` issue: stop; the tag is not cut.
- A tag push needs a manual extra step: stop and record it as a defect of the release pipeline.

## 6. Maintenance

Every later release repeats steps 1, 3 and 4. The audit tool's allowlist changes only with `THIRD_PARTY_NOTICES.md` and ADR-0025 in the same pull request.
