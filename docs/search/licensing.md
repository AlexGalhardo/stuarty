# Licensing and codec patents: what an MIT editor can ship

> Stage 1, research. All sources accessed 2026-11-14.
> Marking: **FACT** (verified at the source), **INFERENCE** (deduction, with the reasoning), **OPINION** (the architect's judgement), **NOT VERIFIED**.
> This is not legal advice. Neither the author nor the architect is a lawyer; section 8 lists what needs one.
> Full evidence with every citation: [`notes/licensing.md`](notes/licensing.md). The author's decision on this subject is D-006 in [`../brainstorming.md`](../brainstorming.md).

## Contents

1. [Summary](#1-summary)
2. [FFmpeg: LGPL, GPL and what "separate process" buys](#2-ffmpeg-lgpl-gpl-and-what-separate-process-buys)
3. [Which prebuilt FFmpeg is really LGPL](#3-which-prebuilt-ffmpeg-is-really-lgpl)
4. [H.264 without x264](#4-h264-without-x264)
5. [Patents](#5-patents)
6. [What the LGPL build cannot do](#6-what-the-lgpl-build-cannot-do)
7. [Dependency matrix](#7-dependency-matrix)
8. [Open points and questions for a lawyer](#8-open-points-and-questions-for-a-lawyer)
9. [Export preset and where its numbers come from](#9-export-preset-and-where-its-numbers-come-from)

## 1. Summary

| Finding | Consequence for the product |
| --- | --- |
| Calling `ffmpeg` as a separate process keeps our code out of the LGPL and the GPL, but whoever distributes the binary still carries its obligations | FFmpeg is always a separate process, never linked. Every artifact we publish (Docker image, installer) ships the licence text, the configure line and a source offer |
| Publishing a Docker image or an installer is distribution; running FFmpeg on our own server is not | The policy applies to the image too. The Studio's `apk add ffmpeg` (a GPL build with x264) does not carry over |
| Of the prebuilt binaries surveyed, only the BtbN `lgpl` variant is LGPL, and it is LGPL version 3 | That is the build we pin. Third-party `ffmpeg` Docker images and `ffmpeg-static` are GPL or non-redistributable |
| Cisco's patent coverage for OpenH264 applies only to the binary the end user downloads from Cisco, for unpaid use | The BtbN `lgpl` build compiles OpenH264 in, which is **not** the covered case. See section 8, point 1 |
| H.264 patents are still active in the United States and Brazil; AAC encoding in an end-user product is also licensed | Prefer the encoder that the operating system or the hardware vendor already licensed |
| The `eq`, `boxblur`, `hqdn3d`, `delogo`, `perspective` filters are GPL | Colour adjustment and blur must use other filters or our own code |
| Remotion is source-available with paid tiers; GSAP forbids competing visual animation builders; `@ffmpeg/core` is GPL | None of the three is a dependency |
| The Pixabay licence does not allow redistributing raw files in a repository; OFL fonts and CC0 media do | Bundled media is OFL fonts only (D-014) |

## 2. FFmpeg: LGPL, GPL and what "separate process" buys

- **FACT.** FFmpeg is LGPL 2.1 or later with optional GPL parts; "If those parts get used the GPL applies to all of FFmpeg". GPL parts need `--enable-gpl` and include libx264, libx265 and a list of internal filters. `--enable-version3` moves the binary to LGPL version 3. `--enable-nonfree` (fdk-aac, OpenSSL) makes the binary non-redistributable. Sources: <https://ffmpeg.org/legal.html>, <https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/LICENSE.md>.
- **FACT.** FFmpeg's own LGPL compliance checklist: no `--enable-gpl` and no `--enable-nonfree`; dynamic linking; distribute the exact corresponding source and any changes; document the configure line; attribute FFmpeg and name the licence where the download is offered; no clause forbidding reverse engineering; no GPL libraries. Source: <https://ffmpeg.org/legal.html>.
- **FACT.** The FSF: pipes, sockets and command-line arguments "are normally used between two separate programs", and separate programs distributed together are an aggregate that may carry incompatible licences. Running a modified GPL program on a server without releasing it is a private use. Source: <https://www.gnu.org/licenses/gpl-faq.html>.

**INFERENCE.** Starting `ffmpeg` with arguments and reading its output is the textbook case of separate programs, so our MIT code is not subject to FFmpeg's licence in either build. The obligations about the FFmpeg binary itself remain with whoever distributes it. Linking `libav*` into our process would be the LGPL linking case and, with a GPL build, would make the whole application GPL. The separate process is the clean path.

## 3. Which prebuilt FFmpeg is really LGPL

| Build | Licence | Verdict |
| --- | --- | --- |
| BtbN/FFmpeg-Builds, `lgpl` variant | LGPL version 3 (`--enable-version3`); no x264, x265, fdk-aac | **Adopt**, pinned by version and checksum |
| BtbN `gpl` variant, gyan.dev, johnvansickle, evermeet.cx | GPL | Avoid in anything we publish |
| `ffmpeg-static` (npm) | GPL; downloads the GPL builds above | Avoid |
| Alpine `ffmpeg` package | GPL (`--enable-gpl --enable-libx264`) | Avoid; this is what the Studio image uses |
| jrottenberg/ffmpeg image | `--enable-nonfree` | Not redistributable |
| mwader/static-ffmpeg, linuxserver/ffmpeg | include libx264 | GPL (inference from their documentation) |
| Debian and Ubuntu `ffmpeg` package | NOT VERIFIED | Do not rely on it |

- **FACT.** The BtbN `lgpl` variant includes libass, FreeType, HarfBuzz, FriBidi, libvpx, SVT-AV1, libaom, rav1e, OpenH264, Opus, and the hardware interfaces for NVENC, AMF and QSV. Sizes on the day of the research: win64 167.5 MiB static, linux64 133.9 MiB static. Source: <https://github.com/BtbN/FFmpeg-Builds>.
- The machine used for this planning has the gyan.dev full build on PATH, which is GPL (checked with `ffmpeg -version`). It is fine for development and is not what ships.

## 4. H.264 without x264

| Encoder | Where it exists | Licence position | Notes |
| --- | --- | --- | --- |
| `h264_mf` (Media Foundation) | Windows | The codec belongs to the operating system | Software and hardware; `nv12` input is the safe choice |
| `h264_videotoolbox` | macOS | The codec belongs to the operating system | Documentation section not read in this research |
| `h264_nvenc`, `h264_qsv`, `h264_amf` | Windows and Linux with the matching GPU | The codec lives in the vendor driver | Not available on a CPU-only server |
| `h264_vaapi` | Linux with a supported GPU | Depends on the distribution's drivers; NOT VERIFIED | |
| `libopenh264` | Everywhere | Code is BSD-2-Clause; patent coverage only for Cisco's own binary | Profiles, B-frames and quality against x264 were NOT VERIFIED |
| `libx264` | Everywhere | GPL, and no patent coverage from anyone | Not shipped. A user may point the product at their own FFmpeg |

- **FACT.** Cisco's binary licence covers "THE PERSONAL USE OF A CONSUMER OR OTHER USES IN WHICH IT DOES NOT RECEIVE REMUNERATION", requires that the binary be downloaded by the end user separately and not combined with third-party software before download, that the user can enable or disable it, and that the application display "OpenH264 Video Codec provided by Cisco Systems, Inc.". Whoever compiles from source is not covered. Source: <https://www.openh264.org/BINARY_LICENSE.txt>.
- **INFERENCE.** Hardware and operating system encoders are not bit-exact across machines, drivers or generations. Reference tests cannot compare encoded bytes; they compare composed frames.
- **INFERENCE.** The hard case is a Linux server without a GPU: its only H.264 encoder in an LGPL build is OpenH264.
- **FACT.** For social networks, H.264 with AAC in MP4 is the only format that can be treated as safe. YouTube's specification is the only complete official one that could be read; official acceptance of AV1 or VP9 uploads was not confirmed for any network. Source: <https://support.google.com/youtube/answer/1722171>.

## 5. Patents

- **FACT.** H.264, Via LA: units 1 to 100,000 per year cost US$ 0, then US$ 0.20 per unit; the page does not mention open source and gives no expiry date. Source: <https://www.via-la.com/licensing-programs/avc-h-264/>.
- **FACT (community source).** H.264 patents expired in Europe, Japan, China and Canada; they are listed as active in Brazil until 2030-11-10 and in the United States until 2030-11-26. Source: <https://meta.wikimedia.org/wiki/Have_the_patents_for_H.264_MPEG-4_AVC_expired_yet%3F>.
- **FACT.** AAC, Via LA: a licence is required from makers of end-user encoder or decoder products; US$ 15,000 initial fee, US$ 1,000 for small entities; distributing AAC bitstreams is free. Source: <https://www.via-la.com/licensing-programs/aac/>.
- **FACT.** AV1: the AOMedia patent licence is royalty free with defensive termination and no indemnity. Third parties (Sisvel, Access Advance, Avanci) claim fees; this was read only in secondary sources. Source: <https://aomedia.org/license/patent-license/>.
- **FACT (secondary).** HEVC has several pools; Access Advance took over the Via LA HEVC and VVC programme in December 2025. Fees for software were NOT VERIFIED.
- **FACT.** Every open source video editor checked (OBS, Shotcut, HandBrake, Audacity) is GPL. None is MIT. Their practice of shipping x264 settles copyright, not patents, and is not a precedent for this project.

**INFERENCE.** Three levels of exposure, lowest first: (1) encoding through the browser's WebCodecs, where the browser is the licensee; (2) encoding through the operating system or GPU encoder, where the vendor is the licensee; (3) distributing a software H.264 encoder, where the distributor is exposed. Decision D-006 stays at level 2 and treats level 3 as the user's own choice.

## 6. What the LGPL build cannot do

- **FACT.** These internal filters are GPL and absent from an LGPL build: `eq` (brightness, contrast, saturation, gamma), `boxblur`, `smartblur`, `hqdn3d`, `delogo`, `perspective`, `cropdetect`, `colormatrix`, among others. The list came from an automated reading of `LICENSE.md`. Source: <https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/LICENSE.md>.
- **INFERENCE.** Likely LGPL substitutes exist (`gblur`, `avgblur`, `colorbalance`, `colorlevels`, `curves`, `lut3d`, `hue`, `colorchannelmixer`), but the licence of each was not checked filter by filter.

The spike `spikes/export-pipeline` lists the filters and encoders actually present in the pinned build and tries each H.264 encoder on a real machine. Its result, not this section, is what the render specification must rely on. See [`spikes.md`](spikes.md).

## 7. Dependency matrix

"Compatible" means we can depend on it and distribute it alongside MIT code without relicensing ours.

| Dependency | Licence | How it is used | Compatible | Obligations or verdict |
| --- | --- | --- | --- | --- |
| FFmpeg, LGPL build | LGPL 3 (BtbN) | separate process | yes | checklist of section 2 |
| x264, x265 | GPL | not used | not bundled | user may bring their own FFmpeg |
| fdk-aac | non-free | not used | no | |
| OpenH264 | BSD-2-Clause code; Cisco binary licence for patents | through FFmpeg | yes (code) | see section 8, point 1 |
| libvpx, SVT-AV1, rav1e | BSD family | through FFmpeg | yes | notices |
| libass | ISC | not used for burn-in | yes | |
| whisper.cpp | MIT | separate process | yes | notice |
| Whisper models (OpenAI) | MIT ("code and model weights") | downloaded at run time | yes | third-party variants need a check each |
| MCP Go SDK | MIT moving to Apache-2.0 | linked | yes | notices |
| vukem | MIT | linked | yes | |
| mediabunny | MPL-2.0 | browser bundle, if adopted | yes | file-level copyleft: publish changes to its files |
| mp4box.js | BSD-3-Clause | browser bundle, if adopted | yes | |
| PixiJS, Revideo, Motion Canvas, Lottie | MIT | not planned | yes | |
| Remotion | own licence, source-available, paid above 3 people | not used | **no** | avoid |
| GSAP | free "standard licence" that forbids competing visual animation builders | not used | **risk** | avoid |
| `@ffmpeg/core` (ffmpeg.wasm) | GPL-2.0-or-later | not used | **no** | avoid |
| MLT, GStreamer | LGPL | not used | yes | |
| Fonts under OFL | OFL | bundled | yes | keep each licence file; rename if modified |
| Pixabay media | Pixabay Content Licence | not used in the repository | **no** | standalone redistribution is forbidden |
| CC0 media | CC0 | not bundled in the MVP (D-014) | yes | record the origin of each file |

## 8. Open points and questions for a lawyer

1. **OpenH264 inside the pinned build.** Decision D-006 says H.264 comes from the system encoder "or by OpenH264 downloaded from Cisco on first run". The BtbN `lgpl` build has OpenH264 compiled in, and Cisco's coverage does not extend to a build from source. So shipping that build ships a software H.264 encoder outside Cisco's coverage, whether or not the product uses it. Options: (a) accept it, as every project that ships BtbN builds does; (b) build our own LGPL FFmpeg without OpenH264 and load Cisco's binary at run time, which is the model Fedora uses and costs a build pipeline for three operating systems; (c) ship no software H.264 encoder and fail with a clear message on machines that have none. **Decided by the author (D-017):** (a) through the Prototype and ALPHA, and (b) before the 1.0 release.
2. Whether a hosted service that encodes H.264 on a server is a "unit" under the Via LA programme. Out of scope until a SaaS exists (D-003).
3. Whether the 100,000 free units apply to an open source project with diffuse downloads.
4. The native AAC encoder of FFmpeg in a distributed product (the AAC programme licenses end-user encoder products).
5. The LGPL version 3 text was not read in this research; the "installation information" clause should be checked against a user-replaceable FFmpeg path.
6. **Codecs chosen after this research (SDD-011 §4.2), not researched here.** The normalized media uses FFmpeg's native MPEG-4 Part 2 encoder (`mpeg4`) and the browser proxy uses VP9 (`libvpx-vp9`, BSD licence). Neither file leaves the user's machine, but the product ships the encoders. The patent status of MPEG-4 Part 2 (standardized in 1999) and the terms of the VP9 patent grant were not verified at a primary source. To be confirmed before the first public release; the fallback that needs no new research is MJPEG for the normalized media (measured in SDD-011 §4.2: twice the size, 3.5 times slower to decode) and no proxy change.

## 9. Export preset and where its numbers come from

Every value below comes from YouTube's recommended upload settings, the only complete official specification read. The other networks have no verified official specification in this research, so this is the conservative common denominator and must be rechecked before it is frozen.

| Parameter | Value |
| --- | --- |
| Container | MP4 with the `moov` atom first (`-movflags +faststart`) |
| Video | H.264, progressive, 4:2:0 (`yuv420p`), High profile where the encoder supports it |
| Frame rate | the project's frame rate |
| Bitrate at 1080p | 8 Mbit/s at 24 to 30 fps, 12 Mbit/s at 48 to 60 fps |
| GOP | closed, half the frame rate |
| Audio | AAC-LC, 48 kHz, stereo |
| Colour | BT.709 primaries, transfer and matrix, limited range |
| Dimensions | even numbers |

Source: <https://support.google.com/youtube/answer/1722171>. Platform limits found for TikTok ads (at least 516 kbit/s, at most 500 MB and 10 minutes) and X (H.264 High, AAC-LC, at most 60 fps) are in [`notes/features.md`](notes/features.md); no official specification was found for Kwai.
