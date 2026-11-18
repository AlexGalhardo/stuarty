# Plan 027: Adopt the own FFmpeg build in the engine, the image, the notices and the security tests

This plan answers how the product switches from the BtbN FFmpeg to the own LGPL build: the engine manifest holds the version, size and SHA-256 per platform, the image and CI install it, the notices say OpenH264 is not distributed, and the security and golden suites pass against it on three runners. When delivered, the image smoke test, the notices check, the PSNR comparison and the `-race` suites pass with the own build.

- Owner: senior-b (engine manifest, goldens, PSNR); senior-c (security tests); senior-d (image, notices, CI)
- Effort: L
- Depends on: plan 026; an author decision to allow the OpenH264 download and the regeneration of the 44 golden frames
- Origin: EPIC-16 (P3-RND-001): TASK-0076, TASK-0078; EPIC-17 (P3-SEC-002): TASK-0081, TASK-0083; EPIC-25 (P3-REL-002): TASK-0114, TASK-0115, TASK-0116, TASK-0117, TASK-0118
- Status: BLOCKED (done on branch `task/v025-ffmpeg-own` but outside the release until the author allows the OpenH264 download and the goldens are regenerated)
- Written against: 8787714a

## 1. Why

ADR-0011 declares honestly that no own build exists yet, so its open point is not closed. Until the product ships and tests against the own build, T14, T15, T26 and T27 are proven only against BtbN, and the Dockerfile and the notices still describe BtbN and a compiled-in OpenH264.

## 2. Current state (drift check)

```text
cat .config/ffmpeg.env
task notices:check
task image:smoke
go test ./engine/ffmpeg/... ./engine/export/...
git log --oneline -n 10 task/v025-ffmpeg-own
```

Already true (from the former card statuses), all on branch `task/v025-ffmpeg-own` (2026-11-17, pin `ffmpeg-9.0.2-2`, senior-d): the engine manifest update, the Dockerfile and CI install, the image documentation files, the notices rows, the updated smoke test and the macOS CI job; `notices -check` ok, T14/T15 ok, image smoke ok. Outside the release until the author allows the OpenH264 download and the goldens are regenerated.

Not yet true: no own build existed on the three runners when audited; `TestOwnBuildHasNoGPLFlags` ran on a fake of the own build; `TestPSNRAgainstPinned` did not exist; `TestNoOutboundByDefault` was not found by that name; the `no-egress` job had never run before the release v0.22.0.

## 3. Steps

### Step 1: Update the engine manifest and re-run the golden suite

Owner: senior-b. Files: `engine/ffmpeg/flavour.go`, `engine/ffmpeg/openh264.go`, `engine/ffmpeg/manifest.json`, `engine/ffmpeg/install_openh264.go`, `engine/ffmpeg/testdata/builds/`.

Update the FFmpeg manifest to the own build's version, size and SHA-256 per platform, and re-run the ALPHA golden suite with it (regenerate the 44 goldens that change with OpenH264 2.6.0, after the author allows it).

Commit: `feat(engine): pin the own FFmpeg build per platform`.

Acceptance:

- [ ] `go test ./engine/ffmpeg/testdata/builds/...` passes, with a test that fails before and passes after.

### Step 2: Prove the engine against the own build on three runners

Owner: senior-b. Files: the same as step 1, `engine/export/`.

Commit: `test(engine): <english summary>` or none if only runs.

Acceptance:

- [ ] `go test ./engine/ffmpeg/... ./engine/export/...` passes with the own build on the three runners; the composited frames equal the stored references; the exported file passes verification (SDD-004 section 13 check 3).
- [ ] The own build's configure line contains no `--enable-gpl`, no `--enable-nonfree` and no `--enable-libopenh264` (`TestOwnBuildHasNoGPLFlags`, NFR-043).
- [ ] `go test ./engine/export/ -run TestPSNRAgainstPinned` reports PSNR of at least 50 dB per frame between the own build and the previous one on the golden cases (reported, not blocking, as in SDD-004 section 11).

### Step 3: Install the own build in the Dockerfile and CI

