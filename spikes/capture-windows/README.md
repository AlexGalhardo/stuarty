# Spike: Windows Graphics Capture into fragmented MP4

Epic 61, task 1. Disposable: it is never promoted to product code. The helper of task 6 is written
again in `capture/`.

Question: can a native Windows helper capture 1920x1080 at 60 fps into crash-safe fragmented MP4
through the pinned LGPL FFmpeg, and what does it cost?

All numbers below are **provisional under load**. They were taken on the author's working machine
while other agents kept it between 53% and 100% busy (average 72% to 83% during the long runs).
NFR-067 is judged only on an idle machine and is not judged here.

## What it does

- Opens one borderless 1920x1080 window of its own, with a scrolling pattern and a clock, and
  captures only that window by its unique title. It never captures the desktop, another
  application, a microphone, system audio or a webcam, and keeps no frame of anything else.
- Captures with `windows-capture` 2.0.1 (Windows Graphics Capture), BGRA, no cursor, no border.
- Writes one raw frame per slot of an absolute 60 fps grid into an FFmpeg child (`rawvideo` on a
  pipe). A slot with no new frame repeats the previous one and is counted as a dropped frame, so
  the file keeps its real length.
- FFmpeg encodes with `h264_nvenc` and writes `-movflags +frag_keyframe+empty_moov+default_base_moof`.
- Writes a statistics line every 5 s and a summary at the end, and reads CPU times of its own
  process, of the painting thread and of the FFmpeg child.

Crates: `windows-capture` 2.0.1 and `windows` 0.62.2, both exact. `windows` is already in the graph
of `windows-capture` at that version; it is used directly only to create the test window and to read
CPU times, with features `windows-capture` already enables. `wasapi` and `cpal` are not used (audio
is epic 62). `scap` was not needed. Nothing else was added.

## Reproduce

```bash
export CARGO_TARGET_DIR=<scratch>/target
cd spikes/capture-windows && cargo build --release
$CARGO_TARGET_DIR/release/capture-windows-spike.exe --out out/full10.mp4 --seconds 600 \
  --ffmpeg <lgpl-ffmpeg>/ffmpeg.exe --log out/full10.log --pos 2559,0
ffprobe -v error -count_frames -show_entries stream=nb_read_frames,duration -of default=nw=1 out/full10.mp4

# kill once mid-way: -Mode tree kills helper and FFmpeg, -Mode helper kills only the helper
powershell -File run-kill-test.ps1 -Spike <exe> -Ffmpeg <ffmpeg> -OutDir out -KillAfter 300 -Seconds 600 -Mode tree
```

`--pos 2559,0` leaves one pixel of the test window on the right edge of a 2560 px wide display. A
window fully outside every display receives no frames from Windows Graphics Capture (measured: one
frame in 12 s), so a position with at least one visible pixel is required.

Machine: AMD Ryzen 7 5700X3D (16 logical cores), NVIDIA GeForce RTX 4060, display 2560x1080 at
74 Hz, Windows 11 (10.0.26300), FFmpeg `n9.0.2-39-g7adfbe40dc-20261031` (LGPL build).

## Results

### Ten minutes, final settings

| Measure | Value |
| --- | --- |
| Capture time / file duration (`ffprobe`) | 600.03 s / 601.28 s (the file runs about 1.2 s past the timer: the writer keeps going while the capture stops) |
| Frames in the file (`ffprobe -count_frames`) | 36,077, H.264, 1920x1080, `r_frame_rate` 60/1, 5.3 MB |
| Slots with a new frame / repeated frame | 35,236 / 841 |
| **Dropped frames (repeated slots)** | **841 = 2.33%** (2.07% after the first 5 s; the first 5 s lost 9) |
| Frames Windows delivered | 44,365 of 45,033 painted (1.5% missing); about 74 per second, so about 20% more than the 60 needed |
| Arrival gaps of 20 ms or more | 662 (1.5% of arrivals) |
| Longest write into the pipe | 153.8 ms, one write over 100 ms, none over 1 s |
| CPU of the helper (capture and copy) | 23.7% of one core (1.5% of the machine); 2.28 ms of copy per frame |
| CPU of FFmpeg with `h264_nvenc` | 24.0% of one core |
| System load during the run | 72% on average (53% to 100% per 5 s) |

**NFR-067 (at most 1% dropped) is not met on this machine under this load**, and cannot be judged
here. The rate did not follow the load: 1.9% in intervals under 70% load, 2.1% at 70% to 90%, 2.4%
above 90%.

### Kill once, mid-way

