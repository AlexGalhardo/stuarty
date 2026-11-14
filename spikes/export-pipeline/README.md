# Spike: export pipeline (Go + LGPL FFmpeg)

Disposable spike for Vukem Video planning. Date of measurements: 2026-11-14.

## Hypotheses

- H1. An LGPL FFmpeg build has everything the export needs.
- H2. Go can feed rasterized overlay frames to FFmpeg fast enough (three overlay strategies: S1 full-frame pipe, S2 cropped-region pipe, S3 PNGs placed by `overlay` expressions).
- H3. Composed frames are deterministic (run to run, and LGPL build vs gyan GPL build).
- H4. Non-x264 H.264 encoders have acceptable quality at 8 Mbit/s compared with libx264.

## Method

Everything is reproduced by `run.sh` (`export LGPL_FFMPEG=<path to BtbN ffmpeg.exe>` first). Source in `main.go` (Go, `golang.org/x/image` v0.48.0 for the bundled Go Regular font; `go mod tidy` also pulled `golang.org/x/sys` and `golang.org/x/text` as indirect deps of x/image). `h1.sh` is the H1 probe. Raw outputs are in `logs/` and `results/`.

- LGPL binary: `ffmpeg-n9.0-latest-win64-lgpl-9.0.zip` from `https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n9.0-latest-win64-lgpl-9.0.zip` (the 9.0 stable-branch build; the `master` builds were not used). Reported version: `n9.0.2-57-gd838f9f9ae-20261114`.
- Source fixture (`exporter gen-source`): `testsrc2=s=1080x1920:r=30:d=30` + `sine=f=440:r=48000`, libx264 (PATH, fixture only) `-preset veryfast -crf 18 -g 15`, AAC 128k, 36.9 MB.
- Text layer: Go rasterizes 30 bitmaps (900x180, straight-alpha NRGBA, Go Regular 100 px, 5 px dark shadow), one per second ("Scene NN - Vukem"). Animation: x moves `40 + 60*u` px (u = seconds within the slot), alpha fades in/out over 0.25 s each.
- Common filter prefix: `[0:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,fps=30,format=yuv420p[bg]`.
  - S1: `-f rawvideo -pix_fmt rgba -s 1080x1920 -framerate 30 -i pipe:0`, `[bg][1:v]overlay=0:0:format=yuv420:eof_action=pass,format=yuv420p`. Go clears an 8.3 MB buffer, blits the bitmap, writes it, for each of 900 frames (renderer goroutine + 6-buffer pool).
  - S2: same with a 1080x400 raw frame and `overlay=0:1200`.
  - S3: 30 PNG inputs `-loop 1 -framerate 30 -t 1`, per input `format=rgba,fade=t=in:st=0:d=0.25:alpha=1,fade=t=out:st=0.75:d=0.25:alpha=1,setpts=PTS+K/TB`, then a chain of 30 `overlay=x='40+60*(t-K)':y=1310:eval=frame:eof_action=pass:format=yuv420:enable='between(t,K,K+0.999)'`. Upper bound is K+0.999 (not K+1) so adjacent slots never overlap on the boundary frame. PNG encoding time (about 210 ms) is added to the S3 wall time.
- Encode args: `-c:v ENC -b:v 8M -g 60 -pix_fmt yuv420p -r 30 -map 0:a -c:a aac -b:a 192k -movflags +faststart` (libx264 adds `-preset veryfast`). The LGPL binary was used for everything except the libx264 reference (PATH gyan binary, GPL, not shippable).
- Wall time is measured in Go from process start to exit (includes ffmpeg startup, source decode, scale/crop, overlay, encode, mux, audio). Validation: ffprobe JSON (video+audio, 1080x1920, yuv420p, h264/aac, 30/1 fps, stream and format duration within 1/30 s of 30 s) plus a top-level MP4 box walk (moov before mdat).
- H3: S1 composed frames after the filter graph (yuv420p, before any encoder) are dumped with `-map [v] -f framemd5`; the hash column is compared. Also the sha256 of the raw RGBA bytes Go pipes is recorded.
- H4: lossless reference = S1 composed frames encoded to FFV1 (`ref.mkv`, 140.9 MB); each encoder re-encodes the reference at `-b:v 8M -g 60`; `ssim` and `psnr` filters of the LGPL ffmpeg compare output to the reference.

