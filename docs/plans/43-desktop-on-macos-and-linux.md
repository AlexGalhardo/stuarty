# 43: The desktop on macOS and Linux

## Goal

A creator on a Mac or on Linux installs Stuarty and gets the same application Windows users have: it opens, plays, exports, keeps their keys in the system keychain and publishes on schedule.

## Context and related decisions

- ADR-0041 (the Wails shell over the one web UI), ADR-0023, ADR-0033 (the keychain per system), ADR-0046 (the background service), ADR-0003 (determinism on three systems).
- Author decision D-074 (Windows and Docker first; macOS and Linux are their own epic). PRD-003 RF-213, NFR-045.
- `desktop/checklist.md`, `desktop/detect/`, `desktop/results/`, `scripts/desktop-verify.sh`.
- Absorbs the earlier plan on the desktop on real machines and the update feed.

## Scope

- Modules: `desktop/shell/`, `desktop/packaging/macos/` and `linux/`, `desktop/update/`, `desktop/service/` (start at sign-in and tray on each system), `service/vault/` (the keychain provider on each system), the webview detection page, the golden frame and audio tests on each system, `desktop/results/`.

## Out of scope

- Paid code signing and notarisation: a decision of the author about money (STOP).
- Distribution through stores or package repositories.
- Windows (already covered by each epic).

## Dependencies

**Blocked until a real macOS machine and a real Linux desktop machine are available to the session** (no virtualised stand-in for the webview and keychain checks). Technically after 04, 21 and 26.

## Technical design

- **Per system, the checklist of `desktop/checklist.md`**, extended with the Stuarty sections: install from the package, first start, the webview detection page (which features the system webview has: WebCodecs, WebGPU or WebGL2, `OffscreenCanvas`), the first flow of epic 38 in the shell, the keychain (store, read, survive a restart, behave when locked), tray and close-to-background, start at sign-in, single instance, update from the feed, uninstall leaving nothing behind.
- **Webview differences**: where the system webview lacks a feature the preview needs, the fallback executor of epic 26 is used and the result is recorded; a missing feature is never patched around by bundling another browser engine (that would be an ADR-level decision).
- **Linux keychain**: the Secret Service through the session bus; when absent (a minimal desktop), the documented fallback of ADR-0033 is the key file, and the UI says which one is in use.
- **Determinism**: the golden frame tests (CPU path: exact) and the audio goldens (within tolerance) run on each system; a difference is a defect.
- **Packaging**: the existing packages (pkg, AppImage) are rebuilt under the new name; the update feed is exercised from the previous build to the new one.
- **Evidence**: one results file per system and date in `desktop/results/`, with the checklist, screenshots and the commands' outputs.

## Tasks

- [ ] 1. Extend `desktop/checklist.md` with the Stuarty sections. Verify: `task lint:md` exits 0.
- [ ] 2. macOS: build, package, install; run the checklist. Verify: `bash scripts/desktop-verify.sh` passes on macOS and the results file is committed.
- [ ] 3. macOS: run the Go tests, the golden frames and the audio goldens. Verify: `task test` and the determinism tests pass on macOS.
- [ ] 4. Linux: the same as task 2. Verify: `bash scripts/desktop-verify.sh` passes on Linux and the results file is committed.
- [ ] 5. Linux: the same as task 3, plus the keychain with and without the Secret Service. Verify: `go test ./service/vault/...` passes in both conditions.
- [ ] 6. Fix what the two systems revealed, each fix with a test that runs on that system. Verify: both checklists pass again.
- [ ] 7. Exercise the update feed on both systems. Verify: the update from the previous build succeeds and is recorded.

## Test strategy

- The desktop checklist on real hardware, recorded.
- The Go suite and the determinism goldens on each system.
- The first flow in the shell on each system.

## Acceptance criteria

1. The packaged application installs, starts and completes the first flow on a real macOS machine and on a real Linux desktop.
2. Golden frames match exactly and audio goldens match within tolerance on both.
3. Credentials are stored in each system's keychain, or in the documented fallback, stated in the UI.
4. Close-to-background, tray, single instance and start at sign-in work on both, or each gap is recorded with its reason.
5. `desktop/results/` holds a dated results file per system.

## Risks and mitigation

- **A system webview without the features the preview needs.** Mitigation: the fallback executor; the gap recorded for epic 47.
- **Unsigned applications are blocked by the system.** Mitigation: document the user's steps; signing is the author's decision. **STOP** for anything paid.

## Size

M

## Hand-off for review

The diff of `epic/43-desktop-on-macos-and-linux`; the two results files; the list of gaps per system.
