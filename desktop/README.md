# Desktop

The packaged desktop application of RF-213 (ADR-0023). Nothing here contains
editing logic or renderer code, and nothing imports `core/` or `engine/`.

| Path | What it is | State |
| --- | --- | --- |
| `detect/` | The feature-detection page and its server (SDD-012 section 21.2) | built; see its README |
| `results/` | One results file per tested webview | filled by P3-DSK-001 |
| `shell/` | Package `shell`, with no window toolkit: the bridge `window.vukemDesktop` (`bridge.go`: message parsing, the page script, the answer script), starts and stops `vukem-video serve` on a free loopback port, reads `runtime.json` and the local token, waits for `/up`, reports a startup failure as a typed error (`process.go`), obtains the one-time launch address after a handshake (`launch.go`); the navigation policy `Allowed(url, port)` and `GuardScript` (`navigate.go`) | built, tested with a fake server |
| `shell/app/` | The window: Wails v3 v3.1.0, one window on the local address, native menu (Quit, Reload, About, Open workspace folder, Toggle full screen on F11), the native file dialog and file drops of the bridge, a dialog for a startup failure. A Go module of its own | builds on the three systems; run on Windows 11 (screenshot in `results/windows/`) |
| `update/` | The auto-update client (SDD-009 section 10.5): off by default, one https GET of a signed feed from one host, SHA-256 and signature of the artifact checked before the shell's `Installer` is called. The feed URL and the public key are build-time values (`-X`) that `desktop.yml` sets from the repository variable `UPDATE_FEED_PUBLIC_KEY`; a build without them has no client. The release signs and publishes the feed (SDD-009 section 10.6) | built, tested against a local https feed |
| `packaging/` | The unsigned installers of the release (D-033, SDD-009 section 10.6): `windows/` (Inno Setup script and smoke test), `linux/` (`.deb` with `dpkg-deb`), `macos/` (`.dmg` with `hdiutil`), built by `.github/workflows/desktop.yml` | built by CI; not run locally |

## Decision

The author chose Wails v3 on 2026-11-15 (ADR-0023 item 5). The detection runs on
the real webviews (P3-DSK-001) still decide whether a platform falls back to
Electron. Wails is only in `shell/app/go.mod`, so the server binary and the root
`go.mod` never link it.

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
3. on Quit, closing the window or a crash of the shell, stops the server.

Menu: File (Open workspace folder, Quit), View (Reload, Toggle full screen), Help (About, with the
version set by `-X main.version`).

## The bridge

After every page load the shell defines `window.vukemDesktop` on the editor page
(SDD-012 section 13.1 item 2): `pickFiles()` opens the native file dialog and
resolves with absolute paths, `onDropPaths(cb)` delivers the paths of dropped
files. The editor sends them to the import API; it never reads the bytes. The
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
