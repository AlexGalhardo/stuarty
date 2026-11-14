# Spikes

> Three disposable experiments were run on 2026-11-14 to settle questions that reading could not. Each lives in `spikes/<name>/` with its own README, method, raw results and limitations. This file is the summary: the hypothesis, what was measured, the conclusion and the decision it produced. The code of a spike is never promoted to the product.

All three ran on one machine: AMD Ryzen 7 5700X3D (8 cores, 16 threads), NVIDIA RTX 4060, 32 GB, Windows 11 Pro. That is the main limitation of every number below: one machine, one operating system, synthetic or small inputs, and other processes running at the same time. The Prototype repeats the measurements that matter on Linux and macOS (ROADMAP).

## Summary

| Spike | Question | Answer | Decision it produced |
| --- | --- | --- | --- |
| `export-pipeline` | Is the LGPL FFmpeg build enough, and is export fast and deterministic? | Yes, with two missing filters and one placement strategy that is not pixel-stable | ADR-0011 (codec policy), ADR-0003 item 3 and SDD-004 (pixels are composed in Go) |
| `go-compositor` | Is composing frames in Go fast enough? | Yes for the common case; rotation and scaling need a fast sampler | SDD-004 §12 (targets met, sampler rule, banding rule) |
| `whisper-local` | Is local transcription usable on a CPU, with word timings? | Yes with the `small` model; word timings need repair | ADR-0021 (default model, mandatory post-processing, how to judge success) |

## 1. `export-pipeline`

**Hypotheses.** H1: the LGPL build of FFmpeg has every filter and encoder the product needs. H2: exporting a vertical 1080x1920 video with text rendered by Go is faster than real time. H3: the result is the same on every run. H4: the H.264 encoders available without GPL code give acceptable quality.

**Method.** BtbN build `ffmpeg-n9.0-latest-win64-lgpl-9.0` (version n9.0.2-57). A 30 second synthetic source. Text rasterized in Go and delivered to FFmpeg in three ways: S1, full frames through a pipe; S2, only the overlay region through a pipe; S3, PNG files placed with `overlay` expressions. Per-frame checksums (`framemd5`) for determinism; SSIM against a lossless reference for quality.

**Results.** FACT, measured:

- H1: 25 of the 27 filters checked are present. Missing: `eq` and `boxblur` (they are GPL filters). Encoders that work: `h264_mf`, `libopenh264`, `h264_nvenc`. The native AAC encoder works.
- H2: 30 seconds of video export in 2.5 to 7.6 seconds (3.9 to 12 times real time). With full frames through a pipe: `libopenh264` 7.6 s, `h264_nvenc` 4.3 s, `h264_mf` 5.3 to 5.6 s. Piping only the overlay region is 1.4 to 1.6 times faster.
- H3: 0 of 900 frames differ between runs, between the LGPL build and a GPL build, and between S1 and S2. With S3, 630 of 900 frames differ from S1: sub-pixel placement by `overlay` expressions is not the placement our code computes.
- H4: SSIM at 8 Mbit/s: `libx264` 0.9829 (reference, GPL, not shipped), `h264_nvenc` 0.9821, `libopenh264` 0.9797, `h264_mf` 0.9651. `h264_mf` fell back to Microsoft's software encoder although an NVIDIA GPU was present.

**Conclusion.** The LGPL build is enough. Colour adjustment and blur cannot come from FFmpeg filters in that build, and letting FFmpeg place overlays gives pixels that differ from the plan. INFERENCE: the simplest way to make preview and export agree is for our own code to compose every pixel and for FFmpeg to only decode and encode.

**Decisions.** ADR-0011: ship only the LGPL build; encoder order by measured quality; `h264_mf` is the last resort on Windows. ADR-0003 item 3 and SDD-004: the export executor composes in Go. The BtbN build has OpenH264 compiled in, which is not the arrangement Cisco's patent grant covers: D-017 accepts it for the Prototype and ALPHA and requires our own build before 1.0.

## 2. `go-compositor`

**Hypothesis.** Composing in pure Go (video layers, a text bitmap, opacity, a colour matrix) at 1080x1920 and 30 fps reaches at least 1.5 times real time for one full-frame video plus text, and at least 1 time real time for a heavier case.

**Method.** One FFmpeg process per source decodes to raw RGBA through a pipe; Go composes; a second FFmpeg process encodes. 30 seconds, 900 frames. Five cases, each with `h264_nvenc` and `libopenh264`, two runs each, single-threaded and split in 8 horizontal bands, 60 runs in all. Every encoded file was validated with `ffprobe`. Determinism by SHA-256 of every composed frame.

| Case | Content |
| --- | --- |
| C1 | one 1080x1920 video copied, plus a moving, fading 900x180 text bitmap |
| C2 | a 1920x1080 source scaled and cropped to cover 1080x1920 by the decoder, plus the text |
| C3 | C1 plus a second video scaled to 432x243 by its decoder, at 80% opacity, moving every frame |
| C4 | C1 plus a second video scaled and rotated in Go every frame (`x/image/draw`) |
| C5 | C1 with a colour matrix applied to every pixel |

**Results.** FACT, measured (times real time, lowest and highest of the runs):

