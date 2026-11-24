# SDD-024: The local AI helper, face tracking and vertical cuts

- Status: Draft (2026-11-24). Written by epic 70 before its code.
- Depends on: ADR-0002, ADR-0007, ADR-0008, ADR-0011, ADR-0021, ADR-0031, ADR-0032, ADR-0050, SDD-000 (names), SDD-001 (keyframes and crop), SDD-002 (commands), SDD-003 (the surfaces), SDD-007 sections 6.8 and 6.12, SDD-011 section 10 (the transcript and the artifacts), SDD-012 (thin client), SDD-023 (the words of an asset).
- Implements: ADR-0050 and the author decisions of 2026-11-23 (local by default, models downloaded on request, YuNet for faces) and D-133, D-139 (the helper is Rust in `aihelper/`, over `ort`).
- Owner area: epic 70 (`aihelper/`, `engine/aihost/`, `core/facetrack/`, `service/facetrack*.go`, `service/cuts*.go`, the adapters, the vertical-cut entry of the editor).

## Contents

1. Overview
2. The helper and its host
3. The catalogue and the install
4. Licence evidence
5. The helper protocol
6. The face track job
7. Tracking and framing
8. Cut proposals
9. Operations, scopes and surfaces
10. Errors and limits
11. Measured numbers
12. Tests

## 1. Overview

A creator turns a long video into several vertical cuts that keep the speaker's face in frame, on their own computer. Five rules carry the design:

1. **Detection is one thing, decision is another.** The helper finds faces in frames and says nothing about a project. Where the frame goes, how fast it may pan and which cuts are worth keeping are rules of `core/` (rule 1 of AGENTS.md); the UI computes none of them (SDD-012 section 1).
2. **Local by default.** The face model runs on the user's machine, in a separate process, with no cgo in the server (ADR-0050 item 1). Nothing leaves the machine.
3. **Nothing is downloaded until a person asks.** The helper runtime and the model are two pinned downloads started only by the install step of section 3, which shows their size and licence first. With nothing installed the product opens no outbound connection (ADR-0032, ADR-0050 item 3).
4. **A proposal changes nothing.** `CutsPropose` is a read operation. The timelines of a project change only when a person accepts a proposal and the edit batch it carries is sent (section 8).
5. **Deterministic.** The same track and the same options give the same keyframes, byte for byte, on every system: the rules of section 7 use integers only.

## 2. The helper and its host

- `stuarty-aihelper` is a Rust program in `aihelper/` (ADR-0050 item 1, the third place Rust may live; rules 2 to 4, 7 and 8 of `.agents/rules/rust.md` apply). It is a few hundred kilobytes and links no ONNX Runtime: it loads the official library from a path the server passes (`ort` in its `load-dynamic` mode), so no build step downloads a binary.
- The server never links the runtime. It starts the helper through `engine/aihost`, which uses `ffmpeg.Runner.RunHelper` and so the process policy of SDD-007 section 6 unchanged (section 6.12 lists what differs): no root, the minimal environment, a run directory, a deadline, cancel with a grace period, kill of the whole tree.
- **Execution providers.** DirectML on Windows, then CPU (ADR-0050 item 2); CoreML on Apple systems after epic 43. The helper tries DirectML first when asked for `auto`, falls back to CPU when the provider cannot be created, and says which one it used. The CUDA build is not shipped.
- **The runtime library.** The official ONNX Runtime (MIT) in the build that includes DirectML (`Microsoft.ML.OnnxRuntime.DirectML`, distributed on nuget.org). One library serves both providers. It is a pinned download of section 3, never part of the installer.
- Platforms: Windows amd64 is complete in this epic. Linux and macOS entries stay `published: false` until epic 43 builds the helper there and the spike of section 11 is repeated.

## 3. The catalogue and the install

The catalogue is `engine/aihost/manifest.json`, embedded in the binary, in the format and with the validation of `engine/transcribe/manifest.json` (SDD-007 section 6.8 item 3): per file the name, kind, version, operating system and architecture, URL on a listed host, exact size, SHA-256, licence and notice text, and for an archive the members taken by their exact names. It lists three files per platform, installed together as one **install set** named `face`:

| Entry | Kind | What it is | Licence |
| --- | --- | --- | --- |
| `aihelper` | helper | `stuarty-aihelper`, an asset of this repository's `helpers-<version>` release, built in CI (`published: false` until a CI run has built it) | MIT |
| `onnxruntime` | helper | `onnxruntime.dll` and `onnxruntime_providers_shared.dll`, taken by name from the pinned `Microsoft.ML.OnnxRuntime.DirectML` package | MIT |
| `yunet` | model | `face_detection_yunet_2023mar.onnx`, 232,589 bytes, from opencv_zoo at a pinned commit | MIT |

Rules:

