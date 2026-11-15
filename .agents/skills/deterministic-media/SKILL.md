---
name: deterministic-media
description: Keep FFmpeg output and composed frames byte-identical across machines, CPU counts and SIMD levels, and diagnose a golden that differs only on CI.
---

# Deterministic media

NFR-010: the composed frames of the reference scenes are the same bytes on every run and every operating system. FFmpeg makes that hard in three ways.

## Rules

1. **Filter threads.** swscale and zscale cut a frame into slices by thread count. Every graph whose output is compared runs on one filter thread.
   - A `-vf` chain takes `-filter_threads 1`.
   - A `-filter_complex` graph takes `-filter_complex_threads 1`. `-filter_threads` does not reach a complex graph. `ffargs.Cmd.FilterThreads` picks the right option.
2. **Encoder threads.** `mpeg4` encodes one slice per thread, so the normalized file uses `-threads 1` (`ingest.NormalizedThreads`). A fixed count above 1 is refused on small machines by the thread budget (FF-16).
3. **SIMD.** The automatic DCT of `mpeg4` picks a SIMD version by CPU features that is not bit-exact. Use `-dct int`.
   - The decoder is already bit-exact.
   - The export scaler with `accurate_rnd+full_chroma_int` is bit-exact too.
   - A plain `bicubic` scale is not, which matters only for the proxy, and the proxy is never compared.
4. **Test fixtures** (`engine/ingest/mediagen`) follow the same rules: `-filter_threads 1`, `-threads 1` and `-dct int`.

## How to diagnose a golden that differs only on CI

1. Reproduce the thread count of the runner. On Windows, run the test with a CPU affinity mask, for example `Start-Process go ... -PassThru` followed by `$p.ProcessorAffinity = 0xF`. Go then sees 4 CPUs in `runtime.NumCPU`; `GOMAXPROCS` does not change `NumCPU`.
2. Reproduce the SIMD level. Run the FFmpeg command by hand with `-cpuflags 0` (C only) and with `-cpuflags -avx512`, then compare `md5`. A stage whose output changes is not bit-exact.
3. Bisect the pipeline. Hash each artifact (normalized, proxy, thumbnails) and decode with `-f md5 -` to find the first stage that differs.
4. Only then regenerate the goldens, with `go test ./engine/export -run Golden -update`. Every regeneration is a reviewed change with its reason in the commit body.

## Time limits under the race detector

The Go compositor slows about tenfold under `-race`. Export deadlines are stretched only in tests with the race tag (`engine/export/race_test.go`), never in the product.