## Environment

- CPU AMD Ryzen 7 5700X3D 8-core; GPU NVIDIA GeForce RTX 4060 (driver 621.24); RAM 31.9 GB; Windows 11 Pro 10.0.26300; Go 1.27.0.
- PATH ffmpeg: `9.0.2-full_build-www.gyan.dev` (`--enable-gpl --enable-version3 ... --enable-libx264`).
- LGPL ffmpeg: BtbN `n9.0.2-57-gd838f9f9ae-20261114`, configure includes `--enable-version3 --enable-libopenh264 --enable-amf --enable-ffnvcodec --enable-libvpl` and `--disable-libx264 --disable-libx265`; full line in `logs/h1_version_lgpl.txt`. ffprobe from the same zip.

## Results

### H1: capabilities of the LGPL build

- Configure line contains `--enable-version3` but neither `--enable-gpl` nor `--enable-nonfree` (`logs/h1_gpl_check.txt`).
- Encoders listed: `libopenh264`, `h264_mf`, `h264_nvenc`, `h264_qsv`, `h264_amf`, `h264_d3d12va`, `h264_vaapi`, `h264_vulkan`; no `libx264`. AAC: native `aac` and `aac_mf` both listed and both encode a 2 s sine OK.
- 2 s 1080x1920 encode at 8M with each H.264 encoder (`logs/h1_encoder_trials.txt`):

| Encoder | Result on this machine |
| --- | --- |
| h264_mf | WORKS (MFT name 'H264 Encoder MFT', the Microsoft software encoder; ran at about 1x realtime for a 2 s clip including init) |
| libopenh264 | WORKS (statically linked, no extra DLL needed here) |
| h264_nvenc | WORKS (RTX 4060) |
| h264_qsv | FAILED: `Error creating a MFX session: -9.` ... `Could not open encoder before EOF` (no Intel GPU) |
| h264_amf | FAILED: `AMFQueryVersion failed with error 1` / `Failed to create  hardware device context (AMF) : Unknown error occurred` (no AMD GPU) |
| h264_d3d12va, h264_vulkan, h264_vaapi | FAILED with a plain `-vf format=yuv420p` input: `Impossible to convert between the formats supported by the filter 'Parsed_format_0' and the filter 'auto_scale_0'` (dst pixel format `d3d12` / `vulkan` / `vaapi`; they need `hwupload` with a hardware device, which was NOT tried, so this is not a verdict on the hardware) |

- Filters (all 27 requested, checked with `-filters` on both binaries): present in the LGPL build: overlay, scale, zscale, tonemap, xfade, fps, format, setpts, trim, atrim, amix, volume, afade, loudnorm, adelay, concat, pad, crop, transpose, colorspace, gblur, fade, alphamerge, premultiply, blend (25).
  MISSING in the LGPL build (`Unknown filter`; present in the gyan GPL build): **`eq`** and **`boxblur`**. These are the GPL-only ones (eq is GPL-licensed; boxblur is absent from this build too). Substitutes present: `gblur`, `avgblur`, `colorlevels`, `lutyuv`, `hue`, `huesaturation`, `colorbalance`, `curves`, `lut3d`, `colorchannelmixer`. `drawtext`, `subtitles`, `ass` also exist in this build but are not used by design.

### H2: strategies x encoders (run 1 / run 2)

30 s, 900 frames, 1080x1920, 8 Mbit/s target. Realtime factor = 30 s / wall. For S1/S2 "Go render" is the cumulative CPU time of the Go rasterizer goroutine (overlapped with encoding); for S3 it is the PNG pre-render time (already included in wall). All 24 outputs passed validation (video+audio, 1080x1920, yuv420p, h264+aac, duration within 1 frame, moov before mdat).

