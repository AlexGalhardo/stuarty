# Roadmap of Stuarty

The order of the work: which epics exist, what each depends on, and what can run side by side. One epic is one file of this folder (`NN-<slug>.md`), in the format of `.agents/rules/plans.md`. The process is ADR-0045; the product is PRD-001 to PRD-004.

- Status: **Outline (2026-11-18).** The epic files are not written yet. This outline fixes the tracks and their order; the epic files, the dependency graph, the plan of parallel work and the table that maps every requirement to an epic replace it.
- There are no phases and no milestones. The first complete flow to deliver is a short vertical video with captions and a thumbnail, from the briefing to the publication (PRD-004 section 3).
- The earlier plans (001 to 034) were replaced on 2026-11-18 (author decision D-073). What still matters from them is absorbed below: finishing the Rust renderer, reverting to a revision and checkpoints, the dashboard, the final editor UI, animated crop and mask.

## 1. Foundation (sequential, before everything)

| Epic | Why first |
| --- | --- |
| Remove the Gio window | less code to rename; closes ADR-0041 |
| Rename the code to Stuarty, with a workspace migration | touches almost every file; must happen before five branches are open (ADR-0044) |

## 2. Platform track (three seniors)

| Epic | Depends on |
| --- | --- |
| Secret vault and controlled outbound: connections, approval queue, audit (ADR-0032, ADR-0033) | foundation |
| The project as the container of a production (ADR-0039) | foundation |
| Dashboard: sidebar, navigation, profile, theme (PRD-004 section 4.2) | foundation |
| Asset library with origin and licence | project |
| AI provider layer with API keys, cost and ceilings; image and voice generation (ADR-0036) | vault, assets |
| OAuth connections for AI: Sign in with ChatGPT and OpenRouter | provider layer |
| Video generation through providers | provider layer |
| Hosted agent in the chat panel and MCP tools by domain (ADR-0037, ADR-0043) | dashboard |
| Memory, skills and prompts; the curated skills; the guided briefing (ADR-0040) | project, dashboard |
| Writing: scripts and copy tied to the project | provider layer, memory |
| Social connections: the connector framework, the guided setup, YouTube (ADR-0035) | vault |
| Instagram and Facebook | connectors |
| Publishing: calendar, approval queue, scheduler and the desktop background service (ADR-0046) | connectors |
| X and TikTok (draft) | publishing |
| Blog: long-form writing, export, WordPress, webhook, API | writing, publishing |
| Optional Google account and login on a server (ADR-0034) | vault |
| VPS package: compose with Caddy, install, update and backup scripts | Google account |
| Later: metrics, Reddit, the desktop on macOS and Linux | — |

## 3. Editor track (two seniors)

| Epic | Depends on |
| --- | --- |
| Finish the Rust renderer and make it the only one (ADR-0029) | foundation |
| Minimal image document: layers, templates with slots, export through the CPU path (ADR-0038) | foundation |
| Image editor in the UI: layer panel, canvas, inspector | minimal image document, dashboard |
| Image: adjustments, filters, masks, more blend modes; operations through a model | renderer, image editor, provider layer |
| Minimal motion: easing curves, animated crop and mask, presets, nested compositions (ADR-0042) | renderer |
| HyperFrames as an external engine | assets |
| Audio: processors and noise reduction | foundation |
| Export: presets per platform, batch queue, H.265 and AV1 | foundation |
| Revert to a revision, checkpoints and a diff (PRD-002 RF-120, RF-121) | foundation |
| Final editor UI, shortcuts and the open fixes of the feedback queue | dashboard |

## 4. After the epics

The QA campaign of `.agents/rules/plans.md` section 6 runs in rounds until a full round finds no problem. Then the integration pull request is split into releases, starting at v0.30.0, and CI and the branch protection are restored (ADR-0045).
