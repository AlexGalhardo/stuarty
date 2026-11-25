# SDD-025: Screen recording

- Status: Draft (2026-11-24). Written by epic 61 before its code; extended by epic 62 (section 16: audio, webcam, pause, pointer events) and epic 65 (section 17: automatic zoom, smooth cursor, click highlight).
- Depends on: ADR-0049 (the native capture helper), ADR-0051 (agents never start a capture), ADR-0011, ADR-0032, ADR-0052, SDD-000 (names), SDD-003 (surfaces), SDD-005 (persistence), SDD-007 (threats, section 6 process policy), SDD-011 (import), SDD-013 section 4.9 (the precedent of a native child).
- Implements: PRD-004 section 4.11 (RF-430 to RF-442), NFR-067, NFR-068.
- Owner area: epic 61 (`capture/`, `core/recording/`, `engine/capturehost/`, `service/recording*.go`, the `recordings` table, the adapters, the record dialog).

## Contents

1. Overview
2. Names
3. The state machine
4. The review of a finished recording
5. Limits
6. The helper protocol
7. The host
8. The file
9. Service operations and rules
10. Recovery
11. Surfaces
12. The dialog
13. Threats
14. Measures
15. Tests
16. Audio, webcam, pause and pointer events (epic 62)
17. Finishing a take: automatic zoom, smooth cursor and click highlight (epic 65)

## 1. Overview

A person records one display or one window of Windows into a project. A separate executable, `stuarty-capture`, captures frames with Windows Graphics Capture and pipes them to an LGPL FFmpeg child that writes a fragmented MP4. The server starts and supervises the helper through `engine/capturehost`; the rules live in `core/recording`.

Rules that carry the design:

1. **The helper captures and writes; it decides nothing about a project.** Every rule (states, limits, who may start) is in `core/recording` and the service (ADR-0049 item 3).
2. **Only a person starts a capture.** `RecordingStart` needs a privileged browser session and answers `E_PRIVILEGED_SESSION` to everyone else before any process starts (ADR-0051). No approval rule can lift it.
3. **Stopping is always allowed** to anyone with the `media` scope.
4. **A recording enters the project only when a person confirms it** (ADR-0051 item 7). "Not now" keeps it out of the project and invisible to agents.
5. **A recording survives a crash.** Fragmented MP4, and a recovery at the next start.
6. **Nothing leaves the machine.** The helper opens no connection and records no keystroke.

Where capture does not exist (web, Docker, network mode, macOS, Linux until epics 63, 67 and 68) every operation but the reads answers `E_CAPTURE_UNAVAILABLE` with `details.reason` (`platform`, `web`, `network_mode`, `ffmpeg_missing`, `helper_missing`, `protocol_version`).

## 2. Names

| Kind | Name |
| --- | --- |
| Identifier | `rec_<10 Crockford base32>` (SDD-000 section 2) |
| Table | `recordings` (section 9.4) |
| Operations | `RecordingSources`, `RecordingStart`, `RecordingStop`, `RecordingCancel`, `RecordingGet`, `RecordingList`, `RecordingConfirm`, `RecordingDismiss`, `RecordingPlace` (SDD-000 section 1); `RecordingPause`, `RecordingResume`, `RecordingEvents` (epic 62, section 16) |
| Events | none in epic 61: the dialog reads `RecordingGet` once a second while a recording runs (a stream event is a later addition) |
| Error codes | `E_CAPTURE_UNAVAILABLE`, `E_CAPTURE_FAILED`, `E_RECORDING_NOT_FOUND` (SDD-000 section 8) |
| Job kind shown to MCP | `capture`: a recording read through `job_get` and stopped through `job_cancel` (section 11) |
| Variables | `STUARTY_CAPTURE_HELPER`, `STUARTY_MAX_RECORDING_SECONDS` (SDD-000 section 10) |
| Folder | `<workspace>/recordings/<recording id>/capture.mp4` (SDD-000 section 9) |
| Helper | `stuarty-capture` (`stuarty-capture.exe`), protocol version 1 |

## 3. The state machine

`core/recording` holds a pure function over these states: `prepared`, `recording`, `paused`, `finishing`, `ready`, `failed`, `cancelled`. `ready`, `failed` and `cancelled` are final. `paused` is epic 62 (section 16.4): `recording` goes to `paused` on a pause and back on a resume, and a `paused` recording can be stopped, cancelled or fail like a `recording` one.

