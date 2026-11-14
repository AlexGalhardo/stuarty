# The 20/80 feature cut

> Stage 1, research. All sources accessed 2026-11-14.
> Marking: **FACT**, **INFERENCE**, **OPINION** (the architect's judgement).
> Evidence: the raw inventory across CapCut, Canva and Clipchamp in [`notes/features.md`](notes/features.md), the product profiles in [`projects/`](projects/), and the author's scope decision D-009 in [`../brainstorming.md`](../brainstorming.md).

## Contents

1. [How to read the table](#1-how-to-read-the-table)
2. [How strong the evidence is](#2-how-strong-the-evidence-is)
3. [The table](#3-the-table)
4. [What the cut says about the product](#4-what-the-cut-says-about-the-product)
5. [Target platform facts that shape the features](#5-target-platform-facts-that-shape-the-features)
6. [Gaps](#6-gaps)

## 1. How to read the table

- **Use case 1**: short videos of 30 seconds to 5 minutes for Instagram, TikTok, Kwai and X: memes, cuts, informative videos. Vertical 9:16, captions mandatory in practice.
- **Use case 2**: commercial videos for physical and digital products, with motion design: animated text, transitions, templates, music, logos.
- **Value**: high, medium or low for that use case.
- **Complexity**: S, M or L. It is the architect's estimate (OPINION) for this architecture: a Go command core, FFmpeg export, our own rasterizer.
- **Phase**: `Prototype`, `ALPHA` and `BETA` together are the road to 1.0. "MVP" in decision D-009 means everything marked Prototype or ALPHA. `Later` means after 1.0 or never.

## 2. How strong the evidence is

Weak, and the table should be read with that in mind.

- **FACT.** The only usage data found is CapCut's own: Text, Effects, Extract Audio and Overlay are its most used features, and auto captions are among its most used AI features. Source: <https://www.capcut.com/newsroom/revealing-the-creative-trends-on-capcut-in-2025>. It is vendor data.
- **FACT.** No independent survey of which editor features creators use was found. A circulating figure of "88% of creators use AI captions" has no accessible method and is not used.
- **INFERENCE.** The value columns are therefore judgement anchored on three things: what all three commercial products offer, what the two use cases need by definition, and the author's own priorities in D-009.

## 3. The table

| Feature | Use case 1 | Use case 2 | Complexity | Phase | Justification |
| --- | --- | --- | --- | --- | --- |
| Import video, audio, image by local path | high | high | M | Prototype | Base of everything. Ingest normalizes phone footage ([`engine.md`](engine.md) §5) |
| Project create, open, list, save | high | high | S | Prototype | Without it there is nothing to edit; saving is automatic because every command is persisted |
| Multi-track timeline | high | high | L | Prototype | Present in all three products; overlays and captions need tracks |
| Cut, split, trim | high | high | M | Prototype | Cuts are use case 1 |
| Move and reorder items | high | high | S | Prototype | Basic editing |
| Undo and redo | high | high | M | Prototype | Follows from the operation log; also the safety net for agents (D-011) |
| Static text | high | high | M | Prototype | CapCut's most used feature |
| Captions as a first-class item, burned in | high | medium | M | Prototype | Captions are mandatory in practice for short video; part of the Prototype's end-to-end scenario |
| Export MP4 H.264 + AAC, 1080p | high | high | M | Prototype | The only format safe for the target networks ([`licensing.md`](licensing.md) §4) |
| Aspect presets 9:16, 1:1, 16:9, 4:5 | high | high | S | Prototype | 9:16 is the default; variants per network matter for use case 2 |
| Frame and contact-sheet inspection | high | high | M | Prototype | How an agent, and a paused human, see the truth ([`agents-mcp.md`](agents-mcp.md) §6) |
| Timeline lint | high | high | M | Prototype | Text feedback for agents; catches overlaps, gaps, missing media |
| Delete with ripple, close gaps | high | medium | S | ALPHA | Rhythm of cuts; a command built from several moves |
| Reframe: crop, scale and position a clip in the frame | high | high | M | ALPHA | Turning 16:9 footage into 9:16 |
| Overlay: picture in picture, logo, image layer | high | high | M | ALPHA | CapCut lists Overlay among its most used |
| Captions with per-word highlight | high | medium | M | ALPHA | The standard look of the format; needs word timings |
| Automatic transcription | high | medium | L | ALPHA | D-009, D-010. Also the agent's main view of what the footage says |
| Edit captions by text and timing | high | medium | M | ALPHA | Transcription always needs correction |
| Music and audio tracks, volume, fade in and out | high | high | M | ALPHA | Present in all three products |
| Loudness normalization at export | medium | high | S | ALPHA | The Studio already targets -14 LUFS; a single pass at render |
| Basic transitions: fade, crossfade, slide | medium | high | M | ALPHA | Present in all three; kept to a short fixed list |
| Text styles, bundled fonts, text presets | medium | high | M | ALPHA | Brand look; fonts are bundled OFL only (D-014) |
| Animated text: entrance and exit presets | high | high | M | ALPHA | Engagement in short video, motion design in commercials; presets expand to keyframes |
| Simple keyframes: position, scale, rotation, opacity | medium | high | L | ALPHA | D-009. The base of motion design for use case 2 |
| Speed change, constant per clip | high | medium | M | ALPHA | Memes and cuts. Speed ramps are out |
| Silence removal | high | low | M | ALPHA | D-009. High value for talking-head cuts; one command that emits cuts |
| Basic colour adjustment: brightness, contrast, saturation | medium | high | M | ALPHA | Needs a route that avoids the GPL `eq` filter ([`licensing.md`](licensing.md) §6) |
| Reusable templates with slots | high | high | L | ALPHA (last) | D-009. Depends on a stable project format, so it is the last ALPHA item |
| Export job with progress and cancel | high | high | M | ALPHA | An agent must never be told "success" for a broken file ([`post-mortems.md`](post-mortems.md) §9) |
| Trash for projects and media | medium | medium | S | ALPHA | D-011: nothing an agent calls destroys data |
| Audio ducking under speech | medium | high | M | BETA | A command that writes volume keyframes; needs transcription or speech detection first |
| Blur and a short list of filters | medium | medium | M | BETA | Needs LGPL-safe filters or own code |
| Stickers, emoji and GIF as image layers | medium | low | S | BETA | Cheap once overlays exist; colour emoji in text depends on the rasterizer |
| Export GIF and audio only | medium | low | S | BETA | Memes; cheap through FFmpeg |
| Export 60 fps and 4K | medium | medium | S | BETA | Depends on the encoder available on the machine |
| Brand kit: logo, colours, fonts saved per workspace | low | high | M | BETA | Builds on templates |
| Beat markers for cutting to music | medium | medium | M | BETA | Audio analysis; a nice command for agents |
| Keyboard shortcuts, onboarding, UI polish | medium | medium | M | BETA | D-013 |
| Desktop app, packaged | medium | medium | L | BETA | D-004 |
| Interchange: export and import OpenTimelineIO | low | medium | M | BETA | Exit door for projects; not a native format |
| Chroma key and background removal | medium | high | L | Later | Valuable for product shots, expensive, and background removal needs a model |
| Record camera, screen and voice | medium | medium | M | Later | The browser can do it, but it is a different product surface |
| Noise reduction and voice enhance | medium | medium | L | Later | Needs audio models or GPL filters |
| Text to speech | medium | medium | L | Later | External service or model; outside local-first minimalism |
| Caption translation | medium | medium | M | Later | An agent can already do it through the caption commands |
| Stock library of music, video, images | medium | high | L | Later | Licensing is the problem (D-014) |
| Publish directly to social networks | medium | medium | M | Later | Third-party APIs and tokens; it would also give agents an outbound channel, against D-011 |
| Brief to video, the old Studio flow | medium | medium | M | Later | Can return as an agent that drives the command core (D-002) |
| Masks and blend modes | low | medium | L | Later | Outside the 20% |
| Motion tracking | low | medium | L | Later | Outside the 20% |
| AI video generation, upscale | low | medium | L | Later | Outside the product |
| Real-time collaboration between several humans, comments | low | medium | L | Later | The operation log allows it later; no requirement now |
| Advanced colour grading, scopes, LUT nodes | low | low | L | Later | Professional tooling |
| Multicam, compound clips, plugins | low | low | L | Later | Professional tooling |
| Equalizer and multi-bus mixer | low | low | L | Later | Professional tooling |
| Speed ramps | medium | low | L | Later | Complicates the time mapping of captions for little gain |
| SaaS: accounts, quotas, billing, uploads, object storage | n/a | n/a | L | Later | D-003 |

## 4. What the cut says about the product

- **INFERENCE.** The Prototype column is exactly the end-to-end scenario the briefing asks for: an agent imports a video, cuts it, adds a caption and exports a valid MP4, and the same project shows correctly in the UI. Nothing else is in the Prototype.
- **INFERENCE.** In CapCut the selling point is "stylized automatic captions, a template and a vertical export", not the non-linear editor around it ([`projects/capcut.md`](projects/capcut.md) §9). The ALPHA column delivers that same core.
- **OPINION.** D-009 puts 18 features in ALPHA. That is a large phase. The ROADMAP splits it into task groups that can each be released, with templates last.
- **INFERENCE.** Several ALPHA features are one command built on others, which is cheap in this architecture: ripple delete, silence removal, animated text presets and ducking all expand into moves, cuts or keyframes. That is why they are in, while features that need a model or a new pixel operation are out.

## 5. Target platform facts that shape the features

| Platform | What was verified in an official source | What was not |
| --- | --- | --- |
| YouTube Shorts | up to 3 minutes; vertical or square; full upload encoding recommendations | safe zone |
| X | H.264 High, AAC-LC, at most 60 fps, 4:2:0, no open GOP; 0.5 s to 20 min for a standard post | container, safe zone |
| TikTok | ads: 9:16 at least 540×960, at least 516 kbit/s, at most 500 MB and 10 minutes; the posting API accepts up to 4 GB | organic upload limits, safe zone percentages |
| Instagram Reels | ads guide: safe zone of about 14% top, 35% bottom, 6% each side | organic maximum duration, codec |
| Kwai | nothing | everything |

- **FACT.** Sources are in [`notes/features.md`](notes/features.md). Several official pages block automated readers.
- **OPINION.** One default preset serves all: MP4, H.264 with AAC, 1080×1920, 30 fps. The only safe zone with an official source is Meta's for ads, so the lint rule for text in the safe area uses those margins until the other templates are obtained. Kwai is treated like TikTok, without evidence, and this is stated in the product documentation.

## 6. Gaps

- No independent usage data per feature.
- No official specification for Kwai; partial ones for TikTok, Instagram and X.
- Cells marked "s?" and "?" in the raw inventory are inference, not fact.
- Prices and plan limits of the commercial products diverge between sources and must not be quoted.