Second run, planned for 10 minutes, killed with `taskkill /T /F` on the helper and its FFmpeg child at
300.25 s after the spawn (capture time about 299.3 s):

- `ffprobe` opens the file with no error: H.264 1920x1080, **299.0 s, 17,940 frames**.
- FFmpeg's own last progress line before the kill said 17,942 frames: two frames lost.
- Statistics before the kill: 17,666 slots, 454 repeated (2.57%).

Killing only the helper (a crash of the helper, FFmpeg still alive): FFmpeg sees the end of its
input and finishes by itself. A 40 s run gave a complete 38.98 s file, 2,339 frames.

What made the tree kill lose almost nothing (40 s runs, kill at 39 s of capture):

| FFmpeg output options | Recovered |
| --- | --- |
| `frag_keyframe` only (first version) | 33.0 s (lost about 6 s) |
| plus `-flush_packets 1` | 35.0 s (lost about 4 s) |
| plus `-delay 0 -rc-lookahead 0 -bf 0` on NVENC | 39.0 s (lost under 0.5 s) |

The first full-length kill (without these options) recovered 288.0 s of about 299 s. The NVENC
latency and the muxer buffering, not the fragment length, decide the tail you lose. The product
must set them explicitly.

### Encoders without NVIDIA (60 s each, same pipeline)

| Encoder | FFmpeg CPU | Dropped slots | Note |
| --- | --- | --- | --- |
| `libopenh264` | 99% of one core | 3.5% | opens in `ffprobe`; software fallback |
| `h264_mf` | 142% of one core | 4.6% | opens in `ffprobe` |

Both kept up with the pipe, with the 1 s startup cost included. They are under the same load and
are not better or worse than NVENC on drops within the noise.

## What the numbers say

1. **The path works.** Windows Graphics Capture of a 1920x1080 window, a raw BGRA pipe to FFmpeg
   (about 480 MB/s) and fragmented MP4 are enough. CPU is small: about 50% of one core in total
   with NVENC. Nothing needs cgo in the server.
2. **Windows throttles by default.** With `MinimumUpdateIntervalSettings::Default` this build
   delivered about 42 frames/s for a 74 Hz window. Any explicit interval (8 ms was used) lifts it
   to the compositor rate. Setting it is required.
3. **Dropped frames are not caused by CPU cost.** Where they come from, ranked by evidence:
   - the first write into the pipe blocks 150 ms to 600 ms while NVENC starts (9 to 25 repeated
     slots in the first 5 s); the product should warm the encoder before the grid starts;
   - gaps of 20 ms or more between deliveries (1.5% of arrivals), about as many as the repeated
     slots. `windows-capture` 2.0.1 creates the frame pool with **one buffer** and its callback is
     synchronous, so a frame still being copied blocks the next one. A patched local copy with four
     buffers gave 1.51% against 3.05% for the original in the only clean pair of 60 s runs
     (sequential, nothing else of mine running). Other pairs overlapped with another run of mine
     and are discarded. This is suggestive, not proven;
   - the pacing grid counts any slot that had no new frame as dropped, which is strict for a 74 Hz
     source that has 20% to spare.
4. **A longer test on an idle machine is needed** before NFR-067 is accepted or rejected. If a pool
   deeper than one buffer is confirmed to matter, task 6 needs a thin binding of its own over the
   `windows` crate (already in the graph) instead of the `windows-capture` handler. That is a
   decision for the author, because ADR-0049 approves the crate, not a replacement.
5. `Frame::buffer()` allocates a staging texture on every frame. It costs 2.3 ms per frame at
   1080p (14% of a core), acceptable, but 4K would need a reused texture.

## Antivirus

Windows Defender cannot be judged on this machine: `Get-MpComputerStatus` reports the service and
real-time protection off, `Get-MpThreatDetection` is empty, and the Defender event log has no event
during any run (the last entries are from 2025). `SecurityCenter2` also lists Kaspersky and Avast
entries; whether either is active was not checked. No security setting was changed. The unsigned executable was built locally and has no Mark of the Web, so SmartScreen does
not apply to it. **Result: inconclusive.** A machine with Defender on, and a downloaded (marked)
copy of the executable, are needed.

## Not covered

- Idle-machine numbers (NFR-067).
- Audio, webcam and cursor events (epic 62); `wasapi` and `cpal` are unused here.
- Capture of a real display or of a window of another application: forbidden for this spike.
- Memory use of the helper and of FFmpeg: not measured.
- Windows 10 behaviour: the border option needs Windows 11 or 10 2104.
- A 4K or a higher refresh display.
