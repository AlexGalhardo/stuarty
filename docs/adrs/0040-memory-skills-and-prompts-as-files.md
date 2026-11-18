# ADR-0040: Memory, skills and prompts are files in the workspace, with an index

- Status: Accepted (2026-11-18)
- Deciders: author
- Links: author decisions D-063, D-071, D-042 and D-043 (absorbed); ADR-0009, ADR-0031, ADR-0037; PRD-004

## Context

The dashboard has sections for Memory (long-term learnings), Skills and Prompts. Earlier decisions kept memory and skills as Markdown files in a plugin outside the product. The Agent Skills format (a folder with a `SKILL.md`) is what coding agents read today, and public registries of skills exist; some entries there imitate well-known projects and show inflated install counts (research note `web/06-skills-open-source`).

## Decision

1. **The content is files.** Memory entries, skills and prompts are Markdown files with front matter inside the workspace, readable by a person and by any agent with file access. Skills use the Agent Skills format, so they work outside Stuarty too.
2. **SQLite holds only an index** for search and for the UI, rebuilt from the files; the files are the truth.
3. **Three kinds of skill:**
   - **Generated:** written by the product from its operation catalogues (the video editor's exists today), versioned with the binary and never edited by hand.
   - **Curated:** shipped with the product, reviewed and versioned with it.
   - **The user's and third parties':** shown with their origin, licence and full text before they can be enabled.
4. **A skill is code and instruction, so it is untrusted until read.** Nothing is installed from a registry silently, and a third-party skill gets no access to the vault (ADR-0033).
5. **Sources for the curated set:** `coreyhaines31/marketingskills` (MIT) and `heygen-com/hyperframes` (Apache-2.0), with attribution, review and adaptation. The briefing skill and the skills of Stuarty's own editors are written by the project.
6. **Memory is a step of the flow.** After a production, what was learned is written to Memory; before planning the next one, it is read.

## Alternatives

- **Everything in the database.** Rejected: an outside agent could not read it without going through the API.
- **A plugin outside the product that the UI only displays.** Rejected: it was the earlier direction, and the sections would only list a folder.

## Consequences

Positive: portable, readable, and the same files serve the hosted agent and an agent in a terminal.

Negative: the index must follow edits made outside the product.

Negative: curating and translating third-party skills is editorial work that repeats when their sources change.
