# Plan 026: The own FFmpeg build pipeline, published and signed assets

This plan answers how the product gets an FFmpeg build that is LGPL only, reproducible, published as signed release assets and pinned from a single source. When delivered, every platform build has no `--enable-gpl`, no `--enable-nonfree` and no linked OpenH264 library, rebuilds give the same SHA-256 (or the differences are justified), `.config/ffmpeg.env` pins the new release, and every asset verifies with `cosign verify-blob` and `gh attestation verify`.

- Owner: senior-d
- Effort: L
- Depends on: an author decision to allow the OpenH264 download and the regeneration of the golden frames (the own pin stays out of the release until then)
- Origin: EPIC-24 (P3-REL-001): TASK-0110, TASK-0111, TASK-0112, TASK-0113
- Status: BLOCKED (the own pin stays out of the release until the author allows the OpenH264 download and the goldens are regenerated)
- Written against: 8787714a

## 1. Why

ADR-0011 and author decision D-017 require shipping only an LGPL FFmpeg as a separate process, with the source link, the configure line and checksums published. The BtbN build in use today is a stopgap: `.config/ffmpeg.env` still said `FFMPEG_SOURCE=btbn` with the `FFMPEG_OWN_*` slots empty when this was audited.

## 2. Current state (drift check)

```text
cat .config/ffmpeg.env
bash scripts/ffmpeg-verify.sh --help
go test ./.config/ffmpeg-build/...
git branch --list "task/v02*ffmpeg*" "task/v024-senior-d2"
```

Already true (from the former card statuses):

- The pipeline `scripts/ffmpeg-build.sh` and `.github/workflows/ffmpeg-build.yml` exist; run 43795267511 passed 10 of 10 builds plus the Windows verify; the comparison was fixed by a3579bbe; confirmation in run 43956351325 (`publish=false`).
- Release `ffmpeg-9.0.2-1` is published: 5 platforms, SHA-256 checked, `gh attestation verify` ok.
- The own pin is deferred by the author for v0.25: the Linux binary needs glibc 2.43 (the Debian 13.7 image has 2.41), so rebuild 9.0.2-2 in the Debian container; 44 goldens change with OpenH264 2.6.0; add mp3lame, vorbis and webp to the build; CI installs OpenH264. Ready commits are on branch `task/v024-senior-d2` (3bddba83, 37f6d4f9, 74c1d770, 7417804d, 7655a9d0).
- Done on branch `task/v025-ffmpeg-own` (2026-11-17): the pin `ffmpeg-9.0.2-2` in `.config/ffmpeg.env`; notices `-check` ok, T14/T15 ok, image smoke ok. Outside the release until the author allows the OpenH264 download and the goldens are regenerated.

## 3. Steps

### Step 1: Publish the build outputs as release assets

Owner: senior-d. Files: `.config/ffmpeg-build/`, `.github/workflows/ffmpeg-build.yml`, `scripts/ffmpeg-build.sh`.

Publish the build outputs as a set of repository release assets, each with its SHA-256, a link to the source tarball and the configure line, for Linux amd64 and arm64 (archives and image), Windows amd64, macOS arm64 and amd64 (confirm the platform list with the architect).

Commit: `ci(cli): publish the FFmpeg build outputs as release assets`.

Acceptance:

- [ ] `go test ./.config/ffmpeg-build/...` passes; a new or changed test covers the change (fails before, passes after).
- [ ] The release exists with one asset per platform, each with its SHA-256, the source tarball link and the configure line.

### Step 2: Switch the pin, keeping the single-source rule

Owner: senior-d. Files: `.config/ffmpeg.env`.

Move `.config/ffmpeg.env` to the new pin (`ffmpeg-9.0.2-2` on the Debian container build); the Dockerfile and `scripts/install-ffmpeg.sh` both read from it.

Commit: `ci(cli): pin the own FFmpeg build in ffmpeg.env`.

Acceptance:

- [ ] `.config/ffmpeg.env` does not contain the text `<recorded by`, and each SHA-256 equals the `sha256sum` of a fresh download.
- [ ] `go test ./.config/ffmpeg-build/...` passes.

### Step 3: Sign every asset and attach provenance

Owner: senior-d. Files: `.github/workflows/ffmpeg-build.yml`, `scripts/ffmpeg-verify.sh`.

Sign each published build asset with cosign (keyless, with the identity of `ffmpeg-build.yml`) and attach a build provenance attestation, like the release artifacts of SDD-009 section 9.3.

Commit: `ci(cli): sign the FFmpeg build assets with cosign and attest provenance`.

Acceptance:

- [ ] `cosign verify-blob` and `gh attestation verify` succeed for every build asset with the identity of the workflow; the commands are in `scripts/ffmpeg-verify.sh`.

### Step 4: Verify the builds are LGPL only and reproducible

Owner: senior-d. Files: `scripts/ffmpeg-verify.sh`, `engine/ffmpeg/` (filter allowlist test).

Acceptance:

- [ ] For each platform build, `ffmpeg -version` has no `--enable-gpl`, no `--enable-nonfree` and no linked OpenH264 library (`ffmpeg -buildconf` and a linker listing, automated in `scripts/ffmpeg-verify.sh`).
- [ ] `ffmpeg -filters` contains every filter the engine references: 25 of the 27 checked by the spike plus any the product added; the two GPL-only filters (`eq`, `boxblur`) are not referenced (item 6 of ADR-0011; the filter allowlist test of `go test ./engine/ffmpeg/...`).
- [ ] A rebuild from the same inputs on a second run gives the same SHA-256 per platform, or the differences are listed and justified in the report.
- [ ] `task lint` reports no findings.

## 4. Out of scope

- Adopting the build in the Dockerfile, notices and CI (plan 027).
- A cosign or SLSA scheme different from SDD-009 section 9.3.
- Hardware encoders.

## 5. STOP conditions

- The author has not allowed the OpenH264 download: do not move the own pin into a release; stop.
- A platform build links OpenH264 or enables GPL or nonfree: stop; the release is blocked.
- The Linux binary needs a newer glibc than the image provides: stop and report; rebuild in the Debian container.
- Any added codec library (mp3lame, vorbis, webp) has a licence not on the allowlist: stop and ask the orchestrator.

## 6. Maintenance

The FFmpeg pin must be at most 90 days old at release time; the freshness check reports newer patch releases of the branch. Any change to `.config/ffmpeg-build/` re-runs the verification script and the rebuild comparison.
