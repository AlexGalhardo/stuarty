# Licensing and codec patents: MIT video editor (web + desktop)

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).

Research of 2026-11-14. Not legal advice. Legend: **FACT** (read in the primary source, URL cited, accessed 2026-11-14), **INFERENCE** (my reasoning from facts), **OPINION** (recommendation), **NOT VERIFIED** (I could not open the primary source or it does not provide the data).

## Index

1. FFmpeg LGPL vs GPL vs nonfree
2. Encoders without x264/x265 (H.264, HEVC, AV1/VP9, audio) and platform acceptance
3. Patents (AVC, HEVC, AAC, AV1) and how real projects deal with them
4. Ready-made FFmpeg builds and licenses
5. ffmpeg.wasm
6. Dependency matrix
7. Recommendation (OPINION) and export preset
8. Gaps and points for a lawyer
9. Sources

---

## 1. FFmpeg LGPL vs GPL vs nonfree

### 1.1 What each flag implies

- FACT: FFmpeg's `LICENSE.md` separates the components. Source: <https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/LICENSE.md> (accessed 2026-11-14).
  - **GPL (require `--enable-gpl`)**: external libraries avisynth, frei0r, libcdio, libdavs2, librubberband, libvidstab, **libx264, libx265**, libxavs, libxavs2, libxvid; and internal filters (list below). If used, "the GPL applies to all of FFmpeg" (<https://ffmpeg.org/legal.html>, accessed 2026-11-14).
  - **version3 (`--enable-version3`)**: gmp, libaribb24, liblensfun (LGPLv3), mbedTLS, RK MPI, OpenCORE, VisualOn, libastcenc (Apache 2.0); smbclient requires gpl+version3. Effect: the binary goes from LGPLv2.1+ to **LGPLv3+** (GPLv3+ if combined with gpl).
  - **nonfree (`--enable-nonfree`)**: Fraunhofer **FDK AAC** and **OpenSSL**. The resulting binary is "not redistributable". (fdk-aac is incompatible with GPL: <https://ffmpeg.org/ffmpeg-codecs.html>, accessed 2026-11-14.)
- GPL filters listed in LICENSE.md (FACT, via automated reading of the page; confirm with `grep -l "GPL" libavfilter/*.c` in the tarball before relying on it): `vf_blackframe, vf_boxblur, vf_colormatrix, vf_cover_rect, vf_cropdetect, vf_delogo, vf_eq, vf_find_rect, vf_fspp, vf_histeq, vf_hqdn3d, vf_kerndeint, vf_lensfun, vf_mcdeint, vf_mpdecimate, vf_nnedi, vf_owdenoise, vf_perspective, vf_phase, vf_pp7, vf_pullup, vf_repeatfields, vf_sab, vf_signature, vf_smartblur, vf_spp, vf_stereo3d, vf_super2xsai, vf_tinterlace, vf_uspp, vf_vaguedenoiser, vsrc_mptestsrc`, plus `removegrain` (asm).
- INFERENCE, relevant for an editor: **`eq`** (brightness/contrast/saturation/gamma), **`hqdn3d`** (denoise), **`delogo`**, **`perspective`**, **`boxblur`**, **`cropdetect`**, **`smartblur`** and **`colormatrix`** exist only in a GPL build. Probable LGPL substitutes (existence in FFmpeg known, license of each one NOT checked filter by filter): `gblur`/`avgblur`, `colorbalance`, `colorlevels`, `curves`, `lutyuv`/`lut3d`, `hue`, `colorchannelmixer`, `scale`, `overlay`, `xfade`, `fade`, `zoompan`, `drawtext`. For the final list, run `ffmpeg -filters` on the chosen LGPL build and compare with what the editor exposes.
- FFmpeg's legal page does not define the flags; it only says not to use `--enable-gpl`/`--enable-nonfree` on the LGPL path.

### 1.2 FFmpeg's own LGPL checklist (FACT)

Source: <https://ffmpeg.org/legal.html> (accessed 2026-11-14). Items: build without `--enable-gpl` and without `--enable-nonfree`; **dynamic linking** (on Windows, DLLs); distribute the FFmpeg source code (modified or not) **exactly matching** the binary; include the `git diff` of your changes; document the configure line in a text file at the source root; package a tarball/zip; host the source on the same server as the binary; attribution with links on every page that has a download; cite "LGPLv2.1" in the About and in the EULA; if the EULA claims ownership of the code, state that you do not own FFmpeg; **remove the reverse-engineering prohibition from the EULA** (and from all translations); spell the name correctly; do not obfuscate DLL names (prefix/suffix is fine); repeat for each LGPL library embedded (LAME etc.); have no GPL libs (libx264).

### 1.3 Sidecar (separate process): does it change the obligation?

- FACT: the FSF says that pipes, sockets and command-line arguments "are mechanisms normally used between two separate programs", but if the semantics of the communication is "intimate" (exchange of complex internal structures), it may be considered a single program. Also: an "aggregate" of separate programs distributed together is allowed even with incompatible licenses. Source: <https://www.gnu.org/licenses/gpl-faq.html> (accessed 2026-11-14).
- INFERENCE: calling `ffmpeg` via `exec`/`spawn` with CLI arguments and reading stdout/stderr is the classic case of separate programs. Consequence: the **MIT code of the app is not subject** to either the LGPL or the GPL of ffmpeg. But the obligations **on the ffmpeg binary itself** (corresponding source, notices, license text, configure line) remain, because you **distribute** it. If it is GPL, the obligation is heavier: offer the complete corresponding source of ffmpeg (+x264 etc.) and license the binary under the GPL, without additional restrictions.
- INFERENCE: linking `libav*` inside the app process (FFI, native addon, Wails/Tauri with bindings) is the LGPL "link" case. It works with LGPL if the linking is dynamic and the user can swap the library. With GPL, the whole app becomes GPL. So: **a sidecar is the cleanest path** for an MIT app.
- INFERENCE: LGPLv3 (builds with `--enable-version3`, the BtbN case, see 4.1) carries an "installation instructions" clause for "User Products"; for a user-replaceable sidecar (configurable ffmpeg path) this is easy to comply with. I did not read the LGPLv3 text in this session: confirm.

### 1.4 Server (SaaS) vs desktop

- FACT: the GPL does not require publishing modifications by someone who only runs the program on a website; the case is "a special case of private use" (FSF FAQ, UnreleasedMods question; same URL). The AGPL is the license that covers this (outside ffmpeg).
- INFERENCE: SaaS that runs GPL ffmpeg on the server **without delivering the binary** does not trigger copyleft obligations. **But**: (i) a **published** Docker image (self-host) with GPL ffmpeg is distribution, and the GPL applies to that component; (ii) copyleft has nothing to do with patents: the patent risk exists regardless of distribution (section 3).
- INFERENCE: a distributed desktop app = full distribution: the whole checklist above applies.

---

## 2. Encoders without x264/x265

### 2.1 Software H.264: OpenH264 (Cisco)

- FACT (code license): BSD-2-Clause (GitHub API, `cisco/openh264`, accessed 2026-11-14).
- FACT (patents, binary terms): <https://www.openh264.org/BINARY_LICENSE.txt> (accessed 2026-11-14).
  - Cisco's binary is only licensed under Cisco's license with MPEG LA if conditions are met.
  - Coverage "FOR THE PERSONAL USE OF A CONSUMER OR OTHER USES IN WHICH IT DOES NOT RECEIVE REMUNERATION"; "NO LICENSE IS GRANTED ... FOR ANY OTHER USE". Content providers/broadcasters may need their own license from MPEG LA.
  - The binary must be **downloaded separately by the end user**, "not integrated or combined with third-party software before it is downloaded"; the user must be able to enable/disable its use; the app must display "OpenH264 Video Codec provided by Cisco Systems, Inc." and reproduce the license text in the EULA.
  - Whoever compiles from source (or packages it) does not have Cisco's coverage and is responsible for the licenses.
- INFERENCE: (a) it is **not possible to embed** libopenh264 in a desktop installer and count on Cisco's coverage; the app would have to download from Cisco's server at runtime (as Firefox does). (b) The coverage is for use without remuneration: a **paid SaaS** that encodes on the server is probably **not covered**, and the operator would have to seek its own license. (c) Cisco only covers patents in the MPEG LA/Via LA pool, not third-party ones (webrtchacks/Mozilla 2013, via search; NOT VERIFIED in the primary source, the Mozilla link returned 404).
- Profiles and quality: FACT (partial): the FFmpeg doc says the `main` profile enables CABAC in `libopenh264` (<https://ffmpeg.org/ffmpeg-codecs.html>). NOT VERIFIED: support for High profile, B-frames, and quality vs x264 (I did not find a benchmark in a primary source; from memory, OpenH264 is Constrained Baseline/Main focused on WebRTC, without B-frames in encoding, with efficiency well below x264. Treat as a hypothesis to test with VMAF).

### 2.2 Hardware / OS encoders

- FACT (FFmpeg doc, <https://ffmpeg.org/ffmpeg-codecs.html>, accessed 2026-11-14):
  - `h264_nvenc/hevc_nvenc/av1_nvenc`: requires `nv-codec-headers` (ffnvcodec) headers at build time; runtime requires an NVIDIA GPU and driver; features depend on the SDK (12.0 to 13.1).
  - QSV (`h264_qsv`, HEVC, VP9, AV1...): Intel QuickSync; `low_power` reduces consumption.
  - `h264_mf/hevc_mf/av1_mf` (Media Foundation, Windows): support software and hardware; hardware requires D3D11; input nv12/yuv420p, nv12 is safer.
  - VideoToolbox, AMF and VAAPI sections: NOT READ (the page had ~14k final characters I did not go through).
- FACT (BtbN): the `lgpl` variant compiles `amf`, `ffnvcodec`, `onevpl` (QSV, not arm64), `openh264`, `libvpx`, `svtav1`, `aom`, `rav1e`, `vvenc`, `kvazaar` (scripts at <https://github.com/BtbN/FFmpeg-Builds/tree/master/scripts.d>; `x264` and `fdk-aac` stay disabled outside gpl/nonfree: I checked `ffbuild_enabled` in the scripts via raw.githubusercontent.com). `defaults-lgpl.sh` uses `--enable-version3` (LGPLv3).
- INFERENCE: the headers (ffnvcodec, AMF) are only interface; the actual encoder lives in the manufacturer's driver. For that reason, the OS/GPU hardware encoder brings neither libx264 nor a patent into your binary: the manufacturer/OS licensed the codec in the driver (the "manufacturer already licensed it" scenario, see section 3.4; this is a reading, I did not verify NVIDIA/Intel/AMD/Apple contracts).
- INFERENCE / NOT VERIFIED: availability per OS (NVENC: Windows+Linux with NVIDIA; QSV: Intel, Windows+Linux; AMF: AMD, Windows and Linux; VAAPI: Linux; VideoToolbox: macOS; `h264_mf`: Windows). **Determinism**: hardware encoders are **not bit-exact across machines/drivers/generations**; quality at equal bitrate is usually below x264 `slow`. For an editor, this is acceptable for export, but hash-based regression tests do not work.
- Practical failure: on a CPU-only cloud SaaS server there is no hardware encoder; the H.264 path needs a fallback.

### 2.3 Royalty-free (by codec licensing)

| Codec/lib | Code license (FACT, GitHub API, accessed 2026-11-14) | Note |
|---|---|---|
| libvpx (VP8/VP9) | BSD-3-Clause | the BtbN lgpl build includes it (except winarm64) |
| SVT-AV1 | BSD-3-Clause-Clear | the BtbN lgpl build includes it (except win32); the "Clear" license does not grant patents by itself; patent via AOMedia license |
| rav1e | BSD-2-Clause | same |
| libaom | BSD-2 + AOM patent (I did NOT check the file) | included |
| Opus (audio) | BSD (not checked in this session) | included (`50-libopus.sh`) |

- FACT: the AOMedia Patent License 1.0 grants a "no-charge, royalty-free, irrevocable" license on the licensors' "Necessary Claims", with a **defensive termination** if the licensee sues someone for patent infringement over an Implementation; **there is no indemnity** nor guarantee against third parties ("AS IS"). Source: <https://aomedia.org/license/patent-license/> (accessed 2026-11-14).
- CPU speed: NOT VERIFIED (no primary benchmark). From memory, SVT-AV1 is usable in reasonable time on multicore CPUs, libaom and rav1e are slower; VP9 is slower than x264. Test on the target hardware.

### 2.4 Acceptance of AV1/VP9/WebM on platforms

- **YouTube** (FACT): recommended specifications page (<https://support.google.com/youtube/answer/1722171>, accessed 2026-11-14): MP4, moov at the front (faststart), no edit lists; H.264 High, progressive, 2 consecutive B-frames, closed GOP of half the fps, CABAC, 4:2:0; audio AAC-LC, Opus or Eclipsa, 48 kHz; SDR in BT.709. The visible section does not talk about AV1/VP9 acceptance on upload (it only says VP9 is needed to display 4K). A secondary search indicates WebM with AV1/VP9 is accepted (third-party sources, NOT VERIFIED).
- **TikTok** (partial FACT): ads (<https://ads.tiktok.com/help/article/video-ads-specifications>): .mp4/.mov/.mpeg/.3gp/.avi, 9:16 ≥ 540x960, ≥ 516 kbps, up to 10 min, ≤ 500 MB; **codec and fps: not specified**.
- **Instagram, X, WhatsApp, Kwai**: NOT VERIFIED in an official source. The pages tried (help.instagram.com/1038274519691569, facebook.com/business/help/1275400645914358, help.x.com/en/using-x/x-videos returned 403, faq.whatsapp.com/1119540752042898) returned empty, 403, 404 or content without a spec. Kwai was not even tried. Third-party guides converge on MP4, H.264, AAC, 1080x1920 9:16, 30 fps; bitrate varies from 3.5 to 15 Mbps; no official confirmation of AV1/VP9. **Operational conclusion (INFERENCE)**: treat H.264+AAC in MP4 as the only format guaranteed for social networks; AV1/VP9/WebM only as a YouTube/file option.

### 2.5 Audio

- FACT: FFmpeg's **native** AAC encoder is the default, implemented inside FFmpeg (LGPL). VBR (`-q:a`) exists; the `anmr` coder is experimental and not recommended; "`q=1` ≈ 128 kbps in musical stereo". `libfdk_aac`: experimental VBR; incompatible with GPL, requires `--enable-nonfree` (<https://ffmpeg.org/ffmpeg-codecs.html>).
- INFERENCE: use native AAC in CBR (e.g. 128 to 192 kbps) on the LGPL build and avoid fdk-aac (nonfree, not redistributable). Opus is royalty-free, but **many social networks require AAC in MP4**; Opus in MP4 is only accepted by YouTube (the page above lists Opus as valid).
- AAC patent: see 3.3. FFmpeg having its own encoder does not eliminate the patent.

---

## 3. Patents

### 3.1 H.264/AVC (Via LA)

- FACT (<https://www.via-la.com/licensing-programs/avc-h-264/>, accessed 2026-11-14):
  - Unit = "decoder, encoder or product consisting of one decoder and one encoder". Annual fees: units 1 to 100,000: **US$ 0.00** ("available to one legal entity of an affiliated group"); 100,001 to 5 million: US$ 0.20; above: US$ 0.10. Annual cap per company: US$ 9.75 million (2017 onward).
  - Streaming: title ≤ 12 min: US$ 0; longer title: the lower of 2% of revenue and US$ 0.02 per title. There are tiers by subscribers/users (OTT, FAST, **Social**, Cloud Gaming) with a minimum fee of US$ 100,000 for small/emerging companies and a cap of US$ 4.5 million.
  - Products incorporated into the PC OS have the same table; end users may use it "for personal and consumer purposes (including commercial internal use) without remuneration".
  - The page **does not mention open source**, and **does not state an expiry date** for the license or the patents; it says the patent list (Attachment 1) was updated on 2026-11-01 and that there is "no warranty" that it covers all essential patents.
- **Expiry status**: I could not open the official list (the essentiality page returns 404). FACT from a community source (<https://meta.wikimedia.org/wiki/Have_the_patents_for_H.264_MPEG-4_AVC_expired_yet%3F>, edited 2026-09-16; it warns that it is not legal advice): Europe expired (2025-01-26), Japan (2025-08-09), China (2025-08-16), Canada (2024-03-30); **Brazil active until 2030-11-10**; US: MPEG LA list patents with the last listed expiry on **2030-11-26** (Siemens US 9356620), but "it is not clear which are necessary for the High Profiles"; a press article cites the end of 2027 as the commonly cited US expiry (secondary). **Reading**: in the US, Brazil and Malaysia there are still active patents; one cannot state "AVC is free" in 2026.
- INFERENCE: below 100,000 "units/year" the fee would be zero, **but** it requires being licensed (a contract with Via LA). Whether a "unit" applies to an open source download, and whether the operator of a SaaS that encodes on a server is a "unit", is a question for a lawyer.

### 3.2 HEVC

- FACT: on 2025-12-15 **Access Advance took over the administration of Via LA's HEVC/VVC program**, creating "VCL Advance"; licensing still initially separate; stated goal: a single pool (<https://ipfray.com/breaking-access-advance-acquires-via-licensing-alliances-hevc-vvc-patent-pools/> and streetinsider, via search, accessed 2026-11-14; secondary). HEVC Advance page (<https://accessadvance.com/licensing-programs/hevc-advance/>): "FRAND" terms, no fees on the page, details in PDF/linked pages. **Fees and exemption for software: NOT VERIFIED.** Besides the pool, there are licensors outside it (history; I did not verify the current list). **Avanci Video** also charges streaming services; fees only in a secondary source (1.6 to 2.0% of revenue or US$ 0.12 to 0.15 per user, not verified).
- INFERENCE: HEVC is the worst case of patent risk; it is advisable **not to offer software HEVC** in an MIT product. Only via OS/GPU hardware.

### 3.3 AAC (Via LA)

- FACT (<https://www.via-la.com/licensing-programs/aac/>, accessed 2026-11-14): a license is required from **manufacturers/developers of end-user encoder and/or decoder products**; per-unit fee of US$ 0.98 (1 to 500 thousand) decreasing to US$ 0.10; **initial fee US$ 15,000** (US$ 1,000 for small entities: ≤ 15 employees and < US$ 1 million in revenue); 5-year renewable term; **distributing AAC bitstreams does not require a license**; "non-final components or implementations are not covered: whoever incorporates them into an end-user product needs its own license"; the page quotes the Android CDD: "implementations of this code, including open source or shareware, may require patent licenses".
- INFERENCE: a desktop editor that embeds the AAC encoder (FFmpeg native) is, in theory, an "end-user encoder product". Escape routes: AAC via the OS encoder (AudioToolbox on macOS, Media Foundation on Windows) or Opus where the platform accepts it.

### 3.4 AV1

- FACT: AOMedia, royalty-free under the patent license (see 2.3), with no indemnity.
- Partial FACT (search, secondary): Sisvel has an AV1 pool with per-device fees (e.g. € 0.32 / € 0.24 for consumer displays; undated page) and said in Jul 2025 it had licensed about half of the market for finished AV1 products; Access Advance and Avanci charge streaming services (AV1 included). AOMedia disputes the need; I did not verify AOMedia's current position nor the Sisvel page (the direct URL returned 404).
- INFERENCE: AV1 is "royalty-free in the codec license", **not** "free of third-party claims".

### 3.5 Three risk scenarios for the MIT project

(OPINION/INFERENCE, not a legal opinion.)

1. **Distributing a desktop binary with a software H.264 encoder (x264, or OpenH264 embedded)**: the distributor is the one who "manufactures/distributes the encoder". Real risk, especially if there is revenue or large scale. x264 is not covered by anyone; embedded OpenH264 loses Cisco's coverage.
2. **Using only OS/GPU encoders (VideoToolbox, Media Foundation, NVENC, QSV, AMF, VAAPI)**: the manufacturer/OS in theory has already licensed it when distributing the driver/OS. Project risk: much lower. (Interpretation, I did not see the contracts. VAAPI on Linux uses Mesa/Intel drivers; the licensing status in free distros is different, NOT VERIFIED.)
3. **WebCodecs in the browser/webview**: the browser is the licensee (Chrome/Edge/Safari); the project does not distribute a codec. FACT: plain Chromium does **not** ship proprietary codecs; AAC and H.264/HEVC are "Limited to Google Chrome" (<https://www.chromium.org/audio-video/>, accessed 2026-11-14), and Chromium's FFmpeg is licensed bsd/isc/lgpl-2.1/mit (README.chromium). In Electron/webview without proprietary codecs, WebCodecs H.264 may not work: NOT VERIFIED (Electron's FAQ does not address the topic). In practice, using WebView2 (Windows, Edge), WKWebView (macOS), WebKitGTK (Linux, depends on the distro's GStreamer) via Tauri/Wails puts the codec in the **OS runtime**, not in your package.

### 3.6 How open source projects handle it

(FACT only where there is a URL; otherwise marked.)

| Project | Project's own license (FACT) | What is known about codec/patent |
|---|---|---|
| OBS Studio | GPLv2+ (README) | x264 included; patent policy NOT VERIFIED (I did not open a dedicated page) |
| Shotcut | GPLv3 (README, COPYING) | uses MLT/FFmpeg; policy NOT VERIFIED |
| MLT | LGPL-2.1 (GitHub API) | framework of Shotcut/Kdenlive |
| Kdenlive | NOT VERIFIED (README has no license) | uses MLT/FFmpeg |
| HandBrake | GPLv2 (README) | includes x264; no page read on patents |
| Audacity | GPLv3 (README) | optional FFmpeg downloaded separately (not confirmed in this session) |
| Blender | NOT VERIFIED | |
| Firefox | n/a | OpenH264 downloaded from Cisco at runtime (Mozilla blog 2013, via search; direct page 404) |
| Chromium vs Chrome | BSD/LGPL in ffmpeg (README.chromium) | proprietary codecs only in Chrome (chromium.org/audio-video) |
| Electron | MIT (GitHub API) | proprietary codecs: NOT VERIFIED |
| Tauri (Apache-2.0), Wails (MIT) | GitHub API | use the OS webview; codec stays in the OS (INFERENCE) |

- FACT relevant: **all** the open source video editors above are GPL/LGPL. None is MIT. Their practice is not a direct precedent for an MIT project that wants to avoid copyleft.
- INFERENCE: GPL projects that distribute x264 solve **copyright** (GPL), not **patents**; in practice they coexist with patent risk. I did not verify their official statements.

---

## 4. Ready-made FFmpeg builds

| Build | License (FACT) | Content/note |
|---|---|---|
| **BtbN/FFmpeg-Builds** (<https://github.com/BtbN/FFmpeg-Builds>) | MIT repository; gpl / **lgpl** / nonfree variants, static and shared; `lgpl` = `--enable-version3` (**LGPLv3**, COPYING.LGPLv3); `gpl` = `--enable-gpl --enable-version3` | lgpl without x264/x265/fdk; **with** libass, harfbuzz/freetype/fribidi (scripts 25-fribidi, 45-harfbuzz, 50-freetype, `50-libass`), libvpx, svt-av1, aom, rav1e, openh264, vvenc, kvazaar, opus, mp3lame, nvenc(ffnvcodec), AMF, oneVPL(QSV). Daily, 12:00 UTC; branches 8.1 and 9.0 + master. Sizes today: win64-lgpl 167.5 MiB (shared 77.9); linux64-lgpl 133.9 MiB (shared 62.9); win64-gpl 192.6 MiB |
| **gyan.dev** | "all builds are 64-bit, static and GPLv3" (<https://www.gyan.dev/ffmpeg/builds/>) | GPL only; essentials and full; release 8.1.2 |
| **johnvansickle** | "licensed under GPL v3" (<https://johnvansickle.com/ffmpeg/>) | GPL only; static Linux |
| **evermeet.cx** | configure contains `--enable-gpl --enable-version3 --enable-libx264 --enable-libx265` etc. | GPL, mac; also `--enable-libopenh264` |
| **ffmpeg-static (npm)** | GPL-3.0 repo (GitHub API); downloads binaries from gyan.dev, johnvansickle, evermeet, osxexperts | therefore **GPL** |
| **jellyfin-ffmpeg** | "mainly LGPL, optional GPL parts" (README); Jellyfin builds use GPL | configure detail NOT VERIFIED |
| **linuxserver/ffmpeg** | example uses `-c:v libx264` | GPL (INFERENCE) |
| **jrottenberg/ffmpeg** | configuration contains `--enable-gpl --enable-nonfree --enable-libfdk_aac --enable-libx264/x265` | **nonfree, not redistributable** |
| **mwader/static-ffmpeg** | includes libx264; fdk-aac only with build-arg | GPL (INFERENCE) |
| **Alpine** | `license="GPL-2.0-or-later AND LGPL-2.1-or-later"`; `--enable-gpl --enable-libx264 --enable-version3` | GPL |
| **Debian/Ubuntu `ffmpeg`** | NOT VERIFIED (salsa.debian.org and packages.* did not return the data) | general knowledge: GPL with libx264. Confirm |

- INFERENCE: **the only ready-made, maintained source that is LGPL in fact is the BtbN `lgpl` variant** (and your own builds). Third-party `ffmpeg:latest` from Docker Hub and `ffmpeg-static` are GPL (or nonfree): using them in a published Dockerfile or in an installer makes the artifact GPL.
- Dockerfile (INFERENCE): download the BtbN `linux64-lgpl` (or `linuxarm64-lgpl`) tarball with a fixed version (e.g. `n8.1`) and checksum, copy `ffmpeg`/`ffprobe` into an `oven/bun`/Caddy-based image; include the LGPL text and a link to the source (the BtbN build generates `COPYING.LGPLv3`). Published images = distribution: provide the source offer (link to the BtbN/FFmpeg commit).
- Note on the user's global rule: Docker images with a fixed tag, never `latest`.

## 5. ffmpeg.wasm

- FACT: the `ffmpeg.wasm` repository is MIT, but the package **`@ffmpeg/core`** declares `"license": "GPL-2.0-or-later"` (<https://raw.githubusercontent.com/ffmpegwasm/ffmpeg.wasm/main/packages/core/package.json>) and the build Dockerfile uses `--enable-gpl --enable-libx264 --enable-libx265` (<https://raw.githubusercontent.com/ffmpegwasm/ffmpeg.wasm/main/Dockerfile>). The documentation lists FFmpeg 5.1.4, x264, x265 3.4, libvpx, LAME, Opus, freetype, harfbuzz, libass etc., and says you can build your own core without x264 (<https://ffmpegwasm.netlify.app/docs/overview>). Accessed 2026-11-14.
- INFERENCE: serving `ffmpeg-core.wasm` from an MIT app = **distributing a GPL binary** to browsers: requires offering the corresponding source and licensing that artifact as GPL (the wasm runs isolated, so the JS app stays MIT as an "aggregate", same logic as the sidecar, but the interaction is more intimate: same page/Worker, loaded by JS; grey zone). Plus the H.264/HEVC patent risk distributed to every visitor. Alternative: a custom core build **without** `--enable-gpl` (only libvpx/opus/libvorbis), or prefer WebCodecs/mediabunny.

## 6. Dependency matrix

Usage: SC = sidecar (separate process); LD = dynamic link; EST = static link/JS bundle; SVC = service/network. "MIT compat.?" = we can **depend on and distribute together** without relicensing our code.

| Dependency | License (FACT + source) | Planned use | MIT compat.? | Obligations |
|---|---|---|---|---|
| FFmpeg LGPL build | LGPLv2.1+ (v3+ in BtbN) (legal.html, BtbN) | SC | Yes | checklist 1.2; corresponding source; notices; no EULA with anti-reverse-engineering |
| FFmpeg GPL build | GPLv2+/v3 | SC | Yes as a separate aggregate (FSF FAQ), but the **distributed artifact is GPL** | offer of complete source; GPL on the binary |
| x264 / x265 | GPL (LICENSE.md) | avoid | Do not embed | same; patent |
| fdk-aac | nonfree (LICENSE.md) | avoid | No (not redistributable) | n/a |
| libopenh264 | BSD-2 + Cisco binary patent license | SC (binary downloaded at runtime) | Yes (code) | attribution "OpenH264 ... Cisco"; text in EULA; download from Cisco; user toggle |
| libvpx | BSD-3 | via ffmpeg | Yes | copyright notice |
| SVT-AV1 | BSD-3-Clause-Clear | via ffmpeg | Yes | notice; patent via AOM |
| rav1e | BSD-2 | via ffmpeg | Yes | notice |
| libass | ISC | via ffmpeg (captions) | Yes | notice |
| FreeType | FTL (BSD + credit) or GPLv2; FTL compatible with GPLv3, not GPLv2 (freetype.org/license.html) | via ffmpeg (drawtext) | Yes (choose FTL) | credit clause |
| HarfBuzz | "Old MIT" (COPYING) | via ffmpeg | Yes | notice |
| FriBidi | LGPL-2.1 (GitHub API) | via ffmpeg | Yes (dynamic/sidecar) | source offer for the lib |
| whisper.cpp | MIT | SC or lib | Yes | notice |
| Whisper models (OpenAI) | "code and model weights MIT" (openai/whisper README) | runtime download | Yes | MIT notice. Third-party variants (distil, fine-tunes) have their own license: check each one |
| MLT | LGPL-2.1 | probably not used | Yes if dynamic | LGPL |
| GStreamer | LGPL (FAQ) | probably not used | Yes | LGPL; GPL plugins ("ugly/bad") separately |
| **Remotion** | its own license, source-available, "not open source" (docs/license/faq). Free for individuals and companies up to 3 people or NGOs; Automators US$ 0.01/render (min. US$ 100/month); Creators US$ 25/seat/month; Enterprise from US$ 500/month (docs/license/pricing) | **avoid as a dependency of an MIT product** | **No** (a fork/clone that depends on it imposes the Remotion license on whoever uses it); allowed for SaaS to generate videos from its own templates, **forbidden** to let users submit their own Remotion project for render | Legal question; prefer Revideo/Motion Canvas |
| Revideo | MIT (LICENSE) | optional | Yes | notice |
| Motion Canvas | MIT | optional | Yes | notice |
| PixiJS | MIT | EST (preview) | Yes | notice |
| **mediabunny** | **MPL-2.0** (README, GitHub API) | EST (JS bundle), WebCodecs | Yes: MPL is **file-level** copyleft: if you modify mediabunny files, publish those changes; the rest of the app stays MIT | keep headers; do not use the brand |
| mp4box.js | BSD-3-Clause (GitHub API) | EST | Yes | notice |
| **GSAP** | standard "No Charge" license (Webflow) in force 2025-04-30/05-30: free for commercial use, including plugins that were previously paid; **prohibits** a product that offers animation creation via a visual builder competing with Webflow without written consent (<https://gsap.com/standard-license/>) | EST (JS) | **Risk**: is a video editor with a visual animation timeline a candidate for "Competitive Product"? INFERENCE, ask a lawyer / GSAP. MIT alternatives | do not remove notices; do not reverse-engineer |
| Lottie (lottie-web, dotlottie-web) | MIT | EST | Yes | notice |
| Google Fonts | mostly OFL, part Apache/Ubuntu (NOT VERIFIED: the <https://fonts.google.com/faq> page returned no text) | embed in the app/repo | Yes, **keeping each font's license** | OFL: keep notice; do not sell the font alone; rename if modified ("Reserved Font Name"). Text in a generated video: use of the output is usually not restricted by the OFL (INFERENCE; confirm) |
| Pixabay (music/SFX/video) | Pixabay Content License: free use, no attribution, modifiable; **forbidden to sell/distribute standalone** (without creative effort), use as a trademark, etc.; **nothing said about repositories** (<https://pixabay.com/service/license-summary/>) | template assets | **Do not use as assets in the MIT repo**: the license is not open source (INFERENCE): redistributing raw files in a public repo is "standalone distribution" | prefer **CC0** (OpenGameArt, Freesound CC0, Kenney) or original assets; keep a record of origin/license per file |
| Icons | Lucide/Heroicons/Material Symbols (ISC/MIT/Apache licenses; NOT verified in this session) | EST | Probably yes | confirm |

## 7. Recommendation (OPINION)

### (a) Server-side export (SaaS and self-host)

1. Use **ffmpeg as a sidecar** (CLI), never linked.
2. Standard Docker image with an **LGPL** build (BtbN `lgpl`, fixed version), with the source offer and notices; it is the only option the project can **publish** without making the artifact GPL.
3. H.264 encoder layers, in this order: (i) the host's hardware if it exists (NVENC/QSV/VAAPI; AMF) with runtime detection; (ii) `libopenh264` **only** for self-host and non-remunerated use, with the binary downloaded from Cisco's server by the operator (conditions in 2.1); (iii) no software H.264 → **export VP9/AV1** (libvpx/SVT-AV1) for web/YouTube and guide self-hosters to swap the ffmpeg path for their own GPL build (document as "bring your own x264", without distributing it).
4. **For a paid SaaS** that delivers software H.264: its own patent risk (3.1, OpenH264 does not cover paid use). Business decision + lawyer: license via Via LA or use only hardware/AV1/VP9. Probably acceptable to start with the "<100 thousand units" path only with advisory support.
5. Only use filters from the LGPL build (check the GPL list in 1.1: `eq`, `hqdn3d`, `delogo`, `boxblur` etc. are excluded).

### (b) Desktop export

1. Prefer **OS/GPU encoders** (VideoToolbox, Media Foundation/`h264_mf`, NVENC/QSV/AMF/VAAPI) via LGPL sidecar or native APIs; package the LGPL ffmpeg **shared** or static as a separate process + source + notices.
2. Do not embed x264/x265/fdk-aac. Do not embed OpenH264 in the installer (loses Cisco's coverage); if used, download at runtime with a toggle and attribution.
3. HEVC only via hardware/OS. AAC via the OS encoder when available (to avoid the FFmpeg native encoder and the Via LA question in 3.3), otherwise native AAC + lawyer.
4. Offer VP9/AV1+Opus in WebM/MP4 as a free alternative.

### (c) Preview

- Use **WebCodecs** (mediabunny/mp4box.js) and `<video>` in the browser/webview: the codec belongs to the browser/OS. Generate **proxies** in VP9/AV1 or H.264 from the browser itself. Avoid GPL ffmpeg.wasm (section 5). No software H.264 encoding on the client.

### Default "social networks" export preset (OPINION, numbers with indicated source)

All with **YouTube** as the source (the only complete official spec I managed to read: <https://support.google.com/youtube/answer/1722171>, accessed 2026-11-14); the rest of the networks remained **without a verified official spec** (2.4), so the preset is the conservative common denominator.

| Parameter | Value | Source |
|---|---|---|
| Container | MP4, moov at the start (`-movflags +faststart`), no edit lists | YouTube |
| Video | H.264 (via hardware/OS encoder or OpenH264/x264 per policy) | YouTube; Instagram/TikTok/X/WhatsApp/Kwai: third-party consensus, not official |
| Profile/level | High (YouTube recommends); if the encoder only does Main/Baseline, use what the encoder allows; level 4.1 (1080p30) to 4.2 (1080p60): INFERENCE, not an official number | YouTube (profile); level: inference |
| pix_fmt | yuv420p (4:2:0) | YouTube |
| fps | keep the timeline fps: 24/25/30/48/50/60; deinterlace | YouTube |
| GOP | closed; size = half the fps; 2 B-frames; CABAC | YouTube (only x264/hw that support B-frames; OpenH264 not verified) |
| Bitrate (SDR, 24 to 30 / 48 to 60 fps) | 1080p 8 / 12 Mbps; 1440p 16 / 24; 2160p 35 to 45 / 53 to 68; 720p 5 / 7.5 | YouTube |
| Vertical 1080x1920 bitrate for networks | use the 1080p value from the table (8 Mbps at 30 fps). TikTok requires ≥ 516 kbps, ≤ 500 MB, ≤ 10 min (ads) | YouTube; TikTok ads |
| Rate control mode | VBR (YouTube); CRF only with x264 | YouTube |
| Audio | AAC-LC, 48 kHz, stereo, 384 kbps (YouTube recommends 384 stereo; 128 kbps usually suffices for networks: INFERENCE) | YouTube |
| Color | BT.709 primaries/transfer/matrix, limited range (`-colorspace bt709 -color_primaries bt709 -color_trc bt709 -color_range tv`); avoid RGB matrix | YouTube |
| HDR | outside the default preset | YouTube (other page, not read) |

## 8. Gaps and points for a lawyer

1. Whether a **SaaS that encodes H.264 on the server** is a "unit/encoder" under Via LA, and whether a Via LA license applies to the operator even without distributing a binary.
2. Whether **OpenH264 downloaded at runtime by an MIT app** really inherits Cisco's coverage (it requires non-remunerated use and a binary "not combined").
3. Effect of the "100,000 units" exemption for an open source project with diffuse downloads, and whether it is worth joining the pool.
4. **Native AAC encoder** in a distributed product (initial fee US$ 15,000 / US$ 1,000 small entity; exact scope).
5. HEVC: fees and exemption for software not verified; Access Advance/Via LA change (Dec 2025) in progress.
6. **GSAP** vs a visual animation editing product (Webflow clause).
7. **Remotion** as a dependency of an MIT product: application to self-hosters and users at companies with more than 3 people.
8. Grey zone of **GPL ffmpeg.wasm** served to browsers and of a Docker image with GPL ffmpeg.
9. Content licenses of templates (Pixabay x MIT repo) and of fonts (OFL in generated video).
10. Not verified for lack of an accessible primary source: official spec for Instagram, X, WhatsApp, Kwai; official acceptance of AV1/VP9 on any network; supported profiles and quality of OpenH264; VideoToolbox/AMF/VAAPI sections of the FFmpeg doc; Debian/Ubuntu `ffmpeg` package; patent policy of OBS/HandBrake/Shotcut/Kdenlive/Blender/Audacity; Electron/Tauri/Wails and codecs; LGPLv3 license (installation in "User Products"); Sisvel/Avanci/Access Advance fees; Opus/libaom/icon licenses.

## 9. Sources (all accessed 2026-11-14)

- <https://ffmpeg.org/legal.html>
- <https://raw.githubusercontent.com/FFmpeg/FFmpeg/master/LICENSE.md>
- <https://ffmpeg.org/ffmpeg-codecs.html>
- <https://www.openh264.org/BINARY_LICENSE.txt>
- <https://www.via-la.com/licensing-programs/avc-h-264/>
- <https://www.via-la.com/licensing-programs/aac/>
- <https://accessadvance.com/licensing-programs/hevc-advance/>
- <https://aomedia.org/license/patent-license/>
- <https://meta.wikimedia.org/wiki/Have_the_patents_for_H.264_MPEG-4_AVC_expired_yet%3F>
- <https://support.google.com/youtube/answer/1722171>
- <https://ads.tiktok.com/help/article/video-ads-specifications>
- <https://www.gnu.org/licenses/gpl-faq.html>
- <https://github.com/BtbN/FFmpeg-Builds> (README, `variants/`, `scripts.d/`, releases/tag/latest)
- <https://www.gyan.dev/ffmpeg/builds/> ; <https://johnvansickle.com/ffmpeg/> ; <https://evermeet.cx/ffmpeg/>
- <https://raw.githubusercontent.com/eugeneware/ffmpeg-static/master/README.md>
- <https://raw.githubusercontent.com/jrottenberg/ffmpeg/main/README.md> ; <https://raw.githubusercontent.com/wader/static-ffmpeg/master/README.md> ; <https://raw.githubusercontent.com/linuxserver/docker-ffmpeg/master/README.md> ; <https://raw.githubusercontent.com/jellyfin/jellyfin-ffmpeg/jellyfin/README.md>
- <https://git.alpinelinux.org/aports/plain/community/ffmpeg/APKBUILD>
- <https://raw.githubusercontent.com/ffmpegwasm/ffmpeg.wasm/main/packages/core/package.json> ; .../main/Dockerfile ; <https://ffmpegwasm.netlify.app/docs/overview>
- <https://www.chromium.org/audio-video/> ; Chromium `third_party/ffmpeg/README.chromium`
- <https://www.remotion.dev/docs/license/pricing> ; <https://www.remotion.dev/docs/license/faq>
- <https://gsap.com/standard-license/>
- <https://github.com/Vanilagy/mediabunny>
- <https://freetype.org/license.html> ; <https://raw.githubusercontent.com/harfbuzz/harfbuzz/main/COPYING>
- <https://raw.githubusercontent.com/openai/whisper/main/README.md>
- <https://pixabay.com/service/license-summary/>
- Licenses via GitHub API (`/repos/<repo>/license`): SVT-AV1, rav1e, libvpx, whisper.cpp, libass, mp4box.js, lottie-web, pixijs, motion-canvas, MLT, mediabunny, openh264, wails, tauri, electron, fribidi; revideo (LICENSE raw: MIT)
- Secondary search: ipfray.com / streetinsider (Access Advance + Via LA, 2025-12-15); Sisvel/Streaming Media (AV1, Avanci)
- Attempted without success (404/403/empty): help.instagram.com, facebook.com/business/help, help.x.com, faq.whatsapp.com, blog.mozilla.org (OpenH264 2013), via-la essentiality overview, direct sisvel.com AV1, fonts.google.com/faq, wiki.mozilla.org OpenH264
