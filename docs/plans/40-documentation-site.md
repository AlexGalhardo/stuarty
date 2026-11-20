# 40: Documentation site for Stuarty

## Goal

Someone who has never heard of Stuarty lands on the site, understands in a minute what it is and what stays on their machine, installs it, and reaches a first published short by following one page. Everything else is findable from there, in English and in Portuguese.

## Context and related decisions

- ADR-0024 (the documentation site), ADR-0014 (references generated from the contracts), ADR-0044 (the name), ADR-0031.
- Author decisions D-047, D-049 (no MVP, beta or 1.0 wording), D-053. PRD-003 RF-211, NFR-040, NFR-041.
- SDD-010 (the documentation site). The site keeps npm (rule 9 of `AGENTS.md`).
- Absorbs the earlier plan on the first export in ten minutes and the site audit.

## Scope

- Modules: `site/` (content, navigation, landing page, the generators' output), `internal/tools/clidoc/` and the other generators (only to cover the new domains), `scripts/docs-commands-smoke.sh`, `scripts/first-export-timing.sh`, SDD-010.
- Runs last among the content epics: it documents the sections that are merged when it starts and lists the ones that are not.

## Out of scope

- Deploying the site, its domain and its hosting: repository settings and a domain are the author's (STOP conditions). The site builds locally.
- Marketing pages beyond the landing page, a blog, videos.
- A new design: the design system's tokens are reused.

## Dependencies

02, and each section it documents (at least 04 to 10, 13 to 20, 25, 27, 28, 35, 38).

## Technical design

- **Structure**: Start (what Stuarty is, what leaves your machine and what never does, install on Windows, install with Docker, install on a server), First short video (the flow of epic 38 as a tutorial, with the scripted commands and the UI path), Guides (one per sidebar section, the video editor, the image editor, writing, publishing with one page per network's developer app, AI connections, the agent chat, memory, skills and prompts), Agents (connect over MCP, the CLI, the HTTP API, the generated reference per domain, the approval queue from an agent's point of view), Reference (generated: operations, CLI, MCP tools, errors, configuration), Security and privacy, Troubleshooting, Glossary.
- **Generated, not written**: references come from the registry and the CLI (`task codegen`); `task generate:check` fails when the site is stale. The connector pages include each network's limits from the same data the product validates against, with the source date.
- **Every command on the site runs**: `scripts/docs-commands-smoke.sh` extracts and runs the commands of the tutorial and of the install pages against a local build.
- **The ten-minute promise** (NFR-040): `scripts/first-export-timing.sh` measures install to first export on a clean profile; the number is published on the page it concerns, with the date and the machine.
- **Old content**: every page of the editor-only site is kept, rewritten or removed, with redirects for moved pages; the former product name survives only in the migration page. No page mentions MVP, beta or 1.0.
- **Two languages**: every page exists in both; a check fails on a missing translation.
- **Honest gaps**: a section whose epic is not merged gets no page; the hand-off lists them for a follow-up.

## Tasks

- [x] 1. Amend SDD-010 with the structure; produce the keep, rewrite, remove table for the existing pages. Verify: `task lint:md` exits 0; the table is in the hand-off.
- [x] 2. Extend the generators to every merged domain and wire `task generate:check`. Verify: `task codegen && task generate:check` exits 0.
- [x] 3. Write Start and the three install pages. Verify: `bash scripts/docs-commands-smoke.sh` passes for them.
- [x] 4. Write the first-short tutorial from `scripts/first-flow.sh`. Verify: the smoke script runs the tutorial's commands end to end.
- [x] 5. Write the guides for the merged sections, including one page per network's developer app. Verify: `task lint:site` exits 0.
- [x] 6. Write Agents, Security and privacy, Troubleshooting and the Glossary. Verify: `task lint:site` exits 0.
- [x] 7. Remove or redirect the old pages; search for the former name and for milestone wording. Verify: a grep for the former product name outside the migration page, and for "MVP", "beta" and "1.0" as product stages, returns nothing in `site/src`.
- [x] 8. Translate everything to Brazilian Portuguese and add the missing-translation check. Verify: `task site:build` exits 0 and the check passes.
- [x] 9. Measure the first export time on a clean profile with the machine idle. Verify: the number, the date and the machine are on the page and in the hand-off.

## Test strategy

- `task lint:site`, `task site:build`, the link check of the site build.
- `scripts/docs-commands-smoke.sh` for every command shown.
- The generators' own tests and `task generate:check`.

## Acceptance criteria

1. `task site:build` exits 0 with no broken internal link, in both languages.
2. Every command on the install pages and in the tutorial runs (`bash scripts/docs-commands-smoke.sh`).
3. The reference covers every operation of every merged domain and is generated (`task generate:check`).
4. The landing page states in plain words what leaves the machine and when.
5. The former product name appears only on the migration page; no milestone wording remains.
6. Each network's page shows that network's limits with the source date.

## Risks and mitigation

- **Documenting sections that are still changing.** Mitigation: this epic runs late; references are generated.
- **A domain and hosting decision.** **STOP**: the site's address and deployment are the author's; build locally and leave the question recorded.

## Size

L

## Hand-off for review

The diff of `epic/40-documentation-site`; the pages table; the build and smoke outputs; the sections left without pages; the measured time.
