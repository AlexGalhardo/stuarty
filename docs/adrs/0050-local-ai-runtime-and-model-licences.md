# ADR-0050: Local AI runtime and the licence policy of models

- Status: Accepted (2026-11-23, author decision D-133)
- Deciders: author
- Amends: `.agents/rules/rust.md` item 1 and AGENTS.md rule 5 (where Rust may live), the same way ADR-0049 amended them for `capture/`
- Links: ADR-0011, ADR-0021, ADR-0032, ADR-0036, ADR-0042, NFR-043, SDD-007 section 6, SDD-024 (to be written by epic 70)

This record decides how local models run in Stuarty and which models may be offered.

## 1. Context

Transcription already runs locally through whisper.cpp as a separate process, with pinned downloads (`engine/transcribe/manifest.json`). The product has no Python and no ONNX runtime. The author asked to evaluate WhisperX, YuNet, BEN2, MediaPipe and Depth-Anything-V2. A project licence says nothing about its weights, and several differ.

## 2. Decision

1. **One AI helper**, `stuarty-aihelper`, a separate process built on ONNX Runtime (MIT), under the process policy of SDD-007 section 6, started by the server through `engine/aihost`. No Python and no cgo in the product.
   It is written in **Rust, in a new top-level folder `aihelper/`**: the third and last place Rust may live, after `renderer/` and `capture/` (ADR-0049). Rules 2 to 4, 7 and 8 of `.agents/rules/rust.md` apply to it; the rules about bit-exact pixels and WebAssembly do not, because the helper composes nothing. The one crate approved for it is `ort` (the ONNX Runtime binding) at an exact version, in its `load-dynamic` mode: the official ONNX Runtime library (MIT) is loaded from a path and is fetched through a pinned manifest and script, like the FFmpeg and whisper.cpp pins, so the build downloads nothing. Any other crate is a decision of the author (D-139).
2. **Execution:** DirectML on Windows, CoreML on Apple systems, CPU everywhere. The CUDA build is not shipped (D-133).
3. **Models are never bundled.** Each is downloaded only on a person's explicit request, shows its size and licence first, and is pinned by SHA-256. With no model installed, the product opens no connection.
4. **Licence gate:** a model enters the catalogue only when the licences of its code and its weights both allow use and redistribution in an MIT product, and the evidence is recorded. Non-commercial, GPL, AGPL, gated or unstated weights are refused.
5. External APIs stay optional, through the user's own connection, the approval queue and the cost ceilings.

## 3. Decision per tool

| Tool | Code | Weights | Decision |
| --- | --- | --- | --- |
| whisper.cpp | MIT | MIT | Keep |
| YuNet | MIT | MIT (`LICENSE` of opencv_zoo at commit `48b10fa505aeb51fbd127fc60a5c660040e823b7`, Copyright (c) 2020 Shiqi Yu); origin recorded below | Accept (checked by epic 70 on 2026-11-24) |
| Depth-Anything-V2 Small | Apache-2.0 | Apache-2.0 | Optional |
| Depth-Anything-V2 Base, Large, Giant | Apache-2.0 | CC-BY-NC-4.0 | Refused |
| BEN2 base | MIT | No separate licence text [to validate] | Blocked until confirmed |
| MediaPipe Face Landmarker | Apache-2.0 | Apache-2.0 per its model card | Only if YuNet measures insufficient; desktop runtime [to validate] |
| WhisperX | BSD-2-Clause | Per-language aligners; gated pyannote | Not shipped (Python, ADR-0021) |
| RobustVideoMatting | GPL-3.0 | Undeclared | Refused |

## 4. Alternatives

| Option | Why not |
| --- | --- |
| A bundled Python runtime | Size, patching and supply-chain surface for every user |
| One executable per model | Duplicates the runtime and the process review |
| cgo bindings in the server | Breaks the no-cgo rule (ADR-0021) |
| External APIs by default | Contradicts the local-first decision |

## 5. Consequences

- A second native helper to build and ship, with its size measured in epic 70.
- Some quality is given up by refusing larger depth models and diarization.
- The catalogue carries legal evidence and is tested, so an excluded model cannot enter by accident.

## 6. Decisions of the author (D-133)

1. BEN2 base stays out until its weights licence exists in writing or in a licence file.
2. The residual risk of the YuNet training data is accepted; epic 70 records its origin and stops if a licence forbids this use.
3. Execution stays with DirectML, CoreML and CPU.

## 7. Origin of the YuNet weights (recorded by epic 70, 2026-11-24)

- File: `face_detection_yunet_2023mar.onnx`, 232,589 bytes, SHA-256 `8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4`, from `https://github.com/opencv/opencv_zoo` at commit `48b10fa505aeb51fbd127fc60a5c660040e823b7`, folder `models/face_detection_yunet/`.
- Licence: MIT, in `LICENSE` of that folder (Copyright (c) 2020 Shiqi Yu); the README of the folder says all its files are licensed under it.
- Origin: the README names the model source as the `libfacedetection.train` project of Shiqi Yu (`tasks/task1/onnx/yunet.onnx`) and reports its accuracy on the WIDER Face validation set. The training set of that project is, per the upstream project, WIDER Face; the terms of that dataset are not the MIT licence of the weights, and this record has not verified them against the dataset's own page.
- **Residual risk, accepted by the author (D-133):** the weights are MIT-licensed by their author, while the licence of the data they were trained on is separate and not restated by the weights licence. The product shows this to the person before the download (SDD-024 section 3 item 2) and never bundles the file. The epic stops if a licence forbids this use; none was found.
- Runtime: ONNX Runtime 1.24.4 DirectML build from `Microsoft.ML.OnnxRuntime.DirectML` on nuget.org, MIT (SDD-024 section 4). The `ort` crate is `=2.0.0-rc.15`: the project has published no stable 2.x release, which `.agents/rules/rust.md` item 2 records.
