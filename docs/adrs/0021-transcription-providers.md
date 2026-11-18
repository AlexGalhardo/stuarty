# ADR-0021: Transcription behind a provider interface; whisper.cpp local by default

- Status: Accepted (2026-11-14); transcription becomes one capability of the provider layer of [ADR-0036](0036-ai-provider-layer.md) (2026-11-18); whisper.cpp stays the local default
- Deciders: author (D-009, D-010), architect
- Links: author decision D-010, spikes, features, ADR-0018, ADR-0020, SDD-004

## Context

Captions are mandatory in practice for short video, and the timed transcript is the cheapest way for an agent to know what footage says. D-010: local by default with whisper.cpp, external API optional. Per-word timings are needed for word-highlight captions and for silence removal.

## Decision

1. `engine/transcribe` defines a `Provider` interface: input is the asset's normalized audio and an optional language; output is a transcript with segments and words, each word with start and duration in milliseconds of the source, and a confidence when available.
2. The default provider runs **whisper.cpp as a separate process** (MIT), like FFmpeg. The binary and the model are not inside our binary: they are downloaded to the workspace by an explicit install step (a CLI command or a button in the UI) from pinned URLs with checksums, after telling the user the size. A job or an agent call never triggers the download: when the helper is missing the call fails with the install command in the error (SDD-007 §6.8), which keeps the rule that agents cause no outbound traffic (ADR-0020).
3. A second provider calls an **OpenAI-compatible transcription HTTP API**. It is enabled only when the operator configures an endpoint and a key. Sending audio out is therefore an operator decision, never a per-call argument (ADR-0020).
4. A transcript is a derived artifact of an asset, keyed by asset hash, provider, model and language (ADR-0010). Transcribing the same asset again with the same settings returns the stored artifact.
5. Transcript times are in the source's time base. Caption items created from a transcript are computed from the clip's position, in-point and speed, so they survive a later trim by being recomputed by a command, not by hand.
6. Transcription runs as a job (ADR-0018) with progress.
7. Word timings from the provider are post-processed in one place (merge sub-word tokens, attach punctuation, clamp to segment bounds), so both providers yield the same shape.

## Evidence from the spike

Measured in `spikes/whisper-local` on one Windows 11 machine (Ryzen 7 5700X3D, 8 cores) with real LibriVox speech in Portuguese and English:

- whisper.cpp is `ggml-org/whisper.cpp`, MIT. The latest stable release, `v1.9.5`, ships no prebuilt binaries; `v1.9.2` is the newest that ships Windows binaries (CPU x64 zip of 8.2 MB, CLI `whisper-cli.exe`). The product therefore pins a tag that has assets for every platform it supports, or builds the helper itself.
- Speed at 8 threads on 90 s clips, in multiples of real time: `tiny` 17, `base` 13, `small` 4.6, `large-v3-turbo` 0.8 to 1.7. A 60 s video takes about 13 s with `small`. The CLI default of 4 threads is slower.
- Language detection found Portuguese with every model.
- Word timings are not usable as they come: the first word after a pause starts 141 to 367 ms early on average (up to 1.2 s), because each word ends where the next begins; zero-length words and words of a second or more occur. The DTW mode was slower and worse on this measure.
- The CLI exits with 0 on corrupt or silent audio and prints an error, so success must be judged by the output file, not the exit code. Progress is printed to standard error only a few times per clip.

Decisions that follow:

- The default local model is `small` (487 MB). `base` is the fast option and `large-v3-turbo` is opt-in.
- The post-processing of item 7 is mandatory and has four steps, in this order (SDD-011 §10.4): move the start of a word that follows a pause to the audible onset found by silence detection on the same audio (and trim the tail of the word before the pause); repair zero-length words by splitting the time with their neighbour; enforce a minimum word duration; derive a word confidence from its token probabilities.
- The runner uses 8 threads or the number of physical cores, whichever is smaller, and treats a missing or empty output as failure.

Implementation note (P2-RND-011): the post-processing is `transcribe.Postprocess`. On the canned output whose first word after a pause starts 300 ms before the audible onset, the repaired start equals the onset (measured error 0 ms against the 100 ms limit); the minimum word length stays 100 ms. The measurement on real speech, where the onset comes from `silencedetect` and not from the fixture, belongs to the CI job with the real helper (P2-RND-012); this record is updated with that number.

## Alternatives

- **Local only.** No way out when quality or speed is insufficient.
- **API only.** Needs a key and internet from the first use and sends user audio to a third party.
- **Link whisper.cpp through cgo bindings.** Breaks the no-cgo rule and static cross-compilation.
- **WhisperX or faster-whisper.** Better word alignment, but a Python runtime to distribute.

## Consequences

Positive: works offline and free by default; quality can be bought by configuration.

Negative: local transcription needs an install step that downloads a model of tens to hundreds of megabytes.

Negative: local speed depends on the CPU; the measured numbers and the default model come from `spikes/whisper-local`.

Negative: word timings from whisper.cpp may need the post-processing of item 7 to be usable; its quality is checked by the spike.