| Strategy | Encoder | Wall s | Encode fps | Realtime x | Size bytes | Go render s / PNG ms | Valid |
| --- | --- | --- | --- | --- | --- | --- | --- |
| S1 | h264_mf | 5.61 / 5.29 | 160.5 / 170.3 | 5.35 / 5.68 | 30781529 / 30781529 | 1.85 / 1.62 | yes |
| S1 | libopenh264 | 7.58 / 7.64 | 118.8 / 117.7 | 3.96 / 3.92 | 29897777 / 29897777 | 1.02 / 1.03 | yes |
| S1 | h264_nvenc | 4.36 / 4.34 | 206.2 / 207.5 | 6.87 / 6.92 | 31056252 / 31056252 | 1.42 / 1.42 | yes |
| S1 | libx264 (GPL ref) | 5.70 / 5.70 | 157.9 / 158.0 | 5.26 / 5.27 | 30650964 / 30650964 | 1.87 / 2.15 | yes |
| S2 | h264_mf | 3.43 / 3.72 | 262.1 / 241.9 | 8.74 / 8.06 | 30781529 / 30781529 | 0.66 / 0.92 | yes |
| S2 | libopenh264 | 5.62 / 6.34 | 160.2 / 141.9 | 5.34 / 4.73 | 29897777 / 29897777 | 0.38 / 0.40 | yes |
| S2 | h264_nvenc | 2.49 / 2.73 | 361.0 / 330.2 | 12.03 / 11.01 | 31056252 / 31056252 | 0.43 / 0.47 | yes |
| S2 | libx264 (GPL ref) | 3.77 / 3.88 | 239.0 / 231.7 | 7.97 / 7.72 | 30650964 / 30650964 | 0.68 / 0.70 | yes |
| S3 | h264_mf | 4.01 / 4.10 | 224.5 / 219.3 | 7.48 / 7.31 | 30851603 / 30851603 | 222 / 222 | yes |
| S3 | libopenh264 | 6.46 / 7.12 | 139.3 / 126.5 | 4.64 / 4.22 | 29847106 / 29847106 | 212 / 211 | yes |
| S3 | h264_nvenc | 3.21 / 3.28 | 280.3 / 274.0 | 9.34 / 9.13 | 31057157 / 31057157 | 212 / 213 | yes |
| S3 | libx264 (GPL ref) | 3.99 / 4.44 | 225.7 / 202.9 | 7.52 / 6.76 | 30638819 / 30638819 | 212 / 499 | yes |

Raw lines: `results/h2.jsonl`. Output size is bit-identical between runs and between S1 and S2 for every encoder; S3 differs slightly (see H3).

### H3: determinism (`logs/h3_compare.txt`)

| Comparison (900 frames, yuv420p framemd5 hash) | Differing frames |
| --- | --- |
| S1, LGPL run 1 vs LGPL run 2 | 0 |
| S1, LGPL (BtbN) vs gyan (PATH) binary, same graph | 0 |
| S1 vs S2 (LGPL) | 0 |
| S1 vs S3 (LGPL) | 630 |

The Go raw RGBA stream sha256 was identical in all three S1 dumps (`eec6c5a6...638e46`, `results/h3.jsonl`). S1 vs S3 are different compositing implementations (Go integer alpha/rounding and integer x vs ffmpeg `fade` alpha and `overlay` expression coordinate); I did not quantify the pixel magnitude of the difference (only frame-hash equality was checked). The visual check of frame 5.5 s looked the same by eye.

### H4: quality at 8 Mbit/s vs lossless reference

Content is synthetic `testsrc2` (very high detail), so absolute values are low; compare only between rows.

| Encoder | Size bytes | Actual kbit/s | SSIM Y | SSIM All | PSNR avg dB (min) |
| --- | --- | --- | --- | --- | --- |
| h264_mf (software MFT) | 30285206 | 8076 | 0.974384 | 0.965141 | 30.338103 (23.652483) |
| libopenh264 | 29401318 | 7840 | 0.980125 | 0.979724 | 30.415587 (23.651572) |
| h264_nvenc | 30559865 | 8149 | 0.982080 | 0.982116 | 30.451366 (23.618437) |
| libx264 veryfast (GPL ref) | 30154581 | 8041 | 0.982820 | 0.982874 | 30.472151 (23.631754) |

## Conclusion

