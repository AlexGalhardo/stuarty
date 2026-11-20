# PRD-001: The video editor

- Status: Accepted (2026-11-14); revised on 2026-11-18 for Stuarty (ADR-0031, ADR-0042): the phase column was removed, section 4.9 was added and section 7 was rewritten
- Depends on: author decisions D-001, D-002, D-013, D-014, D-048, D-065, D-066, D-067; research notes `features-20-80` and `performance`
- Implemented by: SDD-001, SDD-002, SDD-004, SDD-005, SDD-006, SDD-011, SDD-012, SDD-013; the requirements of section 4.9 are specified by the epics that build them (`docs/plans/00-roadmap.md`)

The video editor is one domain of Stuarty (PRD-004 covers the others). This document says what the editor must do.

## 1. Problem

Making a short vertical video with captions, or a product commercial with animated text and music, takes a non-linear editor that is either a closed product with paywalls and watermarks or an open project whose engine is tied to its browser UI. Few can be driven end to end by an AI agent from a terminal with no window open, and fewer let a person and an agent work on the same project and see the same result.

## 2. Users

| User | What they do | What they need |
| --- | --- | --- |
| The creator | Plans, edits and publishes content; works in the UI and with an agent | A timeline, a preview and an inspector that cover everything; an agent that can do the same edits |
| The agent operator | A developer or technical creator who already uses a coding agent (Claude Code, Codex, OpenCode). Describes the video; the agent edits | Commands an agent can discover, run and verify; a result they can open and adjust by hand |
| The self-hoster | Runs the product on their own machine or server | One binary, no cloud account, nothing sent out without a connection they created |

Agents themselves are users of the interfaces; their requirements are in [PRD-002](002-agent-operability.md).

## 3. Use cases

1. **Short video** of 30 seconds to 5 minutes for YouTube Shorts, Instagram, TikTok, Kwai and X: memes, cuts of longer footage, informative talking-head videos. Vertical 9:16 by default. Captions are mandatory in practice. This is the first complete flow of Stuarty (ADR-0031).
2. **Commercial video** for a physical or digital product: animated text, transitions, a logo, music, a reusable template.
3. **Long video** for YouTube, cut from recorded footage, from which short cuts are derived.

Reference scenario: an agent, through MCP and through the CLI, imports a video, cuts it, adds a caption and exports a valid MP4; the same project opens correctly in the UI.

## 4. Functional requirements

### 4.1 Projects and media

| ID | Requirement |
| --- | --- |
| RF-001 | Create, list, open and rename projects. Every change is saved as it happens; there is no save action |
| RF-002 | A timeline has a frame size and a frame rate. Presets: 9:16 (1080×1920, default), 1:1 (1080×1080), 16:9 (1920×1080), 4:5 (1080×1350). Default rate 30 fps. Changing size or rate is an explicit command that reports what it rounded |
| RF-003 | Import video, audio and image files by local path. Import starts an ingest with visible progress; a file that cannot be used is reported with the reason and how to fix it |
| RF-004 | A media library lists the assets with duration, dimensions, a thumbnail and ingest state |
| RF-005 | Move projects and assets to a trash and restore them from it |
| RF-006 | Export a project as a bundle (document, history and media) and import it on another machine |

### 4.2 Timeline

| ID | Requirement |
| --- | --- |
| RF-010 | Tracks can be added, removed, renamed and reordered. A track can be hidden (visual) or muted (audio) |
| RF-011 | Place an item on a track at a time, from an asset or as text |
| RF-012 | Move, trim at either end, split at a time and delete items. Items on one track never overlap; an attempt that would overlap is refused with the conflicting item and the next free position |
| RF-013 | Undo and redo, per actor, any number of steps |
| RF-014 | Delete with ripple, and close a gap, so following items on the chosen tracks move earlier |
| RF-015 | Change the speed of a clip to a constant factor between 0.25 and 4, or along a speed curve; its audio follows without pitch change |
| RF-016 | Remove silences from a clip: a command that finds pauses in speech above a threshold and cuts them, as one undoable step |

