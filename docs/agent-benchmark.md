# Agent benchmark: the model ladder

Can a modest AI agent edit video with Vukem Video almost as well as a large one? This document answers that question release after release. Each round runs three agents, one per model tier (Claude Haiku, Sonnet and Opus, all at medium effort), through exactly the same fixed script of tasks, as first-class users of the product. Every gap between the smallest and the largest model is treated as a defect of the product (documentation, error messages, schemas, MCP, CLI, HTTP API or UI), not of the model, and the goal is that the gap shrinks from one round to the next.

## 1. Method

- The three agents start with a clean context and read only what a user can read: the documentation site, the `--help` of the CLI, the MCP tool list and schemas, and the OpenAPI document served by the server. They never read the source code while working.
- They run in parallel, each with its own server, workspace and desktop window, and receive the same briefing word for word.
- The script has nine blocks: setup, HTTP API, CLI, MCP, every control of the web UI, every control of the desktop shell, production, observed performance and a retrospective.
- Production means 21 videos of up to 5 minutes, each to a fixed client brief and through a fixed surface (MCP, CLI, HTTP API, web UI or desktop shell), chosen so that together they use every feature of the editor: a product commercial, an educational short with automatic captions, a meme, a cut-to-the-beat edit, motion design, a 5 min YouTube video, a screen tutorial, a podcast clip, a review, a cinematic trailer, a multi-format ad, a green screen piece, a photo slideshow, a lyric video, a before and after, an event recap with mixed sources, a GIF with stickers, a channel intro, a news segment, an audio-only episode, and a project round trip through OTIO and a bundle.
- Every task has assertions checked by a script, never by a judge: duration within one frame, resolution, loudness, captions coverage, black frames, cut timing, lint findings, and the document reopening identical.
- Product performance is measured after the agents stop, on an idle machine, by re-exporting the projects the agents made.

## 2. Score

| Field | Meaning |
| --- | --- |
| Block score | 100 × passed assertions / assertions of the block, minus 10 per intervention, at least 0 |
| Round score | mean of the block scores of the API, CLI, MCP, web UI, desktop and production blocks |
| Gap | Opus score − Haiku score, per block and in total |
| Cost | tokens, tool calls, wall time and interventions per model |

A block with a gap of 15 points or more always yields at least one product issue.

## 3. Rounds

Each round adds a section below, and no round is ever removed.

| Round | Date | Version | Commit | Haiku | Sonnet | Opus | Gap | Product issues |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| L1 | 2026-11-16 | 0.24.0 | v0.24.0 | 12.3 (92.3) | 63.8 (93.8) | 95.6 (95.6) | 83.3 (3.3) | 65 |

Scores are the production block score; the raw pass rate before the intervention penalty is in parentheses.

### L1 (0.24.0, 2026-11-16)

Only the production block and the two basic API and CLI checks were scored by the verifier in this round. The other blocks (MCP, every control of the web UI and of the desktop shell) were walked by the agents and produced issues, but their assertions were not yet checked by a script.

| | Haiku | Sonnet | Opus |
| --- | --- | --- | --- |
| Production assertions passed | 252 / 273 (92.3%) | 258 / 275 (93.8%) | 262 / 274 (95.6%) |
| API basic | 5 / 5 | 4 / 5 | 5 / 5 |
| CLI basic | 5 / 7 | 6 / 7 | 7 / 7 |
| Interventions | 8 | 3 | 0 |
| Production block score | 12.3 | 63.8 | 95.6 |
| Tokens | about 1.64 M | about 0.87 M | about 0.30 M |
| Tool calls | 701 | 467 | 612 |
| Wall time | 2 h 06 | 2 h 18 | 2 h 39 |

What the round showed:

- Every model delivered all 21 videos, and the share of passed assertions differs by only 3.3 points between Haiku and Opus. The large gap in score comes from interventions: Haiku and Sonnet exhausted their context and had to be continued by a new agent with the same briefing (8 and 3 times), while Opus finished in one run. Reducing how much an agent must read and retry per task is therefore the main product lever.
- The failures shared by the three models were product defects: exported loudness below the target, a stale `base_revision` accepted on project-level edits, captions from a transcript created in the same batch, the transcription helper that a person could not install while the server ran, and the desktop window that could not be driven.
- The round produced 65 product issues (1 critical, 4 high, 25 medium and 35 low). They were fixed for 0.25.0, except those that need a product decision.

Product performance, re-exporting the agents' projects on an idle Windows 11 machine with 16 logical cores (median of 3 runs, FFmpeg n9.0.2):

| Project | Length | Export time | Real-time factor | Peak RAM | Mean CPU |
| --- | --- | --- | --- | --- | --- |
| V01, vertical short | 30 s | 21.8 s | 1.37x | 1.5 GiB | 18% |
| V06, long video | 300 s | 142 to 154 s | 1.94x to 2.11x | 1.9 to 2.1 GiB | 17 to 18% |
| V10, 24 fps with heavy colour | 90 s | 56.3 s | 1.60x | 1.7 GiB | 14 to 19% |
| V16, 60 fps mixed sources | 180 s | 146 to 156 s | 1.16x to 1.23x | 2.7 GiB | 16 to 18% |

In the editor, opening the V06 project took 0.63 to 0.78 s, playback advanced 29.2 to 29.5 frames per second, and a click on the ruler moved the playhead in 31 to 36 ms. Export used about 3 of the 16 cores, so the compositor has room to run in parallel.
