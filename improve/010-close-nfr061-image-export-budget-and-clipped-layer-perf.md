# Plan 010: Close NFR-061 image export budget (<2 s) and fix clipped-layer band/memory bottlenecks

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `improve/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a62856c36..HEAD -- engine/export/image.go engine/export/image_bands.go engine/export/image_window.go engine/renderhost/host.go engine/renderhost/prepared.go renderer/stuarty-renderer/src/compose.rs renderer/stuarty-renderer/src/host.rs renderer/stuarty-renderer/src/ffi.rs`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: perf
- **Planned at**: commit `a62856c36`, 2026-11-25

## Why this matters

NFR-061 (`s_export <= 2.00 s` on the 4000×4000 30-layer reference image document) is currently the ONLY failing numeric NFR in Stuarty (`docs/plans/00-roadmap.md:72`: measured at 2.50 s vs 2.00 s limit). Three concrete algorithmic/bandwidth wastes explain the gap, plus a severe clipped-layer cliff:

1. **`drawBand` uploads full uncropped layers per band (`engine/export/image_bands.go:676-687`)**: unlike `drawWindowBand` (`image_window.go:353-358`), which already crops each prepared picture to the band using `windowCrop` + `d.pic.Window(...)`, `drawBand` uploads the entire prepared picture to every band it touches and over-counts memory in `bandsHold`.
2. **Duplicate source PNG decodes per export (`engine/export/image.go:347-370`)**: `imageTexture` calls `loadImage(path)` per layer with no per-run decoded cache, decoding the same 2000×2000 PNG 10 times on the reference document (`image_reference_test.go:63-71`).
3. **Double-copy WASM texture uploads (`engine/renderhost/host.go`, `renderer/stuarty-renderer/src/host.rs:130-135`)**: `PutTexture`/`PutPrepared` allocates via `vr_alloc`, writes Go→WASM, then Rust copies again with `pix.to_vec()` before Go frees the staging buffer.
4. **Clipped layers collapse parallelism to 1 core and allocate 128 MiB per layer (`engine/export/image_bands.go:501-507`, `renderer/stuarty-renderer/src/compose.rs:276-344`)**: `prepare_clipped` allocates two full-canvas `width * height * 4` buffers (128 MiB at 4000×4000) and loops over all 16M pixels ignoring `rows`, while `drawBands` forces `bands = 1` whenever ANY layer has a clip/stencil.

## Current state

- `engine/export/image_bands.go:501-507, 613-629, 676-687`:

```go
	clipped := slices.ContainsFunc(drawn, func(d drawLayer) bool { return d.layer.Clip != nil || d.layer.Stencil })
	if clipped {
		bands = 1
	}
```

```go
	for i := range drawn {
		d := &drawn[i]
		if !keepAll && (d.y1 <= y0 || d.y0 >= y1) {
			continue
		}

		if err := host.PutPrepared(ctx, d.layer.ItemID, d.pic); err != nil {
			return rendererError(err)
		}

		kept = append(kept, d.layer)
	}
```

  Compare `engine/export/image_window.go:353-358`, which already has `windowCrop(d, ...)` and `d.pic.Window(...)` proven bit-exact in `engine/export/image_window_test.go`.

- `renderer/stuarty-renderer/src/compose.rs:296-343`: `prepare_clipped` allocates `Image::new(width, height)` (line 296) and `vec![0_u8; width * height * 4]` (line 313) and loops over every pixel of the entire canvas instead of only `rows`.
- Rules that apply (`.agents/rules/rust.md`, `.agents/skills/renderer-parity/SKILL.md`):
  - Read `.agents/skills/renderer-parity/SKILL.md` before modifying `renderer/` or `engine/export/`.
  - CPU path MUST stay bit-exact (`NFR-062`), integer/fixed-point only, zero `unsafe` outside reviewed FFI boundaries.
  - Any change to `renderer/` requires rebuilding the reproducible WASM module via `bash scripts/build-renderer.sh` and checking `TestImageReferenceDocument` determinism and parity tests.

## Commands you will need

