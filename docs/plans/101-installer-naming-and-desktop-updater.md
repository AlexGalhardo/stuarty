# 101: Explicit web-localhost installer naming and desktop in-app auto-updater

## Goal

Users downloading Stuarty can immediately tell the desktop application (`stuarty-desktop-X-Y-Z.exe`) apart from the browser/localhost server installer (`stuarty-web-localhost-X-Y-Z.exe`). Inside Stuarty Desktop, users can check for and install the latest stable release directly from the Login screen or the dashboard sidebar footer, and toggle automatic updates on or off whenever the computer is connected to the internet.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  1. Rename the standalone localhost server installer from `stuarty-0-60-0.exe` to `stuarty-web-localhost-0-60-0.exe` (pattern `stuarty-web-localhost-X-Y-Z.exe`) so it is unmistakable that it runs the web localhost version rather than the native desktop shell.
  2. Add an "Update to latest stable" control in two places inside Stuarty Desktop: on the Login / Unlock screen and in the dashboard sidebar footer.
  3. Include an on/off toggle for automatic update checks and downloads whenever the user has an internet connection.
- Existing infrastructure verified in the codebase:
  - `.config/installers/windows/stuarty.iss:50` currently defines `OutputBaseFilename=stuarty-{#FileVersion}`.
  - `desktop/update/` and `desktop/shell/updates.go` + `desktop/shell/launcher.go` + `desktop/shell/app/updater.go` already implement signed `feed.json` verification, `Settings.UpdateCheck`, `CheckNow`, `SetAutoCheck`, and installer execution, but currently expose them ONLY through the native OS menu bar (not through `window.vukemDesktop` or the web UI).
  - `web/resources/js/lib/desktopBridge.ts` and `desktop/shell/app/bridge.go` already define the narrow desktop bridge (`window.vukemDesktop`) for background settings (`getBackground` / `setBackground`), which is the exact pattern to follow for update state and actions.
- ADR-0032 (outbound only with explicit consent: automatic updates remain opt-in or explicitly toggled by the user; toggling off makes zero outbound calls), ADR-0041 (one web UI + Wails shell), SDD-009 §10.5 and §18, SDD-012 §13.1 and §23.4.

## Scope

