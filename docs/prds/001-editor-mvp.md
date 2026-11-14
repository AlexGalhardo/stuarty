# PRD-001: The editor

- Status: Accepted (2026-11-14)
- Depends on: [brainstorming](../brainstorming.md) D-001, D-002, D-009, D-013, D-014; [features 20/80](../search/features-20-80.md); [performance](../search/performance.md)
- Implemented by: SDD-001, SDD-002, SDD-004, SDD-005, SDD-006, SDD-011, SDD-012 (see [traceability](../traceability.md))

## 1. Problem

Making a short vertical video with captions, or a product commercial with animated text and music, takes a non-linear editor that is either a closed product with paywalls and watermarks or an open project whose engine is tied to its browser UI. None of them can be driven end to end by an AI agent from a terminal with no window open, and none lets a person and an agent work on the same project and see the same result.

## 2. Users

| User | What they do | What they need |
| --- | --- | --- |
| The agent operator | A developer or technical creator who already uses a coding agent (Claude Code, Codex, OpenCode). Describes the video; the agent edits | Commands an agent can discover, run and verify; a result they can open and adjust by hand |
| The hands-on editor | Someone who edits in the UI, CapCut style | A timeline, a preview and an inspector that cover everything; no surprise paywall |
| The self-hoster | Runs the product on their own machine or server | One binary, no cloud account, nothing sent out |

Agents themselves are users of the interfaces; their requirements are in [PRD-002](002-agent-operability.md).

## 3. Use cases

1. **Short video** of 30 seconds to 5 minutes for Instagram, TikTok, Kwai and X: memes, cuts of longer footage, informative talking-head videos. Vertical 9:16 by default. Captions are mandatory in practice.
2. **Commercial video** for a physical or digital product: animated text, transitions, a logo, music, a reusable template.

Reference scenario for the Prototype: an agent, through MCP and through the CLI, imports a video, cuts it, adds a caption and exports a valid MP4; the same project opens correctly in the UI.

## 4. Functional requirements

Phase: `P` Prototype, `A` ALPHA, `B` BETA. "MVP" (decision D-009) is P plus A.

### Projects and media

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-001 | Create, list, open and rename projects. Every change is saved as it happens; there is no save action | P |
| RF-002 | A project has a frame size and a frame rate. Presets: 9:16 (1080×1920, default), 1:1 (1080×1080), 16:9 (1920×1080), 4:5 (1080×1350). Default rate 30 fps. Changing size or rate is an explicit command that reports what it rounded | P (size and rate chosen when the project is created; changing the size), A (changing the rate) |
| RF-003 | Import video, audio and image files by local path. Import starts an ingest with visible progress; a file that cannot be used is reported with the reason and how to fix it | P |
| RF-004 | A media library lists a project's assets with duration, dimensions, a thumbnail and ingest state | P |
| RF-005 | Move projects and assets to a trash and restore them from it | A |
| RF-006 | Export a project as a bundle (document, history and media) and import it on another machine | B |

### Timeline

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-010 | Tracks can be added, removed, renamed and reordered. A track can be hidden (visual) or muted (audio) | P |
| RF-011 | Place an item on a track at a time, from an asset or as text | P |
| RF-012 | Move, trim at either end, split at a time and delete items. Items on one track never overlap; an attempt that would overlap is refused with the conflicting item and the next free position | P |
| RF-013 | Undo and redo, per actor, any number of steps | P |
| RF-014 | Delete with ripple, and close a gap, so following items on the chosen tracks move earlier | P (delete with ripple), A (close a gap) |
| RF-015 | Change the speed of a clip to a constant factor between 0.25 and 4; its audio follows without pitch change | A |
| RF-016 | Remove silences from a clip: a command that finds pauses in speech above a threshold and cuts them, as one undoable step | A |

### Picture

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-020 | A visual item fills the frame by default (cover). It can instead be fitted (contain) | P |
| RF-021 | A visual item has position, scale, rotation, crop and opacity | A |
| RF-022 | Images and video can be layered on upper tracks as overlays (picture in picture, logo) | A |
| RF-023 | Transitions at an item's entry and exit: fade, slide from each of four sides, and a crossfade helper between two clips. Each has a duration | A |
| RF-024 | Keyframes over time for position, scale, rotation and opacity, with easing linear, ease-in, ease-out and ease-in-out | A |
| RF-025 | Brightness, contrast and saturation per visual item | A |
| RF-026 | Shape items: rectangle, rounded rectangle and ellipse, with fill and stroke, for backgrounds and lower thirds | A |
| RF-027 | Blur and a short fixed list of filters | B |
| RF-028 | Animated images (GIF) and stickers as overlay items | B |

