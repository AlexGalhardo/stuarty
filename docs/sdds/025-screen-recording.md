# SDD-025: Screen recording

- Status: Draft (2026-11-24). Written by epic 61 before its code; extended by epic 62 (section 16: audio, webcam, pause, pointer events), epic 65 (section 17: automatic zoom, smooth cursor, click highlight) epic 63 (section 18: basic recording in the browser) and epic 64 (section 19: agents in the recording flow).
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
18. Basic recording in the browser (epic 63)
19. Agents in the recording flow (epic 64)

## 1. Overview

A person records one display or one window of Windows into a project. A separate executable, `stuarty-capture`, captures frames with Windows Graphics Capture and pipes them to an LGPL FFmpeg child that writes a fragmented MP4. The server starts and supervises the helper through `engine/capturehost`; the rules live in `core/recording`.

Rules that carry the design:

1. **The helper captures and writes; it decides nothing about a project.** Every rule (states, limits, who may start) is in `core/recording` and the service (ADR-0049 item 3).
2. **Only a person starts a capture.** `RecordingStart` needs a privileged browser session and answers `E_PRIVILEGED_SESSION` to everyone else before any process starts (ADR-0051). No approval rule can lift it.
3. **Stopping is always allowed** to anyone with the `media` scope.
4. **A recording enters the project only when a person confirms it** (ADR-0051 item 7). "Not now" keeps it out of the project and invisible to agents.
5. **A recording survives a crash.** Fragmented MP4, and a recovery at the next start.
6. **Nothing leaves the machine.** The helper opens no connection and records no keystroke.

Where native capture does not exist (web, Docker, network mode, macOS, Linux until epics 67 and 68) every operation of the helper but the reads answers `E_CAPTURE_UNAVAILABLE` with `details.reason` (`platform`, `web`, `network_mode`, `ffmpeg_missing`, `helper_missing`, `protocol_version`). From epic 63 a person in a browser can still record there: the browser captures and uploads, and the server only stores and assembles the file (section 18).

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

HTTP and CLI have every operation. MCP has no new tool (the tool list is at its byte budget, NFR-020): `job_get` with a `job_id` that starts `rec_` answers the recording as a job of kind `capture` (state `queued`, `running`, `succeeded`, `failed`, `cancelled` for `prepared`, `recording` and `finishing`, `ready`, `failed`, `cancelled`), and `job_cancel` with such an id is `RecordingStop`. The text of an added recording names its asset id, which is what an agent places with `edit`. No MCP route starts, confirms or dismisses a recording. Epic 64 (section 19) adds no tool either: an agent over MCP reads the plan, the marks and the proposals of a recording with `job_get`, and prepares, marks and proposes through HTTP and the CLI. The three upload operations of section 18 are HTTP only (no CLI command, no MCP tool): a browser capture cannot be driven from outside.

## 12. The dialog

The record dialog (`web/resources/js/`) draws what the service returns: a project, a source picker (displays and windows from `RecordingSources`), a frame rate, Start, then a visible recording state with the elapsed time and Stop, then the confirmation step (Add to the project, Not now, Delete) with the sentence of section 4. It computes no limit and no state. Where capture is unavailable it says why, in the words of the reason the service gave (section 1).

From epic 62 the picker also has a microphone list (none, the default one, or each device), a switch for the sound the computer plays (only where `system_audio` is true), a camera list (or "No camera was found."), and a switch for the pointer and clicks, on by default, with the sentence that the positions stay on the machine. While a recording runs, the dialog shows a level meter for each sound track from `levels` of the answer, which it asks for about three times a second, a Pause button that becomes Resume while `paused`, and Stop. A silent computer-sound track can be protected content, and the dialog says so beside its meter. The ids it sends are the ones `RecordingSources` listed; names are text, never markup.

Where native capture is unavailable the same dialog records in the browser (section 18).

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
| T-137 | A recording uploaded from a browser: who sends it, how much, in what order, under what name, and what it becomes (section 18) |
| T-165 | An agent starts or resumes a capture by any path, through a prepared recording or through an approval rule (section 19.3) |
| T-166 | An agent applies a cut, or a proposal changes a timeline before a person decides (section 19.5) |
| T-167 | A capture that is on and cannot be seen (section 19.4) |

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

