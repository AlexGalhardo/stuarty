# Plan 022: Security acceptance on three operating systems, the long fuzz campaign and the independent review

This plan answers whether the security model of SDD-007 holds on Linux, macOS and Windows: the security tests pass everywhere, the `no-egress` job sees no non-loopback connection, every fuzz target runs 30 minutes without a crash, all 30 acceptance checks of section 15 pass, and an independent review of the 1.0 candidate leaves no unresolved high finding.

- Owner: senior-c (coordination); every area owner fixes findings in their own code; the orchestrator schedules the independent reviewer
- Effort: M
- Depends on: CI runners for Linux and macOS (the pull-request workflows are paused); plan 027 for the runs against the own FFmpeg build
- Origin: EPIC-06 (P1-SEC-001): TASK-0031; EPIC-07 (P2-SEC-009): TASK-0035; EPIC-30 (P4-RND-002): TASK-0143; EPIC-32 (P4-SEC-001): TASK-0149, TASK-0151, TASK-0154, TASK-0155
- Status: BLOCKED (needs real CI runs on Linux and macOS; the former status of EPIC-30 says it needs the idle reference machine and the three runners)
- Written against: 8787714a

## 1. Why

Release 1.0 exit criterion 3 requires the SDD-007 acceptance table to pass on all three systems and an independent review. Today `threatcheck` and `TestSecurity_` pass only on Windows (115 tests); Linux and macOS were not run since CI was paused.

## 2. Current state (drift check)

```text
go run ./internal/tools/threatcheck -phase P1
go test ./internal/tools/threatcheck/...
go test -run TestSecurity_ ./...
ls docs/security-review-1.0.md
git log --oneline -n 10 -- internal/tools/threatcheck scripts/no-egress.sh
```

Already true (from the former card statuses):

- `TestSecurity_T10_DefaultRunOpensNoOutboundConnection` exists; the `No egress` job traces serve, CLI and MCP under strace (job 133561985720 ok); the scanner is tested on fixtures and the IPv6 loopback bug is fixed (1e2d1d94 + 554f899b, release/v0.22.0). Docker run: 13 loopback calls, no other address.
- The dedicated `Security tests` job on three operating systems exists (c5bb5a83, release/v0.22.0); actionlint ok; local Windows run 2.5 min, 0 failures.
- `threatcheck` rejects a skip without a platform or `// skip-env` (e3f1cf19); 12 skips are marked.
- Hostile corpus extended (2cb1848a, release/v0.22.0): 18 new argv cases (56), 3 hostile samples (29), demuxer and filter lists from the pinned build with a test. The FFmpeg freshness check says `behind=false`, n9.0.2, 16 days old (limit 90 days, until 2027-01-29).
- 30-minute fuzz runs exist on Windows for 8 targets; `FuzzHostCheck` was added later and has not run 30 minutes. SDD-007 section 15.1 records 30 lines but not 30 of 30 passing (items 2, 5, 15, 16, 25, 29, 30 partial; items 5 (Unix mode), 15 (RLIMIT part) and 25 (strace job) not run on Windows).
- `docs/security-review-1.0.md` exists (21d57def): 0 critical, 0 high, 1 medium, 2 low, 2 info; SR1-01 and SR1-03 are with senior-c; SR1-05 was fixed in e265dd68. SR1-02 (the font parse deadline does not interrupt the goroutine) is deferred to the Rust port of text shaping, which belongs to the renderer epics.

## 3. Steps

### Step 1: Run the security tests and the no-egress job on the three systems

Owner: senior-c. Files: none new (findings become fixes in the owning folders).

Acceptance:

- [ ] `go test -run TestSecurity_ ./...` passes on Linux, macOS and Windows.
- [ ] `go run ./internal/tools/threatcheck` exits 0 on the three runners, with no skipped test that lacks the platform in its name.
- [ ] The `no-egress` job (Linux, `strace -f -e trace=connect,sendto,sendmsg`) finds no `connect` to a non-loopback address in the reference scenario of the ALPHA (check 25 of SDD-007 section 15) and in the BETA scenario (bundle import and export, OTIO import, font import, export formats, brand kit, helper installs).
- [ ] The items 3 to 10, 13 to 15, 18, 19, 22 to 25, 27 and 28 of SDD-007 section 15 hold, each by its named test.

Commit: `test(service): <english summary>` per new or changed test, or none if only runs.

### Step 2: Re-run the FFmpeg threat tests and the engine security subset

Owner: senior-b (runs), senior-c (reviews). Files: `engine/ffmpeg/testdata/hostile/`, `engine/ffmpeg/testdata/argv/`, `engine/ffmpeg/manifest.json`.

Acceptance:

- [ ] `go test -run 'TestSecurity_T1[456]|TestSecurity_T2[17]' ./engine/...` passes on the three runners.
- [ ] `vukem-video doctor --verify` on the release candidate archive reports every SHA-256 equal to the embedded manifest (output pasted in the pull request).

### Step 3: Fuzz every target for 30 minutes

Owner: senior-c. Files: none new.

Fuzz each target of SDD-007 section 12 for 30 minutes (starting value, confirmed by measuring): `FuzzCanonicalizeImportPath`, `FuzzSniffMedia`, `FuzzUntrustedClean`, `FuzzHostCheck`, `FuzzFontHeader`, `FuzzTemplateArchive`, `FuzzFFArgsCheck`, plus the BETA targets (`FuzzImport` of OTIO and the bundle reader).

Acceptance:

- [ ] Each fuzz target ran 30 minutes without a crash, with the durations listed.
- [ ] The three fuzz targets of the engine find no crash in 30 minutes.

### Step 4: Triage the independent review

Owner: senior-c; reviewer: a fresh review session or an outside person named by the author, who wrote no code. Files: `docs/security-review-1.0.md`.

Re-run the review against the release candidate commit with SDD-007 and only the repository; triage every finding (SR1-01 and SR1-03 are open with senior-c).

Acceptance:

- [ ] `docs/security-review-1.0.md` exists, names the reviewer and the reviewed commit, and lists no unresolved finding of high severity; each resolved finding points to the pull request that fixed it.

### Step 5: Record the acceptance table and close the documents

Owner: senior-c. Files: `docs/sdds/007-security-model.md`, `CHANGELOG.md`.

Acceptance:

- [ ] All 30 checks of SDD-007 section 15 have a result recorded in the pull request, 30 of 30 passing.
- [ ] The docs are updated: `docs/sdds/007-security-model.md` section 14 (residual risks at 1.0) and Appendix A status, and `CHANGELOG.md`; `SECURITY.md` belongs to the release plan, and the verification commands of SDD-007 section 13.2 are those of `scripts/release-verify.sh`.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- Fixing findings in code owned by another area (they become fixes in the owning folder by the area owner).
- The Rust text shaping port that closes SR1-02.
- Writing `SECURITY.md`.

## 5. STOP conditions

- A finding of high severity cannot be fixed in time: stop and report to the orchestrator; the release is blocked.
- No independent reviewer is available: stop and ask the orchestrator to name one with the author.
- A check of section 15 passes only with a skipped test: stop; a skip without its platform in the name is a failure.

## 6. Maintenance

Re-run the whole table at every release candidate and after any change to the files named in the SDD-007 threat table. A new fuzz target joins section 12 of SDD-007 and this campaign in the same pull request.
