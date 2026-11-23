# Screen recording: epics

The index of the screen recording and local AI epics, in order of execution. The epics themselves live in `docs/plans/`, in the format of `.agents/rules/plans.md`.

| Epic | Title | Size | Priority | Depends on |
| --- | --- | --- | --- | --- |
| [61](../../plans/61-recording-foundation-windows-capture.md) | Recording foundation and Windows screen capture | XL | Must | 57, redesign choice (the spike needs neither) |
| [62](../../plans/62-windows-recording-audio-webcam-events.md) | Windows recording: audio, webcam, pause, cursor and click events | L | Must | 61 |
| [63](../../plans/63-basic-web-recording.md) | Basic web recording | M | Should | 61 |
| [64](../../plans/64-agent-operation-of-recording.md) | Agents in the recording flow | L | Must | 61, 62 |
| [65](../../plans/65-auto-zoom-smooth-cursor-click-highlight.md) | Automatic zoom, smooth cursor and click highlight | L | Must | 62 |
| [66](../../plans/66-recording-layouts-backgrounds-teleprompter.md) | Recording layouts, backgrounds and teleprompter | M | Should | 62 |
| [67](../../plans/67-macos-capture.md) | Recording on macOS | L | Should | 62, 43, signing decision |
| [68](../../plans/68-linux-capture.md) | Recording on Linux | L | Could | 62, 43 |
| [69](../../plans/69-edit-by-text-and-filler-words.md) | Edit by text, filler words and repetitions | L | Must | none from capture |
| [70](../../plans/70-local-ai-helper-and-vertical-cuts.md) | Local AI helper and face-aware vertical cuts | XL | Should | ADR-0050, 69 |
| [71](../../plans/71-caption-styles-blur-studio-voice.md) | Caption styles, sensitive-data blur and studio voice | M | Should | none from capture |
| [72](../../plans/72-optional-local-models.md) | Optional local models: background removal and depth | L | Could | 70, BEN2 licence decision |
| [73](../../plans/73-hyperframes-end-to-end-round.md) | A real end-to-end round with HyperFrames | S | Should | 33 |

Decisions: [ADR-0049](../../adrs/0049-native-capture-helper-per-os.md), [ADR-0050](../../adrs/0050-local-ai-runtime-and-model-licences.md), [ADR-0051](../../adrs/0051-agent-consent-for-capture.md), all proposed.

See [the research](research.md), [the gap analysis](gap-analysis.md) and [the execution order](roadmap.md).