| Case | Single-threaded | 8 bands | Compose time for 30 s of video |
| --- | --- | --- | --- |
| C1 | 1.59 to 2.36 | 2.05 to 2.60 | 1.3 to 2.3 s |
| C2 | 1.26 to 1.65 | not run | 6.0 to 7.7 s |
| C3 | 1.87 to 2.36 | 1.87 to 2.41 | 1.8 to 3.0 s |
| C4, `draw.BiLinear` | 0.31 to 0.36 | 0.93 to 1.10 | 81 to 96 s, 26 to 30 s in bands |
| C4, `draw.ApproxBiLinear` | 1.15 to 1.90 | 1.26 to 1.84 | 15 to 19 s, 6 to 8 s in bands |
| C5 | 1.76 to 2.03 | 1.88 to 2.54 | 15 to 16 s, 4 to 7 s in bands |

- Where the time goes in C1: reading decoded frames (which is waiting for FFmpeg to decode and convert YUV to RGBA) takes 11 to 17 s of a 12 to 19 s run; composing takes about 2 s; writing to the encoder 2 to 3 s. With the encoder replaced by a null sink the run is barely faster, and `h264_nvenc` and `libopenh264` give almost the same total.
- Memory of the Go process peaks at 93 to 134 MB (one run reached 182 MB). FFmpeg children are not included.
- Determinism: 0 of 900 frames differ between two runs for C1, C4 with `BiLinear` and C4 with `ApproxBiLinear`. C1 in 8 bands equals C1 single-threaded. **C4 with `BiLinear` in 8 bands differs from the single-threaded result in 2 of 900 frames.** The cause was not investigated; the transform was applied per band on sub-images, and edge handling at band boundaries is the likely reason (a guess).
- Run-to-run noise is large: the same C1 configuration took 12.7 s in one run and 18.9 s in another, with other processes active.

**Conclusion.** The target for the common case is met (the worst single run was 1.59 times real time, typical runs 2.1 to 2.4). The heavier cases C3 and C5 are met. Rotation and scaling in Go is met only with a fast sampler or in bands. INFERENCE: Go composition is not the bottleneck; FFmpeg decoding plus the conversion to RGBA is, so the remaining lever is to exchange frames in YUV 4:2:0, which SDD-004 keeps as its first fallback.

**Decisions** (SDD-004 §12):

1. The Go compositor stays. No fallback is needed for the Prototype.
2. A layer whose size is constant is scaled by its decoder, never in Go (C2 and C3 show the decoder does it within budget).
3. `x/image/draw.BiLinear.Transform` is not used on the per-frame path. Rotation and animated scaling use a sampler written for the purpose (bilinear, fixed-point), whose speed and quality the Prototype measures against `ApproxBiLinear`.
4. Splitting work in bands must not change any pixel. A transformed layer is sampled per output pixel from the whole source, so a band boundary cannot change the result; a test compares one band against eight for every reference scene and fails on any difference.
5. Bands are used for per-pixel work (colour matrix, transforms); they bring nothing to a plain copy.

## 3. `whisper-local`

**Hypotheses.** H1: a prebuilt whisper.cpp binary exists for our platforms. H2: transcription on a CPU is fast enough to wait for. H3: word-level timings are good enough for word-highlight captions. H4: the command-line contract (exit code, progress, output) is usable by a job runner. H5: the text is accurate enough in Portuguese and English.

**Method.** `ggml-org/whisper.cpp` (MIT). Real speech from LibriVox recordings in Portuguese and English, clips of 90 seconds. Models `tiny`, `base`, `small`, `large-v3-turbo` at 4 and 8 threads. Word timings compared with a hand-checked reference.

**Results.** FACT, measured or verified at the source:

- H1: the latest stable release at the time, `v1.9.5`, publishes no prebuilt binaries (0 assets); so does `v1.9.4`. `v1.9.2` is the newest with binaries (9 assets, a Windows x64 CPU archive of 8.2 MB with `whisper-cli.exe`).
- H2: speed at 8 threads, in multiples of real time: `tiny` 17, `base` 13, `small` 4.6, `large-v3-turbo` 0.8 to 1.7. A 60 second video takes about 13 seconds with `small`. The default of 4 threads is slower.
- H3: word timings are not usable as they come. The first word after a pause starts 141 to 367 ms early on average, up to 1.2 s, because each word ends where the next begins. Words of zero length and words longer than a second occur. The DTW option was slower and worse on this measure.
- H4: the program exits with code 0 on corrupt or silent audio and only prints an error. Progress reaches standard error a few times per clip.
- H5: language detection found Portuguese with every model; `small` and above gave usable text on the samples.

**Conclusion.** Local transcription is practical with `small`. Success must be judged by the output file, not the exit code. Word timings need a repair pass. A helper binary cannot be assumed to exist for every platform at the newest tag.

**Decisions** (ADR-0021, SDD-011): default model `small`; `base` as the fast option and `large-v3-turbo` on request; 8 threads or the number of physical cores, whichever is smaller; a mandatory four-step repair of word timings (move the start of a word that follows a pause to the audible onset, enforce a minimum duration, repair zero-length words, derive a confidence); a missing or empty output is a failure; the product pins a tag that has binaries for each platform or builds the helper itself; the helper is installed by an explicit step, never downloaded by a job (SDD-007 §6.8).

## What the spikes did not answer

- Nothing was measured on Linux or macOS, on a machine without a discrete GPU, or with real camera footage (variable frame rate, rotation metadata, HDR).
- Text shaping with `go-text/typesetting` (emoji, right-to-left scripts, speed) was not measured; ADR-0012 leaves it to a Prototype task.
- The preview in the browser (how many `<video>` elements can decode at once, drift correction) was not measured; SDD-004 §10 starts from 4 and the Prototype confirms.
- Quality of the fast sampler against `draw.BiLinear` was not compared.
- Memory of the FFmpeg child processes was not measured.