1. **Only a person installs.** The install is `ToolsInstall` with `name` `face` (the epic's `ModelInstall`): `stuarty tools:install face` and the install button of the editor. The caller rules, the `202` answer, the progress read from `ToolsStatus`, the size cap, the digest check in constant time, the archive extraction by name and `tools/installed.json` are those of SDD-007 section 6.8 unchanged. No job, edit, read or agent call downloads anything.
2. **Size and licence come first.** `ToolsStatus` carries `offers`: for each installable set its files (name, bytes, host, licence), the total, and whether it is installed. The editor and the terminal show them before the person confirms; `ToolsInstall` repeats them in its answer.
3. **A wrong file is refused and removed.** A different size or digest deletes the file and fails with `E_TRANSCRIBE_UNAVAILABLE` and `details.reason` `checksum_mismatch`; an interrupted download leaves no file in `tools/<name>/` and nothing in `tools/.download/`.
4. **Verified at each start.** Before the helper runs, `VerifiedTool` recomputes the digests of the helper, the runtime library and the model against `installed.json` (SDD-007 section 6.8 item 6). A changed file makes the feature unavailable and `doctor` says why.
5. **Hosts.** The host allowlist of the install client is the union of the hosts of every manifest. The new ones are `api.nuget.org` and `media.githubusercontent.com` (where the pinned model URL redirects); each manifest may name only its own hosts, and each hop of a redirect is checked.
6. **Not installed.** A `face_track` job or a `CutsPropose` call without the set fails with `E_TRANSCRIBE_UNAVAILABLE` (the code of every local helper that is not installed) and `details` `{missing, download_bytes, install_command, tool, model}` with `tool` `face`, `model` `yunet` and `install_command` `"stuarty tools:install face"`; it downloads nothing.
7. **The catalogue is a gate.** A model enters it only with the licence evidence of section 4, and a test refuses an entry whose licence is not on the allowlist of ADR-0050 item 4 (non-commercial, GPL, AGPL, gated or unstated weights).

## 4. Licence evidence

| Part | Licence | Evidence read on 2026-11-24 |
| --- | --- | --- |
| YuNet weights | MIT | `models/face_detection_yunet/LICENSE` of opencv_zoo at the pinned commit: MIT, Copyright (c) 2020 Shiqi Yu; its README says "All files in this directory are licensed under MIT License" |
| YuNet training data | Not stated by the weights licence | The model comes from the `libfacedetection.train` project (README of opencv_zoo, "Model source"), reported there on the WIDER Face validation set. The training set is, per the upstream project, WIDER Face, whose own terms are not the MIT licence of the weights. This is the **residual risk accepted by the author in D-133**; it is recorded here and in ADR-0050 and is not hidden from the user: the install prompt says which dataset the model was trained on |
| ONNX Runtime | MIT | `LICENSE` of the NuGet package: MIT, Copyright (c) Microsoft Corporation; `ThirdPartyNotices.txt` ships in the package and is copied into the notices |
| `ort` crate | MIT OR Apache-2.0 | `LICENSE-MIT` and `LICENSE-APACHE` of `ort` 2.0.0-rc.15 and of its dependencies (`ort-sys`, `libloading`, `windows-link`, `smallvec`); `cargo deny check` passes with the allowlist of `internal/tools/notices` |
| `stuarty-aihelper` | MIT | this repository |

STOP rule of the epic: a weights licence that forbids this use stops the feature. None does; the open point is the dataset, accepted by D-133.

## 5. The helper protocol

Version 1. The helper is started with constant arguments chosen by `engine/aihost`; no file name of a project, no URL and no user text is ever an argument.

```text
stuarty-aihelper --version
stuarty-aihelper probe --runtime <abs path of onnxruntime.dll>
stuarty-aihelper faces --runtime <path> --model <path> --width 640 --height 640
                       [--provider auto|cpu|directml] [--min-score-milli 700] [--threads N]
```

- `faces` reads frames of `width x height` pixels in RGB24 from standard input until it ends and writes one JSON object per line on standard output. The YuNet model of the catalogue has a fixed input of 640 x 640, so the host scales each decoded frame to fit and pads it at the right or the bottom (`scale=640:640:force_original_aspect_ratio=decrease,pad=640:640:0:0`) and maps the boxes back by the one scale factor. `width` and `height` are multiples of 32 from 32 to 4096.
- Lines: `{"type":"ready","protocol":1,"provider":"cpu"|"directml","width":W,"height":H}` once, then `{"type":"frame","index":N,"faces":[{"x":..,"y":..,"w":..,"h":..,"score":..}]}` per frame (pixels of the padded frame, at most 16 faces, best first, overlaps removed at 0.3), then `{"type":"done","frames":N,"infer_ms":M}`.
- A failure is `{"type":"error","code":C,"message":T}` and exit code 1; `code` is one of `usage`, `frame_size`, `runtime_load`, `model_load`, `model`, `session`, `provider_unavailable`, `inference`, `short_frame`, `io`. The message carries no path and no secret.
- The host bounds every reply: a line is at most 64 KiB, the number of frames cannot exceed the number it sent, and a line that is not one of these objects ends the run with `E_INTERNAL`.

## 6. The face track job

`JobStart` kind `face_track`, scope `media`, for an asset of the project. Arguments: `asset_id` (required), `fps` (1 to 10, default 5; frames analysed per second), `start_ms` and `end_ms` (default the whole asset).

- The job refuses at once, with no row and no download, when the set is not installed (section 3 item 6), when the asset is not a ready video, or when the range is longer than `STUARTY_MAX_TRANSCRIBE_SECONDS` of video (the same ceiling; no new variable).
- FFmpeg decodes the range to the analysis frames through a pipe into the helper (`ffargs` with the output `rawvideo`, `fps` and the scale and pad filters of section 5). Progress is the fraction of frames delivered.
- The result is the artifact `facetrack` (variant `yunet-2023mar-<fps>-<start>-<end>`, extension `json`) of the asset, and the job result `{asset_id, samples, faces, provider, fps}`:

```json
{"version":1,"model":"yunet-2023mar","provider":"cpu","source":{"width":1920,"height":1080},
 "fps":5,"start_ms":0,"end_ms":60000,
 "samples":[{"t":0,"faces":[{"x":4100,"y":2000,"w":800,"h":1400,"s":930}]}]}
```

Coordinates are integers in ten-thousandths of the source frame (`x`, `w` of its width; `y`, `h` of its height) and `s` is the score in thousandths, so nothing in the artifact is a float. `t` is milliseconds on the asset's time axis.

The same asset, range, fps and model give the same artifact: a repeated job finds it stored and returns at once (the pattern of the `transcript` artifact).

## 7. Tracking and framing

All of it is `core/facetrack`, pure, in integers (positions in ten-thousandths of the source frame, times in milliseconds). Defaults in brackets; every option is validated with a range.

1. **Primary face.** Per sample the face with the highest `score * area` is the candidate. The track keeps its subject: it changes to another face only when that face's `score * area` is at least 1.5 times the current subject's for 3 consecutive samples, so two people talking do not make the frame jump on every breath.
2. **Hold.** With no face in a sample the last position is held for `hold_ms` [1500]; beyond it the frame eases to the centre of the source over the same time.
3. **Smoothing.** The target position moves the frame by an exponential smoothing step of time constant `smoothing_ms` [400], computed in integers.
4. **Dead zone.** The frame does not move while the target is within `dead_zone` [800, ten-thousandths of the visible window] of the current centre: a speaker who nods does not shake the picture.
5. **Maximum pan speed.** The centre moves at most `max_pan` [6000, ten-thousandths of the source width per second], so a cut between two speakers is a quick pan and never a jump.
6. **Keyframes.** One keyframe is written when the centre changed by at least 1/10000 and at least `keyframe_ms` [200] passed since the last one, plus the first and the last, so a long clip does not carry thousands of keyframes.
7. **The window.** For a source of `W x H` and a target frame of `FW x FH` with fit `cover`, the visible fraction is `vw = min(1, H*FW / (W*FH))` of the width and `vh = min(1, W*FH / (H*FW))` of the height. The window is centred on the track and clamped to the frame: `crop_left = clamp(cx - vw/2, 0, 1 - vw)`, `crop_right = 1 - vw - crop_left`, and the same with `cy` for `crop_top` and `crop_bottom` when `vh < 1`. A face that would leave the window is kept inside as far as the pan speed allows.
8. **The command `clip.apply_face_framing`** (scope `edit`) takes `item_id`, the options, and an internal `resolved` member filled by the service from the `facetrack` artifact (the pattern of `clip.remove_silences`; a caller never sends it). It sets `fit` `cover` and writes keyframes of `crop_left`, `crop_right` (and `crop_top`, `crop_bottom`) over the source range the clip plays, replacing any it wrote before, in one undoable step. Keyframes the user made on those four properties are not touched: the command refuses with `E_INVALID_STATE` and a hint when the clip already has crop keyframes it did not write (marked by the command's own name in the keyframe group).
9. **Acceptance of the rules.** On the drawn fixture of section 11 reframed from 16:9 to 9:16, the face box is inside the window in at least 95% of frames; golden tracks fix the keyframes byte for byte.

## 8. Cut proposals

`CutsPropose` (scope `edit`, no change to the project) returns up to `count` [5] non-overlapping proposals of an asset, from its transcript (SDD-023 section 2) and its face track:

1. The words are grouped into sentences (a pause of 700 ms or more, or terminal punctuation, ends one). Consecutive sentences are joined into a range of `min_ms` [20,000] to `max_ms` [60,000]; a range starts at a sentence start and ends at a sentence end.
2. A range needs a face in at least 70% of its samples; the others are not proposed.
3. The score is the words per second of the range (a dense stretch beats a slow one) plus the share of the range with a face, rounded; ties are broken by the earlier start, so the answer is deterministic.
4. Each proposal is `{id, start_ms, end_ms, words, face_share, score, title, batch}`. `title` is the first words of the range. `batch` is the list of commands of the edit a person accepts: for a new timeline of the project, `settings.reframe` `9:16`, `clip.add` of the asset for the range, and `clip.apply_face_framing` on the clip. The UI and an agent send `batch` as it is; neither builds it.
5. **Accepting is the person's act.** The editor shows the proposals with the range, the words and a preview, and creates the timeline and sends the batch for each one the person keeps (`item:add` then `Edit`). An agent proposes the same way and stops there: it cannot create the timelines of a proposal on its own scope until the review flow of the approval queue (epic 64) covers it, and until then it uses `edit` with the person's own scope only when the person asked for it.
6. Without a face track `CutsPropose` fails with `E_INVALID_STATE` and the hint "run a face_track job for this asset"; without a transcript it fails like `TranscriptView`.

## 9. Operations, scopes and surfaces

| Operation | Scope | HTTP | CLI | MCP |
| --- | --- | --- | --- | --- |
| `ToolsInstall` (set `face`) | person only (SDD-007 6.8) | `POST /system/tools/install` | `tools:install face` | none |
| `ToolsStatus` (`offers`) | `read` | `GET /system/tools` | none | none |
| `JobStart` kind `face_track` | `media` | `POST /projects/{id}/jobs` | `job:start` | `job_start` |
| `CutsPropose` | `edit` | `GET /projects/{id}/cuts?asset_id=` | `cuts:propose` | `media_inspect` with the section `cuts` |
| `clip.apply_face_framing` | `edit` | `POST /projects/{id}/edit` | `edit` | `edit` |

The MCP catalogue keeps its tool count and its size budget (NFR-020, `internal/tools/surfacebudget`): the new job kind and the new section are values of existing arguments.

## 10. Errors and limits

| Case | Code | Details |
| --- | --- | --- |
| The set is not installed | `E_TRANSCRIBE_UNAVAILABLE` | `tool` `face`, `model` `yunet`, `missing`, `download_bytes`, `install_command` |
| A download refused | `E_TRANSCRIBE_UNAVAILABLE` | `reason` `checksum_mismatch`, `download_failed`, `bad_archive` or `not_published` |
| The asset has no video or is not ready | `E_INVALID_STATE`, `E_ASSET_NOT_READY` | |
| A range or an option out of range | `E_VALIDATION` | `path` |
| The helper failed | `E_INTERNAL` | `reason` the helper's `code` |
| The deadline passed | `E_INTERNAL` | `reason` `timeout` |

The deadline of a run is the duration of the range divided by 2 plus 60 seconds, at most 2 hours; a stall (no line for 120 seconds) kills it. At most one `face_track` job runs at a time (`E_QUEUE_FULL`, `details.limit` `face_tracks`).

## 11. Measured numbers

`docs/perf/2026-11-24-aihelper-spike.md`: on a loaded Ryzen 7 5700X3D the helper runs YuNet at 145 to 209 frames per second on the CPU and 254 to 262 with DirectML, creating the DirectML session costs 2 to 3 seconds, and the drawn fixture is detected in 100 of 100 frames. A one-hour video analysed at 5 frames per second is 18,000 frames and takes a few minutes on the CPU alone. Detection on real footage is not measured until a licensed face video is available.

## 12. Tests

- `aihelper/`: unit tests of the decoding, the argument parser and the JSON escaping; the Rust gate of `.agents/rules/rust.md`.
- `engine/aihost`: a fake helper (the test binary itself) for the protocol: a normal run, a failing helper, a line that is not JSON, too many frames, a deadline, a cancel, and a run with the real FFmpeg through the pipe.
- The catalogue: `TestSecurity_T27_AIManifestPinsEveryHelper` (HTTPS on the listed host, digests in lower-case hex, only allowed licences), a wrong digest, an interrupted download, and `TestSecurity_T26_FaceInstallNeverAutoDownloaded` (no request without the install call, no connection with nothing installed).
- `core/facetrack`: golden tracks, determinism (twice, identical), the dead zone, the speed limit, the hold, the subject change, the 95% rule on the drawn fixture, and properties (the window is always inside the frame).
- `service` and the adapters: the job with the fake helper, `CutsPropose` changes no revision, parity of the surfaces of section 9.
- The real run (the helper, the runtime and the model installed) is skipped unless `STUARTY_TEST_AIHELPER_DIR` names a folder that holds them.
