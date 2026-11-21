# 58: Sound assets

## Goal

A creator finds, listens to and reuses every sound of their library (music, effects, narration, generated voices) from one page, the way they already do for pictures and videos.

## Context and related decisions

- Author decision D-121 (2026-11-20): a "Sound Assets" link in the dashboard sidebar and a page that is the sibling of Visual Assets and Video Assets.
- The library exists (epic 08, SDD-011 section 14): `web/resources/js/assetlibrary/AssetLibrary.tsx` serves `pages/VisualAssets/` and `pages/VideoAssets/`; "use in a project" was fixed by VV-321 (the timeline choice).
- ADR-0002 (the UI computes nothing), SDD-011, SDD-012 section 2.1 (navigation).

## Scope

- Modules: `web/resources/js/pages/SoundAssets/`, the sidebar entry (`web/resources/js/dashboard/`), the library filter for audio in the service if the existing filter does not cover it, the route in `web/routes/`, en and pt-BR strings, SDD-011 and SDD-012, the user docs of the library.
- Must not touch: the sidebar footer, sign-in and settings (epic 57), the projects page (epic 56).

## Out of scope

- Generating sound; editing audio; new audio formats.

## Dependencies

08, 21 (merged).

## Technical design

- The page reuses `AssetLibrary` with the audio kind: search, tags, duration, size, import a file, "use in a project" (with the timeline choice), trash.
- A player per row with a waveform drawn from the peaks the engine already computes for the timeline (SDD-011); if no peaks exist for a file, the server computes them on request, never the browser.
- The sidebar gets "Sound Assets" next to Visual Assets and Video Assets, with an icon of the same family.

## Tasks

- [x] 1. SDD-011 and SDD-012: the page, the filter, the waveform source. Verify: `task lint:md` exits 0.
- [x] 2. The audio filter and the peaks on request in the service, with tests on HTTP, MCP and CLI parity. Verify: `go test ./service/... ./web/...` passes.
- [x] 3. The page, the player with the waveform and the sidebar entry. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, Vitest and a Playwright spec (import a sound, play it, use it in a project with two timelines).

## Acceptance criteria

1. The sidebar shows Sound Assets; the page lists only audio, with search, tags and duration.
2. A sound plays with its waveform; the waveform comes from the server.
3. A sound can be imported and used in a project, choosing the timeline when there are several.
4. `task lint`, `task test`, `task test:web` and `bun run check:thin-client` pass; no new dependency.

## Risks and mitigation

- A long file has no peaks yet: compute them as a job with progress, not in the request.

## Size

M

## Hand-off for review

The commits, each acceptance criterion with the command and its result, how to try the page.
