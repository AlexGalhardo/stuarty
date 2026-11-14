# Spike: local transcription with whisper.cpp (Vukem Video)

Disposable spike. Nothing here is product code. Binaries, models and audio are kept outside the repo (scratchpad); only scripts and small text results are kept here.

## Hypotheses

- H1. whisper.cpp is usable as a prebuilt sidecar on Windows.
- H2. CPU speed is acceptable.
- H3. Word-level timestamps are good enough for word-highlight captions.
- H4. The CLI has a usable output contract (JSON, exit codes, progress, language auto-detection).
- H5. Accuracy sanity check (tiny sample).

## Environment

| Item | Value |
| --- | --- |
| CPU | AMD Ryzen 7 5700X3D, 8 cores / 16 threads |
| RAM | 34,278,871,040 bytes (about 32 GB) |
| OS | Windows 11 Pro 10.0.26300, Git Bash |
| ffmpeg | 9.0.2 (gyan.dev full build) |
| whisper.cpp | 1.9.2 (`whisper-cli.exe --version` prints `whisper.cpp version: 1.9.2`), CPU backend `ggml-cpu-haswell.dll` (AVX2, FMA, F16C, OpenMP) |
| GPU | not used (CPU build) |

Note: this is a fast desktop CPU. Expect slower numbers on laptops.

## H1. Prebuilt sidecar

Source verified with `gh api repos/ggml-org/whisper.cpp` (owner/name `ggml-org/whisper.cpp`, licence MIT) and `gh release list -R ggml-org/whisper.cpp`.

Findings, as observed:

- Latest release marked "Latest": `v1.9.5`, published 2026-10-06T17:39:17Z. It has **0 assets** (`gh api .../releases/tags/v1.9.5 --jq '.assets|length'` returns 0). Same for `v1.9.4`.
- Prebuilt binaries are attached to other tags: pre-release `b5454` (2026-10-06, same day as v1.9.5, 12 assets) and the newest stable release with assets is `v1.9.2` (2026-08-04, 9 assets). The `bNNNN` tags are CI builds marked Pre-release.
- I used the newest **stable** release with Windows assets: `v1.9.2`.

Windows assets of `v1.9.2`:

| Asset | Size (bytes) |
| --- | --- |
| whisper-bin-x64.zip (plain CPU, x64) | 8,194,445 |
| whisper-bin-Win32.zip | 5,189,502 |
| whisper-blas-bin-x64.zip | 20,981,386 |
| whisper-blas-bin-Win32.zip | 12,220,980 |
| whisper-cublas-11.8.0-bin-x64.zip | 269,446,789 |
| whisper-cublas-12.4.0-bin-x64.zip | 670,611,449 |

No Vulkan asset in v1.9.2. Pre-release `b5454` adds: `whisper-bin-x64.zip` 8,928,640, `whisper-blas-bin-x64.zip` 21,715,607, `whisper-bin-win-cpu-arm64.zip` 4,371,193, `whisper-bin-win-cuda-11.8.0-x64.zip` 284,761,231, `whisper-bin-win-cuda-12.4.0-x64.zip` 684,913,404, `whisper-bin-win-cuda-13.4-arm64.zip` 291,157,544, `whisper-bin-win-opencl-adreno-arm64.zip` 4,944,716 (still no Vulkan asset). Names changed from `cublas` to `win-cuda` there.

The CLI is `whisper-cli.exe` (zip extracts to a `Release/` folder; `main.exe` is a deprecated stub). It needs the sibling DLLs (`whisper.dll`, `ggml*.dll`, several `ggml-cpu-*.dll` variants chosen at runtime, `SDL2.dll`). Supported inputs: flac, mp3, ogg, wav. The CPU zip also ships `whisper-server.exe`, `whisper-vad-speech-segments.exe`, `whisper-quantize.exe`.

Licences: whisper.cpp MIT. Models: Hugging Face repo `ggerganov/whisper.cpp` card says `license: mit`; the upstream `openai/whisper` repo is MIT and `openai/whisper-large-v3-turbo` card says `mit`.

