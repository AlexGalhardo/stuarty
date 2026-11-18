# Desktop

Two windows live here. The editor a person runs is the Gio application in
`gio/` (ADR-0030): it opens the workspace in this process and calls
`service.Service`, so it does not reimplement a timeline. The browser UI is
unchanged. The Wails webview in `shell/app/` (ADR-0023) is still what the
installers package.

## Gio editor

| Path | What it is |
| --- | --- |
| `gio/` | Module `github.com/AlexGalhardo/vukem-video/desktop/gio`. The window, the timeline and the inspector. No editing rule: splits, trims and defaults are commands. |
| `gio/cmd/vukem-video-desktop/` | The program. `-workspace` selects the directory, `-demo` opens a sample project, `-screenshot` writes one PNG without a window. |

Build and run, from `desktop/gio`:

```text
go test .
go build -o vukem-video-desktop ./cmd/vukem-video-desktop
./vukem-video-desktop
```

The top bar switches between the dark theme and a light one. The choice is
stored in `gio.json` under the user config directory (`vukem-video/gio.json`),
separate from the webview shell's `desktop.json`.

The editor is local mode only. Import, the preview and export need FFmpeg and
ffprobe on `PATH`, or `VUKEM_VIDEO_FFMPEG` and `VUKEM_VIDEO_FFPROBE`. Text,
shapes and the timeline work without them. `-screenshot out.png` renders one
frame with the headless GPU (EGL on Linux) and exits.

### Linux

A C compiler, pkg-config, and the window-system headers. Debian and Ubuntu:

```text
sudo apt install gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev \
  libxkbcommon-dev libxkbcommon-x11-dev libxcursor-dev libxfixes-dev \
  libegl1-mesa-dev libgles2-mesa-dev libvulkan-dev libffi-dev
```

A machine without Wayland can build with `-tags nowayland`. A machine without
Vulkan can build with `-tags novulkan`.

### macOS

The Xcode command line tools (`xcode-select --install`). The window uses Cocoa.

### Windows

```text
set CGO_ENABLED=0
go build -o vukem-video-desktop.exe ./cmd/vukem-video-desktop
```

No extra C toolchain: the window uses the system graphics stack.

## Webview shell

The packaged desktop application of RF-213 (ADR-0023). Nothing in `shell/`,
`detect/`, `update/` or `packaging/` contains editing logic or renderer code,
and nothing there imports `core/` or `engine/`.

| Path | What it is | State |
| --- | --- | --- |
| `detect/` | The feature-detection page and its server (SDD-012 section 21.2) | built; see its README |
| `results/` | One results file per tested webview | filled by P3-DSK-001 |
| `shell/` | Package `shell`, with no window toolkit: the bridge `window.vukemDesktop` (`bridge.go`: message parsing, the page script, the answer script), starts and stops `vukem-video serve` on a free loopback port, reads `runtime.json` and the local token, waits for `/up`, reports a startup failure as a typed error (`process.go`), obtains the one-time launch address after a handshake (`launch.go`); the navigation policy `Allowed(url, port)` and `GuardScript` (`navigate.go`) | built, tested with a fake server |
| `shell/app/` | The window: Wails v3 v3.1.0, one window on the local address, native menu (File, Edit, Clip, Timeline, View and Help, Quit, Reload, About, Open workspace folder, Toggle full screen on F11), the native file dialog and file drops of the bridge, a dialog for a startup failure. A Go module of its own | builds on the three systems; run on Windows 11 (screenshot in `results/windows/`) |
| `update/` | The auto-update client (SDD-009 section 10.5): off by default, one https GET of a signed feed from one host, SHA-256 and signature of the artifact checked before the shell's `Installer` is called. The feed URL and the public key are build-time values (`-X`) that `desktop.yml` sets from the repository variable `UPDATE_FEED_PUBLIC_KEY`; a build without them has no client. The release signs and publishes the feed (SDD-009 section 10.6) | built, tested against a local https feed |
| `packaging/` | The unsigned installers of the release (D-033, SDD-009 section 10.6): `windows/` (Inno Setup script and smoke test), `linux/` (`.deb` with `dpkg-deb`), `macos/` (`.dmg` with `hdiutil`), built by `.github/workflows/desktop.yml` | built by CI; not run locally |

## Decision

