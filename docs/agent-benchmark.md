# Agent benchmark: the model ladder

Can a modest AI agent edit video with Vukem Video almost as well as a large one? This document answers that question release after release. Each round runs three agents, one per model tier (Claude Haiku, Sonnet and Opus, all at medium effort), through exactly the same fixed script of tasks, as first-class users of the product. Every gap between the smallest and the largest model is treated as a defect of the product (documentation, error messages, schemas, MCP, CLI, HTTP API or UI), not of the model, and the goal is that the gap shrinks from one round to the next.

## 1. Method

- The three agents start with a clean context and read only what a user can read: the documentation site, the `--help` of the CLI, the MCP tool list and schemas, and the OpenAPI document served by the server. They never read the source code while working.
- They run in parallel, each with its own server, workspace and desktop window, and receive the same briefing word for word.
- The script has nine blocks: setup, HTTP API, CLI, MCP, every control of the web UI, every control of the desktop shell, production, observed performance and a retrospective.
- Production means five videos to a fixed client brief, each through a fixed surface: a 30 s commercial short (MCP), a 45 s educational short with automatic captions (CLI), a 60 s meme and cuts short (HTTP API), a 2 min motion design piece (web UI) and a 5 min video (MCP and the desktop shell).
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
