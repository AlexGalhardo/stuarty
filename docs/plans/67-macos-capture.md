# 67: Recording on macOS

## Goal

A creator on a Mac records with the same flow as on Windows, with the system permission explained plainly at first use.

## Context and related decisions

- Author decision (2026-11-23): macOS comes after Windows.
- ADR-0049, ADR-0051, epic 43 (the desktop on macOS, blocked for lack of hardware), author decision D-033 (unsigned installers), `desktop/packaging/macos/Info.plist`, which has no usage keys for camera, microphone or screen today.
- `docs/planning/screen-recording/research.md` section 3.

## User stories

- As a creator on a Mac, I want to record my screen, voice and computer sound so that I have the same studio as on Windows.
- As a creator, I want to be told what permission is needed and why so that I can grant it safely.

## Scope

- Modules: `capture/` macOS backend, `desktop/packaging/macos/`, the permission onboarding in the dialog, SDD-023, user docs.
- Must not touch: the core rules and the protocol, which stay common.

## Out of scope

Intel-specific tuning beyond a pass or fail check, Mac App Store distribution.

## Dependencies

62, **43 (real macOS hardware)**, and the author's decision on signing and notarization.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop, macOS | Screen, microphone, webcam, system audio; cursor and click events if the permission is granted |
| HTTP, CLI, MCP | No change |

## Core commands and permissions

No new operation. `RecordingSources` reports the state of each system permission. Scopes and the person rule are those of epics 61, 62 and 64.

## Technical design

- ScreenCaptureKit in the macOS backend; Swift where the API requires it, called from the helper [to validate].
- The helper lives inside the app bundle, so the system attributes the permission to a known identity [to validate on real machines].
- Microphone inside the stream needs macOS 15. Earlier versions use a separate audio path or lose that option; minimum versions are fixed by the spike.
- Global click events need Input Monitoring or Accessibility [to validate]. Refused permission degrades to a recording with no click data, stated in the UI.

## Tasks

- [ ] 1. Spike on real machines: helper signed, ad hoc and notarized; repeated prompts; permission after an update. Verify: a results table for macOS 13 to 26 in the hand-off.
- [ ] 2. The author's decision on signing, recorded in ADR-0049. Verify: `task lint:md` exits 0.
- [ ] 3. The ScreenCaptureKit backend, video. Verify: the Rust gate of `capture/` on macOS and a playable file read by `ffprobe`.
- [ ] 4. Audio and webcam. Verify: the tone and flash fixture of epic 62 on macOS.
- [ ] 5. Cursor and click events. Verify: a synthetic input test, or the limit documented.
- [ ] 6. Bundle keys and the permission onboarding. Verify: `bun run typecheck`, `bun run check:i18n`, `task test:web`, and a first-run check on a clean macOS user.

## Test strategy

Manual and scripted runs on real Macs, since CI cannot grant these permissions. Each result names the macOS version.

## Acceptance criteria

1. **Given** a clean macOS user, **when** they record for the first time, **then** the prompt appears once and recording works after granting it.
2. **Given** an app update, **when** the user records again, **then** the permission is still valid, or the UI explains exactly what to do.
3. **Given** the permission is refused, **when** recording is attempted, **then** a code from SDD-000 and a clear instruction appear and no helper keeps running.
4. **Given** the sync fixture, **when** measured, **then** drift is at most 40 ms.
5. `task lint`, `task test` and the Rust gate pass on macOS.

## Risks and mitigation

- Permission loss with ad hoc signing, helper not listed in System Settings: this is the largest technical risk of the plan.
- **STOP** if capture cannot work without signing or notarization: paying for an Apple Developer account is the author's decision.
- **STOP** if no real Mac is available.

## Size

L: a new backend and permission behaviour that only real machines can prove.

## Priority

Should. Many creators use Macs, but the hardware and signing are outside the team's control.

## Hand-off for review

The commits, the results table by version, each criterion with its command and result.
