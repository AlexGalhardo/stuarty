# 01: Remove the Gio window

## Goal

A person who installs the desktop application gets one window that shows the same UI as the browser, so every screen built from now on appears on the desktop without being written twice. The unfinished native window added in release 0.29.0 leaves the tree.

## Context and related decisions

- ADR-0041 (one web UI; the desktop is the Wails shell; the Gio window is removed), which supersedes ADR-0030; ADR-0023.
- Author decision D-055. PRD-003 RF-213.
- SDD-012 section 21 (already amended).

## Scope

- Modules: `desktop/gio/` (deleted), `desktop/README.md`, `desktop/packaging/`, `scripts/`, `.github/workflows/desktop.yml` and `release.yml` (only lines that build or ship the Gio binary), `site/content/` pages that describe the native window, `README.md` if it still names it.
- The Wails shell (`desktop/shell`, `desktop/shell/app`), the update feed (`desktop/update`) and the detection page (`desktop/detect`) stay untouched and keep working.

## Out of scope

- Any change to the web UI or to the shell's behaviour.
- The rename of the product (epic 02).
- Validating the shell on macOS and Linux (epic 43).

## Dependencies

None. It is the first epic.

## Technical design

- `desktop/gio` is a nested Go module (`desktop/gio/go.mod`, `gioui.org` v0.9.0) with its own command `cmd/vukem-video-desktop`. Nothing in the root module imports it, so deleting the folder cannot break the server build.
- The name `vukem-video-desktop` is also used by the packaging of the Wails shell. Separate the two: a reference is removed only when it points at the Gio module (a build of `./desktop/gio/...`, a Gio-only dependency such as the Wayland, EGL or Vulkan development packages, a job that uploads the Gio binary).
- Third-party notices: `gioui.org` and its dependencies leave `THIRD_PARTY_NOTICES.md`, `licenses/` and `.config/third-party.json` if they are listed there; regenerate with `task notices`.

## Tasks

- [ ] 1. List every reference to the Gio module and classify it as "Gio only" or "shared with the shell" in the hand-off. Verify: `git grep -n -iE "desktop/gio|gioui" -- ':!desktop/gio'` output is attached with a class per line.
- [ ] 2. Delete `desktop/gio/`. Verify: `go build ./...` exits 0 and `(cd desktop/shell/app && go build ./...)` exits 0.
- [ ] 3. Remove the Gio-only lines from `desktop/packaging/`, `scripts/` and the two workflows; keep the shell's packaging intact. Verify: `git grep -n -iE "desktop/gio|gioui"` prints nothing outside `docs/adrs/`.
- [ ] 4. Rewrite `desktop/README.md` for the shell only, and fix the site pages that describe a native window (both languages). Verify: `task lint:md` and `task lint:site` exit 0.
- [ ] 5. Regenerate the notices. Verify: `task notices:check` exits 0.
- [ ] 6. Run the shell's tests. Verify: `go test ./desktop/...` and `(cd desktop/shell/app && go test ./...)` pass.

## Test strategy

- No new behaviour, so no new test. The existing suites of `desktop/shell`, `desktop/update` and `internal/tools/updatefeed` must pass unchanged.
- Build the shell on Windows and open it once: it must show the projects page.

## Acceptance criteria

1. `desktop/gio/` does not exist and no file outside `docs/adrs/` mentions `desktop/gio` or `gioui`.
2. `go build ./...`, `go vet ./...` and `go test ./desktop/... ./internal/tools/updatefeed/...` exit 0.
3. The Wails shell builds on Windows (`go build` in `desktop/shell/app`) and its window opens on the local address.
4. `task lint:md`, `task lint:site` and `task notices:check` exit 0.
5. `desktop/README.md` describes one desktop application, the shell.

## Risks and mitigation

- **Deleting something the shell needs.** Mitigation: task 1 classifies before anything is removed; the shell is built and opened in acceptance criterion 3.
- **STOP** if a workflow or a script cannot be untangled without changing how the shell is packaged: report the lines and wait.

## Size

S

## Hand-off for review

The diff of `epic/01-remove-the-gio-window`; the classified list of task 1; the output of the commands of the acceptance criteria; a note of anything left on purpose with its reason.