`RecordingConfirm` imports every file of the recording as an asset (tagged `recording`) and removes the media files, **keeping `events.jsonl`** because it is read after the recording was added; that file is deleted when the recording is deleted. An added recording cannot be deleted (section 3, it is removed as an asset), so its pointer file goes with the screen asset: when `TrashEmpty` deletes that asset for good, it deletes `recordings/<id>/events.jsonl` (and the folder, once empty) and sets `events_count` to 0. Trashing the asset deletes nothing, because it can be restored; the other tracks and the row stay (D-144, VV-416). The row adds `tracks` (a JSON list of `{kind, file, start_ms, duration_ms, bytes, lost, asset_hash, asset_id}`), `pauses` (a JSON list of `{from_ms, to_ms}`) and `events_count`; `levels` is not stored (the last `level` of the helper while it runs). The columns are clear: they hold no device name and no pointer position. MCP gets no new tool: `job_get` of a `rec_` id names the tracks and the click count, and `job_cancel` stays `RecordingStop`.

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

## 18. Basic recording in the browser (epic 63)

A person who uses Stuarty in a browser (on Docker, on a server, or on a machine with no native helper) records the screen, the microphone and, if the browser offers it, the sound the page shares, and the take lands in the same project through the same recording row, states, review and placement as a native one. The browser captures and uploads; **the server decides**: the order and integrity of what arrives, the limits, who may upload, and the file that results. The browser computes no edit and holds no rule (ADR-0007).

### 18.1 Where it works

The browser APIs (`navigator.mediaDevices.getDisplayMedia`, `getUserMedia`, `MediaRecorder`) exist only in a secure context: a page on HTTPS or on `localhost`, `127.0.0.1` or `[::1]`. On plain HTTP anywhere else `navigator.mediaDevices` is absent and the dialog **disables** recording and says why, in words that name the two ways out (HTTPS, or open the studio on `localhost`). The browser asks the person for each permission itself; the product never tries to pre-answer or hide a prompt, and a refusal is shown as a refusal. For the page to be allowed to ask at all, every response carries `Permissions-Policy: camera=(self), microphone=(self), display-capture=(self), geolocation=()` (SDD-007 section 3.6): the framework's default turns the camera and the microphone off for every page, and the screen capture of a browser has no other user than the page of Stuarty. A capture that the browser stops (the person pressed its own "Stop sharing") ends the recording the same way Stop does.

Native capture stays where it exists: when `RecordingSources` answers, the dialog is the native one of sections 12 and 16 and offers no browser recorder; when it answers `E_CAPTURE_UNAVAILABLE` with any reason but `ffmpeg_missing` (the server needs an FFmpeg to assemble the file as well), the dialog offers the browser recorder instead of the sentence that capture is unavailable. Chrome on Android, global pointer and click data, a guarantee of computer sound and finishing effects are out of scope.

Before the person starts, the dialog states what this browser can and cannot do, from `MediaRecorder.isTypeSupported` and the tracks the browser returns: which container it records (WebM with VP9, VP8 or AV1 video and Opus audio, or MP4 with H.264 and AAC), whether the sound of the computer can be shared (the picker of the browser decides, and only for a tab or, on some systems, the whole screen), and that Firefox and Safari differ. The list of candidate types is data of the recorder module; the first one the browser supports is the one used.

### 18.2 Names

| Kind | Name |
| --- | --- |
| Operations | `RecordingUploadBegin`, `RecordingUploadChunk`, `RecordingUploadFinish` (SDD-000 section 1) |
| Source kind | `browser` (the others are `display` and `window`); the row has no `encoder` helper name, and `encoder` holds the container (`webm` or `mp4`) |
| Routes | `POST /recordings/upload`, `PUT /recordings/{id}/upload/{track}/{index}`, `POST /recordings/{id}/finish` |
| Error codes | none new: `E_PRIVILEGED_SESSION`, `E_VALIDATION`, `E_INVALID_STATE`, `E_LIMIT_EXCEEDED`, `E_PAYLOAD_TOO_LARGE`, `E_QUEUE_FULL`, `E_RECORDING_NOT_FOUND`, `E_CAPTURE_FAILED` with the `details.reason` of 17.4 |
| Files | `recordings/<id>/parts/<track>/<index>.part`, then `capture.webm` or `capture.mp4` (and `camera.webm` or `camera.mp4`); the name of a part is the track and the number, never text of the caller |
| Variables | none new: `STUARTY_MAX_IMPORT_BYTES`, `STUARTY_MAX_RECORDING_SECONDS`, `STUARTY_MIN_FREE_BYTES` |

