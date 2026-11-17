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

No round has run yet. Each round adds a section below, and no round is ever removed.

| Round | Date | Version | Commit | Haiku | Sonnet | Opus | Gap | Product issues |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
