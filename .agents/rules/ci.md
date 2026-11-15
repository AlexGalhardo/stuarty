# CI

1. A Windows machine does not show what the Linux and macOS runners show: build tags, file-system case, `unix.Statfs_t` field types, Bun versus Node behaviour. Run the cross-target lint of `skills/ci-preflight/` before every push.
2. Until the end of BETA, by the author's decision on 2026-11-15, the pull-request workflows are disabled (`gh workflow disable`): `ci`, `codeql`, `desktop-detect`, `docs-last-updated`, `image`, `pr-title`, `secrets` and `installers`. The full local gate of `skills/ci-preflight/` replaces them, and release pull requests merge with `gh pr merge N --rebase --admin`. `installers.yml` is also called by `release.yml` through `workflow_call`: check after each release that its jobs ran. Re-enable everything with `gh workflow enable` at the BETA exit; do not delete the workflows.
3. Timing asserts skip under `-race` (`raceEnabled` constants in `*_race_test.go` and `*_norace_test.go`). A test that needs more time under the race detector gets more time there, not a looser limit everywhere.
4. A flaky test is a defect. Find the cause (CPU count, SIMD, load, ordering); never rerun until green and call it fixed.
5. A failing check whose cause is a real bug of the product is fixed in the product, not in the test. `TestSecurity_T12` on macOS was a `filepath.Rel` case bug, not a runner quirk.
