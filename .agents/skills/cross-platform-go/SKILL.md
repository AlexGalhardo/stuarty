---
name: cross-platform-go
description: Write Go code and tests that behave the same on Linux, macOS and Windows, covering paths and case, file-system calls, processes, executables and timing.
---

# Cross-platform Go

The development machine is Windows. CI runs Linux and macOS too. These are the differences that broke a build before.

## Paths

- `filepath.Rel` compares case-sensitively on macOS, even on a case-insensitive volume. If a match has already been made component by component with case folding (`pathguard.within`), build the rest of the path from those components; do not call `Rel`.
- A test that relies on a case-insensitive volume probes the volume first (`os.Stat` of a case variant) and skips otherwise. A macOS volume can be either.
- `filepath.EvalSymlinks` resolves `/var` to `/private/var` on macOS. Compare resolved paths with resolved paths.

## File-system and system calls

- Field types of `unix.Statfs_t` differ: `Bsize` is `int64` on Linux and `uint32` on macOS. A conversion that one linter calls unnecessary and another calls unsafe belongs in per-OS files (`disk_linux.go`, `disk_other.go`), not behind a `//nolint` that the other target reports as unused.
- `/proc` exists only on Linux. A process probe that cannot read `/proc/<pid>/stat` must treat that as "gone", not as an error.

## Executables

- On Windows a tool is `ffmpeg.exe`. A test that builds a tool path from a directory adds the suffix by `runtime.GOOS` (`exeName`).
- CPU affinity on Windows is inherited by child processes and changes `runtime.NumCPU`, which makes it a good way to reproduce a smaller CI runner.

## Timing

- Timing asserts skip under `-race`, using the `raceEnabled` constants in `race_test.go` and `norace_test.go`.
- Latency budgets are measured on an idle machine. When several builds run at once, a p95 assert can fail without a regression. Re-run alone before acting.
