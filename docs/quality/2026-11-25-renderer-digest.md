# The renderer module digest, Linux and Windows (epic 93)

Date: 2026-11-25. Toolchain: Rust 1.100.0 (`rust-toolchain.toml`). Source: `renderer/` as of the integration branch `release/v0.56.0` (the last change of `renderer/` is `9d6ff1d5d`; the commits after it only add a test).

## Before

| Module | Built by | SHA-256 |
| --- | --- | --- |
| embedded (`engine/renderhost/renderer.wasm`) | the Windows machine of the author, `build-renderer.sh embed` | `b1d9979ccf1a383d0463f89e50c1efaf9a02a991024cfa5f425f3634ae0e962e` |
| `bash scripts/build-renderer.sh wasm` on Windows 11, this source | Windows | `b1d9979ccf1a383d0463f89e50c1efaf9a02a991024cfa5f425f3634ae0e962e` |
| artifact `renderer-wasm-linux` of CI run 45560729813 (commit `1995b2364`, whose `renderer/` source is the same) | `ubuntu-24.04` | `5732776064820df31a3396625b259b76c370d553688aa977fd133049fa0c1b44` |

So the embedded module was reproducible on Windows and the `Renderer` job of CI failed its `check` step on every run: the two systems built different modules from one source and one toolchain.

## The report of `wasmdiff`

`go run ./internal/tools/wasmdiff <windows .wasm> <linux .wasm>`:

```text
section                   A size    B size  payload
type                         616       616  equal
function                    1654      1654  DIFFERS
table                          7         7  equal
memory                         6         6  equal
global                         9         9  equal
export                       370       370  equal
element                      657       657  DIFFERS
code                     1333632   1333631  DIFFERS
data                     2568313   2568313  DIFFERS
custom:name               187986    187952  DIFFERS
custom:producers              78        78  equal
custom:target_features       148       148  equal

first difference at byte 840, in the function section
function bodies: A has 1652, B has 1652; 353 differ by index; first: function body 37
data segments: 1 of 2 differs; around the first difference the readable strings are the same and
the binary bytes between them are not
```

The types, the exports and the memory are equal: the module has the same interface and the same functions, laid out in another order, with other bodies where a body calls another by index.

## The cause

Every crate that cargo builds (the registry crates and the two crates of the workspace) has another disambiguator in its symbols on the two systems, and the crates of the sysroot (`core`, `alloc`, `std`, prebuilt) have the same one. In the `name` section, `read_fonts` is `Cs…_9read_fonts` with `scI76FWPyCJv` on Windows and `s5RJMkdUZoO6` on Linux, `harfrust` has `sbRJVNVpNM3Q` and `scaY7sBxvMv0`, `alloc` has `sd7qzcshS4qx` on both.

That disambiguator comes from the `-C metadata` that cargo passes, and cargo hashes into it the text of `rustc -vV`, which holds `host: x86_64-pc-windows-msvc` on this machine and `host: x86_64-unknown-linux-gnu` on the runner. The linker orders functions and panic locations by their symbols, so a different hash reorders them and changes the bodies that refer to each other by index. The paths were already equal: the readable text of the data segment is the same on both, so the remaps of `build-renderer.sh` work.

Confirmed by an experiment, not assumed: a `RUSTC_WRAPPER` that prints the Linux host in that one line of `rustc -vV`, run on Windows, gave `5732776064820df3…0fa0c1b44`, the digest of the Linux artifact, byte for byte (`wasmdiff` exits 0 on the two files).

## The change

`scripts/build-renderer.sh` builds the module, on a host other than x86-64 Linux, with `internal/tools/rustchost` as the `RUSTC_WRAPPER` (a small program of the standard library, built with `go build` into the target folder): it changes the `host:` line of `rustc -vV` to the host of the runner and passes every other call through unchanged, so the proc macros and the build scripts still run for the real host. On x86-64 Linux nothing changes. The script also deletes the cached `rustc -vV` of cargo (`target/.rustc_info.json`) and keeps the first build of `verify` in `renderer/target/verify-first/`, which the CI job uploads when the two builds differ.

Rule 9 of `.agents/rules/rust.md` and section 4.1 of SDD-013 say it.

## After

| Module | SHA-256 |
| --- | --- |
| `bash scripts/build-renderer.sh wasm` on Windows 11 | `5732776064820df31a3396625b259b76c370d553688aa977fd133049fa0c1b44` |
| `bash scripts/build-renderer.sh verify` on Windows 11, two clean builds | `5732776064820df31a3396625b259b76c370d553688aa977fd133049fa0c1b44` both |
| artifact `renderer-wasm-linux` of run 45560729813 | `5732776064820df31a3396625b259b76c370d553688aa977fd133049fa0c1b44` |
| embedded, `engine/renderhost/renderer.wasm.sha256` | `5732776064820df31a3396625b259b76c370d553688aa977fd133049fa0c1b44` |

`go test ./engine/renderhost/... ./engine/export/...` pass with the new module and no golden file changed: the two modules draw the same pixels.

## CI

- Run 45560729813 produced the Linux artifact above (its `Renderer` job failed the `check` step because the embedded module was the Windows one).
- The runs of the branch of this epic, with the digest in the job summary and the job `Renderer (windows-latest)`, are made by the orchestrator, who pushes; their links are added here when they exist.
