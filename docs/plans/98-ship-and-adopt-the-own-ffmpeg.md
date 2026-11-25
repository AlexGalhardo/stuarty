# 98: Ship the own FFmpeg build in the installers and adopt it

## Goal

A creator who installs Stuarty on Windows exports at once, with no FFmpeg to find, because the installer carries the project's own LGPL build and discovery prefers it.

## Context and related decisions

- Epic 46 built the own FFmpeg (`ffmpeg-9.0.2-3`, LGPL, reproducible, marker `stuarty<N>`, `STUARTY_OPENH264_DIR`) and adopted it in the Docker image, but left three things open: the signed assets are not published, no installer carries an FFmpeg, and `.config/ffmpeg.env` still pins the third-party build.
- Author decision D-152 (2026-11-25): the own build ships with a release; D-087: OpenH264 is never bundled or fetched by the build, it is installed by the person.
- ADR-0011 (only LGPL, as a separate process), ADR-0045 (releases deferred until CI returns), SDD-011 section 8, SDD-009.

## Scope

- Modules: `desktop/packaging/` (Windows installer first; the Linux and macOS packages only where they can be built here), `engine/ffmpeg/` discovery next to the executable, the desktop release workflow inputs, SDD-011, SDD-009, `licenses/` and the notices.
- Must not touch: the FFmpeg build itself (epic 46), `renderer/`, the publishing domain.

## Out of scope

Bundling or fetching OpenH264; macOS and Linux packages that cannot be built and verified here (they follow epic 43); code signing (D-033).

## Dependencies

46 (approved and integrated). The publication of the signed assets needs CI and the author.

## Technical design

- The installer takes `ffmpeg.exe` and `ffprobe.exe` from the published `ffmpeg-9.0.2-3` Windows archive, checked by the SHA-256 the pins name, and installs them next to `stuarty.exe`, with the licence texts of the build.
- Discovery already looks next to the executable; it must prefer the bundled own build over the PATH and keep the person's own FFmpeg setting first.
- Until the assets are published, the installer build reads the archive from a local folder given by a build input, so it can be tested here; the published URL and digest replace it at release.
- Adoption flips `.config/ffmpeg.env` to `FFMPEG_SOURCE="own"` with the `FFMPEG_OWN_*` block that `ffmpeg-pin.sh print` gives for the published archives. A `*.env` file can only be committed by the author.

## Tasks

- [x] 1. The Windows installer carries the own FFmpeg and its licences, from a local archive or the published one by digest. Verify: the installer build script runs here, and the installed folder holds `ffmpeg.exe`, `ffprobe.exe` and the licences, each with the pinned digest.
- [x] 2. Discovery prefers the bundled build after the person's setting. Verify: `go test ./engine/ffmpeg/...` with a test that puts a bundled build next to a fake executable and another FFmpeg on the PATH.
- [x] 3. A fresh Windows install exports the reference project with no FFmpeg supplied. Verify: install into a temporary folder with an isolated home and run the reference export through the installed `stuarty.exe`.
- [x] 4. Notices and SDD-011, SDD-009 updated. Verify: `task notices:check` and `task lint:md` exit 0.
- [ ] 5. Publication (the author, through CI): run `ffmpeg-build.yml` with `publish=true`, which builds every platform, signs and attests the archives. Verify: `bash scripts/release-verify.sh` for that release.
- [ ] 6. Adoption (the author commits): `.config/ffmpeg.env` switched to the own build with the published digests. Verify: `go test ./internal/tools/ffmpegpins/...` and the image smoke test.

## Test strategy

The packaging tests of the Windows installer, the discovery unit test, and one real install and export on this machine.

## Acceptance criteria

1. A fresh Windows install exports the reference project with no FFmpeg supplied by the person.
2. The installed FFmpeg has the pinned digest and reports the `stuarty` marker.
3. A person's own FFmpeg setting still wins over the bundled build.
4. The licence texts of the build are installed and listed in the notices.
5. `task lint` and the tests of the touched packages pass.

## Risks and mitigation

- A larger installer. Mitigation: only `ffmpeg.exe` and `ffprobe.exe`, no extra libraries.
- **STOP** if the installer would have to fetch an archive at install time, or bundle OpenH264.

## Size

M

## Hand-off for review

The commits, the installer build output, the digests of the installed binaries, the export of the reference project from the fresh install, and what waits for the author (tasks 5 and 6).
