# Snapping the edges of a text cut to silences, 2026-11-24

Written by epic 89 (task 5), on the labelled fixture of epic 69 (`docs/perf/2026-11-23-transcript-fillers.md`). The rule is SDD-023 section 3.1. This is an accuracy measurement, not a speed one: the floors it fixed are in `core/transcript/snap_eval_test.go`.

## Summary

| Question | English | Portuguese (pt-BR) |
| --- | --- | --- |
| One-word cuts measured | 147 | 138 |
| Cuts the snap changed (a silence within 0.3 s beside an edge) | 9 (6 %) | 9 (7 %) |
| Cuts that clip a neighbouring word, whisper times | 0.741 | 0.797 |
| Cuts that clip a neighbouring word, snapped | 0.741 | 0.797 |
| Cuts that leave part of the removed word, whisper times | 0.776 | 0.841 |
| Cuts that leave part of the removed word, snapped | 0.762 | 0.819 |
| Cuts with neither problem, whisper times / snapped | 0.082 / 0.082 | 0.058 / 0.065 |
| Of the cuts the snap changed: clip a neighbour | 3 to 3 | 6 to 6 |
| Of the cuts the snap changed: leave part of the word | 7 to 5 | 5 to 2 |

What the numbers say:

1. **The snap never made a cut worse**: it clips no more neighbours than the whisper times (the STOP condition of the epic is not met) and it leaves fewer words half cut, in both languages.
2. **It rarely applies to this fixture.** Of the one-word cuts, 120 of 147 (English) and 118 of 138 (Portuguese) have both neighbours in the same breath, with no pause between the words in the transcript: the synthetic voice speaks without pauses inside a sentence, there is no silence beside such an edge to snap to, and the rule leaves the cut at the whisper time. The snap changes only the cuts beside a pause (6 to 7 %).
3. **It does not lower the share of cuts that clip a neighbour**, and by construction cannot: the rule only moves an edge outward, into the gap the transcript shows between two words, so the snapped range contains the whisper range (section 3.1 guarantee) and a cut that clipped a neighbour at the whisper times clips it after the snap. What it fixes is the other defect, the part of the removed word that stays. The criterion of the epic that asks for a lower clipping share is therefore not met by this rule on this fixture; the clipping comes from word times inside a run, which only an aligner (D-133, the author's decision) or a rule that may also move an edge inward could change (see the limits).
4. The 74 % and 80 % of cuts that clip a neighbour are the word-timing error of epic 69 seen from the cut: a word inside a run is a median 160 to 240 ms off, and the tolerance here is 40 ms.

## Method

- Fixture: `core/transcript/testdata/{en,pt}.labels.json` (the onset of every spoken word, from the synthesizer) and `{en,pt}.transcript.json` (the whisper.cpp transcript of epic 69). The audio was regenerated with `gen-speech.ps1`; its labels and its content hash equal the committed ones, so the audio is the audio of the transcripts.
- Silences: `{en,pt}.silence.json` is the `silence-1.json` artifact that the product's own ingest wrote for that audio (`stuarty media:import <wav> --project <id> --wait`, pinned LGPL FFmpeg), kept in `testdata/`. The -35 dB set is read at 30 fps with the rule of the service (`FramesInside`: the start rounds up, the end down), and the transcript words are converted with the rule of `transcriptFrames`.
- A cut is one word: every word that the labels and the transcript agree on (the longest common subsequence of epic 69) and that has a labelled neighbour on each side. The range is cut at the whisper times or snapped by `transcript.SnapEdges` with the default window of 0.3 s (9 frames).
- A cut **clips a neighbour** when its start is more than 40 ms before the end of the previous word, or its end more than 40 ms after the onset of the next. The end of a labelled word is the onset of the next one, or the start of the first silence (-35 dB) between the two when there is one, because the synthesizer reports onsets only. A cut **leaves part of the word** when its start is more than 40 ms after the onset of the word or its end more than 40 ms before the end of the word.

`go test ./core/transcript -run TestLabelledFixture -v` prints these numbers.

## Thresholds

Floors of `TestLabelledFixtureSnap`, ten points under the measurement where a share would be, and exact where the guarantee is by construction:

| | English | Portuguese |
| --- | --- | --- |
| Cuts that clip a neighbour, snapped, against whisper times | not more | not more |
| Cuts that leave part of the word, snapped, against whisper times | fewer | fewer |
| Cuts the snap changes | at least 5 | at least 5 |

## Limits of this measurement

- The audio is synthetic and has almost no pause inside a sentence; recordings of people breathe, hesitate and pause, and have far more silences beside the edges of a word (the case the rule is written for), so the share of cuts the snap changes is expected to be higher there. This fixture bounds the harm (none) and shows the gain only where a pause exists.
- The truth has the error of the synthesizer clock (about 60 ms, drifting inside long sentences, see the report of epic 69), above the 40 ms tolerance; shares are comparable between the two columns, not absolute.
- A rule that also moves an edge inward (a start edge later, an end edge earlier, toward the nearest silence) was tried and dropped: on the same cuts it changed 17 (English) and 21 (Portuguese) cuts, lowered the clipping share by one cut in Portuguese and none in English, and takes away part of the words the person chose, which the transcript says the person selected.

## Reproduce

```text
powershell -NoProfile -ExecutionPolicy Bypass -File core/transcript/testdata/gen-speech.ps1 -Lang en -Out <folder>
stuarty project:create ...   (any project with a timeline)
stuarty media:import <folder>/en.wav <folder>/pt.wav --project <id> --wait
```

The silence artifact is `artifacts/<hash>/silence-1.json` of the workspace.