- H1: **Confirmed with caveats.** The LGPL build (no gpl/nonfree) has 25 of the 27 filters; `eq` and `boxblur` are missing (GPL-only). Architecture impact: do not rely on `eq`/`boxblur`; use `gblur`/`avgblur`, `lutyuv`/`colorlevels`/`hue`/`curves`/`colorbalance` or our own Go processing for brightness/contrast/saturation/blur. Three H.264 encoders work on this machine: `h264_mf` (here the Microsoft software MFT), `libopenh264`, `h264_nvenc`; QSV and AMF fail only because the hardware is absent; d3d12va/vulkan/vaapi were not properly tested (need hwupload). The export must probe encoders at runtime and fall back (nvenc/qsv/amf -> mf -> openh264); the probe must really open the encoder, since all are listed in `-encoders`.
- H2: **Confirmed on this machine.** All 12 strategy/encoder combos finish a 30 s export in 2.5 to 7.6 s (3.9x to 12x realtime), including libx264 reference. S1 (full-frame pipe) is already 3.9x (openh264) to 6.9x (nvenc) realtime; Go rasterizer cost was 1.0 to 1.9 s per 30 s and overlaps with encoding. S2 (cropped region) is 1.4x to 1.6x faster than S1; S3 (PNG + overlay chain, only 30 s of text with 30 inputs) sits between S1 and S2 but gets more expensive with every text item and cannot express arbitrary per-frame animation. Architecture impact: a frame-pipe overlay from Go is viable; send only the dirty region (S2) where the layer bounds are known; S3 is only worth it for simple static-image-with-fade overlays. Only one overlay layer was tested; multiple full-screen layers, blur, transitions and 5-minute exports were not.
- H3: **Confirmed for the pipe path (S1/S2), partly for S3.** Composed frames are bit-identical across runs, between the BtbN LGPL and gyan binaries, and S1 equals S2. S3 differs from S1 in 630 of 900 frames, so S3 is not interchangeable with the pipe path if exact reproducibility vs a preview is needed. Encoded file sizes are identical across runs, but encoder output determinism was not checked with hashes of the MP4 bytes, and hardware encoders (nvenc) may vary across drivers/GPUs.
- H4: **Confirmed (acceptable) with one caveat.** At 8 Mbit/s SSIM All: nvenc 0.9821, openh264 0.9797, libx264 0.9829; nvenc is within 0.0008 of libx264 and openh264 within 0.0031. The software MF encoder is clearly worse: SSIM All 0.9651 (U/V 0.945/0.948) at the same bitrate. Architecture impact: treat `h264_mf` as the last-resort fallback (or tune its rate-control/quality options, untested) and prefer hardware or OpenH264.

## Surprises

- `eq` and `boxblur` are both missing in the LGPL build, not just `eq`.
- `h264_mf` resolved to the Microsoft software encoder despite the NVIDIA GPU, and had the worst SSIM, particularly chroma.
- OpenH264 was the slowest option (3.9x realtime on S1) but its quality is close to libx264 and the result is shippable.
- S1 vs S3 differ in 630 frames although both look identical to the eye.

## Limitations

- Only Windows 11, one machine (Ryzen 7 5700X3D, RTX 4060). Not tested: Linux, macOS (VideoToolbox), Linux without GPU, machines with Intel/AMD GPUs, OpenH264 with an external DLL, h264_d3d12va/vaapi/vulkan with hwupload.
- One 30 s synthetic source (testsrc2 + sine); real footage, HDR (zscale/tonemap were only checked for existence), non-normalized inputs, 5-minute exports, memory use and multiple overlay layers were not measured.
- Timings are two runs each (no mean computed) on a desktop with other processes possibly running; no CPU/GPU utilisation was recorded. The ffmpeg process was not pinned to a thread count.
- The reference quality test uses a synthetic, extremely detailed pattern, ABR (`-b:v`) only, no CRF/CQ modes or encoder tuning (presets, profiles, B-frames).
- S3 only tested with 30 inputs and one animation type; its timeline `enable` upper bound was K+0.999 rather than K+1.
- The audio path was only AAC from a sine tone (no mixing/loudnorm).
- Binaries and videos are not in the repo; they live in the session scratchpad and `work/` (git-ignored).