### Text and captions

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-030 | Text items with content, a font from the bundled set, size, colour, stroke, shadow, background box, alignment and position | P |
| RF-031 | Caption items: timed text burned into the picture, with a style shared by all captions of a track | P |
| RF-032 | Captions with per-word timing and a highlight style for the word being spoken | A |
| RF-033 | Automatic transcription of a clip's speech into caption items | A |
| RF-034 | Edit captions: change a word, retime, split and merge captions, change line breaks | A |
| RF-035 | Animated text presets (entrance and exit) that expand into keyframes | A |
| RF-036 | Text is never placed where platform interface covers it unless the user chooses to: a safe-area guide in the UI and a lint rule | A |

### Audio

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-040 | A clip's own audio plays and exports with it | P |
| RF-041 | Audio items on audio tracks (music, voice), with volume per item and fade in and fade out | A |
| RF-042 | Loudness of the exported mix is normalized to a project target, -14 LUFS integrated by default | A |
| RF-043 | Lower the music automatically under speech (ducking), as generated volume keyframes | B |
| RF-044 | Markers on the beats of a music item, to cut on the rhythm | B |

### Templates

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-050 | Save a project as a template with named slots for media, text, captions and colours | A |
| RF-051 | Create a project from a template by filling its slots; unfilled required slots are reported | A |
| RF-052 | Export and import a template as a file | A |
| RF-053 | A brand kit per workspace: logo, colours and fonts that templates can refer to | B |

### Preview and export

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-060 | Play, pause, seek and scrub the timeline in the UI with picture and sound | P |
| RF-061 | When paused, the picture shown is the exact frame the export will produce | P |
| RF-062 | Export to MP4 with H.264 video and AAC audio at the project's size and rate | P |
| RF-063 | An export runs as a job with real progress and can be cancelled. It is reported as successful only after the file passed verification: both streams present, duration within one frame of the timeline, expected dimensions and pixel format, fast-start layout | P (an export runs as a job with real progress and is reported as successful only after the file passed verification), A (cancellation from every surface) |
| RF-064 | Verified export at 60 fps and at 4K where the machine's encoder supports it (the document format accepts these sizes and rates from the start; before BETA they are not covered by tests or by the bitrate table); export GIF and audio only | B |
| RF-065 | Export and import OpenTimelineIO | B |

### User interface

| ID | Requirement | Phase |
| --- | --- | --- |
| RF-070 | The UI has a media library, a preview, a multi-track timeline and an inspector for the selected item | P (library, preview, timeline, the four manual operations of D-023), A (inspector and the rest) |
| RF-071 | Every command of the catalogue can be performed from the UI | A |
| RF-072 | Changes made by any actor appear in an open UI without reloading | P |
| RF-073 | The UI is available in English and Brazilian Portuguese | A |
| RF-074 | Advanced and customizable keyboard shortcuts with a cheat sheet, onboarding and interface polish (the basic shortcuts of the editor arrive with the ALPHA UI, SDD-012 §18.2) | B |
| RF-075 | Controls outside the canvas are operable by keyboard and meet WCAG 2.2 level AA for contrast, focus and names | B |

## 5. Non-functional requirements

Each number is measured on the reference machine named in the test strategy (SDD-008) and is confirmed or corrected in the Prototype.

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
| NFR-011 | During playback the preview differs from the export by at most the tolerance of ADR-0003 (SSIM of at least 0.97 on luma; no layer displaced by more than 1 pixel at preview resolution) |
| NFR-012 | A project saved by any released version opens in every later version |
| NFR-013 | No committed change is lost by a crash of the process |

## 6. Success metrics

1. The reference scenario runs as an automated end-to-end test through MCP, through the CLI and in the UI, on every pull request.
2. An agent given only the shipped skill file and the command help completes the reference scenario without human help in at least 9 of 10 runs.
3. Every command of the catalogue has a golden test, an undo round-trip test and a UI entry point.
4. A 60 second vertical video with automatic captions goes from import to exported file in under 5 minutes of wall time on the reference machine.

## 7. Out of scope

| Feature | Why |
| --- | --- |
| AI video generation, upscale | Not editing; needs models and services |
| Chroma key, background removal | Expensive; background removal needs a model |
| Motion tracking, masks, blend modes | Outside the 20% |
| Advanced colour grading, scopes | Professional tooling |
| Multicam, nested timelines, plugins | Professional tooling |
| Audio mixer, equalizer, noise reduction | Professional tooling; noise reduction needs models or GPL filters |
| Speed ramps | Complicates caption timing for little gain |
| Recording camera, screen or voice | A different product surface |
| Text to speech, caption translation as features | An agent can do the second through caption commands; the first needs a service |
| Stock media library | Licensing (D-014) |
| Publishing to social networks | Would give agents an outbound channel (D-011) |
| Real-time collaboration between several humans, comments | No requirement; the operation log allows it later |
| Accounts, quotas, billing, uploads, object storage | A hosted service comes after 1.0 (D-003) |
| The old brief-to-video flow | May return as an agent on top of the command core (D-002) |
| Safari and Firefox | Chrome first; the design keeps the cost of adding them low |