### 18.3 The contract

The product had one upload route before this, `POST /images/import/file` (SDD-021 section 10.2), which takes a whole file in one request. A recording cannot use it: it has no known end while it runs, may be many gigabytes, and has to survive a closed tab, so it goes up in ordered chunks, each checked, and what arrived before a failure is kept.

The three operations are for **a person in a privileged browser session**: every other caller (the local token, a key, an agent, the MCP server) gets `E_PRIVILEGED_SESSION` as the first answer, before the project, the row or the disk is looked at. The CLI and MCP have no command and no tool: a capture of a browser cannot be driven from outside, and `RecordingGet`, `RecordingList` and `job_get` read the state as they do for a native recording. Stopping stays open to every caller with `media` (section 18.6).

| Operation | Route | Input | Answer |
| --- | --- | --- | --- |
| `RecordingUploadBegin` | `POST /recordings/upload` | JSON: `project_id`, `container` (`webm` or `mp4`), `camera` (boolean: a camera track will follow), `label` (optional, plain text, at most 120 characters, sealed as a window title is) | the `Recording` in state `recording`, `source.kind` `browser`, with `upload` (below) |
| `RecordingUploadChunk` | `PUT /recordings/{id}/upload/{track}/{index}?sha256=<hex>&elapsed_ms=<n>` | the bytes of the chunk are the whole body (`application/octet-stream`, a content type of `application/json` is accepted and ignored, as in SDD-007 section 3.6 step 6); `track` is `screen` or `camera`; `index` counts from 0 on each track; `sha256` is the lowercase hexadecimal digest of the body; `elapsed_ms` is the recorder's clock, informational | `recording_id`, `track`, `index`, `next_index`, `duplicate`, `bytes` (all tracks), `text` |
| `RecordingUploadFinish` | `POST /recordings/{id}/finish` | JSON: `tracks`, a list of `{kind, chunks}` (the number of chunks the browser sent on that track), `elapsed_ms` | the `Recording`, in state `finishing` |

`Recording` gains `upload`, present while a browser recording is `recording`: `container`, `max_chunk_bytes`, and for each track `kind`, `next_index` and `bytes`. It is what a client reads to resume after a dropped connection.

The body of a chunk is limited by the route (`Op.MaxBody` of the framework, 16 MiB): a declared or streamed body over it is `E_PAYLOAD_TOO_LARGE` (413) and nothing is stored. The browser slices the recording with `MediaRecorder.start(1000)`, so a chunk is about a second of media. The server reads at most the limit plus one byte, hashing as it writes into a temporary file, and keeps the chunk only if the digest equals `sha256`, so memory is never the size of the upload.

### 18.4 Order, integrity, limits and resume

The rules are in `core/recording` (`upload.go`); the service applies them in this order, and the first that fails answers.

1. A person (`E_PRIVILEGED_SESSION`), then the recording exists and the caller covers its project (`E_RECORDING_NOT_FOUND`).
2. The recording is `recording` and its source is `browser`, and `track` is one the row asked for (`E_INVALID_STATE`, `details.reason` `not_receiving`, or `E_VALIDATION` for a track the row did not ask for or a malformed `index` or `sha256`).
3. **Order.** A chunk is accepted only at `next_index`. A lower index that was already stored is a retry: if its digest equals the stored one the answer is the same success with `duplicate` true and nothing changes; if it differs, `E_INVALID_STATE` with `details.reason` `chunk_conflict`. A higher index is `E_INVALID_STATE` with `details.reason` `out_of_order` and `details.expected` (the `next_index`). An index of 100,000 or more is `E_LIMIT_EXCEEDED` (`details.reason` `chunks`).
4. **Integrity.** A body whose SHA-256 is not `sha256` is `E_VALIDATION` with `path` `sha256` and `details.reason` `checksum_mismatch`; an empty body is `E_VALIDATION` with `path` `body`. Nothing of a refused chunk stays on the disk.
5. **Limits**, checked with `core/recording` `Limits`: the bytes of all tracks may not pass `STUARTY_MAX_IMPORT_BYTES` (`E_LIMIT_EXCEEDED`, `details.reason` `size`); the free space may not fall under `STUARTY_MIN_FREE_BYTES` (`details.reason` `disk_low`); `elapsed_ms` may not pass `STUARTY_MAX_RECORDING_SECONDS` (`details.reason` `duration`). A refusal for a limit keeps everything received so far: the client finishes the recording and the take is what it has, with `stop_reason` of the limit.
6. **One at a time.** Begin refuses (`E_QUEUE_FULL`, `details.limit` `recordings`, with `value` and `ceiling`) while another browser upload of the same actor is still receiving chunks (state `recording`; one that is being assembled holds no tab), and while four are, after it has first settled the ones that were left with no chunk for five minutes (17.6). A browser upload does not count against the one native capture of section 9, and a native capture does not block a browser one: they share nothing but the disk.

