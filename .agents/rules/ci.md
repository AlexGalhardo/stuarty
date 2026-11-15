# CI

1. A Windows machine does not show what the Linux and macOS runners show: build tags, file-system case, `unix.Statfs_t` field types, Bun versus Node behaviour. Run the cross-target lint of `skills/ci-preflight/` before every push.
2. The Go suite is paused on `ubuntu-latest` and `windows-latest` until the BETA phase, by the author's decision on 2026-11-15. The jobs still run so the required checks of `protect-main` report; macOS runs the suite. Do not remove the jobs: a required check that never reports blocks every merge.
3. Timing asserts skip under `-race` (`raceEnabled` constants in `*_race_test.go` and `*_norace_test.go`). A test that needs more time under the race detector gets more time there, not a looser limit everywhere.
4. A flaky test is a defect. Find the cause (CPU count, SIMD, load, ordering); never rerun until green and call it fixed.
5. A failing check whose cause is a real bug of the product is fixed in the product, not in the test. `TestSecurity_T12` on macOS was a `filepath.Rel` case bug, not a runner quirk.
