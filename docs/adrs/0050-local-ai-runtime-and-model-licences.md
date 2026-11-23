# ADR-0050: Local AI runtime and the licence policy of models

- Status: Accepted (2026-11-23, author decision D-133)
- Deciders: author
- Links: ADR-0011, ADR-0021, ADR-0032, ADR-0036, ADR-0042, NFR-043, SDD-007 section 6, SDD-024 (to be written by epic 70)

This record decides how local models run in Stuarty and which models may be offered.

## 1. Context

Transcription already runs locally through whisper.cpp as a separate process, with pinned downloads (`engine/transcribe/manifest.json`). The product has no Python and no ONNX runtime. The author asked to evaluate WhisperX, YuNet, BEN2, MediaPipe and Depth-Anything-V2. A project licence says nothing about its weights, and several differ.

## 2. Decision

1. **One AI helper**, a separate process built on ONNX Runtime (MIT), under the process policy of SDD-007 section 6. No Python and no cgo in the product.
2. **Execution:** DirectML on Windows, CoreML on Apple systems, CPU everywhere. The CUDA build is not shipped (D-133).
3. **Models are never bundled.** Each is downloaded only on a person's explicit request, shows its size and licence first, and is pinned by SHA-256. With no model installed, the product opens no connection.
4. **Licence gate:** a model enters the catalogue only when the licences of its code and its weights both allow use and redistribution in an MIT product, and the evidence is recorded. Non-commercial, GPL, AGPL, gated or unstated weights are refused.
5. External APIs stay optional, through the user's own connection, the approval queue and the cost ceilings.

## 3. Decision per tool

| Tool | Code | Weights | Decision |
| --- | --- | --- | --- |
| whisper.cpp | MIT | MIT | Keep |
| YuNet | MIT | MIT per the README; training data origin [to validate] | Accept, after recording that check |
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
