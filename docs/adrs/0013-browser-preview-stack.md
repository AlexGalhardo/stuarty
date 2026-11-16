# ADR-0013: Browser stack: React, a canvas compositor, `<video>` proxies

- Status: Accepted (2026-11-14)
- Deciders: architect; author (D-013, D-023, D-024)
- Links: stack trade-offs §2, §3, §9, performance §3, ADR-0003, ADR-0007, SDD-012

## Context

The stack draft proposed React, Vite, Tailwind, shadcn/ui, Zustand, Immer, dnd-kit, PixiJS, mediabunny or mp4box.js, wavesurfer.js, Web Workers and OPFS, with preview "evolving to WebCodecs". The decisions since then (export on the server, thin client, truth on pause) remove most of the media work from the browser. The target is the latest stable Chrome.

## Decision

Adopted:

| Piece | Choice | Why |
| --- | --- | --- |
| UI | React 19, TypeScript, Vite | already in the repository |
| Styles and components | Tailwind 4, shadcn/ui | already in the repository |
| Theme | dark by default, light available through a toggle (D-024) | a dark surround does not bias the perception of the picture |
| UI state | Zustand | selection, zoom, panels, playhead; the project is a read model (ADR-0007) |
| Preview compositor | PixiJS v8 on a canvas, behind an interface `PreviewExecutor` | layers are sprites with transform, crop and opacity; avoids a DOM/CSS preview |
| Video sources | `<video>` elements playing the proxy served with HTTP Range | enough for playback; exactness comes from truth on pause |
| Timeline | DOM elements with Pointer Events | a 5 minute project has tens of items |
| Waveforms and thumbnails | drawn from peaks and strips produced at ingest | no decoding in the browser |
| Ordinary pages | vukem's Inertia adapter | project list, settings |
| Editor page | a client-rendered route with JSON and server-sent events | decoupled from Inertia |
| Real-time | `EventSource` | ADR-0017 |
| Tests | Playwright end to end; Vitest only where TypeScript has logic | the browser holds little logic |

Not adopted in the MVP: Immer (undo is in the core), dnd-kit (classic line unreleased since 2024-12), WebCodecs and mediabunny or mp4box.js (an optimisation for scrubbing, to revisit with measurements), wavesurfer.js, Web Workers for media, OPFS, the File System Access API, WebGPU.

Rules:

1. The compositor draws what a plan window says and nothing else.
2. The number of `<video>` elements decoding at once is capped by a measured constant.
3. The playhead clock follows the audio during playback.
4. The browser stores no project data.

## Alternatives

- **DOM and CSS preview.** The cause of FreeCut's and Reframe's divergences.
- **Canvas 2D without a library.** Fewer dependencies; no path to colour lookup tables or blur without rewriting.
- **WebCodecs decoding with a frame cache from the start.** Frame-accurate scrubbing, at the cost of decoder pooling, frame lifetime management and a demuxer, for a need that truth on pause already covers.

## Consequences

Positive: a small browser footprint; the features that differ most between browsers and system webviews are not used, so Safari, Firefox and a desktop webview are cheap to add.

Negative: playback is approximate by design; the exact frame appears only on pause.

Negative: PixiJS is a large library for a small job. It stays behind `PreviewExecutor` so it can be replaced.