Owner: senior-d. Files: `.config/docker/`, `scripts/install-ffmpeg.sh`, `.github/workflows/ci.yml`.

The Dockerfile and CI install the own build; the image writes `/usr/share/doc/vukem-video/ffmpeg/{LICENSE.txt,configure.txt,SOURCE.txt}` from the new build.

Commit: `ci(cli): install the own FFmpeg build in the image and CI`.

Acceptance:

- [ ] `go test ./.config/docker/... ./licenses/...` passes, with a test that fails before and passes after.

### Step 4: Update the third-party notices

Owner: senior-d. Files: `.config/third-party.json`, `THIRD_PARTY_NOTICES.md`, `licenses/`.

Add the new FFmpeg row; change the OpenH264 row to "not distributed; downloaded from Cisco on an explicit user step"; regenerate with `task notices`.

Commit: `ci(cli): record the own FFmpeg build and the OpenH264 download in the notices`.

Acceptance:

- [ ] `task notices:check` passes; the OpenH264 row says it is not distributed.

### Step 5: Update the smoke test and the macOS CI job

Owner: senior-d. Files: `scripts/image-smoke.sh`, `.github/workflows/ci.yml`.

The smoke test: no `libopenh264` encoder until the explicit step; after the step the encoder test passes. The macOS CI job uses the own macOS build instead of the runner FFmpeg.

Commit: `ci(cli): smoke test the own build and use it on macOS`.

Acceptance:

- [ ] `task image:smoke` passes with the new pin.
- [ ] The macOS job log shows the own build binary, not the runner FFmpeg.

### Step 6: Re-run the FFmpeg threat tests against the own build

Owner: senior-c. Files: `engine/ffmpeg/security_flavour_test.go`, `engine/ffmpeg/security_manifest_test.go`.

Re-run `TestSecurity_T14_NonMediaInputsRefused` and `TestSecurity_T15_ArgvPolicy` against the own build, because the demuxer and filter names are pinned from `ffmpeg -demuxers` and `-filters` of the pinned build.

Commit: `test(service): <english summary>` if a test changes.

Acceptance:

- [ ] `go test -run 'TestSecurity_T1[45]|TestSecurity_T2[67]' ./...` passes on the three runners with the own build.

### Step 7: Close the container, network and race checks and the documents

Owner: senior-d, senior-c. Files: `docs/adrs/0011-ffmpeg-distribution-and-codec-policy.md`, `docs/sdds/007-security-model.md`, `site/content/docs/en/self-hosting/ffmpeg-and-licences.mdx`, `CHANGELOG.md`.

Acceptance:

- [ ] A clean container without network exports the reference scene with the best available encoder and fails with the hint of `troubleshooting/no-encoder` when none works (RF-204).
- [ ] With default settings the process opens no outbound connection during a full export (RF-206, `TestNoOutboundByDefault` of the security track).
- [ ] The three CI operating systems pass the `-race` tests with the own build (NFR-045).
- [ ] Item 15 of SDD-007 section 14 (the OpenH264 licence question) states the closed state and the pull request quotes the new text; `docs/adrs/0011-ffmpeg-distribution-and-codec-policy.md` closes its open point with the evidence; `docs/sdds/007-security-model.md` section 6.8 (the Cisco host) and `CHANGELOG.md` are updated; `site/content/docs/en/self-hosting/ffmpeg-and-licences.mdx` is updated.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- Building or signing the assets (plan 026).
- Hardware encoder support.
- Changing the OpenH264 licence stance beyond what ADR-0011 records.

## 5. STOP conditions

- The author has not allowed the OpenH264 download or the golden regeneration: stop; leave the branch out of the release.
- More than the expected 44 goldens change, or a PSNR below 50 dB: stop and report which cases.
- A security test needs a different demuxer or filter list: stop; the allowlist changes only with SDD-007 in the same pull request.

## 6. Maintenance

Every FFmpeg pin bump re-pins the manifest, regenerates the demuxer and filter lists, re-runs the T14, T15, T26 and T27 tests and the PSNR comparison, and re-generates the notices.
