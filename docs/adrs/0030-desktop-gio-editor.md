# ADR-0030: The desktop editor is a Gio window over the service

- Status: Superseded (2026-11-18) by [ADR-0041](0041-one-web-ui-and-the-wails-shell.md): the Gio window is removed and the desktop is the Wails shell. Accepted earlier on 2026-11-18
- Deciders: author
- Amends: ADR-0023 items 3 and 5 (no second UI; the desktop window is Wails), for the editor window only
- Links: ADR-0007, ADR-0008, ADR-0023, SDD-012 section 21

## Context

ADR-0023 ships the desktop application as a system webview around the browser UI. That keeps one UI, and it is still how the installers are built. The author asked for release 0.29.0 to redraw the desktop editor as a native window, close to the CapCut desktop layout, without a second editing engine and without changing the browser.

Gio (gioui.org) is an immediate-mode GUI toolkit in Go. It can draw the window. The editing rules already live in `core/` and are reached only through `service.Service` (ADR-0008). The desktop process can be the workspace owner, the same way a CLI command is when no server is running.

## Decision

1. **The desktop editor is `desktop/gio`.** The command `desktop/gio/cmd/vukem-video-desktop` opens the workspace with `owner.Open` in local mode and draws the window with Gio. Every project, import, edit, undo, preview frame and export is a method of `service.Service`. The window holds no timeline rule.
2. **The browser UI does not change.** `web/` stays the client of `vukem-video serve`. The Gio module is nested, like `desktop/shell/app`, so the server binary does not link Gio.
3. **The webview shell stays until packaging moves.** `desktop/shell` and the installers in `desktop/packaging` still build the Wails window of ADR-0023. ADR-0030 does not retarget those installers.
4. **Gio is an approved dependency of the desktop module only**, at `gioui.org` v0.9.0. Linux needs a C compiler, pkg-config, and the Wayland or X11, EGL and Vulkan development libraries. macOS needs the Xcode command line tools. Windows builds with `CGO_ENABLED=0`.

## Alternatives

- **Keep drawing the browser UI inside Wails.** One UI, but not the native layout that was asked for.
- **Reimplement editing in the window.** Rejected: it would fork the command core (ADR-0002, ADR-0007).
- **Talk to a child `vukem-video serve` over HTTP.** Works, and it is what the Wails shell does, but it needs the embedded web assets and a second process for a window that only calls the service.

## Consequences

Positive: a person can edit in a native window with the same commands an agent sends. Playback uses `export.NewSequenceRenderer`: the export's decoders stay open and read ahead, and the audio mix of an export is played in step with that clock. A paused scrub still asks for one composited frame. `ViewFrame` (one JPEG) stays the agent and browser path.

Negative: two desktop windows exist until the installers are switched. The Gio window does not yet play proxy video, waveforms, captions word by word, or the rest of the browser inspector. Those gaps are listed on the release pull request. Audio is not played. A missing FFmpeg disables import, preview and export and leaves editing of text and shapes working.