The window a person launches is the Gio editor (ADR-0030). The author chose
Wails v3 on 2026-11-15 for the webview shell (ADR-0023 item 5). The detection
runs on the real webviews (P3-DSK-001) still decide whether a platform falls
back to Electron. Wails is only in `shell/app/go.mod`, and Gio is only in
`gio/go.mod`, so the server binary and the root `go.mod` link neither.

## Build and test

```text
go test ./desktop/...                                  # detect and shell, from the repository root
cd desktop/shell/app && go build -ldflags "-X main.version=0.9.0" .
cd desktop/shell/app && CGO_ENABLED=0 go build -tags purego .   # Linux and macOS with no cgo
```

On Windows the window uses WebView2 (the runtime ships with Windows 11). On Linux
and macOS the `purego` tag builds with no C toolchain and no WebKitGTK headers;
without it Wails needs cgo, and on Linux the GTK and WebKitGTK development
packages (`libgtk-3-dev` and `libwebkit2gtk-4.1-dev`, or the 6.0 packages for
GTK 4), and on macOS the Xcode command line tools.

Run it: `vukem-video-desktop -binary <path to vukem-video>`. Without `-binary` it
uses the `vukem-video` next to its own executable.

## Build and run on Windows

From the repository root, with Go and Bun installed (Windows 11 already has WebView2):

```text
cd web && bun install --frozen-lockfile && bun run build && cd ..   # the UI the server embeds
go build -o dist/vukem-video.exe ./web/cmd/app
cd desktop/shell/app && go build -ldflags "-H windowsgui -X main.version=0.11.0" -o ../../../dist/vukem-video-desktop.exe .
```

Put both executables in one folder and double-click `vukem-video-desktop.exe`
(never commit an `.exe`). Without `bun run build` the server answers 500 on
every page, because the Vite manifest is missing. The first start takes a while
while WebView2 starts. The shell:

1. starts `vukem-video serve` on a free `127.0.0.1` port, with the workspace in
   `VUKEM_VIDEO_HOME` (default of the server otherwise), and waits for `/up`;
2. proves the server knows the workspace token (handshake), asks for a one-time
   launch address (`POST /api/v1/system/launch`) and opens it in the window, so
   the window gets its session cookie and never sees the token (ADR-0019);
3. on Quit, closing the window or a crash of the shell, stops the server: a job object on Windows,
   `Pdeathsig` on Linux, and on macOS (no parent-death signal) a small watcher in `/bin/sh` that
   ends the server when the shell is gone.

Menu: File (Open project, Import files, Export, Open workspace folder, Quit), Edit, Clip, Timeline,
View (the panels, Reload, Toggle full screen) and Help (Keyboard shortcuts, updates, About, with the
version set by `-X main.version`). The editor items come from `shell/menu.go` and are the items of the
menu table of the page (`web/resources/js/editor/commands/menu.ts`): a click sends the action id to the
page through `window.vukemDesktop.onMenuAction`, and the page runs the same action as its own control, so
the shell decides nothing about editing. No editor item is a native accelerator: Undo, Redo and Select
all only show `Ctrl+Z`, `Ctrl+Shift+Z` and `Ctrl+A` (`Cmd` on macOS), because the key reaches the webview,
where the editor decides between the undo of a field and the project's (SDD-012 section 21.3 item 4).
A test of the web unit suite fails when the two tables name different actions.

## The bridge

After every page load the shell defines `window.vukemDesktop` on the editor page
(SDD-012 section 13.1 item 2): `pickFiles()` opens the native file dialog and
resolves with absolute paths, `onDropPaths(cb)` delivers the paths of dropped
files, `onMenuAction(cb)` delivers the action id of a clicked menu item. The editor sends them to the import API; it never reads the bytes. The
shell accepts a message only from the main frame of its own server origin, and
the only operation it knows is `pickFiles`. Without the object (a browser) the
editor keeps the pasted-path behaviour.

## Known limit

Wails v3.1.0 gives no hook to refuse a navigation before it happens. The shell
sends the window back to its home page after a navigation ends at another origin
(`GuardScript`); the pure rule `Allowed` is what the tests check. A native veto
needs a change in Wails.

## CI

`.github/workflows/desktop-detect.yml` runs the detection page in Playwright's
WebKit on Linux and macOS (the results files are artifacts; a `fail` verdict does
not fail the job) and builds the shell for the three systems.
