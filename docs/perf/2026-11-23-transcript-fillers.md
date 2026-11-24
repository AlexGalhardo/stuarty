# Word timing and filler recall of the pinned whisper.cpp, 2026-11-23

Written by epic 69 (task 2) before the detector was promised to anyone (SDD-023 section 7, SDD-011 section 10.8). Decision of the author D-133: whisper.cpp stays and its word timing is measured first. This is not a `perfreport` table: it measures accuracy, not speed, and has no limit that a build can fail on; the regression floors it fixed are in `core/transcript/eval_test.go`.

## Summary

| Question | English | Portuguese (pt-BR) |
| --- | --- | --- |
| Does whisper keep a spoken filler as a word? | Mostly: `um` and `hmm` always (written `um`), `er` always (written `ER`), `uh` once in five | Rarely as itself: `hum` is written `um`, `ahn` is written `A8`, a lengthened `é` is written `é` |
| Detector recall of fillers | 0.69 (11 of 16) | 0.06 (1 of 16) |
| Detector recall of repetitions | 1.00 (6 of 6) | 0.83 (5 of 6) |
| Detector precision | 0.94 (17 of 18) | 1.00 (6 of 6) |
| Onset of a word that starts a sentence | median 25 ms, p90 96 ms | median 27 ms, p90 133 ms |
| Onset of any word | median 158 ms, p90 595 ms | median 237 ms, p90 626 ms |

What the numbers say:

1. **Repetitions are reliable in both languages**, and so is cutting a sentence that starts after a pause: the post-processing of SDD-011 section 10.4 puts the onset after a pause within about 25 ms of the audible onset.
2. **English hesitations that whisper writes are found**; the ones it drops (`uh` in this fixture) are not in the transcript, so no rule over the transcript can find them.
3. **Portuguese hesitations are mostly not findable from the text.** Whisper writes a spoken `hum` as `um` and a lengthened `é` as `é`: the same words as the article and the verb, which the rule must not flag. The detector flags only what whisper writes unambiguously (`hum`, `hmm`, `ahn`, `éé`, `ãh`, `né`, `tipo`).
4. **The onset of a word inside a run of speech is about 160 ms (English) to 240 ms (Portuguese) off in the median** and up to 600 ms at the 90th percentile, because whisper.cpp without DTW gives contiguous words and the post-processing can only correct a boundary where there is a pause. Deleting a range whose edges are inside a run (one `um` between two words) cuts at the whisper times: a neighbouring word may lose up to that error (a median of 160 to 240 ms, more at the 90th percentile), or part of the filler may stay. A refinement that snaps the edges of a cut to the silences of the asset would fix most of this without a new dependency; it is not part of epic 69 and is recorded as a risk.

An aligner would not change 3, which is about the words that were written, and would change 4 only for the words inside a run. It is a new dependency and the author's decision (D-133); this measurement does not ask for it.

## Method

- Helper and model exactly as pinned: whisper.cpp `v1.9.2`, model `small` (SHA-256 `1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b`), installed by `stuarty tools:install whisper --model small --yes` into a scratch workspace, run through the product (`stuarty transcribe --language en|pt --wait`) so that the post-processing of SDD-011 section 10.4 applies. The transcript artifacts are kept in `core/transcript/testdata/`.
- Fixture: `core/transcript/testdata/gen-speech.ps1` (Windows only) speaks 12 sentences per language with the voices `Microsoft Zira Desktop` (en-US) and `Microsoft Maria Desktop` (pt-BR) and writes the labels from the text it spoke: 16 fillers and 6 repetitions per language, and the onset of every word as the synthesizer reports it. 51 s of English, 74 s of Portuguese, 16 kHz mono.
- A labelled item is a filler word, or the first copy of a repeated sequence (the one a person removes). A suggestion matches an item of the same kind when their time ranges come within 300 ms of each other. Precision is the share of suggestions that match an item, recall the share of items matched.
- Word timing: the words the labels and the transcript agree on (longest common subsequence of the normalised texts: 149 of 157 in English, 140 of 162 in Portuguese), onset error in absolute milliseconds against the synthesizer's onset. The synthesizer's onsets agree with `silencedetect` to about 60 ms after a pause (checked on the first sentence boundary: 4045 ms reported against 4103 ms of silence end), which bounds how exact this truth is.
- Machine: Windows 11, AMD Ryzen 7 5700X3D, FFmpeg LGPL build of the test folder. The transcription took 28 s (English) and 31 s (Portuguese) on a shared machine; speed is not judged here.

## Results

`go test ./core/transcript -run TestLabelledFixture -v` prints these numbers and every miss.

| | Items | Found | Suggestions | Right | Precision | Recall |
| --- | --- | --- | --- | --- | --- | --- |
| English, fillers | 16 | 11 | | | | 0.69 |
| English, repetitions | 6 | 6 | | | | 1.00 |
| English, all | 22 | 17 | 18 | 17 | 0.94 | 0.77 |
| Portuguese, fillers | 16 | 1 | | | | 0.06 |
| Portuguese, repetitions | 6 | 5 | | | | 0.83 |
| Portuguese, all | 22 | 6 | 6 | 6 | 1.00 | 0.27 |

Of the 5 English fillers counted as missed, 4 are `uh` that whisper dropped. The fifth is an `um` that the detector found 570 ms away from its label, so the same event is counted as a miss and as the one false positive: the synthesizer's clock drifts inside long sentences. Counting that event once, English filler recall is 12 of 16 (0.75) and precision 18 of 18. The table keeps the stricter numbers.

The Portuguese miss of a repetition is the single-letter `a a` of "a, a câmera", which whisper wrote as one `A`.

## Thresholds

Fixed at about ten points under the measurement, as regression floors for `TestLabelledFixture`; they hold the rules and the post-processing, not the model:

| | Precision | Recall | Filler recall | Repetition recall | Median onset of a sentence start |
| --- | --- | --- | --- | --- | --- |
| English | at least 0.85 | at least 0.70 | at least 0.60 | at least 0.90 | at most 60 ms |
| Portuguese | at least 0.85 | at least 0.20 | no floor | at least 0.70 | at most 60 ms |

## Limits of this measurement

- The audio is synthetic. A synthesizer says `uh` as a vowel that whisper often drops, and says the Portuguese `hum` as the article `um`, because the letter `h` is silent. Real hesitations, with a creak, a nasal hum and a long vowel, may be written more or less often. The fixture proves what the rules do with what whisper writes, and bounds what whisper does with a clean, isolated hesitation; it does not stand in for recordings of people.
- One voice per language, one model, 22 labelled items per language: the fractions move by 0.05 for one item.
- The onset truth has an error of the order of 60 ms.

A human-recorded fixture with labels (a recording of a person who hesitates, labelled by hand) would replace this one; it needs a recording the repository may keep, which is the author's to provide.

## Reproduce

```text
powershell -NoProfile -ExecutionPolicy Bypass -File core/transcript/testdata/gen-speech.ps1 -Lang en -Out <folder>
powershell -NoProfile -ExecutionPolicy Bypass -File core/transcript/testdata/gen-speech.ps1 -Lang pt -Out <folder>
stuarty tools:install whisper --model small --yes
stuarty media:import <folder>/en.wav <folder>/pt.wav --project <id> --wait
stuarty transcribe --project <id> --asset <asset> --language en --wait
```

The transcript artifact is `artifacts/<hash>/transcript-1-<key>.json` of the workspace. The labels and the transcripts used here are `core/transcript/testdata/<lang>.labels.json` and `<lang>.transcript.json`.
