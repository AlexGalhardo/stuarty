# Spike: pure-Go compositor (decoder ffmpeg -> Go compose -> encoder ffmpeg)

Disposable spike for Vukem Video planning, 2026-11-14. Builds on `../export-pipeline/`.

## Hypothesis

Compositing in pure Go (video layers, text bitmap, opacity, colour matrix) is fast enough at 1080x1920 30 fps: >= 1.5x realtime for one full-frame video plus text (C1), >= 1x for a heavier case.

## Method

Everything is reproduced by `run.sh` (`export LGPL_FFMPEG=<BtbN ffmpeg.exe>`, `WORK=<scratch dir>`; ffmpeg with libx264 on PATH is used only to generate the two fixtures). Source: `main.go` (pipeline, timers, validation), `compose.go` (compositing), `mem_windows.go` (peak memory). Only dependency: `golang.org/x/image` v0.48.0.

- Fixtures (`compositor gen-source`, libx264 veryfast crf 18 g 15, AAC): portrait 1080x1920 testsrc2 + 440 Hz sine, landscape 1920x1080 testsrc2 + 660 Hz sine, both 30 s / 30 fps.
- Decoders: `ffmpeg -i <src> -an [-vf ...] -f rawvideo -pix_fmt rgba pipe:1`. Encoder: `ffmpeg -f rawvideo -pix_fmt rgba -s 1080x1920 -framerate 30 -i pipe:0 -i portrait.mp4 -map 0:v -map 1:a -c:v <enc> -b:v 8M -g 60 -pix_fmt yuv420p -r 30 -c:a aac -b:a 192k -movflags +faststart out.mp4`. Encoders: `h264_nvenc`, `libopenh264`. Null variants: `-f null -` (`null`) and `-pix_fmt yuv420p -f null -` (`nullconv`).
- Go pipeline: one reader goroutine per decoder, one compose goroutine, one writer goroutine (main), 4-buffer pools. Timers (cumulative): read = time inside `io.ReadFull` on the decoder pipe (includes waiting for ffmpeg), compose = time inside the compose call, write = time inside the encoder `Write`. Stages overlap, so they do not sum to wall time.
- Wall = decoders+encoder start to encoder exit. Realtime = 30 s / wall. Peak memory = `PeakWorkingSetSize` of the Go process from `GetProcessMemoryInfo` (psapi); ffmpeg children are not included.
- Cases: C1 copy of the 1080x1920 frame + 900x180 text (previous spike's text, alpha-blended, moves and fades). C2 landscape source with `scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920` and `-sws_flags bicubic+accurate_rnd+full_chroma_int` in the decoder. C3 = C1 + pip 432x243 (decoder `scale=432:243`, same sws flags), 80% opacity (205/256), position changes every frame. C4 = C3 but pip delivered 1920x1080 and scaled (0.2 to 0.3) and rotated 10 degrees in Go with `x/image/draw` `Transform` (Src into a 640x440 scratch, then 80% blend); BiLinear and ApproxBiLinear. C5 = C1 with a 3x4 Q10 integer colour matrix (brightness +0.05, contrast 1.1, saturation 1.2) applied to every pixel during the copy.
- `-sws_flags` is applied only to the C2 background and C3 pip decoders. The `-allsws` C1 runs apply it to the C1 decoder too.
- Banded variants: 8 goroutines over horizontal bands, spawned per frame. For C4 the transform is also banded by sub-images of the scratch buffer.
- Validation: same ffprobe + moov-before-mdat checks as the previous spike. Determinism: SHA-256 of every composed RGBA frame, null encoder, two runs.

## Environment

AMD Ryzen 7 5700X3D (16 threads), RTX 4060, 31.9 GB, Windows 11 Pro 10.0.26300, Go 1.27.0. LGPL ffmpeg BtbN `n9.0.2-57-gd838f9f9ae-20261114` (from the previous spike); fixtures generated with PATH gyan ffmpeg 9.0.2 (libx264).

## Results

All columns as measured (`results/runs.jsonl`). Breakdown column = read / compose / write seconds. Rows in run order; for C1 the second and third `bands=1` rows at the end of each encoder block are the `-allsws` experiment (see rows with the same case after the banded ones); null rows are in the order null, nullconv, null banded x8, null allsws.

| case | encoder | sampler | bands | run | wall s | realtime x | fps | peak RSS MB | read / compose / write s | ffprobe valid |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| c1 | h264_nvenc | - | 1 | 1 | 14.3 | 2.1 | 62.9 | 93.2 | 13.5 / 1.9 / 2.7 | true |
| c2 | h264_nvenc | - | 1 | 1 | 20.95 | 1.43 | 43 | 93 | 20.5 / 6 / 2.7 | true |
| c3 | h264_nvenc | - | 1 | 1 | 16.02 | 1.87 | 56.2 | 95.9 | 17.6 / 3 / 2.5 | true |
| c5 | h264_nvenc | - | 1 | 1 | 16.96 | 1.77 | 53.1 | 93.2 | 12.5 / 16.1 / 2.1 | true |
| c4 | h264_nvenc | BiLinear | 1 | 1 | 82.35 | 0.36 | 10.9 | 127.2 | 25.6 / 81.5 / 2.1 | true |
| c4 | h264_nvenc | ApproxBiLinear | 1 | 1 | 18.25 | 1.64 | 49.3 | 127.4 | 27.5 / 16.2 / 2.1 | true |
| c1 | h264_nvenc | - | 8 | 1 | 14.29 | 2.1 | 63 | 96.3 | 13.8 / 1.8 / 2.4 | true |
| c3 | h264_nvenc | - | 8 | 1 | 13.66 | 2.2 | 65.9 | 99.3 | 15 / 2.1 / 2.4 | true |
| c4 | h264_nvenc | BiLinear | 8 | 1 | 32.12 | 0.93 | 28 | 133.6 | 43 / 30.1 / 2 | true |
| c5 | h264_nvenc | - | 8 | 1 | 11.83 | 2.54 | 76.1 | 96.4 | 11.4 / 4.1 / 1.7 | true |
| c4 | h264_nvenc | ApproxBiLinear | 8 | 1 | 16.32 | 1.84 | 55.1 | 131.9 | 30.1 / 6.3 / 1.9 | true |
| c1 | h264_nvenc | - | 1 | 1 | 13.7 | 2.19 | 65.7 | 93.1 | 13.2 / 2 / 2.4 | true |
| c1 | libopenh264 | - | 1 | 1 | 13.62 | 2.2 | 66.1 | 93 | 13.6 / 2 / 1.9 | true |
| c2 | libopenh264 | - | 1 | 1 | 18.21 | 1.65 | 49.4 | 93.1 | 18.1 / 6.8 / 1.9 | true |
| c3 | libopenh264 | - | 1 | 1 | 14.62 | 2.05 | 61.6 | 95.9 | 16.5 / 2.7 / 3.4 | true |
| c5 | libopenh264 | - | 1 | 1 | 17.02 | 1.76 | 52.9 | 93 | 14.1 / 15.3 / 1.8 | true |
| c4 | libopenh264 | BiLinear | 1 | 1 | 96.42 | 0.31 | 9.3 | 127.3 | 33.3 / 96.2 / 1.4 | true |
| c4 | libopenh264 | ApproxBiLinear | 1 | 1 | 19.8 | 1.52 | 45.5 | 127.1 | 28 / 16.2 / 1.8 | true |
| c1 | libopenh264 | - | 8 | 1 | 11.66 | 2.57 | 77.2 | 96.5 | 11.5 / 1.7 / 3.7 | true |
| c3 | libopenh264 | - | 8 | 1 | 13.98 | 2.15 | 64.4 | 99.3 | 15.4 / 2.1 / 3.7 | true |
| c4 | libopenh264 | BiLinear | 8 | 1 | 28.86 | 1.04 | 31.2 | 133.7 | 30.6 / 28.4 / 1.6 | true |
| c5 | libopenh264 | - | 8 | 1 | 15.56 | 1.93 | 57.9 | 97.1 | 15.2 / 6.8 / 3.1 | true |
| c4 | libopenh264 | ApproxBiLinear | 8 | 1 | 23.82 | 1.26 | 37.8 | 132.3 | 43.9 / 7.9 / 1.9 | true |
| c1 | libopenh264 | - | 1 | 1 | 15.41 | 1.95 | 58.4 | 93.1 | 15.3 / 2.3 / 2.1 | true |
| c1 | null | - | 1 | 1 | 20.15 | 1.49 | 44.7 | 93.1 | 19.9 / 2.3 / 2.1 | n/a |
| c1 | nullconv | - | 1 | 1 | 11.79 | 2.54 | 76.3 | 93.2 | 11.8 / 2 / 1.9 | n/a |
| c1 | null | - | 8 | 1 | 11.06 | 2.71 | 81.4 | 96.5 | 11 / 1.5 / 2.1 | n/a |
| c1 | null | - | 1 | 1 | 11.32 | 2.65 | 79.5 | 93.4 | 11.3 / 1.6 / 1.9 | n/a |
| c1 | h264_nvenc | - | 1 | 2 | 12.73 | 2.36 | 70.7 | 93.2 | 11.9 / 1.5 / 2.9 | true |
| c2 | h264_nvenc | - | 1 | 2 | 18.42 | 1.63 | 48.9 | 93.3 | 17.7 / 6.4 / 2.9 | true |
| c3 | h264_nvenc | - | 1 | 2 | 13.6 | 2.21 | 66.2 | 96.1 | 14.1 / 2.8 / 2.8 | true |
| c5 | h264_nvenc | - | 1 | 2 | 15.84 | 1.89 | 56.8 | 93.2 | 11.2 / 15 / 2 | true |
| c4 | h264_nvenc | BiLinear | 1 | 2 | 90.87 | 0.33 | 9.9 | 127.2 | 26.8 / 90.3 / 1.9 | true |
| c4 | h264_nvenc | ApproxBiLinear | 1 | 2 | 15.81 | 1.9 | 56.9 | 127.1 | 22 / 14.7 / 2.1 | true |
| c1 | h264_nvenc | - | 8 | 2 | 11.54 | 2.6 | 78 | 96.3 | 11.1 / 1.3 / 2.5 | true |
| c3 | h264_nvenc | - | 8 | 2 | 12.42 | 2.41 | 72.4 | 99.2 | 13.5 / 1.8 / 2.6 | true |
| c4 | h264_nvenc | BiLinear | 8 | 2 | 27.18 | 1.1 | 33.1 | 134.1 | 31 / 26.5 / 1.8 | true |
| c5 | h264_nvenc | - | 8 | 2 | 14.48 | 2.07 | 62.2 | 96.6 | 14 / 5.8 / 1.9 | true |
| c4 | h264_nvenc | ApproxBiLinear | 8 | 2 | 21.58 | 1.39 | 41.7 | 132 | 38.1 / 7.1 / 2.1 | true |
| c1 | h264_nvenc | - | 1 | 2 | 18.9 | 1.59 | 47.6 | 93.1 | 17.5 / 2.3 / 3.4 | true |
| c1 | libopenh264 | - | 1 | 2 | 13.61 | 2.2 | 66.1 | 93.1 | 13.5 / 2.1 / 2.1 | true |
| c2 | libopenh264 | - | 1 | 2 | 23.85 | 1.26 | 37.7 | 93.3 | 23.8 / 7.7 / 2.1 | true |
| c3 | libopenh264 | - | 1 | 2 | 12.71 | 2.36 | 70.8 | 95.8 | 12.2 / 2.6 / 9.9 | true |
| c5 | libopenh264 | - | 1 | 2 | 14.76 | 2.03 | 61 | 93.1 | 10.6 / 14.6 / 1.5 | true |
| c4 | libopenh264 | BiLinear | 1 | 2 | 96.33 | 0.31 | 9.3 | 127.3 | 31 / 96.1 / 1.4 | true |
| c4 | libopenh264 | ApproxBiLinear | 1 | 2 | 25.98 | 1.15 | 34.6 | 127 | 39.4 / 19.3 / 2.1 | true |
| c1 | libopenh264 | - | 8 | 2 | 14.67 | 2.05 | 61.4 | 96.4 | 14.5 / 2.1 / 2.3 | true |
| c3 | libopenh264 | - | 8 | 2 | 16.01 | 1.87 | 56.2 | 99.6 | 19 / 2.7 / 2.6 | true |
| c4 | libopenh264 | BiLinear | 8 | 2 | 29.95 | 1 | 30 | 181.7 | 38.6 / 28.7 / 1.6 | true |
| c5 | libopenh264 | - | 8 | 2 | 15.94 | 1.88 | 56.5 | 96.4 | 15.7 / 6.6 / 1.6 | true |
| c4 | libopenh264 | ApproxBiLinear | 8 | 2 | 20.13 | 1.49 | 44.7 | 132.1 | 39 / 6.6 / 1.5 | true |
| c1 | libopenh264 | - | 1 | 2 | 12.95 | 2.32 | 69.5 | 93 | 12.9 / 2.1 / 1.8 | true |
| c1 | null | - | 1 | 2 | 11.41 | 2.63 | 78.9 | 93.3 | 11.4 / 2 / 1.8 | n/a |
| c1 | nullconv | - | 1 | 2 | 11.8 | 2.54 | 76.3 | 93.1 | 11.7 / 2 / 2 | n/a |
| c1 | null | - | 8 | 2 | 10.39 | 2.89 | 86.6 | 96.4 | 10.4 / 1.3 / 1.8 | n/a |
| c1 | null | - | 1 | 2 | 12.17 | 2.46 | 73.9 | 93 | 12.1 / 2 / 1.8 | n/a |

### Determinism

`logs/determinism.txt` (per-frame SHA-256, 900 frames):

- C1 run 1 vs run 2: 0 differing frames. C1 banded x8 vs unbanded: 0 differing.
- C4 BiLinear run 1 vs run 2: 0 differing. C4 ApproxBiLinear run 1 vs run 2: 0 differing.
- C4 BiLinear banded x8 vs unbanded: **2 of 900 frames differ** (hash of hashes 4c355816 vs 0e2fe509). Not investigated; the likely cause is the transform's edge handling at band boundaries on the sub-image, but that is a guess.
- Only one machine; not compared across machines.

## Conclusion

- C1 (common case): target 1.5x **met** on this machine. Plain runs 2.10x to 2.36x (nvenc), 2.20x (openh264, both runs). Single outliers exist (nvenc 18.9 s = 1.59x, null encoder first run 1.49x), so run-to-run noise is large.
- C3: **met** (1.87x to 2.41x). C5: **met** (1.76x to 2.54x). C2: nvenc 1.43x / 1.63x and openh264 1.65x / 1.26x; 1x met, 1.5x not always.
- C4: BiLinear unbanded 0.31x to 0.36x, **not met**. BiLinear banded x8 0.93x to 1.10x (borderline). ApproxBiLinear unbanded 1.15x to 1.90x, banded 1.26x to 1.84x; 1x met.
- Where time goes in C1: compose is about 2 s per 30 s clip, writing 2 to 3 s; read (waiting for ffmpeg decode + yuv420p to RGBA conversion) is 11 to 14 s and sets the limit. Go composition of the common case is cheap; ffmpeg's decode and RGBA conversion is the bottleneck, and the encoder choice barely changes the result (nvenc about equal to openh264, null encoder not much faster).
- Architecture: (1) have the decoder pre-scale layers (C2/C3) whenever sizes are known; C2's scale costs several seconds of decoder time; (2) banding helps compose-heavy cases (C5 compose 15 s to 4 to 7 s; C4) but not C1/C3, where decoding dominates, so banding is optional for simple layers and worthwhile for per-pixel maths; (3) `x/image/draw` `BiLinear.Transform` is too slow (about 90 ms per frame); `ApproxBiLinear` is workable, otherwise a hand-written sampler (and its own banding) is needed; (4) composed frames are deterministic run to run, but banding the x/image transform was not bit-identical.

## Limitations

- One machine, one run pair per configuration; noise is large (for example C1 nvenc 12.7 s vs 18.9 s). No means computed. Other processes may have been running.
- Synthetic testsrc2 content; short GOP 15 fixtures; ffmpeg threading not pinned.
- Peak memory covers the Go process only, not ffmpeg children.
- Read time includes waiting on the decoder, so it is not pure I/O.
- ApproxBiLinear and BiLinear differ in quality; no quality comparison was made. C4 edges are not antialiased (no premultiplied-alpha handling).
- The cause of the 2-frame banded C4 mismatch was not investigated.
- Hardware encoder output varies by driver; encoded bytes were not hashed.