**Resume after a dropped connection.** The browser keeps a chunk until the server says it has it. A request that fails is sent again with the same index and digest: if the server had stored it the answer is `duplicate` and nothing is doubled. If the browser lost the answers and does not know where it is, `RecordingGet` returns `upload.tracks[].next_index` and it continues from there. A MediaRecorder cannot be resumed after a reload of the page: a recording whose browser is gone is finished with what was received (17.6).

### 18.5 Assembly

`RecordingUploadFinish` (or a stop, 17.6) moves the row to `finishing` and returns; the work runs in the server and the dialog reads `RecordingGet` as it does for a native recording. For each track the server checks that the chunks `0` to `chunks - 1` are all there (a `chunks` that differs from what was received is `E_INVALID_STATE` with `details.reason` `chunks_missing`, `details.kind`, `details.expected` and `details.received`, and the row stays `recording`), joins them in order into one file in the folder of the recording, and remuxes it with the LGPL FFmpeg, as a copy of the streams and never a re-encode, into a file whose duration and index are written (`capture.webm` or `capture.mp4`). The file is then probed with the same ceilings and format check as any import (SDD-007 sections 6.2 and 9): a file with no decodable duration, an unknown format or a size over the limit makes the recording `failed` with `E_CAPTURE_FAILED` whose message starts with the reason, `assemble:` (or `no_frames:` for a take with no chunk of the screen, as in section 10), and the chunks are kept until a person deletes the recording, as section 10 keeps any file. On success the parts are removed, the row is `ready` with `review` `pending`, the duration, the size of the picture and the bytes are those of the probed file, and the review step, the asset created by `RecordingConfirm` and the placement are those of sections 4, 9 and 16.9. The track list of the row names the file of each track (`capture.webm`, `camera.webm`) and no `start_ms` offset beyond 0: the browser recorders are started together and the small offset between them is not corrected (documented, as the device latency of 16.2).

### 18.6 A closed tab, a stop and a restart

A browser recording whose page was closed stays `recording` with the chunks it received. It is finished, with exactly those chunks and no count check, by any of: `RecordingStop` (any caller with `media`, as always; the dialog offers it for an interrupted take), the settling of five idle minutes at the next `RecordingUploadBegin` of the workspace (`stop_reason` `abandoned`), or the recovery at start-up (`stop_reason` `recovered`). The recovery of section 10 handles a browser row before the helper checks: it assembles the parts that are on the disk when an FFmpeg is available, and marks the recording `failed` with a message that starts `ffmpeg_missing:` when it is not (the parts stay). Because the last chunk is at most a second of media, a closed tab loses at most that. `RecordingCancel` of a recording in progress discards it and its parts, for any caller with `media`, as for a native one. `RecordingPause` and `RecordingResume` do not exist for a browser recording (`E_INVALID_STATE`): the dialog has a Stop and nothing else in this version.

### 18.7 The dialog

The dialog is the one of section 12. Where native capture is unavailable it shows the capability sentences of 17.1, a microphone switch, a switch for the sound of the page, a camera switch and Start, then a visible state (the clock of the recorder, a red mark, the bytes the server confirmed and a line that says when an upload is retrying), Stop, the wait for `finishing`, and then the review step of section 4 as it is. A take that was interrupted is listed at the top with Finish it (a stop) and Delete. The recorder module in `web/resources/js/editor/record/` captures and uploads; it computes no limit, no state and no duration the server did not confirm: the numbers on the screen after a stop are those of `RecordingGet`.

### 18.8 Threats and tests