| From | Event | To |
| --- | --- | --- |
| `prepared` | the helper reported `started` | `recording` |
| `prepared` | cancel, or a failed start | `cancelled`, `failed` |
| `recording` | stop requested, or a limit reached | `finishing` |
| `recording` | cancel | `cancelled` |
| `recording` | the helper or the encoder ended without a stop | `failed` (or `ready` at recovery, section 10) |
| `finishing` | the helper reported `finished` and the file is readable | `ready` |
| `finishing` | the helper failed, or the file has no frame | `failed` |
| `finishing` | cancel | `cancelled` |

Any other pair is `E_INVALID_STATE`. A `ready` or `failed` recording whose review is not `added` can still be deleted by a person (`RecordingCancel`, section 9), which makes it `cancelled` and removes its file.

## 4. The review of a finished recording

A recording that is `ready` has a `review`: `pending` (a person has not decided), `added` (a person confirmed; the file is a library asset of the project) or `dismissed` ("Not now").

| Review | Who can read the recording | Where the file is |
| --- | --- | --- |
| `pending` | anyone with `read`: id, state, duration, size, never the bytes | `recordings/<id>/capture.mp4` |
| `dismissed` | a person only; every other caller gets `E_RECORDING_NOT_FOUND`, and it is absent from lists and jobs | the same |
| `added` | anyone with `read`, with the asset id | in the content store (the recording's own copy is removed once the import has copied it) |

`RecordingConfirm` (a person) imports the file with `MediaImport` and sets `added`. The dialog step says: "Agents with access to this project will be able to read this recording." `RecordingDismiss` (a person) sets `dismissed`; a person may confirm or delete it later.

## 5. Limits

| Limit | Default | Where it comes from |
| --- | --- | --- |
| Longest recording | 14,400 s | `STUARTY_MAX_RECORDING_SECONDS` (1 to 14,400, so a recording can always be imported) |
| Free disk | 2 GiB on the volume of the workspace | `STUARTY_MIN_FREE_BYTES` |
| File size | the import limit | `STUARTY_MAX_IMPORT_BYTES` |
| Frame rate | 24, 30 or 60 (default 30) | the request |
| Frame size | the source size cut to even numbers; no side over 8,192 and at most 33,177,600 pixels | the import limits |

`core/recording` has `Check(limits, elapsed, bytes, freeBytes)` that returns the reason to stop (`duration`, `disk`, `size`) or none. The start refuses (`E_LIMIT_EXCEEDED`) when the free space is already under the limit. During capture the host asks every second and stops cleanly; what was written is kept and the recording is `ready` with `stop_reason`.

## 6. The helper protocol

Version 1. One JSON object per line (UTF-8, at most 65,536 bytes) on standard input (commands) and standard output (events). Standard error is a free-text log kept as a tail by the host. Unknown fields are ignored. A line that is not valid JSON, or whose `type` is unknown, makes the helper answer `error` with `code` `bad_request` and keep running; the same from the helper makes the host discard it.

| Direction | Message | Fields |
| --- | --- | --- |
| helper to host | `ready` | `v` (1), `helper` (version), `os`, `display`, `window` (booleans) |
| host to helper | `sources` | none |
| helper to host | `sources` | `displays` and `windows`: `id`, `name` (a title for a window), `width`, `height` |
| host to helper | `start` | `recording`, `source` (`kind` `display` or `window`, `id`), `fps`, `encoder`, `ffmpeg` (absolute path), `output` (absolute path ending `.mp4`) |
| helper to host | `started` | `width`, `height`, `fps` |
| helper to host | `progress` | each second: `elapsed_ms`, `frames`, `dropped`, `bytes` |
| host to helper | `stop` | none |
| helper to host | `finished` | `reason` (`stopped`, `source_closed`), `frames`, `dropped`, `duration_ms`, `bytes` |
| helper to host | `error` | `code` (`bad_request`, `source_gone`, `unsupported`, `encoder_failed`, `io`, `internal`), `message` (printable, at most 200 characters) |

End of input is a stop. The helper serves one recording and exits when it ends (a process that only lists sources exits when its input closes); the host passes it no command-line argument (it answers `--version` for the discovery of the tool, and `--allow-test-source`, which only the tests of `capture/` pass, admits the fake source and the `raw` encoder). The JSON is read and written by hand over the standard library, because ADR-0049 approves no JSON crate. The `encoder` is one of a closed list (`h264_nvenc`, `h264_qsv`, `h264_amf`, `h264_mf`, `libopenh264`); the helper owns the FFmpeg argument template of each, so no user text reaches an FFmpeg argument. The `output` must be absolute, end in `.mp4` and lie in a directory that exists. The host chose the encoder with `engine/ffmpeg` (`EncoderForSize`, so the capability test of an export applies).

## 7. The host

`engine/capturehost` starts `stuarty-capture` as `Runner.RunHelper` starts `stuarty-render` (SDD-007 section 6.10): an absolute path from `STUARTY_CAPTURE_HELPER` (default: next to the server binary), no argument, a minimal environment, its own run directory `tmp/<recording id>/`, a deadline (the longest recording plus a grace), a kill of the process tree on cancel, and a refusal to run as root. It reads the `ready` line within 10 s and checks `v`. It never starts the helper for a caller that is not a person: the check is in the service, before the host is touched.

Failure paths: a malformed line, a line over the limit, a missing `ready`, no `started` within 15 s, an exit with no `finished`, or silence on `progress` for 10 s each end the child, never retry, and leave the file for recovery.

## 8. The file

FFmpeg (started by the helper with a minimal environment) reads raw BGRA from a pipe and writes `-f mp4 -movflags +frag_keyframe+empty_moov+default_base_moof -flush_packets 1`, a keyframe every second, with the low-latency options of the encoder (`-delay 0 -rc-lookahead 0 -bf 0` on NVENC), as measured in `spikes/capture-windows/README.md`. The helper warms the encoder before the frame grid starts, asks Windows for frames at an 8 ms minimum update interval, and writes one frame per slot of an absolute grid of `fps`: a slot with no new frame repeats the last one and counts as dropped, so the file keeps its real length. The capture border of Windows is left on and the cursor is not drawn (cursor events are epic 62). A frame of another size than the first (a resized window) is copied into a canvas of the first size, cut or padded with black.

## 9. Service operations and rules

### 9.1 Operations

| Operation | Scope | Caller | Rule |
| --- | --- | --- | --- |
| `RecordingSources` | `read` | any | lists displays and windows through the helper; `E_CAPTURE_UNAVAILABLE` where there is no capture; window titles are untrusted text |
| `RecordingStart` | `media` | **a person** (`Caller.Browser`) | `E_PRIVILEGED_SESSION` otherwise, first, before the project, the helper or the disk is looked at; one recording at a time per workspace (`E_QUEUE_FULL`, limit `recordings`); needs `project_id`, a source, `fps` |
| `RecordingStop` | `media` | any | `recording` to `finishing`; idempotent on `finishing` |
| `RecordingCancel` | `media` | any for a recording in progress (it is discarded); **a person** to delete a finished one | removes the file |
| `RecordingGet`, `RecordingList` | `read` | any | the visibility of section 4; the list is by project |
| `RecordingConfirm`, `RecordingDismiss` | `media` | **a person** | `ready` and `pending` (or `dismissed`) only |
| `RecordingPlace` | `edit` | any | `added` only; one batch that puts the asset in the document if needed and adds one clip at the end of a video track (created if none), so one undo removes it |

### 9.2 What a recording row holds

`id`, `project_id`, `state`, `review`, `source_kind`, `source_label` (sealed, section 9.4), `width`, `height`, `fps`, `encoder`, `started_at`, `ended_at`, `duration_ms`, `frames`, `dropped`, `bytes`, `stop_reason`, `error_code`, `error_message`, `asset_hash`, `asset_id`, `actor`, `created_at`.

### 9.3 Person rule

The service takes the privileges from `Caller.Browser` as `ApprovalDecide` does (`approval.CheckPrivileged`). With an account (ADR-0052) only a password session is a person. The local token, a key, `stuarty open`, a hosted agent and the MCP server are not.

### 9.4 Persistence

The table `recordings` is in SDD-000 section 7. Its column `source_label` is a sealed column (SDD-005 section 5.17.2, row key `id`) because a window title is the user's own text. Time and size columns are clear.

## 10. Recovery

At start-up, before the server accepts requests, every row in `prepared`, `recording` or `finishing` is settled: if its file has at least one decodable frame (`ffprobe` reads a duration above zero) it becomes `ready`, `review` `pending`, with the real duration and `stop_reason` `recovered`; otherwise `failed` with `E_CAPTURE_FAILED` and a message that names the reason: `no_frames` (no file, or no decodable frame in it) or `helper_lost` (this process has no capture to read the file with). A recording never stays `recording`. The recovery deletes no file: a failed recording keeps what it has until a person deletes it (`RecordingCancel`). It reads the rows without the sealed `source_label`, so it settles them while the studio is locked, before anyone signs in (ADR-0052); the label is untouched.

## 11. Surfaces

HTTP and CLI have every operation. MCP has no new tool (the tool list is at its byte budget, NFR-020): `job_get` with a `job_id` that starts `rec_` answers the recording as a job of kind `capture` (state `queued`, `running`, `succeeded`, `failed`, `cancelled` for `prepared`, `recording` and `finishing`, `ready`, `failed`, `cancelled`), and `job_cancel` with such an id is `RecordingStop`. The text of an added recording names its asset id, which is what an agent places with `edit`. No MCP route starts, confirms or dismisses a recording.

## 12. The dialog

The record dialog (`web/resources/js/`) draws what the service returns: a project, a source picker (displays and windows from `RecordingSources`), a frame rate, Start, then a visible recording state with the elapsed time and Stop, then the confirmation step (Add to the project, Not now, Delete) with the sentence of section 4. It computes no limit and no state. Where capture is unavailable it says why, in the words of the reason the service gave (section 1).

From epic 62 the picker also has a microphone list (none, the default one, or each device), a switch for the sound the computer plays (only where `system_audio` is true), a camera list (or "No camera was found."), and a switch for the pointer and clicks, on by default, with the sentence that the positions stay on the machine. While a recording runs, the dialog shows a level meter for each sound track from `levels` of the answer, which it asks for about three times a second, a Pause button that becomes Resume while `paused`, and Stop. A silent computer-sound track can be protected content, and the dialog says so beside its meter. The ids it sends are the ones `RecordingSources` listed; names are text, never markup.

The entry is a "Record screen" button in the library panel of the editor (`editor/library/Library.tsx`), the natural place today: the media panel is where files come in. The redesign of epic 75 moves it with the panel. The dialog reads `RecordingSources` and `RecordingList` when it opens, so a recording still running, or one waiting for a person, is picked up again after the dialog was closed, and the recordings set aside with "Not now" are listed above the picker with Add to the project and Delete; while a recording runs it calls `RecordingGet` once a second, and the clock it draws is `duration_ms` of the answer, which the service takes from the last line of the helper.

**Packaging.** `scripts/build-capture.sh` builds `stuarty-capture.exe` into `capture/dist/` with its SHA-256 (`gate` runs the Rust gate). The Windows installer (`desktop/packaging/windows/stuarty-desktop.iss`) installs it next to `stuarty.exe` when it is given `/DCaptureBinary=`, and the server finds it there, or in `libexec/stuarty/`, or at `STUARTY_CAPTURE_HELPER`. The release workflow passes the file once it builds it (a change of `.github/workflows/`, which the author reviews).

## 13. Threats

The threats of this domain, with their tests, are rows T-118 to T-122 of SDD-007 section 2:

| ID | Threat |
| --- | --- |
| T-118 | An agent or an injected prompt starts a capture |
| T-119 | An agent reads a recording a person dismissed, or one not yet confirmed |
| T-120 | The helper or its protocol runs more than it should |
| T-121 | A capture that nobody sees or stops, or a disk filled by it |
| T-122 | Window titles in clear text in `db.sqlite` |

## 14. Measures

NFR-067 is measured by `scripts/measure-capture.ps1`, one command: it opens a 1920 by 1080 test window, records it for 10 minutes at 60 fps through the real helper and host, and prints the dropped frames, the CPU of the helper and of FFmpeg, and the system load. The test window paints at every refresh of the display, so the source has frames for every slot; a display that shows a still desktop sends no new frame, and its repeated slots count as dropped too (the dialog says "repeated" for that reason). The script runs `TestCaptureNFR067` (`web/app/servicetest`), which uses the same service, host and helper as the dialog, and prints the average load of the machine next to the result. The number is valid only on an idle machine. NFR-068 is measured by killing the helper and FFmpeg together and reading the file with `ffprobe` (`spikes/capture-windows/README.md` has the first numbers).

## 15. Tests

Unit tests of every transition and limit (`core/recording`); a fake helper for the host and the service (timeout, malformed line, kill); a real capture test on Windows; the security tests of T-118 to T-122; a crash test that kills the helper and checks recovery.

## 16. Audio, webcam, pause and pointer events (epic 62)

A recording can hold up to four tracks, can be paused, and can save the pointer. The rules are in `core/recording` (`tracks.go`, `events.go`); the helper captures and writes, and decides nothing.

### 16.1 Tracks and files

| Track `kind` | Source | File in `recordings/<id>/` | Format |
| --- | --- | --- | --- |
| `screen` | the display or window of section 8 | `capture.mp4` | fragmented MP4, as section 8 |
| `camera` | one webcam | `camera.mp4` | fragmented MP4 of the same kind, no audio, at the frame rate of the recording |
| `microphone` | one capture endpoint | `microphone.wav` | PCM 16-bit little-endian, 48,000 Hz, one channel (two when the device refuses one) |
| `system` | the mix the computer plays (loopback of the default render endpoint) | `system.wav` | PCM 16-bit little-endian, 48,000 Hz, two channels |

Every track is optional but `screen`. One file per source: nothing is mixed, so the person can balance the sound later. A WAV file is written with its header up to date every second, so an interrupted recording keeps what was written (NFR-068); the byte limit of section 5 counts every file. Protected content (DRM) can make the system mix silent: the helper cannot tell it from a silent computer, so the dialog says that a silent system track may be protected content. Per-application audio capture is out of scope.

### 16.2 The shared clock

The helper holds one `Clock`. Its origin is the instant the screen grid starts (the `started` message), after the encoder warm-up of section 8. Every source is opened before the origin and writes only from it, so every track starts at `start_ms` 0 of the recording; `start_ms` stays in the data for a source that cannot (a later device). The clock is the performance counter of Windows (QPC): `std::time::Instant` is built on it, and WASAPI stamps every packet with a QPC position, which the helper converts to clock time. The clock counts **active** time: the time of a pause is removed (16.4), and every track and every pointer event is in active time, so one position means one moment in every file.

Drift correction happens while recording, not at placement, because only the helper sees the device clocks. The audio writer compares the stamp of every packet with the number of frames it has written: a packet that starts more than 10 ms after the written length gets silence inserted (this is also how a loopback endpoint that delivers nothing while the computer is silent stays aligned); one that starts more than 10 ms before it loses its overlapping frames. The screen and camera files are written on an absolute grid of the clock (section 8), so a file is as long as the clock says. A stop reaches the thread of each track at a different moment, so the clock fixes the end once, when the stop is asked for: active time stops there, sound stamped after it is cut, and every track pads or catches up to that one instant. Placement therefore needs only the `start_ms` of each track. The limit is 40 ms between any two tracks (NFR-068 fixture, 16.10). What the helper does not control is the latency of the device itself (a webcam delivers a frame 30 to 100 ms after it was exposed, a Bluetooth microphone more): documented, not corrected.

### 16.3 Devices

`RecordingSources` also lists `microphones` and `cameras` (`id`, `name`, and `default` for a microphone) and says whether `system_audio` exists. The `id` is made by the helper from the device: the letter `m` or `c` and 16 hexadecimal digits of the FNV-1a hash of the endpoint id of the system, so it is opaque, stable and a valid source id; a device name is untrusted text and is never stored. The microphone is opened with WASAPI in shared mode (the `wasapi` crate, approved by ADR-0049) with automatic format conversion; the system mix is the same API on the default render endpoint with the loopback flag. A device that cannot be opened ends the start with `E_CAPTURE_FAILED` and `details.reason` `device_unavailable`; nothing is recorded. A device that disappears during a recording (a USB microphone unplugged) ends only its own track: the recording goes on and the track reports `lost` with the length it has.

### 16.4 Pause and resume

`RecordingPause` stops every source from writing and freezes the clock; `RecordingResume` restarts them and the clock. The paused interval is kept as a pause (`from_ms`, `to_ms`, on the helper wall clock since the origin) and is removed from every file at once, so the tracks stay aligned across any number of pauses and a recording has no dead time. The screen and camera grids produce no slot while paused, audio packets that fall in a pause are dropped (a packet that spans the boundary is cut at it), and the pointer events carry active time, so a click sits at the same position as the frame it is on. A paused recording cannot run into the duration limit: `elapsed_ms` is active time. `Resume` restarts capture of the screen, so it needs a person (`E_PRIVILEGED_SESSION` to everyone else, checked before anything else, as in `RecordingStart`); `Pause` only reduces exposure and anyone with `media` may do it, as a stop. The WGC border of the capture stays on during a pause.

### 16.5 The webcam

A webcam is a source like the screen: frames go to a mailbox, an absolute grid of the clock writes one frame per slot (the last one repeated when the camera is slow) into its own FFmpeg child, which writes `camera.mp4` with the encoder of the screen. The helper opens the camera before the origin and refuses a start whose camera gives no frame in 5 s. The Windows device is Media Foundation, through the `windows` crate that D-142 approves for `capture/` (`src/camera_mf.rs`, a reviewed boundary module). A source reader with video processing on turns whatever the camera sends into RGB32; the picture asked for is the largest native one that fits in 1280 by 720; rows are turned top-down by a function that is tested alone. The camera id is `c` and the hash of the symbolic link of the device. Listing the cameras only enumerates them: no camera is opened, so no camera light comes on, until a person starts a recording with it. With no camera, `cameras` is empty and the dialog says no camera was found. The test camera of the helper (`--allow-test-source`) makes the same track without a device and tests the file and the clock. A test that opens a real camera runs only with `STUARTY_TEST_REAL_CAMERA=1`. How a given camera lays its rows out (bottom-up is assumed when the stride attribute is absent) is [to validate on a real camera].

### 16.6 Pointer events

When the person asks for them (`events` true in the start request; the dialog has a switch, on by default), the helper writes `events.jsonl`: one JSON object per line, UTF-8, each at most 200 bytes, at most 2,000,000 lines.

| Line | Fields |
| --- | --- |
| first line, `k` `header` | `v` 1, `width` and `height` of the captured source in pixels, `clock` `active_ms` |
| `k` `move` | `t` (active ms since the origin), `x`, `y` |
| `k` `down`, `k` `up` | `t`, `b` (`left`, `right`, `middle`), `x`, `y` |

Coordinates are pixels of the captured source, `x` from 0 to `width` minus one and `y` from 0 to `height` minus one: for a display, relative to its top-left corner; for a window, to the top-left corner of its frame, read again at every sample because a window moves. A position outside the source is not written (neither is a press that starts there), and a release is written for every press that was. A move is written when the position changed and at least 16 ms passed since the last one. The helper polls the position and the state of the three mouse buttons every 4 ms and writes the edges, so a click is stamped within 20 ms of the press (acceptance criterion 3). Polling replaces the raw-input window of the epic text: it needs no window and no message loop, reads no device other than the mouse, and gives the same data.

**Privacy rule: no key is ever read.** The helper installs no hook of any kind (no `SetWindowsHookEx`, low-level or not), registers no raw-input device for the keyboard, and reads the state of no key but the three mouse buttons (virtual keys 1, 2 and 4). What a person types therefore never reaches the helper, the file or any message. A pointer position can reveal what is on the screen, so the file stays on the machine in `recordings/<id>/`, is read only through `RecordingEvents`, and is deleted with the recording. `core/recording` validates the file when it is read: a line with a `k` other than the four above, a field it does not know or a value out of range is discarded and counted, so a file that holds anything else cannot reach a caller. Documented limits [to validate on the author machine]: a pointer over a window of a higher integrity level than the helper may read as still; a game that hides and recentres the pointer shows it fixed.

### 16.7 The protocol, version 2

Version 2 adds fields and messages; the server refuses a helper that answers another version (`E_CAPTURE_UNAVAILABLE`, `details.reason` `protocol_version`). Everything else of section 6 stands.

| Direction | Message | New fields |
| --- | --- | --- |
| helper to host | `ready` | `v` 2, and `audio`, `camera`, `events` (booleans) |
| helper to host | `sources` | `microphones` (`id`, `name`, `default`), `cameras` (`id`, `name`, `default`), `system_audio` (boolean) |
| host to helper | `start` | `microphone` (a device id or `default`), `system_audio` (boolean), `camera` (a device id), `events` (boolean), and `outputs`: absolute paths in the folder of `output`, ending `.wav` for `microphone` and `system`, `.mp4` for `camera`, `.jsonl` for `events`; a path is required for exactly each track that was asked for |
| helper to host | `started` | `tracks`: the list of `{kind, start_ms}` that is recording |
| host to helper | `pause`, `resume` | none |
| helper to host | `paused`, `resumed` | `elapsed_ms` (active) |
| helper to host | `level` | about four a second: `microphone` and `system`, each 0 to 1000 (the peak of the last interval, in permille of full scale), only for the tracks that exist |
| helper to host | `progress` | `bytes` now counts every file |
| helper to host | `finished` | `tracks`: `{kind, start_ms, duration_ms, bytes, lost}` for each; `pauses`: `{from_ms, to_ms}`; `pointer_events` (count) |

`pause` when the recording cannot pause and `resume` when it is not paused are answered with `error` `bad_request` and change nothing. A `level`, `paused` or `resumed` line counts as a sign of life for the silence limit of section 7. A test build of the helper (`--allow-test-source`) admits `fake` sources for the new kinds (a tone, a flash, a camera pattern and a scripted pointer), which is how the clock, the pause and the events are tested without a device.

### 16.8 Operations and rules

| Operation | Scope | Caller | Rule |
| --- | --- | --- | --- |
| `RecordingSources` | `read` | any | now also the microphones, the cameras and `system_audio` |
| `RecordingStart` | `media` | **a person** | new members `microphone`, `system_audio`, `camera`, `events`; each is checked in `core/recording` before the helper starts |
| `RecordingPause` | `media` | any | `recording` to `paused`; idempotent on `paused`; anything else `E_INVALID_STATE` |
| `RecordingResume` | `media` | **a person** | `E_PRIVILEGED_SESSION` first, then `paused` to `recording`; idempotent on `recording` |
| `RecordingEvents` | `read` | any | the pointer events of a `ready` recording, by the visibility of section 4: `from_ms`, `kind` (`clicks`, the default, or `all`), `limit` (1 to 5,000, default 1,000); the answer holds the size of the source, the total, the events and `truncated`. `E_INVALID_STATE` for a recording that is not `ready`; no events when the person did not ask for them |
| `RecordingPlace` | `edit` | any | one batch: the screen on a visual track as before, the camera on its own visual track, each sound track on its own audio track, every clip at the same start so the tracks stay aligned |

`RecordingConfirm` imports every file of the recording as an asset (tagged `recording`) and removes the media files, **keeping `events.jsonl`** because it is read after the recording was added; that file is deleted when the recording is deleted. Limit [open, for the author]: an added recording cannot be deleted (section 3, it is removed as an asset), so the pointer file of an added recording stays in `recordings/<id>/` after its assets are removed; no operation deletes it yet. The row adds `tracks` (a JSON list of `{kind, file, start_ms, duration_ms, bytes, lost, asset_hash, asset_id}`), `pauses` (a JSON list of `{from_ms, to_ms}`) and `events_count`; `levels` is not stored (the last `level` of the helper while it runs). The columns are clear: they hold no device name and no pointer position. MCP gets no new tool: `job_get` of a `rec_` id names the tracks and the click count, and `job_cancel` stays `RecordingStop`.

### 16.9 Placement

`RecordingPlace` puts every clip at the start the screen clip gets (the end of its track, as in epic 61), because the tracks share the origin of the recording. The camera goes to a visual track named `Camera`, the microphone to an audio track named `Microphone` and the system mix to one named `System audio`; an unlocked track of that name is reused, otherwise it is made in the same batch. A recording with only a screen places as before.

### 16.10 Measures and tests

NFR-068 fixture: the test sources of the helper produce a flash and a tone burst at the same instant of the clock, twice a second, and the test measures the position of each in the video file and in the WAV file after a recording with three pauses. Drift is how far apart the tracks are and whether that grows, so the test holds two numbers to 40 ms: the 95th percentile of the distance between each flash and the nearest flash or burst of the other tracks, and the change of that distance between the first and the last fifth of the recording. One frame that is written late (a disk that stalls for a moment, a busy machine) is jitter: the grid fills the slot with the last frame, the file keeps its length and the tracks stay aligned, so the test reports the largest distance but does not fail on it. A flash with no counterpart within 125 ms (a source that gave no new frame for that pulse, a quarter of its period) is not a distance: it is counted and reported as missed. The 40 ms of the epic is held as this drift, not as the largest single distance, which a loaded machine can push past 100 ms. The helper raises the priority of the threads that stamp data and asks Windows for a 1 ms timer. The default test runs it for 12 seconds; the full one (10 minutes, three pauses) is `cargo test --release -- --ignored full_ten_minute_drift` in `capture/`. Real devices (a microphone, the loopback of a real sound, a webcam, the real pointer) need the author hardware and are listed as not run in the hand-off of the epic. The click test feeds a scripted pointer through the same code as the real one and checks the time (within 20 ms) and the coordinates. The privacy tests are the validator of `core/recording`, a scan of the helper source for hooks and key reads, and a security test that no operation returns key data.

## 17. Finishing a take: automatic zoom, smooth cursor and click highlight (epic 65)

A screen take that holds pointer events (section 16.6) can be finished with three commands of the core: `recording.auto_zoom`, `recording.set_cursor` and `recording.set_click_highlight` (SDD-002 C.93 to C.95, names in SDD-000 section 18). Each is an ordinary undoable edit with scope `edit`, so a person, an agent, the CLI and the HTTP API reach it the same way (`POST /projects/{id}/edit`, the `edit` tool of MCP). No operation, tool or route is added.

### 17.1 What the capture gives

Confirmed against epic 62: the helper never draws the pointer into the video (Windows Graphics Capture with `WithoutCursor`, section 8), and it writes the pointer to `events.jsonl` only when the person asked for it. The cursor of a finished take is therefore drawn by the cursor overlay of this section, never by the video; a take with no events has no cursor and no click to follow. A web recording (epic 63) has no global pointer data: it can be zoomed by hand with `keyframe.set` and the commands of this section answer `E_INVALID_STATE` for it.

### 17.2 Where the data comes from

The three commands read the pointer through `resolved`, as `clip.remove_silences` and `clip.face_frame` do (SDD-002 A.9): the service finds the recording whose screen track became the asset of the clip, reads `events.jsonl` with the validator of `core/recording`, keeps what falls inside the source range the clip plays and writes it into `args.resolved`. The stored operation carries it, so replaying the log gives the same document. For the zoom and the highlight it holds the button edges (`down`); for the cursor it holds the positions, thinned by `core/recording` to at most one per 50 ms and at most 30,000 in all (a longer range raises the spacing). A caller cannot send `resolved`. The service answers `E_INVALID_STATE` with a hint, before anything is applied, when the asset is not the screen track of a recording of the project, when the recording is not in `ready` review, or when the person did not ask for pointer events.

### 17.3 No new item, no renderer change

The three effects are made of what the timeline already has, so the frame plan, the preview and the export draw them with the layer kinds that exist and SDD-013 is unchanged (a shape is a `bitmap` layer of the plan):

- **Zoom**: keyframes of `x`, `y` and `scale` of the clip, written by the command with `ease_in_out`; the editor adjusts or removes each keyframe. The static `x`, `y` and `scale` are the rest state and are never changed.
- **Cursor**: shape items on a visual track named `Cursor`, directly above the track of the clip: an ellipse whose `x` and `y` keyframes are the smoothed pointer path, mapped through the transform the clip has at that frame. A property holds at most 100 keyframes (SDD-001 section 3.8), so the path is cut into items of at most 90 keyframes each, one after the other.
- **Click highlight**: one shape item per click on a visual track named `Click highlight`, a ring or a disc whose `scale` and `opacity` keyframes play the effect; it never outlasts the next click, so the items of one clip never overlap.

The overlays are drawn from the transform of the clip **at the time the command runs**: they are not linked to it. After changing the zoom by hand or moving the clip, run `recording.set_cursor` and `recording.set_click_highlight` again; each replaces the items it made before (they are named `Cursor <item id>` and `Click <item id> <n>`, SDD-000 section 18) in one undoable step. Moving the clip alone leaves the overlays where they were; this is the limit of the overlay design and is the price of needing no new item type.

### 17.4 The zoom planner

`core/recording` holds the planner, with integers only: the same clicks and options always give the same plan. The positions are ten-thousandths of the source (`recording.Unit`) and the times are milliseconds on the axis of the asset.

1. Keep the button edges inside the source range the clip plays, in time order.
2. **Group**: a click joins the group when it is at most `merge_ms` after the previous one and the box that holds the group stays within 60 % of the visible window at the zoom level (so every click of a zoom is on screen). Otherwise it starts a group.
3. Each group becomes one zoom: its focus is the centre of the box of its clicks, the full level is reached `lead_ms` before its first click, after an ease in of `ramp_ms`; it is held until `hold_ms` after its last click and eases out in `ramp_ms`. The ease in cannot start before the clip does and the ease out ends at the end of the clip at the latest.
4. **Chain**: when the ease in of a zoom would start less than one second after the previous one has finished easing out, the view does not return to rest: it moves from one focus to the other in at least 300 ms. The move ends at the full zoom of the next, which is before its first click when the time between the two allows it, and starts when the first has been held 0.5 s after its last click at the earliest (the first is let go at the end of its own hold when that comes sooner). A chained pair counts as two zooms.
5. At most `max_zooms` zooms are made, the first in time; `effective.dropped` counts the rest.

Limits, so that no default can make a viewer ill: the level is 1.1 to 3.0 (default 1.6); `lead_ms` 0 to 3000 (400); `ramp_ms` 200 to 3000 (700); `hold_ms` 500 to 10000 (1200); `merge_ms` 0 to 10000 (2500); `max_zooms` 1 to 24 (24, which keeps `x`, `y` and `scale` under the 100 keyframes of a property, four per zoom). The view never leaves the picture: the offset is clamped so the scaled image always covers the frame.

### 17.5 The cursor track

The smoothed position is an exponential filter of time constant `smoothing_ms` (0 to 2000, default 150; 0 follows the raw path), run in integer milli-pixels on a 10 ms step and sampled every `keyframe_ms` (50 to 1000, default 100). It is drawn as a `dot` (a filled ellipse with a dark outline) or a `ring` of `size` pixels (8 to 128, default one fortieth of the frame height, at least 16) and keeps that size when the clip is zoomed. `visible` false removes the cursor items. The pointer is a point of the source: a position outside the part of the source the frame shows is drawn outside the frame and is not seen.

### 17.6 The click highlight

`style` is `ring` (a hollow ring that fades out), `wave` (a ring that grows while it fades) or `pulse` (a disc that shrinks to nothing as it fades); `none` removes the highlights. `color` (default `#FFC400`), `size` in pixels (16 to 400, default one twelfth of the frame height) and `duration_ms` (200 to 2000, default 600) are the options. A click in the same frame as the one before it adds no item.

### 17.7 Tests

Golden cases of the three commands (SDD-002 C.93 to C.95) on a fixture with five clicks ten seconds apart, the planner tests of `core/recording`, a parity test of the service (edit by the Service, CLI, HTTP and MCP gives the same document), and an export test of a recorded fixture that compares frames with golden images. Not run: a real recording with the real pointer on the author machine.
