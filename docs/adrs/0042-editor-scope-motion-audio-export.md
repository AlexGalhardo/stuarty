# ADR-0042: The video editor's scope for motion, audio and export

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: PRD-001 section 7 (what was out of scope), ADR-0027 (HyperFrames as an external engine)
- Links: author decisions D-065, D-066, D-067, D-038 (reduced); ADR-0003, ADR-0011, ADR-0029; PRD-001

## Context

The author wants the editor finished with what creators use almost all the time, plus the most used fifth of three professional tools: After Effects for motion, Audition for audio and Media Encoder for export. Author decision D-038 aimed at feature parity with CapCut; that is no longer the goal. The familiar layout and shortcuts stay.

The editor has keyframes for position, scale, rotation and opacity with fixed easings, text presets, six effects, shape masks, volume, fades, ducking, silence removal, loudness normalization, and export to MP4 (H.264 and AAC), GIF, WAV, AAC and PNG. HyperFrames (Apache-2.0) is the engine agents already know for motion graphics (research note `web/04-ferramentas-similares`).

## Decision

1. **Motion.** In the editor: editable easing curves, animated crop and mask, presets for layers and text, and nested compositions. Anything more complex is made in HyperFrames as an external engine, on the agent's or the user's side, and comes back as a clip; HyperFrames is never linked into the binary.
2. **Audio.** Local processors (equalizer, compressor, limiter, gate, de-esser) and local noise reduction with a permissively licensed model or library, which needs the author's approval. Audio does not leave the machine for this.
3. **Export.** Presets per platform, a batch queue (several outputs from one project, each in the format of its destination), and H.265 and AV1 through the system's and the hardware's encoders, inside the LGPL-only policy of ADR-0011.
4. **Out of scope:** tracking, expressions, particles, 3D and rotoscoping; spectral editing and studio multitrack; ProRes, DNxHR and finishing formats.
5. **Every rule stays in `core/`**, every formula is written in the specification before any executor, and the renderer of ADR-0029 draws it.

## Alternatives

- **Motion only in the editor.** Rejected: it rebuilds what exists.
- **Motion only in HyperFrames.** Rejected: two tools for any animation.
- **Voice clean-up by an AI provider.** Rejected: cost per use, and audio leaving the machine.

## Consequences

Positive: a bounded list that an epic can finish.

Negative: a second engine in the creator's workflow for complex motion; the hand-off of a clip into the timeline and the asset library must be smooth.

Negative: a noise reduction model is a new dependency with a licence to check.
