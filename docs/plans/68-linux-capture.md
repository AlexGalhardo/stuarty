# 68: Recording on Linux

## Goal

A creator on Linux records the screen, voice and webcam, and knows in advance which finishing features their desktop session allows.

## Context and related decisions

- Author decision (2026-11-23): Linux comes after macOS.
- ADR-0049, epic 43 (the desktop on Linux, blocked for lack of hardware).
- `docs/planning/screen-recording/research.md` section 3.

## Honest limits

- **On Wayland there is no generic way to read global clicks.** The InputCapture portal depends on the compositor and does not give passive monitoring of everything. Automatic zoom from clicks therefore may be unavailable there.
- The portal asks the user to choose a source. A restore token may avoid repeating it, and support varies by desktop.
- This epic does not promise parity with Windows.

## User stories

- As a creator on Linux, I want to record through my desktop's portal so that it works on Wayland.
- As a creator on Wayland, I want to know that zoom from clicks is unavailable so that I am not surprised after a take.

## Scope

- Modules: `capture/` Linux backend, `desktop/` Linux packaging of the helper, the capability report, the dialog, SDD-023, user docs.
- Must not touch: the core rules and the protocol.

## Out of scope

Compositor-specific protocols, input monitoring through root or `/dev/input`, Flatpak packaging.

## Dependencies

62 and **43 (real Linux machines)**. It can run side by side with 67.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop, Wayland | Screen and audio through portal and PipeWire; cursor metadata where offered; **no click data unless a tested portal allows it** |
| Desktop, X11 | Screen, cursor and clicks |
| HTTP, CLI, MCP | No change |

## Core commands and permissions

No new operation. `RecordingSources` reports `click_events: available | unavailable` with the reason. Scopes and the person rule are those of epics 61, 62 and 64.

## Technical design

- `org.freedesktop.portal.ScreenCast` with PipeWire on Wayland; `x11grab` or an equivalent on X11.
- Cursor mode `Metadata` when the portal offers it.
- System audio through a PipeWire monitor source [to validate].

## Tasks

- [ ] 1. Spike on GNOME and KDE on Wayland, and on X11: portal, restore token, cursor metadata, InputCapture, audio. Verify: a results table in the hand-off.
- [ ] 2. The Wayland backend. Verify: the Rust gate of `capture/` on Linux and a playable file read by `ffprobe`.
- [ ] 3. The X11 backend. Verify: the same on an X11 session.
- [ ] 4. Audio and webcam. Verify: the tone and flash fixture of epic 62.
- [ ] 5. The capability report and the dialog's message. Verify: `go test ./service/...`, `bun run typecheck`, `bun run check:i18n` and `task test:web` pass.

## Test strategy

Real sessions on each desktop; each result names the distribution, desktop and versions.

## Acceptance criteria

1. **Given** GNOME or KDE on Wayland, **when** a person records for 60 seconds, **then** `ffprobe` reports 60 s plus or minus 0.5 s.
2. **Given** a Wayland session with no click source, **when** the dialog opens, **then** it states that zoom from clicks is unavailable before recording.
3. **Given** the same session, **when** `recording.auto_zoom` runs, **then** it fails with a code from SDD-000 and changes nothing.
4. **Given** an X11 session, **when** clicks are made, **then** they appear in `RecordingEvents`.
5. `task lint`, `task test` and the Rust gate pass on Linux.

## Risks and mitigation

- Fragmentation across compositors: only tested desktops are claimed as supported.
- **STOP** if a solution would need root, a kernel input device or a compositor plugin.

## Size

L: two capture paths and several desktops to test.

## Priority

Could. It matters to the open source audience, but serves fewer creators and has real limits.

## Hand-off for review

The commits, the results table, each criterion with its command and result, and the desktops not tested.