| Purpose | Command | Expected on success |
| --- | --- | --- |
| Image export & parity tests | `go test -count=1 -run 'Image' ./engine/export/...` | `ok`, exit 0 |
| Reference image benchmark (`s_export`) | `go test -run '^$' -bench BenchmarkImageReference -benchtime=3x -count=1 ./engine/export/` | reports `s_export <= 2.00 s` on idle machine |
| Renderhost tests | `go test -race -count=1 ./engine/renderhost/...` | `ok`, exit 0 |
| Rust renderer gate | `cd renderer && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test && cargo deny check` | `ok`, exit 0 |
| Rebuild reproducible WASM | `bash scripts/build-renderer.sh` | builds cleanly with deterministic SHA-256 |
| Go lint & vet | `task lint:go && task vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):

- `engine/export/image.go`
- `engine/export/image_bands.go`
- `engine/export/image_bands_test.go` (or `image_window_test.go`)
- `engine/renderhost/host.go`
- `engine/renderhost/prepared.go`
- `renderer/stuarty-renderer/src/compose.rs`
- `renderer/stuarty-renderer/src/host.rs`
- `renderer/stuarty-renderer/src/ffi.rs`
- Embedded WASM artifact & pinned digest files updated by `bash scripts/build-renderer.sh` (only when Steps 3–4 modify Rust!)

**Out of scope** (do NOT touch):

- Video timeline export pipeline (`engine/export/job.go`, FFmpeg encoding).
- GPU shader path (`renderer/stuarty-renderer-gpu/`) except verifying CPU-vs-GPU parity still passes.
- `web/resources/js/` frontend code.

## Git workflow

- Branch: `perf/image-export-nfr061-and-clips` cut from `release/v0.60.0`.
- Commit Go-only optimizations first (Steps 1 & 2, which require NO WASM rebuild), then Rust + WASM optimizations (Steps 3 & 4) in separate commits:
  1. `perf(export): crop prepared layers per band in drawBand and cache decoded source images per run`
  2. `perf(renderer): bound prepare_clipped to active band rows and support multi-band clipped exports`
- Do NOT push or merge to `main`.

## Steps

### Step 1: Cache decoded source images per `imageRun` (`engine/export/image.go`)

1. Read `engine/export/image.go:330-390` (`imageTexture`, `loadImage`) and `imageRun` struct definition.
2. Add a per-`imageRun` concurrency-safe decoded-image cache (mutex + per-key single-flight wait channel or `sync.Once` entry keyed by canonical file path / `AssetID`) storing the decoded immutable `*image.RGBA` (or `image.Image`) for the duration of a single export run (released when `imageRun` returns).
3. Verify callers never mutate the cached decoded image in place (if any caller mutates pixels, return a copy or ensure `imageTexture` treats decoded images as read-only).
4. Add a unit test in `engine/export/` verifying a multi-layer document sharing one asset decodes the PNG once and produces byte-identical output.

**Verify**: `go test -race -count=1 -run 'Image' ./engine/export/...` -> `PASS`, exit 0.

### Step 2: Crop prepared layers per band in `drawBand` (`engine/export/image_bands.go`)

1. Read `engine/export/image_window.go:340-385` (`drawWindowBand` and `windowCrop`) and `engine/export/image_bands.go:612-695` (`bandsHold`, `drawBand`).
2. In `drawBand` (`image_bands.go:676-687`), when `!keepAll`:
   - Compute the band crop via `windowCrop(d, 0, y0, fp.Width, y1)` and obtain the cropped prepared picture via `d.pic.Window(...)` (handling empty intersection cleanly by continuing, matching `drawWindowBand`).
   - Upload the cropped prepared picture via `host.PutPrepared(ctx, layer.ItemID, pic)` and append the adjusted `layer` to `kept`.
3. Update `bandsHold` (`image_bands.go:613-629`) to sum the cropped window byte size per band rather than full `d.pic.Bytes()`, allowing `bandCount` to utilize full available parallelism within `imageWorkMemory`.
4. Measure `BenchmarkImageReference` on an idle machine and verify byte-identical output against `TestImageReferenceDocument`!

**Verify**: `go test -count=1 -run 'TestImageReference|TestImageBand|TestImageWindow' ./engine/export/...` -> `PASS`, exit 0, byte-identical output.

### Step 3: Bound `prepare_clipped` to `rows` in `renderer/stuarty-renderer/src/compose.rs` and enable multi-band clipped exports

1. Read `renderer/stuarty-renderer/src/compose.rs:275-370` and every caller of `prepare_clipped` in `compose.rs`.
2. Notice `Target` already accepts `rows: Range<usize>` and `prepare_clipped` only draws rows inside `rows.clone()` — yet it allocates full-canvas `width * height * 4` buffers and loops over `0..height`!
3. Refactor `prepare_clipped` to allocate buffers sized ONLY to `(width, band_h)` where `band_h = rows.end - rows.start` (offsetting `Target` or slicing only the active row range `rows.start * width * 4 .. rows.end * width * 4` — check `Target` indexing in `compose.rs` first to ensure coordinate origins match!), and return a `Prepared` whose `Placement.cy` is centered on `rows.start + band_h / 2.0` (so the composed band pixels are 100% bit-exact!).
4. In `engine/export/image_bands.go:501-520`:
   - Instead of forcing `bands = 1` when `clipped` is true, keep multi-band `bands` and for each band `[y0, y1)` include every layer touching `[y0, y1)` PLUS any stencil/clip source layer referenced by a clipped layer included in that band (preserving inverted-clip behavior even when the source layer is off-band or off-canvas!).
5. Add a parity test in `engine/export/` comparing `bands = 1` vs `bands = 4` on a multi-layer document with normal clips, inverted clips, and off-band stencil sources, asserting 100% byte-identical RGBA output.

**Verify**: `cd renderer && cargo test && cargo clippy --all-targets -- -D warnings` -> `PASS`, exit 0.

### Step 4: Eliminate double-copy staging on WASM uploads (optional if Step 1+2 already meet `<2.00 s` with headroom)

1. Read `renderer/stuarty-renderer/src/ffi.rs` and `host.rs:115-160` (`vr_alloc`, `vr_put_texture`, `vr_put_prepared`) and `engine/renderhost/host.go:255-320`.
2. If `BenchmarkImageReference` still needs extra headroom after Steps 1–3, add owning WASM entrypoints (`vr_take_texture` / `vr_take_prepared`) that reconstruct the `Vec<u8>` directly from the `vr_alloc` pointer/capacity (transferring ownership into `Host` so Go does NOT call `vr_free` on success, avoiding `pix.to_vec()`). Follow `.agents/rules/rust.md` item 4 (`// SAFETY:` comments required on every `unsafe` FFI boundary block).
3. Rebuild the reproducible WASM module with `bash scripts/build-renderer.sh` and verify all parity + determinism tests pass.

