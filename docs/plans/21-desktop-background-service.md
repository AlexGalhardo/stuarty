# 21: Desktop background service for scheduled posts

## Goal

A creator schedules a post for tomorrow morning, closes the Stuarty window, and the post still goes out on time: as long as the computer is on. A tray icon shows that Stuarty is still there and lets them stop it.

## Context and related decisions

- ADR-0046 (the scheduler; the desktop keeps a resident service), ADR-0041, ADR-0023 (the desktop shell).
- Author decision D-078. PRD-003 RF-219, RF-213; PRD-004 RF-406.
- Research note `web/07-stack-desktop-web-self-hosted` (what the shell framework offers for tray and single instance).

## Scope

- Modules: `desktop/shell/` (window lifecycle, tray, single instance, start at sign-in), `desktop/service/` (new: the resident process mode), `desktop/packaging/` (the Windows installer option), `web/resources/js/pages/Settings/` (the background settings), SDD-020 (the missed-time behaviour on a desktop), `docs/sdds/012-ui-architecture.md` (the shell section).
- Windows only is verified here (author decision D-074). The code paths for macOS and Linux are written behind build tags where the framework makes them free, and are verified by epic 43.

## Out of scope

- A system service that runs with no user signed in: the keychain that holds the master key belongs to the user session.
- Waking the computer from sleep.
- Server deployments: there the server process is always running.

## Dependencies

20.

## Technical design

- **One process, two faces**: the desktop application already runs the server in-process. Closing the window hides it instead of exiting when "keep running in the background" is on and at least one target is scheduled; the tray menu offers Open, Pause publishing, and Quit.
- **Default off, asked once**: the first time a person schedules a target on the desktop, a dialog explains the choice and records the answer. Nothing resident is installed silently.
- **Start at sign-in**: optional, registered per user with the mechanism the shell framework provides (no administrator rights), started hidden; removed when the option is turned off and by the uninstaller.
- **Single instance**: a second launch focuses the running instance instead of starting a second scheduler; the lease of epic 20 remains the real protection.
- **Honesty about limits**: the settings page and the scheduling dialog say that a post goes out only if the computer is on and the user is signed in, and that a missed post waits for a decision (epic 20).
- **Quit means quit**: Quit from the tray stops the scheduler and tells the user how many scheduled targets will wait.
- **No new outbound**: the resident mode opens nothing that the scheduler would not open.

## Tasks

- [x] 1. Amend SDD-012 and SDD-020 with the lifecycle, the options and the limits. Verify: `task lint:md` exits 0.
- [x] 2. Implement hide-to-tray, the tray menu and Quit with its warning. Verify: `go test ./desktop/shell/...` passes for the lifecycle logic (extracted from the framework calls).
- [x] 3. Implement single instance. Verify: `go test ./desktop/shell/... -run TestSingleInstance` passes.
- [ ] 4. Implement start at sign-in with its removal. Verify: `go test ./desktop/service/...` passes on Windows; the registration is checked and cleaned by the test.
- [ ] 5. Add the first-time dialog and the settings, in both languages. Verify: `task test:web` and `bun run check:i18n` pass.
- [ ] 6. Update the Windows installer and uninstaller. Verify: `bash scripts/desktop-verify.sh` passes.
- [ ] 7. Run the real thing on Windows: schedule a target two minutes ahead with the fake connector, close the window, watch it publish; sign out and in with start at sign-in on. Verify: the steps and the result are recorded in `desktop/results/` and in the hand-off.

## Test strategy

- Unit: lifecycle decisions as pure functions (what closing does given the options and the schedule).
- Integration on Windows: single instance, start-at-sign-in registration and removal.
- Manual on Windows: the two scenarios of task 7.

## Acceptance criteria

1. With the option on and a target scheduled, closing the window keeps the process alive with a tray icon, and the target publishes at its time.
2. With the option off, closing the window exits as before.
3. A second launch focuses the running instance.
4. Turning start at sign-in off, or uninstalling, leaves no registration behind.
5. Quit from the tray stops the process and states how many targets will wait.
6. `go test ./desktop/...`, `task test:web` and `bash scripts/desktop-verify.sh` pass on Windows.

## Risks and mitigation

- **The shell framework lacks a tray or single-instance feature on the pinned version.** Mitigation: check first; a version change or another library is a dependency decision. **STOP** in that case.
- **Antivirus or the operating system flags a resident process.** Mitigation: per-user registration, visible tray icon, clear name.

## Size

M

## Hand-off for review

The diff of `epic/21-desktop-background-service`; the manual run record; the outputs of the acceptance commands; what was not verified on macOS and Linux.
