# SDD-025: Screen recording

- Status: Draft (2026-11-24). Written by epic 61 before its code.
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
| Operations | `RecordingSources`, `RecordingStart`, `RecordingStop`, `RecordingCancel`, `RecordingGet`, `RecordingList`, `RecordingConfirm`, `RecordingDismiss`, `RecordingPlace` (SDD-000 section 1) |
| Events | none in epic 61: the dialog reads `RecordingGet` once a second while a recording runs (a stream event is a later addition) |
| Error codes | `E_CAPTURE_UNAVAILABLE`, `E_CAPTURE_FAILED`, `E_RECORDING_NOT_FOUND` (SDD-000 section 8) |
| Job kind shown to MCP | `capture`: a recording read through `job_get` and stopped through `job_cancel` (section 11) |
| Variables | `STUARTY_CAPTURE_HELPER`, `STUARTY_MAX_RECORDING_SECONDS` (SDD-000 section 10) |
| Folder | `<workspace>/recordings/<recording id>/capture.mp4` (SDD-000 section 9) |
| Helper | `stuarty-capture` (`stuarty-capture.exe`), protocol version 1 |

## 3. The state machine

`core/recording` holds a pure function over these states: `prepared`, `recording`, `finishing`, `ready`, `failed`, `cancelled`. `ready`, `failed` and `cancelled` are final.

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

End of input is a stop. The helper serves one recording and exits when it ends (a process that only lists sources exits when its input closes); it takes no command-line argument. The JSON is read and written by hand over the standard library, because ADR-0049 approves no JSON crate. The `encoder` is one of a closed list (`h264_nvenc`, `h264_qsv`, `h264_amf`, `h264_mf`, `libopenh264`); the helper owns the FFmpeg argument template of each, so no user text reaches an FFmpeg argument. The `output` must be absolute, end in `.mp4` and lie in a directory that exists. The host chose the encoder with `engine/ffmpeg` (`EncoderForSize`, so the capability test of an export applies).

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

At start-up, before the server accepts requests, every row in `prepared`, `recording` or `finishing` is settled: if its file has at least one decodable frame (`ffprobe` reads a duration above zero) it becomes `ready`, `review` `pending`, with the real duration and `stop_reason` `recovered`; otherwise `failed` with `E_CAPTURE_FAILED` and `details.reason` `no_frames` or `helper_lost`. A recording never stays `recording`.

## 11. Surfaces

HTTP and CLI have every operation. MCP has no new tool (the tool list is at its byte budget, NFR-020): `job_get` with a `job_id` that starts `rec_` answers the recording as a job of kind `capture` (state `queued`, `running`, `succeeded`, `failed`, `cancelled` for `prepared`, `recording` and `finishing`, `ready`, `failed`, `cancelled`), and `job_cancel` with such an id is `RecordingStop`. An agent places a recording with `edit`. No MCP route starts, confirms or dismisses a recording.

## 12. The dialog

The record dialog (`web/resources/js/`) draws what the service returns: a project, a source picker (displays and windows from `RecordingSources`), a frame rate, Start, then a visible recording state with the elapsed time and Stop, then the confirmation step (Add to the project, Not now, Delete) with the sentence of section 4. It computes no limit and no state. Where capture is unavailable it says why, in the words of the reason the service gave (section 1).

The entry is a "Record screen" button in the library panel of the editor (`editor/library/Library.tsx`), the natural place today: the media panel is where files come in. The redesign of epic 75 moves it with the panel. The dialog reads `RecordingSources` and `RecordingList` when it opens, so a recording still running, or one waiting for a person, is picked up again after the dialog was closed; while a recording runs it calls `RecordingGet` once a second, and the clock it draws is `duration_ms` of the answer, which the service takes from the last line of the helper.

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