### 4.3 Picture

| ID | Requirement |
| --- | --- |
| RF-020 | A visual item fills the frame by default (cover). It can instead be fitted (contain) |
| RF-021 | A visual item has position, scale, rotation, crop and opacity |
| RF-022 | Images and video can be layered on upper tracks as overlays (picture in picture, logo) |
| RF-023 | Transitions at an item's entry and exit: fade, slide from each of four sides, zoom, whip, dip to colour, wipe, and a crossfade helper between two clips. Each has a duration |
| RF-024 | Keyframes over time for position, scale, rotation and opacity, with easing linear, ease-in, ease-out and ease-in-out |
| RF-025 | Brightness, contrast, saturation, temperature and tint per visual item, and a colour lookup table |
| RF-026 | Shape items: rectangle, rounded rectangle and ellipse, with fill and stroke, for backgrounds and lower thirds |
| RF-027 | Blur, a short fixed list of filters and of effects (glow, grain, motion blur, RGB split, shake, vignette), a shadow and an outline |
| RF-028 | Animated images (GIF) and stickers as overlay items |
| RF-029 | Chroma key, blend modes and a shape mask with feather per visual item |

### 4.4 Text and captions

| ID | Requirement |
| --- | --- |
| RF-030 | Text items with content, a font from the bundled set, size, colour, stroke, shadow, background box, alignment and position |
| RF-031 | Caption items: timed text burned into the picture, with a style shared by all captions of a track |
| RF-032 | Captions with per-word timing and a highlight style for the word being spoken |
| RF-033 | Automatic transcription of a clip's speech into caption items |
| RF-034 | Edit captions: change a word, retime, split and merge captions, change line breaks; import subtitle files |
| RF-035 | Animated text presets (entrance and exit) that expand into keyframes |
| RF-036 | Text is never placed where platform interface covers it unless the user chooses to: a safe-area guide in the UI and a lint rule |

### 4.5 Audio

| ID | Requirement |
| --- | --- |
| RF-040 | A clip's own audio plays and exports with it |
| RF-041 | Audio items on audio tracks (music, voice), with volume per item and fade in and fade out |
| RF-042 | Loudness of the exported mix is normalized to a project target, -14 LUFS integrated by default |
| RF-043 | Lower the music automatically under speech (ducking), as generated volume keyframes |
| RF-044 | Markers on the beats of a music item, to cut on the rhythm |

### 4.6 Templates

| ID | Requirement |
| --- | --- |
| RF-050 | Save a timeline as a template with named slots for media, text, captions and colours |
| RF-051 | Create a timeline from a template by filling its slots; unfilled required slots are reported |
| RF-052 | Export and import a template as a file |
| RF-053 | A brand kit per workspace: logo, colours and fonts that templates can refer to |

### 4.7 Preview and export

| ID | Requirement |
| --- | --- |
| RF-060 | Play, pause, seek and scrub the timeline in the UI with picture and sound |
| RF-061 | When paused, the picture shown is the exact frame the export will produce |
| RF-062 | Export to MP4 with H.264 video and AAC audio at the timeline's size and rate |
| RF-063 | An export runs as a job with real progress and can be cancelled. It is reported as successful only after the file passed verification: both streams present, duration within one frame of the timeline, expected dimensions and pixel format, fast-start layout |
| RF-064 | Verified export at 60 fps and at 4K where the machine's encoder supports it; export GIF, audio only and a full-resolution PNG of one frame |
| RF-065 | Export and import OpenTimelineIO |

### 4.8 User interface

| ID | Requirement |
| --- | --- |
| RF-070 | The UI has a media library, a preview, a multi-track timeline and an inspector for the selected item |
| RF-071 | Every command of the catalogue can be performed from the UI |
| RF-072 | Changes made by any actor appear in an open UI without reloading |
| RF-073 | The UI is available in English and Brazilian Portuguese |
| RF-074 | Advanced and customizable keyboard shortcuts with a cheat sheet, onboarding and interface polish; the layout and the default shortcuts are familiar to a CapCut user |
| RF-075 | Controls outside the canvas are operable by keyboard and meet WCAG 2.2 level AA for contrast, focus and names |

