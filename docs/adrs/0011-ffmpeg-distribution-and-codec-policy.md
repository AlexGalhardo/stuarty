# ADR-0011: FFmpeg distribution and codec policy

- Status: Accepted (2026-11-14); item 8 is scoped to the product by author decision D-126 (2026-11-21)
- Deciders: author (D-006, D-017), architect
- Links: author decision D-006, licensing, spikes, SDD-004, SDD-007, SDD-009

## Context

The product is MIT and is distributed as a binary and a Docker image. x264 and x265 are GPL. Publishing an image or installer is distribution. The Studio encodes with `libx264` from Alpine's GPL FFmpeg. Social networks require H.264 with AAC in MP4. H.264 patents remain active in the United States and Brazil.

## Decision

1. FFmpeg and ffprobe are **separate processes**, never linked. They are started through vukem's `contrib/engine` with `-protocol_whitelist` restricted to local files and pipes. Arguments are built from typed values and checked against an allowlist before every start; the concat demuxer and file lists are not used (SDD-007 §6).
2. Everything we publish contains only an **LGPL build**: the BtbN `lgpl` variant on a release branch, pinned by version and SHA-256. The artifact carries the licence text, the configure line and a link to the corresponding source.
3. The product can also use **an FFmpeg the user points to** (`VUKEM_VIDEO_FFMPEG`). That is how someone gets x264: their binary, their choice.
4. **Output format** in the MVP: MP4, H.264, AAC-LC, 4:2:0, BT.709 limited, `+faststart`, even dimensions.
5. **H.264 encoder selection**, first that works on the machine: `h264_videotoolbox` (macOS); `h264_nvenc`, `h264_qsv`, `h264_amf` (matching GPU); `h264_mf` (Windows); `h264_vaapi` (Linux with a supported GPU); `libopenh264`; `libx264` only when the user's own FFmpeg provides it and the user enabled it. Detection is by a real test encode at start-up, cached. If none works, export fails with an error that says how to fix it.
6. Only filters present in the pinned LGPL build are used. The GPL-only filters (`eq`, `boxblur`, `hqdn3d` and others) are never referenced; colour adjustment and blur use LGPL filters or our own code.
7. Reference tests compare composed frames before encoding, with the software path, because hardware encoders are not bit-exact across machines.
8. No dependency on Remotion, GSAP or `@ffmpeg/core` in the product (the editor, the renderer and the export). The documentation site may use GSAP and three.js for its landing page; they never enter the binary (author decision D-126, 2026-11-21).

## Evidence from the spike

Measured in `spikes/export-pipeline` on one Windows 11 machine (Ryzen 7 5700X3D, RTX 4060) with the BtbN `lgpl` build `n9.0.2-57`:

- The configure line has `--enable-version3` and neither `--enable-gpl` nor `--enable-nonfree`.
- Of 27 filters checked, 25 are present. Missing: `eq` and `boxblur`.
- H.264 encoders that worked: `h264_nvenc`, `h264_mf`, `libopenh264`. `h264_qsv` and `h264_amf` failed for lack of hardware. The native AAC encoder works.
- `h264_mf` resolved to Microsoft's software encoder even with an NVIDIA GPU present.
- Quality at 8 Mbit/s against a lossless reference (SSIM): `libx264` 0.9829 (reference only, not shipped), `h264_nvenc` 0.9821, `libopenh264` 0.9797, `h264_mf` 0.9651.
- A 30 s 1080×1920 export took 2.5 to 7.6 s depending on encoder and overlay strategy.

Consequence for item 5: on Windows without a supported GPU the operating system encoder is measurably worse than OpenH264 at the same bitrate. The order of item 5 keeps the operating system encoder ahead of OpenH264 because of the licensing position (D-006), and compensates with a higher default bitrate for `h264_mf`. The user can select the encoder explicitly.

## Alternatives

- **Bundle a GPL FFmpeg with x264.** Best quality and identical output everywhere; the published artifacts would contain a GPL binary and a software H.264 encoder, with the patent exposure on the distributor. Rejected by the author.
- **Ship no FFmpeg.** No obligations; a worse first run. Kept as the user-override path.
- **Royalty-free output (VP9 or AV1 with Opus).** Acceptance by the target networks could not be confirmed.

## Consequences

Positive: our code stays MIT and the artifacts we publish contain no GPL component.

Negative: encode speed and quality vary by machine; the same project does not produce the same bytes on two machines.

Negative: a Linux server with no GPU has only OpenH264. Its quality at a given bitrate is expected to be below x264 (not measured before the spike).

Negative: the Studio's render code (`-c:v libx264`, `apk add ffmpeg`) is not reusable as is.

## Resolved by the author

The pinned build has OpenH264 compiled in, and the Cisco patent coverage applies only to the binary a user downloads from Cisco. The author decided (D-017): accept the BtbN build through the Prototype and ALPHA; before 1.0 the project produces its own LGPL FFmpeg build without OpenH264 that loads the Cisco binary at run time, as Fedora does. That build pipeline is a BETA task group and an exit criterion of BETA. That binary is obtained only by the explicit install step (`tools:install openh264`), from a pinned URL with a checksum, through the outbound purpose `helper_download` (SDD-007 §10), never automatically.

Status of this point: the pipeline is written (`.config/ffmpeg-build/`, `scripts/ffmpeg-build.sh`, `.github/workflows/ffmpeg-build.yml`, `scripts/ffmpeg-verify.sh`) and the image, `scripts/install-ffmpeg.sh` and the smoke test can use its output behind `FFMPEG_SOURCE`, but no build has been run or released yet: the BtbN pin stays the one in use until the first release of the own build fills the `FFMPEG_OWN_*` pins. The build links libvpx, Opus, zlib, zimg, LAME (LGPL-2.0-or-later, used under the LGPL-3.0-or-later of the build), libogg, libvorbis and libwebp statically (the last three for the fixture and export audio and image encoders), and the NVENC headers (loaded at run time); the OpenH264 library is not linked. Decision of the author (2026-11-15), how the `libopenh264` encoder is exposed without it: a **dlopen patch**, as Fedora does. A patch of FFmpeg's `libopenh264` wrapper (`.config/ffmpeg-build/patches/0001-libopenh264-dlopen.patch`, pinned by SHA-256 in `sources.env`) resolves the three Cisco functions with `dlopen` or `LoadLibrary` from `VUKEM_VIDEO_OPENH264_DIR` and refuses a version other than the pinned 2.6.0; the configure line carries no `--enable-libopenh264`. Without the library the encoder cannot open and the product answers `E_NO_ENCODER` (SDD-011 section 8.4).

The codecs of the normalized media and of the proxy are fixed by SDD-011 §4.2 (MPEG-4 Part 2 in MOV with PCM; VP9 in MP4 with AAC); their patent status is an open point for the author before the first public release (research note `licensing` §8 item 6).