**Verdict: confirmed**, with the caveat that the latest stable tag (v1.9.5) has no binaries. The Go app must pin an explicit tag that has assets (or build/mirror its own).

## H2. Speed on CPU

### Method

Test speech (all real human speech, LibriVox recordings, public domain; no TTS used):

- pt-BR/pt: "O Alienista" by Machado de Assis, chapter 1, reader "Leni". `https://www.archive.org/download/o_alienista_0904_librivox/oalienista_1_machadodeassis_64kb.mp3` (LibriVox book id 3058). Reference text: Project Gutenberg #57001 (Papeis Avulsos, old spelling). Accent of the reader was not checked.
- en: "Alice's Adventures in Wonderland" chapter 1, reader Kristen McQuillin. `https://www.archive.org/download/alice_in_wonderland_librivox/wonderland_ch_01_64kb.mp3`. Reference text: Project Gutenberg #11. (The existing `vukem-launch` film has no voice: `audio-provenance.json` says `"voice": false`.)
- Clips: 90 s each starting at 25 s (`ffmpeg -ss 25 -t 90 -i X.mp3 -ar 16000 -ac 1 out.wav`) plus a 300 s English clip.

Models (downloaded from `https://huggingface.co/ggerganov/whisper.cpp/resolve/main/<file>`, all multilingual, licence MIT):

| File | Size (bytes) |
| --- | --- |
| ggml-tiny.bin | 77,691,713 |
| ggml-base.bin | 147,951,465 |
| ggml-small.bin | 487,601,967 |
| ggml-large-v3-turbo.bin | 1,624,555,275 |

Command per run (word timings on, language fixed so detection time is not included; thread default of the CLI is 4, shown in `--help` as `[4]`, so "default" = 4):

```bash
whisper-cli.exe -m models/ggml-$M.bin -f audio/$CLIP.wav -l $LANG -t $T -ml 1 -sow -ojf -pp -of out/x
```

Wall time is measured around the whole process (includes model load). Realtime factor = audio seconds / wall seconds. One run per cell, no repetition, so noise is real (see large-v3-turbo).

### Results (`results/speed.tsv`)

| Model | Clip | 4 threads (default) | 8 threads | 16 threads |
| --- | --- | --- | --- | --- |
| tiny | pt 90 s | 6.40 s (14.1x) | 5.24 s (17.2x) | 7.35 s (12.2x) |
| tiny | en 90 s | 6.26 s (14.4x) | 5.24 s (17.2x) | 5.74 s (15.7x) |
| base | pt 90 s | 10.09 s (8.9x) | 6.76 s (13.3x) | 8.55 s (10.5x) |
| base | en 90 s | 9.33 s (9.7x) | 6.57 s (13.7x) | 6.89 s (13.1x) |
| small | pt 90 s | 24.39 s (3.7x) | 19.73 s (4.6x) | 19.25 s (4.7x) |
| small | en 90 s | 26.14 s (3.4x) | 19.43 s (4.6x) | 19.50 s (4.6x) |
| large-v3-turbo | pt 90 s | 81.50 s (1.10x) | 55.32 s (1.63x) | 52.30 s (1.72x) |
| large-v3-turbo | en 90 s | 76.93 s (1.17x) | 75.86 s (1.19x) | 114.03 s (0.79x) |

300 s English clip, 8 threads: base 21.89 s (13.7x), small 66.52 s (4.5x), large-v3-turbo 327.44 s (0.92x).

Observations: 8 threads (physical cores) is the sweet spot; 16 (SMT) is not better and was much worse for turbo on the en clip (114 s, possibly noise from other load; not repeated). The large-v3-turbo numbers vary strongly between runs (pt 55 s vs en 76 s at 8 threads) and should be treated as +-30 %. `-dtw` (needs `-nfa`, flash attention off) is slower: small 35.7 s / 31.7 s, turbo 108.9 s / 78.9 s for 90 s (tiny 13.3/7.5 s, base 12.2/11.1 s).