### 4.9 Motion, audio and export scope added for Stuarty (ADR-0042)

| ID | Requirement |
| --- | --- |
| RF-080 | Keyframes have editable easing curves, beyond the four fixed easings |
| RF-081 | Crop and mask can be keyframed |
| RF-082 | Animation presets for any visual item, expanding into keyframes |
| RF-083 | Nested compositions: a timeline can be used as an item of another timeline of the same project |
| RF-084 | A clip made in an external motion engine enters the timeline and the asset library with a record of where it came from, so it can be rendered again and replaced |
| RF-085 | Audio processors per item and per track: equalizer, compressor, limiter, gate and de-esser |
| RF-086 | Noise reduction of speech, computed on the user's machine |
| RF-087 | Export presets per platform, each stating size, rate, codec, bitrate and container |
| RF-088 | A batch export queue: several outputs of one project, each in the format of its destination |
| RF-089 | Export H.265 and AV1 where a system or hardware encoder exists, within the codec policy of ADR-0011 |

## 5. Non-functional requirements

Each number is measured on the reference machine named in the test strategy (SDD-008).

| ID | Requirement |
| --- | --- |
| NFR-001 | A command round trip (request to applied state returned), local, takes at most 50 ms at the 95th percentile on a project of 200 items |
| NFR-002 | A change made by one actor reaches another actor's open UI within 200 ms |
| NFR-003 | While scrubbing, a proxy frame appears within 100 ms of a seek |
| NFR-004 | After pausing, the exact frame appears within 500 ms at preview resolution |
| NFR-005 | Ingest of a 60 s 1080p phone clip completes within 30 s |
| NFR-006 | Export of a 60 s 1080×1920 30 fps project with captions completes within 60 s with a hardware or operating system encoder |
| NFR-007 | The Go process stays under 500 MB of resident memory during an export |
| NFR-008 | The browser tab stays under 1 GB with a 5 minute project open |
| NFR-009 | A project of 5 minutes, 200 items and 2,000 caption words is fully supported at the numbers above |
| NFR-010 | The same project and media produce identical composed frames on two runs on the same machine |
| NFR-011 | During playback the preview differs from the export by at most the tolerance of ADR-0003, and of ADR-0029 where the Rust renderer draws both |
| NFR-012 | A project saved by any released version opens in every later version |
| NFR-013 | No committed change is lost by a crash of the process |

## 6. Success metrics

1. The reference scenario runs as an automated end-to-end test through MCP, through the CLI and in the UI.
2. An agent given only the shipped skill file and the command help completes the reference scenario without human help in at least 9 of 10 runs.
3. Every command of the catalogue has a golden test, an undo round-trip test and a UI entry point.
4. A 60 second vertical video with automatic captions goes from import to exported file in under 5 minutes of wall time on the reference machine.

## 7. Out of scope

| Feature | Why |
| --- | --- |
| Motion tracking, expressions, particles, 3D, rotoscoping | Outside the most used fifth of a motion tool (ADR-0042); complex motion is made in an external engine (RF-084) |
| Advanced colour grading, scopes | Professional tooling |
| Multicam, plugins | Professional tooling |
| Spectral audio editing, studio multitrack mixing | Professional tooling (ADR-0042) |
| ProRes, DNxHR and other finishing formats | Outside export for the Internet (ADR-0042) |
| Recording camera, screen or voice | A different product surface |
| Stock media library | Licensing (author decision D-014) |
| Real-time collaboration between several humans, comments | No requirement; the operation log allows it later |
| Accounts, quotas, billing, a hosted service | Stuarty is self-hosted (author decision D-003) |
| Feature parity with any other editor as a goal | The scope is the list above (ADR-0042) |

Generating media with AI, publishing to social networks, writing and image editing are in scope of Stuarty and are specified in [PRD-004](004-creator-platform.md).