- Modules owned:
  - `.config/installers/windows/stuarty.iss`, `.config/installers/linux/build-appimage.sh`, `.config/installers/macos/` (consistent `stuarty-web-localhost` naming for the standalone localhost server installers).
  - `.github/workflows/installers.yml`, `.github/workflows/release.yml`, `scripts/release-verify.sh`, `scripts/update-feed.sh`, `scripts/desktop-verify.sh` (update expected asset filenames and checksum verification patterns).
  - `desktop/shell/` (`bridge.go`, `updates.go`, `launcher.go`) and `desktop/shell/app/` (`bridge.go`, `updater.go`): expose read/check/install/setAutoUpdate operations on `window.vukemDesktop` (`getUpdateState`, `checkForUpdates`, `installUpdate`, `setAutoUpdate`) plus connectivity-aware auto-update behavior.
  - `web/resources/js/lib/desktopBridge.ts` (typed update bridge helpers).
  - `web/resources/js/components/DesktopUpdateControl.tsx` (reusable compact update button + auto-update switch, rendered only when `canUpdate(bridge)` is true).
  - `web/resources/js/pages/Account/SignIn.tsx`, `web/resources/js/pages/Auth/Login.tsx`, `web/resources/js/dashboard/Sidebar.tsx` (place the update control on Login and in the expanded sidebar footer).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`, `docs/sdds/009-releases-and-ci-cd.md`, `docs/sdds/012-ui-architecture.md`, `site/content/docs/en/getting-started/install-desktop.mdx`.
- Must not touch: `core/`, `service/`, `engine/`, `renderer/`.

## Out of scope

- Changing the desktop installer name (`stuarty-desktop-X-Y-Z.exe` stays as-is).
- Auto-updating Docker/VPS deployments (those use `deploy/update.sh`).
- Bypassing update signature verification (`feed.json` signature and SHA-256 verification stay mandatory per SDD-009 §10.5).

## Dependencies

- Epic 100 (silent Windows desktop shell). Coordinate merges of `Sidebar.tsx` and `SignIn.tsx` with epics 102 and 103.

## Technical design

1. **Web-localhost installer naming**:
   - In `.config/installers/windows/stuarty.iss`, change `OutputBaseFilename=stuarty-{#FileVersion}` to `OutputBaseFilename=stuarty-web-localhost-{#FileVersion}` (so `0.60.0` builds `stuarty-web-localhost-0-60-0.exe`).
   - Update `.config/installers/linux/build-appimage.sh` (`stuarty-web-localhost-${version//./-}.AppImage`) and macOS installer naming if it publishes a localhost bundle, keeping the installed executable binary name inside the package as `stuarty` / `stuarty.exe` so CLI scripts (`stuarty serve`, `stuarty doctor`) do not break.
   - Update `.github/workflows/installers.yml`, `.github/workflows/release.yml`, `scripts/release-verify.sh`, `site/lib/latest-release.ts` (if it links Windows installers), and SDD-009 §18.

2. **Desktop update bridge (`window.vukemDesktop`)**:
   - Add four narrow bridge operations in `desktop/shell/bridge.go` and `desktop/shell/app/bridge.go`, gated by the existing loopback-origin check (`shell.Allowed`):
     - `getUpdateState` -> `{ current_version, available_version?, release_notes?, auto_update, status: "idle"|"checking"|"available"|"up_to_date"|"downloading"|"installing"|"offline"|"unconfigured"|"error", progress?, error? }`
     - `checkForUpdates` -> triggers a signed feed check against the latest stable release (`feed.json`) and returns the updated state (offline/DNS failure maps cleanly to `status: "offline"` rather than a scary crash).
     - `installUpdate` -> downloads, verifies SHA-256 and signature, stages, and runs the installer for the verified `available_version` (same verified path as `shell.Launcher`).
     - `setAutoUpdate(enabled: boolean)` -> persists `Settings.UpdateCheck` (and a new `Settings.AutoInstall` boolean or immediate download-and-prompt behavior when online) in `desktop.json`.
   - When `auto_update` is `true`, the launcher checks on startup and periodically (and re-checks when a check previously failed offline), downloading the verified release when online and either applying or prompting cleanly without blocking the UI. When `auto_update` is `false`, zero network calls are made unless the user clicks "Update" / "Check for updates" (preserving ADR-0032).

3. **UI placement (Desktop only)**:
   - Create `web/resources/js/components/DesktopUpdateControl.tsx`:
     - Hidden completely in plain web browsers (`!canUpdate(bridge)`).
     - Shows a button ("Update to vX.Y.Z" when an update is available, or "Check for updates" / "Up to date" when current) plus a clearly labeled on/off switch ("Auto-update when online" / "Atualizar automaticamente quando conectado").
     - Displays download progress inline and surfaces errors through localized strings.
   - Mount `<DesktopUpdateControl variant="card" />` below the card on `Account/SignIn.tsx` and `Auth/Login.tsx`.
   - Mount `<DesktopUpdateControl variant="sidebar" />` in the footer of `dashboard/Sidebar.tsx` when expanded (and as an icon button with tooltip when collapsed so it never wraps text).

## Tasks

- [ ] 1. Update SDD-009 (§10.5, §18) and SDD-012 (§13.1, §23.4) with the `stuarty-web-localhost-X-Y-Z.exe` filename and the desktop update bridge contract. Verify: `task lint:md` exits 0.
- [ ] 2. Rename the standalone localhost installer output in `.config/installers/windows/stuarty.iss`, Linux/macOS installer scripts, `.github/workflows/installers.yml`, `.github/workflows/release.yml`, `scripts/release-verify.sh`, and `site/` download references. Verify: `go test ./desktop/service/...` passes and grep confirms zero stale `OutputBaseFilename=stuarty-{#FileVersion}` occurrences.
- [ ] 3. Extend `desktop/shell/` and `desktop/shell/app/` (`bridge.go`, `updates.go`, `launcher.go`, `updater.go`) with `getUpdateState`, `checkForUpdates`, `installUpdate`, and `setAutoUpdate`, including offline detection and automatic check/apply scheduling when toggled on. Verify: `go test ./desktop/...` passes.
- [ ] 4. Implement `web/resources/js/lib/desktopBridge.ts` update methods and `web/resources/js/components/DesktopUpdateControl.tsx`, wire into `Account/SignIn.tsx`, `Auth/Login.tsx`, and `dashboard/Sidebar.tsx`, and add `en.json` + `pt-BR.json` translations. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run`.
- [ ] 5. Add unit and Playwright tests with a mocked `window.vukemDesktop` covering Login screen update button, sidebar footer update button, auto-update toggle persistence, offline state, and hidden state in plain browsers. Verify: `task test:web` and the new Playwright spec pass.

## Test strategy

- Go unit tests: bridge message parsing and origin validation for all 4 update ops; `Launcher` auto-update toggle persistence; offline retry without panic; installer filename tests.
- Web unit tests (Vitest): `DesktopUpdateControl` renders nothing without bridge; renders update button + toggle on Login and Sidebar with bridge; handles `checking`, `available`, `downloading`, `up_to_date`, `offline`, `error`.
- E2E (Playwright): inject mock `window.vukemDesktop` before page load and verify toggling auto-update and triggering an update from both `/account/signin` and `/`.

## Acceptance criteria

1. `.config/installers/windows/stuarty.iss` produces `stuarty-web-localhost-X-Y-Z.exe` (e.g., `stuarty-web-localhost-0-60-0.exe`), and all release/verification scripts and docs match the new name.
2. When running inside Stuarty Desktop (`window.vukemDesktop` present), both the Login screen and the dashboard sidebar footer display the update button and the auto-update on/off toggle.
3. In a plain web browser (`window.vukemDesktop` absent), neither control renders and no update network call is attempted.
4. Turning the auto-update toggle off stops all background update checks (zero outbound connections per ADR-0032); turning it on persists across restarts in `desktop.json` and checks when online.
5. `go test ./desktop/...`, `task lint`, `task test:web`, and `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n` exit 0.

## Risks and mitigation

- **Modifying CI/release workflows.** Per `REVIEW.md`, changes to `.github/workflows/*.yml` require author review before merge. **STOP** and flag workflow changes clearly in the hand-off if `verify.sh` reports `CHECKS_CHANGED`.
- **Silent auto-install interrupting an active export or recording.** Mitigation: if a recording or export job is active, defer applying the downloaded installer until the job completes or the user confirms restart.

## Size

M

## Hand-off for review

The commits of `epic/101-installer-naming-and-desktop-updater`, list of workflow files touched for author review, Go and Vitest/Playwright outputs, and screenshots of the update control on Login and in the Sidebar footer.