T-137 (SDD-007 section 2) covers the upload: who may send, how much, in what order, with what name, and what the bytes may become. The service tests cover the order, the checksum, the limits, the retry and the resume, the closed tab, the restart and the assembly with a real FFmpeg; the security tests of T-137 cover the privilege of each route, the refusals, the generated names and the body limit; the dialog is covered by the unit tests of the recorder module and by an end-to-end test in Chromium with fake media devices that records 30 seconds, uploads, finishes and places the clip. Firefox and WebKit are reported as run or not run by the hand-off of the epic.

## 19. Agents in the recording flow (epic 64)

ADR-0051 decides what an agent may do with capture; this section is how. An agent prepares a recording, reads it, marks takes and mistakes and proposes cuts. It never starts, resumes or confirms a capture, and a person decides every cut.

### 19.1 Names

| Name | Meaning |
| --- | --- |
| prepared recording | a `recordings` row in state `prepared` whose `plan` column is not empty: what a person will record, with the helper not started. It holds no file, opens no process and is not "in progress" (section 9: it does not count against the one recording at a time) |
| plan | the stored request: the source (`kind`, `id`, optional `name`), `fps`, `microphone`, `system_audio`, `camera`, `events`, the id of a written script (`script_id`, optional) and notes |
| mark | a point or a stretch of a take, in active milliseconds of the recording, tagged `good`, `bad` or `mistake`, with an optional note |
| proposal | a list of ranges to cut out of a take, each with a reason, made by any caller and decided by a person. Its id starts `prp_` |

### 19.2 Operations

| Operation | Scope | Caller | Rule |
| --- | --- | --- | --- |
| `RecordingPrepare` | `media` | any | validates the plan like `RecordingStart` does (`core/recording`), checks the project, and inserts a row in `prepared` with the plan. **It never reaches the helper**: the code path has no capturer. `E_VALIDATION`, `E_PROJECT_NOT_FOUND`, or `E_LIMIT_EXCEEDED` (`details.limit` `prepared_recordings`: 20 per project) |
| `RecordingStart` | `media` | **a person** | takes an optional `recording_id`. With it, the check of a person is still the first thing done, then the plan of that prepared recording is used (the other members of the request must be empty, else `E_VALIDATION`) and the same row goes `prepared` to `recording`; a recording that is not prepared with a plan is `E_INVALID_STATE`. Without it nothing changes |
| `RecordingCancel` | `media` | any | a prepared recording may be cancelled by any caller with `media`: it only removes a plan |
| `RecordingMark` | `edit` | any | adds a mark to a recording that is `recording`, `paused`, `finishing` or `ready` (`E_INVALID_STATE` otherwise); `at_ms` is 0 or more, `end_ms` is empty or after it, at most 500 marks (`E_LIMIT_EXCEEDED`, `details.limit` `recording_marks`), a note is cleaned text of at most 200 characters. A mark changes no file and no timeline |
| `RecordingProposeCuts` | `edit` | any | adds a proposal to a `ready` recording: 1 to 200 ranges in active ms, sorted, not overlapping, each at least 100 ms long and inside `duration_ms`, each with a reason of 1 to 200 characters; at most 5 pending proposals per recording (`E_LIMIT_EXCEEDED`, `details.limit` `recording_proposals`). **It changes no timeline**: no revision moves and nothing is written to a document |
| `RecordingProposalDecide` | `edit` | **a person** | `E_PRIVILEGED_SESSION` first, before anything is read. Takes `recording_id`, `proposal_id`, a decision for every range (`accept` true or false) and optionally `timeline_id`. A proposal that is not pending is `E_INVALID_STATE`. With no range accepted the proposal becomes `rejected` and no timeline is touched. Otherwise the accepted ranges become **one batch** of existing commands on the timeline where the recording was placed (19.5) and the proposal becomes `accepted` |
| `RecordingGet`, `RecordingList` | `read` | any | also `plan` (when prepared), `marks` and `proposals` (with the decision of each range). `RecordingGet` is what `job_get` shows for a `rec_` id, so an agent over MCP reads them with no new tool |

A dismissed recording stays invisible to everyone but a person (section 4), for every operation of this section.

### 19.3 Start and resume are for a person, on every path