**Verify**: `bash scripts/build-renderer.sh && go test -count=1 ./engine/renderhost/... ./engine/export/... -run 'Image|Parity|Determinism'` -> all pass, exit 0.

## Test plan

- `engine/export/`:
  - Multi-layer shared-asset decode count test + byte-identical output check.
  - `bands = 1` vs `bands = 4` bit-exact comparison test for normal layers, rotated/blurred layers, clipped layers, inverted clipped layers, and off-canvas stencil sources.
- `renderer/stuarty-renderer`: unit tests for `prepare_clipped` on sub-canvas `rows` ranges asserting bit-exact equivalence with full-canvas `0..height`.
- Idle-machine benchmark: `go test -run '^$' -bench BenchmarkImageReference -benchtime=3x -count=1 ./engine/export/`.

## Done criteria

- [ ] `go test -count=1 -run 'Image' ./engine/export/...` exits 0 with zero golden/parity diffs
- [ ] `go test -race -count=1 ./engine/renderhost/...` exits 0
- [ ] `cd renderer && cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test && cargo deny check` exits 0
- [ ] Multi-band (`bands=4`) vs single-band (`bands=1`) test passes bit-identically on clipped and inverted-clip documents
- [ ] `BenchmarkImageReference` idle measurement recorded in commit/report (target `s_export <= 2.00 s`)
- [ ] `task lint:go && task vet` exits 0
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `improve/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any single pixel differs between `bands = 1` and `bands = 4` (or between pre-change and post-change `TestImageReferenceDocument`) — NFR-062 requires bit-exact output!
- `cargo` or `scripts/build-renderer.sh` is unavailable on the executor host — complete Steps 1 and 2 (pure Go, zero WASM rebuild needed!) and stop before Step 3, reporting the Go-only `s_export` speedup.
- `vr_alloc` in `ffi.rs` does not use standard `Vec::from_raw_parts`-compatible allocation layout (if so, skip Step 4 rather than introducing unsafe UB).

## Maintenance notes

- Always measure `BenchmarkImageReference` on an idle machine with no parallel subagents running (`AGENTS.md` §9).
- Reviewer focus: verify inverted clips (`invert == true`) where the stencil source does NOT intersect band `[y0, y1)` still receive the source layer in `drawBand` so the full inverted mask draws identically to `bands = 1`.