### Expected wait (this CPU, 8 threads, linear extrapolation from the table)

| Model | 60 s video | 5 min video |
| --- | --- | --- |
| tiny | about 3.5 s | about 17 s |
| base | about 4.5 s | 22 s (measured) |
| small | about 13 s | 67 s (measured) |
| large-v3-turbo | about 37-50 s | 327 s (measured) |

**Verdict: confirmed** for tiny/base/small. large-v3-turbo is about realtime on this CPU, so it is a "quality" option, not a default, and would likely be slower on weaker machines.

## H3. Word-level timestamps

### Options available in 1.9.2 (from `--help`)

`-ml N/--max-len`, `-sow/--split-on-word`, `-ojf/--output-json-full`, `-dtw MODEL`, `-owts` (karaoke script), `--print-confidence`, VAD options (`--vad`, `-vm`; needs a separate VAD model, not tested).

Used: `-ml 1 -sow -ojf`. Every JSON `transcription` entry is then one word. DTW: `-dtw <preset> -nfa`. Without `-nfa` the CLI logs `dtw_token_timestamps is not supported with flash_attn - disabling` and `t_dtw` stays -1 for every token (flash attention is on by default in this version); my first DTW run was therefore a no-op and was redone.

### Reference and method

`ffmpeg -i clip.wav -af silencedetect=noise=-35dB:d=0.3 -f null -` gives `silence_end` times (speech onsets after a pause >= 0.3 s). For each onset I take the first word whose end is after the onset and compare its start with the onset. `analyze.py` does this. Points: 40 onsets for pt, 19 for en (18 comparable). The reference is energy based (-35 dB), so it is itself approximate (breaths, soft word starts); treat the absolute numbers as indicative.

### Results (`results/word-timing.jsonl`, 8 threads, `-ml 1 -sow`)

| Model | Clip | Points | Mean abs error | Median | Max | Signed mean | Within 200 ms |
| --- | --- | --- | --- | --- | --- | --- | --- |
| tiny | pt | 40 | 367 ms | 218 | 1212 | -367 | 20/40 |
| tiny | en | 18 | 261 ms | 158 | 739 | -250 | 10/18 |
| base | pt | 40 | 290 ms | 226 | 727 | -289 | 17/40 |
| base | en | 18 | 141 ms | 123 | 438 | -141 | 14/18 |
| small | pt | 40 | 302 ms | 222 | 1100 | -302 | 19/40 |
| small | en | 18 | 164 ms | 159 | 529 | -159 | 13/18 |
| large-v3-turbo | pt | 40 | 308 ms | 238 | 1237 | -308 | 16/40 |
| large-v3-turbo | en | 18 | 221 ms | 227 | 659 | -216 | 9/18 |

