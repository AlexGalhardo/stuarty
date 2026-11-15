# Desktop

The packaged desktop application of RF-213 (ADR-0023). Nothing here contains
editing logic or renderer code, and nothing imports `core/` or `engine/`.

| Path | What it is | State |
| --- | --- | --- |
| `detect/` | The feature-detection page and its server (SDD-012 section 21.2) | built; see its README |
| `results/` | One results file per tested webview | filled by P3-DSK-001 |
| `shell/` | Package `shell`, with no window toolkit: starts and stops `vukem-video serve` on a free loopback port, reads `runtime.json` and the local token, waits for `/up`, reports a startup failure as a typed error (`process.go`); the navigation policy `Allowed(url, port)` and `GuardScript` (`navigate.go`) | built, tested with a fake server |
| `shell/app/` | The window: Wails v3 v3.1.0, one window on the local address, native menu (Quit, Reload, About, Open workspace folder), a dialog for a startup failure. A Go module of its own | builds on the three systems; not run on a real machine yet |

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

## Known limit

Wails v3.1.0 gives no hook to refuse a navigation before it happens. The shell
sends the window back to its home page after a navigation ends at another origin
(`GuardScript`); the pure rule `Allowed` is what the tests check. A native veto
needs a change in Wails.

## CI

`.github/workflows/desktop-detect.yml` runs the detection page in Playwright's
WebKit on Linux and macOS (the results files are artifacts; a `fail` verdict does
not fail the job) and builds the shell for the three systems.