The paths that start or resume a capture are `RecordingStart`, `RecordingResume` and the three upload operations of section 18 (`RecordingUploadBegin`, `RecordingUploadChunk`, `RecordingUploadFinish`), which start or continue a capture of a person's browser. Each answers `E_PRIVILEGED_SESSION` to anyone who is not a person before the project, the row or the disk is looked at. `RecordingPrepare` is not one of them, and neither is any other operation of this section: the tests prove that after any sequence of them by an agent the helper was never started.

**No approval rule authorizes a capture.** A rule is matched by the kind of an action (`core/approval`). The kinds of a capture (every name that starts with `capture` or `recording`, in any case, such as `capture`, `recording.start` or `RecordingStart`) are never covered by a rule: `approval.NeverAuto` is checked before a rule is written (`ApprovalRuleSet` answers `E_VALIDATION`, path `kind`) and again when a rule is matched, so a rule that already exists, written by hand into the database or by another version, still authorizes nothing. `RecordingStart` does not call the approval queue at all: there is nothing to approve, because there is no standing or automatic yes for a capture. A test walks every shape of a rule (any kind, with and without a connection, a project, a limit per item and per day, enabled or not) against `Decide` for the capture kinds.

### 19.4 The indicator

The indicator is driven by the host. `capturehost` keeps an `Indicator`: a count of the helper processes that were asked to capture (`Helper.Begin`) and have not exited. It rises when the host sends the start request and falls when the process has ended, by whatever way (a stop, a kill, a crash, the deadline); a helper that only lists sources never raises it. It does not read the recording row, the window or the page, so none of them can hide it.

The server tells the shell: `GET /api/v1/system/capture` (local mode, `admin` scope like `system/scheduler`; in network mode 404) answers `{"active": bool, "since_ms": n, "helpers": n}`. The desktop shell asks it once a second while its server runs and draws the answer in the system tray: **a red icon and the tooltip "Stuarty is recording the screen"**, which exists whether the window is open, hidden or closed (the tray is created for it even when the background mode is off). If the server stops answering while the indicator is on, the shell keeps it on for five seconds and then removes it, since the helper dies with its server. On the web, the browser's own indicator is the system one, and the page also shows a banner, "Recording", at the top of the editor from the moment a recording of the project is in `recording` or `paused`, on every page of the editor and not only in the dialog.

Limit [open, for the author]: Windows Graphics Capture draws its own border around what it captures; the host does not and cannot hide it, and the tray icon is the second, independent signal. The drawing of the tray is checked by hand on a desktop (start a recording, close the window, watch the tray); the test of the epic proves the signal with a fake helper.

### 19.5 Applying a proposal

Only a ready recording that a person added and that is on a timeline has something to cut. The service finds the clips whose assets are the tracks of the recording on the timeline (`timeline_id`, or the default) and builds the batch from the accepted ranges, last range first, with `item.split` and `item.delete` (ripple `track`) only: no command is new and one `Undo` restores the timeline. A range is in the clock of the recording; the start of each track on that clock (section 16.2) moves it into the source of each asset. The plan is a pure function of the ranges, the clips and the frame rate (`core/recording`). A clip with a speed other than 1 is `E_INVALID_STATE` (cut it by hand), and so is a recording that is not on the timeline (the hint says to place it first). The batch is sent through `Edit` with the revision read just before and with no merge: its steps are frames of that revision, so any edit made meanwhile, by an agent or by the same person, is `E_REVISION_CONFLICT`, nothing is applied and the proposal stays pending.

### 19.6 Storage

The table `recordings` gets three columns (SDD-005 section 5.18): `plan`, `marks` and `proposals`, JSON text, empty or `[]` by default. A plan holds no window title (the sealed `source_label` keeps the one the person saw), no device name and no pointer position; marks and proposals hold the text of an agent, cleaned and bounded, and are not sealed because they are neither the user's own words about the screen nor what was on it. They are deleted with the row.

### 19.7 Tests

`core/recording` tests the plan, the marks, the ranges and the batch planner. The service tests cover every operation and limit, the unchanged revision after a proposal, one batch and one undo after a decision, and a revision conflict. The security tests (T-165 to T-167 of SDD-007) cover: no sequence of agent operations over any surface starts the helper; each start-or-resume path (the five operations of 19.3) is refused for an agent; no rule shape authorizes a capture; an agent never decides a proposal; and the indicator follows a fake helper's process. The tray drawing is checked by hand (19.4).