DTW (`-dtw <preset> -nfa`, start taken from the first token's `t_dtw`, centiseconds x 10) was **worse** on this metric: mean abs error tiny 471/316, base 308/409, small 391/299, large-v3-turbo 459/396 ms (pt/en), signed mean positive (late) for most cells. So DTW is not a free improvement here.

Key finding: the error is almost entirely **one-directional**: the signed mean equals minus the mean absolute error, i.e. the word after a pause starts *before* the detected speech onset. Cause visible in the output: word ends equal the next word's start (contiguous), so the silence between words is absorbed into the **previous word's tail / the next word's lead**. In a highlight caption the next word would light up early, by 140-370 ms on average after pauses (up to 1.2 s). Also seen: words of 1 s or longer (pt: 12-18 per 90 s clip, mostly words before pauses) and zero/near-zero-duration words (<= 10 ms: 1-3 in pt, 2-13 in en per clip, e.g. consecutive entries `"So" 7220-7220` and `"she" 7220-7220` in the turbo en output).

Not compared: timing inside continuous speech (no independent reference there).

### Structure questions

- Sub-word tokens: with `-ml 1 -sow` entries are whole words (e.g. `"crónicas"`), the sub-word pieces are only inside each entry's `tokens` array (pt: 87-99 of about 195 words have more than one token; en: 41-50 of 316). Without `-sow`, entries split at token level; I did not test `-ml 1` without `-sow`.
- Punctuation: attached to the word text (`"conversation?"`, `"book,"`). 0 punctuation-only entries in all 8 outputs. In `tokens` the punctuation is a separate token (`","`) with its own `p`.
- Confidence: no per-word field; each **token** has `p` (0..1). Word confidence must be derived (min or mean over tokens). Mean token `p`: tiny 0.75/0.88, base 0.83/0.90, small 0.88/0.92, turbo 0.96/0.96 (pt/en).
- Extra entries to filter: an empty-text entry for `[_BEG_]` at the start, occasional empty-text entries (pauses), and timestamp tokens such as `[_TT_308]` inside `tokens`.

### Real JSON excerpt (large-v3-turbo, en90, 8 threads; `timestamps` strings and token ids omitted)

```text
offset ms (from-to)  text           tokens (text:p)
2870-3090            " the"         " the":1.0
3090-3390            " use"         " use":1.0
3390-3510            " of"          " of":1.0
3510-3520            " a"           " a":1.0
3520-4000            " book,"       " book":1.0 ",":0.967
4000-4330            " thought"     " thought":0.865
4330-4660            " Alice,"      " Alice":0.999 ",":0.999
4660-5260            " without"     " without":0.746
5260-5350            " pictures"    " pictures":0.998
5350-5440            " or"          " or":0.996
5440-6160            " conversation?" " conversation":0.988 "?":0.987 "[_TT_308]":0.643
7220-7220            " So"          " So":1.0
7220-7220            " she"         " she":0.999
```

Raw shape of one entry (tiny pt90 output, first word):

```json
{"timestamps":{"from":"00:00:00,050","to":"00:00:00,330"},"offsets":{"from":50,"to":330},"text":" As",
 "tokens":[{"text":" As","timestamps":{"from":"00:00:00,050","to":"00:00:00,330"},"offsets":{"from":50,"to":330},"id":1018,"p":0.617893,"t_dtw":-1}]}
```

(pretty-printed multi-line in the real file.) Note the 7220-7220 pair above: two consecutive words with zero duration, which a highlighter must repair.

**Verdict: partially refuted / needs post-processing.** Word-level data exists and is usable, but raw `-ml 1 -sow` start times lead the audible onset by about 140-370 ms on average after pauses (within 200 ms for about half the points). DTW did not fix it in this test. Post-processing needed: snap word starts after pauses to a speech onset (silencedetect/VAD), enforce a minimum duration, clamp zero-length words, and compute word confidence from tokens.

## H4. Output contract

- JSON (`-oj` or `-ojf`; `-ojf` adds per-token data) top-level keys: `systeminfo` (string), `model` (object: `type`, `multilingual`, `vocab`, `audio`, `text`, `mels`, `ftype`), `params` (`model`, `language` as requested, `translate`), `result` (`{"language": "<detected or requested>"}`), `transcription` (array).
- Each `transcription` entry: `timestamps {from,to}` (strings `HH:MM:SS,mmm`), `offsets {from,to}` (**integer milliseconds**), `text` (leading space), and with `-ojf` `tokens[]` each with `text`, `timestamps`, `offsets` (ms), `id`, `p` (probability), `t_dtw` (centiseconds from DTW, -1 when DTW is off; checked against segment offsets: `t_dtw` 28 for a word at about 240 ms, so units are 10 ms).
- The file is written to `<-of>.json`; stdout still prints the human transcript lines (`[00:00:00.050 --> 00:00:00.330]   As`) even with `-np`, so a wrapper must redirect or ignore stdout.
- Progress: with `-pp` stderr gets lines `whisper_print_progress_callback: progress =  58%` (a handful of updates per 90 s clip, e.g. 32, 58, 88, 100 for tiny; 18, 35, 51, 70, 100 for turbo). Usable for a coarse progress bar (stderr parsing); granularity is per decoded window, not smooth. Without `-pp` there is no progress output.
- Exit codes (`results/contract.txt`): success 0. Missing input file: `error: input file not found '...'` then usage, **exit 2** (as observed). Missing model: `failed to initialize whisper context`, **exit 3**. **Corrupt audio (5000 random bytes named .wav): message `error: failed to read audio file`, but exit code 0**, and no JSON written; a truncated WAV (first 100,000 bytes of a valid file) and a 3000-byte zero file also exit 0 (the truncated one transcribes the partial audio). So the wrapper must verify that the output file exists and not rely on exit code for bad media; validate input with ffprobe/ffmpeg first (the product converts to 16 kHz mono WAV anyway).
- Language auto-detection (`-l auto`, `-dl` to only detect): the Portuguese clip was detected as `pt` by all four models, probabilities tiny 0.991, base 0.994, small 0.973, large-v3-turbo 0.999. The default language is `en`, so the app must pass `-l auto` or a language explicitly. A detect-only run (`-dl`) with tiny took 0.72 s in total (encode 0.46 s) on a 30 s clip.

**Verdict: confirmed**, with two gotchas: exit code 0 on unreadable audio, and stdout always carries the transcript.

## H5. Accuracy sanity (tiny sample)

First 40 reference words only; two clips; one reader per language; not a benchmark. Word errors counted by `wer40.py` (lowercase, accents and punctuation removed) and then classified by hand. The Portuguese reference uses 19th-century spelling (`chronicas`, `villa`, `brazil`, `alli`, `hespanhas`, `ltaguahy` with an OCR typo in the Gutenberg text), so about 6 of the raw "errors" are spelling-variant differences, not recognition errors.

| Model | pt raw errors /40 | pt after removing spelling variants | en raw errors /40 | en note |
| --- | --- | --- | --- | --- |
| tiny | 12 | about 6 (`um->em`, `doutor` inserted, `dr->se`, `simao->mao`, `bacamarte->macamarte`, `Tagua` for Itaguahy) | 4 | 3 are reference/reading differences (`what is` vs `what's`, `conversations` vs `conversation`); `thought->about` is a real miss |
| base | 11 | about 5 (`doutor` inserted, `dr->se`, `simao->mao`, `bacamarte->macamarte`, `Taguai`) | 3 | same 3 reference/reading differences, 0 real |
| small | 9 | about 4 (`vivera->viveram`, `estudara->estudaram`, `dr->doutor`, `Taguaí` for Itaguahy) | 3 | 0 real |
| large-v3-turbo | 7 | about 1-2 (`itaguaí` vs `Itaguahy` is a spelling variant; `dr->doutor` expansion) | 3 | 0 real |

First words of the transcripts (the pt text was copied from the JSON files; mojibake in some terminals is only a console encoding artefact, the JSON is valid UTF-8):

- tiny pt: "As crônicas da Vila de Taguai dizem que, em tempos remotos, viver ali, em certo médico, o doutor se mão macamarte, filho da nobreza da Terra, e o maior dos médicos do Brasil, de Portugal e das Espanhas. Estudara em" (misses the name Simão Bacamarte, wrong verb form `viver`).
- base pt: "As crônicas da Vila de Taguai dizem que em tempos remotos viver ali um certo médico, o doutor se mão macamarte, filho da nobreza da Terra e o maior dos médicos do Brasil de Portugal e das Espanhas. Estudara em" (same errors on the name and `viver`).
- small pt: "As crônicas da vila de Taguaí dizem que, em tempos remotos, viveram ali um certo médico, o doutor Simão Bacamarte, filho da nobreza da terra e o maior dos médicos do Brasil, de Portugal e das Espanhas. Estudaram em Coimbra" (name correct; `viveram`/`Estudaram` plural instead of the reference `vivera`/`Estudara`).
- large-v3-turbo pt: "as crônicas da vila de itaguaí dizem que em tempos remotos vivera ali um certo médico o doutor simão bacamarte filho da nobreza da terra e o maior dos médicos do brasil de portugal e das espanhas estudara em coimbra" (all names and verb forms right, but this run came out with **no punctuation and no capitalisation**, which matters for caption styling; the en output of the same model has punctuation).
- en (all models): "But it had no pictures or conversations in it. And what's the use of a book about Alice without pictures or conversation? So she was considering in her own mind..." (tiny/base/small); the reader's actual text differs slightly from the Gutenberg text, so these differences are not model errors.

Observations: proper nouns (Itaguahy, Bacamarte) are the main failure for small models; English was clean from base upward. The Portuguese reader's accent was not confirmed to be pt-BR; all models still detect `pt`.

**Verdict: confirmed as sanity only (inconclusive as a quality ranking).** Direction is as expected: more errors on pt than en, fewer with larger models.

## Conclusion and architecture impact

| Hypothesis | Verdict |
| --- | --- |
| H1 prebuilt sidecar | Confirmed (pin tag `v1.9.2`; latest stable `v1.9.5` has no assets) |
| H2 CPU speed | Confirmed for tiny/base/small; large-v3-turbo about 0.9-1.7x realtime (not a default) |
| H3 word timings | Partially refuted: data is there, but starts after pauses lead onsets by 141-367 ms mean; post-processing required |
| H4 output contract | Confirmed, with the exit-code 0 on corrupt audio gotcha |
| H5 accuracy | Confirmed as a tiny-sample sanity check; inconclusive for ranking |

What changes in the design:

- Default model: **`small`** (487 MB): about 4.6x realtime on 8 threads (60 s video about 13 s, 5 min video about 67 s measured), clearly better pt than tiny/base in this sample. Offer `base` (148 MB, 13x realtime) as the fast/low-resource option and `large-v3-turbo` (1.6 GB) as an opt-in quality option with an explicit warning (about 5.5 min for a 5 min video here). Because speeds on weaker CPUs are unknown, the app should measure or show an estimate.
- Use `-t` equal to the physical core count, not the logical count.
- Invocation: `-l auto` (or user language), `-ml 1 -sow -ojf -pp -of <path>`; read `offsets` in ms from the JSON file; parse `progress = N%` from stderr; do not depend on stdout.
- Post-process word timings: drop empty/`[_...]` entries, derive word confidence from token `p`, enforce minimum duration, repair zero-length words, and snap word starts that follow pauses (using silencedetect or whisper's VAD) so that highlights do not lead the audio by a few hundred ms. Do not enable `-dtw` by default.
- Validate input media before calling the CLI (exit code 0 on unreadable audio) and check that the JSON was produced.
- Distribution: pin the whisper.cpp release asset by tag and checksum (do not follow "latest"); models are downloaded on first use from Hugging Face (MIT) and cached.

## Limitations

- One machine (8-core desktop CPU), one run per cell, no repetition; large-v3-turbo timings are noisy (the 16-thread en run was 114 s vs 76 s at 8 threads).
- Two clips, one reader per language, 40-word accuracy samples; the pt reference uses old spelling; the en reader departs slightly from the Gutenberg text.
- Word-onset reference is energy based (`silencedetect`, -35 dB, 0.3 s), only after pauses (40 pt, 18 en points); no check of timings inside continuous speech and no human listening.
- Not tested: whisper's built-in VAD (`--vad`), BLAS and CUDA builds, quantised models, `v1.9.5` built from source, the `b5454` pre-release binaries, noisy or music-backed audio, videos longer than 5 min, multiple speakers, memory use.
- `run.sh` setup needs `gh`, `curl`, `unzip`, `ffmpeg`, `python`, and a few GB of downloads; it uses `set -u` only because the per-run exit code must be recorded instead of aborting.

## Reproduce

```bash
./run.sh all          # setup, env, speed, timing (incl. DTW), accuracy, contract
MODELS="tiny base" ./run.sh speed   # subset
```

Files: `run.sh`, `analyze.py` (timing vs silence onsets), `wer40.py` (40-word error count), `results/` (raw numbers: `speed.tsv`, `word-timing.jsonl`, `accuracy.jsonl`, `contract.txt`, `environment.txt`).
